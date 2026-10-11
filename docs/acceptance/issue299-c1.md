# #299/C1: negative-energy remediation

## Finite contract

Current integration base: main `253ce3b4ef044a233de1179534422eed04ded660` (#311).
The original browser trial below used main `21762737a76033667df4a138368070868a34fbf8` (#309). One writer, branch
`codex/issue299-c1`; isolated synthetic PostgreSQL/Hop/Core/storage/browser runtime.
#311 subsequently occupied 000057. The coordinated unpublished numbering is now
C1 000058 and #307 D1 000059–000061; each owner changes only its own checkout.
Previously accepted 000056 trial schemas/binaries/evidence stay unchanged.
The current candidate proves installation on an already installed main/000057,
as well as a clean install; it retains both external-execution and remediation
routes. Candidates are rechecked against main before publication. C1 needs no new cost
allocation subject: existing physical Execution and Quality attempts own costs.
#294/#295 and all retained #298 environments remain with their original owners.

Goal: find an actual negative value in the fixed PostgreSQL numeric text column,
assign it to the authenticated pilot operator, execute the approved existing
quarantine workflow, inspect a new CURATED version and recheck it. Certification
and delivery remain separate existing commands and gates.

Authority: #299/#296, AGENTS, existing workflow/quality/rights/certification/delivery
contracts and industry-pack rules. This document owns only the C1 case association
and its acceptance record. It is not another lifecycle blueprint.

## Reuse check

Existing Native/Hop execution, native regex evaluator, immutable QualityAssessment
and Finding, execution quarantine, attempt costs, idempotency, PostgreSQL locks,
principal/workspace resolver, Audit/Evidence and transactional Outbox are reused.
The RAW-only policy expresses the existing nonnegative-energy rule on PostgreSQL's
fixed numeric decimal text; it introduces no guessed maximum or replacement value.
Output recheck uses the existing complete enterprise-activity quality ruleset.
Existing RAW full-product assessment can fail for unavailable product metrics;
that historical failure alone is not proof that a negative reading was detected.

OpenMetadata already owns quality incidents and a governance workflow framework.
Their integration is an upstream proposed ADR
([official ADR and existing implementation context](https://github.com/open-metadata/OpenMetadata/blob/main/adr-incident-manager-governance-workflows.md)).
Those catalog workflows cannot atomically own Core's immutable version/Execution/
Assessment association. C1 adds only those business facts and the operator path;
it does not recreate a ticket engine, scheduler, evaluator, IAM, alert transport or
approval system. No new dependency/provider or paid service is introduced.

## Required invariants

- Server-authenticated workspace/actor; pilot assignment is explicitly to that
  operator. A descriptive catalog Owner, body actor or arbitrary UUID grants no role.
- Detection requires the same immutable assessment's finite-decimal format rule
  PASS and nonnegative rule FAIL, then records its exact failed Finding, frozen
  rule hash and source version. Missing/malformed/NaN readings retain Quality FAIL
  but cannot become a negative-value case; signed zero is not a negative reading.
  Same detection key replays the same quality attempt and case.
- Immutable case header, assignment, execution association, recheck start/outcome
  and manual-close facts. Commands lock the case parent first. No PATCH status.
- Approved workflow identity/hash and the plan's exact workspace-scoped CURATED
  destination are checked server-side; case input must be one
  exact bound input. Case attempt + existing Execution acceptance and its outbox
  obligation commit atomically. Only one active/latest successful execution; a
  failed execution is retried through the existing explicit retry command.
- Same command replay creates no new execution/cost. A real failed invocation and
  explicit new retry have separate existing attempt costs. Unknown outcomes are
  recovered through existing owners, never blindly restarted by the case layer.
- Recheck pins an explicit successful execution/output and stable quality attempt
  before evaluation. Response loss between assessment and association can replay
  that attempt, including after manual closure. Closure blocks new admission;
  it must not discard accepted assignment/Execution or already committed Quality
  recovery, even after a later assignment or admission-plan change. New physical
  evaluation uses a new explicit attempt key.
- Manual closure cannot mark Quality PASS or Certification CERTIFIED. Old RAW,
  assessments, certifications and release/evidence history are protected.
- Display energy input/accepted/quarantined records at the same grain; enterprise
  aggregation output rows are a separate grain. Unknown impact stays unknown.
- Repaired/certified output still uses CurrentDeliveryGate; source revocation must
  block fresh delivery from a stale ALLOWED browser page with zero payload.

## Original end-to-end acceptance record

PASS on 2026-10-11, against code commit
`bc62ebfa8846922916cba8bfa195cf6e495592f3` based on main `21762737`. Subsequent
evidence-only commit `635fa895` did not change that API or production web code.
The review fixes below have their own subsequent verification record.
API SHA256: `2633d367b912fcf58e2bda77b10c6aaa567a42948686728e18f68576b3aa3e3b`.
Production Next build, real Edge, PostgreSQL 16, Hop 2.19.0/JDBC 42.7.14,
existing Native processing/quality adapters, Redis 7 and the existing pinned
MinIO/Caddy storage harness were used. Exact images and independent PostgreSQL
results are in [runtime-proof.json](issue299-c1-evidence/runtime-proof.json).

The final environment was freshly installed through migrations 000055 → 000056;
it contains no #307 schema and shares no Core/storage objects with retained #298.
Trial entry: `http://127.0.0.1:52175/batches`; API `http://127.0.0.1:60161`.
The case is now manually closed and its source authorization revoked, deliberately
retaining the final negative-state evidence. Private credentials remain in the
ignored local harness files; none are needed in the browser.

### Real browser and provider results

1. Registered the source, ran real PostgreSQL → Hop extraction, atomically accepted
   all three RAW versions, completed the existing entity review and processing
   path, then obtained a baseline quality PASS and CERTIFIED decision.
2. Created a nonempty PUBLISHED baseline release
   `ec5bbd6c-fa99-4b93-95d5-67bc4dd9147e` with release dataset membership and an
   EvidenceSnapshot before the C1 repair. Recorded exact historical row identities
   and hashes before testing the repair/fault/replay commands.
3. From the energy RAW version, detected one actual negative value among 14 records
   and assigned case `5f682e33-c5ae-4108-b671-8a18aa4e6b30` to the trusted operator.
   Stopped only the owned storage gateway; the actual Native call ended FAILED
   (`b4f7c6a0-b647-444c-8cda-dd2fa77a9049`). Storage itself stayed running.
4. Started the same gateway and explicitly retried. New Execution
   `267bd3e6-1e37-4332-82c6-5167f7cf4d4d` succeeded, with 14 energy records =
   13 accepted + 1 quarantined. Quarantine retained `-999` and
   `NEGATIVE_ENERGY_KWH`; the separate enterprise aggregation produced 5 rows.
   Output version `c64a947d-c2a2-4104-8006-af0c652951e6` is a new CURATED version.
5. Ran the complete existing output quality ruleset through the independent
   recheck: assessment `b5c24966-a467-41fb-97cb-477ae828f736` PASS. Existing
   compliance/evidence preparation and certification commands produced a separate
   CERTIFIED decision. RAW still produced REJECTED, including
   `QUALITY_GATE_NOT_PASSED`.
6. Downloaded the new output through the real browser: 640 bytes, SHA256
   `a2545a5642fbf076840d2b2f03848973e990b0c12de6a7ffcbfc6dd990af0eb6`.
   Kept an ALLOWED browser preflight on screen, revoked the actual source
   Authorization through its existing Command, then submitted that stale page.
   Core recorded BLOCKED / `CURRENT_ENTITLEMENT_BLOCKED`, returning no dataset
   payload. Historical CERTIFIED and the prior ISSUED operation stayed intact.
7. Before manual closure, the independent HTTP/PostgreSQL oracle passed 24 checks,
   including eight concurrent same-key retry recoveries returning one Execution.
   After browser closure, 26 checks passed: new handling/recheck admission was
   rejected, accepted recheck and close keys recovered their existing identities,
   and all those replays produced zero new execution/quality/cost/audit/outbox facts.
   Missing credentials, foreign workspace and changed same-key inputs were rejected.

The real runtime recorded three Native invocation costs (baseline, failed C1 call,
explicit successful retry), including the failed invocation; three actual quality
invocation costs (baseline, detection, output recheck) were retained. Each physical
attempt owns its cost through the existing allocation model. Replay added none.

All 17 original history table sets matched before fault, after repair/replay and
after closure, including nonempty release/evidence membership. Original RAW bytes,
old assessments, certification, rights and published bindings were unchanged.
The existing audited `READY → SUPERSEDED` transition of the old CURATED version is
permitted; the oracle proves its event/audit and compares every other field exactly.
It does not falsely claim that lifecycle projection status never changes.

![Closed case, original failure and independent recheck](issue299-c1-evidence/c1-closed.png)

![Revocation blocks the new certified output](issue299-c1-evidence/c1-revoked.png)

### Tests and reproduction

- `go test ./...` and `go vet ./...` PASS with the real isolated test PostgreSQL
  DSN, including clean migration installation on a fresh database.
- `go test -race -count=1 ./internal/quality/remediation/... ./internal/platform/routing/... ./internal/workflow/application/...`
  PASS. Real PostgreSQL tests prove eight same-key acceptances, atomic rollback of
  case/Execution/queue facts when proof routing fails, an observed parent-lock wait
  before a late assignment, immutable guards, separate failed/retry costs, failed
  assessment replay, and committed assessment recovery after response loss and
  manual closure. Controlled test adapters are distinguished from the real browser
  runtime above; four controlled physical quality calls include the lost-response
  assessment and do not imply a fourth real browser evaluation.
- Web lint/typecheck and the production build with its existing command tests PASS.
- The browser uses existing B3 fixture Commands for synthetic rights, certification
  inputs and the baseline release; no success/status rows are fabricated by the
  independent C1 oracle. OpenMetadata is an offline projection in this C1 trial;
  #298's real metadata connection/recovery acceptance is not relabeled as a C1 run.

From `apps/platform`, build the API, worker and the existing B3 fixture into the
ignored `.data` directory:

```powershell
go build -o ../../.data/pgi-b3-c1-latest-api.exe ./cmd/api
go build -o ../../.data/pgi-b3-c1-latest-worker.exe ./cmd/worker
go build -o ../../.data/pgi-b3-c1-latest-fixture.exe ./cmd/ingestion-browser-fixture
```

From the repository root, start a new isolated production browser runtime. The
cache can reuse an existing official MinIO build; it owns no historical Core facts.

```powershell
python examples/enterprise-activity/postgres-ingestion/b3_runtime.py --output .data/c1-trial --binary-prefix pgi-b3-c1-latest --remediation-plan-ref park/remediation/energy-quarantine-v1.yaml
```

Follow the browser actions above. After a baseline release exists, capture the
history before C1. After the real download and revocation, compare the same history:

```powershell
python examples/enterprise-activity/postgres-ingestion/c1_verify.py --runtime .data/c1-trial/runtime.json --output .data/c1-trial/history-before.json
python examples/enterprise-activity/postgres-ingestion/c1_verify.py --runtime .data/c1-trial/runtime.json --previous .data/c1-trial/history-before.json --download <actual-browser-download> --output .data/c1-trial/verified-final.json
```

The harness outputs actual localhost URLs; their ports and UUIDs will differ in a
fresh trial. Stop only a trial's owned gateway for the failure, preserve storage,
and use the explicit retry after recovery. Do not reuse the accepted same-key
download for payload recovery; it must remain a non-payload replay result.

Blockers: current C1 path failure, security/tenant bypass, mutable/lost historical
facts or costs, non-atomic acceptance, duplicate real work on replay, concurrency
violating these invariants, required build/test failures.

Follow-ups: #299/C2 source schema observation, multi-operator IAM/assignment,
generic incident/notification UX, full downstream impact, schedules and production
rollout. #298 human explanation remains deferred/NOT_RUN. C1 does not close #299,
does not pass C2/human acceptance, and its PR remains Draft without automatic merge.

## PR #310 review regression

PASS on 2026-10-11 against code commit
`f75cccec609c16e4f132229744d820a85c18808d`. Three original P2 findings were valid:
caller-selected output, ambiguous negative detection, and closure blocking accepted
assignment/Execution recovery. One adjacent browser recovery gap was also fixed.
No migration number, shared routing, Adapter or delivery gate was changed.

The versioned pilot plan now binds `dpp-source-reader-curated`, the existing B3
asset binding. Core resolves that unique code inside the case workspace, requires
CURATED and compares its identity before atomic queue acceptance. It also verifies
the original accepted target for a fresh retry. The workflow's generic logical
output code is not a caller-selected destination for this finite source pilot.
Changed request fingerprints still conflict. Existing accepted keys recover their
frozen facts before fresh-admission closure/assignee/plan checks; this does not
authorize new work. The HTTP trusted workspace/actor boundary still applies.

Real PostgreSQL race tests passed destination rejection before queue acceptance,
blank/malformed/NaN/Infinity and mixed invalid+negative rejection, signed-zero and
positive values, actual negative detection, atomic rollback, concurrency, immutable
history, closed assignment/execution/retry recovery with zero new facts/costs, and
committed Quality recovery after closure/current admission-plan change. Full Go
tests/vet, web lint/typecheck and the production build/command tests passed.

A fresh isolated production environment (`http://127.0.0.1:53390/batches`) used
real Edge, PostgreSQL, Hop, storage and Native processing. Browser actions accepted
the three RAWs, completed the existing entity review and baseline workflow,
detected 1/14 negative readings, assigned the case and generated a new version
in the approved CURATED asset: 14 inputs = 13 accepted + 1 quarantined; 5 aggregate
enterprise rows. The actual independent output assessment committed PASS.

For the recovery fault, a temporary test-only database trigger paused that output's
Quality insert behind an advisory lock. The controller acquired the case parent
lock, released Quality, observed its committed assessment, and terminated only
the blocked case-association database backend. No business status/result rows
were fabricated. Both locks and the temporary trigger/function were removed.
The page showed PENDING, retained the original request key after manual closure
and refresh, and `恢复原复检` recovered assessment
`aee33552-4164-4066-bb66-ac7b8cda4d1d` / recheck
`1444e874-fa5d-419b-8ce5-9af88cb18cf2`. Actual invocation/cost counts and all
previous dataset/assessment/Execution facts were unchanged by recovery. The
recovery added only its legitimate case outcome and Audit/Evidence/Outbox proof.

The same repaired API was separately bound to the retained original C1 database
for recovery verification. All 27 HTTP/PostgreSQL checks passed, including eight
concurrent closed same-key retries, new-key rejection and zero additional work;
all 17 original history sets matched, including nonempty published release and
EvidenceSnapshot membership. This verifies candidate API recovery on real retained
facts; it does not relabel the original browser certification/delivery run as a
new full browser acceptance.

Exact facts, hashes, costs and checks:
[review-recovery-proof.json](issue299-c1-evidence/review-recovery-proof.json).
PR CI and review are checked separately against the pushed HEAD. Human explanation
remains NOT_RUN; the PR remains Draft without automatic merge.

![Closed pending case retains the original recovery action](issue299-c1-evidence/review-closed-pending.png)

![Closed case recovers the committed PASS assessment](issue299-c1-evidence/review-closed-recovered.png)

## Current input and stage review regression

PASS on 2026-10-11 against C1 production code `b37979db`.
The second review's two P2 findings were valid. Core now checks the typed RAW
stage before accepting a detection/Quality attempt. The approved workflow YAML
owns the complete required input contract. Before queue acceptance, Core binds
the three exact RAW identities to the case's accepted ingestion receipt and
requires the actual successful entity-resolution job/output for that receipt's
enterprise input. Fresh retries revalidate the frozen original inputs. Caller
omissions, duplicates, cross-batch replacements and unrelated resolution outputs
cannot allocate an Execution. No second ingestion/resolution authority is added.

Real PostgreSQL tests use existing ingestion Prepare/Dispatch/Recover/Admit and
entity-match Commands to produce actual receipt/job facts with controlled
adapters. They reject every missing input, cross-batch replacements, duplicate
names and CURATED/STANDARDIZED detection, including READY/SUPERSEDED versions,
before queue/Quality work. The full-suite replay oracle now scopes Outbox counts
to the owned case and workspace Executions; unrelated parallel test writes no
longer produce a false failure.

A fresh production trial (`http://127.0.0.1:58985/batches`) used real Edge,
PostgreSQL/Hop extraction, three-RAW acceptance, entity review and Native
processing. Actual HTTP rejection checks passed before work allocation. The
browser then assigned case `1a6fb201-6618-4f08-800c-ece397aa4f49`, observed real
storage-disconnection failure, restored the gateway and explicitly retried to
output `e5befd0f-b68b-4099-b2d9-ab08e2ec5d0b` in the approved asset. Counts remained
14 energy inputs = 13 accepted + 1 quarantined; aggregation produced 5 enterprise
rows. A real database-backend fault after Quality commit again left PENDING.
After manual closure and refresh, the browser recovered the same recheck
`8f7386ae-2ffb-47e1-a441-bd7f1ec99d0e` and PASS assessment
`016cb2f8-f97f-44ba-bde2-ec128c83254e`; protected facts, physical invocation counts
and costs were unchanged. The trial contains three Native invocation costs
(baseline/failure/retry) and two Quality invocation costs (detection/recheck).
Recovery added zero. Non-RAW HTTP detection was rejected with zero new facts.

Main advanced to `253ce3b4` during closeout. The candidate integrates #311 and
preserves its external execution routes alongside remediation. C1 production
application/web/plan code is unchanged by that integration. Unpublished migration
56 became 58, coordinated with #307's 59–61. A separate database actually
installed latest main/57 first and then upgraded to C1/58; retained trial schemas
and historical evidence were not renumbered or rewritten. Full Go tests/vet,
real PostgreSQL race tests and API/worker builds passed on the integrated code.
The integrated API also passed 27 HTTP/PostgreSQL recovery/history checks against
the original retained C1 database; all 17 protected table sets matched, including
nonempty published release and EvidenceSnapshot membership.

These are targeted current-code regression and integration results. The original
complete certification/download/revocation browser acceptance retains its exact
original code/binary attribution above. Sanitized checks, identities, hashes and
costs are in [review-input-proof.json](issue299-c1-evidence/review-input-proof.json).
Manual explanation remains NOT_RUN; PR #310 remains Draft with no automatic merge.

![Current code: real failure, successful retry and closed PASS recovery](issue299-c1-evidence/review-final-closed-recovered.png)
