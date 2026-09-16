# ADR-0008: Protect `main` and use a stable required CI gate

- Status: Accepted
- Date: 2026-09-16

## Context

The platform is entering implementation. The default branch must not accept unreviewed or unvalidated changes, and branch protection must not depend on an unstable implementation-specific job name.

## Decision

`main` is protected with pull-request-only changes and a required GitHub Actions status check named `required`.

The CI workflow may evolve internally, but it must continue to publish the stable `required` gate. The gate succeeds only when all required CI jobs succeed.

Recommended repository rules for `main`:

- Require a pull request before merging.
- Require at least 1 approving review.
- Dismiss stale approvals when new commits are pushed.
- Require conversation resolution before merging.
- Require status checks to pass before merging.
- Require branches to be up to date before merging.
- Required status check: `required`.
- Block force pushes.
- Block branch deletion.
- Apply rules to administrators as well, except for explicit emergency bypass.

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
- Any emergency bypass should be auditable and exceptional.
