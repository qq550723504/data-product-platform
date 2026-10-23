"""O_EXCL final assembly of an independent read-only C22 packet review.
Only repository-file reads, Git reads, AST parsing and typed index audits occur.
"""
from pathlib import Path
import ast, datetime as dt, hashlib, json, subprocess, sys
ROOT=Path(__file__).resolve().parents[5]
BASE=ROOT/'docs/acceptance/evidence/issue307-d1'
OUT=Path(__file__).parent
PLAN=BASE/'candidate22-planning01'
sys.path.insert(0,str(ROOT/'examples/lifecycle-d1'))
import evidence_indexes as audit

def need(ok,message):
    if not ok: raise ValueError(message)
def load(path):return json.loads(path.read_bytes())
def meta(path):
    raw=path.read_bytes();return dict(sha256=hashlib.sha256(raw).hexdigest(),bytes=len(raw))
def save(path,value):
    with path.open('xb') as stream:stream.write((json.dumps(value,ensure_ascii=False,indent=2)+'\n').encode('utf-8'))
def verified_index(path,member_root):
    doc=load(path)
    result=audit.audit_index(ROOT,dict(id=path.parent.name+'-'+path.stem,index=path.relative_to(ROOT).as_posix(),sha256=meta(path)['sha256'],format=audit.FILES_MAP,member_root=member_root,expected_count=doc['count'],expected_status=doc['status']))
    return doc,result

need(subprocess.check_output(['git','-c','core.longpaths=true','-c','core.autocrlf=false','rev-parse','HEAD'],cwd=ROOT).decode().strip()=='10e39eede9c0b30f1cab1397a4a42c5698479862','packet anchor HEAD drift')
for name,expected in load(OUT/'initial-stable-input-index.json')['files'].items():need(meta(ROOT/name)==expected,'stable reviewed input drift: '+name)
for name,status in [('independent-frozen-file-closure01.json','PASS_FROZEN_FILE_TUPLES_WINDOWS_PRIVATE_ROLES_AND_SAVED_NATIVE_CLOSURE'),('independent-cold-backup-document-and-anchor01.json','PASS_ACTUAL_SAVED_COLD_BACKUPS_AUTHORITY_ABSENCE_AND_BYTE_BINDINGS'),('independent-file-only-verifiers.json','PASS_CANONICAL_AUDIT_ONLY_AND_THREE_SAVED_FILE_AGGREGATIONS')]:need(load(OUT/name)['status']==status,'independent check missing')

# The fixed source-review and author indexes are audited as typed files maps;
# their declared public members are also pinned by this independent review.
inputs=set();results=[]
for path,member_root in [(BASE/'candidate22-source-review02/reviewed-artifacts01.json','.'),(BASE/'candidate22-source-review02/evidence-index.json','docs/acceptance/evidence/issue307-d1/candidate22-source-review02'),(BASE/'remaining-candidate22-preparation01/non-delete-evidence-index.json','docs/acceptance/evidence/issue307-d1/remaining-candidate22-preparation01'),(PLAN/'candidate22-host-bindings01.json','.')]:
    doc,result=verified_index(path,member_root);results.append(result);inputs.add(path)
    prefix=ROOT if member_root=='.' else ROOT/member_root
    inputs.update(prefix/name for name in doc['files'])

def add_tree(folder):
    need(folder.is_dir(),'missing reviewed directory '+str(folder))
    for path in folder.rglob('*'):
        need(not path.is_symlink() and not path.is_junction(),'linked reviewed member')
        if path.is_file():
            need(not any(x in ('scratch','__pycache__') for x in path.relative_to(folder).parts),'unexpected temporary reviewed member')
            inputs.add(path)
