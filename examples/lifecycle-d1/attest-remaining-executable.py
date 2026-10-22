"""Trusted host topology attester; cannot arm IAM, create permission or send.

Requires the exact new human text and fixed window first. Inspect actual Docker
owners/mounts/networks/budget and sign a <=1 minute topology fact. Host key is
never mounted into an operator. MinIO IAM is separately read back before the
trusted bootstrap enables only the frozen policies' identities.
"""
import argparse
import base64
import datetime as dt
import importlib.util
import json
from pathlib import Path
from docker_host_paths import hostpath
import execution_freeze

spec=importlib.util.spec_from_file_location('freeze',Path(__file__).with_name('freeze-remaining-executable.py'))
f=importlib.util.module_from_spec(spec);spec.loader.exec_module(f)
r=f.r

def require(value,message):
    if not value:raise ValueError(message)

def instant(value):return dt.datetime.fromisoformat(value.replace('Z','+00:00'))

def private_files(folder, allowed, required):
    """Directory mounts expose every member, not just the named config file."""
    require(not folder.is_symlink() and not folder.is_junction() and folder.is_dir(),'private owner folder is not regular')
    members=list(folder.iterdir())
    require({p.name for p in members}.issubset(allowed)
            and required.issubset({p.name for p in members})
            and all(p.is_file() and not p.is_symlink() and not p.is_junction() for p in members),
            'unexpected private file, missing member, directory or symlink')

def helper_private_contract(folder,stage):
    required={'operator.json','candidate.json','permission.json','confirmation-template.txt'}
    allowed=required|{'human-confirmation.txt','topology-'+stage+'.json'}
    if stage.startswith('recover-'):required=required|{'recovery.json'};allowed=allowed|{'recovery.json'}
    if stage=='recover-M1B':allowed=allowed|{'M1-mixed-checkpoint.json'}
    private_files(folder,allowed,required)

def authority(candidate,permission,human,template,now,recovery):
    require(human==template and bool(template),'new exact human confirmation absent/different')
    require(permission['Execution']==candidate['Execution']==f.EXECUTION and permission['Profile']==candidate['Profile']==f.PROFILE,'foreign authorization')
    require(permission['NotBefore']==candidate['NotBefore']==f.START and permission['ExpiresAt']==candidate['NotAfter']==f.END,'window moved')
    require(permission['MaxPerTarget']==1 and permission['MaxTotal']==6,'budget differs')
    require(instant(permission['ConfirmedAt'])<=now,'confirmation is future-dated')
    require(now>=instant(f.START) and now<instant(f.END)+(dt.timedelta(hours=2) if recovery else dt.timedelta()),'outside absolute authority window')
    if not recovery:require(permission['Enabled'] is True,'send permission is sealed')

