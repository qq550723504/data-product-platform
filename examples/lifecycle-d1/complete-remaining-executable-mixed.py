"""Finite continuation: retained mixed clone lacks only its M1 approval.

Do not restore again, renew policy, touch source Core, enable DELETE or send.
"""
import datetime as dt
import importlib.util
from pathlib import Path
import time

spec=importlib.util.spec_from_file_location('r',Path(__file__).with_name('prepare-remaining-runner.py'))
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)

def main():
    root=r.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-executable03/mixed'
    old=r.PRIVATE/'remaining-executable03';original=old/'mixed'
    fail=r.load(root/'C-protected.json');r.require(fail['exit_code']==1 and r.inspect(fail['id'])['State']['Pid']==0,'Retained C preparation failure differs')
    private=r.PRIVATE/'remaining-executable04';out=r.ROOT/'docs/acceptance/evidence/issue307-d1/remaining-executable04'
    r.require(not private.exists() and not out.exists(),'Continuation already allocated');private.mkdir();out.mkdir()
    project='dpp-lifecycle307-dfda43b4';prefix=project+'-candidate07';pg,mi,gw=(r.inspect(prefix+x)['Id'] for x in ('-checkpoint','-minio','-gateway'))
    for ident in (pg,mi,gw):r.require(r.inspect(ident)['State']['Pid']==0,'Retained clone active')
    owner=private/'owner';admin=private/'admin';owner.mkdir();admin.mkdir()
    for src,dst in [('clone-owner',owner),('clone-admin',admin)]:
        cfg=r.load(original/src/'remaining.json');cfg['Base']['StartedAt']=r.utc().isoformat();cfg['Base']['ExpiresAt']=(r.utc()+dt.timedelta(minutes=55)).isoformat();r.write(dst/'remaining.json',cfg)
        (dst/'manifest.json').write_bytes((original/src/'manifest.json').read_bytes())
    (admin/'inventory.json').write_bytes((original/'clone-admin/inventory.json').read_bytes())
    owned=[]
    def helper(name,mode,cfgdir,netns):
        output=out/name;output.mkdir()
        ident=r.docker('create','--pull','never','--name',prefix+'-04-'+name,'--label','lifecycle307.remaining='+cfg['Batch'],'--network','container:'+netns,'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--mount',f'type=bind,source={old}/reader,target=/run/reader,readonly','--mount',f'type=bind,source={cfgdir},target=/run/private,readonly','--mount',f'type=bind,source={output},target=/run/output','--entrypoint','/run/reader',r.PG,'-mode',mode).decode().strip();owned.append(ident);r.docker('start',ident);code=int(r.docker('wait',ident));(out/(name+'.stdout.txt')).write_bytes(r.docker('logs',ident));r.write(out/(name+'.json'),{'container':ident,'mode':mode,'exit_code':code,'binary_sha256':r.sha(old/'reader'),'native_delete':0});r.require(code==0,name+' failed')
    try:
        for ident in (pg,mi,gw):owned.append(ident);r.docker('start',ident)
        r.native.wait_pg(pg,'fixture');time.sleep(3)
        r.require(r.native.sql(pg,'SELECT count(*) FROM lifecycle_disposition_approval;')=='0','Failed C entry already created approval')
        helper('M1-ready','remaining-mixed',owner,gw)
        helper('C-protected','remaining-mixed-protected',owner,gw)
        helper('inspect-disable','remaining-inspection-disable',admin,mi)
        helper('final-readonly','remaining-verify-native',admin,mi)
        r.write(out/'final-core.json',r.native.fingerprints(pg))
        r.write(out/'checkpoint-completion.json',{'group':'mixed','original_checkpoint':str(root.relative_to(r.ROOT)),'source_core_sha256':r.sha(root/'source-core.json'),'restored_core_sha256':r.sha(root/'restored-core.json'),'core_dump_sha256':r.sha(original/'checkpoint/core.dump'),'globals_sha256':r.sha(original/'checkpoint/globals.sql'),'native_archive_sha256':r.sha(original/'checkpoint/native.tar'),'restored_core':pg,'restored_native':mi,'restored_gateway':gw,'restore':'PASS_FULL_CORE_AND_NATIVE','C_protection':'PASS_NATIVE_GOVERNANCE_BLOCK','mixed_outcomes':'NOT_RUN','send_claims':0,'native_delete':0,'source_mutations':0})
    finally:
        seals=[]
        for ident in dict.fromkeys(reversed(owned)):
            state=r.inspect(ident)
            if state['State']['Running']:r.docker('stop','--time','20',ident)
            state=r.inspect(ident);r.require(state['State']['Pid']==0 and not any(state['NetworkSettings']['Ports'].values()),'Unsealed clone')
            seals.append({'id':ident,'name':state['Name'],'pid':0,'ports':state['NetworkSettings']['Ports']})
        r.write(out/'seal.json',{'containers':seals,'retained':True,'native_delete':0,'cleanup':0})

if __name__=='__main__':main()
