"""Exact retired-test-volume receipts, layered on the container archive contract."""
from pathlib import Path
import subprocess
import retired_resources as r

SCHEMA='d1-retired-test-volumes-v1'

def reference(spec):
    raw=r.audit.checked_bytes(r.audit.unlinked_absolute(spec['path']),spec['sha256'])
    r.need(len(raw)==spec['bytes'],'volume proof bytes differ')
    return raw

def checked(path,digest,parent,parent_digest):
    plan=r.audit.decode(r.audit.checked_bytes(r.audit.unlinked_absolute(path),digest))
    r.need(plan['schema']==SCHEMA and plan['parent_plan_sha256']==parent_digest
           and plan['provider_delete']==0 and plan['authority']['human_message_id'],'volume maintenance scope differs')
    approved=set(plan['remove']);kept=set(plan['preserve']);candidates=set(plan['candidate_volumes'])
    r.need(approved and not approved&kept and approved|kept==candidates and candidates<=set(parent['volumes']),
           'volume candidate/decision partition differs')
    budgets={b['volume'] for b in parent['budgets']}
    r.need(not approved&budgets,'budget volume may never be removed')
    for name,entry in plan['remove'].items():
        r.need(entry['metadata']==parent['volumes'][name]['metadata']
               and set(parent['volumes'][name]['mounted_before'])<=set(parent['retired_historical']),
               'volume has retained user or identity drift')
        content=r.audit.decode(reference(entry['content']))
        r.need(content['volume']==name and content['reader'] in parent['retired_historical']
               and content['exit_code']==0 and content['source_start']==content['provider_calls']==0,
               'unbound offline volume content')
        for spec in entry.get('references',[]):reference(spec)
        if entry['reason']=='EMPTY_NO_FILE_DATA':
            r.need(all(f['type'] in ('0','5') and f['size']==0 for f in content['files']), 'empty volume contains data/link/special file')
        elif entry['reason']=='REPRODUCIBLE_OLD_PG_RESTORE':
            allocation=r.audit.decode(reference(entry['allocation']))
            packet=r.audit.decode(reference(entry['packet']))
            r.need(allocation['execution']==packet['Execution'],'restore origin execution differs')
            group=next(g for g in allocation['groups'] if g['group']==entry['group'])
            binding=next(g for g in packet['Groups'] if g['Group']==entry['group'])
            r.need(name in (group['volumes']['checkpoint-core'],group['volumes']['rehearsal-core'])
                   and any(f['name'].rsplit('/',1)[-1]=='PG_VERSION' and f['type']=='0' for f in content['files']),
                   'not a bound old PostgreSQL restore')
            keys={'core.dump':'CoreDumpSHA256','globals.sql':'GlobalsSHA256','native.tar':'NativeArchiveSHA256'}
            r.need(set(entry['backups'])==set(keys),'complete reconstructable checkpoint missing')
            for file,key in keys.items():
                spec=entry['backups'][file]
                r.need(Path(spec['path']).name==file and spec['sha256']==binding[key], 'backup not frozen to original packet')
                reference(spec)
        else:raise ValueError('resource retirement: unknown volume decision')
    return plan

def receipt_facts(plan,digest,directory):
    """Immutable exact outcomes only; current presence is checked separately."""
    removed=set()
    for folder in Path(directory).iterdir():
        r.need(folder.is_dir() and not folder.is_symlink() and not folder.is_junction(),'invalid volume receipt path')
        intent=r.read(folder/'intent.json');outcome=r.read(folder/'result.json')
        name=intent['volume']
        r.need(name in plan['remove'] and folder.name==r.sha(name.encode()) and name not in removed
               and intent['plan_sha256']==digest and intent['operation']=='docker volume rm'
               and outcome['volume']==name and outcome['plan_sha256']==digest
               and outcome['status']=='REMOVED_EXACT_TEST_VOLUME' and outcome['exit_code']==0
               and outcome['stdout'].split()==[name] and outcome['confirmed_absent'] is True,
               'unconfirmed or unbound volume removal')
        removed.add(name)
    return removed


