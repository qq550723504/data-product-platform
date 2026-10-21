"""Host composition of frozen owners only; no provider client or business SQL writes."""
from pathlib import Path
import datetime as dt
import hashlib
import importlib.util
import json
import os
import subprocess
import sys
import time
import uuid

ROOT=Path.cwd(); OUT=Path(__file__).resolve().parent
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
spec=importlib.util.spec_from_file_location('c18_fixed_window_support',ROOT/'examples/lifecycle-d1/window_six_support.py')
w=importlib.util.module_from_spec(spec);spec.loader.exec_module(w)
h=w.Host(); C=h.candidate; PRIVATE=w.PRIVATE; CONTROL=w.CONTROL
need=w.need; load=w.load; sha=w.sha
ENV=os.environ.copy();ENV.update(GIT_CONFIG_COUNT='1',GIT_CONFIG_KEY_0='core.longpaths',GIT_CONFIG_VALUE_0='true',PYTHONUTF8='1')

def save(path,value):
    path.parent.mkdir(parents=True,exist_ok=True)
    with path.open('xb') as f:f.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode())
def raw_new(path,data):
    path.parent.mkdir(parents=True,exist_ok=True)
    with path.open('xb') as f:f.write(data)
def now():return dt.datetime.now(dt.timezone.utc)
def window():
    need(w.p.window.instant(C['NotBefore'])<=now()<w.p.window.instant(C['NotAfter']),'Fixed sending window closed')
    need(not (CONTROL/'terminal.json').exists(),'Execution already terminal')
def docker(*args,timeout=50):return h.docker(*args,timeout=timeout)
def group(name):return h.group(name)
def closed(cid):return w.base.closed(h.inspect(cid))
def invoke(argv,tag,timeout=90):
    window();folder=OUT/tag;folder.mkdir(exist_ok=False);start=now().isoformat()
    with (folder/'stdout.txt').open('xb') as stdout,(folder/'stderr.txt').open('xb') as stderr:
        try:r=subprocess.run(argv,cwd=ROOT,env=ENV,stdout=stdout,stderr=stderr,timeout=timeout);code=r.returncode
        except BaseException as error:
            save(folder/'process.json',{'at':start,'finished':now().isoformat(),'args':argv,'error_type':type(error).__name__,'error':str(error)});raise
    save(folder/'process.json',{'at':start,'finished':now().isoformat(),'args':argv,'exit_code':code})
    need(code==0,'Frozen entry failed: '+tag)
    return (folder/'stdout.txt').read_bytes()
def start(cid):
    window();need(closed(cid),'Bound owner already active: '+cid);docker('start',cid)
def owner(cid,tag,timeout=90):
    window();need(closed(cid),'Cannot replay an active owner')
    folder=OUT/tag;folder.mkdir(exist_ok=False);start_at=now().isoformat();docker('start',cid)
    code=int(docker('wait',cid,timeout=timeout).decode('utf-8','strict').strip())
    logs=subprocess.run(['docker','logs',cid],capture_output=True,cwd=ROOT,env=ENV)
    raw_new(folder/'stdout.txt',logs.stdout);raw_new(folder/'stderr.txt',logs.stderr)
    item=h.inspect(cid);save(folder/'process.json',{'cid':cid,'started_at':start_at,'finished_at':now().isoformat(),'exit_code':code,'closed':w.base.closed(item),'actual_exit':item['State']['ExitCode']})
    need(code==0 and w.base.closed(item) and item['State']['ExitCode']==0,'Bound owner failed: '+tag)
def permission(g,step,enabled):
    window();path=PRIVATE/g['Group']/step/'permission.json';before=path.read_bytes();obj=json.loads(before)
    need(obj['Execution']==C['Execution'] and obj['CandidateSHA256']==w.base.CANDIDATE and obj['ConfirmationSHA256']==w.base.TEMPLATE and obj['NotBefore']==C['NotBefore'] and obj['ExpiresAt']==C['NotAfter'] and obj['MaxPerTarget']==1 and obj['MaxTotal']==6,'Bound permission differs')
    if enabled:
        need(obj['Enabled'] is False,'Owner already enabled')
        human=path.parent/'human-confirmation.txt';approved=(CONTROL/'approved-referenced-template.txt').read_bytes()
        if human.exists():need(human.read_bytes()==approved,'Existing machine approval differs')
        else:raw_new(human,approved)
        obj['ConfirmedAt']=now().isoformat().replace('+00:00','Z')
    obj['Enabled']=enabled
    archive=CONTROL/'permission-history'/str(uuid.uuid4());archive.mkdir(parents=True)
    raw_new(archive/'before.private',before);raw_new(archive/'after.private',(json.dumps(obj,separators=(',',':'))+'\n').encode())
    path.write_bytes((archive/'after.private').read_bytes())
    need(load(path)==obj,'Permission readback differs')
    save(archive/'change.json',{'at':now().isoformat(),'group':g['Group'],'owner':step,'enabled':enabled,'before_sha256':hashlib.sha256(before).hexdigest(),'after_sha256':sha(path),'changed_fields':['Enabled','ConfirmedAt'] if enabled else ['Enabled'],'authorization_sha256':sha(w.base.AUTHORIZATION_PUBLIC)})
