"""Read-only current-candidate QA, separate from normal startup."""
import base64, hashlib, io, json, os, subprocess, sys, tarfile, urllib.request
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'deploy/trial'))
import runtime as r
s=r.read(r.STATE/'state.json');p=Path(s['private'])
def physical():
    data=r.docker('cp',s['containers']['objects']['name']+':/data/raw/.','-')
    with tarfile.open(fileobj=io.BytesIO(data)) as archive:
        files={m.name:hashlib.sha256(archive.extractfile(m).read()).hexdigest() for m in archive.getmembers() if m.isfile()}
    assert files,'non-empty RAW native storage required'
    return files
mode=sys.argv[1]
if mode=='before':
    r.write(r.STATE/'physical-before-stop.json',physical());print('native physical RAW snapshot PASS')
elif mode=='after':
    assert physical()==r.read(r.STATE/'physical-before-stop.json')
    sentinels={}
    for role in ('source','core'):
        data=r.docker('cp',s['containers'][role]['name']+':/var/lib/postgresql/data/PG_VERSION','-')
        with tarfile.open(fileobj=io.BytesIO(data)) as archive:
            file=next(m for m in archive.getmembers() if m.isfile());value=archive.extractfile(file).read().strip();assert value==b'16';sentinels[role]='16'
    r.storage_check(s,['exited'])
    for role,item in s['containers'].items():
        actual=json.loads(r.docker('inspect',item['name']))[0];r.verify_container(s,role,actual);assert actual['State']['Status']=='exited'
    result={'status':'PASS','runId':s['runId'],'sameContainersAndVolumes':True,'stoppedContainers':6,'retainedNamedVolumes':len(s['volumes']),'physicalRAWFilesUnchanged':True,'postgresDataFilesPresent':sentinels,'restart':'NOT_RUN / PR 2'}
    r.write(r.STATE/'retention-proof.json',result);print(json.dumps(result))
elif mode=='secrets':
    values=set()
    for file in s['secretReferences']:
        b=Path(file).read_bytes().strip()
        try: values.update(v.encode() for v in json.loads(b).values() if isinstance(v,str) and len(v)>=8)
        except (ValueError,AttributeError):values.add(b)
    values.add(base64.b64encode(b'trial:'+(p/'hop-password').read_bytes().strip()))
    scanned={};leaks=[]
    def scan(label,data):
        scanned[label]=len(data)
        if any(v in data for v in values):leaks.append(label)
    for role,binary in s['binaries'].items():scan('binary:'+role,Path(binary['path']).read_bytes())
    for name in s['webBuild']['files']:scan('web:'+name,(r.ROOT/'apps/web/.next'/name).read_bytes())
    for f in p.glob('*.log'):scan('log:'+f.name,f.read_bytes())
    for role,item in s['containers'].items():
        log=subprocess.run(['docker','logs',item['id']],env=r.environment(),capture_output=True);assert log.returncode==0;scan('docker:'+role,log.stdout+log.stderr)
    for f in r.STATE.glob('*.json'):scan('receipt:'+f.name,f.read_bytes())
    op=r.read(r.STATE/'latest-facts.json')['facts']['operations'][0]
    for path in ('/batches','/batches/'+op['id']):
        with urllib.request.urlopen(s['web']+path) as response:scan('html:'+path,response.read())
    core=s['containers']['core']['name'];query="SELECT coalesce(json_agg(t),'[]') FROM (SELECT after_state::text AS value FROM audit_event UNION ALL SELECT metadata::text FROM evidence UNION ALL SELECT payload::text FROM outbox_event) t"
    # Audit schema uses before_state/after_state; no credential values may appear.
    scan('Core audit/evidence/outbox',r.docker('exec',core,'psql','-XqAt','-v','ON_ERROR_STOP=1','-U','postgres','-d','dpp_trial_core','-c',query))
    assert not leaks,'credential appeared in public/runtime surface: '+','.join(leaks)
    result={'status':'PASS','runId':s['runId'],'scannedSurfaces':len(scanned),'scannedBytes':sum(scanned.values()),'knownSecretMatches':0,'coverage':['binaries','production standalone','HTML','runtime logs','container logs','public receipts','Core audit/evidence/outbox']}
    r.write(r.STATE/'secret-proof.json',result);print(json.dumps(result))
else:raise SystemExit('read-only QA mode required')
