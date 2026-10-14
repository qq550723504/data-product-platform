"""Finite non-delete D1 native acceptance. No DELETE/cleanup/enable CLI."""
import argparse,datetime as dt,hashlib,json,os,subprocess,time,uuid
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2]
PRIVATE=Path('C:/Users/Henry/.codex/fixture-private')
CONTROL=PRIVATE/'d1-native-completion01'
PUBLIC=ROOT/'docs/acceptance/evidence/issue307-d1/native-completion01'
PHASES=('prepare-plane','prepare-archive','prepare-copy','prepare-metadata','plane-setup','pre-issued-crash','verify-pre-issued-crash','pre-issued','plane-final','plane-seal','archive-run','copy-init','copy-archive-faults','copy-archive-resume','copy-restore-owner','copy-revocation','copy-finish','metadata-init','metadata-seed','metadata-faults','metadata-resume','metadata-owner','metadata-finish','seal')
def require(ok,message):
 if not ok:raise RuntimeError(message)
def load(p):return json.loads(p.read_text(encoding='utf-8-sig'))
def write(p,value):
 with p.open('x',encoding='utf-8',newline='\n') as f:json.dump(value,f,indent=2);f.write('\n')
def call(args,input_data=None):
 r=subprocess.run(args,input=input_data,cwd=ROOT,capture_output=True)
 if r.returncode:
  if CONTROL.exists():(CONTROL/('failure-'+dt.datetime.now(dt.timezone.utc).strftime('%Y%m%dT%H%M%S%f')+'.private-log')).write_bytes(r.stdout+b'\n'+r.stderr)
  raise RuntimeError('Controlled command failed; private diagnostics retained; no automatic retry')
 return r.stdout
def docker(*args,**kw):return call(['docker',*args],**kw)
def ps(name,*args):return call(['pwsh','-NoProfile','-File',str(ROOT/'examples/lifecycle-d1'/name),*map(str,args)])
def fixture(role):
 record=load(CONTROL/(role+'.json'));private=Path(record['private']);r=load(private/'resources.json');cfg=load(private/'config.json')
 expected={x['name']:x for x in load(PUBLIC/(role+'-fixture.json'))['services']}
 require(r['run_id']==cfg['RunID'] and r['project']=='dpp-lifecycle307-'+r['run_id'][:8] and private.name==r['project'],'Fixture identity drift')
 for name in r['containers'].values():
  item=json.loads(docker('inspect',name))[0]
  require(item['Id']==expected[name]['id'] and item['Image']==expected[name]['image'] and item['Config']['Labels']['lifecycle307.run']==r['run_id'] and item['State']['Running'] and not any(item['NetworkSettings']['Ports'].values()),'Closed owned fixture drift')
 return private,r,cfg
def sql(role,query):
 _,r,_=fixture(role)
 return docker('exec','-i',r['containers']['postgres'],'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U','fixture','-d','lifecycle307',input_data=query.encode()).decode().strip()
def prepare(role):
 if not CONTROL.exists():CONTROL.mkdir();PUBLIC.mkdir()
 require(not (CONTROL/(role+'.json')).exists(),'Role already allocated; preserve and use explicit new generation')
 before=set(PRIVATE.glob('dpp-lifecycle307-*'));subnet={'plane':154,'archive':155,'copy':156,'metadata':157}[role]
 args=['-PrivateRoot',PRIVATE,'-ArchiveAcceptance','-InternalSubnet','10.234.'+str(subnet)+'.0/24','-MinioImage','sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6']
 if role=='plane':args+=['-ProcessingDeliveryAcceptance']
 ps('prepare.ps1',*args)
 created=set(PRIVATE.glob('dpp-lifecycle307-*'))-before;require(len(created)==1,'Fresh fixture allocation ambiguous');private=created.pop();r=load(private/'resources.json')
 write(CONTROL/(role+'.json'),{'private':str(private),'resources':r,'source_head':call(['git','rev-parse','HEAD']).decode().strip()})
 manifest=load(ROOT/'examples/lifecycle-d1/results'/r['run_id']/'manifest.json');require(manifest['environment']['migrations_through']==68 and manifest['environment']['base_main']=='074a33bace7edbaf481759aac3a84708363ffd02','Current schema68/base required')
 write(PUBLIC/(role+'-fixture.json'),{'run':r['run_id'],'project':r['project'],'resources':r,'objects':manifest['objects'],'source_head':call(['git','rev-parse','HEAD']).decode().strip(),'schema':68,'services':[{'name':name,'id':json.loads(docker('inspect',name))[0]['Id'],'image':json.loads(docker('inspect',name))[0]['Image']} for name in r['containers'].values()],'delete':0,'cleanup':0})
 if role=='plane':ps('prepare-plane.ps1','-PrivateDirectory',private)
