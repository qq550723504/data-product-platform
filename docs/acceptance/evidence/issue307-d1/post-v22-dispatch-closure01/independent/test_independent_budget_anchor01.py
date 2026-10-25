"""Independent mock-only preexisting budget counterexamples. No Docker/provider."""
import copy,importlib.util,json,tempfile,unittest,uuid
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch
R=Path(r'C:/Users/Henry/.codex/worktrees/9400/data-product-platform')
spec=importlib.util.spec_from_file_location('independent_old_dispatch',R/'examples/lifecycle-d1/audit-old-business-dispatches.py');m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
PUBLIC=R/'docs/acceptance/evidence/issue307-d1/post-v22-p1-readonly04'
def load(p):return json.loads(p.read_text(encoding='utf-8'))
anchor=load(PUBLIC/'preparation.json')
class RetainedBudget(unittest.TestCase):
 def test_preexisting_loss_or_changed_SHA_never_starts_Core(self):
  for kind in ('missing','changed'):
   with self.subTest(kind=kind),tempfile.TemporaryDirectory() as td:
    root=Path(td);private=root/'private';calls=[]
    budget=copy.deepcopy(anchor['budgets']);name=next(iter(budget['business-races']))
    if kind=='missing':del budget['business-races'][name]
    else:budget['business-races'][name]['sha256']='f'*64
    def fake_load(p):
     if str(p).startswith(str(private)) and p.name in ('recovery.json','p1-readonly-permission.json'):return {'Enabled':False}
     return load(p)
    def fake_sha(p):
     if p.name=='candidate.json':return 'f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243'
     if p.name=='bindings.json':return '8e54b47e4a1f83c7ce66555ed6ce2f3ec66c962a394a8fa1feb84eeaccd82fc5'
     if str(p).startswith(str(private)):
      rel=p.relative_to(private);old=load(PUBLIC.parent/rel.parts[0]/'preparation.json')
      if p.name=='manifest.json':p=PUBLIC.parent/rel.parts[0]/'preparation.json'
      else:return {n.replace('\\','/'):h for n,h in old['inputs'].items()}['/'.join(rel.parts[1:])]
     import hashlib
     return hashlib.sha256(p.read_bytes()).hexdigest()
    def write(p,v):
     p.parent.mkdir(parents=True,exist_ok=True)
     with p.open('x',encoding='utf-8') as f:json.dump(v,f)
    p1=SimpleNamespace(PRIVATE=private,PUBLIC=PUBLIC,PACKET=PUBLIC,old_closed=lambda:copy.deepcopy(anchor['old_closed']),load=fake_load,sha=fake_sha,inspect=lambda cid:{'closed':True},closed=lambda v:True,input_entries=lambda x:{n.replace('\\','/'):h for n,h in x['inputs'].items()}.items(),backups=lambda:copy.deepcopy(anchor['backup_sha256']),budgets=lambda:copy.deepcopy(budget),docker=lambda *a:calls.append(a),write=write)
    with self.assertRaisesRegex(ValueError,'backup/budget anchor drift'):m.retained(p1)
    aid=str(uuid.uuid4())
    with patch.object(m,'ROOT',root),patch.object(m,'module',return_value=p1):self.assertFalse(m.run(aid))
    terminal=load(root/'docs/acceptance/evidence/issue307-d1/post-v22-dispatch-audit'/aid/'terminal.json')
    self.assertEqual(terminal['status'],'BLOCKED_OR_UNCONFIRMED');self.assertIn('budget anchor drift',terminal['failure']);self.assertEqual(calls,[]);self.assertEqual(terminal['source_stopped_PID0'],'NOT_STARTED')
if __name__=='__main__':unittest.main(verbosity=2)
