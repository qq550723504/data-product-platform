"""Read-only retention of the real v29 HEAD-drift failure; never resume it."""
import hashlib
import json
from pathlib import Path
import evidence_indexes as audit

EXECUTION='a5d1e569-f80e-4b38-8f32-d8139a30a077'
OLD_ROOT=Path('C:/Users/Henry/.codex/worktrees/d1-c19/data-product-platform')
REL='docs/acceptance/evidence/issue307-d1'
PINS={
 'delete-confirmed-v29-execution01/window01/terminal.json':'7b9a17ff853b059c88fa0a05450bc1a1676bf9a51222cffaf529220e2f9a36f8',
 'delete-confirmed-v29-execution01/window01/post-seal-readback.json':'3ef20b945c510b9f8bc4698740b2e334d2d6c6405cce92e32f2f90d52d934784',
 'remaining-candidate19-preparation01/allocation.json':'232409f09247252a8526ee23f59f4a8ea00bb6b592f8985e2ba3ffdb0fe0bec8',
 'remaining-candidate19-preparation01/owner-descriptors.json':'eefd51e3ee7ea643f397820664c15d0ac0ec783fb828a167971166dc31a4d607',
 'execution-freeze-review01/retained-v29-receipt-index.json':'5c5c9ef750d90d1a70597797b71fea6b02c3f3a2a17695556bf852e173ec708d',
}

def need(ok,message):
    if not ok:raise ValueError('retained v29: '+message)

def sha(path):return hashlib.sha256(audit.unlinked_absolute(path).read_bytes()).hexdigest()
def load(path):return audit.decode(audit.unlinked_absolute(path).read_bytes())

def closed_members(rows,expected,closed):
    need(len(rows)==len(expected)==len({x['Id'] for x in rows})
         and {x['Id'] for x in rows}==set(expected),'owner observation membership differs')
    need(all(closed(x) for x in rows),'retained owner active/exposed/restarting')

def budget_owner(volume,name,nonce):
    need(volume['Name']==name and volume['Driver']=='local' and volume['Scope']=='local'
         and not volume.get('Options') and volume['Labels'].get('lifecycle307.remaining')==EXECUTION
         and volume['Labels'].get('lifecycle307.allocation')==nonce,'v29 budget ownership differs')

def budget_mounts(expected,current):
    need(len(current)==len(set(current)) and set(current)==set(expected),'old budget mount graph differs')

