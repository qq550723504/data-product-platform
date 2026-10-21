"""Failure containment contract. Modeled I/O never proves native IAM closure."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from contextlib import ExitStack

spec=importlib.util.spec_from_file_location('v27_closure',Path(__file__).with_name('close-v27-business-dispatches.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class SealTests(unittest.TestCase):
    def test_every_close_continues_and_partial_facts_are_observed(self):
        for fault in (None,'missing-profile','observer-stop','Core-read','disable-all','one-stop'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
                root=Path(tmp);private=root/'private';public=root/'public';private.mkdir();public.mkdir()
                stack.enter_context(patch.object(m,'CONTROL',private));stack.enter_context(patch.object(m,'PUBLIC',public))
                (private/'worker-private').mkdir();(private/'root-private').mkdir()
                if fault!='missing-profile':(private/'worker-private/closure.json').write_text('{"Enabled":true}')
                (private/'root-private/iam.json').write_text('{"disabled_identities":["test-only"]}')
                out=private/'disable';out.mkdir();(out/'status-changes.json').write_text('{"unconfirmed":[],"disabled_readback":{"test-only":"disabled"}}')
                owners={n:dict(cid=n,output=str(out)) for n in ('observer','check-disabled','disable-all')}
                manifest=dict(source=dict(Core='Core',Native='Native'),owners=owners,retained={'original':'same'})
                running={'observer','Core','Native'};stops=[];phases=[];reads=[]
                def docker(*args):
                    self.assertEqual(args[0],'stop');cid=args[-1];stops.append(cid)
                    if fault=='observer-stop' and cid=='observer' or fault=='one-stop' and cid=='check-disabled':raise OSError('modeled stop failure')
                    running.discard(cid);return b''
                def inspect(cid):return dict(Id=cid,State=dict(Running=cid in running,Pid=1 if cid in running else 0))
                def fp(cid):
                    reads.append(cid)
                    if fault=='Core-read':raise OSError('modeled unavailable Core')
                    return {'append-only':'new facts'}
                def run_owner(_,kind):
                    phases.append(kind)
                    if fault=='disable-all':raise OSError('modeled disable failure')
                for obj,name,value in [(m,'docker',docker),(m,'inspect',inspect),(m,'closed',lambda x:not x['State']['Running'] and x['State']['Pid']==0),(m,'retained',lambda:{'original':'same'}),(m,'current_rows',lambda *a:[{'original-failure':'preserved'}]),(m.c,'run_owner',run_owner),(m.c,'delta',lambda *a,**k:{'old_rows_preserved':True}),(m.p,'append_anchors',lambda *a:{})]:stack.enter_context(patch.object(obj,name,value))
                result={};m.seal(manifest,['Native','Core'],SimpleNamespace(fingerprints=fp),{},{},result)
                self.assertEqual(set(stops),{'observer','check-disabled','disable-all','Core','Native'})
                self.assertEqual(phases,['disable-all'])
                self.assertTrue(result['retained_unchanged'])
                self.assertEqual(result['profile_disabled'],fault!='missing-profile')
                self.assertEqual(result['identity_containment'],'UNCONFIRMED' if fault=='disable-all' else 'FRESH_DISABLED_READBACK')
                if fault=='observer-stop':self.assertEqual(reads,[])
                if fault in ('observer-stop','Core-read'):self.assertEqual(result['final_Core'],'UNCONFIRMED')
                else:self.assertEqual(result['final_Core'],'OBSERVED_AFTER_OBSERVER_STOPPED')
                self.assertFalse(any(x in str(stops) for x in ('sender','enable','wire')))

    def test_original_v27_target_scope_is_exact(self):
        targets=m.targets()
        self.assertEqual([t['Case'] for t in targets],['P2','T1','T2'])
        self.assertEqual(len({t['Attempt'] for t in targets}),3)
        self.assertNotIn('P1',[t['Case'] for t in targets])

if __name__=='__main__':unittest.main()
