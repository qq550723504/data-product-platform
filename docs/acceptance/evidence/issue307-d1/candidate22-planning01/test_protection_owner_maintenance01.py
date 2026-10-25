"""Maintenance composition doubles; never make a live GitHub request."""
from contextlib import ExitStack
import copy
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
from test_execution_freeze import snapshot,HEAD
spec=importlib.util.spec_from_file_location('owner_maintenance_under_test',Path(__file__).with_name('protection-owner-maintenance01.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)


def save(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    m.f.save(path,value)


class OwnerMaintenanceTests(unittest.TestCase):
    def test_pre_and_post_checks_allow_only_the_explicit_lock_bit(self):
        before=snapshot(True);binding={'branch_lock':{'after':copy.deepcopy(before)}}
        with patch.object(m,'HEAD',HEAD),patch.object(m,'RULE','RULE_EXACT'):
            self.assertEqual(m.before_gate(before,binding),{'branchProtectionRuleId':'RULE_EXACT','lockBranch':False})
            after=copy.deepcopy(before);after['rule']['lockBranch']=False;after['protection']['lock_branch']['enabled']=False
            m.after_gate(before,after)
            for fault in ('head','admin','fork-sync','force','delete','review','contexts','rule'):
                bad=copy.deepcopy(after)
                if fault=='head':bad['head']='b'*40
                elif fault=='admin':bad['protection']['enforce_admins']['enabled']=False
                elif fault=='fork-sync':bad['rule']['lockAllowsFetchAndMerge']=True
                elif fault=='force':bad['rule']['allowsForcePushes']=True
                elif fault=='delete':bad['protection']['allow_deletions']['enabled']=True
                elif fault=='review':bad['protection']['required_pull_request_reviews']['required_approving_review_count']=0
                elif fault=='contexts':bad['protection']['required_status_checks']['contexts']=[]
                else:bad['rule']['id']='foreign'
                with self.subTest(fault=fault),self.assertRaises(ValueError):m.after_gate(before,bad)
            bad=copy.deepcopy(before);bad['protection']['required_signatures']['enabled']=False
            with self.assertRaisesRegex(ValueError,'full approved native protection'):m.before_gate(bad,binding)

    def composition(self,mode):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            root=Path(tmp);base=root/'authorized-maintenance01';private=root/'private';old=private/'remaining-candidate21'
            before=snapshot(True);after=copy.deepcopy(before);after['rule']['lockBranch']=False;after['protection']['lock_branch']['enabled']=False
            save(root/'protection-owner-maintenance-proposal01.json',{'input':{'branchProtectionRuleId':'RULE_EXACT','lockBranch':False}})
            save(base/'human-authority.json',dict(status='DIRECT_HUMAN_MAINTENANCE_AUTHORITY_RECEIVED',
                scope='CONDITIONAL_EXACT_LOCKBRANCH_FALSE_ONLY_THEN_RELOCK_AFTER_FINAL_CI',delete_authorization=False,
                proposal_sha256=m.f.sha(root/'protection-owner-maintenance-proposal01.json')))
            save(old/'execution-binding.json',{'branch_lock':{'after':before}});save(old/'preparation-terminal.json',{'recorded_expiry':True})
            client=type('FakeClient',(),{})();client.snapshots=0;client.mutations=[]
            def take():
                client.snapshots+=1
                value=before if client.snapshots<3 else after
                if mode=='drift' and client.snapshots==2:
                    value=copy.deepcopy(value);value['head']='b'*40
                return copy.deepcopy(value)
            def graph(query,variables):
                self.assertIn('mutation($input:UpdateBranchProtectionRuleInput!)',query)
                self.assertEqual(variables,{'input':{'branchProtectionRuleId':'RULE_EXACT','lockBranch':False}})
                client.mutations.append(copy.deepcopy(variables))
                if mode=='unknown':raise TimeoutError('mutation outcome unknown')
                return {'updateBranchProtectionRule':{'branchProtectionRule':after['rule']}}
            client.snapshot=take;client.graph=graph
            for obj,name,value in [(m,'BASE',base),(m,'PRIVATE',private),(m,'HEAD',HEAD),(m,'RULE','RULE_EXACT'),
                                   (m,'BINDING',m.f.sha(old/'execution-binding.json')),
                                   (m.retained,'TERMINAL_SHA256',m.f.sha(old/'preparation-terminal.json'))]:
                stack.enter_context(patch.object(obj,name,value))
            stack.enter_context(patch.object(m.f,'GitHub',return_value=client))
            closure=stack.enter_context(patch.object(m,'closed_gate',return_value={'status':'DOUBLE_NO_HOST_IO'}))
            output=base/'operation01'
            if mode in ('drift','unknown'):
                with self.assertRaises((ValueError,TimeoutError)):m.run(output,apply=True)
                self.assertEqual(len(client.mutations),1 if mode=='unknown' else 0)
            else:
                result=m.run(output,apply=mode=='apply')
                self.assertEqual(len(client.mutations),1 if mode=='apply' else 0)
                self.assertIn('CHECK_ONLY' if mode=='check' else 'EXACT_LOCKBRANCH_ONLY_UNLOCK',result['status'])
            closure.assert_called_once()
            if mode in ('apply','unknown'):
                with self.assertRaisesRegex(ValueError,'previous mutation attempt'):m.run(base/'operation02',apply=True)
                self.assertEqual(len(client.mutations),1)
                self.assertFalse((base/'operation02').exists())

    def test_whole_success_and_check_only(self):
        self.composition('apply');self.composition('check')

    def test_fresh_drift_and_unknown_mutation_never_replay(self):
        self.composition('drift');self.composition('unknown')


if __name__=='__main__':unittest.main()
