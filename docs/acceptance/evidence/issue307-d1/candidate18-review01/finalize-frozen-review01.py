"""Seal bounded c18 frozen preparation review; no runtime or authority changes."""
import datetime as dt
import hashlib
import json
from pathlib import Path
import sys

OUT=Path(__file__).resolve().parent
ROOT=OUT.parents[4]
REL='docs/acceptance/evidence/issue307-d1'
BASE=ROOT/REL
PLAN=BASE/'candidate18-planning01'
PACKET=BASE/'remaining-candidate18'
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes as audit

def need(value,message):
    if not value:raise ValueError(message)
def load(path):return audit.decode(path.read_bytes())
def meta(path):
    raw=audit.unlinked_absolute(path).read_bytes()
    return {'sha256':hashlib.sha256(raw).hexdigest(),'bytes':len(raw)}
def save(name,value):
    with (OUT/name).open('x',encoding='utf-8',newline='\n') as target:
        json.dump(value,target,indent=2)
        target.write('\n')

analysis={name:load(OUT/name) for name in ('preparation-file-analysis01.json','business-file-analysis01.json','frozen-file-analysis01.json','complete-entry-file-analysis01.json')}
need(all(value['status'].startswith('PASS_INDEPENDENT_C18_') for value in analysis.values()),'independent analysis incomplete')
complete=analysis['complete-entry-file-analysis01.json']
for name,expected in complete['documents'].items():need(meta(ROOT/'docs/acceptance'/name)==expected,'reviewed confirmation/runbook changed')
need(complete['documents']['issue307-d1-candidate18-confirmation.md']['sha256']=='d38318b208c24d9d0054fbf3bdf27f2911aa043fbb1db1ade2c2b94cc58ff35f','confirmation changed')
need(complete['documents']['issue307-d1-v28-window-runbook.md']['sha256']=='e10480c9900c4b5663eb67bc06f06de32cd7495d14c72474e91c84ca6c5a4191','runbook changed')

# Explicitly bound, already terminal external inputs. The changing acceptance
# matrix, this live review, future manifest/plan and publication receipts are
# excluded so neither index can become a self-reference.
members=set()
for directory in (PACKET,BASE/'remaining-candidate18-readonly01'):
    members.update(x for x in directory.rglob('*') if x.is_file())
for name in ('allocation.json','owner-descriptors.json','non-delete-terminal.json','non-delete-evidence-index.json','budget-initialization.json','profiles.json','retained-before.json'):
    members.add(BASE/'remaining-candidate18-preparation01'/name)
for name in ('independent-final-review01.json','evidence-index.json','merge-contract.md'):
    members.add(BASE/'candidate18-source-review01'/name)
for name in ('candidate18-host-bindings01.json','host-anchor-readback01.json','engine-service-bindings01.json','allocation-readback01.json',
             'final-cold-inventory01.json','authority-schedule-absence01.json','retained-backups48-readback01.json',
             'observe-final-cold01.py','publication-secret-scan01.json'):
    members.add(PLAN/name)
members.update(x for x in (PLAN/'postfreeze-attester01').glob('*') if x.is_file())
members.update(ROOT/'docs/acceptance'/name for name in complete['documents'])
host=load(PLAN/'candidate18-host-bindings01.json')
members.update(audit.bound_path(ROOT,name) for name in host['files'])
files={x.relative_to(ROOT).as_posix():meta(x) for x in sorted(members)}
save('reviewed-artifacts01.json',{'status':'C18_FROZEN_REVIEWED_EXTERNAL_ARTIFACTS','count':len(files),'files':files})
external_spec={'id':'c18-frozen-reviewed-external-artifacts','index':(OUT/'reviewed-artifacts01.json').relative_to(ROOT).as_posix(),
    'sha256':meta(OUT/'reviewed-artifacts01.json')['sha256'],'format':'files-map-v1','member_root':'.','expected_count':len(files),
    'expected_status':'C18_FROZEN_REVIEWED_EXTERNAL_ARTIFACTS'}
external_verified=audit.audit_index(ROOT,external_spec)

