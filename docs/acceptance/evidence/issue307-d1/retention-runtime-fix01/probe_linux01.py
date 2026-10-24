import datetime,hashlib,io,json,os,subprocess,sys,tarfile,uuid,zipfile
from pathlib import Path
root=Path(__file__).resolve().parents[5]
sys.path.insert(0,str(root/'examples/lifecycle-d1'))
import retention_staging as staging
import fixture_artifacts as artifacts
out=Path(__file__).parent
private=Path(r'C:\Users\Henry\.codex\fixture-private\retention-runtime-fix01')
image='sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8'
sources=['f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce','66c17878149ff1959339bc60e319f3b0494786e62da33b77017bfc8aab13613d']
sequence=0

def run(name,args,*,data=None,cwd=root,env=None,timeout=120):
 global sequence
 sequence+=1;prefix=f'probe-{sequence:02d}-{name}'
 at=datetime.datetime.now(datetime.timezone.utc).isoformat()
 r=subprocess.run(args,input=data,cwd=cwd,env=env,capture_output=True,timeout=timeout)
 for suffix,raw in [('stdout',r.stdout),('stderr',r.stderr)]:
  with (out/(prefix+'.'+suffix)).open('xb') as f:f.write(raw)
 with (out/(prefix+'.json')).open('x',encoding='utf-8') as f:json.dump(dict(command=args,started_at=at,finished_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),returncode=r.returncode),f,indent=2)
 if r.returncode:raise RuntimeError(prefix+' failed: '+r.stderr.decode('utf-8','replace')[:300])
 return r.stdout

def inspect(name):return json.loads(run('inspect',['docker','inspect',name]))[0]

before={cid:inspect(cid)['State'] for cid in sources}
allocated=[];cid=None;failure=None
try:
 spec=json.loads((root/artifacts.SPEC).read_bytes());raw=(private/'approved-runtime-source.zip').read_bytes()
 if hashlib.sha256(raw).hexdigest()!=spec['archive_sha256']:raise ValueError('runtime zip differs')
 build=private/'linux-probe-source';build.mkdir()
 for name,content in artifacts.archive_members(raw).items():
  p=build/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(content)
 env=os.environ.copy();env.update(GOWORK='off',GOFLAGS='',CGO_ENABLED='0',GOOS='linux',GOARCH='amd64')
 for key in ['POSTGRES_DSN','TEST_POSTGRES_DSN','REDIS_ADDR']:env.pop(key,None)
 binary=private/'retention-test-linux'
 run('compile',['go','test','-c','-o',str(binary),'./cmd/lifecycle-fixture-retention'],cwd=build/'apps/platform',env=env,timeout=180)
 views={}
 for role in ['private','output','budget']:
  name='d1-retention-probe-'+role+'-'+uuid.uuid4().hex[:12]
  run('volume-create',['docker','volume','create','--label','dpp.retention-maintenance=preparation-only',name]);allocated.append(name)
  metadata=json.loads(run('volume-inspect',['docker','volume','inspect',name]))[0]
  linux,windows=staging.paths(metadata)
  # Exactly one new, empty volume; no old directory may be repaired.
  listing=run('empty',['wsl','-d','docker-desktop','--','ls','-A',linux])
  if listing.strip():raise ValueError('new volume not empty')
  content=io.BytesIO()
  with tarfile.open(fileobj=content,mode='w') as tar:
   directory=tarfile.TarInfo('.');directory.type=tarfile.DIRTYPE;directory.uid=directory.gid=10001;directory.mode=0o700;tar.addfile(directory)
   if role=='private':
    entry=tarfile.TarInfo('./control.json');entry.uid=entry.gid=10001;entry.mode=0o600;entry.size=2;tar.addfile(entry,io.BytesIO(b'{}'))
  run('initialize-new',['wsl','-d','docker-desktop','--','tar','-xpf','-','-C',linux],data=content.getvalue())
  def call(*args):return run('linux-snapshot',['wsl','-d','docker-desktop','--',*args])
  views[role]=dict(metadata=metadata,linux=linux,windows=windows,before=staging.validate_directory(metadata,windows,files={'control.json'} if role=='private' else set(),call=call))
 args=['docker','create','--name','d1-retention-probe-'+uuid.uuid4().hex[:12],'--network','none','--read-only','--user','10001:10001','--cap-drop','ALL','--security-opt','no-new-privileges:true','--restart','no','--tmpfs','/tmp:rw,noexec,nosuid,size=32m','--env','D1_RETENTION_VOLUME_PROBE=1']
 for role in ['private','output','budget']:
  args+=['--mount','type=volume,src='+views[role]['metadata']['Name']+',dst=/run/'+role+(',readonly' if role=='private' else '')]
 args+=['--mount','type=bind,src='+str(binary)+',dst=/runtime/test,readonly','--mount','type=bind,src='+str(build/'apps/platform/cmd/lifecycle-fixture-retention/testdata')+',dst=/testdata,readonly',image,'/runtime/test','-test.v','-test.count=1']
 cid=run('create',args).decode().strip();run('start-test',['docker','start','-a',cid],timeout=120)
 item=inspect(cid)
 if item['State']['Running'] or item['State']['Pid'] or item['State']['ExitCode'] or item['NetworkSettings']['Ports'] or item['HostConfig']['NetworkMode']!='none':raise ValueError('isolated probe not closed')
 for role in ['private','output','budget']:
  views[role]['after']=staging.validate_directory(views[role]['metadata'],views[role]['windows'],call=call)
  mount=next(m for m in item['Mounts'] if m['Destination']=='/run/'+role)
  if mount['Name']!=views[role]['metadata']['Name'] or mount['Source']!=views[role]['metadata']['Mountpoint'] or mount['Type']!='volume' or mount['RW']!=(role!='private'):raise ValueError('mount identity differs')
 # Windows control update keeps existing Linux custody; it cannot rely on UNC modes.
 control=Path(views['private']['windows'])/'control.json';control.write_bytes(b'{"test_only":true}')
 views['private']['after_host_update']=staging.validate_directory(views['private']['metadata'],views['private']['windows'],files={'control.json'},call=call)
except Exception as error:
 failure=dict(type=type(error).__name__,detail=str(error));raise
finally:
 if cid:
  try:
   item=inspect(cid)
   if item['State']['Running']:run('stop-test',['docker','stop','--time','5',cid])
  except Exception as close_error:
   failure=failure or dict(type=type(close_error).__name__,detail=str(close_error))
 after={source:inspect(source)['State'] for source in sources}
 if after!=before:failure=failure or dict(type='SourceDrift',detail='retained native State changed')
 result=dict(status='PASS_ISOLATED_LINUX_CUSTODY_FSYNC_WAIT_TESTS' if failure is None else 'FAILED_ISOLATED_PROBE_PRESERVED',failure=failure,cid=cid,volumes=allocated,views=locals().get('views',{}),source_states_unchanged=after==before,retained_source_start=0,retained_provider_io=0,retention_write=0,delete=0,cleanup=0)
 with (out/'linux-probe01.json').open('x',encoding='utf-8') as f:json.dump(result,f,indent=2)
 print(json.dumps({k:v for k,v in result.items() if k!='views'}),flush=True)
