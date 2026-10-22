import hashlib,json,subprocess
from pathlib import Path
root=Path(__file__).resolve().parent
repo=root.parents[1]
state=json.loads((repo/'.data/independent299-runtime/runtime.json').read_text())
def cmd(args):return subprocess.check_output(args,cwd=repo)
paths=cmd(['git','ls-files','apps/platform','apps/web','migrations','industry-packs','examples/enterprise-activity/postgres-ingestion']).decode().splitlines()
paths=[p for p in paths if '/results/' not in p]
sources={p:hashlib.sha256((repo/p).read_bytes().replace(b'\r\n',b'\n')).hexdigest() for p in paths}
baseline={p:hashlib.sha256(cmd(['git','show','HEAD:'+p]).replace(b'\r\n',b'\n')).hexdigest() for p in paths}
assert sources==baseline,'tracked source differs from selected main'
names=state['containers']+['dpp-pgi09-ind299-20261014-'+x for x in ['postgresql','elasticsearch','execute-migrate-all','openmetadata-server']]
containers=json.loads(cmd(['docker','inspect',*names]))
images={}
mounts={}
for c in containers:
 image=json.loads(cmd(['docker','image','inspect',c['Image']]))[0]
 images[c['Name'].lstrip('/') ]={'imageId':c['Image'],'tag':c['Config']['Image'],'repoDigests':image.get('RepoDigests',[])}
 mounts[c['Name'].lstrip('/')]=[{'type':m['Type'],'name':m.get('Name'),'destination':m['Destination'],'writable':m['RW']} for m in c['Mounts']]
binary={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (repo/'.data').glob('pgi-b3-independent-*.exe')}
reports={p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in (repo/'docs/acceptance').glob('issue299-c*.md')}
build=repo/'apps/web/.next-pgi-b3-independent'
web={str(p.relative_to(build)).replace('\\','/'):hashlib.sha256(p.read_bytes()).hexdigest() for p in build.rglob('*') if p.is_file() and 'cache' not in p.parts}
report={'head':cmd(['git','rev-parse','HEAD']).decode().strip(),'tree':cmd(['git','rev-parse','HEAD^{tree}']).decode().strip(),
 'runId':state['run_id'],'go':cmd(['go','version']).decode().strip(),'sources':sources,'binaries':binary,'images':images,'mounts':mounts,'authoritativeReports':reports,'webBuild':web,
 'launcherSha256':hashlib.sha256((root/'launch.py').read_bytes()).hexdigest(),'oldCoreIncident':'FAIL / NOT_RESTORED'}
(root/'identity.json').write_text(json.dumps(report,indent=2),encoding='utf-8')
print(json.dumps({'sourceFiles':len(sources),'binaries':binary,'images':len(images),'webFiles':len(web),'head':report['head']}))
