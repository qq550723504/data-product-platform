"""Protected continuation and strict physical-call ledger closure, no retained I/O."""
import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch
import uuid

import protected_resume as p
import runtime as r
import resume as c


def uid(): return str(uuid.uuid4())


class ProtectedResumeContracts(unittest.TestCase):
    def fixture(self):
        s={"runId":"synthetic-run","workspaceId":uid(),"actorId":uid(),"phase":"RUNNING","activationBinding":{},"activation":{"status":"PASS"}}
        ws,actor=s['workspaceId'],s['actorId'];subject=p.identity(s)['principalRef'];versions=[]
        tables={name:[] for name in ('dataset','dataset_version','schema_migration','lifecycle_protected_object_intent','lifecycle_storage_call','lifecycle_storage_observation','cost_event','cost_allocation','evidence','evidence_relation','audit_event','outbox_event','outbox_event_consumption','rights_declaration','execution_input')}
        tables['schema_migration']=[{'version':71}]
        tables['rights_declaration']=[{'id':uid(),'workspace_id':ws,'claimant_ref':'frozen'}]
        tables['execution_input']=[{'execution_id':uid(),'dataset_version_id':uid()}]
        tables['evidence']=[{'id':uid(),'title':'original','source_type':'OLD','source_id':uid()}]
        for kind in ('RAW','RAW','RAW','STANDARDIZED','CURATED'):
            ds,version,intent=uid(),uid(),uid();sha='a'*64
            versions.append({'id':version,'sha256':sha,'bytes':12,'uri':'s3://raw/core/'+ws+'/'+version})
            tables['dataset'].append({'id':ds,'workspace_id':ws,'dataset_type':kind})
            tables['dataset_version'].append({'id':version,'dataset_id':ds,'status':'READY','checksum_algorithm':'SHA256','checksum_value':sha,'byte_size':12,'storage_uri':versions[-1]['uri']})
            tables['lifecycle_protected_object_intent'].append({'id':intent,'workspace_id':ws,'dataset_version_id':version,'content_sha256':sha,'byte_size':12})
        base=copy.deepcopy(tables)
        for intent in tables['lifecycle_protected_object_intent']:
            for operation in ('PROTECTED_HEAD','PROTECTED_GET'):
                call,observation,cost,evidence_ids=uid(),uid(),uid(),[uid(),uid()]
                at='2026-10-21T00:00:00Z' if operation=='PROTECTED_HEAD' else '2026-10-21T00:00:01Z'
                tables['lifecycle_storage_call'].append({'id':call,'operation':operation,'workspace_id':ws,'actor_id':actor,'principal_ref':subject,'protected_object_intent_id':intent['id'],'copy_id':None,'reader_pin_id':None,'dependency_revision':19,'started_at':at})
                tables['lifecycle_storage_observation'].append({'id':observation,'workspace_id':ws,'call_id':call,'invoked_at':at,'observed_at':at,'http_status':200,'failure':'','response_sha256':r.digest(b'') if operation=='PROTECTED_HEAD' else intent['content_sha256']})
                tables['cost_event'].append({'id':cost,'workspace_id':ws,'execution_id':None,'activity_id':call,'cost_type':'LIFECYCLE_PROVIDER_INVOCATION','quantity':1,'unit':'invocation','amount':None,'currency':None,'pricing_mode':'ACTUAL','occurred_at':at})
                tables['cost_allocation'].append({'id':uid(),'cost_event_id':cost,'lifecycle_storage_call_id':call,'quality_assessment_id':None})
                for ix,(fact,event,subject_type,fields) in enumerate([(call,'LifecycleStorageCallStarted','LIFECYCLE_STORAGE_CALL',{'operation':operation,'protected_object_intent_id':intent['id']}),(observation,'LifecycleStorageCallObserved','LIFECYCLE_STORAGE_OBSERVATION',{'call_id':call,'http_status':200,'invocation_confirmed':True,'failure':''})]):
                    meta=dict(fields,fact_id=fact,workspace_id=ws,dependency_revision=19)
                    tables['outbox_event'].append({'id':uid(),'aggregate_id':fact,'aggregate_type':subject_type,'event_type':event,'payload':meta,'status':'PENDING','attempts':0,'required_handlers':[]})
                    tables['audit_event'].append({'id':uid(),'object_id':fact,'object_type':subject_type,'action':event,'workspace_id':ws,'actor_id':actor,'metadata':meta})
                    tables['evidence'].append({'id':evidence_ids[ix],'source_id':fact,'source_type':subject_type,'title':event,'workspace_id':ws,'created_by':actor,'evidence_type':'LIFECYCLE_OCCUPANCY','metadata':meta})
                    tables['evidence_relation'].append({'id':uid(),'evidence_id':evidence_ids[ix],'object_id':fact,'object_type':subject_type,'relation_type':'CURRENT_CONTROL_FACT'})
        columns={name:sorted({k for row in values for k in row}) for name,values in tables.items()}
        def inventory(values):
            return {'contract':p.CONTRACT,'workspaceId':ws,'identity':p.identity(s),'expected':{'schema':71,'schemaVersions':[71],'versions':versions},'source':{'enterprise':[1]*6,'lease':[1]*10,'energy':[1]*14},'inventory':{'tables':{name:{'columns':columns[name],'rows':[json.dumps(row,sort_keys=True) for row in rows]} for name,rows in values.items()}}}
        return s,inventory(base),inventory(tables)

    def mutate(self,value,table,call):
        rows=[json.loads(v) for v in value['inventory']['tables'][table]['rows']]
        call(rows)
        value['inventory']['tables'][table]['rows']=[json.dumps(row,sort_keys=True) for row in rows]

    def test_exact_five_head_get_pairs_accept_actual_cost_and_evidence_appends(self):
        s,before,after=self.fixture()
        result=p.compare(s,before,after,probe=True)
        self.assertEqual(result['verifiedVersions'],5);self.assertEqual(len(result['callIds']),10)
        self.assertFalse(result['runtimeRootFallback'])
        p.compare(s,before,copy.deepcopy(before))
        with self.assertRaises(r.Refused):p.compare(s,before,after)

    def test_historical_facts_and_unrelated_appends_refused(self):
        for table,change in [('rights_declaration',lambda rows:rows[0].update(claimant_ref='changed')),('dataset_version',lambda rows:rows[0].update(status='INVALID')),('execution_input',lambda rows:rows.clear()),('evidence',lambda rows:rows[0].update(title='changed')),('rights_declaration',lambda rows:rows.append({'id':uid()}))]:
            with self.subTest(table=table):
                s,before,after=self.fixture();self.mutate(after,table,change)
                with self.assertRaises(r.Refused):p.compare(s,before,after,probe=True)

    def test_missing_cost_or_evidence_and_extra_or_foreign_call_refused(self):
        for table,change in [('cost_event',lambda rows:rows.pop()),('cost_allocation',lambda rows:rows.pop()),('evidence_relation',lambda rows:rows.pop()),('audit_event',lambda rows:rows.pop()),('lifecycle_storage_call',lambda rows:rows.append(dict(rows[0],id=uid()))),('lifecycle_storage_call',lambda rows:rows[0].update(operation='PROTECTED_PUT')),('lifecycle_storage_call',lambda rows:rows[0].update(operation='PROTECTED_GET')),('lifecycle_storage_call',lambda rows:rows[0].update(principal_ref='foreign')),('lifecycle_storage_call',lambda rows:rows[0].update(workspace_id=uid())),('lifecycle_storage_call',lambda rows:rows[1].update(started_at='2026-10-20T00:00:00Z')),('lifecycle_storage_observation',lambda rows:rows[0].update(response_sha256='b'*64)),('cost_allocation',lambda rows:rows[0].update(quality_assessment_id=uid()))]:
            with self.subTest(table=table):
                s,before,after=self.fixture();self.mutate(after,table,change)
                with self.assertRaises(r.Refused):p.compare(s,before,after,probe=True)

    def test_matching_but_false_metadata_across_all_facts_refused(self):
        for field,value in [('workspace_id',uid()),('dependency_revision',99),('fact_id',uid()),('protected_object_intent_id',uid())]:
            s,before,after=self.fixture()
            for table,key in [('outbox_event','payload'),('audit_event','metadata'),('evidence','metadata')]:
                self.mutate(after,table,lambda rows,key=key:next(v for v in rows if key in v)[key].update({field:value}))
            with self.assertRaises(r.Refused):p.compare(s,before,after,probe=True)

    def test_outbox_can_publish_without_changing_frozen_payload(self):
        s,before,after=self.fixture()
        # Treat completed probe as the next pre-host baseline.
        before=copy.deepcopy(after)
        def publish(rows):
            for row in rows:row.update(status='PUBLISHED',attempts=1,claimed_by='outbox-dispatcher',claimed_at='now',claim_token=uid(),last_error=None,published_at='now',available_at='now')
        # Production snapshots have these authored columns even before claim.
        for value in (before,after):value['inventory']['tables']['outbox_event']['columns']+=['claimed_by','claimed_at','claim_token','last_error','published_at','available_at']
        self.mutate(after,'outbox_event',publish);p.compare(s,before,after)
        self.mutate(after,'outbox_event',lambda rows:rows[0]['payload'].update(fact_id=uid()))
        with self.assertRaises(r.Refused):p.compare(s,before,after)

    def test_protected_inspection_never_falls_back_to_legacy_get(self):
        s,baseline,_=self.fixture();s['activationBinding']={'path':'bound'}
        with patch.object(p,'observe',return_value=baseline) as observe,patch.object(r,'command',side_effect=AssertionError('legacy GET')):
            self.assertEqual(r.inspect_facts(s),baseline);observe.assert_called_once_with(s)
        s['activation']['status']='UNKNOWN'
        with patch.object(r,'command') as command:
            with self.assertRaises(r.Refused):r.inspect_facts(s)
            command.assert_not_called()

    def test_failed_probe_keeps_after_inventory_and_does_not_retry(self):
        s,before,after=self.fixture()
        with tempfile.TemporaryDirectory() as d:
            folder=Path(d);s['workspaceObjectProfile']='same-profile'
            with patch.object(p,'invoke',side_effect=r.Refused('provider failed')) as invoke,patch.object(p,'observe',return_value=after),patch.object(p,'compare') as compare:
                with self.assertRaises(r.Refused):p.probe(s,before,folder,folder)
                self.assertEqual(r.read(folder/'after-probe-inventory.json'),after)
                invoke.assert_called_once();compare.assert_not_called()

    def test_full_resume_renews_once_probes_before_hosts_and_status_has_no_get(self):
        s,before,after=self.fixture();s.update(phase='STOPPED',private='',binaries={},privateHashes={},secretReferences=[],containers={'core':{'id':'owned-core'}},processes={},activationBinding={'path':'bound'},originSnapshot={'sha256':'kept'})
        with tempfile.TemporaryDirectory() as d,patch.object(r,'STATE',Path(d)/'state'):
            r.STATE.mkdir();s['private']=str(Path(d)/'private');order=[]
            def probe(*args):order.append('probe');return after
            with patch.object(c,'precheck',return_value=({'head':'b'*40,'deltaPaths':[]},before)),patch.object(r,'save'),patch.object(r,'docker'),patch.object(c,'wait_containers'),patch.object(c,'compare_facts',side_effect=lambda st,base:base),patch.object(c,'refresh_identity',side_effect=lambda *args:order.append('renew') or {'nativeSTSInvocations':1,'expiresAt':'future'}) as renew,patch.object(p,'probe',side_effect=probe) as prober,patch.object(c,'launch_hosts',side_effect=lambda *args:order.append('hosts')),patch.object(r,'status',return_value={'phase':'RUNNING'}):
                c.resume(s);c.resume(s)
            self.assertEqual(order,['renew','probe','hosts']);renew.assert_called_once();prober.assert_called_once()

    def test_protected_compare_rejects_old_three_raw_baseline(self):
        s,before,_=self.fixture();s['activationBinding']={'path':'bound'}
        with patch.object(r,'inspect_facts',return_value=before):
            with self.assertRaises(r.Refused):c.compare_facts(s,{'workspaceId':s['workspaceId'],'facts':{'versions':[]}})

    def test_stale_or_unknown_stop_baseline_refused_before_start_or_sts(self):
        for status in ('PASS','UNKNOWN'):
            s,before,_=self.fixture();s.update(phase='STOPPED',activationBinding={'path':'bound'},head='a'*40,sources={},containers={},processes={},ports={},stopInspection={'status':status})
            old={'workspaceId':s['workspaceId'],'facts':{'versions':[]}}
            with patch.object(r,'verify'),patch.object(c,'committed_sources',return_value={}),patch.object(r,'sources',return_value={}),patch.object(r,'command',return_value=b'a'*40),patch('stop_receipts.read',return_value=old),patch.object(r,'docker') as docker,patch.object(c,'refresh_identity') as renew:
                with self.assertRaises(r.Refused):c.precheck(s)
                docker.assert_not_called();renew.assert_not_called()

    def test_matching_protected_precheck_uses_sql_baseline_without_provider_work(self):
        s,before,_=self.fixture();s.update(phase='STOPPED',activationBinding={'path':'bound'},head='a'*40,sources={},containers={},processes={},ports={},stopInspection={'status':'PASS'})
        with patch.object(r,'verify'),patch.object(c,'committed_sources',return_value={}),patch.object(r,'sources',return_value={}),patch.object(r,'command',return_value=b'a'*40),patch('stop_receipts.read',return_value=before),patch.object(p,'expected',return_value=before['expected']),patch.object(p,'invoke') as invoke,patch.object(c,'refresh_identity') as renew:
            binding,baseline=c.precheck(s)
            self.assertEqual(baseline,before);self.assertEqual(binding['head'],'a'*40)
            invoke.assert_not_called();renew.assert_not_called()

    def test_sql_observation_calls_snapshot_and_source_readonly_only(self):
        s,before,_=self.fixture();s['containers']={'source':{'id':'same-source'}}
        source=json.dumps(before['source'])
        def command(args,**kwargs):
            self.assertTrue(r.READ_ONLY_OBSERVATION.get())
            self.assertEqual(args[:4],['docker','exec','-i','same-source'])
            self.assertIn(b'REPEATABLE READ READ ONLY',kwargs['data'])
            return source.encode()
        with patch.object(p,'expected',return_value=before['expected']),patch.object(p,'invoke',return_value=before['inventory']) as invoke,patch.object(r,'command',side_effect=command):
            observed=p.observe(s)
            self.assertEqual(observed['source'],source)
            invoke.assert_called_once_with(s,'snapshot')

    def test_observer_rejects_uncommitted_source_and_changed_cached_binary(self):
        s,_,_=self.fixture();s['sources']={}
        with patch.object(r,'command',return_value=b'a'*40) as command,patch.object(c,'committed_sources',return_value={}),patch.object(r,'sources',return_value={'deploy/trial/runtime.py':'dirty'}):
            with self.assertRaises(r.Refused):p.tool(s)
            self.assertEqual(command.call_count,1)
        with tempfile.TemporaryDirectory() as d:
            s['private']=d;folder=Path(d)/'resume-observations'/'same';folder.mkdir(parents=True)
            binary=folder/'trial-activate.exe';binary.write_bytes(b'bound')
            s['protectedObserver']={'head':'a'*40,'path':str(binary),'sha256':r.digest(b'bound')}
            with patch.object(r,'command',return_value=b'a'*40) as command,patch.object(c,'committed_sources',return_value={}),patch.object(r,'sources',return_value={}):
                self.assertEqual(p.tool(s),binary)
                binary.write_bytes(b'changed')
                with self.assertRaises(r.Refused):p.tool(s)
                self.assertEqual(command.call_count,2)

    def test_protected_renewal_skips_unmetered_raw_probe(self):
        s,_,_=self.fixture();s.update(activationBinding={'path':'bound'},binaries={},secretReferences=[],privateHashes={},resume={})
        with tempfile.TemporaryDirectory() as d:
            private=Path(d);s['private']=str(private)
            (private/'profile.json').write_text('{}')
            old=private/'old-protected.json';old.write_text(json.dumps({'Scope':{'WorkspaceID':s['workspaceId'],'ActorID':s['actorId']}}));s['workspaceObjectProfile']=str(old)
            directory=private/'resume';receipts=private/'receipts';directory.mkdir();receipts.mkdir();calls=[]
            def command(args,**kwargs):
                calls.append([str(v) for v in args])
                if str(args[-1])=='renew-core':
                    (directory/'core-identity.json').write_text('new-private-identity')
                    return json.dumps({'nativeSTSInvocations':1,'expiresAt':'future'}).encode()
                if str(args[0])=='go':(directory/'trial-resume.exe').write_bytes(b'isolated-test-tool')
                return b''
            with patch.object(r,'command',side_effect=command),patch.object(r,'save'):
                c.refresh_identity(s,directory,receipts)
            self.assertEqual(sum(v[-1]=='renew-core' for v in calls),1)
            self.assertFalse(any(v[-1]=='verify-core' for v in calls))

    def test_probe_failure_prevents_host_launch_and_retains_unknown(self):
        s,before,_=self.fixture();s.update(phase='STOPPED',activationBinding={'path':'bound'},binaries={},privateHashes={},secretReferences=[],containers={},processes={},originSnapshot={'sha256':'kept'})
        with tempfile.TemporaryDirectory() as d,patch.object(r,'STATE',Path(d)/'state'):
            r.STATE.mkdir();s['private']=str(Path(d)/'private')
            with patch.object(c,'precheck',return_value=({'head':'b'*40,'deltaPaths':[]},before)),patch.object(r,'save'),patch.object(r,'docker'),patch.object(c,'wait_containers'),patch.object(c,'compare_facts',return_value=before),patch.object(c,'refresh_identity',return_value={'nativeSTSInvocations':1}) as renew,patch.object(p,'probe',side_effect=r.Refused('hash mismatch')),patch.object(c,'launch_hosts') as hosts:
                with self.assertRaises(r.Refused):c.resume(s)
                self.assertEqual(s['phase'],'UNKNOWN');hosts.assert_not_called();renew.assert_called_once()
            with self.assertRaises(r.Refused):c.resume(s)
            renew.assert_called_once()


if __name__=='__main__':unittest.main()
