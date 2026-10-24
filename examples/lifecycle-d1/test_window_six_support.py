"""Trusted H1 and containment composition; recorded Docker only, no provider."""
from contextlib import ExitStack
import copy
import datetime as dt
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('six_support_under_test',Path(__file__).with_name('window_six_support.py'))
w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)


class SixSupportTests(unittest.TestCase):
    def cli_maintenance(self,action,fault=None):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            private=Path(tmp);control=private/'authorized';control.mkdir()
            folder=private/'late-hold';folder.mkdir();case=folder/'case-late';case.mkdir()
            profile=case/'remaining.json'
            original=b'{"Base":{"Enabled":true,"Batch":"fixture-only"},"Group":"late-hold"}\r\n'
            profile.write_bytes(original)
            output=folder/'outputs/root-disable-all';output.mkdir(parents=True)
            receipts={'result.json':b'{"retained":1}\r\n','status-changes.json':b'{"disabled":true}\n'}
            for name,raw in receipts.items():(output/name).write_bytes(raw)
            (output/'attempt-start.json').write_bytes(b'original journal\n')
            cid='a'*64
            group={'Group':'late-hold','CaseHelpers':{'late':cid},'CaseConfigSHA256':{'late':w.base.digest(original)}}
            if fault=='profile-drift':group['CaseConfigSHA256']['late']='0'*64
            states={'State':{'Running':False,'Pid':0,'ExitCode':0},'HostConfig':{'RestartPolicy':{'Name':'no'}}}
            if fault=='live-owner':states['State'].update(Running=True,Pid=4)
            if fault=='failed-owner':states['State']['ExitCode']=1
            class RecordedHost(w.Host):
                def __init__(self,containment=False):
                    self.candidate={'Groups':[group]}
                    self.bindings=[{'group':'late-hold','root_owners':{'disable-all':{'container':cid}}}]
                def root_owner(self,bound,stage):
                    # The real inherited archive_root must receive the bound
                    # object before its external root-owner I/O boundary.
                    root=self.binding(bound)['root_owners'][stage]
                    if root['container']!=cid:raise AssertionError('wrong fixed owner')
                    return copy.deepcopy(states),output
                def inspect(self,ident):
                    if ident!=cid:raise AssertionError('unbound case owner')
                    return copy.deepcopy(states)
            for obj,name,value in [(w,'PRIVATE',private),(w,'CONTROL',control),(w.base,'PRIVATE',private),(w.base,'CONTROL',control)]:stack.enter_context(patch.object(obj,name,value))
            at=w.p.window.instant(w.p.W['NotBefore'])+dt.timedelta(minutes=1)
            stack.enter_context(patch.object(w.base,'now',lambda:at))
            stack.enter_context(patch.object(w.base,'START',w.p.window.instant(w.p.W['NotBefore'])))
            stack.enter_context(patch.object(w.base,'END',w.p.window.instant(w.p.W['NotAfter'])))
            stage='disable-all' if action=='archive-root' else 'late'
            if fault=='unknown-stage':stage='../../unbound'
            stack.enter_context(patch.object(w.base.sys,'argv',['window_six_support.py',action,'--group','late-hold','--stage',stage]))
            stack.enter_context(patch.object(w,'Host',RecordedHost))
            stack.enter_context(patch('subprocess.Popen',side_effect=AssertionError('offline CLI test forbids external processes')))
            stack.enter_context(patch('builtins.print'))
            if fault:
                with self.assertRaises((ValueError,KeyError)):w.main()
                self.assertEqual(profile.read_bytes(),original)
                self.assertFalse((control/'revocations').exists())
                for name,raw in receipts.items():self.assertEqual((output/name).read_bytes(),raw)
            elif action=='archive-root':
                w.main()
                history=list((output/'.history').iterdir());self.assertEqual(len(history),1)
                for name,raw in receipts.items():
                    self.assertFalse((output/name).exists())
                    self.assertEqual((history[0]/name).read_bytes(),raw)
                self.assertEqual((output/'attempt-start.json').read_bytes(),b'original journal\n')
                self.assertEqual(profile.read_bytes(),original)
            else:
                w.main()
                revocation=control/'revocations/late-hold/late'
                self.assertEqual((revocation/'original.private').read_bytes(),original)
                expected=json.loads(original);expected['Base']['Enabled']=False
                self.assertEqual(json.loads(profile.read_bytes()),expected)
                self.assertEqual(w.load(revocation/'revocation.json')['before_sha256'],w.base.digest(original))
                for name,raw in receipts.items():self.assertEqual((output/name).read_bytes(),raw)

    def test_archive_root_main_preserves_receipts_and_journal(self):
        self.cli_maintenance('archive-root')

    def test_revoke_case_main_preserves_original_and_only_disables_case(self):
        self.cli_maintenance('revoke-case')

    def test_main_maintenance_rejects_live_failed_drifted_or_unbound_owners(self):
        for action,fault in [('archive-root','live-owner'),('archive-root','unknown-stage'),('revoke-case','failed-owner'),('revoke-case','profile-drift'),('revoke-case','unknown-stage')]:
            with self.subTest(action=action,fault=fault):self.cli_maintenance(action,fault)

    def test_main_nonmaintenance_actions_keep_containment_and_dispatch_boundary(self):
        for action,method in [('check','check'),('seal','seal'),('h1-proof','h1_proof')]:
            with self.subTest(action=action),patch.object(w.base.sys,'argv',['window_six_support.py',action]),patch.object(w,'Host') as host,patch('builtins.print'):
                getattr(host.return_value,method).return_value={}
                w.main()
                host.assert_called_once_with(containment=action=='seal')
                getattr(host.return_value,method).assert_called_once_with()
                host.return_value.group.assert_not_called()

    def test_main_missing_maintenance_context_refuses_before_group_lookup(self):
        for action in ('archive-root','revoke-case'):
            for tail in ([],['--group','late-hold'],['--stage','late']):
                with self.subTest(action=action,tail=tail),patch.object(w.base.sys,'argv',['window_six_support.py',action,*tail]),patch.object(w,'Host') as host,patch('builtins.print'):
                    with self.assertRaises(ValueError):w.main()
                    host.return_value.group.assert_not_called()
                    host.return_value.archive_root.assert_not_called()
                    host.return_value.revoke_case.assert_not_called()

    def test_unknown_initial_inspect_still_stops_known_cid(self):
        cid='a'*64
        for fault in (None,'final-inspect','stop-fails'):
            calls=[];reads=[]
            class RecordedHost(w.Host):
                def __init__(self):pass
                def inspect(self,ident):
                    reads.append(ident)
                    if len(reads)==1 or fault=='final-inspect':raise OSError('recorded inspect unavailable')
                    return {'State':{'Running':False,'Pid':0},'HostConfig':{'RestartPolicy':{'Name':'no'},'PortBindings':{}},'NetworkSettings':{'Ports':{}}}
                def docker(self,*args,**kw):
                    calls.append(args)
                    if fault=='stop-fails':raise OSError('recorded stop result unavailable')
                    return b''
            if fault=='final-inspect':
                with self.assertRaises(ValueError):RecordedHost().stop(cid)
            else:self.assertTrue(RecordedHost().stop(cid)['stopped_pid_zero'])
            self.assertEqual(calls,[('stop','--time','15',cid)])

    def h1(self,fault=None):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            stack.enter_context(patch.object(w.execution_freeze,'authorized_gate',return_value={'status':'RECORDED_BRANCH_FREEZE_DOUBLE'}))
            private=Path(tmp);control=private/'authorized';control.mkdir();folder=private/'late-hold';folder.mkdir()
            for obj,name,value in [(w,'PRIVATE',private),(w,'CONTROL',control),(w.base,'PRIVATE',private),(w.base,'CONTROL',control)]:stack.enter_context(patch.object(obj,name,value))
            at=w.p.window.instant(w.p.W['NotBefore'])+dt.timedelta(minutes=5)
            stack.enter_context(patch.object(w.base,'now',lambda:at));stack.enter_context(patch.object(w.base,'START',w.p.window.instant(w.p.W['NotBefore'])));stack.enter_context(patch.object(w.base,'END',w.p.window.instant(w.p.W['NotAfter'])))
            from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
            from cryptography.hazmat.primitives.serialization import Encoding,PrivateFormat,PublicFormat,NoEncryption
            key=Ed25519PrivateKey.generate();(private/'topology-key.private').write_bytes(key.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption()))
            (private/'reader').write_bytes(b'bounded-test-reader');binary=w.sha(private/'reader')
            original={'Group':'late-hold','Base':{'Enabled':True}};late=folder/'case-late';late.mkdir();raw=json.dumps(original).encode()
            disabled=copy.deepcopy(original);disabled['Base']['Enabled']=False;w.r.save(late/'remaining.json',disabled)
            late_cid='a'*64;gateway_cid='b'*64;root_cid='c'*64
            close=folder/'case-close-H1';close.mkdir();cfg={'ClosureOwners':{late_cid:binary},'ClosurePublicKey':key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex(),'ClosureCandidateSHA256':'d'*64};w.r.save(close/'remaining.json',cfg)
            revocation=control/'revocations/late-hold/late';revocation.mkdir(parents=True);(revocation/'original.private').write_bytes(raw)
            w.r.save(revocation/'revocation.json',{'before_sha256':w.base.digest(raw),'disabled_sha256':w.sha(late/'remaining.json')})
            gateway=folder/'gateway.Caddyfile';gateway.write_bytes(b':9000 { respond 403 }')
            users={'delete754ea8f0':'old-delete','inspect754ea8f06ffc':'old-inspect','iafc0e66c754ea8f0':'new-inspect','dafc0e66c754ea8f0':'new-delete'}
            iam=folder/'root-disable-all';iam.mkdir();w.r.save(iam/'iam.json',{'disabled_identities':users})
            group={'Group':'late-hold','Gateway':gateway_cid,'IAMSHA256':w.sha(iam/'iam.json'),'SealedGatewaySHA256':w.sha(gateway),'CaseHelpers':{'late':late_cid},'CaseConfigSHA256':{'late':w.base.digest(raw),'close-H1':w.sha(close/'remaining.json')},'Helpers':{'wire':'w'*64,'H2':'h'*64,'recover-H2':'q'*64}}
            roots={action:{'container':str(i)*64} for i,action in enumerate(('check-disabled','enable-read','enable-send','disable-send','disable-all'),1)}
            for stage in group['Helpers']:(folder/stage).mkdir();w.r.save(folder/stage/'permission.json',{'Enabled':False})
            for action in roots:
                target=folder/('root-'+action);target.mkdir(exist_ok=True);w.r.save(target/'permission.json',{'Enabled':False})
            def state(cid):return {'Id':cid,'Image':w.p.p1.PG,'Config':{'User':'10001:10001','Entrypoint':['/run/reader'],'Cmd':['-mode','remaining-late'],'Env':[]},'State':{'Running':False,'Pid':0,'ExitCode':0,'FinishedAt':(at-dt.timedelta(seconds=2)).isoformat()},'HostConfig':{'NetworkMode':'container:'+gateway_cid,'ReadonlyRootfs':True,'Privileged':False,'AutoRemove':False,'RestartPolicy':{'Name':'no'},'CapDrop':['ALL'],'CapAdd':None,'SecurityOpt':['no-new-privileges'],'Tmpfs':{'/tmp':'','/var/lib/postgresql/data':'size=1m'},'PortBindings':{}},'Mounts':[],'NetworkSettings':{'Ports':{}}}
            states={cid:state(cid) for cid in [late_cid,gateway_cid,root_cid,*group['Helpers'].values(),*[x['container'] for x in roots.values()]]}
            def mount(target,path,rw):return {'Destination':target,'Source':str(path),'RW':rw,'Type':'bind'}
            states[late_cid]['Mounts']=[mount('/run/private',late,False),mount('/run/reader',private/'reader',False),mount('/run/output',folder/'outputs/case-late',True)]
            states[gateway_cid]['Mounts']=[mount('/etc/caddy/Caddyfile',gateway,False)]
            output=folder/'outputs/root-check-disabled';output.mkdir(parents=True)
            result={'action':'check-disabled','group':'late-hold','exact_policy_set_verified':True,'before':{name:'disabled' for name in users}}
            if fault=='old-users-only':result['before']={name:'disabled' for name in list(users)[:2]}
            if fault=='new-sender-enabled':result['before']['dafc0e66c754ea8f0']='enabled'
            w.r.save(output/'result.json',result)
            if fault=='owner-live':states[late_cid]['State']['Pid']=5
            if fault=='enabler-live':states[roots['enable-send']['container']]['State']['Running']=True
            if fault=='stale-check':states[root_cid]['State']['FinishedAt']=(at-dt.timedelta(seconds=46)).isoformat()
            if fault=='gateway-live':states[gateway_cid]['State']['Running']=True
            if fault=='expanded-mount':states[late_cid]['Mounts'].append(mount('/run/host-key',private/'topology-key.private',False))
            class RecordedHost(w.Host):
                def __init__(self):self.candidate={'ReaderBinarySHA256':binary,'PublicKey':cfg['ClosurePublicKey']}
                def group(self,name):return group
                def binding(self,g):return {'root_owners':roots}
                def inspect(self,cid):return copy.deepcopy(states[cid])
                def root_owner(self,g,action):return states[root_cid],output
            h=RecordedHost()
            if fault:
                with self.assertRaises(ValueError):h.h1_proof()
                self.assertFalse((close/'closed-H1.json').exists())
            else:
                receipt=h.h1_proof();self.assertEqual(receipt['execution'],w.p.EXECUTION)
                self.assertTrue((close/'closed-H1.json').exists())
                with self.assertRaises(FileExistsError):h.h1_proof()

    def test_fresh_four_identity_h1_closure_and_current_counterexamples(self):
        for fault in (None,'old-users-only','new-sender-enabled','owner-live','enabler-live','stale-check','gateway-live','expanded-mount'):
            with self.subTest(fault=fault):self.h1(fault)


if __name__=='__main__':unittest.main()
