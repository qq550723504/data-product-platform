"""Check actual retained/new credential bytes without printing their values."""
import hashlib
import json
from pathlib import Path
from urllib.parse import urlsplit

ROOT=Path(__file__).resolve().parents[5]
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
HERE=Path(__file__).parent
sources=[PRIVATE/name/'config.json' for name in ('dpp-lifecycle307-808a6960','dpp-lifecycle307-754ea8f0','dpp-lifecycle307-dfda43b4')]
control=PRIVATE/'remaining-candidate21'
sources+=list(control.rglob('operator.json'))+list(control.rglob('new-secrets.private.json'))+list(control.rglob('remaining.json'))
sources+=list((PRIVATE/'remaining-candidate20').rglob('operator.json'))+list((PRIVATE/'remaining-candidate20').rglob('new-secrets.private.json'))
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
    walk(json.loads(p.read_bytes()))
for p in [control/'topology-key.private']:
    raw=p.read_bytes();secrets.update((raw,raw.hex().encode()))
for p in control.glob('*/checkpoint/postgres.env'):
    for line in p.read_text(encoding='utf-8').splitlines():
        if line.startswith('POSTGRES_PASSWORD='):
            secrets.add(line.split('=',1)[1].encode())
# The only new generation's non-secret public preparation/evidence/docs.
base=ROOT/'docs/acceptance/evidence/issue307-d1'
paths=[]
for name in ('remaining-candidate21','remaining-candidate21-preparation01','remaining-candidate21-readonly01','candidate21-source-review01','candidate21-artifacts-review01','candidate21-planning01','candidate21-publication01','candidate21-crosscheck01','candidate21-expiry01','candidate22-planning01'):
    folder=base/name
    if folder.exists():paths.extend(p for p in folder.rglob('*') if p.is_file())
paths.extend(p for p in (ROOT/'docs/acceptance').glob('issue307-d1-*') if p.is_file() and ('candidate21' in p.name or 'v31' in p.name or 'candidate22' in p.name or 'v32' in p.name))
findings=[]
for path in paths:
    raw=path.read_bytes()
    if any(secret in raw for secret in secrets):findings.append(path.relative_to(ROOT).as_posix())
result={'status':'PASS_NO_ACTUAL_CREDENTIAL_BYTES' if not findings else 'FAIL_ACTUAL_CREDENTIAL_BYTES',
        'files_scanned':len(paths),'secret_values_compared':len(secrets),'findings':findings,
        'source_files':len(sources),'native_io':0,'execution_authority':False}
out=HERE/'publication-secret-scan01.json'
with out.open('xb') as f:f.write((json.dumps(result,indent=2)+'\n').encode())
print(json.dumps(result))
if findings:raise ValueError('Actual private credential bytes in publication; values suppressed')
