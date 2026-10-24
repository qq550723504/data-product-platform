"""Complete read-owner composition; Docker/Linux SDK/checker are doubles.

Actual Go file-only aggregation, SDK protocol and policy semantics have their
own tests. This suite exercises the fixed host entry and its closure outcomes.
"""
from contextlib import ExitStack
import copy
import datetime as dt
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from protection_test_fixtures import receipt as protection_receipt

spec=importlib.util.spec_from_file_location('six_verify_under_test',Path(__file__).with_name('verify-six-native.py'))
v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
r,p=v.r,v.p


class SixCompleteTests(unittest.TestCase):
    def setUp(self):
        # The receipt/candidate doubles describe this UTC test instant, not the
        # host wall clock. Expiry validation itself is deliberately unpatched.
        instant = dt.datetime.fromisoformat(p.W['NotBefore'].replace('Z', '+00:00'))
        self.enterContext(patch.object(r, 'now', return_value=instant))

    def composition(self,group='business-races',fault=None,post_closure=False,maintenance_phase=None):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            root=Path(tmp);control=root/'control';packet=root/'packet';control.mkdir();packet.mkdir()
            stack.enter_context(patch.object(r,'CONTROL',control))
            g=copy.deepcopy(next(x for x in r.load(p.p1.PACKET/'candidate.json')['Groups'] if x['Group']==group))
            folder=control/group;folder.mkdir();(folder/'frozen-inventory.json').write_bytes(b'[]');(folder/'present-inventory.json').write_bytes(b'[]');(control/'six-map.json').write_bytes(b'NON_AUTHORITATIVE_TEST_MAP')
            r.save(control/'allocation.json',{'groups':[{'group':group,'private':str(folder)}]});(packet/group).mkdir();r.save(packet/group/'core-readonly.json',{'recorded_source':group})
            private,output=root/'read-private',root/'public-read';states={};calls=[];name_to_cid={};fp=[]
            baseline=None
            if post_closure:
                baseline=root/'post-closure.json';r.save(baseline,{'recorded_source':group,'closed_dispatches':3})
            def state(cid):return {'Id':cid,'Image':p.p1.PG,'Path':'/run/reader','Args':['-mode','six-verify-native'],'Config':{'User':'10001:10001','Entrypoint':['/run/reader'],'Cmd':['-mode','six-verify-native'],'Env':[],'Labels':{'lifecycle307.six-readonly':p.EXECUTION}},
                'State':{'Running':False,'Pid':0},'HostConfig':{'NetworkMode':'container:'+g['Native'],'ReadonlyRootfs':True,'Privileged':False,'AutoRemove':False,'RestartPolicy':{'Name':'no'},'CapDrop':['ALL'],'CapAdd':None,'SecurityOpt':['no-new-privileges'],'Tmpfs':{'/tmp':'','/var/lib/postgresql/data':'size=1m'},'PortBindings':{}},'Mounts':[],'NetworkSettings':{'Ports':{}}}
            for cid in (g['Core'],g['Native']):states[cid]=state(cid)
            cfg={'Enabled':False,'AuditID':'00000000-0000-4000-8000-000000000011','BinarySHA256':'b'*64,'CandidateSHA256':'c'*64,'FrozenInventorySHA256':'f'*64,'PresentInventorySHA256':'e'*64}
            mode='six-verify-native';label='lifecycle307.six-readonly';label_value=p.EXECUTION;context=None
            if maintenance_phase is not None:
                mode='retention-maintenance-verify-native';label='lifecycle307.maintenance-readonly';label_value='00000000-0000-4000-8000-000000000022'
                end=r.now().replace(microsecond=0)+dt.timedelta(minutes=30)
                cfg.update(Purpose='RETENTION_MAINTENANCE_NATIVE_READONLY',Group=group,MaintenanceID=label_value,Phase=maintenance_phase,MaxProviderDeletes=0,
                    BindingsSHA256='c'*64,RetainUntil=(end+dt.timedelta(hours=24)).isoformat(),MaintenanceEnd=end.isoformat(),EffectiveInventorySHA256='d'*64)
                (folder/'effective-inventory.json').write_bytes(b'[]')
                context=dict(source=g,profile=cfg,reader=str(control/'reader'),checker=str(control/'checker'),expectation=str(packet/group/'expectation'),
                    inputs={name:str(folder/name) for name in ('frozen-inventory.json','present-inventory.json','effective-inventory.json')},record_creation=lambda:None,register_owner=lambda cid,descriptor:None)
                def rejected_fact(*args):raise OSError('original signed registration persistence failure')
                if fault=='creation-intent':context['record_creation']=rejected_fact
                if fault=='registration':context['register_owner']=rejected_fact
                stack.enter_context(patch.object(v.native_protection,'validate_group',side_effect=AssertionError('expired old cutoff must use typed maintenance checker')))
            stack.enter_context(patch.object(r,'profile',lambda *args:copy.deepcopy(cfg)))
            stack.enter_context(patch.object(p.prior.closure,'source_contract',lambda source:None))
            stack.enter_context(patch.object(p.native,'wait_pg',lambda *args:None));stack.enter_context(patch.object(v.time,'sleep',lambda n:None))
            def inspect(cid):
                if fault=='create-unknown' and cid in name_to_cid:raise OSError('recorded exact-name inspect unavailable')
                return copy.deepcopy(states[name_to_cid.get(cid,cid)])
            stack.enter_context(patch.object(r,'inspect',inspect))
            def close(cid):
                cid=name_to_cid.get(cid,cid);calls.append(('close',cid));states[cid]['State'].update(Running=False,Pid=0)
                return {'cid':cid,'closed':True}
            stack.enter_context(patch.object(p.p1,'close_known',close))
            def fingerprint(cid):
                fp.append(cid)
                return {'drift':True} if fault=='Core-drift' and len(fp)>1 else ({'recorded_source':group,'closed_dispatches':3} if post_closure else {'recorded_source':group})
            stack.enter_context(patch.object(p.native,'fingerprints',fingerprint))
            def docker(*args,**kw):
                calls.append(args)
                if args[0]=='network':return json.dumps([{'Internal':True,'Containers':{g['Core']:{},g['Native']:{}}}]).encode()
                if args[0]=='create':
                    name=args[args.index('--name')+1];cid=hashlib.sha256(name.encode()).hexdigest();name_to_cid[name]=cid;states[cid]=state(cid)
                    states[cid]['Name']='/'+name
                    states[cid]['Args']=['-mode',mode];states[cid]['Config']['Cmd']=['-mode',mode];states[cid]['Config']['Labels']={label:label_value}
                    for n,value in enumerate(args):
                        if value=='--mount':
                            fields=dict(x.split('=',1) for x in args[n+1].split(',') if '=' in x)
                            states[cid]['Mounts'].append({'Destination':fields['target'],'Source':fields['source'],'Type':'bind','RW':'readonly' not in args[n+1].split(',')})
                    if fault=='namespace':states[cid]['HostConfig']['NetworkMode']='container:unbound-source'
                    if fault=='create-unknown':raise OSError('recorded create result unknown after daemon effect')
                    return b'untrusted-short-name' if fault=='create-result' else cid.encode()
                if args[0]=='start':
                    states[args[1]]['State'].update(Running=True,Pid=55)
                    if fault=='source-start' and args[1]==g['Core']:raise OSError('recorded start outcome unavailable after effect')
                    if args[1] not in (g['Core'],g['Native']):
                        r.save(output/'files/readonly-start-request.json',{'id':'request','method':'GET'})
                        r.save(output/'files/readonly-observation-request.json',{'id':'request','status':200,'transport_error':False})
                        observation=protection_receipt(group,r.now(),p.W['RecoveryEnd'])
                        if fault=='protection-expired' and group!='late-hold':observation['verification']['copies'][0]['RetainUntil']='2026-10-23T06:25:44Z'
                        if fault=='protection-version' and group!='late-hold':observation['verification']['copies'][0]['Copy']['ProviderVersion']='different'
                        r.save(output/'files'/('retention-native-verification.json' if maintenance_phase else 'six-native-verification.json'),observation)
                return b''
            stack.enter_context(patch.object(r,'docker',docker))
            def process(args,**kw):
                if args[0]=='docker':
                    if args[1]=='wait':
                        if fault=='wait-timeout':raise subprocess.TimeoutExpired(args,1,output=b'wait-partial',stderr=b'wait-error')
                        states[args[2]]['State'].update(Running=False,Pid=0);return SimpleNamespace(returncode=0,stdout=b'0\n',stderr=b'')
                    if args[1]=='logs':
                        if fault=='logs-timeout':raise subprocess.TimeoutExpired(args,1,output=b'logs-partial',stderr=b'logs-error')
                        return SimpleNamespace(returncode=0,stdout=b'RECORDED_SDK_OWNER_NO_NATIVE',stderr=b'')
                if ('-retention-maintenance' if maintenance_phase else '-six') not in args:raise AssertionError('wrong checker used')
                if fault=='checker':return SimpleNamespace(returncode=1,stdout=b'original-rejection',stderr=b'actual-modeled-checker-error')
                result={'status':'PASS_COMPLETE_RETENTION_MAINTENANCE_NATIVE_AGGREGATION' if maintenance_phase else 'PASS_COMPLETE_SIX_NATIVE_AGGREGATION','group':group,'copies':{'business-races':7,'late-hold':6,'mixed':4}[group]}
                return SimpleNamespace(returncode=0,stdout=json.dumps(result).encode(),stderr=b'')
            stack.enter_context(patch.object(v.subprocess,'run',process))
            real_replace=r.replace
            def replace(path,value):
                if fault=='profile-revoke' and value.get('Enabled') is False:raise OSError('recorded profile revoke failure')
                real_replace(path,value)
            stack.enter_context(patch.object(r,'replace',replace))
            if fault:
                with self.assertRaises((ValueError,OSError,subprocess.TimeoutExpired)):v.collect(g,{},packet,private,output,r.now()+dt.timedelta(minutes=20),core_baseline=baseline,maintenance=context)
            else:
                result=v.collect(g,{},packet,private,output,r.now()+dt.timedelta(minutes=20),core_baseline=baseline,maintenance=context);self.assertEqual(result['copies'],{'business-races':7,'late-hold':6,'mixed':4}[group])
                if maintenance_phase:self.assertEqual(result['phase'],maintenance_phase);self.assertEqual(result['retain_until'],cfg['RetainUntil'])
            self.assertFalse(any(x['State']['Running'] for x in states.values()))
            if fault in ('creation-intent','registration'):
                self.assertFalse(any(row[0]=='start' and row[1] not in (g['Core'],g['Native']) for row in calls))
            self.assertTrue(all(len(row[1])==64 for row in calls if row[0]=='close'))
            closure=r.load(output/'closure.json');self.assertEqual(closure['all_closed'],fault!='create-unknown')
            if fault=='create-unknown':self.assertEqual(closure['unknown_temporary_owner']['state'],'UNCONFIRMED')
            self.assertEqual(closure['failure'] is None,fault is None)
            if fault=='wait-timeout':self.assertEqual((output/'operation/wait-stdout.txt').read_bytes(),b'wait-partial')
            if fault=='logs-timeout':self.assertEqual((output/'operation/stdout.txt').read_bytes(),b'logs-partial')

    def test_three_source_groups_use_current_six_checker_and_close(self):
        for group in p.GROUPS:
            with self.subTest(group=group):self.composition(group)

    def test_new_read_audit_uses_post_closure_checkpoint_and_still_rejects_drift(self):
        self.composition(post_closure=True)
        self.composition(post_closure=True,fault='Core-drift')

    def test_source_start_helper_namespace_create_wait_logs_checker_core_revoke_failures_close(self):
        for fault in ('source-start','namespace','create-result','create-unknown','wait-timeout','logs-timeout','checker','Core-drift','profile-revoke','protection-expired','protection-version'):
            with self.subTest(fault=fault):self.composition(fault=fault)

    def test_maintenance_seventeen_both_phases_use_sdk_core_and_typed_checker_closure(self):
        for phase in ('before','after'):
            for group in p.GROUPS:
                with self.subTest(phase=phase,group=group):self.composition(group,maintenance_phase=phase)

    def test_maintenance_shared_entry_failure_combinations_still_revoke_and_close(self):
        for fault in ('source-start','namespace','create-result','create-unknown','wait-timeout','logs-timeout','checker','Core-drift','profile-revoke','creation-intent','registration'):
            with self.subTest(fault=fault):self.composition(fault=fault,maintenance_phase='before')


if __name__=='__main__':unittest.main()
