"""File/Git hashes and docker inspect only; no retained process is started."""
import hashlib,json,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parents[5];out=Path(__file__).parent
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
import fixture_artifacts as f
spec=json.loads((root/f.SPEC).read_bytes());raw=f.runtime_archive(root,spec['source']);members=f.archive_members(raw)
assert f.digest(raw)==spec['archive_sha256'] and len(raw)==spec['archive_bytes'] and {n:f.digest(v) for n,v in members.items()}==spec['files']
assert not f.git(root,'diff','1d3e417bd059aaaee099161cf6e12b4c43fed81c','--',f.SPEC).strip()
packet=json.loads((root/'docs/acceptance/evidence/issue307-d1/remaining-candidate22/candidate.json').read_bytes())
ids=sorted({g[key] for g in packet['Groups'] for key in ('Core','Native')})
states=json.loads(subprocess.check_output(['docker','inspect',*ids]))
for row in states:assert not row['State']['Running'] and row['State']['Pid']==0
public=[dict(Id=row['Id'],Image=row['Image'],State=row['State'],PortBindings=row['HostConfig']['PortBindings'],RestartPolicy=row['HostConfig']['RestartPolicy']) for row in states]
with (out/'retained-sources-inspect01.json').open('x',encoding='utf-8') as stream:json.dump(public,stream,indent=2)
value=dict(status='PASS_READONLY_UNCHANGED_R4_AND_SOURCES_CLOSED',runtime_source=spec['source'],archive_sha256=f.digest(raw),archive_bytes=len(raw),archive_members=len(members),source_cids=ids,retained_source_start=0,provider_io=0,budget_reset=0,pin_release=0,business_protection_write=0,delete=0,scope='Docker inspect only; does not claim fresh native/provider or business acceptance')
with (out/'preservation-readonly-result01.json').open('x',encoding='utf-8',newline='\n') as stream:json.dump(value,stream,indent=2)
print(json.dumps(value))
