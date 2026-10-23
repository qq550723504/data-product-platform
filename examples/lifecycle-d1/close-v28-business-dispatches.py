"""One reviewed zero-DELETE maintenance: P2 DENIED reconciliation, T1/T2 NOT_SENT.

Uses existing Commands, durable storage-call journal and physical containment.
No old authorization, IAM enable, refund, reader unpin or arbitrary target.
"""
import argparse,base64,datetime as dt,hashlib,importlib.util,json,os,secrets,shutil,subprocess,uuid
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
CONTROL=PRIVATE/'post-v28-business-closure01'
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/post-v28-business-closure01'
EVIDENCE=ROOT/'docs/acceptance/evidence/issue307-d1'
OLD=PRIVATE/'remaining-candidate18'
PACKET=EVIDENCE/'remaining-candidate18'
EXECUTION='4fd5a7e7-779e-41d7-a35a-3bab7dd615b5'
CANDIDATE='bb0924c1814b0bcdecd33d3b57a21a24beaafce2a3d64e10e5dac9aba1947f63'
BINDINGS='8d021af81d905cc6de255bf84e460fc0b416d5af5bacc8388cfbb2b423a60624'
HISTORY=EVIDENCE/'delete-confirmed-v28-execution01/window01'

def module(name,file):
 s=importlib.util.spec_from_file_location(name,Path(__file__).with_name(file));m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
c=module('v28_old_closure_reuse','close-old-business-dispatches.py');p=c.p1
load,sha,write,need,docker,inspect,closed,now=c.load,c.sha,c.write,c.need,c.docker,c.inspect,c.closed,c.now
p.CONTROL=CONTROL;c.CONTROL=CONTROL;c.PUBLIC=PUBLIC
n=p.native_module()
OLD_OWN=c.own_contract
ALLOWED=c.ALLOWED|{'lifecycle_storage_call','lifecycle_storage_observation','cost_event','cost_allocation'}

def packet():
 need(sha(PACKET/'candidate.json')==CANDIDATE and sha(PACKET/'bindings.json')==BINDINGS,'fixed failed packet drift')
 need(sha(HISTORY/'terminal.json')=='dfae9324deab1a693e4fa093a70b584c8ae9a60e5767332d63de371f18981ced','original failure changed')
 return load(PACKET/'candidate.json'),load(PACKET/'bindings.json')

def targets():return load(EVIDENCE/'post-v28-core-audit01/actual-v28-targets.json')

def inspect_many(ids):
 need(len(ids)==len(set(ids)) and all(len(x)==64 and all(c in '0123456789abcdef' for c in x) for x in ids),'fixed full CID set invalid')
 rows=[]
 for i in range(0,len(ids),64):rows+=json.loads(docker('inspect',*ids[i:i+64]))
 need(len(rows)==len(ids) and {x['Id'] for x in rows}==set(ids),'CID substitution');return rows

def known_ids():
 ids=[x['cid'] for x in load(HISTORY/'post-seal-readback.json')['owners']];need(len(ids)==len(set(ids))==715,'failed retained membership changed')
 for run in ('native02','native03'):
  a=load(EVIDENCE/'p2-http403-repair01'/run/'allocation.json');ids += [a['server'],a['runner']]
 for run in ('02','03'):
  ids += [x['cid'] for x in load(EVIDENCE/('post-v28-restored-core'+run)/'terminal.json')['closure']]
 return sorted(set(ids))

def physical(active=()):
 rows=inspect_many(known_ids());need(all(closed(x) or x['Id'] in active for x in rows),'retained owner active')
 candidate,bindings=packet();owners={}
 for g,b in zip(candidate['Groups'],bindings):
  folder=OLD/g['Group'];perms=list(folder.rglob('permission.json'));cases=list(folder.glob('case-*/remaining.json'));recover=list(folder.glob('recover-*/recovery.json'))
  need(all(load(f)['Enabled'] is False for f in perms+recover) and all(load(f)['Base']['Enabled'] is False for f in cases),'old control enabled')
  gateway=next(x for x in rows if x['Id']==g['Gateway']);m=next(x for x in gateway['Mounts'] if x['Destination']=='/etc/caddy/Caddyfile')
  need(closed(gateway) and not m['RW'] and sha(p.hostpath(m['Source']))==g['SealedGatewaySHA256'],'gateway unsealed')
  for cid,dest,digest in [(cid,'/run/operator',candidate['SenderBinarySHA256']) for cid in g['Helpers'].values()]+[(cid,'/run/reader',candidate['ReaderBinarySHA256']) for cid in g['CaseHelpers'].values()]+[(r['container'],'/run/owner',candidate['OwnerBinarySHA256']) for r in b['root_owners'].values()]:
   x=next(x for x in rows if x['Id']==cid);m=next(x for x in x['Mounts'] if x['Destination']==dest)
   need(closed(x) and not m['RW'] and sha(p.hostpath(m['Source']))==digest,'old executable changed');owners[cid]=digest
 need(len(owners)==36,'old executable membership differs')
 return owners,rows

