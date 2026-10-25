"""Check actual retained/new credential bytes without printing their values."""
import hashlib
import json
import io
import zipfile
from pathlib import Path
from urllib.parse import urlsplit

ROOT=Path(__file__).resolve().parents[5]
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
HERE=ROOT/'docs/acceptance/evidence/issue307-d1/candidate23-planning01'
sources=[PRIVATE/name/'config.json' for name in ('dpp-lifecycle307-808a6960','dpp-lifecycle307-754ea8f0','dpp-lifecycle307-dfda43b4')]
controls=[PRIVATE/('remaining-candidate'+str(n)) for n in (20,21,22)]
for control in controls:
    sources+=list(control.rglob('operator.json'))+list(control.rglob('new-secrets.private.json'))+list(control.rglob('remaining.json'))
if not (controls[-1]/'topology-key.private').is_file():raise ValueError('Run current publication scan only after current freeze/key creation')
secrets=set()


def walk(value,key=''):
    if isinstance(value,dict):
        for k,v in value.items():walk(v,k)
    elif isinstance(value,list):
        for x in value:walk(x,key)
    elif isinstance(value,str):
        if any(k in key.lower() for k in ('secret','password','token','privatekey')) and len(value)>=12:secrets.add(value.encode())
        if key=='PostgresDSN':
            password=urlsplit(value).password
            if password and len(password)>=12:secrets.add(password.encode())


for p in sources:
    value=json.loads(p.read_bytes());walk(value)
    if p.name=='new-secrets.private.json':secrets.update(v.encode() for v in value.values() if isinstance(v,str) and len(v)>=12)
for control in controls:
    p=control/'topology-key.private'
    if p.exists():
        raw=p.read_bytes();secrets.update((raw,raw.hex().encode()))
    for p in control.glob('*/checkpoint/postgres.env'):
        for line in p.read_text(encoding='utf-8').splitlines():
            if line.startswith('POSTGRES_PASSWORD='):secrets.add(line.split('=',1)[1].encode())
# The only new generation's non-secret public preparation/evidence/docs.
base=ROOT/'docs/acceptance/evidence/issue307-d1'
paths=[]
for name in [p.name for p in base.iterdir() if p.is_dir() and (p.name.startswith(('remaining-candidate22','candidate22-','candidate21-','remaining-candidate21','candidate23-')))]:
    folder=base/name
    if folder.exists():paths.extend(p for p in folder.rglob('*') if p.is_file())
paths.extend(p for p in (ROOT/'docs/acceptance').glob('issue307-d1-*') if p.is_file() and ('candidate21' in p.name or 'v31' in p.name or 'candidate22' in p.name or 'v32' in p.name or 'v33' in p.name))
findings=[];archive_members_scanned=0
for path in paths:
    raw=path.read_bytes()
    if any(secret in raw for secret in secrets):findings.append(path.relative_to(ROOT).as_posix())
    if zipfile.is_zipfile(io.BytesIO(raw)):
        with zipfile.ZipFile(io.BytesIO(raw)) as archive:
            infos=archive.infolist()
            if len(infos)>10000 or sum(x.file_size for x in infos)>128*1024*1024:raise ValueError('Publication archive exceeds bounded inspection')
            for item in infos:
                if item.is_dir():continue
                value=archive.read(item);archive_members_scanned+=1
                if any(secret in value for secret in secrets):findings.append(path.relative_to(ROOT).as_posix()+'!/'+item.filename)
result={'status':'PASS_NO_ACTUAL_CREDENTIAL_BYTES' if not findings else 'FAIL_ACTUAL_CREDENTIAL_BYTES',
        'files_scanned':len(paths),'archive_members_scanned':archive_members_scanned,'secret_values_compared':len(secrets),'findings':findings,
        'source_files':len(sources),'native_io':0,'execution_authority':False}
out=HERE/'publication-secret-scan01.json'
with out.open('xb') as f:f.write((json.dumps(result,indent=2)+'\n').encode())
print(json.dumps(result))
if findings:raise ValueError('Actual private credential bytes in publication; values suppressed')
