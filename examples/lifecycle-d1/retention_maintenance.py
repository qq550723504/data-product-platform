"""Exact-two maintenance host: inspect/attest/close only, never start or enable.

Use the existing official Docker CLI, cryptography Ed25519, and typed evidence
rules. Actual binding allocation, new human authority and source starts are
separate operations. A default worker binary has no trusted host-key anchor.
"""
import argparse
import base64
import datetime as dt
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import uuid
import retention_staging as staging
from evidence_indexes import unlinked_absolute

ROOT=Path(__file__).resolve().parents[2]
KIND='RETENTION_EXTENSION_EXACT_TWO_V1'
spec=importlib.util.spec_from_file_location('renewal_preparation',Path(__file__).with_name('prepare-retention-renewal.py'))
proposal=importlib.util.module_from_spec(spec);spec.loader.exec_module(proposal)
need=proposal.protection.need

def sha(raw):return hashlib.sha256(raw).hexdigest()
def canonical(value):return json.dumps(value,sort_keys=True,separators=(',',':'),ensure_ascii=False).encode()
def load(path):
    path=unlinked_absolute(path)
    need(path.is_file() and not path.is_symlink() and not path.is_junction() and path.stat().st_nlink==1,'MAINTENANCE_INPUT_NOT_REGULAR')
    def pairs(items):
        result={}
        for key,value in items:
            need(key not in result,'MAINTENANCE_DUPLICATE_JSON_KEY');result[key]=value
        return result
    return json.loads(path.read_bytes().decode('utf-8','strict'),object_pairs_hook=pairs)

def fixed_bindings(b,host):
    need(set(b)=={'Kind','MaintenanceID','NotBefore','NotAfter','PlanSHA256','BinarySHA256','HostPublicSHA256','BudgetID','Owners'},'MAINTENANCE_BINDING_FIELDS')
    need(str(uuid.UUID(b['MaintenanceID']))==b['MaintenanceID'] and uuid.UUID(b['MaintenanceID']).int!=0 and b['MaintenanceID']!='4fad1007-6595-4037-a7a5-1d8d38cb8bcc','MAINTENANCE_NEW_ID')
    start,end=map(proposal.protection.instant,(b['NotBefore'],b['NotAfter']))
    need(start<end<=proposal.protection.instant(proposal.DEADLINE) and end-start<=dt.timedelta(minutes=40),'MAINTENANCE_BOUND_WINDOW')
    need(b['PlanSHA256']=='ae85a69fee49b556b88c9437be4c4923a370213b9c9c0e92466328932f826d2f','MAINTENANCE_EXACT_PLAN')
    for value in (b['BinarySHA256'],b['HostPublicSHA256'],b['BudgetID']):need(type(value) is str and len(value)==64 and all(c in '0123456789abcdef' for c in value),'MAINTENANCE_SHA256')
    need(b['Kind']==KIND and len(b['Owners'])==2 and {x['Group'] for x in b['Owners']}==set(proposal.TARGETS),'MAINTENANCE_TARGET_GROUPS')
    need(set(host)=={'bindings_sha256','owners','closed_cids','budget_name','binary','volume_metadata','_bindings_raw'},'MAINTENANCE_HOST_FIELDS')
    need(host['bindings_sha256']==sha(host['_bindings_raw']),'MAINTENANCE_HOST_BINDING')
    need(set(host['owners'])==set(proposal.TARGETS) and len(set(x['CID'] for x in b['Owners']))==2,'MAINTENANCE_OWNER_SET')
    need(type(host['closed_cids']) is list and len(set(host['closed_cids']))==len(host['closed_cids']),'MAINTENANCE_CLOSED_SET')
    for owner in b['Owners']:
        group=owner['Group'];item=host['owners'][group]
        need(owner['Native']==proposal.TARGETS[group][2] and owner['CID']==item['cid'] and owner['DescriptorSHA256']==item['descriptor_sha256'],'MAINTENANCE_FIXED_IDENTITY')
        need(owner['CID'] not in host['closed_cids'],'MAINTENANCE_SELECTED_OWNER_CLOSED_SET')
        for ident in (owner['CID'],owner['Native'],*host['closed_cids']):
            need(type(ident) is str and len(ident)==64 and all(c in '0123456789abcdef' for c in ident),'MAINTENANCE_FULL_CID')
    return b

