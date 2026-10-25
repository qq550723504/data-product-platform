"""Whole audit orchestration; native/Docker calls are explicit doubles here."""
from contextlib import ExitStack
import copy
import importlib.util
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('refresh_under_test',Path(__file__).with_name('refresh-post-v27-seventeen.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)


class RefreshTests(unittest.TestCase):
    def composition(self,fault=None):
        with tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
            root=Path(tmp);public=root/'public';private=root/'private';public.mkdir();private.mkdir()
            stack.enter_context(patch.object(m,'PUBLIC',public));stack.enter_context(patch.object(m,'PRIVATE',private))
            stack.enter_context(patch.object(m,'code_inputs',return_value={'actual-audit.py':'reviewed-bytes'}))
            stack.enter_context(patch.object(m.subprocess,'check_output',return_value=b'head\n'))
            groups=[dict(Group=name,Core='Core-'+name,Native='Native-'+name) for name in m.p.GROUPS]
            stack.enter_context(patch.object(m,'static_gate',return_value=({'Groups':groups},[{}, {}, {}])))
            snapshots=[{'retained':'unchanged'},{'retained':'unchanged'}]
            if fault=='post-preservation':snapshots[1]={'retained':'changed'}
            stack.enter_context(patch.object(m,'snapshot',side_effect=snapshots))
            plan=dict(status='PREPARED_NEW_ZERO_DELETE_READ_AUDIT',source_head='head',code_inputs={'actual-audit.py':'reviewed-bytes'},
                      deletion_authority='NONE',old_authorization_reused=False,max_provider_deletes=0,
                      baselines={k:list(v) for k,v in m.BASELINES.items()},preservation=snapshots[0])
            if fault=='authority':plan['deletion_authority']='OLD_AUTHORIZATION'
            if fault=='delete':plan['max_provider_deletes']=1
            if fault=='baseline':plan['baselines']['business-races'][1]='0'*64
            if fault=='source':plan['code_inputs']={}
            m.save(public/'plan.json',plan);m.save(private/'plan.json',plan)
            if fault=='terminal':m.save(private/'terminal.json',{'status':'old-terminal'})
            calls=[];closed=[]
            def collect(g,b,packet,private_dir,output,deadline,*,core_baseline):
                calls.append(g['Group'])
                self.assertEqual(core_baseline,m.EVIDENCE/m.BASELINES[g['Group']][0])
                self.assertFalse(private_dir.exists());private_dir.mkdir()
                output.mkdir();(output/'files').mkdir()
                if fault=='second-group' and len(calls)==2:
                    m.save(output/'files/readonly-start-retained-request.json',{'method':'GET'})
                    raise ValueError('modeled unobserved native request')
                return {'group':g['Group'],'copies':{'business-races':7,'late-hold':6,'mixed':4}[g['Group']],'technical_requests':0}
            stack.enter_context(patch.object(m.v,'collect',side_effect=collect))
            stack.enter_context(patch.object(m.p.p1,'close_known',side_effect=lambda cid:closed.append(cid) or dict(cid=cid,closed=True)))
            if fault:
                with self.assertRaises(ValueError):m.run(m.sha(public/'plan.json'))
            else:m.run(m.sha(public/'plan.json'))
            if fault in ('authority','delete','baseline','source','terminal'):
                self.assertEqual(calls,[]);self.assertEqual(closed,[]);self.assertFalse((public/'terminal.json').exists());return
            terminal=m.load(public/'terminal.json')
            self.assertEqual(len(closed),6);self.assertEqual(terminal['native_delete'],0)
            self.assertFalse(terminal['old_authorization_reused']);self.assertEqual(terminal['deletion_authority'],'NONE')
            if fault=='second-group':
                self.assertEqual(calls,['business-races','late-hold']);self.assertEqual(terminal['present_versions'],7)
                self.assertEqual(terminal['technical_requests'],1)
                self.assertEqual(terminal['request_journals']['late-hold']['unobserved_attempts'],['retained-request'])
                self.assertEqual(terminal['not_completed_groups'],['late-hold','mixed'])
            elif not fault:
                self.assertEqual(terminal['status'],'PASS_CURRENT_SEVENTEEN_NATIVE_READONLY_SEALED')
                self.assertEqual(terminal['present_versions'],17)
            else:self.assertEqual(terminal['status'],'FAILED_READ_AUDIT_RETAINED')

    def test_three_groups_use_bound_post_closure_core_and_no_delete_authority(self):self.composition()

    def test_authority_budget_checkpoint_code_and_terminal_reject_before_source_io(self):
        for fault in ('authority','delete','baseline','source','terminal'):
            with self.subTest(fault=fault):self.composition(fault)

    def test_partial_failure_retains_request_and_does_not_attempt_next_group(self):self.composition('second-group')

    def test_changed_preservation_fails_after_independent_closes(self):self.composition('post-preservation')


if __name__=='__main__':unittest.main()
