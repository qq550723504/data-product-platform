"""Complete only sealed negative receipts/offline validation for frozen f3b5.

The original wrapper lost stderr from Docker logs. Keep its failed seal and all
immutable candidate/source/profile bytes. Never arm IAM, write human text,
change permission, sign topology, open Core/native or consume a send slot.
"""
import importlib.util
import json
from pathlib import Path
import subprocess

spec=importlib.util.spec_from_file_location('f',Path(__file__).with_name('freeze-remaining-executable.py'))
f=importlib.util.module_from_spec(spec);spec.loader.exec_module(f);r=f.r

def logs(cid):
    result=subprocess.run(['docker','logs',cid],capture_output=True)
    r.require(result.returncode==0,'Cannot read retained owner logs')
    return result.stdout+result.stderr

def main():
    packet=f.PUBLIC;private=f.PRIVATE;c=r.load(packet/'candidate.json');bindings=r.load(packet/'bindings.json')
    r.require(c['Source']=='f3b5de45857add3f852bbac0348e8c30ea105ade','Frozen source identity differs')
    r.require(not (packet/'status.json').exists() and (packet/'seal.json').exists(),'Packet continuation boundary differs')
    candidate_hash=r.sha(packet/'candidate.json');owned=[];gateways=[]
    r.write(packet/'continuation.json',{'reason':'Docker logs stderr was not included by original stdout-only wrapper','frozen_source':c['Source'],'candidate_sha256':candidate_hash,'candidate_bindings_replaced':False,'native_delete':0})
    try:
        for g in c['Groups']:
            for ident in (g['Core'],g['Native'],g['Gateway']):r.require(r.inspect(ident)['State']['Pid']==0,'Source no longer sealed')
            r.docker('start',g['Gateway']);gateways.append(g['Gateway'])
        denial=[]
        for g in c['Groups']:
            for stage,ident in g['Helpers'].items():
                folder=private/g['Group']/stage;p=r.load(folder/'permission.json')
                r.require(p['Enabled'] is False and p['CandidateSHA256']==candidate_hash and not (folder/'human-confirmation.txt').exists() and not (folder/('topology-'+stage+'.json')).exists(),'Candidate unexpectedly armed')
                before=r.inspect(ident);r.require(before['State']['Pid']==0,'Owner active')
                if before['State']['Status']=='created':r.docker('start',ident);r.docker('wait',ident)
                state=r.inspect(ident);raw=logs(ident)
                r.require(state['State']['ExitCode']==1 and b'authority denied' in raw,'Actual sealed owner did not refuse before I/O')
                denial.append({'group':g['Group'],'stage':stage,'container':ident,'exit_code':1,'logs':raw.decode(),'source_core_native_stopped':True,'denial_reason':'missing fresh signed readonly topology' if stage.startswith('recover-') else 'send permission false before config/DB/provider','native_delete':0})
        r.write(packet/'sealed-owner-regression.json',denial)
        offline=private/'offline';r.require(not offline.exists(),'Offline helper already allocated');offline.mkdir();(offline/'candidate.json').write_bytes((packet/'candidate.json').read_bytes())
        ident=r.docker('create','--pull','never','--name','dpp-lifecycle307-candidate07-offline','--label','lifecycle307.remaining='+f.EXECUTION,'--network','none','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--tmpfs','/var/lib/postgresql/data:size=1m','--mount',f'type=bind,source={offline},target=/run/private,readonly','--mount',f'type=bind,source={private}/operator,target=/run/operator,readonly','--entrypoint','/run/operator',r.PG,'-stage','check-candidate').decode().strip();owned.append(ident);r.docker('start',ident);r.require(r.docker('wait',ident).strip()==b'0','Frozen Go validator failed');(packet/'offline-validator.txt').write_bytes(logs(ident))
        status={'candidate':f.PROFILE,'execution':f.EXECUTION,'candidate_sha256':candidate_hash,'confirmation_sha256':r.sha(packet/'confirmation-template.txt'),'source':c['Source'],'source_files':len(r.load(packet/'source-freeze.json')['files']),'versions':18,'prospective_targets':7,'survivors':11,'operator_owners':17,'case_owners':7,'bootstrap_owners':15,'human_confirmation':'NOT_RECEIVED','permission_enabled':False,'current_delete_budget':0,'native_delete':0,'hold_release':0,'bypass':0,'cleanup':0,'PR323':'DRAFT','Issue307':'OPEN','full_D1':'NOT_ACCEPTED','send_H2_UNKNOWN_M1_actual':'NOT_RUN','window_is_proposal_not_schedule':True}
        r.write(packet/'status.json',status)
        for b in bindings:
            for action in b['root_owners']:(private/b['group']/('root-'+action)/'status.json').write_bytes((packet/'status.json').read_bytes())
        # Live attester refusal must happen before Docker or signing while no
        # human text exists. Capture both stdout and stderr, never private files.
        result=subprocess.run(['python','-X','utf8',str(f.ROOT/'examples/lifecycle-d1/attest-remaining-executable.py'),'--group','business-races','--stage','P1'],capture_output=True,cwd=f.ROOT)
        r.require(result.returncode!=0 and not (private/'business-races/P1/topology-P1.json').exists(),'Sealed attester unexpectedly signed')
        r.write(packet/'attester-sealed.json',{'exit_code':result.returncode,'human_confirmation_absent':True,'topology_created':False,'native_delete':0})
    finally:
        seals=[]
        for ident in dict.fromkeys(reversed(owned+gateways)):
            state=r.inspect(ident)
            if state['State']['Running']:r.docker('stop','--time','20',ident)
            state=r.inspect(ident);r.require(state['State']['Pid']==0 and not any(state['NetworkSettings']['Ports'].values()),'Continuation left resource active')
            seals.append({'id':ident,'name':state['Name'],'pid':0,'ports':state['NetworkSettings']['Ports']})
        r.write(packet/'seal-completion.json',{'containers':seals,'retained':True,'native_delete':0,'cleanup':0})
    payloads={p.relative_to(f.ROOT).as_posix():r.sha(p) for p in packet.rglob('*') if p.is_file()}
    references={}
    for pattern in ('remaining-runner*','remaining-case*','remaining-executable*','remaining-policy-regression*'):
        for folder in packet.parent.glob(pattern):
            for p in folder.rglob('*'):
                if p.is_file():references[p.relative_to(f.ROOT).as_posix()]=r.sha(p)
    r.write(packet/'evidence-index.json',{'candidate':f.PROFILE,'files':payloads,'references':references})
    print(json.dumps(status))

if __name__=='__main__':main()