def descriptor(item):
    # Complete immutable Docker configuration/mount/network descriptors, never
    # print raw Env. State/PID are checked separately, not hashed as immutable.
    return {key:item[key] for key in ('Id','Image','Path','Args','Config','HostConfig','Mounts')}

def closed(item):
    return (item['State']['Running'] is False and item['State']['Pid']==0 and
            not item['NetworkSettings']['Ports'] and item['HostConfig']['RestartPolicy']['Name']=='no')

def docker(*args):
    result=subprocess.run(['docker',*args],capture_output=True,timeout=30)
    need(result.returncode==0 and type(result.stdout) is bytes,'MAINTENANCE_DOCKER_FAILED')
    return result.stdout

def inspect(ident):
    rows=json.loads(docker('inspect',ident).decode('utf-8','strict'))
    need(len(rows)==1 and rows[0]['Id']==ident,'MAINTENANCE_INSPECT_IDENTITY')
    return rows[0]

def topology(b,host,group,get=inspect,list_items=None):
    fixed_bindings(b,host)
    selected=next(x for x in b['Owners'] if x['Group']==group)
    item=host['owners'][group];actual=get(selected['CID']);native=get(selected['Native'])
    need(actual['State']['Running'] and actual['State']['Pid']>0 and native['State']['Running'] and native['State']['Pid']>0,'MAINTENANCE_OWNER_NOT_RUNNING')
    need(sha(canonical(descriptor(actual)))==selected['DescriptorSHA256'],'MAINTENANCE_DESCRIPTOR_DRIFT')
    hc=actual['HostConfig'];cfg=actual['Config']
    need(actual['Path']=='/runtime/retention' and actual['Args']==['-wait'] and not actual['NetworkSettings']['Ports'] and hc['RestartPolicy']['Name']=='no' and not hc['Privileged'] and hc['ReadonlyRootfs'] and
         hc['NetworkMode']=='container:'+selected['Native'] and cfg['User']=='10001:10001' and set(hc['CapDrop'])=={'ALL'} and
         set(hc['SecurityOpt'])=={'no-new-privileges:true'} and hc['Tmpfs']=={'/tmp':'rw,noexec,nosuid,size=16m'},'MAINTENANCE_SANDBOX')
    expected={'/run/private':(item['private_volume'],False,'volume'),
              '/run/output':(item['output_volume'],True,'volume'),
              '/run/budget':(host['budget_name'],True,'volume'),
              '/runtime/retention':(str(unlinked_absolute(host['binary'])),False,'bind')}
    mounts={x['Destination']:x for x in actual['Mounts']}
    need(len(mounts)==len(actual['Mounts']) and set(mounts)==set(expected),'MAINTENANCE_MOUNT_SET')
    for dest,(source,rw,kind) in expected.items():
        m=mounts[dest];need(m['Type']==kind and m['RW'] is rw,'MAINTENANCE_MOUNT_DRIFT')
        need((m['Name']==source and m['Source']==host['volume_metadata'][source]['Mountpoint']) if kind=='volume' else unlinked_absolute(m['Source'])==unlinked_absolute(source),'MAINTENANCE_MOUNT_DRIFT')
    for owner in b['Owners']:
        if owner['Group']!=group:
            previous=get(owner['CID']);need(closed(previous),'MAINTENANCE_OTHER_OWNER_ACTIVE')
            if group=='mixed':
                terminal=load(Path(host['owners']['business-races']['output'])/'terminal.json')
                need(previous['State'].get('ExitCode')==0 and terminal.get('MaintenanceID')==b['MaintenanceID'] and
                     terminal.get('BindingsSHA256')==host['bindings_sha256'] and terminal.get('Status')=='PASS_EXACT_RETENTION_READBACK' and
                     terminal.get('readback_confirmed') is True,'MAINTENANCE_PREVIOUS_NOT_TRUSTED_CLOSED')
    for ident in host['closed_cids']:need(closed(get(ident)),'MAINTENANCE_RETAINED_OWNER_ACTIVE')
    need(sha(Path(host['binary']).read_bytes())==b['BinarySHA256'] and sha((Path(item['private'])/'root.json').read_bytes())==selected['ConfigSHA256'],'MAINTENANCE_BINARY_CONFIG_DRIFT')
    allowed={'bindings.json','root.json','approved-proposal.json','host-public.key','authorization.json','user-message-verbatim.txt','permission.json','topology.json'}
    for mounted in host['owners'].values():
        folder=unlinked_absolute(mounted['private'])
        need(folder.is_dir() and {p.name for p in folder.iterdir()}==allowed,'MAINTENANCE_PRIVATE_FILE_SET')
        for path in folder.iterdir():
            clean=unlinked_absolute(path)
            need(clean.is_file() and clean.stat().st_nlink==1,'MAINTENANCE_PRIVATE_FILE_LINK')
    private_files={'bindings.json','root.json','approved-proposal.json','host-public.key','authorization.json','user-message-verbatim.txt','permission.json','topology.json'}
    expected_mounts={host['budget_name']:{x['CID'] for x in b['Owners']}}
    for mounted in host['owners'].values():
        for role in ('private','output'):
            volume=mounted[role+'_volume']
            need(volume not in expected_mounts,'MAINTENANCE_VOLUME_ALIAS')
            expected_mounts[volume]={mounted['cid']}
            pinned=host['volume_metadata'][volume]
            current=json.loads(docker('volume','inspect',volume).decode('utf-8','strict'))
            need(len(current)==1 and current[0]==pinned,'MAINTENANCE_VOLUME_METADATA_DRIFT')
            staging.paths(pinned)
            staging.validate_directory(pinned,mounted[role],files=private_files if role=='private' else None)
    need(set(host['volume_metadata'])==set(expected_mounts),'MAINTENANCE_VOLUME_SET')
    budget=host['budget_name'];current=json.loads(docker('volume','inspect',budget).decode('utf-8','strict'))
    need(len(current)==1 and current[0]==host['volume_metadata'][budget],'MAINTENANCE_BUDGET_METADATA_DRIFT')
    _,budget_path=staging.paths(current[0])
    staging.validate_directory(current[0],budget_path)
    expected_budget={x['CID'] for x in b['Owners']}
    # Inventory is the complete host Docker inventory, not a provided subset.
    if list_items is None:
        ids=docker('ps','-aq','--no-trunc').decode('utf-8','strict').splitlines()
        all_items=[get(ident) for ident in ids]
    else:all_items=list_items()
    for name,wanted in expected_mounts.items():
        mounted={x['Id'] for x in all_items if any(m.get('Type')=='volume' and m.get('Name')==name for m in x['Mounts'])}
        need(mounted==wanted,'MAINTENANCE_VOLUME_MOUNT_SET')
    return selected

