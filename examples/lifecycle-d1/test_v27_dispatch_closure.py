"""Failure containment contract. Modeled I/O never proves native IAM closure."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import datetime as dt
from types import SimpleNamespace
import unittest
from unittest.mock import patch
from contextlib import ExitStack

spec=importlib.util.spec_from_file_location('v27_closure',Path(__file__).with_name('close-v27-business-dispatches.py'))
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)

class SealTests(unittest.TestCase):
    def test_complete_IAM_retains_real_absence_as_a_separate_request(self):
        for fault in (None,'missing-absence','absence-200','foreign-user','missing-policy','stale','future-observation','extra-read'):
            with self.subTest(fault=fault),tempfile.TemporaryDirectory() as tmp,ExitStack() as stack:
                root=Path(tmp);(root/'root-private').mkdir();out=root/'output';out.mkdir();at=m.now()
                users={('user'+str(i)):'disabled' for i in range(13)}
                (root/'root-private/iam.json').write_text(json.dumps(dict(disabled_identities=users)))
                (out/'result.json').write_text(json.dumps(dict(action='check-disabled',exact_policy_set_verified=True,changed={},before=users)))
                requests=[('/minio/admin/v3/user-info',user,200) for user in users]+[('/minio/admin/v3/info-canned-policy','',200) for _ in range(13)]+[('/minio/admin/v3/user-info','d4e524c04808a6960',404)]
                if fault=='missing-absence':requests.pop()
                if fault=='absence-200':requests[-1]=(requests[-1][0],requests[-1][1],200)
                if fault=='foreign-user':requests[0]=(requests[0][0],'other',200)
                if fault=='missing-policy':requests.pop(13)
                if fault=='extra-read':requests.append(requests[0])
                for i,(path,user,status) in enumerate(requests):
                    (out/f'start-{i}.json').write_text(json.dumps(dict(method='GET',path=path,access=user,at=(at-dt.timedelta(seconds=2)).isoformat())))
                    (out/f'observation-{i}.json').write_text(json.dumps(dict(http_status=status,transport_error=False,at=(at+dt.timedelta(seconds=10) if fault=='future-observation' else at-dt.timedelta(seconds=1)).isoformat())))
                state=dict(State=dict(StartedAt=(at-dt.timedelta(seconds=3)).isoformat(),FinishedAt=(at-dt.timedelta(seconds=46) if fault=='stale' else at).isoformat(),ExitCode=0))
                stack.enter_context(patch.object(m,'CONTROL',root));stack.enter_context(patch.object(m,'inspect',lambda *a:state));stack.enter_context(patch.object(m,'closed',lambda *a:True))
                if fault:
                    with self.assertRaises(ValueError):m.fresh_iam(dict(cid='test-only',output=str(out)))
                else:
                    result=m.fresh_iam(dict(cid='test-only',output=str(out)))
                    self.assertEqual(result['quantity'],27)
                    self.assertEqual(result['absent_identity'],'d4e524c04808a6960')

    def test_bounded_windows_argv_still_requires_every_owner(self):
        ids=[format(i,'064x') for i in range(540)];calls=[]
        def docker(*args):
            self.assertLessEqual(len(args)-1,64);calls.append(args)
            return json.dumps([dict(Id=cid) for cid in args[1:]]).encode()
        with patch.object(m,'docker',docker):self.assertEqual(len(m.inspect_many(ids)),540)
        self.assertEqual(len(calls),9)
        with patch.object(m,'docker',return_value=b'[]'),self.assertRaises(ValueError):m.inspect_many(ids)
        for bad in (ids+[ids[0]],['short-id']):
            with patch.object(m,'docker',side_effect=AssertionError('no I/O')),self.assertRaises(ValueError):m.inspect_many(bad)

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
