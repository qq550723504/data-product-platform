# #325 stop receipts and exact-byte recovery

## Merge contract

Goal: an accepted bootstrap's historical artifacts survive ordinary stop/resume;
an already stopped run affected by the observed pathname collision can recover
its original evidence and bind the current stop inventory without starting any
service, issuing credentials, or accessing objects.

`deploy/trial/stop_receipts.py` owns the stop-receipt contract;
`stop_recovery.py` owns the finite recovery plan/execute contract. Core remains
the system of record. This change has no Core schema, business command, policy,
worker, processing, quality, certification, delivery, or storage-adapter changes.

The real retained #325 run is the deployed-data exception to the pre-production
compatibility rule: its accepted bootstrap freezes `before-stop-facts.json`,
which the old `runtime.stop` also overwrites. No other legacy format or damaged
artifact is admitted by recovery.

## Stop and resume

Every new stop writes exclusive `stops/<attempt>/started.json`, `facts.json`,
and `observation.json`. An unavailable inventory appends UNKNOWN without
replacing an older successful receipt. `stopInspection` is only the current
projection; it binds the observation and facts by exact absolute path and hash.
The old root-level `before-stop-facts.json` and `before-stop-observation.json`
are never rewritten. Repeating a known STOPPED stop allocates nothing.

Resume checks the bound attempt, run/workspace/actor/root, observation hash,
facts hash, and the existing protected five-version/schema71 checks before any
container start or STS invocation. It does not guess a receipt from timestamps,
use the legacy root slot, or repair an invalid receipt automatically.

## Explicit recovery

After a separately authorized controller deployment, the operator may propose:

```powershell
python -B deploy/trial/trial.py stop-recovery-plan --facts <retained-exact-hash-historical-copy> --output <new-plan.json>
```

This plan is read-only. It requires the same activated STOPPED run, stopped
native owners, committed controller, original ledger/activation bindings,
exactly one mismatched bootstrap artifact at the legacy root stop pathname,
and a separate retained local copy with the original frozen SHA256. All other
ledger artifacts must still match. The current protected inventory must match
the accepted activation. A foreign path, mismatched identity, additional drift,
or an orphan recovery journal refuses the plan.

Recovery source admission verifies the immutable activation document, its
identity/previous business and ledger references, frozen Git sources, original
business artifacts, activated binaries/Web and bound protected profile before
comparing the current committed controller-only delta. It does not call the
ordinary full activation validator before classifying the collision. The plan
still verifies both ledger headers and every ledger file, allowing exactly the
single root stop pathname with its exact-hash retained historical copy. Ordinary
`activation.verify_binding`, `runtime.active_business` and resume source checks
continue to require complete ledger validation; there is no skip/relax flag.

Only separate approval of that exact plan permits:

```powershell
python -B deploy/trial/trial.py stop-recover --plan <new-plan.json> --plan-sha256 <reviewed-sha256>
```

The existing operator lock serializes the operation. The plan hash identifies
one durable recovery journal and one deterministic stop-receipt directory.
Execution preserves both current and historical bytes before atomically
restoring the historical pathname. It never edits an old manifest or changes
its expected hash. It publishes the new stop binding and append-only recovery
manifest/history only after the original ledger validates again.

An interrupted operation remains STOPPED with an UNKNOWN recovery claim;
ordinary resume remains blocked. The same plan can finish from its exact saved
archives, including a lost response after restoration or before registry commit.
A completed same-plan replay returns its existing receipt without restoring
again, rebinding a later stop, or running services. A changed plan/claim or target
outside the two reviewed hashes refuses. Unrecognized partial allocation stays
available for explicit inspection rather than being adopted automatically.

Deployment, original-environment recovery, a subsequent resume, credential
renewal, and object access are separate actions. This PR does not perform them.

## Reuse and validation

Reuse the current operator lock, `bootstrap.put` durable exclusive writes,
existing file fingerprints and native ownership checks, protected inventory
validator, and Python's `os.replace`. This is a repair of the existing trial
assembly contract, not new workflow/storage infrastructure; an OSS workflow
runtime or a second persistence system would not repair these local references.

`test_stop_receipts.py` invokes the real CLI, stop, resume precheck, full
business/activation/ledger validators, receipt/recovery implementation, and
protected probe ledger comparison against temporary files and a real temporary
Git repository. Sources, committed archives, origin, active business and expected
activation proof are not mocked. Native/provider I/O and environment adapters
are simulated. The previous #342 fixture mocked `active_business` and missed the
real activated validation chain; this regression reproduces that failure before
the source-admission fix. Recovery tests cover read-only planning, exact-byte
restoration, same-plan replay, interruptions after restore and before registry
commit, identity/path/hash tampering, unrelated artifact drift, and orphan claims.
These controller regressions do not claim a live restart of the retained run.

Merge blockers: historical bytes/bindings can be lost or changed; any recovery
or replay can start services or issue credentials; resume can bypass a missing
or UNKNOWN receipt; same-plan interruption cannot converge; ownership/identity
can broaden; required tests/CI fail. General incident recovery and unrelated
legacy formats remain outside this finite contract.
