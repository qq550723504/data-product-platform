"""Read-only contract for the actual failed v27 and later trusted closures.

The fixed accepted old set excludes the new generation by construction. No
status projection is rewritten, identity enabled, budget reset or source run.
"""
import hashlib
import json
from pathlib import Path
import subprocess

import evidence_indexes as audit

OLD_ROOT=Path('C:/Users/Henry/.codex/worktrees/d1-c17/data-product-platform')
READ_ROOT=Path('C:/Users/Henry/.codex/worktrees/d1-path/data-product-platform')
EXECUTION='6f40bca9-cfe2-4362-ad2b-4e38bbbd8453'
COMMIT='723e04d1897dc9e8586573601b7c1d81ee25813b'
REL='docs/acceptance/evidence/issue307-d1'
CORE_BASELINES={
    'business-races':('post-v27-business-closure03/final-Core-fingerprint.json','ffdf75013a393da6a8444b28284fdfcc47b5a01a346f7fdf25a8c03138cd8a7f'),
    'late-hold':('remaining-candidate17/late-hold/core-readonly.json','7aafb25df2004970410fc80ebbe1206c49ebf2d3315d4294a95c7c903b10c01b'),
    'mixed':('remaining-candidate17/mixed/core-readonly.json','2c670718760a3a7306a9fdba4946e99d9fb024f7258235f4c7f6d880064afd55'),
}
PINS={
    'remaining-candidate17/candidate.json':'20d08f0a2d334e779b64f0c082515131c285d9fd005c6fbaa94294b147f38d42',
    'remaining-candidate17/bindings.json':'77ce0be4d5dccd95a276a5b4c3f060689bea870f4f3caef1a827ef81154efa24',
    'post-v27-seventeen01/terminal.json':'74dd235972c266474038c57714511e5b02574a145b68b21d43aef5d528cc76b2',
    'post-v27-seventeen01/plan.json':'26d27adacec543172a7c570ca3a8d174565348e9c424e5a17dea06b286cc1667',
    'post-v27-seventeen01/final-closed-and-history-readback.json':'660c0a5b674607ca3782bbfacf434020e9d2667dbcb09999f42a10011d1788f0',
    'post-v27-seventeen01/evidence-index01.json':'0be33d194eabd991541c74a3e027bff573ed00b6e1a940280f008eb06fcc119b',
    'post-v27-business-closure03/terminal.json':'cdae9ec2ca7ee149bc73bad73456443ea82e988c647d9a775dd8d0403c396a7f',
}


def need(value,message):
    if not value:raise ValueError('retained v27: '+message)


def sha(path):return hashlib.sha256(audit.unlinked_absolute(path).read_bytes()).hexdigest()
def load(path):return audit.decode(audit.unlinked_absolute(path).read_bytes())
def cfg_hash(item):return hashlib.sha256(json.dumps({k:item[k] for k in ('Image','Config','HostConfig','Mounts')},sort_keys=True).encode()).hexdigest()


def load_bindings(path):
    value=json.loads(audit.unlinked_absolute(path).read_bytes(),object_pairs_hook=audit.pairs)
    need(type(value) is list and len(value)==3 and [x.get('group') for x in value]==['business-races','late-hold','mixed'],'old bindings must be the exact ordered list')
    return value


def fixed_old_files(private,expected):
    """Only the pinned old folders are scanned, still requiring exact membership."""
    folders={audit.relative_parts(name)[0] for name in expected}
    observed={}
    for name in sorted(folders):
        folder=audit.bound_path(private,name,directory=True)
        paths=list(folder.rglob('permission.json'))+list(folder.glob('*/case-*/remaining.json'))+list(folder.glob('*/recover-*/recovery.json'))+list(folder.glob('*/checkpoint/*'))
        for path in paths:
            if path.is_file():observed[path.relative_to(private).as_posix()]=sha(path)
    need(observed==expected,'old control/backup membership or bytes changed')
    return observed


def budget_users(expected,actual):
    need(len(actual)==len(set(actual)) and set(actual)==set(expected),'old budget complete mount graph changed')


def budget_owner(volume,name,nonce):
    need(volume['Name']==name and volume['Driver']=='local' and volume['Scope']=='local' and not volume.get('Options')
         and volume['Labels'].get('lifecycle307.remaining')==EXECUTION and volume['Labels'].get('lifecycle307.allocation')==nonce,'old budget volume ownership changed')