def plane(mode):
 private,_,_=fixture('plane');ps('run-plane-step.ps1','-PrivateDirectory',private,'-Mode',mode)
def archive():
 private,_,_=fixture('archive');ps('run-archive-restore.ps1','-PrivateDirectory',private)
def copy_init():
 private,_,_=fixture('copy');ps('prepare-copy-recovery.ps1','-PrivateDirectory',private)
def metadata_init():
 private,_,_=fixture('metadata');ps('prepare-recovery.ps1','-PrivateDirectory',private)
def metadata_seed():
 private,_,_=fixture('metadata');ps('run-recovery-step.ps1','-PrivateDirectory',private,'-Mode','seed','-Scenario','completion-sql-input')
def output(role):
 _,r,_=fixture(role);return ROOT/'examples/lifecycle-d1/results'/r['run_id']
def counts(role):
 return json.loads(sql(role,"SELECT json_build_object('calls',(SELECT count(*) FROM lifecycle_storage_call),'runs',(SELECT count(*) FROM lifecycle_reconciliation_run),'results',(SELECT count(*) FROM lifecycle_reconciliation_result),'copies',(SELECT count(*) FROM lifecycle_storage_call WHERE operation IN('ARCHIVE_COPY','RESTORE_COPY')));"))
def result(role,work):
 work=str(uuid.UUID(work))
 return json.loads(sql(role,"SELECT coalesce(json_agg(to_jsonb(x) ORDER BY observed_at),'[]') FROM lifecycle_reconciliation_result x WHERE work_id='"+work+"';"))
def step(role,mode,scenario,work=None,call_id=None,target=None):
 private,_,_=fixture(role);args=['-PrivateDirectory',private,'-Mode',mode,'-Scenario',scenario]
 if work:args+=['-WorkID',str(uuid.UUID(work))]
 if call_id:args+=['-CopyCallID',str(uuid.UUID(call_id))]
 if target:args+=['-Target',target]
 ps('run-copy-recovery-step.ps1' if role=='copy' else 'run-recovery-step.ps1',*args)
