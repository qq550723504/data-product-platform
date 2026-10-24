"""Successor retention success + real refusal boundaries, no Docker/provider."""
import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import historical_descriptors as descriptors
import retained_v32 as retained


def encoded(value):return json.dumps(value).encode()
def cid(number):return '%064x' % number


class HistoricalDescriptorTests(unittest.TestCase):
    def test_raw_fact_is_unchanged_and_only_one_empty_dns_representation_is_explained(self):
        old = dict(Id=cid(1),Image='image',Config={'User':'10001'},HostConfig={'Dns':None,'Privileged':False},Mounts=[])
        current = copy.deepcopy(old);current['HostConfig']['Dns']=[]
        expected = {old['Id']:descriptors.original_digest(old)}
        original = copy.deepcopy(current)
        self.assertEqual(descriptors.comparison_digest(current,expected),expected[old['Id']])
        self.assertEqual(current,original)
        self.assertNotEqual(descriptors.original_digest(current),expected[old['Id']])
        self.assertEqual(descriptors.comparison_digest(old,expected),expected[old['Id']])
        unknown = copy.deepcopy(current);unknown['Id']=cid(2)
        self.assertEqual(descriptors.comparison_digest(unknown,expected),descriptors.original_digest(unknown))
        self.assertNotEqual(descriptors.comparison_digest(unknown,expected),expected[old['Id']])
        for fault in ('custom-dns','privileged','mount','image','user','dns-type'):
            with self.subTest(fault=fault):
                changed=copy.deepcopy(current)
                if fault=='custom-dns':changed['HostConfig']['Dns']=['1.1.1.1']
                if fault=='privileged':changed['HostConfig']['Privileged']=True
                if fault=='mount':changed['Mounts']=[{'Source':'root-key'}]
                if fault=='image':changed['Image']='foreign'
                if fault=='user':changed['Config']['User']='0'
                if fault=='dns-type':changed['HostConfig']['Dns']='[]'
                with self.assertRaises(ValueError):descriptors.comparison_digest(changed,expected)

    def test_actual_pinned_historical_membership_and_plan_byte_drift(self):
        root=Path(__file__).resolve().parents[2]
        self.assertEqual(len(descriptors.expected(root)),725)
        with tempfile.TemporaryDirectory() as tmp:
            target=Path(tmp)
            for name in descriptors.PLANS:
                p=target/'docs/acceptance/evidence/issue307-d1'/name;p.parent.mkdir(parents=True,exist_ok=True)
                p.write_bytes((root/'docs/acceptance/evidence/issue307-d1'/name).read_bytes())
            self.assertEqual(descriptors.expected(target),descriptors.expected(root))
            p.write_bytes(p.read_bytes()+b' ')
            with self.assertRaises(ValueError):descriptors.expected(target)