def snapshot():
 owners,rows=physical();rv=module('v28_prior_history','retained_v27.py')
 prior=rv.verify(ROOT,inspect,closed,docker,set(),PRIVATE,p.hostpath)
 candidate,_=packet();allocation=load(OLD/'allocation.json');budgets={};backups={}
 accepted=rv.historical_descriptors.expected(ROOT)
 for g,a in zip(candidate['Groups'],allocation['groups']):
  name=a['volumes']['budget'];users=sorted(x['Id'] for x in rows if any(m['Type']=='volume' and m['Name']==name for m in x['Mounts']))
  actual=docker('ps','-aq','--no-trunc','--filter','volume='+name).decode('utf-8','strict').split();need(sorted(actual)==users and len(users)==len(set(actual)),'budget mount expansion')
  volume=json.loads(docker('volume','inspect',name))[0];need(volume['Labels']['lifecycle307.remaining']==EXECUTION and volume['Labels']['lifecycle307.allocation']==allocation['allocation_nonce'],'budget ownership drift')
  raw=docker('cp',g['Helpers']['wire']+':/run/budget','-');digest=hashlib.sha256(raw).hexdigest()
  expected=next(x['tar_sha256'] for x in load(HISTORY/'post-seal-readback.json')['budget'] if x['group']==g['Group'])
  need(digest==expected,'used budget bytes changed');budgets[name]=dict(sha256=digest,mounted_cids=users)
  for file,key in [('core.dump','CoreDumpSHA256'),('globals.sql','GlobalsSHA256'),('native.tar','NativeArchiveSHA256')]:
   path=OLD/g['Group']/'checkpoint'/file;need(sha(path)==g[key],'old backup changed');backups[str(path)]=sha(path)
 profiles={str(x):sha(x) for group in ('business-races','late-hold','mixed') for x in (OLD/group).rglob('*.json')}
 files={x.relative_to(ROOT).as_posix():sha(x) for x in HISTORY.rglob('*') if x.is_file()}
 return dict(prior=prior,owners=owners,descriptors={x['Id']:rv.cfg_hash(x,accepted) for x in rows},budgets=budgets,backups=backups,profiles=profiles,failed_evidence=files)

def code_inputs():
 names=p.call(['git','-c','core.longpaths=true','ls-files','apps/platform','examples/lifecycle-d1']).decode().splitlines()
 return {name:sha(ROOT/name) for name in names if name.endswith(('.go','.py','go.mod','go.sum'))}

def allocate(kind,binary,private,output,source,audit):
 output.mkdir();args=['post-v28-close'] if kind=='observer' else ['-action',kind]
 need(kind in ('observer','check-disabled','disable-all'),'no enable action')
 cid=p.text('create','--pull','never','--name','dpp307-v28-closure-'+audit[:8]+'-'+kind,'--label','lifecycle307.closure='+audit,'--network','container:'+source,'--user','10001:10001','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--restart','no','--pids-limit','64','--memory','256m','--cpus','1','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount','type=bind,source='+str(private)+',target=/run/private,readonly','--mount','type=bind,source='+str(binary)+',target=/run/executor,readonly','--mount','type=bind,source='+str(output)+',target=/run/output','--entrypoint','/run/executor',p.PG,*args)
 return dict(kind=kind,cid=cid,binary=str(binary),binary_sha256=sha(binary),private=str(private),output=str(output),network_source=source,arguments=args)

def own(owner,manifest):
 # Same mount/UID/capabilities contract; only the new maintenance role gets the
 # explicitly indexed inspection file. Old observer contracts stay unchanged.
 copy=dict(owner);copy['kind']='maintenance';x=OLD_OWN(copy,manifest)
 if owner['kind']=='observer':
  entries=list(Path(owner['private']).iterdir());allowed={'closure.json','inspection.json'}|({'proof.json'} if (Path(owner['private'])/'proof.json').exists() else set())
  need({p.name for p in entries}==allowed and all(p.is_file() and not p.is_symlink() and not p.is_junction() for p in entries),'maintenance private expansion')
 return x

