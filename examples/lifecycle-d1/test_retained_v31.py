"""Whole expired-C21 preservation composition, with explicit file/Docker doubles."""
from contextlib import ExitStack
import copy
import hashlib
import io
import json
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest.mock import patch
import remaining_sequence as sequence
import retained_v31 as r


def write(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_bytes((json.dumps(value,separators=(',',':'))+'\n').encode())


def budget(*,uid=10001,nonempty=False):
    out=io.BytesIO()
    with tarfile.open(fileobj=out,mode='w') as archive:
        entry=tarfile.TarInfo('budget');entry.type=tarfile.DIRTYPE;entry.uid=entry.gid=uid;entry.mode=0o700
        archive.addfile(entry)
        if nonempty:
            entry=tarfile.TarInfo('budget/extra.json');entry.uid=entry.gid=uid;entry.size=2
            archive.addfile(entry,io.BytesIO(b'{}'))
    return out.getvalue()


class RetainedV31Tests(unittest.TestCase):
    def composition(self,fault=None):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            root=Path(tmp)/'current';original=Path(tmp)/'original';private=Path(tmp)/'private'
            evidence=root/r.REL;old=private/'remaining-candidate21';old.mkdir(parents=True)
            cid=lambda text:hashlib.sha256(text.encode()).hexdigest()
            ids={cid('prior'+str(n)) for n in range(907)}
            states={};allocated={};technical={}
            for n in range(89):
                ident=cid('C21'+str(n));state=dict(Running=False,Pid=0,Status='exited',ExitCode=0,StartedAt='old-start',FinishedAt='old-stop',OOMKilled=False,Dead=False,Paused=False,Restarting=False)
                descriptor=dict(original=n);states[ident]=dict(Id=ident,State=copy.deepcopy(state),descriptor=descriptor)
                (allocated if n<86 else technical)[ident]=dict(descriptor=copy.deepcopy(descriptor),state=state)
            cids=list(allocated);groups=[];candidate=dict(Execution=r.EXECUTION,Groups=[])
            resources=[];snapshots=[];volumes={};networks={};mountsets={};budgets=[]
            data=budget(uid=0 if fault=='budget-uid' else 10001,nonempty=fault=='budget-nonempty')
            labels={'lifecycle307.remaining':r.EXECUTION,'lifecycle307.allocation':'old-nonce'}
            for n,name in enumerate(('business-races','late-hold','mixed')):
                wire=cids[n];volume='budget-'+name
                groups.append(dict(group=name,helpers={'wire':wire},volumes={'budget':volume},budget_expected_mounted_cids=[wire]))
                copies=[dict(ID=name+'-'+stage,Key=key) for group,stage,key in sequence.ORDER if group==name]
                helper_map={'wire':wire,**{stage:wire for group,stage,_ in sequence.ORDER if group==name}}
                candidate['Groups'].append(dict(Group=name,Helpers=helper_map,Copies=copies))
                for k in range(10):write(old/name/('worker-'+str(k))/'permission.json',dict(Enabled=False,Execution=r.EXECUTION))
                for k in range(5):
                    name_v=volume if k==0 else 'owned-'+name+'-'+str(k)
                    value=dict(Name=name_v,Driver='local',Scope='local',Options=None,Labels=copy.deepcopy(labels),CreatedAt='original')
                    volumes[name_v]=value;mountsets[name_v]=[wire];resources.append(dict(volume=name_v,allocation='old-nonce'))
                    snapshots.append(dict(kind='volume',value=copy.deepcopy(value),mounted_cids=[wire]))
                ident=cid('network-'+name)
                value=dict(Id=ident,Name='restore-'+name,Driver='bridge',Scope='local',Internal=True,IPAM={'Config':[{'Subnet':'10.1.'+str(n)+'.0/28'}]},Options={},Labels=copy.deepcopy(labels),Containers={})
                networks[ident]=value;resources.append(dict(network=ident,allocation='old-nonce'))
                snapshots.append(dict(kind='network',value=copy.deepcopy(value)))
                budgets.append(dict(group=name,volume=volume,mounted_cids=[wire],sha256=hashlib.sha256(data).hexdigest(),empty=True))
            allocation=dict(execution=r.EXECUTION,profile='candidate21/v31',allocation_nonce='old-nonce',
                retained={'closed_cids':sorted(ids)},groups=groups,owners=cids,resources=resources)
            terminal=dict(status='EXPIRED_UNAUTHORIZED_FROZEN_PREPARATION_CLOSED',execution=r.EXECUTION,retry_allowed=False,
                          authorization='NOT_RECEIVED',schedule='NOT_CREATED',native_delete_this_observation=0,
                          fresh_native_IAM='NOT_OBSERVED_SOURCE_NOT_STARTED')
            public_terminal=evidence/'candidate21-expiry01/terminal.json'
            write(public_terminal,terminal);write(old/'preparation-terminal.json',terminal)
            write(old/'allocation.json',allocation)
            write(evidence/'remaining-candidate21-preparation01/allocation.json',allocation)
            write(evidence/'remaining-candidate21-preparation01/owner-descriptors.json',{k:v['descriptor'] for k,v in allocated.items()})
            write(evidence/'remaining-candidate21/candidate.json',candidate)
            for name in ('reader','operator','owner','readonly-check.exe'):(old/name).write_bytes(b'original build '+name.encode())
            files={}
            for path in sorted(evidence.rglob('*')):
                if not path.is_file():continue
                relative=path.relative_to(root);other=original/relative;other.parent.mkdir(parents=True,exist_ok=True);other.write_bytes(path.read_bytes())
                files[relative.as_posix()]=dict(sha256=r.sha(path),bytes=path.stat().st_size)
            write(evidence/r.INDEX,dict(status='C21_ORIGINAL_PUBLIC_EVIDENCE_INDEX',count=len(files),files=files))
            known={**{k:{'preserved':True} for k in ids},**{k:v['State'] for k,v in states.items()}}
            anchor=dict(status='C21_RETAINED_EXPIRED_FROZEN_ANCHOR',execution=r.EXECUTION,profile='candidate21/v31',authority_reusable=False,
                source_start=0,native_delete=0,private_files=r.prior.file_set(old),owners=allocated,technical_owners=technical,
                known_states=known,resources=snapshots,budgets=budgets)
            write(evidence/r.ANCHOR,anchor)
            for name,value in [('OLD_ROOT',original),('ANCHOR_SHA256',r.sha(evidence/r.ANCHOR)),('INDEX_SHA256',r.sha(evidence/r.INDEX)),('TERMINAL_SHA256',r.sha(public_terminal))]:
                stack.enter_context(patch.object(r,name,value))
            if fault=='private-extra':(old/'extra').write_bytes(b'expansion')
            if fault=='old-binary':(old/'reader').write_bytes(b'changed original binary')
            if fault=='terminal':
                changed=copy.deepcopy(terminal);changed['retry_allowed']=True;write(old/'preparation-terminal.json',changed)
            if fault=='authority':(old/'authorized-execution01').mkdir()
            if fault=='permission':write(old/'business-races/worker-0/permission.json',dict(Enabled=True,Execution=r.EXECUTION))
            if fault=='public-byte':public_terminal.write_bytes(public_terminal.read_bytes()+b' ')
            if fault=='original-byte':(original/public_terminal.relative_to(root)).write_bytes(b'changed original')
            if fault=='owner-restart':states[cids[0]]['State']['StartedAt']='later'
            if fault=='owner-topology':states[cids[0]]['descriptor']['extra']='mount'
            if fault=='active':states[cids[0]]['State'].update(Running=True,Pid=17)
            if fault=='technical-restart':states[next(iter(technical))]['State']['StartedAt']='later'
            if fault=='budget-owner':volumes[groups[0]['volumes']['budget']]['Labels']['lifecycle307.remaining']='foreign'
            if fault=='mount-expansion':mountsets[groups[0]['volumes']['budget']].append(cid('extra'))
            if fault=='network-external':networks[next(iter(networks))]['Internal']=False
            if fault=='prior-membership':ids.pop();ids.add(cid('foreign-prior'))
            calls=[]
            def docker(*args):
                calls.append(args)
                if args[0]=='inspect':
                    rows=[states[k] for k in args[1:]]
                    if fault=='duplicate-observation' and len(rows)>1:rows=[rows[0],*rows[:-1]]
                    return json.dumps(rows).encode()
                if args[:2]==('volume','inspect'):return json.dumps([volumes[args[2]]]).encode()
                if args[:2]==('network','inspect'):return json.dumps([networks[args[2]]]).encode()
                if args[0]=='ps':return ('\n'.join(mountsets[args[-1].removeprefix('volume=')])+'\n').encode()
                if args[0]=='cp':return 'wrong type' if fault=='bytes-contract' else data
                raise AssertionError('non-read action: '+str(args))
            result=r.verify(root,None,lambda x:x['State']['Running'] is False and x['State']['Pid']==0,
                lambda x:x['descriptor'],docker,sequence,ids,private,lambda x:Path(x))
            self.assertEqual(result['status'],'PASS_RETAINED_V31_EXPIRED_FROZEN_CLOSED_996')
            self.assertEqual(len(ids),996);self.assertEqual(result['historical_budgets_preserved'],30)
            self.assertEqual(result['backup_files_preserved'],66)
            self.assertTrue(all(x[0] in ('inspect','volume','network','ps','cp') for x in calls))
            return result

    def test_complete_preservation_success_uses_host_reads_only(self):
        self.composition()

    def test_byte_authority_terminal_topology_resource_and_budget_faults_fail_closed(self):
        messages={
            'private-extra':'private file bytes/membership', 'old-binary':'private file bytes/membership',
            'terminal':'terminal bytes', 'authority':'authorized, scheduled or resumed',
            'permission':'private file bytes/membership', 'public-byte':'pinned file SHA256 differs',
            'original-byte':'original C21 public fact', 'owner-restart':'restarted or changed topology',
            'owner-topology':'restarted or changed topology', 'active':'restarted or changed topology',
            'technical-restart':'restarted or changed topology', 'budget-owner':'volume scope/owner',
            'mount-expansion':'volume mount graph', 'network-external':'restore network drift',
            'prior-membership':'prior retained owner union', 'duplicate-observation':'observation membership',
            'budget-uid':'filesystem owner differs', 'budget-nonempty':'unexpected file', 'bytes-contract':'wrapper must return bytes'}
        for fault,message in messages.items():
            with self.subTest(fault=fault),self.assertRaisesRegex(ValueError,message):self.composition(fault)


if __name__=='__main__':unittest.main()
