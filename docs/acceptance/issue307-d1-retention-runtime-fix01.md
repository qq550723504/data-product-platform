# #307 D1 retention maintenance runtime boundary

Baseline: `8249b55dec8fcf20de2d378887b9aff66047961f`. This local slice prepares the existing exact-two retention extension. It grants no source start, metadata write, unlock, push or DELETE authority. #332 remains locked/Draft; #307 remains OPEN.

The Windows bind probe actually returned `0777 0:0` for both private and output directories. The production Go custody check correctly rejects those directories. A second necessary path was missing: the sealed C23 deletion preflight cannot validate the old expired protection before maintenance or the changed protection afterward. Starting the current writer immediately also races the host's mandatory running-owner attestation.

## Finite repair

1. Use separately allocated Linux local volumes for each owner's private/output directories and one new budget volume. The trusted Windows host maps each exact volume to Docker's `/var/lib/docker/volumes/<name>/_data` and Docker Desktop's `/mnt/docker-desktop-disk/data/docker/volumes/<name>/_data`. It verifies full tar metadata/bytes, UID/GID10001, root0700, files0600, no links and the complete mount set. The worker retains UID10001, private read-only, no socket/key/root capability. Initializer resources are separate network-none owners. No retained volume is initialized or repaired.
2. The fixed Go `-wait` mode validates bindings and custody, then waits for the bound permission to become true within the fixed window. While waiting it does no provider I/O or budget consumption. The host first inspects the real running owner and signs fresh topology; only then can separately authorized control materialization enable it. The complete signed grant, binary/config identity, topology freshness, budget and authorization checks run again before every invocation. No shell/start race or Boolean topology substitute is introduced.
3. Add a distinct maintenance read-only phase contract. Before/after both start from the original full inventory and exactly-P1-missing present inventory. Before keeps the two exact old expiry values. After permits only those two complete Copy identities to have `RetainUntil=2026-10-24T12:40:00Z`; all other bytes/version/hold/inventory/IAM facts remain exact. Effective expectation bytes get a separate SHA and never replace historical bytes. Reuse the SDK collector, official policy ParseConfig/Equals, P1 and absent-user journals and file-only checker. The old C23 entry remains strict.

## Acceptance and remaining gates

Local tests must cover permission wait/expiry/cancellation, Linux directory fsync, exact volume mapping/metadata/link/mount expansion, all three groups in both read-only phases, and identity/protection/phase/hash counterexamples. Local network-none test resources use fresh synthetic data and stay retained after stopping. A simulated SDK or receipt PASS is not a fresh retained-native PASS.

Operational source allocation, runtime freezing, exact HEAD publication CI, new human authority, fresh Core/17-version/native/IAM before and after checks, real mutation attempts and closure remain separate gates. No old authorization is reused, and no proposed time is treated as authorization. Old R/source, budgets, pins and all failures remain preserved. If the hardcoded deadline cannot be met, this packet expires; no deadline is silently extended.

## Acceptance matrix — 2026-10-24 preparation only

