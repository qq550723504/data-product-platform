import copy
from contextlib import ExitStack
import datetime as dt
import json
from pathlib import Path
import subprocess
import tempfile
import types
import unittest
from unittest.mock import patch
ROOT=Path(__file__).resolve().parents[6]
SNAPSHOT=Path(__file__).parent/'host-draft-snapshot01.py'
m=types.ModuleType('independent_seventeen_host_snapshot');m.__file__=str(ROOT/'examples/lifecycle-d1/verify-post-v22-seventeen.py')
exec(compile(SNAPSHOT.read_bytes(),m.__file__,'exec'),m.__dict__)
class OriginalChannelTests(unittest.TestCase):
 def test_known_close_inspect_failure_still_requires_stop(self):
  calls=[]
  with patch.object(m,'inspect',side_effect=ValueError('synthetic unreadable inspect')),patch.object(m,'docker',side_effect=lambda *a:calls.append(a)or b''):
   result=m.close_all(['one','two'])
  self.assertEqual({c[-1]for c in calls if c[0]=='stop'},{'one','two'},f'known close was skipped: {result}')
 def fixture(self,fault):
  with tempfile.TemporaryDirectory(dir=Path(__file__).parent)as tmp,ExitStack()as stack:
   root=Path(tmp);private=root/'private';control=root/'control';public=root/'public';group='business-races';folder=public/group;output=folder/'readonly'
   for p in(private,control,output):p.mkdir(parents=True,exist_ok=True)
   profile={'Enabled':False,'MaxProviderDeletes':0,'CandidateSHA256':'c'*64,'FrozenInventorySHA256':'f'*64,'PresentInventorySHA256':'a'*64}
   for path in(private/'post-v22-readonly.json',control/(group+'-disabled-original.json')):path.write_text(json.dumps(profile),encoding='utf-8')
   source=json.loads((m.p1.PACKET/'candidate.json').read_bytes())['Groups'][0]
   baseline=json.loads((ROOT/'docs/acceptance/evidence/issue307-d1/post-v22-business-closure03/Core-after.json').read_bytes())
   owner={'cid':'helper','private':str(private),'output':str(output),'binary_sha256':'b'*64,'Core':source['Core'],'Native':source['Native'],'Network':source['Network']}
   manifest={'audit_id':'new-synthetic-audit','owners':{group:owner}}
   def docker(*args):return json.dumps([{'Internal':True,'Containers':{owner['Core']:{},owner['Native']:{}}}]).encode()if args[0]=='network'else b''
   def run(args,**kw):
    if len(args)>1 and args[1]=='wait':return types.SimpleNamespace(returncode=0,stdout=b'0\n',stderr=b'')
    if len(args)>1 and args[1]=='logs':
     if fault=='logs-timeout':raise subprocess.TimeoutExpired(args,30,output=b'partial owner stdout',stderr=b'partial owner stderr')
     return types.SimpleNamespace(returncode=0,stdout=b'',stderr=b'')
    if fault=='checker-timeout':raise subprocess.TimeoutExpired(args,45,output=b'partial checker stdout',stderr=b'partial checker stderr')
    return types.SimpleNamespace(returncode=0,stdout=json.dumps({'status':'PASS_COMPLETE_POST_V22_NATIVE_AGGREGATION','copies':7,'group':group}).encode(),stderr=b'')
   def owner_contract(*args,**kw):return {'State':{'StartedAt':m.now().isoformat(),'FinishedAt':m.now().isoformat(),'ExitCode':0}}
   for obj,name,value in[(m,'CONTROL',control),(m,'PUBLIC',public),(m,'docker',docker),(m,'owner_contract',owner_contract),(m,'closed',lambda x:True),(m,'close_all',lambda ids:[{'cid':x,'closed':True}for x in ids]),(m.closure,'source_contract',lambda x:None),(m.native,'wait_pg',lambda *a:None),(m.native,'fingerprints',lambda *a:copy.deepcopy(baseline)),(m.time,'sleep',lambda x:None),(m.subprocess,'run',run),(m.old_verifier,'journals',lambda x:1)]:stack.enter_context(patch.object(obj,name,value))
   with self.assertRaises(ValueError):m.collect(manifest,group)
   if fault=='checker-timeout':
    self.assertEqual((folder/'wait-stdout.txt').read_bytes(),b'0\n','checker timeout replaced original wait channel')
    self.assertEqual((folder/'checker-stdout.txt').read_bytes(),b'partial checker stdout')
   else:
    self.assertTrue((folder/'stdout.txt').exists(),'logs timeout lost original partial stdout')
    self.assertEqual((folder/'stdout.txt').read_bytes(),b'partial owner stdout')
 def test_checker_timeout_cannot_replace_wait_original(self):self.fixture('checker-timeout')
 def test_logs_timeout_keeps_partial_original_channels(self):self.fixture('logs-timeout')
if __name__=='__main__':unittest.main()
