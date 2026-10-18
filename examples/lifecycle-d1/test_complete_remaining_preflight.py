"""Full CLI: actual host gate + three recorded native collectors/checkers.

Docker is a recording double. No credentials, provider calls or live resources.
"""
import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('whole_preflight',Path(__file__).with_name('verify-remaining-native.py'))
v=importlib.util.module_from_spec(spec);spec.loader.exec_module(v)
PACKET=v.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate11'
RECORDED=PACKET.parent/'delete-confirmed-v21-window01/fresh-native'

def write(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_text(json.dumps(value),encoding='utf-8')

class FullDocker:
    def __init__(self,root):
        self.root=root;self.packet=root/'packet';self.owner=root/'owner';self.calls=[]
        shutil.copytree(PACKET,self.packet);self.owner.mkdir()
        for name in ('reader','operator','owner','checker'):(self.owner/name).write_bytes(('fake-'+name).encode())
        self.c=v.load(self.packet/'candidate.json');self.b=v.load(self.packet/'bindings.json')
        for key in ('Execution','Profile','NotBefore','NotAfter'):self.c[key]=v.window.compiled()[key]
        for name,key in [('reader','ReaderBinarySHA256'),('operator','SenderBinarySHA256'),('owner','OwnerBinarySHA256')]:self.c[key]=v.sha(self.owner/name)
        self.states={};self.nets={};self.volumes={};self.technical={};self.mount_users={};self.variant=None
        projections={'cases':[],'recoveries':[]}
        for g,b in zip(self.c['Groups'],self.b):
            folder=self.owner/g['Group'];folder.mkdir()
            for field,image in [('Core',v.runner.PG),('Native',v.runner.MI),('Gateway',v.runner.GW),('CheckpointCore',v.runner.PG),('CheckpointNative',v.runner.MI)]:self.states[g[field]]=self.state(g[field],image,g)
            self.states[b['recovery_redis']]=self.state(b['recovery_redis'],v.runner.REDIS,g)
            self.nets[g['Network']]={'Id':g['Network'],'Internal':True}
            self.volumes[b['budget_volume']]={'Name':b['budget_volume'],'Labels':{'lifecycle307.remaining':self.c['Execution']}}
            self.mount_users[b['budget_volume']]=list(g['Helpers'].values())
            gateway=folder/'sealed.Caddyfile';gateway.write_bytes((self.packet/g['Group']/'gateway-sealed.Caddyfile').read_bytes())
            self.states[g['Gateway']]['Mounts']=[self.mount('/etc/caddy/Caddyfile',gateway,False)]
            for file,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:
                path=root/'fake-private/remaining-executable03'/g['Group']/'checkpoint'/file
                path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes((g['Group']+file).encode());g[key]=v.sha(path)
            for stage,ident in g['Helpers'].items():
                stage_folder=folder/stage;write(stage_folder/'operator.json',{'DeleteSecret':'','WirePrivateKey':''})
                g['ConfigSHA256'][stage]=v.sha(stage_folder/'operator.json')
                self.states[ident]=self.state(ident,v.runner.PG,g)
                self.states[ident]['HostConfig']['NetworkMode']='container:'+g['Gateway']
                self.states[ident]['Config'].update(Entrypoint=['/run/operator'],Cmd=['-stage',stage])
                self.states[ident]['Mounts']=[self.mount('/run/private',stage_folder,False),self.mount('/run/operator',self.owner/'operator',False),self.mount('/run/output',folder/'outputs'/stage,True),{'Destination':'/run/budget','Source':'/volume','RW':True,'Type':'volume','Name':b['budget_volume']}]
                if stage.startswith('recover-'):
                    rc={'Enabled':True,'SyntheticOnly':True,'ValidUntil':v.window.compiled()['RecoveryEnd']}
                    write(stage_folder/'recovery.json',rc)
                    projections['recoveries'].append({'group':g['Group'],'stage':stage,'config_sha256':v.sha(stage_folder/'recovery.json'),'ValidUntil':rc['ValidUntil']})
            for stage,ident in g['CaseHelpers'].items():
                cfg=v.window.case_profile({'Group':g['Group'],'Base':{'Enabled':True,'ManifestSHA256':b['manifest_sha256']}},g['PolicyHeads'],'fake-only','fake-epoch')
                path=folder/('case-'+stage)/'remaining.json';write(path,cfg);g['CaseConfigSHA256'][stage]=v.sha(path)
                shutil.copyfile(PACKET.parent/'remaining-native01'/g['Group']/'manifest.json',path.parent/'manifest.json')
                projections['cases'].append(v.window.project_case(cfg,g['Group'],stage,v.sha(path)))
                item=self.state(ident,v.runner.PG,g);self.states[ident]=item
                item['HostConfig']['NetworkMode']='container:'+g['Gateway']
                item['Config'].update(Entrypoint=['/run/reader'],Cmd=['-mode',v.window.CASE_MODES[g['Group']][stage]])
                item['Mounts']=[self.mount('/run/private',path.parent,False),self.mount('/run/reader',self.owner/'reader',False),self.mount('/run/output',folder/'outputs',True),self.mount('/run/repo/industry-packs',v.ROOT/'industry-packs',False)]
            for action,root_owner in b['root_owners'].items():
                path=folder/('root-'+action);write(path/'root.json',{'Group':g['Group']});root_owner['root_config_sha256']=v.sha(path/'root.json')
                item=self.state(root_owner['container'],v.runner.PG,g);self.states[item['Id']]=item
                item['HostConfig']['NetworkMode']='container:'+g['Native'];item['Config'].update(Entrypoint=['/run/owner'],Cmd=['-action',action])
                item['Mounts']=[self.mount('/run/private',path,False),self.mount('/run/owner',self.owner/'owner',False),self.mount('/run/output',folder/'outputs'/('root-'+action),True)]
            old=root/'fake-private/remaining-executable03'/g['Group']/'source-verifier'
            write(old/'remaining.json',{'Base':{}});write(old/'inventory.json',{});write(old/'manifest.json',{})
        self.states['25a4e218f72527e8f93a1df2eeb7a09833e5028c03d5ee178009bd9c71ee33e6']=self.state('25a4e218f72527e8f93a1df2eeb7a09833e5028c03d5ee178009bd9c71ee33e6',v.runner.REDIS,self.c['Groups'][0])
        write(self.packet/'profile-windows.json',projections);self.c['ProfileWindowsSHA256']=v.sha(self.packet/'profile-windows.json')
        write(self.packet/'candidate.json',self.c);write(self.packet/'bindings.json',self.b)
        (self.packet/'confirmation-template.txt').write_bytes(b'NON_AUTHORITATIVE_RECORDING_TEST_ONLY\n')
        common={'Enabled':False,'Execution':self.c['Execution'],'CandidateSHA256':v.sha(self.packet/'candidate.json'),'ConfirmationSHA256':v.sha(self.packet/'confirmation-template.txt'),'NotBefore':self.c['NotBefore'],'ExpiresAt':self.c['NotAfter'],'ConfirmedAt':'0001-01-01T00:00:00Z','MaxPerTarget':1,'MaxTotal':7}
        for g,b in zip(self.c['Groups'],self.b):
            for stage in g['Helpers']:
                folder=self.owner/g['Group']/stage;write(folder/'permission.json',dict(common,Profile=self.c['Profile'],BinarySHA256=self.c['SenderBinarySHA256']));shutil.copyfile(self.packet/'candidate.json',folder/'candidate.json');shutil.copyfile(self.packet/'confirmation-template.txt',folder/'confirmation-template.txt')
            for action in b['root_owners']:write(self.owner/g['Group']/('root-'+action)/'permission.json',common)
    @staticmethod
    def mount(target,source,rw):return {'Destination':target,'Source':str(source),'RW':rw}
    @staticmethod
    def state(ident,image,g):
        return {'Id':ident,'Image':image,'State':{'Running':False,'Pid':0},'Mounts':[],
            'Config':{'Entrypoint':[],'Cmd':[],'Env':[]},
            'HostConfig':{'RestartPolicy':{'Name':'no'},'PortBindings':{},'ReadonlyRootfs':True,'Privileged':False,'AutoRemove':False,'CapDrop':['ALL'],'SecurityOpt':['no-new-privileges'],'NetworkMode':'private'},
            'NetworkSettings':{'Ports':{},'Networks':{'private':{'NetworkID':g['Network']}}}}
    def inspect(self,ident):return copy.deepcopy(self.states[ident])
    def docker(self,*args):
        self.calls.append(args);action=args[0]
        if action=='network':return json.dumps([self.nets[args[-1]]]).encode()
        if action=='volume':return json.dumps([self.volumes[args[-1]]]).encode()
        if action=='ps':
            volume=args[-1].split('=',1)[1];raw='\n'.join(self.mount_users[volume])+'\n'
            return raw if self.variant=='str' else raw.encode()
        if action=='cp':
            if self.variant=='nonempty':(Path(args[-1])/'used-attempt.json').write_bytes(b'{}')
            return b''
        if action=='create':
            namespace=args[args.index('--network')+1].removeprefix('container:');g=next(g for g in self.c['Groups'] if g['Native']==namespace)
            ident=hashlib.sha256((g['Group']+'technical').encode()).hexdigest();item=self.state(ident,v.runner.PG,g)
            item['HostConfig']['NetworkMode']='container:'+g['Native']
            for i,arg in enumerate(args):
                if arg=='--mount':
                    fields=dict(p.split('=',1) for p in args[i+1].split(',') if '=' in p);item['Mounts'].append(self.mount(fields['target'],fields['source'],'readonly' not in args[i+1].split(',')))
            self.technical[ident]=(g,next(Path(m['Source']) for m in item['Mounts'] if m['Destination']=='/run/output'));self.states[ident]=item
            return ident.encode()
        if action=='start':
            self.states[args[1]]['State'].update(Running=True,Pid=123)
            if args[1] in self.technical:
                g,output=self.technical[args[1]]
                for p in (RECORDED/g['Group']/'readonly').iterdir():
                    if p.is_file():shutil.copyfile(p,output/p.name)
                self.states[args[1]]['State'].update(Running=False,Pid=0,StartedAt=v.now().isoformat(),FinishedAt=v.now().isoformat())
            return b''
        if action=='wait':return b'0'
        if action=='stop':self.states[args[-1]]['State'].update(Running=False,Pid=0);return b''
        raise ValueError('unexpected Docker command '+str(args))
    def fingerprints(self,core):
        g=next(g for g in self.c['Groups'] if g['Core']==core)
        return v.load(self.packet/g['Group']/'core-readonly.json')

class CompleteCLI(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temp=tempfile.TemporaryDirectory();cls.checker=Path(cls.temp.name)/('checker.exe' if sys.platform=='win32' else 'checker')
        subprocess.run(['go','build','-o',str(cls.checker),'./cmd/lifecycle-fixture-readonly-check'],cwd=v.ROOT/'apps/platform',check=True,capture_output=True)
    @classmethod
    def tearDownClass(cls):cls.temp.cleanup()
    def run_entry(self,mutate=None,denied=False,after=None):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);d=FullDocker(root)
            if mutate:mutate(d)
            argv=['verify-remaining-native.py','--packet',str(d.packet),'--candidate-sha256',v.sha(d.packet/'candidate.json'),'--bindings-sha256',v.sha(d.packet/'bindings.json'),'--private',str(root/'collection'),'--output',str(root/'public'),'--reader',str(d.owner/'reader'),'--checker',str(self.checker),'--checker-sha256',v.sha(self.checker)]
            with patch.object(sys,'argv',argv),patch.object(v,'PRIVATE',root/'fake-private'),patch.object(v.runner,'docker',d.docker),patch.object(v.runner,'inspect',d.inspect),patch.object(v.runner.native,'wait_pg'),patch.object(v.runner.native,'fingerprints',side_effect=d.fingerprints),patch.object(v.time,'sleep'):
                if denied:
                    with self.assertRaises((ValueError,TypeError)):v.main()
                    self.assertTrue((root/'public/failure.json').is_file())
                    self.assertFalse((root/'public/complete.json').exists())
                    self.assertFalse(any(call[0] in ('start','create') for call in d.calls))
                else:
                    v.main();result=v.load(root/'public/complete.json')
                    self.assertEqual(result['technical_requests'],76)
                    self.assertEqual(result['host_gate']['status'],'PASS_COMPLETE_SEALED_HOST_GATE')
                    self.assertEqual(len(result['host_gate']['cases']),7);self.assertEqual(len(result['host_gate']['budgets']),3)
                    self.assertEqual(result['host_gate']['permissions_false'],32)
                    self.assertEqual(sum(call[0]=='start' for call in d.calls),9)
                    self.assertEqual(sum(call[0]=='cp' for call in d.calls),3)
                    self.assertTrue(all(v.closed(item) for item in d.states.values()))
                    self.assertTrue(all(v.load(root/'public'/g['Group']/'physical-close.json')['all_closed'] for g in d.c['Groups']))
                    if after:after(d,root)
    def test_success_combines_all_host_gates_and_three_real_file_aggregations(self):self.run_entry()
    def test_wrong_output_type_fails_before_any_source_start(self):self.run_entry(lambda d:setattr(d,'variant','str'),True)
    def test_wrong_budget_execution_owner_fails(self):self.run_entry(lambda d:next(iter(d.volumes.values()))['Labels'].update({'lifecycle307.remaining':'wrong'}),True)
    def test_expanded_mount_membership_fails(self):self.run_entry(lambda d:next(iter(d.mount_users.values())).append('f'*64),True)
    def test_missing_mount_member_fails(self):self.run_entry(lambda d:next(iter(d.mount_users.values())).pop(),True)
    def test_nonempty_budget_fails(self):self.run_entry(lambda d:setattr(d,'variant','nonempty'),True)
    def test_case_mount_expansion_fails(self):
        def mutate(d):d.states[next(iter(d.c['Groups'][0]['CaseHelpers'].values()))]['Mounts'].append(d.mount('/docker.sock','/host/docker.sock',False))
        self.run_entry(mutate,True)
    def test_external_network_fails(self):self.run_entry(lambda d:next(iter(d.nets.values())).update(Internal=False),True)
    def test_case_private_root_file_fails(self):
        self.run_entry(lambda d:write(d.owner/'business-races/case-renew/root.json',{'fake':'no usable credential'}),True)
    def test_recovery_private_host_key_fails(self):
        self.run_entry(lambda d:(d.owner/'business-races/recover-P1/topology-key.private').write_bytes(b'fake'),True)
    def test_case_private_unbound_directory_fails(self):
        self.run_entry(lambda d:(d.owner/'business-races/case-renew/unbound').mkdir(),True)
    def test_offline_checker_requires_complete_exact_members_and_invocation(self):
        def check(d,root):
            spec=importlib.util.spec_from_file_location('current_offline_receipt',Path(__file__).with_name('check-remaining-candidate12.py'))
            a=importlib.util.module_from_spec(spec);spec.loader.exec_module(a)
            shutil.copytree(root/'public',d.packet/'complete-entry')
            write(d.packet/'complete-entry-invocation.json',{'exit_code':0,'entry_sha256':v.sha(v.ROOT/'examples/lifecycle-d1/verify-remaining-native.py')})
            a.complete_receipt(d.packet,d.c)
            path=d.packet/'complete-entry/complete.json';original=path.read_bytes()
            mutations=[lambda r:r.update(groups=[]),lambda r:r.update(groups=r['groups'][:2]),
                lambda r:r['groups'][0].update(status='FAILED'),lambda r:r.update(versions=17),
                lambda r:r.update(technical_requests=0),lambda r:r['host_gate'].update(permissions_false=0)]
            for mutate in mutations:
                with self.subTest(mutate=mutate):
                    value=json.loads(original);mutate(value);write(path,value)
                    with self.assertRaises(ValueError):a.complete_receipt(d.packet,d.c)
            path.write_bytes(original)
            invocation=d.packet/'complete-entry-invocation.json'
            for change in ({'exit_code':1},{'entry_sha256':'wrong'}):
                value={'exit_code':0,'entry_sha256':v.sha(v.ROOT/'examples/lifecycle-d1/verify-remaining-native.py')};value.update(change);write(invocation,value)
                with self.assertRaises(ValueError):a.complete_receipt(d.packet,d.c)
        self.run_entry(after=check)

class DockerOutputContract(unittest.TestCase):
    def test_real_wrapper_returns_bytes_and_explicit_text(self):
        done=subprocess.CompletedProcess(['docker'],0,stdout=b'a\nb\n',stderr=b'')
        with patch.object(v.runner.subprocess,'run',return_value=done):
            self.assertEqual(v.runner.docker('ps'),b'a\nb\n')
            self.assertEqual(v.runner.docker_text('ps').split(),['a','b'])
    def test_real_wrapper_rejects_str_including_empty_str(self):
        for raw in ('already decoded',''):
            with self.subTest(raw=raw),patch.object(v.runner.subprocess,'run',return_value=subprocess.CompletedProcess(['docker'],0,stdout=raw,stderr=b'')):
                with self.assertRaises(TypeError):v.runner.docker('ps')
    def test_explicit_text_boundary_rejects_str_and_invalid_utf8(self):
        for raw,error in [('wrong',TypeError),(b'\xff',UnicodeDecodeError)]:
            with self.subTest(raw=raw),patch.object(v.runner,'docker',return_value=raw):
                with self.assertRaises(error):v.runner.docker_text('ps')

if __name__=='__main__':unittest.main(verbosity=2)
