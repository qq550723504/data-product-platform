# Bounded operator custody (#325)

## Merge contract

This slice exports exactly two explicitly reviewed, unchanged STANDARDIZED / CURATED
DatasetVersions from their old unversioned `s3://raw/datasets/...` locations to an
already provisioned private local directory. Core owns acquisition, physical-call,
observation, termination, Audit, Evidence and typed CostAllocation facts. The local
receipt is a projection, never a substitute for those facts. This file is the
acquisition contract; `workspace-object-storage.md` remains the protected-copy
contract and `../acceptance/issue307-reference-demand-contract.md` owns existing reference demands.

The operator composition is a private CLI, not an HTTP endpoint or a Core reader
fallback. Its reviewed profile binds run, workspace, actor, principal, provider
witness, exact endpoint, two version/URI/SHA256/size tuples, output directory,
expiry and plan digest. A digest identifies a plan; it does not grant execution
authority. Original native MinIO root credentials remain broader than this tool.
Only a separately authorized operator may supply them; Core STS/IAM is unchanged.

Reuse check: use the already pinned minio-go SDK for S3 signing and GET, and reuse
Core transactions, delivery fence, Audit/Evidence/Outbox and CostEvent. MinIO mc /
AWS CLI do not supply this finite manifest and typed physical-call accounting;
no second transfer stack, IAM, queue or workflow engine is introduced. Existing
protected intents cannot describe these prefix-external, unregistered objects;
inventing a protected copy or provider version to reuse that journal is invalid.

## Invariants

- Prepare validates both Core version identities and source-use restrictions
  under the shared workspace fence. An immutable acquisition contains the entire
  two-member scope atomically. Both DV FKs use qualified reference-admission
  triggers; active acquisitions pin both versions, including first-copy races.
- Before each actual GET, commit an immutable physical call identity and its
  Audit/Evidence/Outbox facts, then fresh-check the exact acquisition and sources.
  Each member permits one transport dispatch. No HEAD, List, region discovery,
  health call, range, query, alternate URI, redirect or retry is forwarded.
- An observation appends status, byte count, digest, EOF/Close proof and invocation
  certainty. Every dispatched transport attempt (including failed/UNKNOWN) gets
  one `OPERATOR_CUSTODY_INVOCATION` CostEvent, quantity 1 invocation, amount unknown,
  linked by `operator_custody_call_id`. Quantity counts a transport attempt, not
  a claim that the provider received it or charged money. Start-only interruption
  remains UNKNOWN and must never be interpreted as zero usage or retried silently.
- Only exact size/hash + EOF + successful Close + durable observation can publish
  a member. Publish private exclusive partial files using same-directory hard
  links (atomic, no overwrite). Unsupported filesystems fail closed. Publish the
  manifest after both files; fresh-check the Core admission before acceptance.
- Reserve `acquisition.json` exclusively in the empty private directory before
  any GET; a second plan cannot share that output directory. Partial hard links
  remain as private evidence. A local manifest alone is never Core acceptance.
- A completed key rehashes the same local files and manifest and makes zero
  provider calls. An incomplete key cannot invoke again, even if no response was
  recorded. Explicit later retries require a new reviewed acquisition/attempt.
- End records are append-only. Success or a synchronously returned failure proves
  the local owner has no active I/O before releasing the acquisition pin. A crash,
  lost DB observation, or uncertain owner leaves the pin active. No TTL releases
  a pin. An owner-stop recovery command is a separate follow-up.
- No body, credential, provider error body, signed header or DSN enters audit,
  evidence, stdout or errors. No source object, DatasetVersion, mapping, workflow,
  existing copy, provider policy or runtime identity is rewritten.

## Original environment is still blocked

This source slice is tested on isolated schema 71. The retained #325 database was
last observed at 68; its activation currently admits custody before migrating
68 -> 69 -> 70. Recording a custody start on 71 therefore requires a separately
reviewed bootstrap/order change and controller admission. This slice does not
change that order or claim the original environment is executable. No migration,
deployment, service/process operation, credential renewal, original-object read,
protected copy or activation is authorized by this implementation.

Independent review, current-head CI and a subsequent explicit execution decision
remain required. Future protected creation continues exclusively through the
existing `workspace-object-copy`; custody acceptance grants no delivery rights.

### Required bootstrap handoff (follow-up under #325)

The existing activation source also pins the #331 baseline and admits only
controller changes. Its `execute` requires schema 68 and `migrate` stages only
through 70. Raising the migration ceiling alone cannot admit this Core change.
The following is the recommended ordering for a separately implemented and
reviewed controller contract; none of these original-environment actions are
performed by this PR:

| Stage | Admission and completion | Permitted effects after separate authority |
| --- | --- | --- |
| Source and plan | Exact reviewed merged source/binary/migration hashes, actual merge-SHA push/main CI, unchanged original identity and frozen prior bindings | Read-only plan; no original service, credential, data or object operation |
| Ledger bootstrap | Stop the exact old web/worker/API owners; prebuild admitted tools; start only the original Core PostgreSQL CID; require fresh schema 68 and exact five-version membership; private write-once pg_dump plus original-column public-table inventory before DDL | Canonical runner against pinned full manifest <=71, ordered 68→69→70→71; verify all original logical rows and five versions preserved; append a bootstrap completion binding; stop Core again |
| Custody | Fresh plan/profile bound to the accepted bootstrap, schema 71, same run/workspace/actor/native profile, exact two object tuples and private destination | Start only original Core→objects→gateway; perform the two journaled GETs; require accepted Core acquisition/termination and typed costs; stop gateway→objects→Core |
| New activation plan | Bind the completed schema-71 bootstrap and accepted Core custody receipt, exact five versions, immutable source/workflow and same native identities | Read-only plan generation, no credential/copy/host side effect |
| Later activation | Separate explicit decision on that reviewed plan and a new source admission contract pinned to the reviewed merged baseline | Require schema 71 with matching bootstrap rather than replaying 68→70; renew the same Core policy, use existing protected-copy writer, verify five versions, then launch admitted hosts |

Original container IDs/images/volumes/networks/ports and process creation identity
tuples must be checked before each owned action. A process that reused a PID is
never a stop target. The bootstrap receipt must retain the original registry,
business/source bindings, pre-DDL backup/inventory, exact migration manifest and
preservation proof. PostgreSQL startup/DDL changes physical WAL/storage; the
preservation claim is original logical business facts and identities, verified
against the original column projection, not byte-identical database files.

The generic `resume.wait_containers` includes storage health and other service
probes, and `refresh_identity` issues STS and object probes. Neither is the
custody readiness path. The custody controller must not add an unjournaled
storage health request, HEAD, GET, region discovery or credential refresh.
Source PostgreSQL, Redis, Hop and all host writers remain stopped in bootstrap
and custody; a source SQL comparison belongs to later separately admitted
activation. The source reference, business mapping and workflow are never rerun.

Each stage needs a durable one-attempt controller journal and explicit incomplete
state that blocks ordinary resume. The canonical runner is atomic per migration,
so loss after 69/70 is not a new schema-68 attempt: preserve it as UNKNOWN for
explicit recovery. Do not run down, restore a backup, reseed, silently retry or
automatically restart hosts. Expiry alone never clears a custody owner pin.
The new activation contract must verify accepted Core custody facts, not merely
the existence/hash of local files, and must pin its new source baseline rather
than removing the existing controller-only source guard. These implementation
and isolation/crash-window tests remain follow-up work, not a claim of readiness.

Merge blockers are boundary bypass, incorrect/rewritable/lost facts or cost,
concurrency/idempotency violations, contract conflict, and failed required checks.
General IAM, generic transfer/recovery tooling, arbitrary member counts, provider
deletion, original-runtime bootstrap, and product acceptance are follow-ups.

## Operator entry (after separate execution authority)

`cmd/operator-custody` takes `-profile`, `-profile-sha256` and
`-idempotency-key`. It requires `POSTGRES_DSN` to an already admitted schema >=71,
and never runs migrations. A private JSON profile has exactly these fields:

```json
{
  "plan": {
    "workspaceId": "<original workspace UUID>",
    "actorId": "<original actor UUID>",
    "principalRef": "<authorized private operator reference>",
    "runId": "<original retained run>",
    "providerWitness": "<reviewed deployment/container witness>",
    "endpoint": "127.0.0.1:<original gateway port>",
    "outputDirectory": "<absolute empty private directory>",
    "planSha256": "<reviewed custody plan digest>",
    "expiresAt": "<explicit UTC deadline within one hour of preparation>",
    "standardized": {"versionId":"<UUID>","uri":"s3://raw/datasets/<exact key>","sha256":"<content SHA256>","bytes":2036},
    "curated": {"versionId":"<UUID>","uri":"s3://raw/datasets/<exact key>","sha256":"<content SHA256>","bytes":640}
  },
  "credentialFile": "<absolute existing private native config.json>",
  "credentialFileSha256": "<reviewed exact config bytes SHA256>"
}
```

The existing native config supplies root-level `RootKey`, `RootSecret`, `DSN`,
`Private`, `Workspace`, `Actor`, and nested `Profile.StorageEndpoint`,
`Profile.Subject`, `Profile.Bucket`, `Profile.StorageTLS`. This is the same shape
produced by `deploy/trial/runtime.py`; there is no root-level endpoint fallback.
Workspace/actor, nested subject (`trial-operator-<runId>`), raw bucket, non-TLS
loopback endpoint and retained private/run path must match the plan.
`POSTGRES_DSN` must equal the native DSN; the output is beneath that retained
private directory and credentials come from its existing `config.json`.
The profile is itself pinned by
`-profile-sha256`. This example is deliberately unexecutable. Unix storage must
exclude group/other access; Windows permits access only to the current owner,
SYSTEM and Administrators. Symlink/junction paths are refused. Private ACLs must
be provisioned by the separately authorized controller/operator first; the CLI
does not change permissions. Successful stdout contains only acquisition/run/
workspace IDs and manifest digest. Failures emit one generic non-secret message.
