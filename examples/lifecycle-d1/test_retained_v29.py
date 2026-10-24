"""Full retained-v29 read path and rejection cases; explicit file/Docker doubles."""
from contextlib import ExitStack
import copy
import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import retained_v29 as r

def dump(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    path.write_bytes((json.dumps(value,ensure_ascii=False,separators=(',',':'))+'\n').encode())

class RetainedV29Tests(unittest.TestCase):
    def composition(self,fault=None):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            root=Path(tmp)/'current';original=Path(tmp)/'original';private=Path(tmp)/'private'
            evidence=root/r.REL;old=private/'remaining-candidate19';grant=old/'authorized-execution01'
            grant.mkdir(parents=True)
            base='delete-confirmed-v29-execution01/window01/'
            terminal=dict(execution=r.EXECUTION,status='FAILED_PR_HEAD_DRIFT_PHYSICALLY_SEALED_NO_DELETE',
                terminal=True,retry_allowed=False,old_authorization_reusable=False,new_native_delete=0,
                new_Core_commands=0,physical_provider_DELETE_attempts=0,
                fresh_IAM_disable='UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START')
            dump(evidence/(base+'terminal.json'),terminal)
            dump(grant/'terminal.json',terminal)
            facts=dict(fresh_IAM_disable=terminal['fresh_IAM_disable'],all_steps_confirmed=False,
                permissions_false=[],cases_revoked=[],recoveries_revoked=[],gateways=[],
                owners=[],budget_readbacks=[],backup_readbacks=[])
            states=[dict(Id=hashlib.sha256(str(n).encode()).hexdigest(),closed=True,descriptor={'recorded':n}) for n in range(820)]
            facts['owners']=[{'cid':x['Id']} for x in states]
            groups=[];volumes={}
            for n,name in enumerate(('business-races','late-hold','mixed')):
                folder=old/name
                for k in range(10):
                    path=folder/('worker-'+str(k))/'permission.json';dump(path,dict(Enabled=False))
                    facts['permissions_false'].append(dict(path=str(path),sha256=r.sha(path),Enabled=False))
                for k in range(2):
                    stage=str(k);case=folder/('case-'+stage)/'remaining.json';recovery=folder/('recover-'+stage)/'recovery.json'
                    dump(case,dict(Base=dict(Enabled=False),Batch=r.EXECUTION));dump(recovery,dict(Enabled=False,ValidUntil='old'))
                    facts['cases_revoked'].append(dict(group=name,stage=stage,sha256=r.sha(case)))
                    facts['recoveries_revoked'].append(dict(group=name,stage='recover-'+stage,sha256=r.sha(recovery)))
                gateway=folder/'gateway.Caddyfile';gateway.write_bytes(b'sealed recorded fixture')
                facts['gateways'].append(dict(group=name,sha256=r.sha(gateway),sealed=True))
                volume='old-volume-'+name
                groups.append(dict(group=name,volumes={'budget':volume}))
                volumes[volume]=dict(Name=volume,Driver='local',Scope='local',Options=None,
                    Labels={'lifecycle307.remaining':r.EXECUTION,'lifecycle307.allocation':'old-nonce'})
            for n in range(24):
                facts['budget_readbacks'].append(dict(volume='budget-'+str(n),sha256=hashlib.sha256(b'old budget').hexdigest(),mounted_cids=[states[n]['Id']]))
            for n in range(57):
                name='backups/'+str(n);path=private/name;path.parent.mkdir(exist_ok=True);path.write_bytes(b'preserved backup')
                facts['backup_readbacks'].append(dict(private_relative_path=name,sha256=r.sha(path),bytes=path.stat().st_size))
            allocation=dict(groups=groups,build_files={},allocation_nonce='old-nonce')
            dump(evidence/'remaining-candidate19-preparation01/allocation.json',allocation)
            dump(old/'allocation.json',allocation)
            dump(evidence/'remaining-candidate19-preparation01/owner-descriptors.json',{x['Id']:x['descriptor'] for x in states[-86:]})
            for name in ('candidate.json','bindings.json','confirmation-template.txt'):
                dump(evidence/'remaining-candidate19'/name,dict(recorded=True))
                dump(original/r.REL/'remaining-candidate19'/name,dict(recorded=True))
            members=[]
            for n in range(124):
                name='receipt-'+str(n);path=root/name;path.write_bytes(b'original recorded receipt')
                (original/name).write_bytes(path.read_bytes())
                members.append(dict(path=name,sha256=r.sha(path),bytes=path.stat().st_size))
            dump(evidence/'execution-freeze-review01/retained-v29-receipt-index.json',dict(count=124,members=members))
            references=[]
            for n in range(924):
                path=original/('reference-'+str(n));path.write_bytes(b'approved old bytes')
                references.append(dict(path=path.name,sha256=r.sha(path),bytes=path.stat().st_size))
            for name in ('user-message-verbatim.txt','authorization-envelope.txt'):(grant/name).write_bytes(b'old authority')
            auth=dict(execution=r.EXECUTION,user_retyped_template=False,approved_reference_artifacts=references,
                user_message_sha256=r.sha(grant/'user-message-verbatim.txt'),authorization_envelope_sha256=r.sha(grant/'authorization-envelope.txt'))
            dump(original/r.REL/'delete-confirmed-v29-execution01/human-authorization.json',auth)
            dump(evidence/(base+'post-seal-readback.json'),facts)
            pins={name:r.sha(evidence/name) for name in r.PINS}
            stack.enter_context(patch.object(r,'PINS',pins));stack.enter_context(patch.object(r,'OLD_ROOT',original))
            calls=[]
            def docker(*args):
                calls.append(args)
                if args[0]=='inspect':return json.dumps([next(x for x in states if x['Id']==cid) for cid in args[1:]]).encode()
                if args[:2]==('volume','inspect'):return json.dumps([volumes[args[2]]]).encode()
                if args[0]=='cp':return b'changed budget' if fault=='budget-bytes' else b'old budget'
                if args[0]=='ps':
                    n=int(args[-1].split('-')[-1]);cid=states[n]['Id']
                    return ((cid+'\n')*(2 if fault=='duplicate-mount' else 1)+('f'*64+'\n' if fault=='expanded-mount' else '')).encode()
                raise AssertionError('mutation or unexpected Docker verb')
            if fault=='active-owner':states[0]['closed']=False
            elif fault=='topology':states[-1]['descriptor']={'foreign':True}
            elif fault=='volume-owner':next(iter(volumes.values()))['Labels']['lifecycle307.remaining']='new-execution'
            elif fault=='permission':dump(Path(facts['permissions_false'][0]['path']),dict(Enabled=True))
            elif fault=='backup':(private/'backups/0').write_bytes(b'changed')
            elif fault=='old-approved':(original/'reference-0').write_bytes(b'changed')
            elif fault=='original-receipt':(original/'receipt-0').write_bytes(b'changed')
            ids={x['Id'] for x in states[:731]}
            result=r.verify(root,None,lambda x:x['closed'],lambda x:x['descriptor'],docker,ids,private,None)
            self.assertEqual(len(ids),820)
            self.assertTrue(all(x[0] in ('inspect','volume','cp','ps') for x in calls))
            return result

    def test_full_retained_read_success_never_adopts_old_authority(self):
        result=self.composition()
        self.assertEqual(result['budgets_preserved'],24)
        self.assertFalse(result['authorization_reusable'])
        self.assertEqual(result['historical_IAM'],'UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START')

    def test_changed_old_fact_owner_budget_mount_control_or_backup_rejected(self):
        for fault in ('active-owner','topology','volume-owner','permission','backup','old-approved',
                      'original-receipt','budget-bytes','duplicate-mount','expanded-mount'):
            with self.subTest(fault=fault),self.assertRaises(ValueError):self.composition(fault)

    def test_owner_membership_and_budget_contract_reject_missing_duplicate_foreign_or_expanded(self):
        good=[dict(Id='a',closed=True),dict(Id='b',closed=True)]
        r.closed_members(good,['a','b'],lambda x:x['closed'])
        for rows in (good[:1],good+[good[0]],[good[0],dict(Id='c',closed=True)]):
            with self.assertRaises(ValueError):r.closed_members(rows,['a','b'],lambda x:x['closed'])
        r.budget_mounts(['a','b'],['b','a'])
        for mounts in (['a'],['a','b','c'],['a','b','b']):
            with self.assertRaises(ValueError):r.budget_mounts(['a','b'],mounts)

if __name__=='__main__':unittest.main()
