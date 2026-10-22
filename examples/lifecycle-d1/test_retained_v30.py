"""Whole C20 retained path with explicit file/Docker doubles; no native actions."""
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
import retained_v30 as r


def dump(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_bytes((json.dumps(value,separators=(',',':'))+'\n').encode())


def budget(*,uid=10001,nonempty=False):
    stream=io.BytesIO()
    with tarfile.open(fileobj=stream,mode='w') as archive:
        member=tarfile.TarInfo('budget');member.type=tarfile.DIRTYPE;member.uid=member.gid=uid;member.mode=0o700
        archive.addfile(member)
        if nonempty:
            member=tarfile.TarInfo('budget/unexpected.json');member.uid=member.gid=uid;member.size=2
            archive.addfile(member,io.BytesIO(b'{}'))
    return stream.getvalue()


class RetainedV30Tests(unittest.TestCase):
    def composition(self,fault=None):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            root=Path(tmp)/'current';private=Path(tmp)/'private';old=private/'remaining-candidate20';evidence=root/r.REL
            old.mkdir(parents=True);clone_folder=private/'candidate20-schema-compatibility01';clone_folder.mkdir()
            (clone_folder/'original').write_bytes(b'old clone proof')
            ids={hashlib.sha256(('prior'+str(n)).encode()).hexdigest() for n in range(820)}
            states={};owners={};groups=[];resources=[];anchor_resources=[];volumes={};networks={};mountsets={};budgets=[];permissions={};copies=[]
            for n in range(86):
                cid=hashlib.sha256(('C20'+str(n)).encode()).hexdigest()
                state=dict(Running=False,Pid=0,Status='created',ExitCode=0,StartedAt='never',FinishedAt='never',OOMKilled=False,Dead=False,Paused=False,Restarting=False)
                descriptor={'old':n};states[cid]=dict(Id=cid,State=state,descriptor=descriptor,HostConfig={'NetworkMode':'none'})
                owners[cid]=dict(descriptor=copy.deepcopy(descriptor),state=copy.deepcopy(state))
            cids=list(states);labels={'lifecycle307.remaining':r.EXECUTION,'lifecycle307.allocation':'original-nonce'}
            rawbudget=budget(uid=0 if fault=='budget-uid' else 10001,nonempty=fault=='budget-nonempty')
            for n,name in enumerate(('business-races','late-hold','mixed')):
                folder=old/name
                for k in range(10):
                    path=folder/('worker-'+str(k))/'permission.json';dump(path,{'Enabled':False});permissions[path.relative_to(old).as_posix()]=r.sha(path)
                volume='budget-'+name
                helper={stage:cids[n*10+i] for i,stage in enumerate({'business-races':['P2','T1','T2','wire'],'late-hold':['H2','wire'],'mixed':['M1A','M1B','wire']}[name])}
                groups.append({'group':name,'helpers':helper,'volumes':{'budget':volume}})
                budgets.append({'group':name,'volume':volume,'sha256':hashlib.sha256(rawbudget).hexdigest(),'empty':True,'mounted_cids':[helper['wire']]})
                copy_rows=[{'ID':name+'-'+stage,'Key':key} for group,stage,key in sequence.ORDER if group==name]
                copies.append({'Group':name,'Copies':copy_rows})
                for k in range(5):
                    vname=volume if k==0 else 'volume-'+name+'-'+str(k)
                    value=dict(Name=vname,Driver='local',Scope='local',Options=None,Labels=copy.deepcopy(labels),CreatedAt='original')
                    volumes[vname]=value;mountsets[vname]=[helper['wire']]
                    resources.append({'volume':vname,'allocation':'original-nonce'})
                    anchor_resources.append({'kind':'volume','value':copy.deepcopy(value),'mounted_cids':mountsets[vname][:]})
                ident=hashlib.sha256(('network'+name).encode()).hexdigest();subnet='10.10.'+str(n)+'.0/28'
                value=dict(Id=ident,Name='net-'+name,Driver='bridge',Scope='local',Internal=True,IPAM={'Config':[{'Subnet':subnet,'Gateway':'fixed'}]},Options={},Labels=copy.deepcopy(labels),Containers={})
                networks[ident]=value;resources.append({'network':ident,'name':value['Name'],'subnet':subnet,'allocation':'original-nonce'})
                anchor_resources.append({'kind':'network','value':copy.deepcopy(value)})
            allocation=dict(execution=r.EXECUTION,allocation_nonce='original-nonce',owners=cids,groups=groups,resources=resources,build_files={})
            dump(evidence/'remaining-candidate20-preparation01/allocation.json',allocation);dump(old/'allocation.json',allocation)
            for filename in ('reader','operator','owner','readonly-check.exe'):
                (old/filename).write_bytes(b'original '+filename.encode());allocation['build_files'][filename]=r.sha(old/filename)
            dump(evidence/'remaining-candidate20-preparation01/allocation.json',allocation);dump(old/'allocation.json',allocation)
            dump(evidence/'remaining-candidate20-preparation01/owner-descriptors.json',{cid:v['descriptor'] for cid,v in owners.items()})
            dump(evidence/'remaining-candidate20-preparation01/budget-initialization.json',{'recorded':True})
            dump(evidence/'remaining-candidate20-preparation01/profiles.json',{'recorded':True})
            dump(evidence/'remaining-candidate12/candidate.json',{'Groups':copies})
            original=old/'ci-blocked-closure01/business-races/case-business/remaining.json';dump(original,{'Base':{'Enabled':True}})
            revoked=old/'business-races/case-business/remaining.json';dump(revoked,{'Base':{'Enabled':False}})
            terminal=dict(status='FAILED_CI_BLOCKED_UNFROZEN_PREPARATION_CLOSED',execution=r.EXECUTION,retry_allowed=False,failures=[],native_preparation='NOT_RUN',IAM='NOT_PROVISIONED_NO_SOURCE_START',source_started=False,source_business_commands=0,physical_provider_calls=0,native_delete=0,authorization='NOT_RECEIVED',schedule='NOT_CREATED',allocation_sha256=r.sha(old/'allocation.json'),old_retained={'closed_cids':sorted(ids)},new_budgets=budgets,revoked_profiles=[{'path':revoked.relative_to(old).as_posix(),'before':r.sha(original),'after':r.sha(revoked)}])
            dump(evidence/'candidate20-planning01/ci-blocked-closure01/terminal.json',terminal);dump(old/'preparation-terminal.json',terminal)
            pins={name:r.sha(evidence/name) for name in r.PINS}
            cloneid=hashlib.sha256(b'clone').hexdigest();clonevol=dict(Name='clone-volume',Driver='local',Scope='local',Options=None,Labels={'old':'clone'},CreatedAt='original')
            states[cloneid]=dict(Id=cloneid,State=copy.deepcopy(next(iter(states.values()))['State']),descriptor={'clone':True},HostConfig={'NetworkMode':'none'})
            volumes['clone-volume']=clonevol;mountsets['clone-volume']=[cloneid]
            clone=dict(cid=cloneid,volume='clone-volume',descriptor={'clone':True},volume_descriptor=clonevol,volume_mounted_cids=[cloneid])
            dump(evidence/r.CLONE_STATE,{'cid':cloneid,'state':copy.deepcopy(states[cloneid]['State'])})
            if fault=='bad-pinned-resource-owner':anchor_resources[0]['value']['Labels']['lifecycle307.remaining']='foreign'
            anchor=dict(status='C20_RETAINED_UNFROZEN_PREPARATION_ANCHOR',execution=r.EXECUTION,authority_reusable=False,source_provider_actions=0,native_delete=0,owners=owners,permissions=permissions,resources=anchor_resources,new_budgets=budgets,private_files=r.file_set(old),schema_private_files=r.file_set(clone_folder),schema_clone=clone)
            dump(evidence/r.ANCHOR,anchor);ash=r.sha(evidence/r.ANCHOR)
            members={name:{'sha256':digest,'bytes':(evidence/name).stat().st_size} for name,digest in pins.items()}
            index=dict(status='C20_ORIGINAL_PUBLIC_EVIDENCE_INDEX',count=len(members),files=members)
            # Members use repository paths, as the production typed audit does.
            index['files']={r.REL+'/'+name:v for name,v in members.items()}
            dump(evidence/r.PUBLIC_INDEX,index);ish=r.sha(evidence/r.PUBLIC_INDEX)
            for key,value in [('PINS',pins),('ANCHOR_SHA256',ash),('PUBLIC_INDEX_SHA256',ish),('CLONE_STATE_SHA256',r.sha(evidence/r.CLONE_STATE))]:stack.enter_context(patch.object(r,key,value))
            if fault=='private-extra':(old/'unexpected').write_bytes(b'new')
            elif fault=='private-drift':(old/'operator').write_bytes(b'changed')
            elif fault=='permission':dump(old/next(iter(permissions)),{'Enabled':True})
            elif fault=='old-authority':(old/'authorized-execution01').mkdir()
            elif fault=='resumed-native':(evidence/'remaining-candidate20-preparation01/non-delete-start.json').write_bytes(b'{}')
            elif fault=='frozen-terminal':(evidence/'remaining-candidate20').mkdir()
            elif fault=='terminal-drift':(old/'preparation-terminal.json').write_bytes(b'changed')
            elif fault=='clone-file':(clone_folder/'original').write_bytes(b'changed')
            elif fault=='clone-restarted':states[cloneid]['State']['StartedAt']='restarted-but-PID0'
            elif fault=='owner-restarted':states[cids[0]]['State']['StartedAt']='restarted-but-PID0'
            elif fault=='topology':states[cids[0]]['descriptor']={'foreign':True}
            elif fault=='active-owner':states[cids[0]]['State'].update(Running=True,Pid=99)
            elif fault=='volume-owner':volumes['budget-business-races']['Labels']['lifecycle307.remaining']='foreign'
            elif fault=='network':next(iter(networks.values()))['Internal']=False
            elif fault=='expanded-mount':mountsets['budget-business-races'].append(cloneid)
            elif fault=='clone-mount':mountsets['clone-volume'].append(cids[0])
            calls=[]
            def docker(*args):
                calls.append(args)
                if args[0]=='inspect':
                    rows=[states[cid] for cid in args[1:]]
                    if fault=='duplicate-owner' and len(rows)>1:rows[-1]=rows[0]
                    result=json.dumps(rows).encode()
                    return result.decode() if fault=='str-wrapper' else result
                if args[:2]==('volume','inspect'):return json.dumps([volumes[args[2]]]).encode()
                if args[:2]==('network','inspect'):return json.dumps([networks[args[2]]]).encode()
                if args[0]=='ps':return ('\n'.join(mountsets[args[-1].removeprefix('volume=')])+'\n').encode()
                if args[0]=='cp':return b'changed budget' if fault=='budget-bytes' else rawbudget
                raise AssertionError('mutation or unexpected Docker verb')
            result=r.verify(root,None,lambda x:not x['State']['Running'] and x['State']['Pid']==0,lambda x:x['descriptor'],docker,sequence,ids,private,None)
            self.assertEqual(len(ids),907);self.assertTrue(all(x[0] in ('inspect','volume','network','ps','cp') for x in calls))
            return result

    def test_complete_retained_success_never_adopts_old_authority(self):
        result=self.composition();self.assertEqual(result['new_budgets_preserved'],3);self.assertFalse(result['authorization_reusable'])

    def test_original_failure_controls_topology_resources_and_clone_reject(self):
        for fault in ('private-extra','private-drift','permission','old-authority','resumed-native','frozen-terminal','terminal-drift','clone-file','clone-restarted','owner-restarted','topology','active-owner','volume-owner','network','expanded-mount','clone-mount','duplicate-owner','str-wrapper','budget-bytes','bad-pinned-resource-owner'):
            with self.subTest(fault=fault),self.assertRaises((ValueError,KeyError)):self.composition(fault)

    def test_rehashed_budget_wrong_uid_or_nonempty_still_rejects(self):
        for fault in ('budget-uid','budget-nonempty'):
            with self.subTest(fault=fault),self.assertRaises(ValueError):self.composition(fault)


if __name__=='__main__':unittest.main()
