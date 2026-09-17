# Enterprise Activity POC UI — Final Acceptance Map

This document maps the minimal POC console to issue #13 and parent UI milestone #75. It deliberately validates the Core product model rather than exposing OpenMetadata, Apache Hop, or Splink as top-level product concepts.

## Operator route map

| #13 requirement | POC route | Core/read-model source | Automated coverage |
| --- | --- | --- | --- |
| Workbench summary | `/` | workspace workbench read model | Next production build + route smoke |
| Data Resource detail | `/resources`, `/resources/{id}` | workspace Data Resource read model | Next production build + route smoke; Go read-model tests |
| Dataset / immutable DatasetVersion | `/datasets`, `/datasets/{id}` | workspace Dataset/DatasetVersion read model | Next production build + route smoke; Core DatasetVersion tests |
| Entity Review | `/reviews` | workspace pending review read model + existing confirm/reject commands | 24 review-command regression tests + Next build |
| Workflow / Execution | `/production`, `/production/{id}` | workspace execution read model + immutable Execution detail | Next production build + route smoke; Core workflow integration tests |
| Data Product / readiness | `/products`, `/products/{id}` | workspace product/releases + Core ProductVersion/Release/Readiness | release-command regression tests + Next build |
| Release evidence trace | `/products/{productId}/releases/{releaseId}` | scoped product/release discovery + Core ProductRelease traceability | trace-scope regression tests + Next build/route smoke; Core traceability tests |
| Evidence center | `/evidence` | workspace products/releases; links into release trace | Next production build + route smoke |

## Acceptance criteria

### Reference flow without direct DB manipulation

The UI is an operator surface over existing Core commands/read models. Review confirmation/rejection and ProductRelease publishing use server actions; neither requires the operator to issue SQL or paste a command into the Core API.

The current reference POC still expects fixture/bootstrap preparation to create the underlying DataResource, Dataset, Workflow, Contract, Rights, Quality, Compliance, ProductVersion, and Release objects. That bootstrap is test/POC setup, not an operator UI requirement.

### Entity Review provenance

The review queue shows:

- source/original values
- normalized values
- candidate entity
- match method
- match rule
- confidence
- engine/model provenance when present
- policy version

Manual confirmation/rejection requires a reason and a server-derived reviewer UUID. Writes are disabled by default and ambiguous outcomes are not automatically retried.

### Dataset semantics

Dataset pages expose the logical Dataset type (`RAW`, `STANDARDIZED`, `CURATED`, `PRODUCT`) separately from immutable DatasetVersion rows. Historical versions remain independently addressable.

### Release readiness

The Product page displays these gates independently:

- production
- dataset
- rights
- quality
- compliance
- contract
- evidence
- delivery

The UI never substitutes a single readiness score. Publishing is enabled only when Core reports `Release.status=READY`, overall readiness `READY`, and every individual gate `PASS`.

### Evidence trace

Release trace navigation follows:

```text
Workspace Data Product
  → ProductRelease
    → EvidenceSnapshot
    → DatasetVersion lineage
      → producing Execution
      → source Data Resource
    → EntityMatchJob / EntityMapping
    → Evidence
    → CostEvent
    → AuditEvent
```

A global traceability lookup is performed only after the release was discovered under the current workspace/product. Returned release/product/workspace-bearing facts are checked again before rendering.

### External engines stay implementation details

OpenMetadata, Hop, and Splink may appear only where their runtime/provenance is relevant (for example `engineType` on an Execution or model provenance on an EntityMapping). They are not first-level navigation concepts and do not own Core state.

## CI acceptance layers

The required CI gate combines complementary checks:

1. **Go platform CI** — migrations, `go test ./...`, API/worker/migrate builds. This includes the reference vertical-slice and traceability integration tests already in Core.
2. **Web command/scope regression** — entity-review, release-publish, and release-trace scope tests.
3. **Next production build** — validates all server/client route modules and type integration.
4. **Production route smoke** — starts the built Next server and performs HTTP GETs for the main POC routes, including dynamic product and release-trace routes. Workspace configuration is intentionally blank in this smoke test so it exercises the safe `SetupRequired` path without requiring a second live Core stack.
5. **Hop/Splink CI** — existing runtime/reference checks remain mandatory and are not bypassed by UI changes.

These layers validate code, API contracts, buildability, route registration, and reference Core behavior. They do not replace a deployment-specific IAM/security test.

## Final live demo checklist

For the actual POC demo environment, configure a real `POC_WORKSPACE_ID` and keep write flags off until an operator identity is provided server-side. Then verify, in order:

1. Workbench shows the reference workspace.
2. Open each input Data Resource and RAW DatasetVersion.
3. Open Entity Review, inspect provenance, and complete any pending review with a reason.
4. Open the resulting STANDARDIZED/CURATED/PRODUCT DatasetVersions and the producing Execution.
5. Open the Data Product and inspect all eight Release Readiness gates.
6. If the Release is Core `READY`, publish it using the guarded server action.
7. Open the Release evidence trace and verify EvidenceSnapshot integrity, DatasetVersion lineage, Execution provenance, entity mappings, Evidence, Cost, and Audit.
8. Navigate the same Release from `/evidence` to prove the operator does not need to paste an arbitrary UUID.

Completion of this checklist, together with green required CI, satisfies the minimal POC UI acceptance of #13/#75. Production IAM and external deployment hardening remain out of scope for this POC.
