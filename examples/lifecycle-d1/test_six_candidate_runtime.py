"""Full host composition with recorded process/provider doubles only.

No Docker daemon, live Core, IAM or object store is contacted. Go contract tests
separately exercise the actual SDK/official policy library and file aggregation.
"""
from contextlib import ExitStack
import copy
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tempfile
import tarfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('six_host_under_test',Path(__file__).with_name('six_candidate_runtime.py'))
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
RECORDED=r.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate13-preparation01'


class SixHostTests(unittest.TestCase):
    def test_current_exact_build_has_all_twelve_regular_unlinked_members(self):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            control=Path(tmp);stack.enter_context(patch.object(r,'CONTROL',control))
            for name in r.BUILD_MEMBERS:(control/name).write_bytes(name.encode())
            a={'source':'a'*40,'build_files':{n:r.sha(control/n) for n in r.BUILD_MEMBERS},'binaries':{n:r.sha(control/n) for n in ('reader','operator','owner','readonly-check.exe')}}
            state={'type':b'commit','dirty':0,'untracked':b''}
            stack.enter_context(patch.object(r.subprocess,'check_output',lambda args,**kw:state['type'] if args[1]=='cat-file' else state['untracked']))
            stack.enter_context(patch.object(r.subprocess,'run',lambda *args,**kw:SimpleNamespace(returncode=state['dirty'])))
            self.assertEqual(r.build_contract(a)['source'],'a'*40)
            for key,value in [('type',b'blob'),('dirty',1),('untracked',b'untracked-runtime.go')]:
                old=state[key];state[key]=value
                with self.subTest(key=key),self.assertRaises(ValueError):r.build_contract(a)
                state[key]=old
            for members in ({},a['binaries'],dict(a['build_files'],extra='f'*64),{k:v for k,v in a['build_files'].items() if k!='checker-build.stderr'}):
                with self.subTest(members=list(members)),self.assertRaises(ValueError):r.build_contract(dict(a,build_files=members))
            changed=control/'reader-build.stderr';original=changed.read_bytes();changed.write_bytes(b'changed')
            with self.assertRaises(ValueError):r.build_contract(a)
            changed.write_bytes(original)
            real=Path.is_symlink
            with patch.object(Path,'is_symlink',lambda path:path.name=='reader-build.stderr' or real(path)):
                with self.assertRaises(ValueError):r.build_contract(a)
            alias=control/'alias';alias.hardlink_to(changed)
            with self.assertRaises(ValueError):r.build_contract(a)
            alias.unlink()
            self.assertEqual(r.build_contract(a)['files'],a['build_files'])

    def budget_initialization(self,fault=None):
        a=r.load(RECORDED/'allocation.json');a['source']='a'*40
        before=(RECORDED/'budget-before-continuation01/late-hold.tar').read_bytes()
        after=(RECORDED/'budget-before-continuation01/business-races.tar').read_bytes()
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            control=Path(tmp)/'private';public=Path(tmp)/'public';control.mkdir();public.mkdir()
            stack.enter_context(patch.object(r,'CONTROL',control));stack.enter_context(patch.object(r,'PUBLIC',public))
            for g in a['groups']:
                folder=control/g['group'];folder.mkdir();g['private']=str(folder)
            r.save(public/'allocation.json',a)
            stack.enter_context(patch.object(r,'state',lambda:a));stack.enter_context(patch.object(r,'cold_topology',lambda a:None))
            started=[];closed=[]
            def observed(cid):
                index=next(n for n,g in enumerate(a['groups']) if g['budget_initializer']==cid)
                state={'Status':'created','StartedAt':r.ZERO,'FinishedAt':r.ZERO,'Running':False,'Pid':0}
                if fault=='already-started' and index==1:state['StartedAt']='2026-10-19T00:00:00Z'
                return {'State':state,'HostConfig':{'RestartPolicy':{'Name':'no'},'PortBindings':{}},'NetworkSettings':{'Ports':{}}}
            stack.enter_context(patch.object(r,'inspect',observed))
            def docker(*args):
                g=next(g for g in a['groups'] if args[1].startswith(g['helpers']['wire']))
                return after if g['budget_initializer'] in started else before
            stack.enter_context(patch.object(r,'docker',docker))
            def owner(cid,destination):
                started.append(cid)
                if fault=='second-failed' and len(started)==2:raise OSError('recorded initializer failed')
                destination.mkdir(parents=True);r.save(destination/'owner.json',{'cid':cid,'closure':{'closed':True},'failure':None})
                (destination/'wait-stdout.txt').write_bytes(b'0\n')
            stack.enter_context(patch.object(r,'run_owner',owner))
            stack.enter_context(patch.object(r.p.p1,'close_known',lambda cid:closed.append(cid) or {'cid':cid,'closed':not(fault=='unconfirmed' and cid==a['groups'][1]['budget_initializer'])}))
            if fault:
                with self.assertRaises((ValueError,OSError)):r.initialize_budgets()
            else:
                r.initialize_budgets();r.budget_completion(a)
                self.assertEqual(started,[g['budget_initializer'] for g in a['groups']])
                receipt=r.load(public/'budget-initialization.json')
                for key,value in [('execution','old-execution'),('source','b'*40),('allocation_sha256','f'*64),('all_complete',False)]:
                    bad=copy.deepcopy(receipt);bad[key]=value;(public/'budget-initialization.json').write_text(json.dumps(bad),encoding='utf8')
                    with self.subTest(field=key),self.assertRaises(ValueError):r.budget_completion(a)
                (public/'budget-initialization.json').write_text(json.dumps(receipt),encoding='utf8')
                with self.assertRaises(ValueError):r.initialize_budgets()
            if fault!='already-started':
                self.assertEqual(closed,[g['budget_initializer'] for g in a['groups']])
                self.assertEqual(r.load(public/'budget-initialization.json')['all_complete'],fault is None)
            else:self.assertEqual(started,[])
            if fault=='second-failed':self.assertEqual(len(started),2)

    def test_new_budgets_initialize_once_and_cross_bound_receipts_fail(self):self.budget_initialization()

    def test_new_budget_restart_failure_and_unconfirmed_closure_stop(self):
        for fault in ('already-started','second-failed','unconfirmed'):
            with self.subTest(fault=fault):self.budget_initialization(fault)

    def test_actual_created_network_prebinding_and_attached_identity_counterexamples(self):
        rows=[json.loads(line) for line in (RECORDED/'materialize-cold-network-observation01.jsonl').read_text().splitlines()]
        self.assertEqual(len(rows),12)
        for row in rows:
            network=row['approved_network'];name=next(iter(row['networks']))
            original={'State':row['state'],'HostConfig':{'NetworkMode':row['network_mode']},'NetworkSettings':{'Networks':row['networks']}}
            r.direct_network(original,network,name)
            attached=copy.deepcopy(original);attached['State'].update(Status='exited',StartedAt='2026-10-19T00:00:00Z',FinishedAt='2026-10-19T00:01:00Z')
            attached['NetworkSettings']['Networks'][name]['NetworkID']=network
            r.direct_network(attached,network,name)
            mutations=[('HostConfig','NetworkMode','foreign'),('State','Status','exited'),('State','StartedAt','2026-10-19T00:00:00Z'),
                       ('State','FinishedAt','2026-10-19T00:00:00Z'),('State','Running',True),('State','Pid',1)]
            for section,key,value in mutations:
                bad=copy.deepcopy(original);bad[section][key]=value
                with self.subTest(role=row['role'],field=key),self.assertRaises(ValueError):r.direct_network(bad,network,name)
            for key,value in [('NetworkID','f'*64),('EndpointID','unexpected'),('IPAddress','10.0.0.7'),('Gateway','10.0.0.1'),('MacAddress','02:42:00:00:00:01'),('IPPrefixLen',24)]:
                bad=copy.deepcopy(original);bad['NetworkSettings']['Networks'][name][key]=value
                with self.subTest(role=row['role'],field=key),self.assertRaises(ValueError):r.direct_network(bad,network,name)
            for membership in ({'foreign':next(iter(row['networks'].values()))},dict(row['networks'],foreign={})):
                bad=copy.deepcopy(original);bad['NetworkSettings']['Networks']=membership
                with self.subTest(role=row['role'],field='membership'),self.assertRaises(ValueError):r.direct_network(bad,network,name)

    def test_actual_initializer_canonical_capability_and_expansion_counterexamples(self):
        allocation=r.load(RECORDED/'allocation.json');descriptors=r.load(RECORDED/'owner-descriptors.json')
        for group in allocation['groups']:
            d=descriptors[group['budget_initializer']]
            original={'Image':d['image'],'Config':{'User':d['user'],'Entrypoint':d['entrypoint'],'Cmd':d['command']},'HostConfig':d['host']}
            self.assertEqual(original['HostConfig']['CapAdd'],['CAP_CHOWN']);r.initializer_security(original)
            for section,key,value in [('Config','User','10001:10001'),('Config','Cmd',['-c','arbitrary']),('HostConfig','NetworkMode','bridge'),
                                      ('HostConfig','CapAdd',['CHOWN']),('HostConfig','CapAdd',['CAP_CHOWN','CAP_SYS_ADMIN']),('HostConfig','CapAdd',['CAP_CHOWN','CAP_CHOWN']),
                                      ('HostConfig','CapDrop',[]),('HostConfig','ReadonlyRootfs',False),('HostConfig','Privileged',True),
                                      ('HostConfig','RestartPolicy',{'Name':'always'}),('HostConfig','PortBindings',{'80/tcp':[{'HostPort':'8080'}]})]:
                bad=copy.deepcopy(original);bad[section][key]=value
                with self.subTest(group=group['group'],field=key),self.assertRaises(ValueError):r.initializer_security(bad)



    def test_existing_or_wrong_owner_volume_never_starts_or_is_adopted(self):
        with tempfile.TemporaryDirectory() as tmp,patch.object(r.p,'PUBLIC',Path(tmp)):
            for fault in ('existing','wrong-label',None):
                calls=[];a=r.p.Allocation()
                def docker(*args,**kw):
                    calls.append(args)
                    if args[:2]==('volume','ls'):return b'occupied-volume\n' if fault=='existing' else b''
                    if args[:2]==('volume','create'):return b'new-volume\n'
                    if args[:2]==('volume','inspect'):return json.dumps([{'Name':'new-volume','Driver':'local','Scope':'local','Options':None,'Labels':{'lifecycle307.remaining':r.p.EXECUTION,'lifecycle307.allocation':'foreign' if fault=='wrong-label' else a.nonce}}]).encode()
                    if args[0]=='ps':return b''
                    raise AssertionError('unexpected mutation/start')
                # Each failed fixture gets its own O_EXCL evidence directory.
                evidence=Path(tmp)/(fault or 'success');evidence.mkdir()
                with patch.object(r.p,'PUBLIC',evidence),patch.object(r.p,'docker',docker):
                    if fault:
                        with self.assertRaises(ValueError):a.volume('new-volume')
                    else:self.assertEqual(a.volume('new-volume'),'new-volume')
                if fault=='existing':self.assertEqual([x[:2] for x in calls],[('volume','ls')])
                self.assertFalse(any(x[0]=='start' for x in calls))

    def test_fixed_docker_bytes_wrapper_preserves_streamed_dump(self):
        with tempfile.TemporaryDirectory() as tmp:
            target=Path(tmp)/'core.dump';calls=[]
            def process(args,**kw):
                calls.append(args);kw['stdout'].write(b'PGDMP-recorded-bytes')
                return SimpleNamespace(returncode=0,stdout=None,stderr=b'')
            with patch.object(r.p.subprocess,'run',process):
                self.assertEqual(r.p.docker('exec','recorded-source','pg_dump',output=target),b'')
            self.assertEqual(target.read_bytes(),b'PGDMP-recorded-bytes')
            self.assertEqual(calls,[['docker','exec','recorded-source','pg_dump']])

    def owner(self,fault=None):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            target=Path(tmp)/'attempt';calls=[];closed=[]
            stack.enter_context(patch.object(r,'inspect',lambda cid:{'State':{'Running':False,'Pid':0},'HostConfig':{'RestartPolicy':{'Name':'no'},'PortBindings':{}},'NetworkSettings':{'Ports':{}}}))
            stack.enter_context(patch.object(r,'docker',lambda *args,**kw:calls.append(args) or b''))
            def close(cid):closed.append(cid);return {'cid':cid,'closed':True}
            stack.enter_context(patch.object(r.p.p1,'close_known',close))
            def process(args,**kw):
                if args[1]=='wait':
                    if fault=='wait-timeout':raise subprocess.TimeoutExpired(args,1,output=b'wait-partial',stderr=b'wait-error')
                    return SimpleNamespace(returncode=0,stdout=b'0\n',stderr=b'')
                if args[1]=='logs':
                    if fault=='logs-timeout':raise subprocess.TimeoutExpired(args,1,output=b'logs-partial',stderr=b'logs-error')
                    if fault=='logs-error':raise OSError('recorded log unavailable')
                    return SimpleNamespace(returncode=0,stdout=b'complete-owner-log',stderr=b'')
                raise AssertionError('unbound process')
            stack.enter_context(patch.object(r.subprocess,'run',process))
            real_write=r.write_raw
            def write(path,raw):
                if fault=='write-error' and path.name=='stdout.txt':raise OSError('recorded evidence write failure')
                real_write(path,raw)
            stack.enter_context(patch.object(r,'write_raw',write))
            if fault:
                with self.assertRaises((OSError,ValueError,subprocess.TimeoutExpired)):r.run_owner('recorded-owner',target)
            else:r.run_owner('recorded-owner',target)
            self.assertEqual(closed,['recorded-owner'])
            if fault=='wait-timeout':
                self.assertEqual((target/'wait-stdout.txt').read_bytes(),b'wait-partial')
                self.assertEqual((target/'wait-stderr.txt').read_bytes(),b'wait-error')
            if fault=='logs-timeout':
                self.assertEqual((target/'stdout.txt').read_bytes(),b'logs-partial')
                self.assertEqual((target/'stderr.txt').read_bytes(),b'logs-error')
            self.assertEqual(calls,[('start','recorded-owner')])

    def test_owner_success_and_wait_logs_write_failure_always_close(self):
        for fault in (None,'wait-timeout','logs-timeout','logs-error','write-error'):
            with self.subTest(fault=fault):self.owner(fault)

    def test_case_cross_field_failure_precedes_permission_and_docker(self):
        with tempfile.TemporaryDirectory() as tmp:
            folder=Path(tmp);case=folder/'case-business';case.mkdir()
            profile={'Group':'business-races','NonDelete':True,'Rehearsal':False,'Batch':r.p.EXECUTION,'CandidateUseUntil':r.p.W['NotAfter'],'ExpectedPolicyHeads':r.p.window.policy_heads('business-races'),'Base':{'StartedAt':r.p.W['NotBefore'],'ExpiresAt':r.p.W['NotAfter']}}
            for field in ('Batch','CandidateUseUntil','ExpectedPolicyHeads'):
                value=copy.deepcopy(profile)
                if field=='ExpectedPolicyHeads':
                    version=next(iter(value[field]));value[field][version]='11111111-1111-4111-8111-111111111111'
                else:value[field]='old'
                (case/'remaining.json').write_text(json.dumps(value))
                with patch.object(r,'cold_topology',side_effect=AssertionError('Docker reached before case semantics')):
                    with self.assertRaises(ValueError):r.preparation_gate({'groups':[{'group':'business-races','private':str(folder),'cases':{'business':'unused'}}]})


    def test_budget_complete_projection_and_real_archive_success_and_fail_closed(self):
        a=r.load(RECORDED/'allocation.json');packet=r.budget_candidate(a)
        targets=r.p.sequence.targets(packet)
        self.assertEqual([stage for _,stage,_ in targets],['P2','T1','T2','H2','M1A','M1B'])
        self.assertNotIn('fixture/p1.csv',[cp['Key'] for _,_,cp in targets])
        for group in r.p.GROUPS:
            raw=(RECORDED/'budget-before-continuation01'/(group+'.tar')).read_bytes()
            if group=='business-races':self.assertEqual(r.p.sequence.budget_archive(raw,packet,group,initial=True)['files'],[])
            else:r.uninitialized_budget(raw)
        for fault in ('missing-helpers','extra-helper','missing-group'):
            bad=copy.deepcopy(a)
            if fault=='missing-helpers':bad['groups'][0].pop('helpers')
            elif fault=='extra-helper':bad['groups'][0]['helpers']['P1']='unapproved'
            else:bad['groups'].pop()
            with self.subTest(fault=fault),self.assertRaises((ValueError,KeyError)):r.budget_candidate(bad)
        original=(RECORDED/'budget-before-continuation01/business-races.tar').read_bytes()
        def mutate(uid=10001,filename=None,link=False):
            stream=io.BytesIO()
            with tarfile.open(fileobj=stream,mode='w') as archive:
                item=tarfile.TarInfo('budget');item.type=tarfile.DIRTYPE;item.mode=0o755;item.uid=item.gid=uid;archive.addfile(item)
                if filename:
                    item=tarfile.TarInfo('budget/'+filename);item.uid=item.gid=uid
                    if link:item.type=tarfile.SYMTYPE;item.linkname='/etc/passwd';archive.addfile(item)
                    else:item.size=2;archive.addfile(item,io.BytesIO(b'{}'))
            return stream.getvalue()
        for raw in (original.decode('latin1'),mutate(0),mutate(filename='used.json'),mutate(filename='link',link=True)):
            with self.subTest(kind=type(raw)),self.assertRaises(ValueError):r.p.sequence.budget_archive(raw,packet,'business-races',initial=True)
        for raw in (original,mutate(uid=0,filename='used.json'),mutate(uid=0,filename='link',link=True),b'invalid'):
            with self.subTest(kind=type(raw)),self.assertRaises((ValueError,tarfile.ReadError)):r.uninitialized_budget(raw)




    def composition(self,fail_dump=False,stale_checkpoint=False):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            control,public=Path(tmp)/'private',Path(tmp)/'public';control.mkdir();public.mkdir()
            stack.enter_context(patch.object(r,'CONTROL',control));stack.enter_context(patch.object(r,'PUBLIC',public))
            state={'groups':[],'owners':[]};states={};calls=[];roles=[];baseline={'relations':[]}
            r.save(public/'recorded-Core.json',baseline)
            # Its old Core SHA is historical; live admission binds the separate
            # accepted full post-closure Core checkpoint, not that old fact.
            stack.enter_context(patch.object(r.p.window,'policy_checkpoint',lambda:{'CoreSHA256':'f'*64}))
            stack.enter_context(patch.object(r,'budget_completion',lambda a:None))
            for name in r.p.GROUPS:
                folder=control/name;folder.mkdir();(folder/'checkpoint').mkdir()
                g={'group':name,'private':str(folder),'source':{'Core':name+'-source-core','Native':name+'-source-native'},'restore_core':name+'-restore-core','rehearsal_core':name+'-trial-core','restore_native':name+'-restore-native','maintenance':{'SOURCE':name+'-reader','RESTORE':name+'-restore-reader','PROVISION':name+'-root'},'rehearsals':{},'utilities':{stage:name+'-'+stage for stage in ('archive','unpack','compare')}}
                state['groups'].append(g)
                for role in g['maintenance']:
                    target=folder/('maintenance-'+role.lower());target.mkdir()
                    r.save(target/('six-provision.json' if role=='PROVISION' else 'six-readonly.json'),{'Enabled':False})
                new=[g[k] for k in ('restore_core','rehearsal_core','restore_native')]+list(g['utilities'].values())+list(g['maintenance'].values())
                state['owners'].extend(new)
                for cid in new+list(g['source'].values()):states[cid]=False
            stack.enter_context(patch.object(r,'state',lambda:state));stack.enter_context(patch.object(r,'preparation_gate',lambda a:{'status':'RECORDED_COLD_GATE_ONLY'}))
            stack.enter_context(patch.object(r,'baseline_core',lambda group:{'relations':['post-closure-drift']} if stale_checkpoint else baseline))
            stack.enter_context(patch.object(r.p.native,'fingerprints',lambda cid:baseline))
            stack.enter_context(patch.object(r.p.native,'wait_pg',lambda cid,user:None))
            stack.enter_context(patch.object(r.time,'sleep',lambda duration:None))
            def inspect(cid):return {'State':{'Running':states[cid],'Pid':1 if states[cid] else 0},'HostConfig':{'RestartPolicy':{'Name':'no'},'PortBindings':{}},'NetworkSettings':{'Ports':{}}}
            stack.enter_context(patch.object(r,'inspect',inspect))
            def close(cid):states[cid]=False;calls.append(('close',cid));return {'cid':cid,'closed':True}
            stack.enter_context(patch.object(r.p.p1,'close_known',close))
            def process(args,**kw):
                calls.append(tuple(args))
                if args[0]!='docker':raise AssertionError('unexpected process')
                if args[1]=='wait':states[args[2]]=False;return SimpleNamespace(returncode=0,stdout=b'0\n',stderr=b'')
                if args[1]=='logs':return SimpleNamespace(returncode=0,stdout=b'PostgreSQL init process complete; ready for start up.\nrecorded-file-manifest\n',stderr=b'')
                raise AssertionError('unexpected process path')
            stack.enter_context(patch.object(r.subprocess,'run',process))
            def docker(*args,**kw):
                calls.append(args)
                if args[0]=='start':
                    states[args[1]]=True
                    if args[1].endswith('-archive'):
                        group=next(g for g in state['groups'] if g['utilities']['archive']==args[1]);r.write_raw(Path(group['private'])/'checkpoint/native.tar',b'recorded-native-archive')
                if 'output' in kw:
                    if fail_dump:raise OSError('recorded streaming dump failure')
                    r.write_raw(kw['output'],b'recorded-dump')
                return b''
            stack.enter_context(patch.object(r,'docker',docker))
            def maintenance(g,role):roles.append((g['group'],role));return {'role':role,'status':'RECORDED_LINUX_AND_CHECKER_DOUBLE'}
            stack.enter_context(patch.object(r,'maintenance',maintenance));stack.enter_context(patch.object(r,'trial',lambda g:None))
            if stale_checkpoint:
                with self.assertRaises(ValueError):r.run_non_delete()
            elif fail_dump:
                with self.assertRaises(OSError):r.run_non_delete()
            else:r.run_non_delete()
            self.assertFalse(any(states.values()))
            terminal=r.load(public/'non-delete-terminal.json')
            if fail_dump or stale_checkpoint:
                self.assertEqual(terminal['status'],'FAILED_OR_UNCONFIRMED_RETAINED')
                self.assertFalse(any(row==('start','late-hold-source-core') for row in calls))
                if stale_checkpoint:self.assertEqual(roles,[])
            else:
                self.assertEqual(terminal['status'],'PASS_NONDELETE_SOURCE_RESTORE_PREPARATION')
                self.assertEqual(terminal['groups_completed'],3)
                self.assertEqual(roles,[(g,role) for g in r.p.GROUPS for role in ('PROVISION','SOURCE','RESTORE')])
                self.assertEqual(sum('pg_dump' in row for row in calls),3)
                self.assertEqual(sum('pg_restore' in row for row in calls),6)

    def test_full_three_group_source_backup_restore_sequence(self):self.composition()
    def test_dump_failure_stops_new_groups_and_closes_every_known_owner(self):self.composition(True)
    def test_current_Core_checkpoint_mismatch_stops_before_IAM(self):self.composition(stale_checkpoint=True)


if __name__=='__main__':unittest.main()
