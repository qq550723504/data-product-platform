"""Full host composition with recorded process/provider doubles only.

No Docker daemon, live Core, IAM or object store is contacted. Go contract tests
separately exercise the actual SDK/official policy library and file aggregation.
"""
from contextlib import ExitStack
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('six_host_under_test',Path(__file__).with_name('six_candidate_runtime.py'))
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)


class SixHostTests(unittest.TestCase):
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

    def composition(self,fail_dump=False):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            control,public=Path(tmp)/'private',Path(tmp)/'public';control.mkdir();public.mkdir()
            stack.enter_context(patch.object(r,'CONTROL',control));stack.enter_context(patch.object(r,'PUBLIC',public))
            state={'groups':[],'owners':[]};states={};calls=[];roles=[];baseline={'relations':[]}
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
