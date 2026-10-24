"""Host composition doubles only; no Docker/provider or retained source I/O."""
import base64
import copy
import ctypes
import datetime as dt
import json
import os
import subprocess
from pathlib import Path
import tempfile
import unittest
import uuid
from unittest.mock import patch
import retention_maintenance as m

class HostTests(unittest.TestCase):
    def setUp(self):
        self.metadata={}
        self.enterContext(patch.object(m.staging,'validate_directory',return_value={}))
        def docker(*args):
            self.assertEqual(args[:2],('volume','inspect'))
            return json.dumps([self.metadata[args[2]]]).encode()
        self.enterContext(patch.object(m,'docker',side_effect=docker))

    def fixture(self,folder):
        at=dt.datetime(2026,10,23,14,tzinfo=dt.timezone.utc)
        binary=folder/'retention';binary.write_bytes(b'fake bound binary')
        b={'Kind':m.KIND,'MaintenanceID':str(uuid.uuid4()),'NotBefore':'2026-10-23T13:59:00Z','NotAfter':'2026-10-23T14:30:00Z',
           'RetainUntil':'2026-10-24T12:40:00Z','PlanSHA256':m.sha(m.proposal.plan_bytes('2026-10-24T12:40:00Z','2026-10-23T14:30:00Z')),'BinarySHA256':m.sha(binary.read_bytes()),'HostPublicSHA256':'a'*64,'BudgetID':'b'*64,'Owners':[]}
        host={'bindings_sha256':'','owners':{},'closed_cids':['7'*64],'budget_name':'d1-retention-budget-test','binary':str(binary),'volume_metadata':self.metadata}
        items={}
        self.metadata[host['budget_name']]=self.volume(host['budget_name'])
        for i,group in enumerate(m.proposal.TARGETS,1):
            private=folder/group;private.mkdir();output=folder/(group+'-output');output.mkdir();(private/'root.json').write_bytes(b'{"mock":"private"}')
            (private/'permission.json').write_text('{"Enabled":true}',encoding='utf-8')
            for name in ['bindings.json','approved-proposal.json','host-public.key','authorization.json','user-message-verbatim.txt','topology.json']:
                (private/name).write_bytes(b'fixture placeholder')
            native=m.proposal.TARGETS[group][2];ident=str(i)*64
            pv='d1-retention-private-'+group;ov='d1-retention-output-'+group
            self.metadata[pv]=self.volume(pv);self.metadata[ov]=self.volume(ov)
            mounts=[{'Destination':dest,'Type':kind,'RW':rw,**({'Name':source,'Source':self.metadata[source]['Mountpoint']} if kind=='volume' else {'Source':source})} for dest,source,rw,kind in [
                ('/run/private',pv,False,'volume'),('/run/output',ov,True,'volume'),('/run/budget',host['budget_name'],True,'volume'),('/runtime/retention',str(binary),False,'bind')]]
            item={'Id':ident,'Image':'sha256:'+'c'*64,'Path':'/runtime/retention','Args':['-wait'],
                  'Config':{'User':'10001:10001','Env':[]},'HostConfig':{'RestartPolicy':{'Name':'no'},'Privileged':False,'ReadonlyRootfs':True,'NetworkMode':'container:'+native,
                    'CapDrop':['ALL'],'SecurityOpt':['no-new-privileges:true'],'Tmpfs':{'/tmp':'rw,noexec,nosuid,size=16m'}},'Mounts':mounts,'State':{'Running':True,'Pid':100+i,'StartedAt':'2026-10-23T13:59:59Z'},'NetworkSettings':{'Ports':{}}}
            desc=m.sha(m.canonical(m.descriptor(item)))
            b['Owners'].append({'Group':group,'CID':ident,'Native':native,'DescriptorSHA256':desc,'ConfigSHA256':m.sha((private/'root.json').read_bytes())})
            host['owners'][group]={'cid':ident,'private':str(private),'output':str(output),'descriptor_sha256':desc,'private_volume':pv,'output_volume':ov}
            items[ident]=item;items[native]={'Id':native,'HostConfig':{'RestartPolicy':{'Name':'no'}},'State':{'Running':True,'Pid':500+i},'NetworkSettings':{'Ports':{}},'Mounts':[]}
        items['2'*64]['State']={'Running':False,'Pid':0}
        items['7'*64]={'Id':'7'*64,'HostConfig':{'RestartPolicy':{'Name':'no'}},'State':{'Running':False,'Pid':0},'NetworkSettings':{'Ports':{}},'Mounts':[]}
        self.bind(b,host)
        return b,host,items,at

    def volume(self,name):
        return {'Name':name,'Driver':'local','Scope':'local','Options':None,'Labels':{'dpp.retention-maintenance':'preparation-only'},'Mountpoint':'/var/lib/docker/volumes/'+name+'/_data'}

    def bind(self,b,host):
        raw=m.canonical(b);host['_bindings_raw']=raw;host['bindings_sha256']=m.sha(raw)
        for o in b['Owners']:
            if o['Group'] not in host['owners']:continue
            (Path(host['owners'][o['Group']]['output'])/'wait-ready.json').write_bytes(m.canonical({'Kind':m.KIND,'MaintenanceID':b['MaintenanceID'],'BindingsSHA256':m.sha(raw),'Group':o['Group'],'Owner':o['CID'],'DescriptorSHA256':o['DescriptorSHA256'],'ConfigSHA256':o['ConfigSHA256'],'At':'2026-10-23T14:00:00Z'}))

    def test_complete_actual_attestation_signature_and_all_group_seal_composition(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);b,host,items,at=self.fixture(root)
            from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
            from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat,PrivateFormat,NoEncryption
            key=Ed25519PrivateKey.generate();path=root/'host.private';path.write_bytes(key.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption()))
            b['HostPublicSHA256']=m.sha(key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw));self.bind(b,host)
            proof=m.attest(b,host,'business-races',path,at,items.__getitem__,lambda:list(items.values()))
            body=base64.b64decode(proof['Body']);key.public_key().verify(base64.b64decode(proof['Signature']),body)
            self.assertEqual(json.loads(body)['Owner'],'1'*64)
            stopped=[]
            def stop(*args):
                self.assertEqual(args[:3],('stop','--time','10'));ident=args[-1];stopped.append(ident);items[ident]['State']={'Running':False,'Pid':0};return b''
            result=m.seal(b,host,items.__getitem__,stop)
            self.assertEqual(result['status'],'PASS_PHYSICAL_MAINTENANCE_CLOSURE')
            self.assertEqual(set(stopped),{'1'*64,*[x['Native'] for x in b['Owners']]})
            self.assertTrue(all(m.load(Path(x['private'])/'permission.json')['Enabled'] is False for x in host['owners'].values()))

    def test_topology_counterexamples_never_sign(self):
        for fault in ['extra-mount','root','ports','restart','extra-budget-owner','other-owner-active','old-owner-active','config','native','inventory-injection','extra-private-file','private-hardlink','volume-alias','extra-output-owner','metadata-drift','not-waiting']:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as d:
                b,host,items,_=self.fixture(Path(d));owner=items['1'*64]
                if fault=='not-waiting':owner['Args']=[]
                elif fault=='extra-mount':owner['Mounts'].append({'Destination':'/socket','Type':'bind','Source':'/var/run/docker.sock','RW':True})
                elif fault=='root':owner['Config']['User']='0'
                elif fault=='ports':owner['NetworkSettings']['Ports']={'9000/tcp':[{}]}
                elif fault=='restart':owner['HostConfig']['RestartPolicy']['Name']='always'
                elif fault=='extra-budget-owner':extra=copy.deepcopy(owner);extra['Id']='8'*64;items[extra['Id']]=extra
                elif fault=='other-owner-active':items['2'*64]['State']={'Running':True,'Pid':200}
                elif fault=='old-owner-active':items['7'*64]['State']={'Running':True,'Pid':700}
                elif fault=='config':(Path(host['owners']['business-races']['private'])/'root.json').write_bytes(b'drift')
                elif fault=='native':items[b['Owners'][0]['Native']]['State']={'Running':False,'Pid':0}
                elif fault=='extra-private-file':(Path(host['owners']['business-races']['private'])/'host.secret').write_bytes(b'forbidden')
                elif fault=='private-hardlink':os.link(Path(host['owners']['business-races']['private'])/'root.json',Path(d)/'root-alias')
                elif fault=='volume-alias':host['owners']['mixed']['output_volume']=host['owners']['business-races']['output_volume']
                elif fault=='extra-output-owner':extra=copy.deepcopy(owner);extra['Id']='8'*64;extra['Mounts']=[owner['Mounts'][1]];items[extra['Id']]=extra
                elif fault=='metadata-drift':host['volume_metadata'][host['owners']['mixed']['output_volume']]['Options']={'device':'/retained'}
                else:host['_all_items']=[]
                with self.assertRaises((ValueError,KeyError)):m.topology(b,host,'business-races',items.__getitem__,lambda:list(items.values()))

    def test_next_owner_requires_previous_exit_and_terminal_readback(self):
        for fault in ['none','exit','failed','unconfirmed','missing']:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as d:
                b,host,items,_=self.fixture(Path(d));items['1'*64]['State']={'Running':False,'Pid':0,'ExitCode':0}
                items['2'*64]['State']={'Running':True,'Pid':200}
                receipt={'MaintenanceID':b['MaintenanceID'],'BindingsSHA256':host['bindings_sha256'],'Status':'PASS_EXACT_RETENTION_READBACK','readback_confirmed':True}
                if fault=='exit':items['1'*64]['State']['ExitCode']=1
                elif fault=='failed':receipt['Status']='FAILED_NO_AUTOMATIC_RETRY'
                elif fault=='unconfirmed':receipt['readback_confirmed']=False
                path=Path(host['owners']['business-races']['output'])/'terminal.json'
                if fault!='missing':path.write_bytes(m.canonical(receipt))
                if fault=='none':m.topology(b,host,'mixed',items.__getitem__,lambda:list(items.values()))
                else:
                    with self.assertRaises((ValueError,FileNotFoundError)):m.topology(b,host,'mixed',items.__getitem__,lambda:list(items.values()))

    def test_real_binary_directory_aliases_share_one_mount_identity(self):
        modes=['canonical','upper-case','short-name'] if os.name=='nt' else ['canonical']
        for mode in modes:
            with self.subTest(mode=mode),tempfile.TemporaryDirectory(prefix='retention-path-alias-') as directory:
                folder=Path(directory).resolve()
                if mode=='upper-case':folder=Path(str(folder).upper())
                elif mode=='short-name':
                    query=ctypes.windll.kernel32.GetShortPathNameW
                    query.argtypes=[ctypes.c_wchar_p,ctypes.c_wchar_p,ctypes.c_uint];query.restype=ctypes.c_uint
                    buffer=ctypes.create_unicode_buffer(32768)
                    self.assertGreater(query(str(folder),buffer,len(buffer)),0)
                    folder=Path(buffer.value)
                self.assertTrue(folder.samefile(folder.resolve()))
                b,host,items,_=self.fixture(folder)
                m.topology(b,host,'business-races',items.__getitem__,lambda:list(items.values()))

    def test_rehashed_wrong_and_linked_binary_mount_paths_still_rejected(self):
        for fault in ['wrong-directory','linked-alias']:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as directory:
                folder=Path(directory).resolve();b,host,items,_=self.fixture(folder)
                other=folder/'other';other.mkdir()
                if fault=='linked-alias':
                    other.rmdir()
                    try:other.symlink_to(host['owners']['business-races']['private'],target_is_directory=True)
                    except OSError:
                        if os.name!='nt':raise
                        # Existing native PowerShell Junction support needs no
                        # symlink privilege. Arguments are data, not script.
                        result=subprocess.run(['powershell','-NoProfile','-NonInteractive','-Command',
                            '& { param($link,$target) New-Item -ItemType Junction -Path $link -Target $target -ErrorAction Stop | Out-Null }',
                            str(other),host['owners']['business-races']['private']],capture_output=True,timeout=30)
                        self.assertEqual(result.returncode,0,result.stderr.decode('utf-8','replace'))
                    self.assertTrue(other.is_symlink() or other.is_junction())
                try:
                    items['1'*64]['Mounts'][3]['Source']=str(other)
                    # Even a matching newly supplied descriptor cannot approve
                    # another directory or hide a link into the expected one.
                    digest=m.sha(m.canonical(m.descriptor(items['1'*64])))
                    b['Owners'][0]['DescriptorSHA256']=digest;host['owners']['business-races']['descriptor_sha256']=digest
                    self.bind(b,host)
                    with self.assertRaises(ValueError):m.topology(b,host,'business-races',items.__getitem__,lambda:list(items.values()))
                finally:
                    if other.is_junction():other.rmdir()
                    elif other.is_symlink():other.unlink()

    def test_seal_continues_after_archive_and_inspect_failures(self):
        with tempfile.TemporaryDirectory() as d:
            b,host,items,_=self.fixture(Path(d));blocked='1'*64;seen=[]
            # Permission archive unavailable, one initial/final inspect fails.
            output=Path(host['owners']['business-races']['output']);raw=(Path(host['owners']['business-races']['private'])/'permission.json').read_bytes();(output/('permission-before-seal-'+m.sha(raw)+'.json')).mkdir()
            def get(ident):
                if ident==blocked:raise OSError('inspect unavailable')
                return items[ident]
            def call(*args):
                seen.append(args[-1]);items[args[-1]]['State']={'Running':False,'Pid':0}
            result=m.seal(b,host,get,call)
            self.assertEqual(result['status'],'UNCONFIRMED_MAINTENANCE_CLOSURE')
            self.assertIn(blocked,seen)
            self.assertTrue(all(x['Native'] in seen for x in b['Owners']))

    def test_binding_deadline_scope_and_duplicate_json_rejected(self):
        for fault in ['old-id','deadline','plan','group']:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as d:
                b,host,_,_=self.fixture(Path(d))
                if fault=='old-id':b['MaintenanceID']='4fad1007-6595-4037-a7a5-1d8d38cb8bcc'
                elif fault=='deadline':b['NotAfter']='2026-10-24T13:00:00Z'
                elif fault=='plan':b['PlanSHA256']='0'*64
                else:b['Owners'][1]['Group']='late-hold'
                self.bind(b,host)
                with self.assertRaises(ValueError):m.fixed_bindings(b,host)
        with tempfile.TemporaryDirectory() as d:
            path=Path(d)/'bad.json';path.write_text('{"Enabled":false,"Enabled":true}',encoding='utf-8')
            with self.assertRaises(ValueError):m.load(path)

    def test_cli_file_only_check_and_exclusive_receipt(self):
        with tempfile.TemporaryDirectory() as d:
            root=Path(d);b,host,_,_=self.fixture(root);raw=host.pop('_bindings_raw');bp=root/'bindings.json';bp.write_bytes(raw);hp=root/'host.json';hp.write_bytes(m.canonical(host));out=root/'receipt.json'
            args=['retention_maintenance.py','check','--bindings',str(bp),'--bindings-sha256',m.sha(raw),'--host',str(hp),'--host-sha256',m.sha(hp.read_bytes()),'--output',str(out)]
            with patch('sys.argv',args),patch.object(m,'docker',side_effect=AssertionError('Docker forbidden')):m.main()
            self.assertEqual(m.load(out)['status'],'PASS_FILE_BINDINGS_ONLY_NOT_AUTHORIZED')
            with patch('sys.argv',args),self.assertRaises(FileExistsError):m.main()

    def test_cold_new_budget_and_confirmed_previous_slot_never_repair_drift(self):
        for group in ('business-races','mixed'):
            for fault in ('none','ownership','nonempty','missing','failed-previous'):
                with self.subTest(group=group,fault=fault),tempfile.TemporaryDirectory() as d:
                    folder=Path(d);b,h,_,_=self.fixture(folder);budget=folder/'budget';budget.mkdir()
                    (budget/'ownership.json').write_bytes(m.canonical(dict(Kind=m.KIND,MaintenanceID=b['MaintenanceID'],BindingsSHA256=h['bindings_sha256'],BudgetID=b['BudgetID'])))
                    if group=='mixed':
                        for suffix in ('invocation','slot'):(budget/('business-races-'+suffix+'.json')).write_bytes(b'original fact')
                        (budget/'business-races-success.json').write_bytes(m.canonical(dict(MaintenanceID=b['MaintenanceID'],BindingsSHA256=h['bindings_sha256'],Status='PASS_EXACT_RETENTION_READBACK')))
                    if fault=='ownership':(budget/'ownership.json').write_bytes(b'{}')
                    if fault=='nonempty':(budget/'mixed-slot.json').write_bytes(b'original consumed budget')
                    if fault=='missing':(budget/'ownership.json').unlink()
                    if fault=='failed-previous':
                        if group=='mixed':(budget/'business-races-success.json').write_bytes(b'{}')
                        else:(budget/'business-races-failed.json').write_bytes(b'UNKNOWN')
                    before={p.name:p.read_bytes() for p in budget.iterdir()}
                    with patch.object(m.staging,'paths',return_value=('test-only',str(budget))):
                        if fault=='none':m.budget_state(b,h,group)
                        else:
                            with self.assertRaises((ValueError,FileNotFoundError)):m.budget_state(b,h,group)
                    self.assertEqual({p.name:p.read_bytes() for p in budget.iterdir()},before)

if __name__=='__main__':unittest.main()