def verify(root,inspect,closed,project_owner,docker,ids,private,hostpath):
    evidence=root/REL
    for name,digest in PINS.items():need(sha(evidence/name)==digest,'pinned old fact changed: '+name)
    terminal=load(evidence/'delete-confirmed-v29-execution01/window01/terminal.json')
    facts=load(evidence/'delete-confirmed-v29-execution01/window01/post-seal-readback.json')
    need(terminal['execution']==EXECUTION and terminal['status']=='FAILED_PR_HEAD_DRIFT_PHYSICALLY_SEALED_NO_DELETE'
         and terminal['terminal'] is True and terminal['retry_allowed'] is False and terminal['old_authorization_reusable'] is False
         and terminal['new_native_delete']==terminal['new_Core_commands']==terminal['physical_provider_DELETE_attempts']==0,
         'v29 original terminal/unused execution changed')
    need(terminal['fresh_IAM_disable']==facts['fresh_IAM_disable']=='UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START'
         and facts['all_steps_confirmed'] is False,'historical unconfirmed IAM was relabelled')
    old=private/'remaining-candidate19';grant=old/'authorized-execution01'
    need(sha(grant/'terminal.json')==PINS['delete-confirmed-v29-execution01/window01/terminal.json'],'private terminal differs')
    copied=load(evidence/'execution-freeze-review01/retained-v29-receipt-index.json')
    need(copied['count']==len(copied['members'])==124,'original receipt membership differs')
    for row in copied['members']:
        a,b=audit.bound_path(root,row['path']),audit.bound_path(OLD_ROOT,row['path'])
        need(a.read_bytes()==b.read_bytes() and sha(a)==row['sha256'] and a.stat().st_size==row['bytes'],'original v29 receipt byte drift')
    auth=load(OLD_ROOT/REL/'delete-confirmed-v29-execution01/human-authorization.json')
    need(auth['execution']==EXECUTION and auth['user_retyped_template'] is False
         and len(auth['approved_reference_artifacts'])==924,'actual old authorization changed')
    for row in auth['approved_reference_artifacts']:
        path=audit.bound_path(OLD_ROOT,row['path'])
        need(sha(path)==row['sha256'] and path.stat().st_size==row['bytes'],'original approved byte drift')
    for name,key in [('user-message-verbatim.txt','user_message_sha256'),('authorization-envelope.txt','authorization_envelope_sha256')]:
        need(sha(grant/name)==auth[key],'old human text/envelope drift')
    allocation=load(evidence/'remaining-candidate19-preparation01/allocation.json')
    need(sha(old/'allocation.json')==PINS['remaining-candidate19-preparation01/allocation.json'],'private allocation changed')
    for name,digest in allocation['build_files'].items():need(sha(old/name)==digest,'v29 original build bytes changed')
    permissions=facts['permissions_false']
    need(len(permissions)==30 and len(facts['cases_revoked'])==len(facts['recoveries_revoked'])==6,'sealed control membership differs')
    actual=set()
    for group in allocation['groups']:
        folder=old/group['group']
        actual.update(path.resolve() for path in folder.glob('*/permission.json'))
        for name in ('candidate.json','bindings.json','confirmation-template.txt'):
            # Public immutable packet is retained independently of revoked profiles.
            need(sha(evidence/'remaining-candidate19'/name)==sha(OLD_ROOT/REL/'remaining-candidate19'/name),'old packet changed')
    expected={Path(row['path']).resolve() for row in permissions}
    need(actual==expected,'old permission file set expanded/missing')
    for row in permissions:
        path=audit.unlinked_absolute(Path(row['path']))
        need(path.is_relative_to(old.resolve()) and sha(path)==row['sha256'] and load(path)['Enabled'] is False
             and not (path.parent/'human-confirmation.txt').exists(),'old worker permission/approval changed')
    for key in ('cases_revoked','recoveries_revoked'):
        for row in facts[key]:
            folder=old/row['group']
            path=folder/('case-'+row['stage'])/'remaining.json' if key=='cases_revoked' else folder/row['stage']/'recovery.json'
            value=load(path);enabled=value['Base']['Enabled'] if key=='cases_revoked' else value['Enabled']
            need(enabled is False and sha(path)==row['sha256'],'old revoked case/recovery changed')
    for row in facts['gateways']:need(sha(old/row['group']/'gateway.Caddyfile')==row['sha256'] and row['sealed'] is True,'old gateway changed')
    known=[row['cid'] for row in facts['owners']]
    need(len(known)==len(set(known))==820 and ids.issubset(known),'retained v29 fixed owner set differs')
    observed=[]
    for n in range(0,len(known),64):observed.extend(json.loads(docker('inspect',*known[n:n+64])))
    closed_members(observed,known,closed)
    by_id={x['Id']:x for x in observed}
    descriptors=load(evidence/'remaining-candidate19-preparation01/owner-descriptors.json')
    for cid,descriptor in descriptors.items():need(project_owner(by_id[cid])==descriptor,'retained v29 owner topology changed')
    for group in allocation['groups']:
        name=group['volumes']['budget'];volume=json.loads(docker('volume','inspect',name))[0]
        budget_owner(volume,name,allocation['allocation_nonce'])
    need(len(facts['budget_readbacks'])==24 and len(facts['backup_readbacks'])==57,'old budget/backup membership differs')
    for row in facts['budget_readbacks']:
        current=docker('cp',row['mounted_cids'][0]+':/run/budget','-')
        need(hashlib.sha256(current).hexdigest()==row['sha256'],'old consumed/empty budget bytes changed')
        mounted=docker('ps','-aq','--no-trunc','--filter','volume='+row['volume']).decode('utf8','strict').split()
        budget_mounts(row['mounted_cids'],mounted)
    for row in facts['backup_readbacks']:
        path=audit.bound_path(private,row['private_relative_path'])
        need(sha(path)==row['sha256'] and path.stat().st_size==row['bytes'],'retained backup changed')
    ids.update(known)
    return dict(status='PASS_RETAINED_V29_PHYSICAL_AND_BYTES_820',closed_cids=sorted(ids),
                terminal_sha256=PINS['delete-confirmed-v29-execution01/window01/terminal.json'],
                historical_IAM=facts['fresh_IAM_disable'],original_receipts=124,
                budgets_preserved=24,backups_preserved=57,authorization_reusable=False,
                pins_released=0,budgets_refunded=0,native_delete=0)
