"""Read-only preservation of expired, never-authorized frozen candidate21.

Reuse the existing typed index, path, owner, resource and budget contracts.
No old entry is executed: its existing preparation terminal denies resumption.
"""
import hashlib
from pathlib import Path
import evidence_indexes as audit
import retained_v30 as prior

EXECUTION='9ba214bd-a481-499b-bef6-ca7a789ef501'
OLD_ROOT=Path('C:/Users/Henry/.codex/worktrees/d1-c21/data-product-platform')
REL='docs/acceptance/evidence/issue307-d1'
ANCHOR='candidate22-planning01/retained-candidate21-anchor02.json'
ANCHOR_SHA256='aedf37a6544077b06aad4053c8cdece559197c0431c75ad00678024d2b47d982'
INDEX='candidate22-planning01/retained-candidate21-public-index02.json'
INDEX_SHA256='eecca3c7b4149072e34cf06e7888d7edf918b6ae30b0d776d38d641300db8e88'
TERMINAL_SHA256='5568708e62c4ad37f8765aba667697de547d83fe0e598d59d7d4341a84b4b188'
need,sha,load=prior.need,prior.sha,prior.load


def verify(root,inspect,closed,project_owner,docker,sequence,ids,private,hostpath):
    evidence=root/REL;old=private/'remaining-candidate21'
    need(sha(evidence/ANCHOR)==ANCHOR_SHA256,'C21 anchor drift')
    anchor=load(evidence/ANCHOR)
    need(anchor['status']=='C21_RETAINED_EXPIRED_FROZEN_ANCHOR'
         and anchor['execution']==EXECUTION and anchor['profile']=='candidate21/v31'
         and anchor['authority_reusable'] is False
         and anchor['source_start']==anchor['native_delete']==0,'C21 anchor scope differs')
    index=load(evidence/INDEX)
    audit.audit_index(root,dict(id='retained-c21-public',index=(evidence/INDEX).relative_to(root).as_posix(),
        sha256=INDEX_SHA256,format=audit.FILES_MAP,member_root='.',expected_count=index['count'],
        expected_status='C21_ORIGINAL_PUBLIC_EVIDENCE_INDEX'))
    # Preserve actual issued bytes, including off-HEAD post-lock receipts.
    # They are historical evidence, not a live branch/protection assertion.
    for name,row in index['files'].items():
        path=audit.bound_path(OLD_ROOT,name)
        need(sha(path)==row['sha256'] and path.stat().st_size==row['bytes'],'original C21 public fact drift')
    terminal=evidence/'candidate21-expiry01/terminal.json'
    need(sha(terminal)==sha(old/'preparation-terminal.json')==TERMINAL_SHA256,'C21 terminal bytes differ')
    fact=load(terminal)
    need(fact['status']=='EXPIRED_UNAUTHORIZED_FROZEN_PREPARATION_CLOSED'
         and fact['execution']==EXECUTION and fact['retry_allowed'] is False
         and fact['authorization']=='NOT_RECEIVED' and fact['schedule']=='NOT_CREATED'
         and fact['native_delete_this_observation']==0
         and fact['fresh_native_IAM']=='NOT_OBSERVED_SOURCE_NOT_STARTED','C21 terminal relabelled')
    need(not (old/'authorized-execution01').exists()
         and not (OLD_ROOT/REL/'delete-confirmed-v31-execution01').exists()
         and not Path('C:/Users/Henry/.codex/automations/307-d1-candidate21-v31').exists(),
         'expired C21 was authorized, scheduled or resumed')
    need(prior.file_set(old)==anchor['private_files'],'C21 private file bytes/membership drift')
    a=load(evidence/'remaining-candidate21-preparation01/allocation.json')
    need(a==load(old/'allocation.json') and a['execution']==EXECUTION,'original C21 allocation drift')
    need(len(ids)==907 and ids==set(a['retained']['closed_cids']),'prior retained owner union differs')
    allocated=anchor['owners'];technical=anchor['technical_owners']
    need(len(allocated)==86 and set(allocated)==set(a['owners'])
         and len(technical)==3 and not set(allocated).intersection(technical)
         and not ids.intersection(set(allocated)|set(technical)), 'C21 owner memberships alias/expand')
    descriptors=load(evidence/'remaining-candidate21-preparation01/owner-descriptors.json')
    need({cid:row['descriptor'] for cid,row in allocated.items()}==descriptors,'C21 original descriptors drift')
    current_ids=set(allocated)|set(technical)
    for n in range(0,len(current_ids),48):
        expected=sorted(current_ids)[n:n+48]
        values=prior.json_docker(docker,'inspect',*expected)
        need(len(values)==len({x['Id'] for x in values})==len(expected)
             and {x['Id'] for x in values}==set(expected),'C21 observation membership differs')
        for item in values:
            saved=(allocated|technical)[item['Id']]
            need(closed(item) and project_owner(item)==saved['descriptor']
                 and {k:item['State'][k] for k in prior.STATE_FIELDS}==saved['state'],
                 'C21 retained owner restarted or changed topology')
    permissions=list(old.glob('*/*/permission.json'))
    need(len(permissions)==30 and all(load(p).get('Enabled') is False for p in permissions)
         and not list(old.rglob('human-confirmation.txt')),'C21 permission or authority changed')
    resources=a['resources']
    need(len(anchor['resources'])==len(resources)==18,'C21 resource membership differs')
    for row in anchor['resources']:
        expected=row['value'];labels=expected['Labels']
        need(labels.get('lifecycle307.remaining')==EXECUTION
             and labels.get('lifecycle307.allocation')==a['allocation_nonce'],'C21 resource ownership drift')
        if row['kind']=='volume':
            value=prior.json_docker(docker,'volume','inspect',expected['Name'])
            need(len(value)==1 and {k:value[0].get(k) for k in prior.VOLUME_FIELDS}==expected,'C21 volume scope/owner drift')
            prior.mounts(docker,expected['Name'],row['mounted_cids'])
        else:
            need(row['kind']=='network' and expected['Internal'] is True
                 and not expected['Containers'],'C21 restore network scope drift')
            value=prior.json_docker(docker,'network','inspect',expected['Id'])
            need(len(value)==1 and {k:value[0].get(k) for k in prior.NETWORK_FIELDS}==expected,'C21 restore network drift')
    candidate=load(evidence/'remaining-candidate21/candidate.json')
    need(len(anchor['budgets'])==3,'C21 independent budget membership differs')
    for row in anchor['budgets']:
        group=next(g for g in a['groups'] if g['group']==row['group'])
        need(row['empty'] is True and row['volume']==group['volumes']['budget']
             and row['mounted_cids']==group['budget_expected_mounted_cids'],'C21 budget binding drift')
        raw=prior.raw(docker,'cp',group['helpers']['wire']+':/run/budget','-')
        need(hashlib.sha256(raw).hexdigest()==row['sha256'],'C21 retained empty budget bytes drift')
        sequence.budget_archive(raw,candidate,row['group'],initial=True)
        prior.mounts(docker,row['volume'],row['mounted_cids'])
    ids.update(current_ids)
    need(len(ids)==996 and ids==set(anchor['known_states']),'C21 known owner union differs')
    return dict(status='PASS_RETAINED_V31_EXPIRED_FROZEN_CLOSED_996',closed_cids=sorted(ids),
                original_public_files=index['count'],private_files_preserved=len(anchor['private_files']),
                terminal_sha256=TERMINAL_SHA256,historical_budgets_preserved=30,backup_files_preserved=66,
                authorization_reusable=False,pins_released=0,budgets_refunded=0,native_delete=0)
