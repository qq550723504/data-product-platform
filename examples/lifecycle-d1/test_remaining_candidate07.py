import copy
import importlib.util
from pathlib import Path
import tempfile
import unittest
import datetime as dt

def module(name,filename):
    spec=importlib.util.spec_from_file_location(name,Path(__file__).with_name(filename));m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
a=module('candidate07_audit','check-remaining-candidate07.py')
h=module('candidate07_attester','attest-remaining-executable.py')

class Candidate07Guards(unittest.TestCase):
    def test_exact_model_and_tamper_rejection(self):
        c=a.load(a.ROOT/a.PACKET/'candidate.json');b=a.load(a.ROOT/a.PACKET/'bindings.json');scope=a.load(a.ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json')
        self.assertEqual(len(a.validate(c,b,scope)[0]),7)
        mutations=[lambda q:q.update(Execution='9ea8f3e2-c805-44b4-b133-a36f50df9d15'),lambda q:q.update(NotAfter='2026-10-17T02:41:00Z'),lambda q:q.update(WirePublicKey=q['PublicKey']),lambda q:q['Groups'][0]['Copies'][0].update(ProviderVersion='foreign-version'),lambda q:q['Groups'][0]['Copies'].pop(),lambda q:q['Groups'][0]['Helpers'].pop('recover-P1'),lambda q:q['Groups'][0]['PolicyHeads'].clear(),lambda q:q['Groups'][0].update(CheckpointCore=q['Groups'][0]['Core'])]
        for i,mutate in enumerate(mutations):
            with self.subTest(i=i):
                changed=copy.deepcopy(c);mutate(changed)
                with self.assertRaises((ValueError,KeyError)):a.validate(changed,b,scope)

    def test_raw_evidence_path_and_bytes(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);(root/'receipt.json').write_bytes(b'{"native_delete":0}\n');records={'receipt.json':a.sha((root/'receipt.json').read_bytes())}
            a.verify_records(root,records);(root/'receipt.json').write_bytes(b'{"native_delete":1}\n')
            with self.assertRaises(ValueError):a.verify_records(root,records)
            with self.assertRaises(ValueError):a.verify_records(root,{'../escape':a.sha(b'')})

    def test_host_attester_rejects_unconfirmed_shifted_and_expired_authority(self):
        c={'Execution':a.EXECUTION,'Profile':'candidate07/v17','NotBefore':a.START,'NotAfter':a.END}
        p=dict(c,Enabled=True,MaxPerTarget=1,MaxTotal=7,ExpiresAt=a.END,ConfirmedAt='2026-10-17T01:59:00Z')
        now=dt.datetime.fromisoformat(a.START.replace('Z','+00:00'))
        h.authority(c,p,b'exact',b'exact',now,False)
        for change,at,recovery in [({'Enabled':False},now,False),({'ExpiresAt':'2026-10-17T02:41:00Z'},now,False),({},now+dt.timedelta(minutes=40),False),({},now+dt.timedelta(hours=3),True),({'Execution':'old'},now,True)]:
            with self.subTest(change=change,recovery=recovery):
                with self.assertRaises(ValueError):h.authority(c,dict(p,**change),b'exact',b'exact',at,recovery)
        with self.assertRaises(ValueError):h.authority(c,p,b'old-confirmation',b'exact',now,False)

if __name__=='__main__':unittest.main()
