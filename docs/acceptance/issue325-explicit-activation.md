# #325 retained ledger bootstrap and protected activation

## Finite contract and reuse

The current source baseline is merged #335,
`6e5c973174404b8f3ea47d8c25b3327d809acaa3`. This change implements the
previously blocked order; it does not execute it on the retained trial.
The same five DatasetVersions, run/workspace/Actor, old business manifest,
frozen workflow, artifacts and native resource identities remain retained.

Reuse runtime ownership/PowerShell verification, the operator lock, canonical
migration runner, pg_dump, original-column public-table inventory, operator-custody
SDK/Core ledger, workspace-object-copy and same-policy STS helper. No second IAM,
workflow, queue, storage transport or schema is introduced. Ansible is a mature
generic alternative, but does not own these immutable same-run bindings; another
control plane is unnecessary for this bounded composition.

Custody/protected semantics belong to [operator custody](../architecture/operator-custody.md)
and [workspace storage](../architecture/workspace-object-storage.md).
This document owns controller order/journals. #307 migrations/executors/IAM and
frozen historical evidence are outside this source write set.

## Source and read-only plans

`bootstrap-plan`, `custody-plan`, `activation-plan` only inspect.
Their real verify→storage_check→PowerShell path transports metadata over UTF-8
stdin, never writes state, lock, diagnostic snapshot, error-log or temporary
inspection files. Only an explicitly requested new --output is written.
Use Python -B to suppress interpreter bytecode caches.

Operator admission JSON supplies the exact merged controller head and ciRunId.
The public GitHub API must prove that commit is an ancestor of main and its
ci.yml run is completed/success, push/main, matching SHA, with a successful
required job and complete job page. API failure refuses before native effects;
no GitHub token or retained storage request is used. Later unrelated main commits
do not alter the selected SHA. Current sources must equal the committed HEAD;
only existing controller paths may differ from #335. Frozen workflow and main CI
workflow are separately pinned; the source guard is never removed.

Plans bind registry/origin/old business hashes, exact daemon/CID/image/config/
volume/network identities, process creation tuples, original five-version facts
and input hashes. A reused live PID or occupied retained port refuses replacement.
Bootstrap can observe live original hosts while containers are stopped, without
stopping them. Custody/activation require exited hosts. New tool hashes are
recorded after building and before any native effect. The helper reuses existing
custody private-path/ACL checks before native operations or SQL/backup; metadata
hashes do not attest current directory permissions. Cached facts are advisory
until live SQL admission repeats them.

## Separate explicit stages

Each executable stage requires a distinct command, reviewed plan/digest and
operator authority under the existing lock. A digest is identity, not authority.

1. **Bootstrap**: allocate exclusive journals/before-state, build/hash tools,
   recheck source/native identity, stop only exact old host owners and confirm exit.
   Start only original Core. SQL-only admit requires schema68 and the exact source
   migration history (50/54/56 intentionally do not exist), three original RAW plus
   original STANDARDIZED/CURATED. Write-once pg_dump and repeatable-read public
   inventory precede DDL. Canonical pinned manifest runs 68→69→70→71; verify
   original-column row multisets and five versions, append binding, stop Core.
   No seed/backfill/source SQL/object read/STS/host launch. Preservation means
   logical facts/identity; startup/DDL necessarily changes WAL/storage bytes.

2. **Custody**: a fresh reviewed private profile binds accepted bootstrap,
   same config/principal, exact two source tuples, bootstrap plan digest, native
   container witness, existing empty private output and expiry within one hour.
   Existing private ACLs are required; no repair. Recheck Core/schema71, start
   Core→objects→gateway only, invoke existing operator-custody once for two
   journaled GETs and typed costs/Audit/Evidence/Outbox. No generic wait_containers,
   health GET, HEAD, discovery, STS or source SQL. Require Core ACCEPTED/quiescent
   termination, exact original request fingerprint, two observed/costed calls and
   local hash/size/manifest. Compare stored expiry at PostgreSQL microsecond
   precision while preserving the original nanosecond request hash. Capture bound
   SQL observation, stop gateway→objects→Core.

3. **Activation plan**: consume accepted bootstrap and Core custody observation,
   original facts and actual custody files. Local files alone are insufficient.
   Resources stay stopped; execution must repeat the SQL authority check live.

4. **Later activation**: under separate authority build admitted API/worker/web
   and helpers, start Core first and recheck schema71 plus exact accepted custody
   before releasing credentials/copies. Then start remaining original services.
   Never migrate or replay68→70 here. Retain Core/source facts/inventory; renew
   unchanged Core policy once using the existing constrained helper. Only
   workspace-object-copy publishes the two files. Verify all five protected
   versions/preservation before same-role/port host launch. Append activation-v2
   binding without replacing old origin/business/source or artifacts.