def prepare():
 need(not p.call(['git','-c','core.longpaths=true','diff','HEAD','--name-only']).strip(),'commit implementation first')
 need(not CONTROL.exists() and not PUBLIC.exists(),'one shot already allocated')
 proof=load(EVIDENCE/'post-v28-restored-core03/terminal.json');need(proof['status']=='PASS_ACTUAL_SCHEMA_V28_MIXED_CLOSURE_AND_REPLAY','actual restored Command gate absent')
 CONTROL.mkdir();PUBLIC.mkdir(parents=True);candidate,bindings=packet();before=snapshot();g=candidate['Groups'][0]
 env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
 build=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(CONTROL/'observer'),'./cmd/lifecycle-fixture-old-dispatch'],cwd=ROOT/'apps/platform',env=env,capture_output=True)
 (CONTROL/'build.private').write_bytes(build.stdout+build.stderr);need(build.returncode==0 and sha(CONTROL/'observer')==proof['observer_binary_sha256'],'accepted maintenance binary mismatch')
 from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
 from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat,PrivateFormat,NoEncryption
 key=Ed25519PrivateKey.generate();(CONTROL/'host-key.private').write_bytes(key.private_bytes(Encoding.Raw,PrivateFormat.Raw,NoEncryption()));public=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex();audit=str(uuid.uuid4())
 worker=CONTROL/'worker-private';worker.mkdir();root=CONTROL/'root-private';shutil.copytree(OLD/'business-races/root-check-disabled',root)
 original=load(root/'root.json');write(worker/'inspection.json',dict(Endpoint=original['Endpoint'],AccessKey=original['RootAccess'],SecretKey=original['RootSecret']))
 base=load(OLD/'business-races/case-business/remaining.json')['Base']
 cfg=dict(Enabled=False,AuditID=audit,WorkspaceID=base['WorkspaceID'],ActorID=base['ActorID'],Purpose='POST_V28_BUSINESS_DENIED_AND_NOT_SENT_CLOSURE',PostgresDSN=base['PostgresDSN'],Core=g['Core'],BinarySHA256=sha(CONTROL/'observer'),PublicKey=public,EvidenceSHA256='0'*64,Token=secrets.token_hex(32),Owners=before['owners'],NotBefore='0001-01-01T00:00:00Z',ExpiresAt='0001-01-01T00:00:00Z',MaxProviderDeletes=0)
 write(worker/'closure.json',cfg);write(CONTROL/'closure-disabled-original.json',cfg)
 allocated={'observer':allocate('observer',CONTROL/'observer',worker,CONTROL/'output-observer',g['Native'],audit)}
 for action in ('check-disabled','disable-all'):allocated[action]=allocate(action,OLD/'owner',root,CONTROL/('output-'+action),g['Native'],audit)
 manifest=dict(status='PREPARED_DISABLED_V28_MIXED_MAINTENANCE',audit_id=audit,source_head=p.call(['git','rev-parse','HEAD']).decode().strip(),source_files=code_inputs(),source=g,retained=before,targets=targets(),owners=allocated,public_key=public,activation_fields=['Enabled','NotBefore','ExpiresAt','EvidenceSHA256'],max_provider_deletes=0,pins_released=0,budgets_refunded=0,actual_schema_rehearsal_sha256=sha(EVIDENCE/'post-v28-restored-core03/terminal.json'),inputs={x.relative_to(CONTROL).as_posix():sha(x) for x in CONTROL.rglob('*') if x.is_file() and x.name!='host-key.private' and x.suffix!='.private'})
 write(CONTROL/'manifest.json',manifest);write(PUBLIC/'preparation.json',manifest)
 for owner in allocated.values():need(closed(own(owner,manifest)),'new owner not stopped')
 print(json.dumps(dict(status=manifest['status'],manifest_sha256=sha(CONTROL/'manifest.json'),source_started=False)),flush=True)

def run_owner(manifest,kind):
 owner=manifest['owners'][kind];need(closed(own(owner,manifest)),'maintenance containment drift')
 # Reuse the independent raw process-channel persistence and O_EXCL receipts.
 original=c.own_contract;c.own_contract=own
 try:return c.run_owner(manifest,kind)
 finally:c.own_contract=original