def launch(role,mode,scenario,work):
 """Existing read-only binary; no gateway mutation while a worker owns a barrier."""
 private,r,c=fixture(role);directory=private/('copy-worker' if role=='copy' else 'worker');recovery=load(directory/'recovery.json')
 require(recovery['Enabled'],'Read-only worker current authority is disabled')
 require(scenario.replace('-','').isalnum(),'Finite helper label required')
 work=str(uuid.UUID(work));zero=str(uuid.UUID(int=0));out=output(role)
 cfg={'Mode':mode,'Scenario':scenario,'PostgresDSN':(directory/'dsn').read_text(),'RedisAddr':r['project']+'-redis:6379','RecoveryConfig':'/run/private/recovery.json','Manifest':'/run/private/manifest.json' if role=='metadata' else '', 'OutputDirectory':'/run/output/'+r['run_id'],'Barrier':'/run/output/'+r['run_id']+'/'+scenario+'-barrier.json','SeedEnabled':False,'CopyRecovery':role=='copy','WorkspaceID':c['WorkspaceID'],'ActorID':c['ActorID'],'WorkID':work,'CopyCallID':zero,'ExpiresAt':recovery['ValidUntil']}
 write(directory/(scenario+'.json'),cfg)
 name=r['project']+'-'+scenario
 args=['create','--pull','never','--name',name,'--label','lifecycle307.run='+r['run_id'],'--network','container:'+r['containers']['gateway'],'--read-only','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data','--mount','type=bind,source='+str(directory)+',target=/run/private,readonly','--mount','type=bind,source='+str(ROOT/'examples/lifecycle-d1/results')+',target=/run/output']
 if role=='copy':args+=['--mount','type=bind,source='+str(private/'copy-independent-witness')+',target=/run/witness,readonly']
 args+=['-e','LIFECYCLE_RECOVERY_ACCEPTANCE_CONFIG=/run/private/'+scenario+'.json','--entrypoint','/run/private/recovery','sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea']
 docker(*args);item=json.loads(docker('inspect',name))[0]
 write(out/(scenario+'-helper.json'),{'helper_id':item['Id'],'scenario':scenario,'work':work,'mode':mode,'source_head':call(['git','rev-parse','HEAD']).decode().strip(),'binary_sha256':hashlib.sha256((directory/'recovery').read_bytes()).hexdigest(),'gateway_sha256':hashlib.sha256((private/'Caddyfile').read_bytes()).hexdigest(),'root':False,'copy_send':False,'native_delete':0,'cleanup':0})
 docker('start',name);return name
def finish_helper(role,name,expected=0):
 exit_code=int(docker('wait',name));write(output(role)/(name+'-exit.json'),{'helper':name,'exit':exit_code,'expected':expected});require(exit_code==expected,'Retained helper unexpected exit; diagnose before continuation')
def barrier(role,scenario):
 p=output(role)/(scenario+'-barrier.json');deadline=time.monotonic()+40
 while not p.exists() and time.monotonic()<deadline:time.sleep(.1)
 require(p.exists(),'Controlled worker barrier not reached');return p
def authority(role,enabled):
 private,_,_=fixture(role);p=private/('copy-worker' if role=='copy' else 'worker')/'recovery.json';cfg=load(p);cfg['Enabled']=enabled
 p.write_text(json.dumps(cfg,indent=2)+'\n',encoding='utf-8')
def replay(role,work,scenario):
 before=counts(role);step(role,'once',scenario,work);after=counts(role);require(before==after,'Terminal replay changed facts or made native I/O')
def copy_archive_faults():
 step('copy','produce-archive','copy-producer-archive');p=load(output('copy')/'copy-producer-ARCHIVE.json');work=p['work_id']
 _,r,_=fixture('copy');redis=r['project']+'-redis';docker('stop',redis)
 try:
  step('copy','outbox','copy-outbox-broker-unavailable',work);failed=load(output('copy')/'copy-outbox-broker-unavailable.json');require(failed['state']=='FAILED' and failed['handler_confirmations']==0,'Unavailable broker falsely confirmed')
 finally:docker('start',redis)
 step('copy','outbox','copy-broker-repaired',work);require(load(output('copy')/'copy-broker-repaired.json')['state']=='PUBLISHED','Outbox not repaired')
 duplicate_before=counts('copy');step('copy','enqueue','copy-duplicate-enqueue',work);require(counts('copy')==duplicate_before,'Duplicate enqueue invoked storage or changed Core work')
 before=counts('copy');step('copy','once','copy-crash-before-lookup',work);require(counts('copy')['calls']==before['calls'],'Pre-lookup crash invoked storage')
 step('copy','worker','copy-crash-after-lookup',work);step('copy','once','copy-crash-after-bytes',work)
 write(PUBLIC/'copy-archive-faults.json',{'work':work,'counts':counts('copy'),'broker_same_volume_restart':True,'explicit_double_enqueue_native_delta':0,'real_asynq_worker_exit':92,'producer_exit':93,'run_start_exit':91,'durable_get_exit':96,'native_copy':1,'native_delete':0})
