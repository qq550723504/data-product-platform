"""One fresh non-delete execution on the already restored run03 regression copy.

Run03 stopped before any task/pin/restriction, at a one-plan/run basis constraint.
Keep that failed basis, use new per-plan identities, and never replace source Core.
"""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import time
import uuid

ROOT=Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('runner',ROOT/'examples/lifecycle-d1/prepare-remaining-runner.py')
r=importlib.util.module_from_spec(spec);spec.loader.exec_module(r)
require,load,write,sha,docker,inspect=r.require,r.load,r.write,r.sha,r.docker,r.inspect

def main():
    require(not r.call(['git','status','--porcelain']).strip(),'Commit source before native replay')
    control=r.PRIVATE/'remaining-runner04';require(not control.exists(),'Finite replay already allocated');control.mkdir()
    out=ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner04';out.mkdir();regout=out/'regression';regout.mkdir()
    prior=r.PRIVATE/'remaining-runner03/regression';worker=load(prior/'remaining.json');worker['Batch']=str(uuid.uuid4());worker['Rehearsal']=True
    write(control/'remaining.json',worker);(control/'manifest.json').write_bytes((prior/'manifest.json').read_bytes())
    (regout/'remaining-state.json').write_bytes((ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner03/regression/remaining-state.json').read_bytes())
    source=r.call(['git','rev-parse','HEAD']).decode().strip();env=os.environ.copy();env.update(GOOS='linux',GOARCH='amd64',CGO_ENABLED='0')
    result=subprocess.run(['go','build','-trimpath','-buildvcs=false','-o',str(control/'reader'),'./cmd/lifecycle-fixture-reader'],cwd=ROOT/'apps/platform',env=env,capture_output=True)
    require(result.returncode==0,'Build failed');write(out/'build.json',{'source_commit':source,'binary_sha256':sha(control/'reader'),'batch':worker['Batch'],'rehearsal':True,'delete':0})
    prefix=r.PROJECT+'-remaining-regression';services=[prefix+'-pg',prefix+'-minio',prefix+'-gateway',prefix+'-redis'];owned=[]
    topology=load(ROOT/'docs/acceptance/evidence/issue307-d1/remaining-runner03/topology.json')
    for service,key in zip(services,('core','minio','gateway','redis')):require(inspect(service)['Id']==topology[key] and not inspect(service)['State']['Running'],'Restored resource identity/state changed')
    def helper(name,rooted=False,netns=None,output=None):
        config=r.PRIVATE/'remaining-runner02/admin' if rooted else control
        args=['create','--pull','never','--name',r.PROJECT+'-remaining-04-'+name,'--label','lifecycle307.remaining='+worker['Batch'],'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/tmp','--network','container:'+(netns or prefix+'-gateway'),'--mount',f'type=bind,source={config},target=/run/private,readonly','--mount',f'type=bind,source={control}/reader,target=/run/reader,readonly','--mount',f'type=bind,source={output or regout},target=/run/output','--mount',f'type=bind,source={ROOT}/industry-packs,target=/run/repo/industry-packs,readonly','--mount',f'type=bind,source={r.PRIVATE}/remaining-runner02/owner/rules,target=/run/rules,readonly','--entrypoint','/run/reader',r.PG,'-mode','remaining-'+('seal' if rooted else 'business')]
        ident=docker(*args).decode().strip();owned.append(ident);docker('start',ident);code=int(docker('wait',ident));(out/(name+'.stdout.txt')).write_bytes(docker('logs',ident));write(out/(name+'.json'),{'container':ident,'exit_code':code,'delete':0});require(code==0,name+' failed')
    try:
        for service in services:owned.append(service);docker('start',service)
        r.native.wait_pg(services[0],'fixture');time.sleep(3)
        state=load(regout/'remaining-state.json');p1=str(uuid.UUID(state['P1']));p2=str(uuid.UUID(state['P2']))
        require(r.native.sql(services[0],f"SELECT (SELECT count(*) FROM lifecycle_reader_pin WHERE execution_id IN('{p1}','{p2}')),(SELECT count(*) FROM lifecycle_content_use_restriction),(SELECT count(*) FROM lifecycle_provider_attempt);")=='0|0|0','Run03 progressed beyond initial basis failure')
        helper('business')
        helper('clone-seal',True,services[1])
        sourceMinio=r.PROJECT+'-minio';owned.append(sourceMinio);docker('start',sourceMinio);time.sleep(3);helper('source-seal',True,sourceMinio,out)
        write(out/'regression-final-core.json',r.native.fingerprints(services[0]))
    finally:
        seals=[]
        for name in dict.fromkeys(reversed(owned)):
            state=inspect(name)
            if state['State']['Running']:docker('stop','--time','20',name)
            state=inspect(name);require(state['State']['Pid']==0 and not any(state['NetworkSettings']['Ports'].values()),'Resource unsealed')
            seals.append({'id':state['Id'],'name':state['Name'],'status':state['State']['Status'],'pid':state['State']['Pid'],'ports':state['NetworkSettings']['Ports']})
        write(out/'seal.json',{'containers':seals,'retained':True,'delete':0,'cleanup':0})

if __name__=='__main__':main()
