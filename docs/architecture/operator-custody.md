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

The existing native config supplies `RootKey`, `RootSecret`, `StorageEndpoint`;
its endpoint must equal the plan endpoint. The profile is itself pinned by
`-profile-sha256`. This example is deliberately unexecutable. Unix storage must
exclude group/other access; Windows permits access only to the current owner,
SYSTEM and Administrators. Symlink/junction paths are refused. Private ACLs must
be provisioned by the separately authorized controller/operator first; the CLI
does not change permissions. Successful stdout contains only acquisition/run/
workspace IDs and manifest digest. Failures emit one generic non-secret message.