def root(g,action,tag):
    window();item,output=h.root_owner(g,action);need(w.base.closed(item),'Root owner still active')
    receipt=h.archive_root(g,action);save(OUT/(tag+'-archive.json'),receipt)
    enabling=action in ('enable-read','enable-send')
    if enabling:permission(g,'root-'+action,True)
    try:owner(item['Id'],tag,60)
    finally:
        if enabling and now()<w.p.window.instant(C['NotAfter']):permission(g,'root-'+action,False)
    result=load(output/'result.json');need(result['action']==action and result['group']==g['Group'] and result['exact_policy_set_verified'] is True,'Root result not fresh/exact')
    save(OUT/(tag+'-result.json'),result)
    return result
def gateway(g,active,tag):
    window();need(closed(g['Gateway']),'Stop gateway before switching frozen config')
    folder=PRIVATE/g['Group'];golden=folder/('gateway-active.Caddyfile' if active else 'gateway-sealed.Caddyfile');expected=g['GatewaySHA256'] if active else g['SealedGatewaySHA256']
    need(sha(golden)==expected,'Gateway golden bytes differ')
    current=folder/'gateway.Caddyfile';archive=CONTROL/'gateway-history'/(tag+'.private');raw_new(archive,current.read_bytes());current.write_bytes(golden.read_bytes());need(sha(current)==expected,'Actual gateway config differs')
    start(g['Gateway'])
def case(g,step,tag):
    window();folder=PRIVATE/g['Group']/('case-'+step)
    need(sha(folder/'remaining.json')==g['CaseConfigSHA256'][step],'Frozen case config differs')
    owner(g['CaseHelpers'][step],tag,100)
def attest(g,step,tag):
    need(sha(ROOT/'examples/lifecycle-d1/attest-remaining-executable.py')=='df9693ea2c669ddb25caa163a7f01873b2f31ae65dcac9536703263579267af9','Frozen attester drift')
    return invoke([sys.executable,'-X','utf8','-B','examples/lifecycle-d1/attest-remaining-executable.py','--group',g['Group'],'--stage',step],tag,60)
def prepare():
    need(load(OUT/'complete-native/complete.json')['status']=='PASS_COMPLETE_SIX_NATIVE_ENTRY','Fresh full preflight incomplete')
    need(load(OUT/'post-full-host-check.stdout.json')['status']=='PASS_PREWINDOW_CLOSED','Post-full stopped Host.check missing')
    for g in C['Groups']:
        window();start(g['Core']);start(g['Native']);start(h.binding(g)['recovery_redis']);w.r.p.native.wait_pg(g['Core'],'fixture')
    time.sleep(3)
    for g in C['Groups']:
        gateway(g,True,'initial-'+g['Group']);root(g,'enable-read','enable-read-'+g['Group'])
    case(group('business-races'),'business','case-business')
    result=load(PRIVATE/'business-races/outputs/case-business/six-business-admission.json')
    need(set(result['admitted'])=={'P2','T1','T2'} and all(x['Admitted'] for x in result['admitted'].values()) and result['new_outputs']==0 and result['native_delete']==0,'Fresh business admission invalid')
    save(OUT/'source-prepared.json',{'status':'SOURCE_BUSINESS_NEW_ADMISSION_READY','at':now().isoformat(),'execution':C['Execution'],'admission':result,'real_delete':0})

def containment(error):
    save(OUT/('failure-'+str(uuid.uuid4())+'.json'),{'at':now().isoformat(),'error_type':type(error).__name__,'error':str(error),'new_send_stopped':True})
    sealed=w.Host(containment=True).seal();save(OUT/('failure-seal-'+str(uuid.uuid4())+'.json'),sealed)
    return sealed

if __name__=='__main__':
    try:
        need(sys.argv[1:] == ['prepare'],'Only the selected bounded phase is exposed')
        prepare();print(json.dumps({'status':'SOURCE_READ_AND_BUSINESS_ADMISSION_READY','native_delete':0}))
    except BaseException as error:
        try:sealed=containment(error);print(json.dumps({'status':'FAILED_PREPARE_SEALED_NO_NEW_SEND','reason':str(error),'all_steps_confirmed':sealed['all_steps_confirmed']}))
        except BaseException as close_error:print(json.dumps({'status':'FAILED_PREPARE_CONTAINMENT_UNCONFIRMED','reason':str(error),'containment_error':str(close_error)}))
        raise
