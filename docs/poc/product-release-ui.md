# POC ProductRelease UI

This increment adds operator-facing Data Product detail, ProductRelease readiness, and a guarded publish action for the Enterprise Activity POC.

## Core remains authoritative

The browser never computes or changes release state. It reads Core `ProductRelease` and `ReadinessResult`, displays each gate independently, and invokes the existing publish command only when Core already reports:

- `ProductRelease.status = READY`
- `ReadinessResult.overall = READY`
- every readiness check is `PASS`

The eight checks shown by the UI are:

1. production
2. dataset
3. rights
4. quality
5. compliance
6. contract
7. evidence
8. delivery

A release is not represented as a single opaque quality/readiness score. Blocker codes and available Core readiness details remain visible to the operator.

## Scope protection

Product navigation begins with the workspace-scoped Core read model. Before any publish write, the server action verifies that:

- the product is returned by the configured workspace;
- the release is returned under that product's workspace-scoped release history;
- the global immutable release detail still references the same product;
- the readiness response still references the same release.

These checks prevent the POC UI from accidentally submitting an arbitrary product/release UUID. They are defense-in-depth for the console, not a replacement for API authentication and authorization.

## Publish configuration

Release writes are disabled by default:

```text
POC_ENABLE_RELEASE_ACTIONS=false
POC_RELEASE_ACTOR_ID=
```

For a trusted local/private POC only:

```text
POC_ENABLE_RELEASE_ACTIONS=true
POC_RELEASE_ACTOR_ID=<operator UUID>
```

`POC_RELEASE_ACTOR_ID` is read on the server. The browser cannot provide or override `X-Actor-ID`.

The feature flag and configured UUID are not IAM. Before multi-user or external deployment, actor identity must come from a verified session and Core must enforce workspace membership and command permissions.

## Idempotency and ambiguous outcomes

Every explicit publish submission gets a server-generated `Idempotency-Key` and invokes:

```text
POST /api/v1/product-releases/{releaseId}/publish
```

The UI never automatically retries a publish. If the POST times out or the returned payload cannot be verified, the outcome is treated as unknown: the operator is told to reload and inspect the Core state before attempting another command.

A Core `409` such as `PRODUCT_RELEASE_NOT_READY` or `IDEMPOTENCY_KEY_CONFLICT` is shown as a state conflict and also requires a refresh/recheck.

## What this increment does not do

This UI does not create or mutate:

- Data Contract versions
- Rights snapshots
- Quality results
- Compliance results
- ProductVersion definitions
- DatasetVersions

Those remain separate Core workflows. The product detail page exposes the frozen IDs and delivery assets needed to understand why each readiness gate passes or blocks.

Release → DatasetVersion → Execution → source/evidence trace navigation and the final live-browser POC smoke test remain part of parent issue #75.
