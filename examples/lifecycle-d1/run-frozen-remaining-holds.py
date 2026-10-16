"""Latest frozen reader H0/H1/H2 preparation on the retained schema69 clone.
No source environment/backup/permission mutation, send, DELETE or cleanup.
"""
import base64
import datetime as dt
import importlib.util
from pathlib import Path
import time
import uuid
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat

spec=importlib.util.spec_from_file_location('f',Path(__file__).with_name('freeze-remaining-executable.py'))
f=importlib.util.module_from_spec(spec);spec.loader.exec_module(f);r=f.r

def main():
    private=r.PRIVATE/'remaining-final-regression01';out=f.PUBLIC.parent/'remaining-final-regression01'
    r.require(not private.exists() and not out.exists(),'Frozen regression already allocated');private.mkdir();out.mkdir()
    c=r.load(f.PUBLIC/'candidate.json');r.require(r.sha(f.PRIVATE/'reader')==c['ReaderBinarySHA256'],'Frozen reader drift')
    prior=r.PRIVATE/'remaining-executable03/late-hold';cfg=r.load(prior/'clone-owner/remaining.json');batch=str(uuid.uuid4());cfg['Batch']=batch
    cfg['Base'].update(Enabled=True,StartedAt=r.utc().isoformat(),ExpiresAt=(r.utc()+dt.timedelta(minutes=55)).isoformat(),Epoch=str(uuid.uuid4()))
    owner=private/'owner';admin=private/'admin';owner.mkdir();admin.mkdir();r.write(owner/'remaining.json',cfg)
    root=r.load(prior/'clone-admin/remaining.json');root.update(Batch=batch);root['Base'].update(cfg['Base']);r.write(admin/'remaining.json',root)
    for path in (owner,admin):(path/'manifest.json').write_bytes((prior/'clone-owner/manifest.json').read_bytes())
    (admin/'inventory.json').write_bytes((prior/'clone-admin/inventory.json').read_bytes())
    prefix='dpp-lifecycle307-754ea8f0-candidate07';pg,mi,gw=(r.inspect(prefix+x)['Id'] for x in ('-checkpoint','-minio','-gateway'));owned=[]
    for ident in (pg,mi,gw):r.require(r.inspect(ident)['State']['Pid']==0,'Retained clone active')
    def helper(name,mode,folder,netns):
        output=out/name;output.mkdir();ident=r.docker('create','--pull','never','--name',prefix+'-final-'+name,'--label','lifecycle307.remaining='+batch,'--network','container:'+netns,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--tmpfs','/var/lib/postgresql/data:size=1m','--mount',f'type=bind,source={folder},target=/run/private,readonly','--mount',f'type=bind,source={f.PRIVATE}/reader,target=/run/reader,readonly','--mount',f'type=bind,source={output},target=/run/output','--entrypoint','/run/reader',r.PG,'-mode',mode).decode().strip();owned.append(ident);r.docker('start',ident);code=int(r.docker('wait',ident));(out/(name+'.stdout.txt')).write_bytes(r.docker('logs',ident));r.write(out/(name+'.json'),{'container':ident,'mode':mode,'exit_code':code,'binary_sha256':c['ReaderBinarySHA256'],'native_delete':0});r.require(code==0,name+' failed');return ident
    try:
        for ident in (pg,mi,gw):owned.append(ident);r.docker('start',ident)
        r.native.wait_pg(pg,'fixture');time.sleep(3)
        r.require(r.native.sql(pg,'SELECT count(*) FROM lifecycle_disposition_approval;')=='0','Clone already has case execution')
        helper('enable-inspection','remaining-inspection-enable',admin,mi)
        case=helper('cases','remaining-late',owner,gw)
        helper('disable-inspection','remaining-inspection-disable',admin,mi)
        cfg['Base']['Enabled']=False;(owner/'remaining.json').write_bytes(f.compact(cfg)+b'\n')
        r.require(r.inspect(case)['State']['Pid']==0 and r.inspect(case)['HostConfig']['RestartPolicy']['Name']=='no','Native owner not closed')
        key=Ed25519PrivateKey.generate();binding=r.sha(out/'cases/remaining-H1.json')
        body={'Execution':batch,'Nonce':str(uuid.uuid4()),'CandidateSHA256':binding,'Group':'late-hold','At':r.utc().isoformat(),'Owners':{case:c['ReaderBinarySHA256']},'StoppedPIDZero':True,'NoRestart':True,'ConfigDisabled':True,'DeleteIdentityDisabled':True,'GatewaySealed':True}
        encoded=f.compact(body);proof={'Body':base64.b64encode(encoded).decode(),'Signature':base64.b64encode(key.sign(encoded)).decode()}
        close=private/'close';close.mkdir();cfg['Base']['Enabled']=True;cfg.update(ClosurePublicKey=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex(),ClosureCandidateSHA256=binding,ClosureOwners={case:c['ReaderBinarySHA256']});r.write(close/'remaining.json',cfg);r.write(close/'closed-H1.json',proof);(close/'manifest.json').write_bytes((owner/'manifest.json').read_bytes())
        helper('closed-H1','remaining-close-H1',close,gw)
        helper('final-readonly','remaining-verify-native',admin,mi)
        claims=int(r.native.sql(pg,'SELECT count(*) FROM lifecycle_provider_send_claim;'));r.require(claims==0,'Unexpected send claim')
        r.write(out/'result.json',{'status':'PASS_FROZEN_READER_H0_H1_NATIVE_NONDELETE','candidate_sha256':r.sha(f.PUBLIC/'candidate.json'),'reader_sha256':c['ReaderBinarySHA256'],'source':c['Source'],'rehearsal_batch':batch,'core':pg,'native':mi,'schema':int(r.native.sql(pg,'SELECT max(version) FROM schema_migration;')),'send_claims':claims,'H2':'READY_UNCLAIMED_NOT_SENT','native_delete':0,'source_environment_mutations':0,'cleanup':0})
    finally:
        seals=[]
        for ident in dict.fromkeys(reversed(owned)):
            state=r.inspect(ident)
            if state['State']['Running']:r.docker('stop','--time','20',ident)
            state=r.inspect(ident);r.require(state['State']['Pid']==0 and not any(state['NetworkSettings']['Ports'].values()),'Unsealed regression resource');seals.append({'id':ident,'name':state['Name'],'pid':0,'ports':state['NetworkSettings']['Ports']})
        r.write(out/'seal.json',{'containers':seals,'retained':True,'native_delete':0,'cleanup':0})
    r.write(out/'evidence-index.json',{'files':{p.relative_to(f.ROOT).as_posix():r.sha(p) for p in out.rglob('*') if p.is_file()}})

if __name__=='__main__':main()
