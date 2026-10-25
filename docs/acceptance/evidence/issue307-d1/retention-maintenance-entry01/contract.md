# Exact-two retention maintenance: finite source contract

This slice implements and locally verifies the maintenance entry promised by native-protection-repair01. It does not authorize metadata writes, start retained resources, create a deletion candidate, or reuse the failed C23 execution. Historical preparation plans and failure indexes remain unchanged. This supplement describes the new entry; earlier NOT_IMPLEMENTED receipts remain true for their observation time.

## Ownership and reuse

Core retention/disposal semantics and immutable business facts are unchanged. The finite command lives under `cmd/lifecycle-fixture-retention`, uses the existing official MinIO Go SDK v7 and exact `pilot.RemainingScopeFor` Copy identities. It reuses Docker CLI inspection, cryptography Ed25519 and existing regular/unlinked evidence path rules. It adds no queue, IAM, storage or generic workflow runtime. These are retained development fixtures; no hypothetical compatibility interface is added.

The only two possible mutations are GOVERNANCE retain-until extensions to 2026-10-24T12:40:00Z:

| Group | Namespace/key | Exact version | Required old retain-until UTC |
| --- | --- | --- | --- |
| business-races | dpp-lifecycle307-quarantine/fixture/control-retention.csv | 2606d71f-207b-42a7-b671-4b7f4ca4d194 | 2026-10-23T06:25:44Z |
| mixed | dpp-lifecycle307-quarantine/fixture/m1-c.csv | 81a8402e-390f-4247-9d34-552df5eadeb3 | 2026-10-23T06:33:33Z |

All other Copy/DV/provider/incarnation/bytes/SHA identities come from that existing frozen scope and the raw historical exact-two-proposal01.json (SHA256 ae85a69fee49b556b88c9437be4c4923a370213b9c9c0e92466328932f826d2f). LegalHold must remain false. Content PUT, DELETE, hold mutation, bypass, cleanup, pin release, budget reset and old authorization reuse are prohibited. Same content means no new DatasetVersion.

## Authority and isolation

The default binary has an empty trustedHostPublicSHA256 and cannot run writes. Future preparation must separately freeze a new maintenance ID, <=40-minute window ending before the fixed expiry, two exact owner CIDs/descriptors, the immutable native CIDs, binary SHA, config SHAs, independent budget ID and host public key. Only then may a reviewed build set the exact public-key anchor with the Go linker. No such actual bindings, owner allocation, build anchor, grant or window have been created in this slice.

A fresh signed purpose-specific grant must bind that maintenance ID, complete binding bytes, actual human message SHA, exact window, max1 per target/max2 total and old_authorization_reused=false. This grant must be signed only after a trusted human-authority check; the command cannot infer human consent from a text file or a prior deletion grant. Actual human verbatim text and machine-approved bindings remain separate. The grant and mutable permission are re-read before every HTTP request and again after durable attempt-start writes. Revocation or window expiry stops new calls. In-flight requests cannot be recalled; their original outcome/UNKNOWN is retained.

The Linux worker runs as UID10001:10001, read-only root, no ports/socket/restart/privilege, cap-drop ALL, no-new-privileges and one fixed tmpfs. It joins only its frozen native container's namespace. Exact mounts are RO private files, RW independent output, RW new shared maintenance budget, and RO frozen binary. The host signs a <=1-minute proof only after actual Docker descriptor/mount/network/state inspection, complete budget mount inventory, closed retained-owner set, exact private eight-file whitelist and nlink/path checks. The host signing key is outside every worker private/output mount. Never mount it into a worker.

## One-shot order and facts

1. Claim a permanent invocation via O_EXCL and fsync before preflight. Failed preflight cannot be replayed.
2. Use actual SDK exact-version content GET and returned version/size/delete-marker checks, bytes/SHA, retention GET and legal-hold GET. Require exact old expiry and hold=false; unrelated extension is drift.
3. Persist the mutation slot and physical attempt start before one PutObjectRetention. The transport independently permits only exact GET content/retention/legal-hold or exact PUT retention XML for this owner. No extra query/header/target/action is accepted. SDK MaxRetries=1 means one attempt; HTTP keepalive/HTTP2/proxy are disabled. UNKNOWN is never resent.
4. Read back exact content/version/size/SHA, GOVERNANCE, exact new expiry and hold=false. Save append-only start/observations, confirmed status/safe provider error code/body SHA, transport invocation quantity and amount=NOT_ASSESSED. Response bodies, root credentials and arbitrary error text are not serialized. These fixture journals are not represented as Core CostEvent rows.
5. Save the durable success checkpoint and terminal fact. readback_confirmed is independent from overall workflow success. Persistence failure yields FAILED_NO_AUTOMATIC_RETRY; a visible partial success file does not authorize continuation. The host requires previous owner stopped/PID0/exit0 and its exact PASS terminal/readback before signing mixed. The shared budget also requires the previous exact success and rejects any failure/extra entry.
6. Both success/failure retain every slot, output and budget byte. Close permissions and physically stop both owners and native services. A crash with an invocation/start but no terminal is UNKNOWN and requires trusted read-only investigation, never a rerun.

`retention_maintenance.py` supports only file `check`, host `attest` and `seal`. It does not start resources, enable permissions, issue grants, initialize budgets, schedule, or apply retention. Seal archives old mutable permission bytes exclusively before revocation, continues stopping all known CIDs despite one archive/inspect/stop failure, and returns UNCONFIRMED when any step lacks confirmation. Before an actual maintenance packet is accepted, its complete old-owner list and immutable containment anchor must be separately frozen; an inability to verify the normal packet must not prevent trusted closure using that anchor. This slice has not allocated or tested a real containment packet.

## Completion and review boundary

Required now: new command and host tests (success, concurrency, purpose/key/window/scope/replay, 403/500/response loss, persistence failures, private hardlinks, topology and budget expansion), existing protection/entry/archive regressions, actual Git archive rebuild of separate R, local Linux cross-build and independent source review. Remote exact-P/R PR CI requires a new bounded branch-maintenance authorization; prior C23 maintenance authority is not reusable.

Future operational gates remain NOT_RUN/NOT_AUTHORIZED: actual protected metadata writes, current complete 17-version/inventory/IAM read, precise new maintenance binding/anchor, fresh human write grant, Linux-native directory fsync and real Docker/provider containment, new current-expectation profile, and any later deletion candidate. Local SDK localhost doubles and Docker compositions do not establish these facts. No new deletion window is requested before this first step closes.

Current source blockers are build/required tests failing, mutation/authority expansion, replay/concurrency violating max1/max2, secrets leakage, lost or rewritten facts, or required success/closure incorrectly reported. Other future infrastructure work is outside this finite slice. #332 remains Draft/unmerged; #307 remains OPEN.