class RetentionTests(unittest.TestCase):
    def test_capture_cannot_pin_changed_issued_postlock_fact_as_original(self):
        from unittest.mock import Mock
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);pins={}
            for number,name in enumerate(retained.ISSUED_PINS):
                path=root/name;path.parent.mkdir(parents=True,exist_ok=True)
                raw=('original-'+str(number)).encode();path.write_bytes(raw)
                pins[name]=hashlib.sha256(raw).hexdigest()
            audit_plan=Mock(return_value={'status':'RECORDED_ONLY'})
            with patch.object(retained,'ISSUED_PINS',pins),patch.object(retained.audit,'audit_plan',audit_plan):
                retained.issued_packet(root);self.assertEqual(audit_plan.call_count,1)
                for name in pins:
                    path=root/name;original=path.read_bytes();path.write_bytes(original+b'changed')
                    audit_plan.reset_mock()
                    with self.subTest(name=name),self.assertRaises(ValueError):retained.issued_packet(root)
                    audit_plan.assert_not_called();path.write_bytes(original)

    def test_absence_never_hides_permission_io_or_link_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);target=root/'authorized-execution01'
            self.assertTrue(retained.absent(target))
            target.mkdir();self.assertFalse(retained.absent(target))
            original=Path.lstat
            for error in (PermissionError('denied'),OSError('I/O failed')):
                for rejected in (target,root):
                    def unreadable(path,*args,**kwargs):
                        if path==rejected:raise error
                        return original(path,*args,**kwargs)
                    with self.subTest(error=type(error).__name__,path=str(rejected)):
                        with patch.object(Path,'lstat',unreadable),self.assertRaises(type(error)):
                            retained.absent(target)
            # Test a reparse observation without requiring Windows symlink
            # privileges in CI; lstat is the actual platform boundary.
            from types import SimpleNamespace
            import stat
            def linked(path,*args,**kwargs):
                if path==root:return SimpleNamespace(st_mode=stat.S_IFDIR,st_file_attributes=stat.FILE_ATTRIBUTE_REPARSE_POINT)
                return original(path,*args,**kwargs)
            with patch.object(Path,'lstat',linked),self.assertRaises(ValueError):retained.absent(target)

    def fixture(self,root):
        private=root/'private';old=private/'remaining-candidate22';old.mkdir(parents=True)
        oldroot=root/'original';oldroot.mkdir();rel=Path(retained.REL)
        prior_ids=[cid(n) for n in range(1,997)]
        new_ids=[cid(n) for n in range(997,1086)]
        allocation=dict(execution=retained.EXECUTION,allocation_nonce='original',owners=new_ids[:86])
        terminal=dict(status='EXPIRED_UNAUTHORIZED_FROZEN_PREPARATION_CLOSED',execution=retained.EXECUTION,
            retry_allowed=False,authorization='NOT_RECEIVED',schedule='NOT_CREATED',native_delete_this_observation=0,
            fresh_native_IAM='NOT_OBSERVED_SOURCE_NOT_STARTED')
        def save(path,value):
            path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(encoded(value))
        save(old/'allocation.json',allocation);save(old/'preparation-terminal.json',terminal)
        for n in range(30):save(old/'group'/str(n)/'permission.json',dict(Enabled=False))
        facts={str(rel/'remaining-candidate22-preparation01/allocation.json'):allocation,
               str(rel/'candidate22-expiry01/terminal.json'):terminal}
        public={}
        for name,value in facts.items():
            for source in (root,oldroot):save(source/name,value)
            raw=encoded(value);public[Path(name).as_posix()]=dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
        owners={};actual={}
        for ident in new_ids:
            item=dict(Id=ident,Name='/original-'+ident,Image='original',Config={'User':'10001'},HostConfig={},Mounts=[],
                State=dict(Running=False,Pid=0,Status='exited',ExitCode=0,StartedAt='before',FinishedAt='closed',OOMKilled=False,Dead=False,Paused=False,Restarting=False))
            actual[ident]=item;owners[ident]=dict(config_sha256=retained.cfg_digest(item),state=copy.deepcopy(item['State']))
        resources=[];volumes={};mounts={};tar={}
        for n in range(40):
            name='budget-'+str(n);wire=new_ids[n]
            meta=dict(Name=name,Driver='local',Scope='local',Options=None,Labels={'lifecycle307.remaining':retained.EXECUTION,'lifecycle307.allocation':'original'},CreatedAt='original')
            volumes[name]=meta;mounts[name]=[wire]
            raw=b'original USED budget' if n==0 else b'original empty budget'
            tar[wire]=raw
            resources.append(dict(kind='volume',identity=name,metadata=meta,mounted_cids=[wire],wire=wire,
                sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw)))
        anchor=dict(status='C22_RETAINED_EXPIRED_FROZEN_ANCHOR',execution=retained.EXECUTION,profile='candidate22/v32',
            authority_reusable=False,source_start=0,native_delete=0,prior_closed_cids=prior_ids,public_files=public,
            private_files=retained.prior.file_set(old),owners=owners,resources=copy.deepcopy(resources[:18]),budgets=resources)
        save(root/rel/retained.ANCHOR,anchor)
        calls=[]
        def docker(*args):
            calls.append(args)
            if args[0]=='inspect':
                return encoded([actual[x] for x in args[1:]])
            if args[:2]==('volume','inspect'):return encoded([volumes[args[2]]])
            if args[0]=='ps':return ('\n'.join(mounts[args[-1].split('=',1)[1]])+'\n').encode()
            if args[0]=='cp':return tar[args[1].split(':')[0]]
            raise AssertionError('unexpected or mutating Docker call '+str(args))
        return private,oldroot,anchor,actual,volumes,mounts,tar,calls,docker,dict(closed_cids=prior_ids)

    def exercise(self,fault=None):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp)
            private,oldroot,anchor,actual,volumes,mounts,tar,calls,docker,base=self.fixture(root)
            anchor_path=root/retained.REL/retained.ANCHOR;pin=retained.sha(anchor_path)
            old=private/'remaining-candidate22';wire=anchor['budgets'][0]['wire']
            if fault=='missing-owner':actual.pop(wire)
            if fault=='state-restart':actual[wire]['State']['StartedAt']='later'
            if fault=='active':actual[wire]['State']['Running']=True
            if fault=='mount-expansion':actual[wire]['Mounts']=[dict(Source='root-key')]
            if fault=='budget-owner':volumes['budget-0']['Labels']['lifecycle307.remaining']='new-execution'
            if fault=='budget-mount-expansion':mounts['budget-0'].append(cid(99999))
            if fault=='used-budget-reset':tar[wire]=b'original empty budget'
            if fault=='budget-content-change':tar[wire]+=b'changed'
            if fault=='permission-enable':(old/'group/0/permission.json').write_bytes(b'{"Enabled":true}')
            if fault=='private-extra':(old/'unexpected').write_bytes(b'changed')
            if fault=='public-drift':(oldroot/next(iter(anchor['public_files']))).write_bytes(b'changed')
            if fault=='new-authority':(old/'authorized-execution01').mkdir()
            if fault=='anchor-drift':anchor_path.write_bytes(anchor_path.read_bytes()+b' ')
            if fault=='budget-omission':
                anchor['budgets']=anchor['budgets'][:-7];anchor_path.write_bytes(encoded(anchor));pin=retained.sha(anchor_path)
            if fault=='prior-membership':base['closed_cids']=base['closed_cids'][:-1]
            if fault=='wrapper-type':
                raw_docker=docker
                def docker(*args):return raw_docker(*args).decode()
            if fault=='docker-error':
                def docker(*args):raise OSError('unreachable')
            with patch.object(retained,'OLD_ROOT',oldroot),patch.object(retained,'ANCHOR_SHA256',pin):
                result=retained.verify(root,None,lambda x:x['State']['Running'] is False,None,docker,None,base,private,None)
            self.assertEqual(result['status'],'PASS_RETAINED_V32_EXPIRED_FROZEN_CLOSED_1085')
            self.assertEqual(result['historical_budgets_preserved'],40)
            self.assertFalse(result['authorization_reusable'])
            self.assertEqual(sum(args[0]=='cp' for args in calls),40)
            self.assertEqual(tar[wire],b'original USED budget')

    def test_complete_success_keeps_used_and_empty_budgets(self):self.exercise()

    def test_actual_failure_never_becomes_preservation_pass(self):
        for fault in ('missing-owner','state-restart','active','mount-expansion','budget-owner','budget-mount-expansion',
                      'used-budget-reset','budget-content-change','permission-enable','private-extra','public-drift',
                      'new-authority','anchor-drift','budget-omission','prior-membership','wrapper-type','docker-error'):
            with self.subTest(fault=fault),self.assertRaises((ValueError,KeyError,OSError)):
                self.exercise(fault)


if __name__=='__main__':unittest.main()
