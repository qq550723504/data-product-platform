"""Whole finite host flow with real signatures/files and doubled Docker/SDK.

No retained source, provider, protection write or DELETE is touched by this suite.
The Go SDK protocol and the shared Core/reader/checker collector are tested too.
"""
import base64
import copy
import ctypes
import datetime as dt
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat,PrivateFormat,NoEncryption
import retention_composition as c
import test_retention_maintenance as host_tests

# Offline tests consume the committed immutable fixture, never the retained
# host checkout. Keep the production hash/identity reader exercised unchanged.
SOURCE_FIXTURE=Path(__file__).resolve().parents[2]/'docs/acceptance/evidence/issue307-d1/remaining-candidate22/candidate.json'


class FakeOperations:
    def __init__(self,test,b,h,items,at,fault=None):
        self.test,self.b,self.h,self.items,self.at,self.fault=test,b,h,items,at,fault
        self.calls=[];self.enabled=[];self.count=0

    def cold(self):
        self.calls.append(('cold',))
        if self.fault=='old-budget':raise ValueError('original old budget drift')
        return {'status':'PASS_SUCCESSOR_HISTORY_AND_COLD_DEPENDENCIES'}

    def branch(self,packet):
        self.calls.append(('branch',))
        if self.fault=='HEAD':raise ValueError('exact HEAD drift')

    def inspect(self,cid):return self.items[cid]
    def start(self,cid):
        self.calls.append(('start',cid));self.items[cid]['State'].update(Running=True,Pid=99,StartedAt='2026-10-23T14:00:00Z')
        if self.fault=='start-unknown':raise OSError('effect happened; outcome unknown')

    def stop(self,cid):
        self.calls.append(('stop',cid))
        if self.fault=='reader-stop' and self.items[cid].get('test_reader'):raise OSError('reader stop remains UNKNOWN')
        self.items[cid]['State'].update(Running=False,Pid=0)
        if self.fault=='closure' and cid==self.b['Owners'][0]['CID']:raise OSError('readback unavailable')
        return True

    def read(self,group,context,destination,deadline):
        phase=context['profile']['Phase'];self.calls.append(('read',phase,group))
        if self.fault==phase+'-'+group:raise ValueError('original native '+phase+' drift')
        context['record_creation']()
        cid=c.host.sha((phase+group).encode());self.items[cid]=dict(Id=cid,Mounts=[],State=dict(Running=False,Pid=0),test_reader=True)
        if self.fault!='reader-missing-registration':context['register_owner'](cid,'d'*64)
        private=destination/'private';private.mkdir();c.save(private/'retention-readonly.json',context['profile'])
        public=destination/'public';public.mkdir()
        c.save(public/'closure.json',dict(all_closed=self.fault not in ('reader-stop','reader-missing-registration'),unknown_temporary_owner=None))
        if self.fault=='reader-stop':self.items[cid]['State'].update(Running=True,Pid=555);raise ValueError('original reader UNKNOWN close')
        if self.fault=='reader-missing-registration':raise ValueError('create outcome not known')
        result=dict(status='PASS_COMPLETE_RETENTION_MAINTENANCE_NATIVE_AGGREGATION',copies={'business-races':7,'late-hold':6,'mixed':4}[group],
            maintenance_id=self.b['MaintenanceID'],bindings_sha256=self.h['bindings_sha256'],phase=phase,retain_until=self.b['RetainUntil'],maintenance_end=self.b['NotAfter'])
        if self.fault=='phase-alias':result['phase']='after'
        return result

    def prepare_writer(self,b,h,group,source):
        self.calls.append(('budget',group))
        if self.fault=='new-budget':raise ValueError('new budget not empty or ownership drift')

    def wait_ready(self,b,h,group,deadline):
        self.calls.append(('ready',group))
        if self.fault=='ready-missing':raise ValueError('never observed false')
        owner=next(o for o in b['Owners'] if o['Group']==group)
        ready=dict(Kind=c.host.KIND,MaintenanceID=b['MaintenanceID'],BindingsSHA256=h['bindings_sha256'],Group=group,Owner=owner['CID'],DescriptorSHA256=owner['DescriptorSHA256'],ConfigSHA256=owner['ConfigSHA256'],At='2026-10-23T14:00:00Z')
        if self.fault=='ready-foreign':ready['Owner']='f'*64
        if self.fault=='ready-replayed':ready['At']='2026-10-23T13:59:59Z'
        c.save(Path(h['owners'][group]['output'])/'wait-ready.json',ready)

    def proof(self,b,h,group,key,at):
        self.calls.append(('proof',group))
        result=c.host.attest(b,h,group,key,at,self.inspect,lambda:list(self.items.values()))
        if self.fault=='window-after-proof':self.test.current=c.host.proposal.protection.instant(b['NotAfter'])
        return result

    def wait(self,cid,output):
        owner=next(o for o in self.b['Owners'] if o['CID']==cid);group=owner['Group']
        self.calls.append(('wait',group));self.enabled.append(group)
        self.test.assertEqual(c.host.load(Path(self.h['owners'][group]['private'])/'permission.json')['Enabled'],True)
        if self.fault in ('first-UNKNOWN','durability'):raise ValueError('original attempt UNKNOWN; no retry')
        self.items[cid]['State'].update(Running=False,Pid=0,ExitCode=0)
        c.save(Path(self.h['owners'][group]['output'])/'terminal.json',dict(MaintenanceID=self.b['MaintenanceID'],BindingsSHA256=self.h['bindings_sha256'],Status='PASS_EXACT_RETENTION_READBACK',readback_confirmed=self.fault!='unconfirmed'))


