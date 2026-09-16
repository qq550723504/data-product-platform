# ADR-0008: Protect `main` and use a stable required CI gate

- Status: Accepted
- Date: 2026-09-16

## Context

The platform is entering implementation. The default branch must not accept unvalidated changes, and branch protection must not depend on an unstable implementation-specific job name.

The repository currently has a solo-maintainer workflow, so requiring an approving review from the PR author would deadlock normal development. Review requirements should be tightened when another regular human reviewer is available.

## Decision

`main` is protected with pull-request-only changes and a required GitHub Actions status check named `required`.

The CI workflow may evolve internally, but it must continue to publish the stable `required` gate. The gate succeeds only when all required CI jobs succeed.

Recommended repository rules for `main` now:

- Require a pull request before merging.
- Required approvals: `0` while the repository has only one regular maintainer; raise to `1` when a second reviewer is available.
- Require conversation resolution before merging.
- Require status checks to pass before merging.
- Require branches to be up to date before merging.
- Required status check: `required`.
- Block force pushes.
- Block branch deletion.
- Do not permit routine direct pushes to `main`.
- Keep administrator/emergency bypass available only for exceptional recovery, and treat every bypass as auditable.

When a second regular human reviewer is added, also enable:

- Required approvals: `1`.
- Dismiss stale approvals when new commits are pushed.

## CI policy

Pull requests targeting `main` and pushes to `main` run CI. Feature branch pushes do not run a duplicate workflow when a PR already exists.

Required validation currently includes:

- Go module lock verification (`go mod tidy` must not change `go.mod` or `go.sum`).
- `gofmt` check.
- `go vet ./...`.
- Docker Compose configuration validation.
- PostgreSQL migration application.
- `go test ./...` including PostgreSQL-backed integration tests.
- Builds of API, worker, and migration binaries.

## Consequences

- Direct pushes to `main` are disallowed once the repository rule is enabled.
- CI implementation can be split into more jobs later without changing the branch-protection check name; only the `required` gate must remain stable.
- The `required` gate is intentionally separate from implementation jobs such as `go-platform`.
- Any emergency bypass should be auditable and exceptional.
