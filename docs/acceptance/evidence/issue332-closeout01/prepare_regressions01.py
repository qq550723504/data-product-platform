"""Prepare independent test copies and private raw evidence, no source starts."""
import hashlib,json,subprocess
from pathlib import Path
root=Path(__file__).resolve().parents[4];out=Path(__file__).parent
private=Path('C:/Users/Henry/.codex/fixture-private/merge-closeout01');private.mkdir(exist_ok=False)
snapshot=private/'python-export01';snapshot.mkdir()
old=root/'docs/acceptance/evidence/issue307-d1/reader-path-fix01'
manifest=json.loads((old/'independent-export-manifest01.json').read_bytes());files={}
for name in manifest['files']:
    if name in ('offline_python_runner01.py','run_python_collection01.py'):source=old/name
    elif name=='collection.json':source=Path('C:/Users/Henry/.codex/fixture-private/reader-path-fix01/python-export01/collection.json')
    else:source=root/name
    raw=source.read_bytes();target=snapshot/name;target.parent.mkdir(parents=True,exist_ok=True)
    with target.open('xb') as stream:stream.write(raw)
    files[name]=dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
with (private/'snapshot-index01.json').open('x',encoding='utf-8',newline='\n') as stream:json.dump(dict(format='files-map-v1',count=len(files),files=files),stream,indent=2)
with (out/'independent-snapshot-binding01.json').open('x',encoding='utf-8',newline='\n') as stream:
    json.dump(dict(format='files-map-v1',root=str(snapshot),count=len(files),files=files,raw_outputs='private; copied summaries/index only'),stream,indent=2)
linux=private/'run_linux01.py'
linux.write_text('''import json,subprocess,sys,uuid
from pathlib import Path
root=Path(r"C:/Users/Henry/.codex/worktrees/d1-merge-closeout/data-product-platform")
private=Path(__file__).parent
sys.path.insert(0,str(root/'examples'))
import temporary_containers as helpers
registry=helpers.TemporaryContainers(private/'helper-archives')
output=private/'linux-output01';output.mkdir()
created=registry.create(str(uuid.uuid4()),['--name','dpp307-merge-closeout-python313-01','--network','none','--read-only','--user','10001:10001','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp:rw,nosuid,nodev,size=256m',
    '--mount','type=bind,source='+str(private/'python-export01')+',target=/work,readonly','--mount','type=bind,source='+str(output)+',target=/out',
    'dpp307-reader-path-python:31316','python','-B','/work/run_python_collection01.py','/out/collection01'])
cid=created['id']
with (private/'linux-helper-created01.json').open('x') as f:json.dump(created,f,indent=2)
run=subprocess.run(['docker','start','-a',cid],capture_output=True,timeout=400)
for name,raw in [('stdout',run.stdout),('stderr',run.stderr)]:
    with (private/('linux-start.'+name)).open('xb') as f:f.write(raw)
with (private/'linux-start-result01.json').open('x') as f:json.dump(dict(returncode=run.returncode,id=cid),f,indent=2)
if run.returncode:raise SystemExit(run.returncode)
closed=registry.complete(created['ticket'],expected=0)
with (private/'linux-helper-closed01.json').open('x') as f:json.dump(closed,f,indent=2)
print(json.dumps(closed))
''',encoding='utf-8')
print(json.dumps(dict(snapshot_members=len(files),new_business_execution=0,new_candidate=0,new_window=0)))