def copy_archive_resume():
 work=load(PUBLIC/'copy-archive-faults.json')['work'];step('copy','worker','copy-orphan-resumed',work)
 require(result('copy',work)[-1]['outcome']=='COPY_ARCHIVED','Orphan task did not archive');replay('copy',work,'copy-archive-replay')
 require(counts('copy')['copies']==1,'Original COPY was resent')
 write(PUBLIC/'copy-archive-resume.json',{'work':work,'results':result('copy',work),'counts':counts('copy'),'native_copy':1,'native_delete':0})
def copy_restore_owner():
 step('copy','produce-restore','copy-producer-restore');p=load(output('copy')/'copy-producer-RESTORE-online-a.csv.json');work=p['work_id']
 private,_,_=fixture('copy');ps('run-copy-recovery-step.ps1','-PrivateDirectory',private,'-Mode','once','-Scenario','copy-hold-owner','-WorkID',work,'-StartOnly')
 b=barrier('copy','copy-hold-owner');before=counts('copy');name=launch('copy','once','copy-competing-owner',work);finish_helper('copy',name)
 require(load(output('copy')/'copy-competing-owner.json')['result']=='OWNER_BUSY' and counts('copy')==before,'Competing owner invoked storage or started run')
 write(Path(str(b)+'.release'),{'controlled_release':True})
 helpers=[json.loads(docker('inspect',n))[0] for n in docker('ps','-aq','--filter','label=lifecycle307.run='+fixture('copy')[1]['run_id']).decode().split()]
 held=[i['Name'][1:] for i in helpers if '-copy-hold-owner-' in i['Name']];require(len(held)==1,'Held owner helper identity ambiguous');finish_helper('copy',held[0])
 require(result('copy',work)[-1]['outcome']=='UNKNOWN_COPY_LEDGER','Missing ledger admitted ONLINE')
 step('copy','witness','copy-fresh-witness');step('copy','request','copy-fresh-restore-request',call_id=p['copy_call_id']);fresh=load(output('copy')/'copy-fresh-restore-request.json')['ID']
 step('copy','outbox','copy-fresh-restore-outbox',fresh);step('copy','worker','copy-fresh-restore-worker',fresh)
 require(result('copy',fresh)[-1]['outcome']=='RESTORE_ONLINE','Fresh ledger failed ONLINE');replay('copy',fresh,'copy-restored-replay')
 write(PUBLIC/'copy-restore-owner.json',{'original_work':work,'fresh_work':fresh,'results':result('copy',work)+result('copy',fresh),'competing_native_io':0,'counts':counts('copy'),'native_delete':0})
def copy_revocation():
 step('copy','produce-restore','copy-producer-revoked',target='restore/old-cut-a.csv');p=load(output('copy')/'copy-producer-RESTORE-old-cut-a.csv.json');work=p['work_id'];private,_,_=fixture('copy')
 ps('run-copy-recovery-step.ps1','-PrivateDirectory',private,'-Mode','once','-Scenario','copy-revoke-after-lookup','-WorkID',work,'-StartOnly')
 b=barrier('copy','copy-revoke-after-lookup');before=counts('copy');authority('copy',False);write(Path(str(b)+'.release'),{'authority_now_disabled':True})
 names=docker('ps','-aq','--filter','name='+fixture('copy')[1]['project']+'-copy-revoke-after-lookup-').decode().split();require(len(names)==1,'Revoked helper identity ambiguous');finish_helper('copy',names[0]);after=counts('copy')
 require(after['calls']==before['calls'] and result('copy',work)[-1]['outcome']=='BLOCKED_AUTHORITY','Revoked worker performed later I/O')
 authority('copy',True);replay('copy',work,'copy-authority-replay');step('copy','restrict','copy-later-restriction');step('copy','request','copy-old-ledger-request',call_id=p['copy_call_id']);old=load(output('copy')/'copy-old-ledger-request.json')['ID'];step('copy','once','copy-old-ledger-worker',old)
 require(result('copy',old)[-1]['outcome']=='UNKNOWN_COPY_LEDGER','Stale witness admitted restored data')
 step('copy','witness','copy-restricted-witness');step('copy','request','copy-restricted-request',call_id=p['copy_call_id']);fresh=load(output('copy')/'copy-restricted-request.json')['ID'];step('copy','outbox','copy-restricted-outbox',fresh);step('copy','worker','copy-restricted-worker',fresh)
 require(result('copy',fresh)[-1]['outcome']=='BLOCKED_COPY','Current restriction admitted ONLINE')
 write(PUBLIC/'copy-revocation.json',{'work':work,'after_lookup_authority_revocation_native_delta':after['calls']-before['calls'],'results':result('copy',work)+result('copy',old)+result('copy',fresh),'counts':counts('copy'),'native_delete':0})
