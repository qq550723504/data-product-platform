"""Native tar/restore of only this run's stopped object, queue and Hop volumes."""
import hashlib,json,subprocess,sys,time,urllib.request
from pathlib import Path
root=Path(__file__).resolve().parent
repo=root.parents[1]
state=json.loads((repo/'.data/independent299-runtime/runtime.json').read_text())
private=Path(state['private']); backup=private/'backup-volumes-final';backup.mkdir(exist_ok=False)
suffix=state['run_id'].replace('-','')[:12]
alpine='alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8'
report={'runId':state['run_id'],'status':'RUNNING','volumes':[]}
def run(args):
 r=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 if r.returncode:
  (backup/'failure.log').write_bytes(r.stdout+r.stderr)
  raise RuntimeError('volume backup command failed; see private log')
 return r.stdout
def helper(label,mounts,args):
 name='dpp-ind299-'+label+'-'+suffix
 run(['docker','create','--name',name,'--label','dpp.ind299.run='+state['run_id'],*mounts,alpine,*args])
 return run(['docker','start','--attach',name])
containers=json.loads(run(['docker','inspect',*state['containers']]))
selected=[]
for item in containers:
 assert item['Config']['Labels']['dpp.trial.run']==state['run_id']
 assert item['State']['Status']=='exited','final backup requires stopped native services'
 name=item['Name'].lstrip('/')
 if 'storage-' in name or 'redis-' in name or name==state['hop']:
  for mount in item['Mounts']:
   if mount['Type']=='volume':selected.append((item,mount))
for index,(item,mount) in enumerate(selected):
 original=mount['Name']; restored='dpp-ind299-restored-'+suffix+'-'+str(index)
 exists=subprocess.run(['docker','volume','inspect',restored],capture_output=True)
 assert exists.returncode!=0,'restore volume already exists'
 run(['docker','volume','create','--label','dpp.ind299.run='+state['run_id'],restored])
 archive=backup/(str(index)+'.tar')
 helper('backup-'+str(index),['--mount',f'type=volume,src={original},dst=/data,readonly','--mount',f'type=bind,src={backup},dst=/backup'],['tar','-C','/data','-cf','/backup/'+archive.name,'.'])
 helper('restore-'+str(index),['--mount',f'type=volume,src={restored},dst=/data','--mount',f'type=bind,src={backup},dst=/backup,readonly'],['tar','-C','/data','-xf','/backup/'+archive.name])
 hashes=[]
 for phase,volume in [('source',original),('restored',restored)]:
  value=helper('fingerprint-'+phase+'-'+str(index),['--mount',f'type=volume,src={volume},dst=/data,readonly'],['sh','-c','cd /data && find . -type f -exec sha256sum {} + | LC_ALL=C sort'])
  (backup/(str(index)+'-'+phase+'-files.txt')).write_bytes(value)
  hashes.append(hashlib.sha256(value).hexdigest())
 assert hashes[0]==hashes[1],'restored volume file content mismatch'
 report['volumes'].append({'source':original,'restored':restored,'path':mount['Destination'],'container':item['Name'],'archiveSha256':hashlib.sha256(archive.read_bytes()).hexdigest(),'fileManifestSha256':hashes[0],'status':'PASS'})
objects=next(v for v in report['volumes'] if 'storage-' in v['source'])
original=next(c for c in containers if 'storage-' in c['Name'])
name='dpp-ind299-restored-minio-'+suffix
run(['docker','create','--name',name,'--label','dpp.ind299.run='+state['run_id'],'--network',state['networks'][0],'-p','127.0.0.1::9000','--mount',f"type=volume,src={objects['restored']},dst=/data",'--mount',f'type=bind,src={private / "storage"},dst=/run/credentials,readonly','--entrypoint','sh',original['Image'],'/run/credentials/start.sh'])
run(['docker','start',name])
item=json.loads(run(['docker','inspect',name]))[0]
endpoint='127.0.0.1:'+item['NetworkSettings']['Ports']['9000/tcp'][0]['HostPort']
for _ in range(40):
 try:
  urllib.request.urlopen('http://'+endpoint+'/minio/health/live',timeout=2);break
 except Exception:time.sleep(.5)
else:raise RuntimeError('restored native MinIO unavailable')
import os
env=dict(os.environ,TEST_INGESTION_B3_CONFIG=str(private/'test.json'))
r=subprocess.run([str(root/'object-manifest.exe'),'-endpoint',endpoint,'-output',str(root/'objects-restored.json')],env=env,capture_output=True)
assert r.returncode==0,'restored native object read failed'
assert json.loads((root/'objects-current.json').read_text())==json.loads((root/'objects-restored.json').read_text()),'native version/size/payload mismatch'
queue=next(v for v in report['volumes'] if 'redis-' in v['source'])
original=next(c for c in containers if 'redis-' in c['Name'])
name='dpp-ind299-restored-redis-'+suffix
run(['docker','create','--name',name,'--label','dpp.ind299.run='+state['run_id'],'--network',state['networks'][0],'--mount',f"type=volume,src={queue['restored']},dst=/data",original['Image'],'redis-server','--dir','/data','--appendonly','yes','--appendfsync','always'])
run(['docker','start',name]);time.sleep(.5)
assert run(['docker','exec',name,'redis-cli','PING']).strip()==b'PONG'
report.update(status='PASS',nativeObjects=len(json.loads((root/'objects-restored.json').read_text())),nativeRedis='PONG',backupPrivate=str(backup),cleanup=0,providerDelete=0)
(root/'volumes-backup-final.json').write_text(json.dumps(report,indent=2))
print(json.dumps({'status':'PASS','volumes':len(report['volumes']),'nativeObjects':report['nativeObjects'],'redis':'PONG','providerDelete':0}))
