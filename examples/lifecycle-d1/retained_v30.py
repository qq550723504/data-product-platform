"""Read-only preservation of terminal, unfrozen C20 and its isolated PG clone.

The failed execution has no native packet or reusable approval. Its original
archive is checked as issued, never repacked under the current UTC recipe.
"""
import hashlib
import json
from pathlib import Path
import evidence_indexes as audit

EXECUTION='5a54e01a-657e-4b8a-ae67-a2d6fd5be8f6'
REL='docs/acceptance/evidence/issue307-d1'
ANCHOR='candidate21-planning01/retained-candidate20-anchor01.json'
ANCHOR_SHA256='0a84e160225ad47940e3d4a170e94317d00adc85f53e6ce17e0f305eddeab2bd'
PUBLIC_INDEX='candidate21-planning01/retained-candidate20-public-index01.json'
PUBLIC_INDEX_SHA256='dd9c3b2caa77896f613e5af343a0a48e64b08d10614c04a58c97f2433a6add30'
CLONE_STATE='candidate21-planning01/retained-c20-clone-state01.json'
CLONE_STATE_SHA256='9186c45d000a20d6331faf7508698f1c2547a82d55a1495e909e81386827377f'
PINS={
 'candidate20-planning01/ci-blocked-closure01/terminal.json':'1aff9d0c516b4be3247e276e9b6a9ab2f9bb05af48e62f7f1dbe0cdcce28e9ae',
 'remaining-candidate20-preparation01/allocation.json':'e952f7c83eaf5af25dcf3878dd88a515ffe3766eb938170117327fa475a448c6',
 'remaining-candidate20-preparation01/owner-descriptors.json':'c5e88a932e5959086ccb316722a72194b0bb2fc96bd7618e7486f038202c2f64',
 'remaining-candidate20-preparation01/budget-initialization.json':'c3a0fc8e890838bd7f2da6c1da53e0ba706aaabbc6e4f6188dc3d1f36603de25',
 'remaining-candidate20-preparation01/profiles.json':'326699cfd50e211c1a17a51080277df1a4012dbb38f3df21c87e18366c540745',
}
STATE_FIELDS=('Running','Pid','Status','ExitCode','StartedAt','FinishedAt','OOMKilled','Dead','Paused','Restarting')
VOLUME_FIELDS=('Name','Driver','Scope','Options','Labels','CreatedAt')
NETWORK_FIELDS=('Id','Name','Driver','Scope','Internal','IPAM','Options','Labels','Containers')


def need(ok,message):
    if not ok:raise ValueError('retained v30: '+message)


def sha(path):return hashlib.sha256(audit.unlinked_absolute(path).read_bytes()).hexdigest()
def load(path):return audit.decode(audit.unlinked_absolute(path).read_bytes())


def file_set(folder):
    # The same existing boundary must normalize both sides of relative_to:
    # hosted Windows TEMP can spell this root with an 8.3 user directory.
    folder=audit.unlinked_absolute(folder)
    values={}
    for path in sorted(folder.rglob('*')):
        need(not path.is_symlink() and not path.is_junction(),'linked retained private file/directory')
        if not path.is_file():continue
        path=audit.unlinked_absolute(path)
        need(path.stat().st_nlink==1,'aliased retained private file')
        values[path.relative_to(folder).as_posix()]={'sha256':sha(path),'bytes':path.stat().st_size}
    return values


def raw(docker,*args):
    value=docker(*args);need(isinstance(value,bytes),'Docker wrapper must return bytes')
    return value


def json_docker(docker,*args):return json.loads(raw(docker,*args).decode('utf-8','strict'))


def mounts(docker,name,expected):
    current=raw(docker,'ps','-aq','--no-trunc','--filter','volume='+name).decode('utf-8','strict').split()
    need(len(current)==len(set(current)) and set(current)==set(expected),'retained full volume mount graph differs')