def fresh_iam(owner):
 folder=Path(owner['output']);result=load(folder/'result.json');users=set(load(CONTROL/'root-private/iam.json')['disabled_identities'])
 need(len(users)==15 and result['action']=='check-disabled' and result['exact_policy_set_verified'] is True and not result['changed'] and set(result['before'])==users and set(result['before'].values())=={'disabled'},'fresh full disabled/policy check failed')
 x=inspect(owner['cid']);need(closed(x) and x['State']['ExitCode']==0,'IAM owner active or failed');end=dt.datetime.fromisoformat(x['State']['FinishedAt']);need(dt.timedelta()<=now()-end<=dt.timedelta(seconds=45),'IAM stale')
 starts=list(folder.glob('start-*.json'));requests=[]
 for f in starts:
  start=load(f);obs=load(folder/f.name.replace('start-','observation-'));need(start['method']=='GET' and start['path'] in ('/minio/admin/v3/user-info','/minio/admin/v3/info-canned-policy') and not obs['transport_error'] and obs['http_status'] in (200,404),'nonreadonly IAM request')
  requests.append(dict(start_sha256=sha(f),observation_sha256=sha(folder/f.name.replace('start-','observation-'))))
 return dict(result=result,finished_at=end.isoformat(),technical_requests=requests,quantity=len(requests),amount='NOT_ASSESSED')

def delta(before,after,anchors):
 p.ALLOWED_RELATIONS=ALLOWED;afteranchors=p.append_anchors(n,packet()[0]['Groups'][0]['Core']);return delta_values(before,after,anchors,afteranchors)

def delta_values(before,after,anchors,afteranchors):
 need({k:v for k,v in before.items() if k!='relations'}=={k:v for k,v in after.items() if k!='relations'},'roles/ACL changed')
 old,new=({x['relation']:x for x in fp['relations']} for fp in (before,after));changed={k for k in old if old[k]!=new[k]};need(set(old)==set(new) and changed<=ALLOWED,'unrelated Core relation changed')
 for name,rows in anchors.items():need(all(afteranchors[name][h]>=count for h,count in rows.items()),'old fact lost or overwritten')
 return dict(old_rows_preserved=True,changed_relations=sorted(changed),new_row_counts={k:new[k]['rows']-old[k]['rows'] for k in sorted(ALLOWED)})