Ordinary start/resume/restart cannot cross incomplete stages or resume old hosts
after bootstrap before explicit activation. Later resume selects admitted
artifacts and the same protected identity, without rerunning business production.

## Journals, failures and verification

Bootstrap/custody use exclusive creation, flush/fsync and separate action start/
result receipts. Either journal directory's allocation-before-registry orphan
blocks retry. Failure/interruption is UNKNOWN: preserve partial files, calls,
costs, DB and resources. No down, restore, cleanup, seed, new provider attempt or
automatic host restart. Runner commits per migration; loss after69/70 is not a
fresh schema68 attempt. Exact-owner stop remains available, but never clears
journals. Recovery outside the bounded checkpoint below remains a separate follow-up.

## Bounded pre-native bootstrap recovery

Goal: explicitly continue the same bootstrap attempt after the deployed #336
controller failed after build/private admission and before any native action.
The first UNKNOWN observation, failed.json, old plan, before-state and tools are
retained. No generic reset, new bootstrap attempt or ordinary resume is admitted.

`bootstrap-recovery-plan --admission <exact merged head/main-CI admission>` is
read-only, including real verify/storage_check/PowerShell. It binds the UNKNOWN
registry, original attempt/plan digest, exact journal/private members and hashes,
original business/origin/config/five-version facts and current native identities.
Only the known deployed #336 source is a recoverable predecessor. Its exact
checkpoint contains before-state/plan, build and private-admission start/result,
and a Refused UNKNOWN fact. Private files are the two old tools and the single
private-mode input. Any host/Core/SQL/backup/migration marker, additional file,
orphan recovery, changed source/input/native identity or later checkpoint refuses.
Old source identity and original plan membership are revalidated; operator notes
and absence of log text alone are not admission facts.

`bootstrap-recover --plan <reviewed recovery plan> --plan-sha256 <digest>` is a
separately authorized command under the existing operator lock. It re-derives
the plan before allocating exclusive `recoveries/<id>` children in the same
attempt's public/private journals. It builds fresh tools there, proves current
private ACLs, and rechecks source/native/input hashes. It then uses the same
bootstrap executor: stop exact old hosts, start only original Core, admit schema68
and five versions, persist pg_dump/inventory before canonical69/70/71, verify
logical preservation/five versions, stop Core. An append-only resolution links
the first failure, old plan, recovery plan and accepted bootstrap plan. The
registry status is a projection; old UNKNOWN facts are never overwritten.

The original attemptId/planSha256/receipts stay identifiable. Custody binds the
accepted manifest's bootstrap-plan digest, including its newly admitted source;
it never guesses from the old attempt projection. Completed recovery replay
only verifies bindings/files/native metadata. Recovery interruption or allocation
orphan stays UNKNOWN and blocks another recovery; no automatic adoption/retry.

Reuse: existing lock, durable exclusive journals/fingerprints, private ACL helper,
native verifier, source/main-CI gate, canonical migration/backup/inventory and
bootstrap executor. Ansible is a general orchestration alternative but cannot
attest this frozen attempt's business bindings; another control plane is unnecessary.
No schema/Core/IAM/provider SDK, queue or recovery runtime is added.

Merge contract: only this pre-native checkpoint and same-attempt continuation;
all later-stage, custody, ownership-pin and post-migration recovery is out of
scope. Tests must cover real read-only storage verification, stale/mismatched
inputs, every action marker/extra member, private/source/native rechecks before
effects, same-attempt append-only resolution, interrupted recovery and replay.
This source change does not deploy or recover the retained original environment.

Completed stage replay only verifies accepted bindings/artifacts, without SQL,
provider, migration, credential or host effects. The executable CLI may acquire
the existing operator lock; plans never do.

Required checks: real readonly verify/storage path; exact merge/main-CI admission;
five-version/workspace/content identity; ordered boundaries and orphan/crash refusal;
real canonical68→69→70→71→71 replay and original-row preservation; real SQL-only
CLI and SDK two-GET/typed-cost acceptance; nanosecond request identity; zero new
provider calls from SQL inspection; normal and optimized Python tests; independent
review and current HEAD CI. These use fresh owned DBs/controlled loopback only.

Original deployment, host stop, DB migration, custody, STS, protected copy and
activation remain NOT_RUN in this source task. No reseed, expanded Core STS,
human/product acceptance or original-runtime execution is inferred from CI.
