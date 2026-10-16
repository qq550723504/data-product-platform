"""Resume only the retained H0 metadata denial after fixing Caddy ordering."""
import base64
import importlib.util
import json
from pathlib import Path
import time
import uuid
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('runner',ROOT/'examples/lifecycle-d1/prepare-remaining-runner.py')
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
require,load,write,sha,docker,inspect=r.require,r.load,r.write,r.sha,r.docker,r.inspect

def main():
    require(not r.call(['git','status','--porcelain']).strip(),'Commit source before native continuation')
    out=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-case02';require(not out.exists(),'Continuation already allocated');out.mkdir();business=out/'business';business.mkdir()
    private=r.PRIVATE/'remaining-case01/late-hold';c=load(private/'owner/remaining.json');prefix='dpp-lifecycle307-754ea8f0-remaining-regression';original=load(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-case01/late-hold/cases.json');require(original['exit_code']==1,'Different retained failure')
    gateway=private/'Caddyfile';old=gateway.read_text();require('route {' not in old,'Gateway already changed');gateway.write_text(old.replace(':9000 {',':9000 {\n route {').rstrip()[:-1]+' }\n}\n')
    (out/'gateway.Caddyfile').write_bytes(gateway.read_bytes())
    services=[prefix+'-pg',prefix+'-minio',prefix+'-gateway'];owned=[]
    def helper(name,mode,config):
        ident=docker('create','--pull','never','--name',prefix+'-02-'+name,'--label','lifecycle307.remaining='+c['Batch'],'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--network','container:'+services[2],'--mount',f'type=bind,source={config},target=/run/private,readonly','--mount',f'type=bind,source={r.PRIVATE}/remaining-case01/reader,target=/run/reader,readonly','--mount',f'type=bind,source={business},target=/run/output','--entrypoint','/run/reader',r.PG,'-mode',mode).decode().strip();owned.append(ident);docker('start',ident);code=int(docker('wait',ident));(out/(name+'.stdout.txt')).write_bytes(docker('logs',ident));write(out/(name+'.json'),{'container':ident,'exit_code':code,'binary_sha256':sha(r.PRIVATE/'remaining-case01/reader'),'mode':mode,'delete':0});require(code==0,name+' failed');return ident
    try:
        for service in services:require(not inspect(service)['State']['Running'],'Prior service active');owned.append(service);docker('start',service)
        r.native.wait_pg(services[0],'fixture');time.sleep(3)
        require(r.native.sql(services[0],"SELECT (SELECT count(*) FROM lifecycle_content_hold),(SELECT count(*) FROM lifecycle_provider_attempt),(SELECT count(*) FROM lifecycle_provider_send_claim);")=='0|0|0','Failure progressed beyond metadata preflight')
        case=helper('cases','remaining-late',private/'owner')
        # Original native dump/restore proved delete identity disabled. Only the
        # inspection identity was enabled, through a fixed SDK admin operation.
        owners={case:sha(r.PRIVATE/'remaining-case01/reader'),original['container']:original['binary_sha256']}
        for ident in owners:
            current=inspect(ident);require(current['State']['Pid']==0 and current['HostConfig']['RestartPolicy']['Name']=='no','Unclosed owner')
        c['Base']['Enabled']=False;(private/'owner/remaining.json').write_text(json.dumps(c)+'\n')
        key=Ed25519PrivateKey.generate();binding=sha(business/'remaining-H1.json')
        body={'Execution':c['Batch'],'Nonce':str(uuid.uuid4()),'CandidateSHA256':binding,'Group':'late-hold','At':r.utc().isoformat(),'Owners':owners,'StoppedPIDZero':True,'NoRestart':True,'ConfigDisabled':True,'DeleteIdentityDisabled':True,'GatewaySealed':True}
        encoded=json.dumps(body,separators=(',',':')).encode();proof={'Body':base64.b64encode(encoded).decode(),'Signature':base64.b64encode(key.sign(encoded)).decode()}
        close=private/'close02';close.mkdir();cc=json.loads(json.dumps(c));cc['Base']['Enabled']=True;cc.update(ClosurePublicKey=key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex(),ClosureCandidateSHA256=binding,ClosureOwners=owners)
        write(close/'remaining.json',cc);(close/'manifest.json').write_bytes((private/'owner/manifest.json').read_bytes());write(close/'closed-H1.json',proof)
        helper('closed-H1','remaining-close-H1',close)
        write(out/'final-core.json',r.native.fingerprints(services[0]))
    finally:
        seals=[]
        for ident in dict.fromkeys(reversed(owned)):
            state=inspect(ident)
            if state['State']['Running']:docker('stop','--time','20',ident)
            state=inspect(ident);seals.append({'id':state['Id'],'name':state['Name'],'pid':state['State']['Pid'],'status':state['State']['Status'],'ports':state['NetworkSettings']['Ports']});require(state['State']['Pid']==0 and not any(state['NetworkSettings']['Ports'].values()),'Unsealed resource')
        write(out/'seal.json',{'containers':seals,'delete':0,'cleanup':0})

if __name__=='__main__':main()
