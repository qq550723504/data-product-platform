"""Wire the newly selected preparation only; never call Docker or a provider."""
from pathlib import Path
import hashlib
import json
import subprocess

ROOT=Path(__file__).resolve().parents[5]
HERE=Path(__file__).parent
intent=json.loads((HERE/'planning-intent.json').read_bytes())
old='a5d1e569-f80e-4b38-8f32-d8139a30a077'
new=intent['execution']
if subprocess.check_output(['git','-c','core.longpaths=true','rev-parse','HEAD'],cwd=ROOT).decode().strip()!=intent['root_fix_head']:
    raise ValueError('repair source HEAD differs')

changes={
 'apps/platform/internal/lifecycle/pilot/remaining_scope.go':[
  (old,new),('candidate19/v29','candidate20/v30'),
  ('2026-10-22T06:00:00Z','2026-10-22T10:00:00Z'),
  ('2026-10-22T06:40:00Z','2026-10-22T10:40:00Z')],
 'examples/lifecycle-d1/prepare-six-candidate.py':[
  ('Finite candidate19 preparation.','Finite candidate20 preparation.'),
  ("PROFILE=='candidate19/v29'","PROFILE=='candidate20/v30'"),(old,new),
  ("str(215+n)","str(218+n)")],
 'examples/lifecycle-d1/six_candidate_runtime.py':[
  ('primitives for candidate19.','primitives for candidate20.')],
 'examples/lifecycle-d1/freeze-six-candidate.py':[
  ('Freeze candidate19 only','Freeze candidate20 only'),
  ('candidate19-source-review01','candidate20-source-review01'),
  ('PASS_BOUNDED_C19_SOURCE_PREPARATION_REVIEW','PASS_BOUNDED_C20_SOURCE_PREPARATION_REVIEW'),
  ('candidate13/v23 through candidate18/v28;','candidate13/v23 through candidate19/v29;')],
 'examples/lifecycle-d1/window_six_support.py':[
  ('Finite candidate19 host','Finite candidate20 host'),
  ('delete-confirmed-v29-execution01','delete-confirmed-v30-execution01')],
 'examples/lifecycle-d1/test_retained_v26.py':[
  ('candidate19/v29','candidate20/v30'),(old,new),
  ('2026-10-22T06:00:00Z','2026-10-22T10:00:00Z'),
  ('2026-10-22T06:40:00Z','2026-10-22T10:40:00Z'),
  ('2026-10-22T08:40:00Z','2026-10-22T12:40:00Z'),
  ('candidate19-source-review01','candidate20-source-review01'),
  ('C19_SOURCE','C20_SOURCE')],
 'examples/lifecycle-d1/test_retained_v27.py':[(old,new)],
 'examples/lifecycle-d1/test_freeze_six_candidate.py':[
  ('2026-10-22 14:00–14:40','2026-10-22 18:00–18:40')],
}
receipt=[]
for name,replacements in changes.items():
 path=ROOT/name
 before=raw=path.read_bytes()
 for a,b in replacements:
  if a.encode() not in raw:raise ValueError('expected old contract missing: '+name+' '+a)
  raw=raw.replace(a.encode(),b.encode())
 path.write_bytes(raw)
 receipt.append(dict(path=name,before_sha256=hashlib.sha256(before).hexdigest(),after_sha256=hashlib.sha256(raw).hexdigest()))
with (HERE/'source-wiring01.json').open('x',encoding='utf-8') as stream:
 json.dump(dict(status='NEW_UNFROZEN_SOURCE_WIRED_ONLY',execution=new,files=receipt,native_delete=0,provider_io=0,old_authorization_reused=False),stream,indent=2)
print(new)