report={
    'status':'PASS_BOUNDED_C18_FROZEN_NONDELETE_PREPARATION_REVIEW',
    'at':dt.datetime.now(dt.timezone.utc).isoformat(),
    'reviewer':'/root/c18_independent_review',
    'scope':'Actual frozen candidate18/v28, non-delete native preparation, full readonly-entry receipts, precise confirmation/runbook and recorded final containment. Independent work is file-only; no Docker/database/provider operation is performed.',
    'source_head':'618f484061c3624ff2195c0b591e7d8e3d743ce1',
    'packet_commit':'baa1555dab9b4fcaf058109b53a81e49ac13ccda',
    'execution':'4fd5a7e7-779e-41d7-a35a-3bab7dd615b5',
    'profile':'candidate18/v28',
    'remaining_blockers':[],
    'candidate_sha256':'bb0924c1814b0bcdecd33d3b57a21a24beaafce2a3d64e10e5dac9aba1947f63',
    'bindings_sha256':'8d021af81d905cc6de255bf84e460fc0b416d5af5bacc8388cfbb2b423a60624',
    'template_sha256':'5cd148132b380b4a3361ae7210c8cc02fe6932fee3824ae77b4fec0cc2523a90',
    'source_freeze_sha256':'3e5e8a28621d18ae41c82fe32105b092e64fbb5623d2ae3d3a00f0b3e72104f0',
    'source_zip_sha256':'4f1a7f8fd0c8a6df4a35136be98667033e2f69aa6b3aaa7d9a96ce2c60a2d9ad',
    'window':{'not_before':'2026-10-21T14:00:00Z','not_after':'2026-10-21T14:40:00Z','readonly_until':'2026-10-21T16:40:00Z','singapore':'2026-10-21 22:00-22:40'},
    'checks':[
        {'status':'PASS','subject':'Frozen runtime/build and authority boundaries','finding':'2129 raw/LF source members match the actual frozen runtime ZIP; 12 original build members and reader/operator/root-owner/checker bytes match. Current six identities, execution and three independent budgets do not reuse any old authorization. Wire key remains only in wire; root credentials are absent from case/operator configs and host key is excluded from workers.'},
        {'status':'PASS','subject':'Source/restore Core and physical backups','finding':'All three source before/after, clean restore, rehearsal restore and clean-after-trial full relations/roles/ACL fingerprints match their exact accepted baselines. Three Core roles and source/restore native are distinct. All 48 actual backup files match: 9 new plus 36 complete old checkpoint files and 3 retained c14 partial files. The full-entry old_backups_retained=27 field is the unchanged earlier subset statistic; c17 nine and c14 partial three are separately enforced by the fixed 531 historical file set.'},
        {'status':'PASS','subject':'Native versions, protection and IAM','finding':'Six independent Windows file-only replays of source/restore preparation plus three full-entry fresh replays pass official policy ParseConfig/Equals and complete inventory/content/protection checks. Each source/restore set has 17 exact versions, 35 known disabled users (34 sole-policy/no groups plus the one fixed NONE/no-groups partial identity), 9 fresh absent users and exact P1 NoSuchVersion404. Targets and survivors match the frozen template one by one.'},
        {'status':'PASS','subject':'Isolated business rehearsal and historical facts','finding':'P2/T1/T2 get only new-generation admission; no old Processing/DirectData replay or new output occurs. H0 is blocked by a real PG hold race; H1 claim is denied and trusted NOT_SENT closure remains explicitly pending; H2 A is prepared without sending and B remains undispatched. M1 A admission and C GOVERNANCE retention denial are recorded while real mixed outcomes remain NOT_RUN. All 20 old immutable-row multisets survive and Cost/Allocation/Audit/Evidence/Outbox additions agree.'},
        {'status':'PASS','subject':'Actual technical requests and cost facts','finding':'Preparation retains 340 paired starts/observations: 274 source/restore GET/HEAD and 66 MinIO admin requests. The fixed fresh full entry retains another 137 GET/HEAD pairs. Real HTTP 200/400/404 and all original physical-attempt/UNKNOWN/budget facts are preserved. Amount stays NOT_ASSESSED; file-only/provider_io=0 fields are not interpreted as zero technical invocation or zero cost.'},
        {'status':'PASS','subject':'Complete-entry topology and final containment','finding':'The actual fixed complete entry passes cases/recoveries/private whitelist, role-specific CID/image/command/mount/network/UID/security contracts and budgets, then closes its callers. Final recorded union is exactly 712 CIDs (623 old +86 allocated +3 readers), all stopped/PID0/noports/no restart; 30 permissions false, six case and six recovery bytes/window exact, 18 old budget SHA/mount sets preserved and three current budgets still empty at UID/GID10001. Twenty-four actual service image/CID bindings and 35 host artifact members match.'},
        {'status':'PASS','subject':'Precise scope, documents and future gates','finding':'The 121-line displayed machine block is byte-identical to the frozen six-target template. P2/T1/T2/H2A/M1A/M1B order and exact copy/version/hash/bytes/hold/retention fields match all six targets and eleven survivors; P1 is permanently excluded. Runbook preserves once-per-target/max6, UNKNOWN no resend, strict fixed window, fresh prerequisites, H1/H2/M1 finite sequencing and failure seal. New human authority and schedule are absent; seven old heartbeats remain PAUSED.'},
        {'status':'PASS','subject':'Required post-freeze regression closure','finding':'Independent post-freeze attester normal and -O each pass three methods including all composed counterexamples. The original author 61-command result remains FAIL with 59 passes and two missing-packet failures; the later successful post-freeze receipts append rather than overwrite it.'}
    ],
    'analyses':{name:meta(OUT/name) for name in analysis},
    'reviewed_documents':complete['documents'],
    'reviewed_external_artifacts':external_spec,
    'reviewed_external_artifacts_audit':external_verified,
    'independent_execution':{'windows_file_only_checker_replays':9,'postfreeze_attester_methods_normal':3,'postfreeze_attester_methods_optimized':3,
                             'Docker_calls':0,'database_calls':0,'provider_calls':0,'real_DELETE':0},
    'recorded_physical_readback_time':load(PLAN/'final-cold-inventory01.json')['observed_at'],
    'retained_reviewer_failure':{'path':'reviewer-composition-failure01.json','cause':'The first file-only wrapper incorrectly selected a final revoked read profile by its old active SHA. Original script/error are retained; matching the unique AuditID plus Enabled=false fixed only reviewer file selection. Frozen runtime/gates/resources were untouched.'},
    'pending_independent_gates':[
        'Final static approval manifest/plan must be audited by the same typed full entry --audit-only after this report; no future manifest/plan is represented as already passed.',
        'The final publishing HEAD, exact-head pull_request CI, review threads and Draft/Open status need the separate publication readback; this report does not substitute source/packet green for that CI.',
        'Any deletion needs fresh direct human authorization for this exact candidate/execution/window and separate scheduling instruction. None exists now; all sending permissions remain false.',
        'All fresh native/Core/IAM/version/topology/budget prerequisites must pass again inside the authorized fixed window; recorded preparation does not grant execution authority.'
    ],
    'not_accepted':{'real_six_source_deletes':'NOT_RUN','H1_trusted_final_closure':'NOT_RUN','H2_real_204_late_hold_unknown':'NOT_RUN',
                    'three_copy_real_mixed_outcomes':'NOT_RUN','full_D1':'NOT_ACCEPTED','human_authorization':'NOT_RECEIVED','schedule':'NOT_CREATED'},
    'preservation':{'old_budget_reset':False,'old_pins_released':False,'old_failure_overwritten':False,'old_authorization_reused':False,
                    'cleanup':0,'hold_release':0,'bypass':0,'PR332':'DRAFT_UNMERGED_REQUIRED','Issue307':'OPEN_REQUIRED'}
}
save('independent-final-review01.json',report)
review_files={path.relative_to(OUT).as_posix():meta(path) for path in sorted(OUT.iterdir())
              if path.is_file() and path.name not in ('evidence-index.json','finalize-frozen-review01.py')}
save('evidence-index.json',{'status':'C18_FROZEN_REVIEW_EVIDENCE_INDEX','count':len(review_files),'files':review_files})
index_spec={'id':'c18-frozen-independent-review','index':(OUT/'evidence-index.json').relative_to(ROOT).as_posix(),
    'sha256':meta(OUT/'evidence-index.json')['sha256'],'format':'files-map-v1','member_root':OUT.relative_to(ROOT).as_posix(),
    'expected_count':len(review_files),'expected_status':'C18_FROZEN_REVIEW_EVIDENCE_INDEX'}
verified=audit.audit_index(ROOT,index_spec)
print(json.dumps({'report':meta(OUT/'independent-final-review01.json'),'review_index':index_spec,'audit':verified,'external_index':external_spec}))
