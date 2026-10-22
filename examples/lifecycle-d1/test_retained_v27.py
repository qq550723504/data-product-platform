"""Fixed old-set retention and the independent post-closure Core contract."""
from contextlib import ExitStack
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import retained_v27 as old

spec=importlib.util.spec_from_file_location('c18_runtime_test',Path(__file__).with_name('six_candidate_runtime.py'))
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)


class RetainedV27Tests(unittest.TestCase):
    def test_old_bindings_are_an_explicit_ordered_list_not_a_metadata_object(self):
        with tempfile.TemporaryDirectory() as tmp:
            path=Path(tmp)/'bindings.json';good=[{'group':g} for g in ('business-races','late-hold','mixed')]
            path.write_text(json.dumps(good));self.assertEqual(old.load_bindings(path),good)
            for value in ({'members':good},good[:2],good[::-1]):
                path.write_text(json.dumps(value))
                with self.assertRaises(ValueError):old.load_bindings(path)

    def test_new_generation_does_not_enter_the_fixed_old_profile_set(self):
        with tempfile.TemporaryDirectory() as tmp:
            private=Path(tmp);prior=private/'remaining-candidate17';new=private/'remaining-candidate19'
            for folder in (prior,new):
                (folder/'business-races/P2').mkdir(parents=True)
                (folder/'business-races/P2/permission.json').write_bytes(b'{"Enabled":false}')
            expected={'remaining-candidate17/business-races/P2/permission.json':old.sha(prior/'business-races/P2/permission.json')}
            self.assertEqual(old.fixed_old_files(private,expected),expected)
            (new/'business-races/P2/permission.json').write_bytes(b'{"Enabled":true}')
            self.assertEqual(old.fixed_old_files(private,expected),expected)

    def test_old_profile_change_or_expansion_cannot_be_hidden_by_new_generation(self):
        for fault in ('changed','extra'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp:
                private=Path(tmp);folder=private/'remaining-candidate17/business-races/P2';folder.mkdir(parents=True)
                path=folder/'permission.json';path.write_bytes(b'{"Enabled":false}')
                expected={'remaining-candidate17/business-races/P2/permission.json':old.sha(path)}
                if fault=='changed':path.write_bytes(b'{"Enabled":true}')
                else:
                    extra=folder.parent/'foreign';extra.mkdir();(extra/'permission.json').write_bytes(b'{"Enabled":false}')
                with self.assertRaises(ValueError):old.fixed_old_files(private,expected)

    def test_old_budget_must_keep_its_complete_exact_mount_graph(self):
        old.budget_users(['wire','P2'],['P2','wire'])
        for actual in (['wire'],['wire','P2','c18-owner'],['wire','P2','P2']):
            with self.subTest(actual=actual),self.assertRaises(ValueError):old.budget_users(['wire','P2'],actual)

    def test_old_budget_volume_identity_cannot_be_relabelled_or_redirected(self):
        good=dict(Name='old-budget',Driver='local',Scope='local',Options=None,Labels={'lifecycle307.remaining':old.EXECUTION,'lifecycle307.allocation':'old-nonce'})
        old.budget_owner(good,'old-budget','old-nonce')
        for key,value in [('Name','foreign'),('Driver','foreign'),('Scope','global'),('Options',{'device':'foreign'}),('Labels',{'lifecycle307.remaining':r.p.EXECUTION,'lifecycle307.allocation':'old-nonce'})]:
            with self.subTest(key=key),self.assertRaises(ValueError):old.budget_owner(dict(good,**{key:value}),'old-budget','old-nonce')

    def test_core_checkpoint_bytes_and_live_core_are_independent_of_old_policy_fact(self):
        import retained_v28 as current
        for group,(name,digest) in current.CORE_BASELINES.items():
            raw=(r.ROOT/'docs/acceptance/evidence/issue307-d1'/name).read_bytes()
            self.assertEqual(old.sha(r.ROOT/'docs/acceptance/evidence/issue307-d1'/name),digest)
            with tempfile.TemporaryDirectory() as tmp,patch.object(r,'ROOT',Path(tmp)):
                path=Path(tmp)/'docs/acceptance/evidence/issue307-d1'/name;path.parent.mkdir(parents=True);path.write_bytes(raw)
                self.assertEqual(r.baseline_core(group),r.load(path))
                path.write_bytes(raw+b' ')
                with self.assertRaises(ValueError):r.baseline_core(group)
        self.assertEqual(r.p.window.POLICY_CHECKPOINT_SHA256,'aab0e61530bee63429a15ce635880a6d1db286cb744767f7d9fdb50082313df8')
        self.assertEqual(r.p.EXECUTION,'a5d1e569-f80e-4b38-8f32-d8139a30a077')
        self.assertNotEqual(r.p.EXECUTION,old.EXECUTION)


if __name__=='__main__':unittest.main()
