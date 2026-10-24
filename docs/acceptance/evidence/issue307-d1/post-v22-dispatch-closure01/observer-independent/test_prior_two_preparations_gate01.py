import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[6]
spec=importlib.util.spec_from_file_location('prior_two_independent',ROOT/'examples/lifecycle-d1/close-old-business-dispatches.py')
m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
E=ROOT/'docs/acceptance/evidence/issue307-d1'
MANIFESTS={n:json.loads((E/f'post-v22-business-closure{n}/preparation.json').read_bytes()) for n in ('01','02')}
RECEIPT=json.loads((E/'post-v22-dispatch-closure01/docker-allocation-contract01.json').read_bytes())
INSPECT=json.loads((Path(__file__).parent/'host-allocation-inspect02.json').read_bytes())
SHA={'01':{'manifest.json':'0db3f1a118f660ac4d2f8b515334288e1ef880d915abdc733c90b730469636c4','terminal.json':'ea420fe2215894d65b1f10e040b8c4cae5b324d37ca18278c50741b2621ad084'},'02':{'manifest.json':'a5adf3b5216382c013b2f5325dee04963ff744c975d28b84b81c119f8bf0fede','terminal.json':'ce9c6d859169974b05922bd75d096937d4ebf78f7a25dc84c263de46c40d8970'}}
class PriorTwoTests(unittest.TestCase):
 def invoke(self,fault=None):
  with tempfile.TemporaryDirectory(dir=Path(__file__).parent) as tmp:
   private=Path(tmp);allocation=copy.deepcopy(RECEIPT);allocation['owner']['private']=str(private/'post-v22-allocation-contract01/private')
   for folder in ('post-v22-business-closure01/worker-private','post-v22-business-closure02/worker-private','post-v22-allocation-contract01/private'):(private/folder).mkdir(parents=True)
   if fault=='proof02':(private/'post-v22-business-closure02/worker-private/proof.json').write_bytes(b'synthetic')
   if fault=='allocationproof':(private/'post-v22-allocation-contract01/private/proof.json').write_bytes(b'synthetic')
   def load(path):
    path=Path(path);name=path.relative_to(private).as_posix()
    if name.endswith('manifest.json'):return copy.deepcopy(MANIFESTS['01' if 'closure01/'in name else '02'])
    if name.endswith('receipt.json'):return allocation
    if path.name=='closure.json':return {'Enabled': fault=='enabled02' and 'closure02/'in name or fault=='allocationenabled' and 'allocation' in name}
    raise AssertionError('unexpected modeled read')
   def sha(path):
    name=Path(path).relative_to(private).as_posix()
    if 'business-closure'in name:
     sequence='01'if 'closure01/'in name else '02';relative=name.split('/',1)[1]
     value={**SHA[sequence],**MANIFESTS[sequence]['inputs']}[relative]
     if fault=='history02' and sequence=='02' and relative=='manifest.json' or fault=='input02' and sequence=='02' and relative=='root-private/iam.json':return '0'*64
     return value
    if name.endswith('receipt.json'):return '0'*64 if fault=='allocationreceipt' else 'c9f97b18d64058bacf818de4a8852a1a60844593a1b638928f9c3dafad428817'
    return '0'*64 if fault=='allocationprofile' else RECEIPT['profile_sha256']
   def inspect(cid):
    x=copy.deepcopy(next(r for r in INSPECT['records']if r['Id']==cid))
    if fault=='running02' and x['scope'].startswith('failed02:') or fault=='allocationrunning' and x['scope']=='allocation-contract':x['State']['Running']=True
    return x
   with patch.object(m.p1,'PRIVATE',private),patch.object(m,'load',load),patch.object(m,'sha',sha),patch.object(m,'inspect',inspect),patch.object(m,'closed',lambda x:not x['State']['Running']),patch.object(m,'own_contract',lambda owner,manifest:inspect(owner['cid'])):
    return m.prior_preparation_closed()
 def test_actual_two_failed_attempts_and_allocation_gate_success(self):
  value=self.invoke();self.assertEqual(set(value),{'01','02','allocation_contract'});self.assertEqual(sum(len(value[n]['closed_cids'])for n in ('01','02')),6);self.assertEqual(sum(len(value[n]['retained_volumes'])for n in ('01','02')),6);self.assertEqual(value['allocation_contract']['closed_cid'],RECEIPT['owner']['cid'])
 def test_history_input_state_and_allocation_drift_refused(self):
  for fault in ('history02','input02','proof02','enabled02','running02','allocationreceipt','allocationprofile','allocationproof','allocationenabled','allocationrunning'):
   with self.subTest(fault=fault),self.assertRaises(ValueError):self.invoke(fault)
if __name__=='__main__':unittest.main()