def attest(b,host,group,key_path,at,get=inspect,list_items=None):
    need(proposal.protection.instant(b['NotBefore'])<=at<proposal.protection.instant(b['NotAfter'])<=proposal.protection.instant(proposal.DEADLINE),'MAINTENANCE_PROOF_WINDOW')
    o=topology(b,host,group,get,list_items)
    private_key=unlinked_absolute(key_path)
    need(private_key.is_file() and private_key.stat().st_nlink==1,'MAINTENANCE_HOST_KEY_LINK')
    for item in host['owners'].values():
        need(not private_key.is_relative_to(Path(item['private']).resolve()),'MAINTENANCE_HOST_KEY_MOUNTED')
        need(not private_key.is_relative_to(Path(item['output']).resolve()),'MAINTENANCE_HOST_KEY_OUTPUT_MOUNTED')
    from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey
    from cryptography.hazmat.primitives.serialization import Encoding,PublicFormat
    key=Ed25519PrivateKey.from_private_bytes(private_key.read_bytes())
    need(sha(key.public_key().public_bytes(Encoding.Raw,PublicFormat.Raw))==b['HostPublicSHA256'],'MAINTENANCE_HOST_KEY_DRIFT')
    payload={'Kind':KIND,'MaintenanceID':b['MaintenanceID'],'BindingsSHA256':host['bindings_sha256'],'Group':group,'Owner':o['CID'],'Native':o['Native'],
             'DescriptorSHA256':o['DescriptorSHA256'],'ConfigSHA256':o['ConfigSHA256'],'BudgetID':b['BudgetID'],'At':at.isoformat().replace('+00:00','Z')}
    raw=canonical(payload)
    return {'Body':base64.b64encode(raw).decode(),'Signature':base64.b64encode(key.sign(raw)).decode()}