class CompositionTests(unittest.TestCase):
    volume=host_tests.HostTests.volume
    bind=host_tests.HostTests.bind

    def setUp(self):
        self.metadata={}
        self.enterContext(patch.object(c.history,'SOURCE_PACKET',SOURCE_FIXTURE))
        self.enterContext(patch.object(c.host.staging,'validate_directory',return_value={}))
        def docker(*args):
            self.assertEqual(args[:2],('volume','inspect'))
            return json.dumps([self.metadata[args[2]]]).encode()
        self.enterContext(patch.object(c.host,'docker',side_effect=docker))

    def fixture(self,folder,fault=None):
        # Freeze one canonical root before deriving paths or signing facts.
        # This also rejects links/traversal before fixture creation; resolving
        # each future reader file would be wrong because it does not exist yet.
        folder=c.indexes.unlinked_absolute(folder)
        b,h,items,self.current=host_tests.HostTests.fixture(self,folder)
        private_base=folder/'controls';private_base.mkdir();control=private_base/b['MaintenanceID'];control.mkdir()
        self.enterContext(patch.object(c,'PRIVATE_BASE',private_base))
        key=Ed25519PrivateKey.generate();public=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw)
        keyfile=control/'host.private';keyfile.write_bytes(key.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption()))
        b['HostPublicSHA256']=c.host.sha(public);self.bind(b,h)
        for o in b['Owners']:
            item=h['owners'][o['Group']];out=Path(item['output']);(out/'wait-ready.json').unlink()
            items[o['CID']]['State'].update(Running=False,Pid=0,ExitCode=0)
            items[o['Native']]['State'].update(Running=False,Pid=0)
            (Path(item['private'])/'permission.json').write_bytes(c.host.canonical(dict(Enabled=False,BindingsSHA256=h['bindings_sha256'])))
        sources=[]
        def ref(name,raw):
            path=control/name;path.write_bytes(raw)
            return dict(path=str(path),sha256=c.host.sha(raw),bytes=len(raw))
        for group,source in c.known_sources().items():
            row=dict(Group=group,source=source)
            for role in ('Core','Native'):
                if source[role] not in items:items[source[role]]={'State':{'Running':False,'Pid':0,'ExitCode':0},'NetworkSettings':{'Ports':{}},'HostConfig':{'RestartPolicy':{'Name':'no'}}}
            row['config']=ref(group+'-config',c.host.canonical(dict(Group=group,Endpoint='127.0.0.1:9000',RootAccess='fixture-only',RootSecret='not-a-real-secret')))
            for name in ('core_checkpoint','frozen_inventory','present_inventory','before_inventory','after_inventory'):row[name]=ref(group+'-'+name,b'[]')
            exp=control/(group+'-baseline');exp.mkdir();(exp/'original.json').write_bytes(b'{}')
            row.update(expectation=str(exp),expectation_files={'original.json':dict(sha256=c.host.sha(b'{}'),bytes=2)})
            sources.append(row)
        items[c.history.BUSINESS_REDIS]={'State':{'Running':False,'Pid':0,'ExitCode':0},'NetworkSettings':{'Ports':{}},'HostConfig':{'RestartPolicy':{'Name':'no'}}}
        for cid,item in items.items():item.setdefault('Id',cid);item.setdefault('Mounts',[])
        readers=[]
        for phase in ('before','after'):
            for group in c.GROUPS:
                audit=str(c.uuid.uuid5(c.uuid.UUID(b['MaintenanceID']),'retention-readonly/'+phase+'/'+group));destination=control/'run'/phase/group
                readers.append(dict(phase=phase,group=group,audit_id=audit,name='dpp307-six-read-'+audit,native=next(s['source']['Native'] for s in sources if s['Group']==group),
                    intent=str(destination/'reader-create-intent.json'),registration=str(destination/'reader-created.json'),closure=str(destination/'public/closure.json'),profile=str(destination/'private/retention-readonly.json')))
        anchor=dict(schema='EXACT_TWO_RETENTION_CONTAINMENT_V2',MaintenanceID=b['MaintenanceID'],bindings_sha256=h['bindings_sha256'],public_key=base64.b64encode(public).decode(),readers=readers,
            controls=[str(Path(h['owners'][g]['private'])/'permission.json') for g in c.host.proposal.TARGETS],
            close_cids=list(dict.fromkeys([o['CID'] for o in b['Owners']]+[s['source'][role] for s in sources for role in ('Core','Native')]+[c.history.BUSINESS_REDIS])))
        packet=dict(schema=c.SCHEMA,review_head='a'*40,rule_id='fixed-rule',protection_sha256='d'*64,
            bindings=ref('bindings',h['_bindings_raw']),host=ref('host',c.host.canonical({k:v for k,v in h.items() if k!='_bindings_raw'})),public_key=ref('host.public',public),
            private_key=str(keyfile),reader=ref('reader',b'fake-reader'),checker=ref('checker',b'fake-checker'),control=str(control),sources=sources,anchor=ref('anchor',c.host.canonical(anchor)))
        packet_sha=c.host.sha(c.host.canonical(packet));ops=FakeOperations(self,b,h,items,self.current,fault)
        entry=c.Composition(packet,packet_sha,ops,lambda:self.current)
        human=control/'actual-human.txt';human.write_bytes(b'new human reference\n')
        common=dict(MaintenanceID=b['MaintenanceID'],BindingsSHA256=h['bindings_sha256'],HumanMessageSHA256=c.host.sha(human.read_bytes()),NotBefore=b['NotBefore'],NotAfter=b['NotAfter'],ConfirmedAt='2026-10-23T13:58:00Z',OldAuthorizationReused=False)
        read=dict(common,Kind=c.READ_KIND,ManifestSHA256=packet_sha,SourceStart=True,Phases=['before','after'],MaxProtectionWrites=0,MaxProviderDeletes=0)
        write=dict(common,Kind=c.host.KIND,RetainUntil=b['RetainUntil'],MaxPerTarget=1,MaxTotal=2)
        def signed(name,body):
            raw=c.host.canonical(body);path=control/name;c.save(path,dict(Body=base64.b64encode(raw).decode(),Signature=base64.b64encode(key.sign(raw)).decode()));return path
        grants=(signed('read-authority.json',read),human,signed('write-authority.json',write),human)
        return entry,ops,grants,read,write,key

    def test_real_path_spellings_freeze_one_canonical_reader_root(self):
        modes=('canonical','upper-case','short-name') if os.name=='nt' else ('canonical',)
        for mode in modes:
            with self.subTest(mode=mode),tempfile.TemporaryDirectory(prefix='dpp307 reader binding ') as d:
                canonical=Path(d).resolve(strict=True);folder=canonical
                if mode=='upper-case':folder=Path(str(canonical).upper())
                elif mode=='short-name':
                    query=ctypes.windll.kernel32.GetShortPathNameW
                    query.argtypes=[ctypes.c_wchar_p,ctypes.c_wchar_p,ctypes.c_uint];query.restype=ctypes.c_uint
                    buffer=ctypes.create_unicode_buffer(32768)
                    length=query(str(canonical),buffer,len(buffer))
                    self.assertTrue(0<length<len(buffer));folder=Path(buffer.value)
                    if str(folder)==str(canonical):self.skipTest('filesystem did not allocate an 8.3 alias')
                self.assertTrue(folder.samefile(canonical))
                if mode!='canonical':self.assertNotEqual(str(folder),str(canonical))
                entry,ops,args,*_=self.fixture(folder)
                self.assertEqual(str(entry.control),str(canonical/'controls'/entry.b['MaintenanceID']))
                self.assertEqual(entry.packet['control'],str(entry.control))
                self.assertFalse((entry.control/'run').exists())
                for reader in entry.anchor['readers']:
                    destination=entry.control/'run'/reader['phase']/reader['group']
                    for field,relative in [('intent','reader-create-intent.json'),('registration','reader-created.json'),('closure','public/closure.json'),('profile','private/retention-readonly.json')]:
                        self.assertEqual(reader[field],str(destination/relative))
                self.assertFalse(ops.calls)
                entry.run(*args)
                self.assertEqual(ops.enabled,['business-races','mixed'])
                self.assertEqual(c.host.load(entry.control/'run/terminal.json')['status'],'PASS_MAINTENANCE_BEFORE_WRITE_AFTER_CLOSED')

    def test_rehashed_reader_path_or_native_drift_still_rejects_before_operations(self):
        for fault in ('foreign-root','other-group','other-phase','traversal','raw-case','other-native'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as d:
                entry,ops,*_=self.fixture(Path(d));packet=copy.deepcopy(entry.packet);anchor=copy.deepcopy(entry.anchor)
                reader=anchor['readers'][0];destination=entry.control/'run'/reader['phase']/reader['group']
                if fault=='foreign-root':reader['intent']=str(entry.control.parent/'other-maintenance'/'reader-create-intent.json')
                elif fault=='other-group':reader['registration']=str(entry.control/'run'/'before'/'late-hold'/'reader-created.json')
                elif fault=='other-phase':reader['closure']=str(entry.control/'run'/'after'/reader['group']/'public/closure.json')
                elif fault=='traversal':reader['profile']=str(destination/'..'/reader['group']/'private/retention-readonly.json')
                elif fault=='raw-case':reader['intent']=str(Path(str(entry.control).upper())/'run'/reader['phase']/reader['group']/'reader-create-intent.json')
                else:reader['native']=entry.sources[1]['source']['Native']
                raw=c.host.canonical(anchor);path=entry.control/'rehashed-bad-anchor.json';path.write_bytes(raw)
                packet['anchor']=dict(path=str(path),sha256=c.host.sha(raw),bytes=len(raw))
                # Both hashes are internally consistent. The real semantic
                # path/native gate, rather than just a stale hash, must reject.
                c.checked_anchor(raw,c.host.sha(raw))
                with self.assertRaisesRegex(ValueError,'COMPOSITION_ANCHOR_READER_PATH_BINDING'):
                    c.Composition(packet,c.host.sha(c.host.canonical(packet)),ops,lambda:self.current)
                self.assertFalse(ops.calls);self.assertFalse((entry.control/'run').exists())

    def test_fixture_traversal_and_real_link_reject_before_any_fixture_write(self):
        for fault in ('traversal','linked-root'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as d:
                root=Path(d).resolve();target=root/'target';target.mkdir();alias=root/'alias'
                if fault=='linked-root':
                    try:alias.symlink_to(target,target_is_directory=True)
                    except OSError:
                        if os.name!='nt':raise
                        result=subprocess.run(['powershell','-NoProfile','-NonInteractive','-Command',
                            '& { param($link,$target) New-Item -ItemType Junction -Path $link -Target $target -ErrorAction Stop | Out-Null }',str(alias),str(target)],capture_output=True,timeout=30)
                        self.assertEqual(result.returncode,0,result.stderr.decode('utf-8','replace'))
                    self.assertTrue(alias.is_symlink() or alias.is_junction())
                else:alias=target/'..'
                try:
                    with patch.object(host_tests.HostTests,'fixture',side_effect=AssertionError('untrusted root reached fixture IO')) as inherited:
                        with self.assertRaisesRegex(ValueError,'linked evidence path|traversing absolute evidence path'):self.fixture(alias)
                        inherited.assert_not_called()
                    self.assertEqual(list(target.iterdir()),[])
                finally:
                    if fault=='linked-root':
                        if alias.is_junction():alias.rmdir()
                        elif alias.is_symlink():alias.unlink()

    def test_complete_before_seventeen_two_ordered_workers_after_seventeen_and_closure(self):
        with tempfile.TemporaryDirectory() as d:
            entry,ops,args,*_=self.fixture(Path(d));entry.run(*args)
            self.assertEqual(ops.enabled,['business-races','mixed'])
            self.assertEqual([row for row in ops.calls if row[0]=='read'],[('read',phase,g) for phase in ('before','after') for g in c.GROUPS])
            terminal=c.host.load(entry.control/'run/terminal.json')
            self.assertEqual(terminal['status'],'PASS_MAINTENANCE_BEFORE_WRITE_AFTER_CLOSED');self.assertEqual(terminal['writers_completed'],2)
            self.assertFalse(any(i['State']['Running'] for i in ops.items.values()))
            with self.assertRaises(FileExistsError):entry.run(*args)

    def test_failures_never_enable_next_worker_and_preserve_partial_receipts(self):
        faults=('HEAD','old-budget','before-business-races','before-late-hold','before-mixed','phase-alias','new-budget','start-unknown',
            'ready-missing','ready-foreign','ready-replayed','window-after-proof','first-UNKNOWN','durability','unconfirmed','after-business-races','after-mixed','closure')
        for fault in faults:
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as d:
                entry,ops,args,*_=self.fixture(Path(d),fault)
                with self.assertRaises((ValueError,OSError)):entry.run(*args)
                terminal=c.host.load(entry.control/'run/terminal.json')
                self.assertEqual(terminal['status'],'FAILED_MAINTENANCE_PRESERVE_ALL_EVIDENCE')
                if not fault.startswith('after-') and fault!='closure':self.assertNotIn('mixed',ops.enabled)
                else:self.assertEqual(ops.enabled,['business-races','mixed'])
                if fault in ('first-UNKNOWN','durability','unconfirmed'):self.assertEqual(ops.enabled,['business-races'])
                if fault in ('ready-missing','ready-foreign','ready-replayed','window-after-proof'):self.assertFalse(ops.enabled)
                self.assertFalse(any(i['State']['Running'] for i in ops.items.values()))
                self.assertTrue(all(c.host.load(Path(p))['Enabled'] is False for p in entry.anchor['controls']))

    def test_signed_read_write_authority_rejects_mismatched_dates_identity_reuse_scope(self):
        cases=[('read',key,val) for key,val in [('MaxProviderDeletes',1),('MaxProtectionWrites',1),('SourceStart',False),('Phases',['after']),('ManifestSHA256','0'*64),('OldAuthorizationReused',True)]]
        cases += [('write',key,val) for key,val in [('RetainUntil','2026-10-24T12:41:00Z'),('MaxTotal',3),('MaxPerTarget',True),('Kind','RETENTION_EXTENSION_EXACT_TWO_V1'),('OldAuthorizationReused',True),('BindingsSHA256','0'*64),('HumanMessageSHA256','0'*64)]]
        for phase,field,value in cases:
            with self.subTest(phase=phase,field=field),tempfile.TemporaryDirectory() as d:
                entry,ops,args,read,write,key=self.fixture(Path(d));body=copy.deepcopy(read if phase=='read' else write);body[field]=value
                raw=c.host.canonical(body);bad=entry.control/'bad-authority.json';c.save(bad,dict(Body=base64.b64encode(raw).decode(),Signature=base64.b64encode(key.sign(raw)).decode()))
                arguments=list(args);arguments[0 if phase=='read' else 2]=bad
                with self.assertRaises(ValueError):entry.run(*arguments)
                self.assertFalse(ops.calls);self.assertFalse((entry.control/'run').exists())

    def test_sha_pinned_anchor_closes_all_after_packet_drift_archive_failure_and_inspect_failure(self):
        with tempfile.TemporaryDirectory() as d:
            entry,ops,args,*_=self.fixture(Path(d),'closure')
            Path(entry.packet['bindings']['path']).write_bytes(b'broken; seal must remain possible')
            with self.assertRaises(ValueError):entry.revalidate()
            anchor=c.checked_anchor(Path(entry.packet['anchor']['path']).read_bytes(),entry.packet['anchor']['sha256'])
            missing=entry.control/'archive-not-created'
            result=c.contain(anchor,ops,missing)
            self.assertEqual(result['status'],'UNCONFIRMED_MAINTENANCE_PHYSICAL_CLOSURE')
            self.assertEqual({row[1] for row in ops.calls if row[0]=='stop'},set(anchor['close_cids']))
            self.assertFalse(any(row[0]=='start' for row in ops.calls))
            for p in anchor['controls']:self.assertFalse(c.host.load(Path(p))['Enabled'])
            with self.assertRaises(ValueError):c.checked_anchor(Path(entry.packet['anchor']['path']).read_bytes(),'0'*64)

    def test_revalidation_rejects_reader_config_baseline_and_private_key_drift(self):
        for target in ('reader','config','baseline','private-key'):
            with self.subTest(target=target),tempfile.TemporaryDirectory() as d:
                entry,ops,args,*_=self.fixture(Path(d))
                path={'reader':Path(entry.packet['reader']['path']),'config':Path(entry.sources[0]['config']['path']),
                    'baseline':Path(entry.sources[0]['expectation'])/'original.json','private-key':entry.key}[target]
                path.write_bytes(b'0'*32 if target=='private-key' else b'drift')
                with self.assertRaises(ValueError):entry.revalidate()
                self.assertFalse(ops.calls)

    def test_dynamic_reader_stop_unknown_and_missing_cid_never_become_seal_pass(self):
        for fault in ('reader-stop','reader-missing-registration'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as d:
                entry,ops,args,*_=self.fixture(Path(d),fault)
                with self.assertRaises(ValueError):entry.run(*args)
                terminal=c.host.load(entry.control/'run/terminal.json')
                self.assertEqual(terminal['writers_completed'],0);self.assertFalse(ops.enabled)
                self.assertEqual(terminal['closure']['status'],'UNCONFIRMED_MAINTENANCE_PHYSICAL_CLOSURE')
                Path(entry.packet['bindings']['path']).write_bytes(b'broken packet')
                anchor=c.checked_anchor(Path(entry.packet['anchor']['path']).read_bytes(),entry.packet['anchor']['sha256'])
                seal=c.contain(anchor,ops,entry.control)
                self.assertEqual(seal['status'],'UNCONFIRMED_MAINTENANCE_PHYSICAL_CLOSURE')
                fixed={row[1] for row in ops.calls if row[0]=='stop'}
                self.assertTrue(set(anchor['close_cids']).issubset(fixed))
                if fault=='reader-stop':self.assertIn(c.host.sha(b'beforebusiness-races'),fixed)
                else:self.assertNotIn(c.host.sha(b'beforebusiness-races'),fixed)

    def test_valid_reader_registration_still_closes_when_intent_or_main_packet_is_lost(self):
        for fault in ('intent-missing','intent-corrupt'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as d:
                entry,ops,args,*_=self.fixture(Path(d),'reader-stop')
                with self.assertRaises(ValueError):entry.run(*args)
                reader=entry.anchor['readers'][0];cid=c.host.sha(b'beforebusiness-races')
                if fault=='intent-missing':Path(reader['intent']).unlink()
                else:Path(reader['intent']).write_bytes(b'corrupt original intent')
                Path(entry.packet['bindings']['path']).write_bytes(b'corrupt runtime package')
                ops.fault=None;ops.calls=[]
                result=c.contain(entry.anchor,ops,entry.control)
                self.assertIn(('stop',cid),ops.calls);self.assertFalse(ops.items[cid]['State']['Running'])
                self.assertEqual(result['status'],'UNCONFIRMED_MAINTENANCE_PHYSICAL_CLOSURE')
                self.assertTrue(set(entry.anchor['close_cids']).issubset(row[1] for row in ops.calls if row[0]=='stop'))

    def test_absent_unstarted_reader_facts_are_absent_not_unconfirmed(self):
        with tempfile.TemporaryDirectory() as d:
            entry,ops,args,*_=self.fixture(Path(d))
            self.assertFalse(c.present(entry.control/'not-created'))
            result=c.contain(entry.anchor,ops,entry.control)
            self.assertEqual(result['status'],'PASS_MAINTENANCE_PHYSICAL_CLOSURE')
            self.assertFalse(any(row[0]=='start' for row in ops.calls))

    def test_portable_source_fixture_keeps_missing_and_drifted_live_gate_closed(self):
        original=SOURCE_FIXTURE.read_bytes()
        self.assertEqual(c.host.sha(original),c.history.SOURCE_PACKET_SHA256)
        with tempfile.TemporaryDirectory() as d:
            path=Path(d)/'immutable-source.json';path.write_bytes(original)
            with patch.object(c.history,'SOURCE_PACKET',path):
                self.assertEqual(list(c.known_sources()),list(c.GROUPS))
                path.write_bytes(original+b'\n')
                with self.assertRaises(ValueError):c.known_sources()
                path.unlink()
                with self.assertRaises(FileNotFoundError):c.known_sources()


if __name__=='__main__':unittest.main()