for name in ['remaining-candidate22','remaining-candidate22-readonly01','candidate22-planning-review01','candidate22-receipt-restoration-review01','candidate22-ci-review01']:add_tree(BASE/name)
inputs.update(BASE/'candidate22-source-review02'/name for name in ['independent-final-review01.json','reviewed-artifacts01.json','evidence-index.json'])
inputs.update(ROOT/name for name in load(OUT/'initial-stable-input-index.json')['files'])
for name in ['issue307-d1-execution-freeze-boundary.md','issue307-d1-v32-preparation-contract02.md','issue307-d1-v32-preparation-contract03.md','issue307-d1-v32-window-runbook.md','issue307-d1-candidate22-confirmation.md','.gitattributes']:inputs.add(ROOT/'docs/acceptance'/name)
for name in ['final-cold-inventory01.json','authority-schedule-absence01.json','retained-backups75-readback01.json','host-anchor-readback01.json','engine-service-bindings01.json','allocation-readback01.json','v29-receipt-drift01.json','v29-checkout-raw-restoration02.json','no-allocation-after-failure01.json','packet-commit01.json','publication-secret-scan02.json','retained-candidate21-anchor02.json','retained-candidate21-public-index02.json','source-review-readback02.json']:inputs.add(PLAN/name)
for name in ['phase-allocate01','bounded-preparation-run01','phase-allocate02','bounded-preparation-run02','phase-complete-native01','phase-freeze02','phase-initialize-budgets02','phase-materialize02','phase-run-non-delete02','publication-bytes-regression01','source-ci-completion02']:add_tree(PLAN/name)
script_names=['run-preparation-phase.py','run-preparation-phase02.py','run-bounded-preparation01.py','run-bounded-preparation02.py','run-complete-entry01.py','observe-host-bindings01.py','observe-final-cold01.py','observe-backups75.py','write-frozen-documents01.py','finalize-static-bindings01.py','write-postlock-confirmation01.py','write-preparation-matrix01.py','verify-publication-bytes01.py','scan-publication-secrets02.py']
for name in script_names:
    path=PLAN/name;ast.parse(path.read_bytes(),filename=name);inputs.add(path)

runbook=(ROOT/'docs/acceptance/issue307-d1-v32-window-runbook.md').read_text(encoding='utf-8')
confirmation=(ROOT/'docs/acceptance/issue307-d1-candidate22-confirmation.md').read_text(encoding='utf-8')
for text in ['P2→T1→T2→H2A→M1A→M1B','UNKNOWN不重发','H0/H1零send','B永不dispatch','C永不删除','12:40Z','10:40Z','execution-binding','精确HEAD PR CI']:
    need(text in runbook,'future bounded sequence missing: '+text)
need('FROZEN_NOT_AUTHORIZED' in confirmation and 'schedule NOT_CREATED' in runbook,'authorization distinction missing')
save(OUT/'independent-final-input-and-sequence-check01.json',dict(status='PASS_STABLE_INPUTS_TYPED_PRIOR_INDEXES_AND_BOUNDED_FUTURE_SEQUENCE',at=dt.datetime.now(dt.timezone.utc).isoformat(),initial_stable_members_rechecked=26,typed_indexes=results,evidence_scripts_AST_parsed=len(script_names),packet_anchor='10e39eede9c0b30f1cab1397a4a42c5698479862',source_business_execution='NOT_RUN',independent_native_provider_calls=0))

# This map binds public/source evidence only. Never publish private credentials,
# temporary fixture paths, or future generated records. The final report and
# either of its own indexes are deliberately excluded from this member map.
for path in inputs:
    need(path.is_file() and path.is_relative_to(ROOT) and not path.is_symlink() and not path.is_junction(),'invalid reviewed member')
    need(not path.is_relative_to(OUT),'own index/final report accidentally entered external member map')
reviewed_path=OUT/'reviewed-artifacts01.json'
reviewed=dict(status='C22_INDEPENDENT_REVIEWED_ARTIFACTS_INDEX',count=len(inputs),files={path.relative_to(ROOT).as_posix():meta(path) for path in sorted(inputs)})
save(reviewed_path,reviewed)
_,reviewed_result=verified_index(reviewed_path,'.')

members=sorted(path for path in OUT.rglob('*') if path.is_file() and path.name not in ['evidence-index.json','independent-final-review01.json'])
need(all(not path.is_symlink() and not path.is_junction() for path in members),'linked own evidence')
index_path=OUT/'evidence-index.json'
index=dict(status='C22_INDEPENDENT_ARTIFACT_REVIEW_EVIDENCE_INDEX',member_root=OUT.relative_to(ROOT).as_posix(),count=len(members),files={path.relative_to(OUT).as_posix():meta(path) for path in members})
save(index_path,index)
_,own_result=verified_index(index_path,OUT.relative_to(ROOT).as_posix())