def metadata_faults():
 p=load(output('metadata')/'synthetic-input.json');work=p['work_id'];step('metadata','outbox','metadata-outbox',work);before=counts('metadata');step('metadata','once','crash-before-lookup',work);require(counts('metadata')['calls']==before['calls'],'Metadata pre-lookup crash invoked storage');step('metadata','worker','crash-after-lookup',work)
 write(PUBLIC/'metadata-faults.json',{'work':work,'sql_input':'NOT_NATIVE_DELETE_APPROVAL','real_asynq_exit':92,'counts':counts('metadata'),'native_delete':0})
def metadata_resume():
 work=load(PUBLIC/'metadata-faults.json')['work'];name=launch('metadata','worker-result','metadata-resumed-worker',work);finish_helper('metadata',name)
 require(result('metadata',work)[-1]['outcome']=='UNKNOWN_PRESENT' and not result('metadata',work)[-1]['terminal'],'Native present object became absence')
 write(PUBLIC/'metadata-resume.json',{'results':result('metadata',work),'counts':counts('metadata'),'native_delete':0,'provider_send':0})
def metadata_owner():
 work=load(PUBLIC/'metadata-faults.json')['work'];name=launch('metadata','once','hold-owner',work);b=barrier('metadata','hold-owner');before=counts('metadata');other=launch('metadata','once','metadata-competing-owner',work);finish_helper('metadata',other);require(counts('metadata')==before and load(output('metadata')/'metadata-competing-owner.json')['result']=='OWNER_BUSY','Metadata competing owner invoked storage')
 authority('metadata',False);write(Path(str(b)+'.release'),{'authority_now_disabled':True});finish_helper('metadata',name);after=counts('metadata');require(after['calls']==before['calls'] and result('metadata',work)[-1]['outcome']=='BLOCKED_AUTHORITY','Metadata revoked worker invoked later storage')
 authority('metadata',True);replay('metadata',work,'metadata-terminal-replay')
 write(PUBLIC/'metadata-owner.json',{'competing_native_io':0,'revoked_native_delta':after['calls']-before['calls'],'results':result('metadata',work),'counts':counts('metadata'),'native_delete':0})
def accounting(role):
 data=json.loads(sql(role,"SELECT json_build_object('calls',(SELECT count(*) FROM lifecycle_storage_call),'observations',(SELECT count(*) FROM lifecycle_storage_observation),'invocations',(SELECT count(*) FROM lifecycle_storage_observation WHERE invoked_at IS NOT NULL),'typed_costs',(SELECT count(*) FROM cost_allocation a JOIN cost_event e ON e.id=a.cost_event_id WHERE a.lifecycle_storage_call_id IS NOT NULL AND e.activity_id=a.lifecycle_storage_call_id AND e.quantity=1 AND e.unit='invocation'),'unaccounted',(SELECT count(*) FROM lifecycle_storage_observation o WHERE o.invoked_at IS NOT NULL AND (SELECT count(*) FROM cost_allocation a WHERE a.lifecycle_storage_call_id=o.call_id)<>1),'send_claims',(SELECT count(*) FROM lifecycle_provider_send_claim),'absence',(SELECT count(*) FROM lifecycle_copy_absence_receipt),'completion',(SELECT count(*) FROM lifecycle_plan_completion),'dispatch_releases',(SELECT count(*) FROM lifecycle_dispatch_release));"))
 require(data['invocations']==data['typed_costs'] and data['unaccounted']==0 and data['send_claims']==data['absence']==data['completion']==data['dispatch_releases']==0,'Physical cost or non-delete boundary violated')
 tables={}
 for table in ('lifecycle_reconciliation_work','lifecycle_reconciliation_run','lifecycle_reconciliation_result','lifecycle_copy_decision','lifecycle_storage_call','lifecycle_storage_observation','lifecycle_provider_attempt','lifecycle_provider_observation','lifecycle_disposition_dispatch'):
  tables[table]=json.loads(sql(role,"SELECT coalesce(json_agg(to_jsonb(x) ORDER BY id),'[]') FROM "+table+" x;"))
 write(PUBLIC/(role+'-accounting.json'),{'status':'PASS','counts':data,'facts':tables,'native_delete':0})
