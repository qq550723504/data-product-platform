"""Behavioral freeze/CI/policy/race regressions; native GitHub tested separately."""
import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
import execution_freeze as f

HEAD='a'*40


def snapshot(locked=False,unprotected=False):
    flags={k: (v if locked else not v) for k,v in f.FLAGS.items()}
    rule=None if unprotected else dict(id='RULE_EXACT',pattern=f.BRANCH,**flags)
    policy=None if unprotected else {
        'required_status_checks':{'strict':True,'contexts':['required']},
        'required_pull_request_reviews':{'required_approving_review_count':2},
        'restrictions':{'users':[{'login':'trusted-owner'}]},
        'required_signatures':{'enabled':True},
        **{k:{'enabled':locked if k in ('lock_branch','enforce_admins') else not locked}
           for k in f.REST_FLAGS}}
    return dict(repository=f.REPOSITORY,repository_id='REPO_ID',branch=f.BRANCH,
                expected_branch=f.BRANCH,head=HEAD,pr=332,draft=True,state='open',
                merged=False,issue_state='open',rule=rule,protection=policy,
                checks=[dict(id=1,name='required',app_id=f.ACTIONS_APP_ID,app_slug='github-actions',suite_id=8,head_sha=HEAD,
                             status='completed',conclusion='success')],
                ci=dict(id=9,attempt=1,path='.github/workflows/ci.yml',event='pull_request',
                        repository=f.REPOSITORY,head_repository=f.REPOSITORY,head_sha=HEAD,
                        pull_requests=[332],status='completed',conclusion='success',suite_id=8,
                        jobs=[dict(check_run_id=1,name='required',run_id=9,run_attempt=1,
                                   head_sha=HEAD,status='completed',conclusion='success')]))


class RecordedGitHub:
    branch=f.BRANCH
    def __init__(self,before,after=None,fault=None):
        self.before=copy.deepcopy(before);self.after=copy.deepcopy(after or snapshot(True))
        self.calls=[];self.fault=fault
    def snapshot(self):
        self.calls.append('snapshot')
        return copy.deepcopy(self.before if len(self.calls)==1 else self.after)
    def lock(self,before):
        self.calls.append('lock')
        if self.fault:raise self.fault
        return copy.deepcopy(self.after['rule'])


