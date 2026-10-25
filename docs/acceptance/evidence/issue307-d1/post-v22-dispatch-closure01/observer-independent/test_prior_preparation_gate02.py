import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[6]
spec=importlib.util.spec_from_file_location('closure_prior_independent',ROOT/'examples/lifecycle-d1/close-old-business-dispatches.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/post-v22-business-closure01'
ORIGINAL=json.loads((PUBLIC/'preparation.json').read_bytes())

class ActualPriorGateTests(unittest.TestCase):
    def invoke(self,fault=None):
        with tempfile.TemporaryDirectory(dir=Path(__file__).parent) as tmp:
            private=Path(tmp);base=private/'post-v22-business-closure01'
            (base/'worker-private').mkdir(parents=True)
            if fault=='proof':(base/'worker-private/proof.json').write_bytes(b'nonsecret synthetic proof')
            def sha(path):
                name=Path(path).relative_to(base).as_posix()
                expected={'manifest.json':'0db3f1a118f660ac4d2f8b515334288e1ef880d915abdc733c90b730469636c4','terminal.json':'ea420fe2215894d65b1f10e040b8c4cae5b324d37ca18278c50741b2621ad084',**ORIGINAL['inputs']}[name]
                if (fault=='history' and name=='manifest.json') or (fault=='input' and name=='root-private/iam.json'):return '0'*64
                return expected
            def load(path):
                if Path(path).name=='manifest.json':return copy.deepcopy(ORIGINAL)
                if Path(path).name=='closure.json':return {'Enabled':fault=='enabled'}
                raise AssertionError('unexpected modeled read')
            def inspect(cid):return {'Id':cid,'Image':m.p1.PG,'State':{'Running':fault=='running'}}
            with patch.object(m.p1,'PRIVATE',private),patch.object(m,'sha',sha),patch.object(m,'load',load),patch.object(m,'inspect',inspect),patch.object(m,'closed',lambda x:not x['State']['Running']):
                return m.prior_preparation_closed()
    def test_fixed_three_failed_owners_preserved(self):
        result=self.invoke();self.assertEqual(result['closed_cids'],sorted(x['cid'] for x in ORIGINAL['owners'].values()));self.assertFalse(result['Enabled']);self.assertFalse(result['proof_present'])
    def test_history_input_enabled_proof_and_running_drift_rejected(self):
        for fault in ('history','input','enabled','proof','running'):
            with self.subTest(fault=fault),self.assertRaises(ValueError):self.invoke(fault)

if __name__=='__main__':unittest.main()
