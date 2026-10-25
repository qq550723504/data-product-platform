"""Check actual isolated command receipts without database/provider access."""
import hashlib
import json
from pathlib import Path

OUT = Path(__file__).resolve().parent
ROOT = OUT.parents[4]
PREP = ROOT/'docs/acceptance/evidence/issue307-d1/remaining-candidate18-preparation01'
EXECUTION = '4fd5a7e7-779e-41d7-a35a-3bab7dd615b5'

def need(value, message):
    if not value: raise ValueError(message)

def load(name): return json.loads((PREP/name).read_bytes())

business = load('business-races/rehearsal/business/files/six-business-admission.json')
need(business['execution'] == EXECUTION and set(business['admitted']) == {'P2', 'T1', 'T2'}, 'business scope differs')
need(all(v['Admitted'] and not v['Replayed'] and not v['Blockers'] for v in business['admitted'].values()), 'new admissions incomplete')
need(business['native_delete'] == business['new_outputs'] == business['send_claims'] == 0 and not business['old_business_owners_replayed'], 'old business replay or sending occurred')
h0 = load('late-hold/rehearsal/late/files/remaining-H0.json')
need(h0['real_pg_waiter'] and not h0['dispatch']['Admitted'] and 'HOLD_ACTIVE_OR_UNKNOWN' in h0['dispatch']['Blockers'] and h0['native_delete'] == 0, 'H0 real lock case differs')
h1 = load('late-hold/rehearsal/late/files/remaining-H1.json')
need(h1['claim_denied'] and h1['NOT_SENT'] == 'PENDING_TRUSTED_CLOSED_OWNER_ATTESTATION' and not h1['remaining_target']['Admitted'] and h1['native_delete'] == 0, 'H1 overclaims trusted terminal closure')
h2 = load('late-hold/rehearsal/late/files/remaining-H2-ready.json')
need(h2['dispatch']['Admitted'] and not h2['dispatch']['Replayed'] and h2['send_claims'] == h2['native_delete'] == 0 and h2['wire_stage'] == 'SEPARATE_FRESH_EXACT_AUTHORITY_REQUIRED', 'H2 crossed wire authority')
mixed = load('mixed/rehearsal/mixed/files/remaining-M1-A-ready.json')
need(len(mixed['copies']) == 3 and mixed['dispatch']['Admitted'] and mixed['native_delete'] == 0, 'M1 A preparation differs')
protected = load('mixed/rehearsal/protected/files/remaining-M1-C-native-nondelete.json')
need(not protected['dispatch']['Admitted'] and protected['dispatch']['Blockers'] == ['PROVIDER_RETENTION_BLOCKED_OR_UNKNOWN'] and protected['native_delete'] == 0 and protected['mixed_outcomes'] == 'NOT_RUN', 'M1 C denial or mixed outcome overclaim')
need(protected['native_protection']['RetentionMode'] == 'GOVERNANCE' and len(protected['native_protection']['CallIDs']) == 2, 'M1 C native protection evidence absent')
delta = {}
for group in ('business-races', 'late-hold', 'mixed'):
    before = load(group + '/rehearsal/immutable-before.json')
    after = load(group + '/rehearsal/immutable-after.json')
    difference = {table: sum(after[table].values()) - sum(rows.values()) for table, rows in before.items()}
    need(all(difference[k] == 0 for k in ('lifecycle_provider_send_claim', 'dataset_version', 'lifecycle_copy_policy', 'lifecycle_copy_absence_receipt')), 'send/output/policy/absence facts changed')
    need(difference['cost_event'] == difference['cost_allocation'] and difference['cost_event'] > 0, 'cost activities/allocation not preserved')
    need(difference['audit_event'] == difference['evidence'] == difference['evidence_relation'] == difference['outbox_event'] and difference['audit_event'] > 0, 'business evidence/outbox additions inconsistent')
    delta[group] = difference

result = {'status': 'PASS_INDEPENDENT_C18_ISOLATED_BUSINESS_RECEIPTS', 'execution': EXECUTION,
          'business_admitted': ['P2', 'T1', 'T2'], 'new_outputs': 0, 'new_send_claims': 0,
          'H0': 'REAL_PG_HOLD_BLOCKED', 'H1': 'CLAIM_DENIED_NOT_SENT_TRUSTED_ATTESTATION_STILL_PENDING',
          'H2': 'A_ADMITTED_NO_SEND_B_NOT_DISPATCHED', 'M1': 'A_ADMITTED_C_NATIVE_RETENTION_BLOCKED_MIXED_OUTCOMES_NOT_RUN',
          'immutable_table_count_deltas': delta, 'real_DELETE': 0, 'reviewer_database_calls': 0, 'reviewer_provider_calls': 0,
          'limits': 'Prepared Core attempt identities are not physical DELETE calls; source business mutations and all real mixed outcomes remain outside this non-delete rehearsal.'}
with (OUT/'business-file-analysis01.json').open('x', encoding='utf-8', newline='\n') as target:
    json.dump(result, target, indent=2)
    target.write('\n')
raw = (OUT/'business-file-analysis01.json').read_bytes()
print(json.dumps({'status': result['status'], 'sha256': hashlib.sha256(raw).hexdigest(), 'bytes': len(raw)}))