report=dict(status='PASS_BOUNDED_C22_FROZEN_NONDELETE_PREPARATION_REVIEW',reviewed_at=dt.datetime.now(dt.timezone.utc).isoformat(),execution='0e5cb8f3-46a9-4339-88e4-7154d6ebe13f',profile='candidate22/v32',host_source='a50b5a6307e79896578669ed53a3e12f08a0a815',runtime_source='81a083751c01a281d42df594f6e91012255081f1',packet_anchor='10e39eede9c0b30f1cab1397a4a42c5698479862',remaining_blockers=[],window=dict(not_before='2026-10-23T10:00:00Z',not_after='2026-10-23T10:40:00Z',SGT='2026-10-23 18:00-18:40',readonly_not_after='2026-10-23T12:40:00Z'),packet={name:meta(BASE/'remaining-candidate22'/name)['sha256'] for name in ['candidate.json','bindings.json','confirmation-template.txt','source-freeze.json']},authoritative_contracts=['AGENTS.md sections 1A,3,13,16,17','docs/acceptance/issue307-d1-execution-freeze-boundary.md','docs/acceptance/issue307-d1-v32-preparation-contract02.md','docs/acceptance/issue307-d1-v32-preparation-contract03.md'],scope='Independent read-only review of the saved actual bounded non-delete preparation and current frozen packet. No new native/Docker/SQL/HTTP/provider call, enabling, permission or schedule mutation, source modification, Git ref update or deletion was performed by the reviewer.',verified_facts=[
'R81/P a50, strict shared scope/windows/policies, actual 12-member original build and private binary hashes,769 runtime archive members and2249 host source members cross-bind. Exact source CI run39903332833 is the completed13-job PR CI for a50; it is not final artifact HEAD CI or main CI.',
'Three-group actual non-delete preparation completed with failure=null, closure and profile revocation receipts. Source/clean/rehearsal restored Core fingerprints and fresh full-entry fingerprints agree. Source Core business Commands/object PUT/DELETE remain0. Isolated rehearsal Commands append legitimate new facts while historical multiset and existing send-claim multiset are preserved; cloned validation is not source execution.',
'18 historical copies include permanently excluded absent P1. The17 current exact versions comprise6 exact ordered target tuples and11 protected survivors. The143-line UTF8/LF/terminal-LF machine template is embedded byte-for-byte exactly once and agrees with candidate, private role-specific frozen files and windows.',
'Actual saved complete native entry PASS covers17 versions/6targets/11survivors,53 disabled known IAM users:52sole-policy/no-groups plus the unique fixed NONE/no-groups partial,15 fresh NoSuchUser, and P1 exact NoSuchVersion404. Independent frozen Windows file-only checker replay passes all3 saved source groups7/6/4 using official policy Equals. Object-lock400 is not object absence.',
'All179 actual technical request start/observation pairs in saved native evidence have unique linked identities and GET/HEAD methods, native audit linkage and group69/58/52 counts. amount remains NOT_ASSESSED. Independent replay has no provider I/O; original technical native observations are real saved author runs.',
'Role distinction is explicit: all30 sender/root permission files have Enabled=false and exact candidate/template/window/count bindings. Six non-delete source-case profiles intentionally have NonDelete=true,Base.Enabled=true,Rehearsal=false with fixed future10:00-10:40 window; their owners are stopped and new authorization/schedule absent. Six separate SyntheticOnly read-only recovery profiles expire12:40 and contain neither DeleteSecret nor WirePrivateKey. Operator/root private mounts, whitelist memberships and role-specific credentials match the frozen contract.',
'All75 actual private backup originals were independently rehashed:66old=63complete+3failedpartial and9new.33budgets retain30old including used markers plus3new initially empty execution/nonce-owned budgets withUID/GID10001, full reconstructed mounted-CID ownership sets and no source/clean/rehearsal volume aliasing. Pins,old failures,UNKNOWN,attempts and costs remain historical facts.',
'Actual saved final cold inventory is the exact1085 unique union996old+86newowners+3newtechnicalreaders,allstopped/PID0/noports/no-restart.30permissionsfalse,new3emptybudget and old-retained gate results match allocation.75host-binding members audit successfully. New authorization NOT_RECEIVED and schedule NOT_CREATED were verified from the final saved absence receipt and current local paths; old schedules are PAUSED or ABSENT.',
'Five v29 receipt restorations were previously independently verified against124issued originals and preserved bad-LF backups. Packet anchor10e now contains their exact original rawCRLF Git blobs. Failed allocate01/bounded01 and original drift receipts remain unchanged; later successful attempt02 is a separate fact.',
'Four actual approval-document byte proofs bind current confirmation/runbook/contract03/childattrs bytes through real temporary Git blob/archive/autocrlftrue/false checkout; current filtered/raw Git object hashes independently agree. Bound author publication-secret-scan02 compares54actual secret values over11029files and12archive members with0findings; reviewer did not claim an independent full secret rescan.',
'Future sequence remainsP2→T1→T2→H2A→M1A→M1B,max1each/max6total,UNKNOWNnoresend. H0/H1zero-send and physical-owner/profile/IAM/gateway closure precede signedNOT_SENT. H2requiresrealA204 beforewitness/holds/revoke/lostresponse,Bneverdispatches. M1requiresAabsence thenBUNKNOWN+C-onlycheckpoint rawcopy/hash beforeBlookup;Cneverdeletes. No new sending or enabling after10:40;readonly12:40does not re-enable or extend send authority.'
],observations=[dict(id='REVIEWER-CASE-ENABLED-ASSUMPTION',status='CORRECTED_REVIEWER_ASSUMPTION_NOT_PACKET_FAILURE',evidence='independent-check-assumption01.json',detail='Initial reviewer helper expected all case Base.Enabled=false; canonical preparation gate requires enabled future-window non-delete case profiles. All original check/correction bytes remain; no source or frozen case was changed.'),dict(id='HISTORICAL-SUBNET-AND-CI',status='HISTORY_PRESERVED',detail='Original source-review01 PASS is historical, superseded by actual subnet blocker and current a50 source-review02 PASS. Old Hop CI failure remains finite UNKNOWN; new exact a50 full source CI independently completed. No old failed receipt was rewritten.')],execution_boundaries=dict(source_six_send_and_recover='NOT_RUN',source_H2_and_mixed_outcomes='NOT_RUN',native_delete=0,source_business_mutations=0,new_delete_authorization='NOT_RECEIVED',schedule='NOT_CREATED',old_authorization_reused=False,cleanup=0,hold_release=0,bypass=0,independent_native_provider_calls=0,final_artifact_HEAD_PR_CI='PENDING_SEPARATE_PUBLICATION_GATE',native_branch_lock='PENDING_FINAL_HEAD_CI',execution_binding='NOT_CREATED',PR332='OPEN_DRAFT_UNMERGED',issue307='OPEN',full_D1='NOT_ACCEPTED'),indexes=dict(reviewed_artifacts=dict(path=reviewed_path.relative_to(ROOT).as_posix(),**meta(reviewed_path),count=reviewed['count'],audit=reviewed_result),evidence=dict(path=index_path.relative_to(ROOT).as_posix(),**meta(index_path),count=index['count'],audit=own_result)),limitations=['Saved actual native observations were independently audited as files and file-only replayed; no live native observations were repeated.','Final static plan/manifest, final artifact commit and exactHEAD CI, native branch lock and execution-binding remain subsequent separate gates. This report creates no execution authority.','Approval excludes speculative cross-slice hardening and follows the current finite AGENTS/contracts boundary.'])
report_path=OUT/'independent-final-review01.json'
save(report_path,report)
print(json.dumps(dict(status=report['status'],remaining_blockers=report['remaining_blockers'],report=meta(report_path),reviewed_artifacts=meta(reviewed_path),reviewed_count=reviewed['count'],evidence_index=meta(index_path),evidence_count=index['count']),ensure_ascii=False))
