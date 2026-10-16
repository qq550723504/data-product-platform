# #307 LC-13 Retry / trusted owner stop / credential containment contract

Current PR closeout: [#318 final contract and merge gates](issue318-final-closure.md).
This LC-13 contract and its no-delete boundary apply to the dated LC-13 runs below;
the separate candidate05/v15 execution is recorded in the current closeout. #318 remains Draft.

Baseline `44d5a5390f93a0a464f66e642fea61b5d79846e8`, 2026-10-14.
Root sole writer; independent read-only design/implementation/evidence review.
#318 remains Draft. No merge/Ready/deploy, object DELETE, hold/retention release,
bypass or resource cleanup. All prior environments, six unknown04a6 pins and5d
facts/configuration remain sealed and retained. New synthetic fixtures only.

User selected explicit new Retry Execution. Original FAILED and immutable
dependencies/pins/calls/costs are preserved; no FAILED→RUNNING or same-ID pin reuse.
Existing Retry Command, queue/Handler, PinnedEngine and native processing remain
the implementation. Trusted stop releases old occupancy by append-only termination;
new Retry gets its own fresh gates/pins/native calls/output and attempt costs.

Authoritative contracts: docs/architecture/data-lifecycle.md (§concurrency,
credential occupancy and use deadline), docs/product/data-lifecycle-design-acceptance.md
(LC-13), existing lifecycle ReaderTerminationVerifier/TerminateFixtureReader,
Delivery Service replay/containment and workflow Retry state machine. This document
owns this finite acceptance scope; it does not create a parallel product protocol.

## Trusted processing owner stop

Only an operator-owned host verifier may attest a fresh closed native helper.
Bind run/workspace/execution, original exact helper ID/image/binary/entrypoint,
server-configured issuer key, complete sorted pin identities/generations/copies,
actual process exit, stopped container/cgroup, restart policy none, no privileged/
host PID/network/Docker socket/daemon credentials, original private reader config
disabled and native reader/producer identities disabled. No remote engine or
untracked output owner exists in this fixture. Reject missing/foreign/stale/
tampered proof, still-running owner, incomplete pins and unknown/native output
intent. Host observation happens outside DB locks; signature/current subject/full
pin set/output facts are checked under the shared fence. Existing signed-byte
Ed25519 host-attestation pattern is reused; unsigned exit JSON is not authority.

Verifier composition must retain synchronous memory stop checks for normal reads.
External verified proof only accepts this closed native processing scope. All old
pins terminate atomically; failure leaves all pins active. Replay is stable and
does not repeat native calls or facts. Do not delete pins or manufacture transfer
bytes. Explicit Retry happens after verified stop, references the old execution,
fresh-authorizes and produces native output under new IDs. Current restriction,
revoked identity or missing stop proof gives0 new processing reads/output.

## Credential composition and occupancy

Existing CredentialProvider has no installed native implementation. Add only a
closed fixture adapter over existing MinIO/madmin service accounts: stable request
key/capability recovery, bounded native expiry, exact version GET policy, disable
(not delete) for revoke, authoritative native lookup and GET-denial verification.
Payload helpers contain no root. Admin provisioning is separate and narrowly
scoped. Every physical provider attempt and native RPC observation/cost is retained.
No key/secret in Core, Audit/Evidence, logs, public receipts or git.

Native S3 cannot enforce platform consumer/channel by itself. Only platform
redemption is supported: authenticated principal→fixed effective consumer, fresh
current identity/rights/certification/lifecycle gate and shared fence at each use.
Raw S3 bearer/presigned direct mode stays unsupported. No general IAM or public API.
Known use_until/future restriction must cap initial/recovery/replay expiry.

Do not reuse FIXTURE_DELIVERY (direct-data only) for credential ownership. Reuse
the existing DELIVERY material requirement with typed exact copy/DeliveryOperation
FK. Unreleased delivery FK already blocks occupancy; explicitly declare its material
demand before first external issuance, binding provider request key/context in the
trusted adapter. Pending/unknown native
issuance and revoke remain active; a lifecycle lease or terminal ISSUED alone never
releases them. Confirmed containment or authoritative expiry+stopped local owner
permits signed owner-verified reference release, preserving ISSUED and observations.
Declaration-only and signed release-only owner compositions must be distinct;
the normal caller-provided quiescent boolean cannot certify native credential stop.
Reference release/dispatch must fail closed while this occupancy is active.

## Required non-delete evidence and blockers

- Actual native processing GET→process exit, rejected untrusted/missing/stale/
  wrong-generation/partial/output-unknown proof, verified atomic stop+replay,
  real new Retry queue delivery→fresh pins/native output/SUCCEEDED, old facts intact.
- Native credential issuance and same-capability recovery/replay;0 credential
  output on current restriction/identity failure; lost/unknown revoke retains
  occupancy and containment Event/Audit/Evidence/Outbox; confirmed native disable
  and actual GET403 permit verified release; ISSUED history stays unchanged.
- Future use_until expiry cap; expiry alone cannot release unknown/local owner;
  explicit bounded native expiry/containment and fresh attempted use refusal.
- Real PostgreSQL constraints/concurrency/idempotency, relevant race/vet and
  adapter contract tests, independent source/evidence review, updated finite matrix.

Blockers: forged stop trust, incomplete owner closure, pin release on lease/unknown,
secret or scope leak, missing physical cost/history, concurrent/replay bypass,
required native path/CI failure or unsupported PASS claim. After three substantive
rounds, freeze this contract and classify new findings by AGENTS.md §17.

Reuse check: existing queue/workflow, locks, Ports/SPI, delivery attempts/observations,
journal/Cost/Audit/Evidence/Outbox, signed host proof, PostgreSQL and MinIO native IAM
cover commodity capabilities. No new parallel infrastructure, general runtime/IAM/DR or fork.
Native policy equivalence uses `github.com/minio/pkg/v3 v3.1.3` (AGPL-3.0,
same existing MinIO/madmin OSS ecosystem), not a local IAM parser. Existing
minio-go bucket policy helpers cannot compare normalized IAM session policy
conditions/actions/resources. Its module floor updates minio-go/v7 to v7.0.88
and related transitive modules. Only the fixture command imports this library;
Core has no SDK dependency. No extra deployment component or fork. The upgraded
complete binary is independently rerun for native processing GET/PUT; old
binaries retain their original source/dependency evidence boundary.
No new schema is required: existing typed DELIVERY requirement/release carries
occupancy, with the missing native adapter/verified release composition supplied here.
General external-engine stop, same-execution generations,
production restore/DR, public credential providers and full D1/D2 remain follow-up.

The controlled MinIO baseline requires service-account expiry of at least
15 minutes. Host naturally waits on persisted actual native expiry before the
short helper; Core/native clocks are never edited. MinIO also requires management
authority to disable own service accounts: this fixture issuer has exact-version
GET plus ListServiceAccounts/UpdateServiceAccount with native Resource=*.
The boundary is isolated synthetic inventory and fixed gateway/adapter, not
production tenant management. Every payload/issuer/root alias is permanently
sealed after confirmed containment before signed release, including unused aliases.
Forensic recovery is limited to four actual retained f3 native failures and
their original stable key recipe; it cannot create capability or replace history.