| Gate | Actual result | Evidence / practical limit |
|---|---|---|
| Original Windows bind custody | FAIL_PRESERVED | Actual private/output `0777 0:0`; original `d1-integrated/.../retention-maintenance-packet01/windows-bind-probe01.json` retained |
| New local-volume mapping/custody | PASS for R2 probe | `retention-runtime-fix01/linux-probe01.json`; actual UNC/Linux metadata, bytes, uid/gid, mode, nlink and mount source matched |
| Linux fsync / two-owner / waiting regressions | PASS for R2 probe | Network-none, UID10001, real directory fsync; SDK uses loopback test doubles; retained provider I/O=0 |
| Explicit initial false and fixed `-wait` entry | PASS_OFFLINE for R3 | Missing/null false rejected; host rejects non-waiting command; latest R3 binary Linux rerun passed in fix03 after engine recovery |
| Latest R3 Linux rerun | PASS_AFTER_ENGINE_RECOVERY | Original failure retained in fix02; fix03 ran the same R3 archive in a fresh network-none test container and new volumes; real custody/fsync/wait tests passed, container stopped/PID0, retained source states unchanged |
| Three groups × before/after receipt contract | PASS_OFFLINE | All17 expected versions and P1 absence contract covered in six file-only combinations; not fresh retained-native observation |
| Phase / identity cross-field binding | PASS_OFFLINE | Maintenance receipt has18 fields and compares actual MaintenanceID, phase and bindings SHA; old modes retain15 fields; foreign UUID, absent identity and late-hold phase-only relabel rejected |
| Host bridge regressions | PASS_OFFLINE | Host8 + staging2 methods in both normal and `-O`; host composition uses mocks; real bridge evidence is the separate R2 probe |
| Actual R3 archive rebuild | PASS_LOCAL_CI_ENTRY | Source `53f6f8a6e69c17fa6896a40293d323f6ee9d0958`, parent `215098f0bc36a7d88eb19d0f24ed148ef0fe238f`; SHA `1f9462c13204dddd6f8fc475b2cf21830d094aba14e1677d03848c8d70906557`,6504490 bytes,780 members; pilot filter + five command packages passed |
| PR/base freeze | PASS_READ_ONLY | At08:43Z published HEAD8249 remained locked, Draft/OPEN, exact pull_request run40069249660 had13 successful jobs; Issue307 OPEN, unresolved threads0; not new-source CI |
| Independent review | PASS_BOUNDED_SOURCE_AND_LOCAL_NONDELETE_REVIEW | Initial valid receipt-binding blocker corrected; independent actual R3 archive rebuild, normal/-O host tests and fix03 Linux evidence audit passed; not operational packet/native17 acceptance |
| Fresh source Core /17 native versions /IAM | NOT_RUN_NOT_AUTHORIZED | Source environments were never started; no source/restore data rewritten |
| Actual maintenance packet / signed authority | NOT_ALLOCATED_NOT_AUTHORIZED | Test volumes are not an actual maintenance budget; no actual owner CID, host key, anchored write binary or maintenance grant has been frozen |
| Two retention PUTs / remaining six DELETEs | NOT_RUN_NOT_AUTHORIZED | retention_write=0; DELETE=0; no old grant reused |

The reader and file-only checker are necessary components, not a claim that a new complete operational host invocation is already frozen. After the latest Linux acceptance and exact-source publication, a maintenance-specific source/Core/topology/finally-close composition must be bound with its actual packet and independently checked. The old failed C23 deletion preflight cannot stand in for that composition. Only then can the source-start/read-only scope and exact-two protection-write scope be presented for human confirmation.

Reuse check: this slice reuses Docker local volumes/CLI, the repository WSL tar audit, Go standard-library Root/fsync, existing SDK collectors and official MinIO policy semantics. No new queue, policy engine, workflow runtime or Core domain entity is introduced.

## Engine interruption and containment

During the latest R3 acceptance Docker Desktop/backend exited and the Linux API pipe disappeared. The first failed inspect and a second failed engine-version read remain in fix02. A separately assigned repair agent restored the engine by hidden normal application startup at2026-10-24T08:57:58Z. It issued no container start/stop/reset/prune commands. Both retained source CIDs remained exited/PID0 with identical StartedAt/FinishedAt and restart=no. The previous Desktop exit trigger remains UNKNOWN; availability recovery does not prove a permanent cause repair or whole-host data integrity.

After recovery, fix03 independently allocated fresh synthetic volumes and ran the exact latest R3 Linux test binary in network-none isolation. The probe passed, stopped/PID0, and preserved both source states. Prior probe successes and failures were not rewritten. All full source inspect originals and raw host logs are retained privately with SHA custody manifests; public files contain redacted/whitelisted projections. No source start, real provider call, retention PUT or DELETE was performed.

## Frozen source review and next boundary

Independent final report: `evidence/issue307-d1/retention-runtime-final-review01/05-independent-review.json`, status `PASS_BOUNDED_SOURCE_AND_EXISTING_ACCEPTANCE`, blockers=[]; reviewed source commit `d8b782183d49f201c105e90e4c5215c1b8d7d3f4`. The final documentation/evidence commit preserves that executable source and links the same R3 ancestry. The explicit files-map evidence index is `evidence/issue307-d1/retention-runtime-acceptance01/evidence-index01.json`; the repository typed index audit verifies every original byte count/SHA without recursively guessing older index formats.

The next reviewable action is source-only bounded publication, requiring its own current authorization; the previous8249 publication grant cannot authorize another HEAD. The operational maintenance package and both actual authority scopes remain separate. No new fixed deletion window, candidate or schedule is created by this local acceptance.