def run(review,review_sha):
 m=load(CONTROL/'manifest.json');r=load(review);need(sha(review)==review_sha and r['status']=='PASS_BOUNDED_INDEPENDENT_V28_CLOSURE_MANIFEST_REVIEW' and r['remaining_blockers']==[] and r['manifest_sha256']==sha(CONTROL/'manifest.json'),'independent exact manifest gate missing')
 need(not (CONTROL/'terminal.json').exists() and not (PUBLIC/'terminal.json').exists(),'already terminal; no replay')
 need(code_inputs()==m['source_files'] and p.call(['git','rev-parse','HEAD']).decode().strip()==m['source_head'],'source drift')
 need(snapshot()==m['retained'],'retained baseline drift')
 for name,digest in m['inputs'].items():need(sha(CONTROL/name)==digest,'input drift')
 g=m['source'];started=[];result=dict(native_delete=0,pins_released=0,budgets_refunded=0,old_authorization_reused=False);failure=None
 try:
  for cid in (g['Core'],g['Native']):started.append(cid);docker('start',cid)
  n.wait_pg(g['Core'],'fixture');before=n.fingerprints(g['Core']);need(before==load(EVIDENCE/'post-v28-core-audit01/Core-after.json'),'fresh Core baseline drift')
  write(PUBLIC/'Core-before.json',before);p.ALLOWED_RELATIONS=ALLOWED;anchors=p.append_anchors(n,g['Core'])
  run_owner(m,'check-disabled');iam=fresh_iam(m['owners']['check-disabled']);owners,rows=physical((g['Core'],g['Native']))
  need(owners==m['retained']['owners'] and now()-dt.datetime.fromisoformat(iam['finished_at'])<=dt.timedelta(seconds=45),'fresh proof boundary expired')
  at=now();write(PUBLIC/'trusted-evidence.json',dict(at=at.isoformat(),manifest_sha256=sha(CONTROL/'manifest.json'),IAM=iam,checked_closed_owner_count=len(rows)-2,execution_owners=owners,Core_before_sha256=sha(PUBLIC/'Core-before.json')))
  cfg=load(CONTROL/'closure-disabled-original.json');cfg.update(Enabled=True,NotBefore=at.isoformat(),ExpiresAt=(at+dt.timedelta(minutes=40)).isoformat(),EvidenceSHA256=sha(PUBLIC/'trusted-evidence.json'))
  (CONTROL/'worker-private/closure.json').write_text(json.dumps(cfg,indent=2)+'\n',encoding='utf-8',newline='\n')
  from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
  key=Ed25519PrivateKey.from_private_bytes((CONTROL/'host-key.private').read_bytes());body=dict(AuditID=m['audit_id'],Nonce=str(uuid.uuid4()),At=at.isoformat(),Execution=EXECUTION,CandidateSHA256=CANDIDATE,BindingsSHA256=BINDINGS,Core=g['Core'],EvidenceSHA256=cfg['EvidenceSHA256'],Targets=m['targets'][1:],Owners=owners,StoppedPIDZero=True,NoRestart=True,ConfigDisabled=True,DeleteIdentityDisabled=True,GatewaySealed=True)
  raw=json.dumps(body,separators=(',',':')).encode();write(CONTROL/'worker-private/proof.json',dict(Body=base64.b64encode(raw).decode(),Signature=base64.b64encode(key.sign(raw)).decode()))
  run_owner(m,'observer');observed=load(CONTROL/'output-observer/result.json');need(observed['completed'] and len(observed['cases'])==3,'partial maintenance result');result['observer']=observed
  after=n.fingerprints(g['Core']);write(PUBLIC/'Core-after.json',after);result['delta']=delta(before,after,anchors)
  need(result['delta']['new_row_counts']['lifecycle_dispatch_release']==3 and result['delta']['new_row_counts']['lifecycle_provider_observation']==2 and result['delta']['new_row_counts']['lifecycle_storage_call']==1 and result['delta']['new_row_counts']['lifecycle_storage_observation']==1 and result['delta']['new_row_counts']['cost_event']==1,'complete mixed closure counts differ')
 except Exception as e:failure=type(e).__name__+': '+str(e)
 finally:
  try:
   cfg=load(CONTROL/'worker-private/closure.json');cfg['Enabled']=False;(CONTROL/'worker-private/closure.json').write_text(json.dumps(cfg,indent=2)+'\n',encoding='utf-8',newline='\n');result['profile_disabled']=True
  except Exception:result['profile_disabled']=False
  try:docker('stop','-t','15',m['owners']['observer']['cid'])
  except Exception:pass
  try:
   if inspect(g['Core'])['State']['Running']:
    write(PUBLIC/'final-Core-fingerprint.json',n.fingerprints(g['Core']));audit=module('v28_current_rows','audit-v28-business-dispatches.py');write(PUBLIC/'final-Core-rows.json',[json.loads(x) for x in n.sql(g['Core'],audit.old.query()).splitlines()])
  except Exception:result['final_Core_unconfirmed']=True
  try:
   if inspect(g['Native'])['State']['Running']:
    run_owner(m,'disable-all');s=load(CONTROL/'output-disable-all/status-changes.json');need(not s['unconfirmed'] and set(s['disabled_readback'].values())=={'disabled'},'identity closure unconfirmed');result['identity_containment']='FRESH_DISABLED_READBACK'
  except Exception:result['identity_containment']='UNCONFIRMED'
  result['closure']=[]
  for cid in [x['cid'] for x in m['owners'].values()]+list(reversed(started)):
   try:docker('stop','-t','15',cid)
   except Exception:pass
   try:result['closure'].append(dict(cid=cid,stopped_PID0=closed(inspect(cid))))
   except Exception:result['closure'].append(dict(cid=cid,stopped_PID0=False))
  try:result['retained_unchanged']=snapshot()==m['retained']
  except Exception:result['retained_unchanged']=False
  result.update(failure=failure,status='PASS_V28_MIXED_CLOSURE_APPEND_ONLY_SEALED' if not failure and result.get('profile_disabled') and result.get('retained_unchanged') and result.get('identity_containment')=='FRESH_DISABLED_READBACK' and not result.get('final_Core_unconfirmed') and all(x['stopped_PID0'] for x in result['closure']) else 'FAILED_OR_UNCONFIRMED_RETAIN_ALL_NO_REPLAY')
  write(PUBLIC/'terminal.json',result);write(CONTROL/'terminal.json',result)
 print(json.dumps(dict(status=result['status'],failure=failure)),flush=True);return result['status']=='PASS_V28_MIXED_CLOSURE_APPEND_ONLY_SEALED'

if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('action',choices=['prepare','run']);parser.add_argument('--review',type=Path);parser.add_argument('--review-sha256');a=parser.parse_args()
 if a.action=='prepare':prepare()
 else:
  need(a.review is not None and a.review_sha256 is not None,'independent manifest required');raise SystemExit(0 if run(a.review,a.review_sha256) else 1)