def helper_contract(h,group,stage,folder,operator,budget_name):
    require(h['Id']==group['Helpers'][stage],'helper full CID differs')
    host=h['HostConfig'];require(host['ReadonlyRootfs'] and not host['Privileged'] and host['RestartPolicy']['Name']=='no' and not host.get('AutoRemove'),'helper security settings drifted')
    require('ALL' in host['CapDrop'] and any('no-new-privileges' in x for x in host['SecurityOpt']),'helper caps/security-opt drifted')
    require(not host.get('PortBindings') and not any(h['NetworkSettings']['Ports'].values()),'helper exposes ports')
    require(host['NetworkMode']=='container:'+group['Gateway'],'helper network namespace differs')
    require(h['Config']['Entrypoint']==['/run/operator'] and h['Config']['Cmd']==['-stage',stage],'helper command drifted')
    mounts={m['Destination']:m for m in h['Mounts']}
    require(len(mounts)==len(h['Mounts']) and set(mounts)=={'/run/private','/run/operator','/run/output','/run/budget'},'helper mount set differs')
    for target,path in [('/run/private',folder),('/run/operator',operator)]:
        require(mounts[target]['Type']=='bind' and hostpath(mounts[target]['Source'])==path.resolve() and mounts[target]['RW'] is False,'readonly config/binary mount differs')
    require(mounts['/run/output']['Type']=='bind' and mounts['/run/output']['RW'] is True and hostpath(mounts['/run/output']['Source'])==(folder.parent/'outputs'/stage).resolve(),'output mount differs')
    require(mounts['/run/budget']['Type']=='volume' and mounts['/run/budget']['RW'] and mounts['/run/budget']['Name']==budget_name,'durable budget mount differs')
    require(not any('docker.sock' in m['Source'].lower() or 'docker_engine' in m['Source'].lower() for m in h['Mounts']),'Docker socket exposed')
    require(not any(x.startswith(('MINIO_ROOT_','ROOT_ACCESS=','ROOT_SECRET=')) for x in h['Config']['Env']),'root credentials in helper environment')

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--group',required=True,choices=['business-races','late-hold','mixed']);parser.add_argument('--stage',required=True);args=parser.parse_args()
    candidate=r.load(f.PUBLIC/'candidate.json');require(r.sha(f.PUBLIC/'candidate.json')==r.load(f.PUBLIC/'status.json')['candidate_sha256'],'public candidate changed')
    group=next(g for g in candidate['Groups'] if g['Group']==args.group);require(args.stage in group['Helpers'],'unbound stage')
    folder=f.PRIVATE/args.group/args.stage;permission=r.load(folder/'permission.json')
    # Refuse before Docker inspection or signing while this candidate is sealed.
    authority(candidate,permission,(folder/'human-confirmation.txt').read_bytes(),(f.PUBLIC/'confirmation-template.txt').read_bytes(),r.utc(),args.stage.startswith('recover-'))
    execution_freeze.authorized_gate(f.ROOT,f.PRIVATE,f.PUBLIC,candidate)
    require(permission['CandidateSHA256']==r.sha(f.PUBLIC/'candidate.json') and permission['ConfirmationSHA256']==r.sha(f.PUBLIC/'confirmation-template.txt') and permission['BinarySHA256']==candidate['SenderBinarySHA256'],'confirmation candidate/binary hash differs')
    require((folder/'candidate.json').read_bytes()==(f.PUBLIC/'candidate.json').read_bytes() and r.sha(folder/'operator.json')==group['ConfigSHA256'][args.stage] and r.sha(f.PRIVATE/'operator')==candidate['SenderBinarySHA256'],'frozen helper files drifted')
    config=r.load(folder/'operator.json');require(not any(k in config for k in ('RootAccess','RootSecret')),'root credentials in helper config')
    helper_private_contract(folder,args.stage)
    if args.stage.startswith('recover-'):require(not config['DeleteSecret'] and not config['WirePrivateKey'],'recovery has send credentials')
    helper=r.inspect(group['Helpers'][args.stage]);helper_contract(helper,group,args.stage,folder,f.PRIVATE/'operator','dpp-lifecycle307-'+next(x['Run'][:8] for x in r.load(f.ROOT/'apps/platform/internal/lifecycle/pilot/remaining_scope.json') if x['Group']==args.group)+'-'+f.GENERATION+'-budget')
    require(helper['Config']['User']=='10001:10001' and helper['HostConfig'].get('Tmpfs')=={'/tmp':'','/var/lib/postgresql/data':'size=1m'},'current helper is not the exact unprivileged owner')
    images={'Core':r.PG,'Native':r.MI,'Gateway':r.GW}
    for role,image in images.items():
        state=r.inspect(group[role]);require(state['Id']==group[role] and state['Image']==image and state['State']['Running'] and state['State']['Pid']>0,'source role/image not active and bound')
        require(state['HostConfig']['RestartPolicy']['Name']=='no' and not state['HostConfig'].get('PortBindings') and not any(state['NetworkSettings']['Ports'].values()),'source role exposed/restarting')
        require({n['NetworkID'] for n in state['NetworkSettings']['Networks'].values()}=={group['Network']},'source network differs')
    gateway=r.inspect(group['Gateway']);mount=next(m for m in gateway['Mounts'] if m['Destination']=='/etc/caddy/Caddyfile')
    require(mount['Type']=='bind' and mount['RW'] is False and r.sha(hostpath(mount['Source']))==group['GatewaySHA256'],'active gateway role/config differs')
    network=json.loads(r.docker('network','inspect',group['Network']))[0];require(network['Id']==group['Network'] and network['Internal'],'network is not private/internal')
    volume=next(m['Name'] for m in helper['Mounts'] if m['Destination']=='/run/budget');budget=r.inspect(volume)
    require(budget['Labels'].get('lifecycle307.remaining')==f.EXECUTION,'budget volume ownership differs')
    # Cross-group order is derived from fresh immutable Core receipts, not a
    # host progress file. It is checked before signing the next slot.
    import remaining_sequence
    sequence = remaining_sequence.observe(candidate, f.PRIVATE, args.group, args.stage, r.inspect, r.native.sql, r.load)
    binding=next(b for b in r.load(f.PUBLIC/'bindings.json') if b['group']==args.group)
    mounted=r.docker_text('ps','-aq','--no-trunc','--filter','volume='+volume).split()
    expected={*group['Helpers'].values(),binding['budget_initializer']}
    require(len(mounted)==len(set(mounted)) and set(mounted)==expected,'budget complete mount graph expanded or lost owner')
    initializer=r.inspect(binding['budget_initializer'])
    require(initializer['Id']==binding['budget_initializer'] and not initializer['State']['Running'] and initializer['State']['Pid']==0
            and initializer['Config']['User']=='0:0' and initializer['Config']['Entrypoint']==['/bin/sh']
            and initializer['Config']['Cmd']==['-e','/run/budget-init.sh'] and initializer['HostConfig']['NetworkMode']=='none'
            and initializer['HostConfig']['RestartPolicy']['Name']=='no' and not initializer['HostConfig'].get('PortBindings')
            and not any(initializer['NetworkSettings']['Ports'].values()),'budget initializer changed or still active')
    initial_mounts={m['Destination']:m for m in initializer['Mounts']}
    require(len(initial_mounts)==len(initializer['Mounts']) and set(initial_mounts)=={'/run/budget','/run/budget-init.sh'}
            and initial_mounts['/run/budget']['Type']=='volume' and initial_mounts['/run/budget']['Name']==volume and initial_mounts['/run/budget']['RW'] is True
            and initial_mounts['/run/budget-init.sh']['Type']=='bind' and initial_mounts['/run/budget-init.sh']['RW'] is False
            and r.sha(hostpath(initial_mounts['/run/budget-init.sh']['Source']))==binding['budget_initializer_script_sha256'],'budget initialization scope/code differs')
    remaining_sequence.budget_archive(r.docker('cp',helper['Id']+':/run/budget','-'),candidate,args.group,initial=False)
    # Late import: signature implementation uses the maintained cryptography OSS.
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat
    key=Ed25519PrivateKey.from_private_bytes((f.PRIVATE/'topology-key.private').read_bytes())
    require(key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw).hex()==candidate['PublicKey'],'host key differs')
    body={'Execution':f.EXECUTION,'CandidateSHA256':r.sha(f.PUBLIC/'candidate.json'),'Group':args.group,'Stage':args.stage,'Helper':helper['Id'],'Core':group['Core'],'Native':group['Native'],'Gateway':group['Gateway'],'Network':group['Network'],'BinarySHA256':candidate['SenderBinarySHA256'],'ConfigSHA256':group['ConfigSHA256'][args.stage],'At':r.utc().isoformat(),'NoPorts':True,'NoRoot':True,'NoSocket':True,'ReadOnly':True,'NoRestart':True,'Internal':True,'Sequence':sequence}
    encoded=f.compact(body);proof={'Body':base64.b64encode(encoded).decode(),'Signature':base64.b64encode(key.sign(encoded)).decode()}
    # Fresh attestations may replace only this ephemeral proof, never permission,
    # confirmation, candidate, budget or immutable case/checkpoint evidence.
    path=folder/('topology-'+args.stage+'.json');path.write_bytes(f.compact(proof)+b'\n')
    print(json.dumps({'status':'FRESH_TOPOLOGY_ATTESTED','group':args.group,'stage':args.stage,'helper':helper['Id'],'at':body['At'],'native_delete':0}))

if __name__=='__main__':main()