def receipts(plan,digest,directory,docker):
    removed=receipt_facts(plan,digest,directory)
    names=docker.raw('volume','ls','-q').decode('utf8','strict').split()
    r.need(len(names)==len(set(names)),'duplicate current volume inventory')
    r.need(not removed&set(names),'retired volume still present')
    r.need((set(plan['remove'])-removed|set(plan['preserve']))<=set(names),'missing volume has no fixed receipt')
    return removed

def present(parent,removed,docker):
    projected={**parent,'volumes':{name:row for name,row in parent['volumes'].items() if name not in removed}}
    return r.present_contract(projected,docker,removed=set(parent['retired_historical']))

def verify(container_path,container_digest,root,volume_path,volume_digest,receipt_root,docker,*,complete):
    parent=r.plan_at(container_path,container_digest);parent['_sha256']=container_digest
    plan=checked(volume_path,volume_digest,parent,container_digest)
    originals=r.immutable(parent,root)
    r.need(r.receipts(parent,Path(root)/'removals')==set(parent['retired_historical']),'container retirement incomplete')
    removed=receipts(plan,volume_digest,receipt_root,docker)
    if complete:r.need(removed==set(plan['remove']),'test volume retirement incomplete')
    present(parent,removed,docker)
    return dict(status='PASS_RETIRED_CONTAINER_AND_TEST_VOLUME_HISTORY' if complete else 'PASS_PREPARED_TEST_VOLUME_RETIREMENT',
        plan_sha256=container_digest,volume_plan_sha256=volume_digest,removed_containers=len(parent['retired_historical']),removed_volumes=len(removed),
        original_files=originals,live_required=parent['live_required'],budget_volumes_preserved=len(parent['budgets']),provider_delete=0,budgets_refunded=0,pins_released=0)

def remove(container_path,container_digest,root,volume_path,volume_digest,receipt_root,docker):
    verify(container_path,container_digest,root,volume_path,volume_digest,receipt_root,docker,complete=False)
    parent=r.plan_at(container_path,container_digest)
    plan=checked(volume_path,volume_digest,parent,container_digest)
    removed=receipts(plan,volume_digest,receipt_root,docker)
    for at,name in enumerate(sorted(set(plan['remove'])-removed)):
        if at%8==0:present(parent,removed,docker)
        users=docker.raw('ps','-aq','--no-trunc','--filter','volume='+name).decode('utf8','strict').split()
        r.need(not users,'test volume acquired a current container dependency')
        volume=docker.json('volume','inspect',name)
        r.need(len(volume)==1 and {k:volume[0].get(k) for k in r.VOLUME}==plan['remove'][name]['metadata'],'test volume identity changed')
        folder=Path(receipt_root)/r.sha(name.encode());folder.mkdir(exist_ok=False)
        r.save(folder/'intent.json',dict(volume=name,plan_sha256=volume_digest,operation='docker volume rm',started_at=r.now()))
        result=subprocess.run(['docker','volume','rm',name],capture_output=True,timeout=60)
        r.write(folder/'stdout',result.stdout);r.write(folder/'stderr',result.stderr)
        r.need(result.returncode==0,'exact volume removal failed; do not retry unknown')
        output=result.stdout.decode('utf8','strict');r.need(output.split()==[name],'removed volume response differs')
        absent=name not in docker.raw('volume','ls','-q').decode('utf8','strict').split()
        r.save(folder/'result.json',dict(volume=name,plan_sha256=volume_digest,status='REMOVED_EXACT_TEST_VOLUME' if absent else 'UNCONFIRMED',exit_code=0,stdout=output,confirmed_absent=absent,observed_at=r.now()))
        r.need(absent,'removed volume remains present');removed.add(name)
        if at%8==0:print(r.encoded(dict(removed_volumes=len(removed),total=len(plan['remove']))).decode(),flush=True)
    return verify(container_path,container_digest,root,volume_path,volume_digest,receipt_root,docker,complete=True)
