import collections
import datetime as dt
import hashlib
import json
from pathlib import Path

ROOT=Path(r'C:\Users\Henry\.codex\worktrees\9400\data-product-platform')
OUT=Path(__file__).parent
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
PACKET=BASE/'remaining-candidate12'
REVIEW=BASE/'remaining-window-review06'
def need(ok,msg):
    if not ok:raise ValueError(msg)
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def load(p):return json.loads(p.read_bytes())

def main():
    inputs=load(REVIEW/'review-inputs.json')
    need(len(inputs['files'])==25 and all(sha(ROOT/n)==s for n,s in inputs['files'].items()),'final25 input SHA')
    physical=load(REVIEW/'physical-preparation-readback.json')
    need(physical['container_count']==len(physical['containers'])==300
         and len({r['cid'] for r in physical['containers']})==300
         and all(r['stopped_PID0_noports_norestart'] for r in physical['containers']),'retained CID closure')
    ids={r['cid'] for r in physical['containers']}
    c=load(PACKET/'candidate.json');bindings=load(PACKET/'bindings.json')
    needed={'25a4e218f72527e8f93a1df2eeb7a09833e5028c03d5ee178009bd9c71ee33e6'}
    for g,b in zip(c['Groups'],bindings):
        needed.update(g[k] for k in ('Core','Native','Gateway','CheckpointCore','CheckpointNative'))
        needed.update(g['Helpers'].values());needed.update(g['CaseHelpers'].values())
        needed.update(r['container'] for r in b['root_owners'].values());needed.add(b['recovery_redis'])
    need(needed<=ids,'new candidate CID absent in physical readback')
    rows=physical['permissions']
    need(physical['permissions_false']==len(rows)==192 and all(r['enabled'] is False for r in rows),'192 false permissions')
    for generation in range(7,13):
        expected={(g['Group'],s) for g in c['Groups'] for s in g['Helpers']}
        expected.update((b['group'],'root-'+s) for b in bindings for s in b['root_owners'])
        actual={(r['group'],r['stage']) for r in rows if r['generation']==generation}
        need(actual==expected and sum(r['generation']==generation for r in rows)==32,'generation permissions finite membership')
    need(physical['new_authorization']=='NOT_RECEIVED' and physical['new_schedule']=='NOT_CREATED'
         and physical['native_delete']==physical['source_starts_in_this_audit']==0 and physical['no_old_fact_rewritten'],'audit authority scope')
    need(not Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate12\authorized-execution01').exists(),'new authorization control exists')

    ledger=load(REVIEW/'technical-read-ledger.json');total=load(REVIEW/'technical-read-ledger-total.json')
    need(ledger['physical_attempts']==ledger['unique_ids']==len(ledger['attempts'])==152,'SDK attempt count')
    expected_ids={load(p)['id'] for p in PACKET.rglob('readonly-start-*.json')}
    need({r['attempt_id'] for r in ledger['attempts']}==expected_ids,'SDK ledger missing/extra immutable attempts')
    need(len({r['attempt_id'] for r in ledger['attempts']})==152,'SDK duplicated attempt')
    statuses=collections.Counter();methods=collections.Counter();files=0
    for row in ledger['attempts']:
        statuses[str(row['http_status'])]+=1;methods[row['method']]+=1
        need(not row['transport_error'] and row['method']=='GET','SDK mutation/unknown transport')
        for member in row['raw_members']:
            p=(ROOT/member['path']).resolve()
            need(p.is_relative_to(PACKET) and sha(p)==member['sha256'] and load(p)['id']==row['attempt_id'],'SDK raw member SHA/attempt')
            files+=1
    need(dict(statuses)==ledger['http_status_counts']=={'200':90,'400':62} and dict(methods)=={'GET':152},'SDK totals')
    need(ledger['business_delete_attempts']==ledger['mutation_requests']==0 and ledger['amount']=='NOT_ASSESSED','SDK ledger scope')
    need(total['readonly_ledger_sha256']==sha(REVIEW/'technical-read-ledger.json')
         and total['physical_attempts']==168 and total['readonly_attempts']==152
         and total['root_check_disabled_attempts']==len(total['root_attempts'])==16,'root + SDK count')
    root_ids=set()
    for row in total['root_attempts']:
        need(row['attempt_id'] not in root_ids and row['attempt_id'] not in expected_ids,'root duplicate attempt')
        root_ids.add(row['attempt_id']);statuses[str(row['http_status'])]+=1
        need(row['method']=='GET' and row['http_status']==200,'root mutation/status')
        folder=Path(row['private_directory'])
        expected_folder=Path(r'C:\Users\Henry\.codex\fixture-private\remaining-candidate12')/row['group']/'outputs/root-check-disabled'
        need(folder==expected_folder,'root journal path scope')
        # Only immutable diagnostic start/observation bytes are hashed. No root
        # or operator profile, usable secret, private key or credential is read.
        need(sha(folder/('start-'+row['attempt_id']+'.json'))==row['raw_start_sha256']
             and sha(folder/('observation-'+row['attempt_id']+'.json'))==row['raw_observation_sha256'],'root raw diagnostic SHA')
    need(dict(statuses)==total['HTTP_status_totals']=={'200':106,'400':62}
         and total['PUT']==total['DELETE']==0 and total['amount']=='NOT_ASSESSED','combined ledger outcomes')
    failures=load(REVIEW/'supplemental-audit-failures.json')
    need(len(failures['errors'])==2 and all(not r['source_or_gate_changed'] for r in failures['errors'])
         and failures['source_starts']==failures['permission_changes']==failures['provider_requests']==failures['native_delete']==0,'optional audit failures retained')
    result={'status':'PASS_INDEPENDENT_SUPPLEMENTAL_RECEIPT_AUDIT','at':dt.datetime.now(dt.timezone.utc).isoformat(),
        'review_inputs':25,'retained_CIDs':300,'disabled_permission_rows':192,'new_candidate_CIDs_included':len(needed),
        'unique_SDK_attempts':152,'SDK_raw_journal_member_checks':files,'root_diagnostic_attempt_SHA_checks':16,
        'total_technical_GET':168,'HTTP200':106,'HTTP400':62,'PUT':0,'DELETE':0,'amount':'NOT_ASSESSED',
        'supplemental_pure_read_errors_retained':2,'parent_live_readbacks_reviewed_not_reexecuted':True,
        'reviewer_Docker_provider_permission_io':0}
    with (OUT/'supplemental-receipt-audit.json').open('x',encoding='utf-8') as f:json.dump(result,f,indent=2);f.write('\n')
    print(json.dumps(result))

if __name__=='__main__':main()
