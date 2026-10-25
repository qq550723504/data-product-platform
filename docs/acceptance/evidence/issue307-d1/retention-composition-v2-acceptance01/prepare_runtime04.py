"""Local immutable R successor only. No remote/GitHub/Docker/provider action."""
import datetime,hashlib,json,os,subprocess,sys
from pathlib import Path
root=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
import fixture_artifacts as artifacts
out=Path(__file__).parent
private=Path.home()/'.codex/fixture-private/retention-composition-v2-acceptance01'
private.mkdir(exist_ok=False)
base='53f6f8a6e69c17fa6896a40293d323f6ee9d0958'
expected_p='f4e25a88f62da6bb14ed24da8b71b634be43f17a'

def git(*args,data=None,env=None):
 result=subprocess.run(['git','-c','core.longpaths=true','-c','core.autocrlf=false',*args],input=data,cwd=root,env=env,capture_output=True)
 if result.returncode:raise RuntimeError('git local artifact operation failed: '+result.stderr.decode('utf-8','replace'))
 return result.stdout

if git('rev-parse','HEAD').decode().strip()!=expected_p:raise ValueError('published baseline drift')
spec=json.loads((root/artifacts.SPEC).read_bytes())
if spec['source']!=base:raise ValueError('runtime source drift')
changed=git('diff','--name-only','HEAD','--','apps/platform').decode().splitlines()
untracked=git('ls-files','--others','--exclude-standard','--','apps/platform').decode().splitlines()
delta=sorted(changed+untracked)
if not delta or not all(name.startswith(('apps/platform/cmd/lifecycle-fixture-reader/','apps/platform/cmd/lifecycle-fixture-readonly-check/','apps/platform/cmd/lifecycle-fixture-retention/','apps/platform/internal/lifecycle/pilot/retention_maintenance.go')) for name in delta):raise ValueError('runtime delta escaped review scope')
env=os.environ.copy();env['GIT_INDEX_FILE']=str(private/'runtime-index')
git('read-tree',base,env=env)
for name in delta:
 raw=(root/name).read_bytes().replace(b'\r\n',b'\n');oid=git('hash-object','-w','--stdin',data=raw).decode().strip()
 git('update-index','--add','--cacheinfo','100644,'+oid+','+name,env=env)
tree=git('write-tree',env=env).decode().strip()
source=git('commit-tree',tree,'-p',base,'-m','fix(lifecycle): bind retention V2 date and trusted wait readiness in retained runtime').decode().strip()
git('update-ref','refs/heads/codex/issue307-retention-runtime-v2',source,'0'*40)
archive=artifacts.runtime_archive(root,source)
with (private/'approved-runtime-source.zip').open('xb') as stream:stream.write(archive)
files=artifacts.archive_members(archive)
spec.update(source=source,base_source=base,runtime_delta_files=delta,archive_sha256=artifacts.digest(archive),archive_bytes=len(archive),files={name:artifacts.digest(raw) for name,raw in files.items()})
spec['shared_contract_files']=sorted(set(spec['shared_contract_files'])|set(delta))
(root/artifacts.SPEC).write_bytes((json.dumps(spec,ensure_ascii=False,indent=2)+'\n').encode())
result=dict(status='PREPARED_LOCAL_R4_NOT_PUBLISHED',source=source,parent=base,tree=tree,archive_sha256=spec['archive_sha256'],archive_bytes=len(archive),members=len(files),runtime_delta_files=delta,
 original_p=expected_p,execution='NOT_CREATED',maintenance_authority='NOT_GRANTED',source_start=0,provider_io=0,protection_write=0,delete=0,at=datetime.datetime.now(datetime.timezone.utc).isoformat())
with (out/'runtime-artifact04.json').open('x',encoding='utf-8') as stream:json.dump(result,stream,indent=2)
print(json.dumps(result))
