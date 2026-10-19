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


class SixHostTests(unittest.TestCase):
    def test_actual_created_network_prebinding_and_attached_identity_counterexamples(self):
        rows=[json.loads(line) for line in (r.PUBLIC/'materialize-cold-network-observation01.jsonl').read_text().splitlines()]
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
        allocation=r.load(r.PUBLIC/'allocation.json');descriptors=r.load(r.PUBLIC/'owner-descriptors.json')
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

    def test_source_contract_allows_only_reviewed_host_repair_not_compiled_or_other_host_drift(self):
        initial={'source':'a'*40}
        def check(changed,untracked=b'',dirty=0):
            def output(args,**kwargs):return untracked if args[1]=='ls-files' else ('\n'.join(changed)).encode()
            with patch.object(r.subprocess,'check_output',output),patch.object(r.subprocess,'run',lambda *args,**kwargs:SimpleNamespace(returncode=dirty)):
                return r.source_contract(initial,'b'*40)
        self.assertEqual(check(sorted(r.REPAIR_FILES)),sorted(r.REPAIR_FILES))
        for changed,untracked,dirty in [(['apps/platform/go.mod'],b'',0),(['apps/platform/internal/lifecycle/pilot/remaining_scope.json'],b'',0),
                                       (['migrations/000001.sql'],b'',0),(['examples/lifecycle-d1/remaining_sequence.py'],b'',0),([],b'untracked.py',0),([],b'',1)]:
            with self.subTest(changed=changed,untracked=untracked,dirty=dirty),self.assertRaises(ValueError):check(changed,untracked,dirty)

    def test_current_checkpoint_is_mandatory_preserves_initial_allocation_and_rejects_cross_binding(self):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            root=Path(tmp);control=root/'private';public=root/'public';control.mkdir();public.mkdir();rebuilt=control/r.REBUILD;rebuilt.mkdir()
            stack.enter_context(patch.object(r,'CONTROL',control));stack.enter_context(patch.object(r,'PUBLIC',public))
            binaries={}
            for name in ('reader','operator','owner','readonly-check.exe'):
                (control/name).write_bytes(name.encode());(rebuilt/name).write_bytes(name.encode());binaries[name]=r.sha(control/name)
            for name in r.REBUILD_MEMBERS-set(binaries):(rebuilt/name).write_bytes(b'original compiler channel')
            original={'source':'a'*40,'allocation_nonce':'initial-nonce','owners':['1'*64,'2'*64],'binaries':binaries}
            r.save(public/'allocation.json',original);original_bytes=(public/'allocation.json').read_bytes()
            r.save(public/'budget-initialization-failure01.json',{'status':'original retained budget failure'})
            stack.enter_context(patch.object(r,'previous_checkpoint',lambda a:None));stack.enter_context(patch.object(r,'budget_failure',lambda:None))
            receipt={'status':'PASS_IDENTICAL_REBUILD_AFTER_COLD_REPAIR','initial_build_source':original['source'],'effective_source':'b'*40,'allocation_sha256':r.sha(public/'allocation.json'),
                     'failure_evidence_sha256':r.sha(public/'budget-initialization-failure01.json'),'binaries':binaries,'files':r.rebuild_files(),'changed_host_paths':[],
                     'active_binary_replaced':False,'source_provider_io':0,'native_delete':0,'previous_checkpoint_sha256':r.PREVIOUS_CHECKPOINT_SHA256}
            r.save(public/(r.REBUILD+'.json'),receipt)
            checkpoint=dict(execution=r.p.EXECUTION,profile=r.p.PROFILE,initial_build_source=original['source'],effective_source=receipt['effective_source'],
                            allocation_sha256=receipt['allocation_sha256'],allocation_nonce=original['allocation_nonce'],owners=sorted(original['owners']),binaries=binaries,
                            rebuild_receipt_sha256=r.sha(public/(r.REBUILD+'.json')),failure_evidence_sha256=receipt['failure_evidence_sha256'],previous_checkpoint_sha256=r.PREVIOUS_CHECKPOINT_SHA256)
            stack.enter_context(patch.object(r,'allocation_state',lambda:copy.deepcopy(original)))
            stack.enter_context(patch.object(r,'source_contract',lambda a,source:[]))
            with self.assertRaises(FileNotFoundError):r.state()
            def write(value):
                for folder in (control,public):(folder/r.CHECKPOINT).write_text(json.dumps(value))
            write(checkpoint);current=r.state()
            self.assertEqual(current['source'],'b'*40);self.assertEqual(current['initial_build_source'],'a'*40)
            self.assertEqual((public/'allocation.json').read_bytes(),original_bytes);self.assertEqual(original['source'],'a'*40)
            for key,value in [('execution','old-execution'),('effective_source','c'*40),('initial_build_source','c'*40),('allocation_nonce','foreign'),
                              ('owners',['3'*64]),('binaries',dict(binaries,reader='f'*64)),('allocation_sha256','f'*64),('rebuild_receipt_sha256','f'*64),('failure_evidence_sha256','f'*64),('previous_checkpoint_sha256','f'*64)]:
                bad=copy.deepcopy(checkpoint);bad[key]=value;write(bad)
                with self.subTest(key=key),self.assertRaises(ValueError):r.state()
            for members in ({},binaries,{**receipt['files'],'extra.log':'f'*64},{k:v for k,v in receipt['files'].items() if k!='checker-build.stderr'}):
                bad=copy.deepcopy(receipt);bad['files']=members;(public/(r.REBUILD+'.json')).write_text(json.dumps(bad))
                rehashed=copy.deepcopy(checkpoint);rehashed['rebuild_receipt_sha256']=r.sha(public/(r.REBUILD+'.json'));write(rehashed)
                with self.subTest(receipt_members=list(members)),self.assertRaises(ValueError):r.state()
            for key,value in [('active_binary_replaced',True),('source_provider_io',1),('native_delete',1),('changed_host_paths',['apps/platform/go.mod'])]:
                bad=copy.deepcopy(receipt);bad[key]=value;(public/(r.REBUILD+'.json')).write_text(json.dumps(bad))
                rehashed=copy.deepcopy(checkpoint);rehashed['rebuild_receipt_sha256']=r.sha(public/(r.REBUILD+'.json'));write(rehashed)
                with self.subTest(receipt_field=key),self.assertRaises(ValueError):r.state()
            (public/(r.REBUILD+'.json')).write_text(json.dumps(receipt,indent=2)+'\n')
            checkpoint['rebuild_receipt_sha256']=r.sha(public/(r.REBUILD+'.json'));write(checkpoint)
            extra=rebuilt/'extra.log';extra.write_bytes(b'extra')
            with self.assertRaises(ValueError):r.state()
            extra.unlink()
            real_symlink=Path.is_symlink
            with patch.object(Path,'is_symlink',lambda path:path.name=='reader-build.stderr' or real_symlink(path)):
                with self.assertRaises(ValueError):r.state()
            linked=root/'linked-channel';linked.hardlink_to(rebuilt/'reader-build.stderr')
            with self.assertRaises(ValueError):r.state()
            linked.unlink()
            write(checkpoint);(rebuilt/'reader').write_bytes(b'different build')
            with self.assertRaises(ValueError):r.state()

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
            profile={'Group':'business-races','NonDelete':True,'Rehearsal':False,'Batch':r.p.EXECUTION,'CandidateUseUntil':r.p.W['NotAfter'],'ExpectedPolicyHeads':{str(i):str(i) for i in range(5)},'Base':{'StartedAt':r.p.W['NotBefore'],'ExpiresAt':r.p.W['NotAfter']}}
            for field in ('Batch','CandidateUseUntil'):
                value=dict(profile);value[field]='old'
                (case/'remaining.json').write_text(json.dumps(value))
                with patch.object(r,'cold_topology',side_effect=AssertionError('Docker reached before case semantics')):
                    with self.assertRaises(ValueError):r.preparation_gate({'groups':[{'group':'business-races','private':str(folder),'cases':{'business':'unused'}}]})

    def test_previous_checkpoint_and_original_failure_are_still_required(self):
        # Actual immutable public/private evidence is modeled in temp files; no
        # daemon or retained private directory is contacted by this regression.
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            control=Path(tmp)/'private';public=Path(tmp)/'public';control.mkdir();public.mkdir();(control/'source-rebuild01').mkdir()
            for obj,name,value in [(r,'CONTROL',control),(r,'PUBLIC',public)]:stack.enter_context(patch.object(obj,name,value))
            for name in r.REBUILD_MEMBERS:(control/'source-rebuild01'/name).write_bytes(name.encode())
            a={'source':'a'*40,'allocation_nonce':'nonce','owners':['owner'],'binaries':{'reader':'f'*64}}
            r.save(public/'allocation.json',a);r.save(public/'materialize-failure01.json',{'original':'cold failure'})
            r.save(public/'source-rebuild01.json',{'files':r.rebuild_files('source-rebuild01')})
            checkpoint=dict(initial_build_source=a['source'],allocation_sha256=r.sha(public/'allocation.json'),allocation_nonce=a['allocation_nonce'],owners=a['owners'],binaries=a['binaries'],
                            rebuild_receipt_sha256=r.sha(public/'source-rebuild01.json'),failure_evidence_sha256=r.sha(public/'materialize-failure01.json'))
            for folder in (control,public):r.save(folder/'source-checkpoint.json',checkpoint)
            stack.enter_context(patch.object(r,'PREVIOUS_CHECKPOINT_SHA256',r.sha(public/'source-checkpoint.json')))
            r.previous_checkpoint(a)
            for path in [control/'source-checkpoint.json',public/'source-checkpoint.json',public/'materialize-failure01.json',control/'source-rebuild01/reader-build.stderr']:
                raw=path.read_bytes();path.write_bytes(raw+b'changed')
                with self.subTest(path=path.name),self.assertRaises(ValueError):r.previous_checkpoint(a)
                path.write_bytes(raw)

    def test_budget_complete_projection_and_real_archive_success_and_fail_closed(self):
        a=r.load(r.PUBLIC/'allocation.json');packet=r.budget_candidate(a)
        targets=r.p.sequence.targets(packet)
        self.assertEqual([stage for _,stage,_ in targets],['P2','T1','T2','H2','M1A','M1B'])
        self.assertNotIn('fixture/p1.csv',[cp['Key'] for _,_,cp in targets])
        for group in r.p.GROUPS:
            raw=(r.PUBLIC/'budget-before-continuation01'/(group+'.tar')).read_bytes()
            if group=='business-races':self.assertEqual(r.p.sequence.budget_archive(raw,packet,group,initial=True)['files'],[])
            else:r.uninitialized_budget(raw)
        for fault in ('missing-helpers','extra-helper','missing-group'):
            bad=copy.deepcopy(a)
            if fault=='missing-helpers':bad['groups'][0].pop('helpers')
            elif fault=='extra-helper':bad['groups'][0]['helpers']['P1']='unapproved'
            else:bad['groups'].pop()
            with self.subTest(fault=fault),self.assertRaises((ValueError,KeyError)):r.budget_candidate(bad)
        original=(r.PUBLIC/'budget-before-continuation01/business-races.tar').read_bytes()
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

    def budget_continuation(self,fault=None):
        evidence=r.PUBLIC;a=r.load(evidence/'allocation.json');failure=r.load(evidence/'budget-initialization-failure01.json')
        originals={group:(evidence/'budget-before-continuation01'/(group+'.tar')).read_bytes() for group in r.p.GROUPS}
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            control=Path(tmp)/'private';public=Path(tmp)/'public';control.mkdir();public.mkdir()
            for obj,name,value in [(r,'CONTROL',control),(r,'PUBLIC',public)]:stack.enter_context(patch.object(obj,name,value))
            a['source_checkpoint_sha256']='f'*64
            for g in a['groups']:
                folder=control/g['group'];folder.mkdir();g['private']=str(folder)
                for stage in [*g['helpers'],*['root-'+action for action in g['root_owners']]]:
                    (folder/stage).mkdir();r.save(folder/stage/'permission.json',{'Enabled':False})
            original=public/'business-races/budget-initialization';original.mkdir(parents=True)
            for name in ('owner.json','wait-stdout.txt'):(original/name).write_bytes((evidence/'business-races/budget-initialization'/name).read_bytes())
            (control/'business-races/initial-budget.tar').write_bytes(originals['business-races'])
            for name in ('budget-initialization.json','budget-initialization-failure01.json'):(public/name).write_bytes((evidence/name).read_bytes())
            states={row['cid']:copy.deepcopy(row['state']) for row in failure['initializers']};started=[];closed=[];gates=[]
            if fault=='business-restarted':states[a['groups'][0]['budget_initializer']]['StartedAt']='2026-10-19T17:30:00Z'
            if fault=='late-already-started':states[a['groups'][1]['budget_initializer']]['Status']='exited'
            if fault=='business-log-changed':(original/'wait-stdout.txt').write_bytes(b'0\nchanged')
            stack.enter_context(patch.object(r,'state',lambda:a));stack.enter_context(patch.object(r,'cold_topology',lambda a:None));stack.enter_context(patch.object(r,'budget_failure',lambda:failure))
            stack.enter_context(patch.object(r,'inspect',lambda cid:{'State':states[cid],'HostConfig':{'RestartPolicy':{'Name':'no'},'PortBindings':{}},'NetworkSettings':{'Ports':{}}}))
            def docker(*args):
                self.assertEqual(args[0],'cp');group=next(g['group'] for g in a['groups'] if args[1].startswith(g['helpers']['wire']))
                if fault=='nonempty-or-wrong-owner' and group=='late-hold':return originals['business-races']
                return originals['business-races'] if any(g['group']==group and g['budget_initializer'] in started for g in a['groups']) else originals[group]
            stack.enter_context(patch.object(r,'docker',docker))
            def owner(cid,destination):
                started.append(cid)
                if fault=='late-failed':raise OSError('recorded initializer failure')
            stack.enter_context(patch.object(r,'run_owner',owner))
            def close(cid):closed.append(cid);return {'cid':cid,'closed':not (fault=='closure-unconfirmed' and cid==a['groups'][1]['budget_initializer'])}
            stack.enter_context(patch.object(r.p.p1,'close_known',close))
            # Exercise the actual shared aggregate+TAR validator for every group
            # at the final gate; only unrelated role/topology I/O is doubled.
            def gate(value):
                gates.append(True)
                for g in value['groups']:r.p.sequence.budget_archive(docker('cp',g['helpers']['wire']+':/run/budget','-'),r.budget_candidate(value),g['group'],initial=True)
            stack.enter_context(patch.object(r,'preparation_gate',gate))
            if fault:
                with self.assertRaises((ValueError,OSError)):r.finish_budget_initialization()
            else:
                r.finish_budget_initialization();r.budget_completion(a)
                self.assertEqual(started,[g['budget_initializer'] for g in a['groups'][1:]])
                self.assertEqual(gates,[True]);self.assertTrue(r.load(public/'budget-initialization02.json')['all_complete'])
                with self.assertRaises(ValueError):r.finish_budget_initialization()
            if fault in ('late-failed','closure-unconfirmed'):
                self.assertEqual(closed,[g['budget_initializer'] for g in a['groups']])
                self.assertFalse(r.load(public/'budget-initialization02.json')['all_complete'])
                if fault=='late-failed':self.assertEqual(started,[a['groups'][1]['budget_initializer']])
            elif fault:self.assertEqual(started,[])
            self.assertNotIn(a['groups'][0]['budget_initializer'],started)
            self.assertEqual((control/'business-races/initial-budget.tar').read_bytes(),originals['business-races'])
            self.assertEqual((public/'budget-initialization.json').read_bytes(),(evidence/'budget-initialization.json').read_bytes())

    def test_budget_continuation_completes_only_never_started_owners(self):self.budget_continuation()

    def test_budget_continuation_rejects_unknown_restarted_used_and_failed_owners(self):
        for fault in ('business-restarted','late-already-started','business-log-changed','nonempty-or-wrong-owner','late-failed','closure-unconfirmed'):
            with self.subTest(fault=fault):self.budget_continuation(fault)

    def composition(self,fail_dump=False):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            control,public=Path(tmp)/'private',Path(tmp)/'public';control.mkdir();public.mkdir()
            stack.enter_context(patch.object(r,'CONTROL',control));stack.enter_context(patch.object(r,'PUBLIC',public))
            state={'groups':[],'owners':[]};states={};calls=[];roles=[];baseline={'relations':[]}
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
            stack.enter_context(patch.object(r,'baseline_core',lambda group:baseline))
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
            if fail_dump:
                with self.assertRaises(OSError):r.run_non_delete()
            else:r.run_non_delete()
            self.assertFalse(any(states.values()))
            terminal=r.load(public/'non-delete-terminal.json')
            if fail_dump:
                self.assertEqual(terminal['status'],'FAILED_OR_UNCONFIRMED_RETAINED')
                self.assertFalse(any(row==('start','late-hold-source-core') for row in calls))
            else:
                self.assertEqual(terminal['status'],'PASS_NONDELETE_SOURCE_RESTORE_PREPARATION')
                self.assertEqual(terminal['groups_completed'],3)
                self.assertEqual(roles,[(g,role) for g in r.p.GROUPS for role in ('PROVISION','SOURCE','RESTORE')])
                self.assertEqual(sum('pg_dump' in row for row in calls),3)
                self.assertEqual(sum('pg_restore' in row for row in calls),6)

    def test_full_three_group_source_backup_restore_sequence(self):self.composition()
    def test_dump_failure_stops_new_groups_and_closes_every_known_owner(self):self.composition(True)


if __name__=='__main__':unittest.main()