class FreezeTests(unittest.TestCase):
    def test_required_check_cannot_substitute_dispatch_foreign_workflow_app_or_incomplete_suite(self):
        changes=[('event','workflow_dispatch'),('path','.github/workflows/other.yml'),
                 ('repository','other/repository'),('head_repository','other/repository'),
                 ('head_sha','b'*40),('pull_requests',[333]),('status','queued'),('conclusion','failure')]
        for field,value in changes:
            current=snapshot(True);current['ci'][field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):f.ready(current,HEAD,require_frozen=True)
        for field,value in [('app_id',9876),('app_slug','other-app'),('suite_id',999)]:
            current=snapshot(True);current['checks'][0][field]=value
            with self.subTest(field=field),self.assertRaises(ValueError):f.ready(current,HEAD,require_frozen=True)
        for fault in ('missing-job','foreign-run','foreign-attempt','failed-job','missing-required'):
            current=snapshot(True)
            if fault=='missing-job':current['ci']['jobs']=[]
            elif fault=='foreign-run':current['ci']['jobs'][0]['run_id']=999
            elif fault=='foreign-attempt':current['ci']['jobs'][0]['run_attempt']=2
            elif fault=='failed-job':current['ci']['jobs'][0]['conclusion']='failure'
            else:current['checks'][0]['name']=current['ci']['jobs'][0]['name']='optional'
            with self.subTest(fault=fault),self.assertRaises(ValueError):f.ready(current,HEAD,require_frozen=True)

    def test_unreadable_or_aliased_control_fact_is_not_absence(self):
        with tempfile.TemporaryDirectory() as tmp:
            private=Path(tmp);original=Path.stat
            def unreadable(path,*args,**kw):
                if path.name=='user-message-verbatim.txt':raise PermissionError('unreadable authority')
                return original(path,*args,**kw)
            with patch.object(Path,'stat',unreadable),self.assertRaises(PermissionError):
                f.control_fact_present(private,'user-message-verbatim.txt')
            with patch.object(Path,'is_symlink',return_value=True),self.assertRaises(ValueError):
                f.control_fact_present(private,'terminal.json')

    def test_full_lock_success_retains_existing_review_checks_restrictions_and_signatures(self):
        with tempfile.TemporaryDirectory() as tmp:
            client=RecordedGitHub(snapshot())
            result=f.acquire(client,HEAD,Path(tmp)/'new')
            self.assertEqual(client.calls,['snapshot','lock','snapshot'])
            self.assertFalse(result['atomic_CAS'])
            self.assertFalse(result['automatic_unlock'])
            self.assertEqual(f.policy_without_freeze(result['protection']),
                             f.policy_without_freeze(snapshot()['protection']))
            self.assertTrue((Path(tmp)/'new/branch-lock.json').is_file())

    def test_unprotected_exact_rule_can_be_created_but_not_shared_or_already_locked(self):
        with tempfile.TemporaryDirectory() as tmp:
            result=f.acquire(RecordedGitHub(snapshot(unprotected=True)),HEAD,Path(tmp)/'new')
            self.assertEqual(result['rule_id'],'RULE_EXACT')
        for kind in ('wildcard','already-locked'):
            before=snapshot(kind=='already-locked')
            if kind=='wildcard':before['rule']['pattern']='codex/*'
            with self.subTest(kind=kind),tempfile.TemporaryDirectory() as tmp:
                client=RecordedGitHub(before)
                with self.assertRaises(ValueError):f.acquire(client,HEAD,Path(tmp)/'new')
                self.assertNotIn('lock',client.calls)

    def test_failed_or_incomplete_ci_foreign_head_repo_and_pr_rejected_before_lock(self):
        changes=[('head','b'*40),('repository','attacker/repo'),('pr',333),
                 ('draft',False),('state','closed'),('merged',True),('issue_state','closed')]
        for field,value in changes:
            before=snapshot();before[field]=value
            with self.subTest(field=field),tempfile.TemporaryDirectory() as tmp:
                client=RecordedGitHub(before)
                with self.assertRaises(ValueError):f.acquire(client,HEAD,Path(tmp)/'new')
                self.assertNotIn('lock',client.calls)
        for status,conclusion in [('queued',None),('completed','failure'),('completed','skipped')]:
            before=snapshot();before['checks'][0].update(status=status,conclusion=conclusion)
            with self.subTest(status=status,conclusion=conclusion),tempfile.TemporaryDirectory() as tmp:
                client=RecordedGitHub(before)
                with self.assertRaises(ValueError):f.acquire(client,HEAD,Path(tmp)/'new')
                self.assertNotIn('lock',client.calls)

    def test_head_policy_or_flags_changed_after_lock_stop_without_unlock(self):
        for fault in ('head','old-policy',*f.FLAGS):
            after=snapshot(True)
            if fault=='head':after['head']='b'*40
            elif fault=='old-policy':after['protection']['required_status_checks']['strict']=False
            else:after['rule'][fault]=not after['rule'][fault]
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp:
                client=RecordedGitHub(snapshot(),after)
                with self.assertRaises(ValueError):f.acquire(client,HEAD,Path(tmp)/'new')
                self.assertEqual(client.calls,['snapshot','lock','snapshot'])
                self.assertTrue((Path(tmp)/'new/lock-intent.json').exists())
                self.assertFalse((Path(tmp)/'new/branch-lock.json').exists())

    def test_uncertain_mutation_or_receipt_failure_keeps_intent_and_never_retries(self):
        with tempfile.TemporaryDirectory() as tmp:
            client=RecordedGitHub(snapshot(),fault=TimeoutError('unknown server outcome'))
            with self.assertRaises(TimeoutError):f.acquire(client,HEAD,Path(tmp)/'new')
            self.assertEqual(client.calls,['snapshot','lock'])
            self.assertTrue((Path(tmp)/'new/lock-intent.json').exists())
        with tempfile.TemporaryDirectory() as tmp:
            client=RecordedGitHub(snapshot());out=Path(tmp)/'new';real=f.save
            def reject(path,value):
                if path.name=='after.json':raise OSError('disk failure after native lock')
                real(path,value)
            with patch.object(f,'save',reject),self.assertRaises(OSError):
                f.acquire(client,HEAD,out)
            self.assertEqual(client.calls,['snapshot','lock','snapshot'])
            self.assertFalse((out/'branch-lock.json').exists())

    def test_duplicate_freeze_output_never_replays_native_mutation(self):
        with tempfile.TemporaryDirectory() as tmp:
            client=RecordedGitHub(snapshot());f.acquire(client,HEAD,Path(tmp)/'new')
            calls=list(client.calls)
            with self.assertRaises(ValueError):f.acquire(client,HEAD,Path(tmp)/'new')
            self.assertEqual(client.calls,calls)

    def test_graphql_update_owns_only_five_fields_not_existing_protection(self):
        with tempfile.TemporaryDirectory() as tmp:
            client=f.GitHub(Path(tmp));seen=[]
            def graph(query,variables):
                seen.append((query,variables))
                return {'updateBranchProtectionRule':{'branchProtectionRule':snapshot(True)['rule']}}
            client.graph=graph
            client.lock(snapshot())
            self.assertEqual(set(seen[0][1]['input']),{'branchProtectionRuleId',*f.FLAGS})
            self.assertNotIn('pattern',seen[0][1]['input'])
            self.assertNotIn('expectedHead',seen[0][1]['input'])

    def test_api_403_404_unknown_and_graphql_errors_fail_closed_with_original_channels(self):
        cases=[(403,{'status':'403','message':'Forbidden'},False),
               (404,{'status':'404','message':'Not Found'},False),
               (404,{'status':'404','message':'Branch not protected'},True),
               (0,{'errors':[{'message':'denied'}]},False)]
        for code,body,allowed in cases:
            with self.subTest(body=body),tempfile.TemporaryDirectory() as tmp:
                runner=lambda *a,**k:SimpleNamespace(returncode=code,stdout=json.dumps(body).encode(),stderr=b'original diagnostic')
                client=f.GitHub(Path(tmp),runner=runner)
                if allowed:self.assertIsNone(client.request('GET','fixed',absent_protection=True))
                else:
                    with self.assertRaises(ValueError):client.request('GET','fixed',absent_protection=True)
                self.assertEqual((Path(tmp)/'001-stderr.txt').read_bytes(),b'original diagnostic')

    def binding_case(self,tmp):
        root=Path(tmp);private=root/'private';packet=root/'packet';out=root/'frozen'
        private.mkdir();packet.mkdir()
        candidate=dict(Execution='new-execution',Profile='candidate20/v30',Source='s'*40,
                       ReaderBinarySHA256='r'*64,SenderBinarySHA256='d'*64,OwnerBinarySHA256='o'*64)
        for name,value in [('candidate.json',candidate),('bindings.json',[]),('source-freeze.json',{})]:
            f.save(packet/name,value)
        (packet/'confirmation-template.txt').write_bytes(b'NEW EXACT PLAN')
        lock=f.acquire(RecordedGitHub(snapshot()),HEAD,out)
        binding=dict(status='FROZEN_EXECUTION_BINDING_NOT_AUTHORIZED',execution=candidate['Execution'],
                     profile=candidate['Profile'],artifact_bindings=f.artifact_bindings(packet,candidate),
                     branch_lock=lock,public_binding_path='frozen/execution-binding.json')
        f.save(out/'execution-binding.json',binding)
        (private/'execution-binding.json').write_bytes((out/'execution-binding.json').read_bytes())
        expected=f.sha(private/'execution-binding.json')
        auth=dict(execution=candidate['Execution'],execution_binding_sha256=expected,final_approved_head=HEAD,
                  approved_reference_artifacts=[{'path':binding['public_binding_path'],'sha256':expected}])
        return root,private,packet,candidate,expected,auth

    def test_actual_binding_and_fresh_snapshot_success(self):
        with tempfile.TemporaryDirectory() as tmp:
            root,private,packet,c,expected,auth=self.binding_case(tmp)
            client=SimpleNamespace(snapshot=lambda:snapshot(True))
            with patch.object(f.subprocess,'check_output',side_effect=[HEAD.encode(),b'']):
                result=f.verify_binding(root,private,packet,c,expected,client,auth=auth)
            self.assertEqual(result['status'],'PASS_FRESH_EXECUTION_BRANCH_FREEZE')

    def test_binding_scope_manifest_terminal_and_checkout_changes_cannot_reuse_authority(self):
        for fault in ('execution','candidate','auth-binding','approval-reference','terminal','checkout','policy','unlocked'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp:
                root,private,packet,c,expected,auth=self.binding_case(tmp);current=snapshot(True)
                if fault=='execution':c['Execution']='old-execution'
                elif fault=='candidate':(packet/'confirmation-template.txt').write_bytes(b'changed exact scope')
                elif fault=='auth-binding':auth['execution_binding_sha256']='0'*64
                elif fault=='approval-reference':auth['approved_reference_artifacts']=[]
                elif fault=='terminal':
                    (private/'authorized-execution01').mkdir();f.save(private/'authorized-execution01/terminal.json',{'terminal':True})
                elif fault=='policy':current['protection']['required_signatures']['enabled']=False
                elif fault=='unlocked':current['rule']['lockBranch']=False
                result_head=b'b'*40 if fault=='checkout' else HEAD.encode()
                with patch.object(f.subprocess,'check_output',side_effect=[result_head,b'']),self.assertRaises(ValueError):
                    f.verify_binding(root,private,packet,c,expected,SimpleNamespace(snapshot=lambda:current),auth=auth)

    def test_host_h1_rejects_branch_before_any_inspect_or_signing(self):
        spec=importlib.util.spec_from_file_location('freeze_host_guard',Path(__file__).with_name('window_six_support.py'))
        w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
        h=object.__new__(w.Host);h.candidate={}
        with patch.object(w.execution_freeze,'authorized_gate',side_effect=ValueError('branch drift')) as gate,patch.object(h,'inspect') as inspect:
            with self.assertRaisesRegex(ValueError,'branch drift'):h.h1_proof()
            gate.assert_called_once();inspect.assert_not_called()


if __name__=='__main__':
    unittest.main()