def seal(b,host,get=inspect,call=docker):
    fixed_bindings(b,host);results=[]
    # A failed permission archive never vetoes physical closure of other owners.
    for group,item in host['owners'].items():
        try:
            path=Path(item['private'])/'permission.json';raw=path.read_bytes();before=load(path)
            archive=Path(item['output'])/('permission-before-seal-'+sha(raw)+'.json')
            if not archive.exists():
                with archive.open('xb') as stream:stream.write(raw);stream.flush();os.fsync(stream.fileno())
            need(archive.read_bytes()==raw,'MAINTENANCE_ARCHIVE_DRIFT')
            before['Enabled']=False
            # Current permission is an explicitly mutable control, not a fact.
            path.write_bytes(canonical(before)+b'\n');need(load(path)['Enabled'] is False,'MAINTENANCE_REVOKE_FAILED')
            results.append({'group':group,'permission_closed':True})
        except Exception as error:results.append({'group':group,'permission_closed':False,'error_type':type(error).__name__})
    ids=list(dict.fromkeys([x['CID'] for x in b['Owners']]+[x['Native'] for x in b['Owners']]))
    for ident in ids:
        try:
            try:stopped=closed(get(ident))
            except Exception:stopped=False
            if not stopped:
                try:call('stop','--time','10',ident)
                except Exception:pass
            need(closed(get(ident)),'MAINTENANCE_CLOSE_UNCONFIRMED');results.append({'cid':ident,'closed':True})
        except Exception as error:results.append({'cid':ident,'closed':False,'error_type':type(error).__name__})
    return {'status':'PASS_PHYSICAL_MAINTENANCE_CLOSURE' if all(x.get('closed',x.get('permission_closed',False)) for x in results) else 'UNCONFIRMED_MAINTENANCE_CLOSURE',
            'results':results,'source_start':0,'enable':0,'delete':0,'retention_write':0}

def main():
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('action',choices=['check','attest','seal'])
    parser.add_argument('--bindings',type=Path,required=True);parser.add_argument('--bindings-sha256',required=True)
    parser.add_argument('--host',type=Path,required=True);parser.add_argument('--host-sha256',required=True)
    parser.add_argument('--group',choices=list(proposal.TARGETS));parser.add_argument('--private-key',type=Path);parser.add_argument('--output',type=Path,required=True)
    args=parser.parse_args();need(sha(args.bindings.read_bytes())==args.bindings_sha256 and sha(args.host.read_bytes())==args.host_sha256,'MAINTENANCE_PINNED_INPUT_DRIFT')
    b=load(args.bindings);host=load(args.host);host['_bindings_raw']=args.bindings.read_bytes();fixed_bindings(b,host)
    if args.action=='check':result={'status':'PASS_FILE_BINDINGS_ONLY_NOT_AUTHORIZED','source_provider_io':0}
    elif args.action=='attest':result=attest(b,host,args.group,args.private_key,dt.datetime.now(dt.timezone.utc))
    else:result=seal(b,host)
    with args.output.open('x',encoding='utf-8',newline='\n') as stream:json.dump(result,stream,ensure_ascii=False,indent=2);stream.write('\n')
    print(json.dumps({'action':args.action,'output_sha256':sha(args.output.read_bytes()),'authority':'NO_ENABLE_NO_START_NO_DELETE'}))

if __name__=='__main__':main()