def verify(root,inspect,closed,project_owner,docker,sequence,ids,private,hostpath):
    evidence=root/REL;old=private/'remaining-candidate20'
    for name,digest in PINS.items():need(sha(evidence/name)==digest,'original public fact drift: '+name)
    need(sha(evidence/ANCHOR)==ANCHOR_SHA256,'retained anchor drift')
    anchor=load(evidence/ANCHOR)
    need(anchor['status']=='C20_RETAINED_UNFROZEN_PREPARATION_ANCHOR' and anchor['execution']==EXECUTION
         and anchor['authority_reusable'] is False and anchor['source_provider_actions']==anchor['native_delete']==0,'retained anchor scope differs')
    index=load(evidence/PUBLIC_INDEX)
    audit.audit_index(root,{'id':'original-c20-public','index':(evidence/PUBLIC_INDEX).relative_to(root).as_posix(),
        'sha256':PUBLIC_INDEX_SHA256,'format':audit.FILES_MAP,'member_root':'.','expected_count':index['count'],
        'expected_status':'C20_ORIGINAL_PUBLIC_EVIDENCE_INDEX'})
    terminal=load(evidence/'candidate20-planning01/ci-blocked-closure01/terminal.json')
    need(terminal['status']=='FAILED_CI_BLOCKED_UNFROZEN_PREPARATION_CLOSED' and terminal['execution']==EXECUTION
         and terminal['retry_allowed'] is False and terminal['failures']==[]
         and terminal['native_preparation']=='NOT_RUN' and terminal['IAM']=='NOT_PROVISIONED_NO_SOURCE_START'
         and terminal['source_started'] is False and terminal['source_business_commands']==terminal['physical_provider_calls']==terminal['native_delete']==0
         and terminal['authorization']=='NOT_RECEIVED' and terminal['schedule']=='NOT_CREATED','original failure was relabelled')
    need(sha(old/'preparation-terminal.json')==PINS['candidate20-planning01/ci-blocked-closure01/terminal.json'], 'private terminal differs')
    need(not (old/'authorized-execution01').exists() and not (evidence/'remaining-candidate20').exists()
         and not (evidence/'remaining-candidate20-preparation01/non-delete-start.json').exists(),'terminal C20 was resumed/frozen/authorized')
    need(file_set(old)==anchor['private_files'],'original C20 private bytes or membership drift')
    clone_folder=private/'candidate20-schema-compatibility01'
    need(file_set(clone_folder)==anchor['schema_private_files'],'retained isolated clone files drift')
    a=load(evidence/'remaining-candidate20-preparation01/allocation.json')
    need(sha(old/'allocation.json')==PINS['remaining-candidate20-preparation01/allocation.json'] and a['execution']==EXECUTION,'original allocation differs')
    need(terminal['allocation_sha256']==PINS['remaining-candidate20-preparation01/allocation.json'],'terminal allocation cross-binding differs')
    for name,digest in a['build_files'].items():need(sha(old/name)==digest,'original runtime build drift')
    need(len(ids)==820 and ids==set(terminal['old_retained']['closed_cids']),'prior retained owner set differs')
    need(len(anchor['owners'])==len(a['owners'])==86 and set(anchor['owners'])==set(a['owners'])
         and not ids.intersection(a['owners']),'retained C20 owner membership differs')
    descriptors=load(evidence/'remaining-candidate20-preparation01/owner-descriptors.json')
    need({cid:row['descriptor'] for cid,row in anchor['owners'].items()}==descriptors,'original owner descriptor cross-binding differs')
    observed=[]
    for n in range(0,len(a['owners']),48):observed+=json_docker(docker,'inspect',*a['owners'][n:n+48])
    need(len(observed)==86 and len({x['Id'] for x in observed})==86,'missing/duplicate owner observation')
    for item in observed:
        need(item['Id'] in anchor['owners'] and closed(item),'C20 retained owner active/exposed/restarting')
        saved=anchor['owners'][item['Id']]
        need(project_owner(item)==saved['descriptor'] and {k:item['State'][k] for k in STATE_FIELDS}==saved['state'],
             'C20 topology or terminal owner was restarted')
    permissions={p.relative_to(old).as_posix():sha(p) for p in old.glob('*/*/permission.json')}
    need(len(permissions)==30 and permissions==anchor['permissions'],'C20 permission membership/bytes differ')
    for name in permissions:need(load(old/name)=={'Enabled':False},'C20 permission no longer false')
    for row in terminal['revoked_profiles']:
        value=load(old/row['path']);base=value['Base'] if 'Base' in value else value
        need(base['Enabled'] is False and sha(old/row['path'])==row['after'],'C20 revoked profile changed')
        if row['before']!=row['after']:
            need(sha(old/'ci-blocked-closure01'/row['path'])==row['before'],'original enabled profile evidence changed')
    volumes={x['volume'] for x in a['resources'] if 'volume' in x}
    networks={x['network']:x for x in a['resources'] if 'network' in x}
    need(len(anchor['resources'])==len(a['resources'])==18 and len(volumes)==15 and len(networks)==3
         and {x['value']['Name'] for x in anchor['resources'] if x['kind']=='volume'}==volumes
         and {x['value']['Id'] for x in anchor['resources'] if x['kind']=='network'}==set(networks),'original resource membership differs')
    for resource in anchor['resources']:
        expected=resource['value']
        need(expected['Labels'].get('lifecycle307.remaining')==EXECUTION
             and expected['Labels'].get('lifecycle307.allocation')==a['allocation_nonce'],'original resource ownership differs')
        if resource['kind']=='volume':
            current=json_docker(docker,'volume','inspect',expected['Name'])
            need(len(current)==1 and {k:current[0].get(k) for k in VOLUME_FIELDS}==expected,'C20 retained volume ownership differs')
            mounts(docker,expected['Name'],resource['mounted_cids'])
        else:
            need(resource['kind']=='network','unknown retained resource')
            need(expected['Internal'] is True and not expected['Containers']
                 and expected['Name']==networks[expected['Id']]['name']
                 and isinstance(expected['IPAM']['Config'],list) and len(expected['IPAM']['Config'])==1
                 and expected['IPAM']['Config'][0].get('Subnet')==networks[expected['Id']]['subnet'],
                 'original isolated network scope differs')
            current=json_docker(docker,'network','inspect',expected['Id'])
            need(len(current)==1 and {k:current[0].get(k) for k in NETWORK_FIELDS}==expected,'C20 retained network differs')
    # Copies come from the existing immutable public fixture, never guessed keys.
    original=load(evidence/'remaining-candidate12/candidate.json')
    candidate={'Execution':EXECUTION,'Groups':[{'Group':g['group'],'Helpers':g['helpers'],
        'Copies':next(x['Copies'] for x in original['Groups'] if x['Group']==g['group'])} for g in a['groups']]}
    need(anchor['new_budgets']==terminal['new_budgets'] and len(anchor['new_budgets'])==3,'old budget membership differs')
    for row in anchor['new_budgets']:
        group=next(x for x in a['groups'] if x['group']==row['group'])
        need(row['empty'] is True and row['volume']==group['volumes']['budget'],'old independent budget changed')
        data=raw(docker,'cp',row['mounted_cids'][0]+':/run/budget','-')
        need(hashlib.sha256(data).hexdigest()==row['sha256'],'retained empty budget bytes differ')
        sequence.budget_archive(data,candidate,row['group'],initial=True)
        mounts(docker,row['volume'],row['mounted_cids'])
    clone=anchor['schema_clone'];current=json_docker(docker,'inspect',clone['cid'])
    need(sha(evidence/CLONE_STATE)==CLONE_STATE_SHA256,'isolated clone state anchor drift')
    saved_state=load(evidence/CLONE_STATE)
    need(len(current)==1 and closed(current[0]) and project_owner(current[0])==clone['descriptor']
         and current[0]['HostConfig']['NetworkMode']=='none' and saved_state['cid']==clone['cid']
         and {k:current[0]['State'][k] for k in STATE_FIELDS}==saved_state['state'],'isolated C20 clone topology/closure/state differs')
    volume=json_docker(docker,'volume','inspect',clone['volume'])
    need(len(volume)==1 and {k:volume[0].get(k) for k in VOLUME_FIELDS}==clone['volume_descriptor'],'isolated clone volume drift')
    mounts(docker,clone['volume'],clone['volume_mounted_cids'])
    need(clone['cid'] not in ids and clone['cid'] not in a['owners'],'clone/source/retained alias')
    ids.update(a['owners']);ids.add(clone['cid'])
    return {'status':'PASS_RETAINED_V30_UNFROZEN_CLOSED_907','closed_cids':sorted(ids),
        'terminal_sha256':PINS['candidate20-planning01/ci-blocked-closure01/terminal.json'],
        'private_files_preserved':len(anchor['private_files']),'public_files_preserved':index['count'],
        'new_budgets_preserved':3,'historical_budgets_preserved':24,'backup_files_preserved':57,
        'IAM':'NOT_PROVISIONED_NO_SOURCE_START','authorization_reusable':False,
        'original_archive_repacked':False,'pins_released':0,'budgets_refunded':0,'native_delete':0}