def old_metadata(root,index):
    """Old host source is proven from its Git commit; history stays current bytes."""
    value=load(index)
    need(value['status']=='RECORDED_CURRENT_SEVENTEEN_READONLY_EVIDENCE' and value['count']==348
         and type(value['files']) is dict and len(value['files'])==348,'accepted read index shape differs')
    history=0
    for name,meta in value['files'].items():
        audit.relative_parts(name);audit.metadata(meta)
        raw=subprocess.check_output(['git','-c','core.longpaths=true','show',COMMIT+':'+name],cwd=root)
        need(hashlib.sha256(raw).hexdigest()==meta['sha256'] and len(raw)==meta['bytes'],'old committed read evidence differs')
        if name.startswith(REL+'/'):
            path=audit.bound_path(root,name)
            need(sha(path)==meta['sha256'] and path.stat().st_size==meta['bytes'],'old native history payload changed')
            history+=1
    return dict(members=348,history=history,old_source=COMMIT)


def verify(root,inspect,closed,docker,ids,private,hostpath):
    evidence=root/REL
    for name,digest in {**PINS,**dict(CORE_BASELINES.values())}.items():need(sha(evidence/name)==digest,'pinned old bytes changed: '+name)
    original=OLD_ROOT/REL/'delete-confirmed-v27-execution01'
    auth_path=original/'human-authorization.json'
    need(sha(auth_path)=='10f067ea519d4603fb72c8446d4af352ea22be743a43903325aa2c44adc4d89a','original authorization changed')
    auth=load(auth_path)
    need(auth['execution']==EXECUTION and auth['user_retyped_template'] is False and len(auth['approved_reference_artifacts'])==826,'old authorization facts differ')
    for item in auth['approved_reference_artifacts']:
        path=audit.bound_path(OLD_ROOT,item['path']);need(sha(path)==item['sha256'] and path.stat().st_size==item['bytes'],'old approved reference bytes changed')
    authdir=private/'remaining-candidate17/authorized-execution01'
    for name,key in [('user-message-verbatim.txt','user_message_sha256'),('authorization-envelope.txt','authorization_envelope_sha256')]:need(sha(authdir/name)==auth[key],'old human text or envelope changed')
    terminal=original/'window01/terminal.json'
    need(sha(terminal)==sha(authdir/'terminal.json')=='a98a6016e64c71e4f883ac4dbeff975f794f0135d6aa9a80cc946a4e51a45672','original failed terminal changed')
    failed=load(terminal)
    need(failed['status']=='FAILED_TOPOLOGY_ATTESTATION_SEALED_NO_DELETE' and failed['real_DELETE']==0,'old failed execution relabelled')
    index=authdir/'window01-evidence-index.json'
    need(sha(index)=='4d063048ba6e1a28b82457fa7153ec1fd0e1bd52dd021b596e5e89fdd3e92c46','old failed evidence index changed')
    entries=load(index);need(type(entries['files']) is dict and entries['count']==len(entries['files'])==771,'old failed evidence membership differs')
    for name,meta in entries['files'].items():
        audit.metadata(meta);path=audit.bound_path(OLD_ROOT,name)
        need(sha(path)==meta['sha256'] and path.stat().st_size==meta['bytes'],'old failed observation changed')
    read=load(evidence/'post-v27-seventeen01/terminal.json')
    need(read['status']=='PASS_CURRENT_SEVENTEEN_NATIVE_READONLY_SEALED' and read['present_versions']==17
         and read['technical_requests']==125 and read['native_delete']==read['business_mutations']==0,'accepted full read audit differs')
    metadata=old_metadata(root,evidence/'post-v27-seventeen01/evidence-index01.json')
    preservation=load(evidence/'post-v27-seventeen01/plan.json')['preservation']
    old=preservation['retained'];need(len(old['files'])==531 and len(old['budgets'])==18 and len(old['prior_maintenance_files'])==91,'old fixed history set differs')
    fixed_old_files(private,old['files'])
    for folder,key in [('post-v27-business-closure02','prior_maintenance_files'),('post-v27-business-closure03',None)]:
        bound=old[key] if key else preservation['maintenance03']
        actual={x.relative_to(private/folder).as_posix():sha(x) for x in (private/folder).rglob('*') if x.is_file() and x.name!='host-key.private'}
        need(actual==bound,'old maintenance/failure files changed')
    facts=load(evidence/'post-v27-seventeen01/final-closed-and-history-readback.json')
    cids=[x['cid'] for x in facts['owners']];need(len(cids)==len(set(cids))==623 and ids.issubset(cids),'accepted retained CID union differs')
    all_items=[]
    for n in range(0,len(cids),64):all_items.extend(json.loads(docker('inspect',*cids[n:n+64])))
    need(len(all_items)==623 and {x['Id'] for x in all_items}==set(cids),'retained CID lookup substituted')
    actual={x['Id']:x for x in all_items}
    need(all(closed(x) for x in all_items),'retained owner is active')
    for cid,digest in preservation['descriptors'].items():need(cfg_hash(actual[cid])==digest,'accepted owner descriptor changed')
    candidate=load(evidence/'remaining-candidate17/candidate.json')
    bindings=load_bindings(evidence/'remaining-candidate17/bindings.json')
    allocation=evidence/'remaining-candidate17-preparation01/allocation.json'
    need(sha(allocation)==bindings[0]['allocation_sha256']==sha(private/'remaining-candidate17/allocation.json'),'old initial allocation changed')
    original_allocation=load(allocation)
    for name,digest in original_allocation['build_files'].items():need(sha(private/'remaining-candidate17'/name)==digest,'old original build bytes changed')
    for group in original_allocation['groups']:
        name=group['volumes']['budget'];volume=json.loads(docker('volume','inspect',name))[0]
        budget_owner(volume,name,original_allocation['allocation_nonce'])
    sealed=load(original/'window01/postfailure-closed-readback.json')
    need([len(sealed[k]) for k in ('permissions','cases_revoked','recoveries_revoked')]==[30,6,6],'old revocation membership changed')
    for key in ('permissions','cases_revoked','recoveries_revoked'):
        for row in sealed[key]:
            folder=private/'remaining-candidate17'/row['group']
            path=(folder/row['stage']/'permission.json' if key=='permissions' else folder/('case-'+row['stage'])/'remaining.json' if key=='cases_revoked' else folder/row['stage']/'recovery.json')
            need(row['enabled'] is False and sha(path)==row['sha256'],'old revoked profile bytes changed')
            cfg=load(path);need((cfg['Base']['Enabled'] if key=='cases_revoked' else cfg['Enabled']) is False,'old profile re-enabled')
    groups={g['Group']:g for g in candidate['Groups']}
    for row in facts['profiles']:
        group=row['group'];folder=private/'post-v27-seventeen01'/group;profile=folder/'six-readonly.json'
        need(sha(profile)==row['sha256'] and load(profile)['Enabled'] is False and load(profile)['MaxProviderDeletes']==0,'old read profile changed or active')
        allocation=load(evidence/'post-v27-seventeen01'/group/'allocated-owner.json');x=actual[allocation['cid']];h=x['HostConfig'];mounts={m['Destination']:m for m in x['Mounts']}
        need(x['Config']['User']=='10001:10001' and x['Config']['Entrypoint']==['/run/reader'] and x['Config']['Cmd']==['-mode','six-verify-native']
             and h['NetworkMode']=='container:'+groups[group]['Native'] and h['ReadonlyRootfs'] and not h['Privileged']
             and h['CapDrop']==['ALL'] and not h.get('CapAdd') and 'no-new-privileges' in h['SecurityOpt']
             and len(x['Mounts'])==len(mounts)==3,'old readonly owner role expanded')
        for dest,path,rw in [('/run/private',folder,False),('/run/reader',private/'remaining-candidate17/reader',False),('/run/output',READ_ROOT/REL/'post-v27-seventeen01'/group/'files',True)]:
            m=mounts[dest];need(m['Type']=='bind' and hostpath(m['Source'])==path.resolve() and m['RW'] is rw,'old readonly owner mount changed')
    budgets={}
    for volume,digest in old['budgets'].items():
        users=preservation['budget_mounts'][volume]
        now=docker('ps','-aq','--no-trunc','--filter','volume='+volume).decode('utf-8','strict').split();budget_users(users,now)
        raw=docker('cp',users[0]+':/run/budget','-');need(hashlib.sha256(raw).hexdigest()==digest,'old budget bytes changed')
        budgets[volume]=dict(sha256=digest,mounted_cids=users)
    ids.update(cids)
    return dict(status='PASS_RETAINED_V27_AND_POST_CLOSURE_623_CLOSED',execution=EXECUTION,authorization_reusable=False,
                original_failure='FAILED_TOPOLOGY_ATTESTATION_SEALED_NO_DELETE',old_failed_index_members=771,approved_old_reference_members=826,
                accepted_read_index=metadata,old_profiles_backups=531,old_budget_volumes=budgets,old_maintenance_files_preserved=True,
                post_closure_Core_sha256=CORE_BASELINES['business-races'][1],closed_cids=sorted(cids),native_delete=0)