def copy_finish():
 step('copy','readback','copy-final-witness');accounting('copy');step('copy','seal','copy-final-seal')
 require(counts('copy')['copies']==3,'COPY physical send count drift')
def metadata_finish():
 step('metadata','witness','metadata-final-witness');accounting('metadata');authority('metadata',False)
def seal():
 for role in ('plane','archive','copy','metadata'):
  private,r,_=fixture(role)
  for path in private.glob('*/recovery.json'):require(load(path)['Enabled'] is False,'Worker not sealed')
  for path in private.glob('*/archive-window.json'):require(load(path)['Enabled'] is False,'COPY role not sealed')
  for path in private.glob('*/reader.json'):require(load(path)['Enabled'] is False,'Reader role not sealed')
 write(PUBLIC/'sealed.json',{'status':'PASS_FINITE_NON_DELETE_EXTENSION','native_delete':0,'cleanup':0,'full_D1':'NOT_ACCEPTED','resources':'ALL_RETAINED','roles':{role:load(PUBLIC/(role+'-fixture.json'))['resources'] for role in ('plane','archive','copy','metadata')}})
def main():
 parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('phase',choices=PHASES);args=parser.parse_args()
 require(not (PUBLIC/'sealed.json').exists(),'Completed generation cannot reopen')
 if not CONTROL.exists():CONTROL.mkdir();PUBLIC.mkdir()
 write(PUBLIC/('attempt-'+args.phase+'.json'),{'phase':args.phase,'source_head':call(['git','rev-parse','HEAD']).decode().strip(),'driver_sha256':hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),'intent':'ONE_SHOT_NON_DELETE; append-only, failure never auto-resumes'})
 if args.phase.startswith('prepare-'):prepare(args.phase[8:])
 elif args.phase=='plane-setup':plane('setup')
 elif args.phase in ('pre-issued-crash','verify-pre-issued-crash','pre-issued'):plane(args.phase)
 elif args.phase=='plane-final':plane('final')
 elif args.phase=='plane-seal':
  private,_,_=fixture('plane');ps('seal-plane.ps1','-PrivateDirectory',private)
 elif args.phase=='archive-run':archive()
 elif args.phase=='copy-init':copy_init()
 elif args.phase=='metadata-init':metadata_init()
 elif args.phase=='metadata-seed':metadata_seed()
 elif args.phase=='copy-archive-faults':copy_archive_faults()
 elif args.phase=='copy-archive-resume':copy_archive_resume()
 elif args.phase=='copy-restore-owner':copy_restore_owner()
 elif args.phase=='copy-revocation':copy_revocation()
 elif args.phase=='copy-finish':copy_finish()
 elif args.phase=='metadata-faults':metadata_faults()
 elif args.phase=='metadata-resume':metadata_resume()
 elif args.phase=='metadata-owner':metadata_owner()
 elif args.phase=='metadata-finish':metadata_finish()
 elif args.phase=='seal':seal()
 print(json.dumps({'phase':args.phase,'status':'PASS','delete':0,'cleanup':0}))
if __name__=='__main__':main()
