# Required acceptance gate

Follow-up to #91. The active main-protection ruleset (23543992, read on
2026-09-17) requires the GitHub Actions check named `required`. Keep that name;
do not weaken or bypass the repository ruleset.

The ci workflow calls three existing workflows at the same commit via
`workflow_call`: browser contracts, live Core acceptance, and demo lifecycle.
For code, migration, workflow, dependency, deployment, test, or other
non-documentation changes, `required` depends on these plus the four existing
job groups and accepts only an explicit `success` for every heavy dependency.

Pure documentation changes are an explicit, narrow exception. A change is
`docs-only` only when every changed path is under `docs/**`, is a repository
Markdown/MDX file, or is a GitHub issue/pull-request template. Workflow files,
scripts, source, tests, migrations, dependency manifests, deployment files, and
all other paths are non-documentation.

For a `docs-only` change:

- the `classify-changes` job must explicitly succeed;
- all seven heavy acceptance job groups must explicitly report `skipped`;
- `required-checks.mjs` still runs and verifies those exact results;
- any missing, failed, cancelled, neutral, successful-heavy-job, or malformed
  dependency result fails the docs-only gate;
- the verified-source artifact is not produced because no executable/runtime
  acceptance claim is being made.

This exception is a CI resource optimization, not historical-green reuse and not
a bypass of the protected `required` status. A cancelled or ambiguously
classified run is not proof of acceptance. No polling across unrelated runs,
privileged `workflow_run`, write token, or historical green status is used.

The called workflows retain manual dispatch for diagnostics, but no longer
start duplicate PR/push runs. A manually dispatched child alone cannot satisfy
`required`.

The policy regression suite covers both modes: full acceptance requires all
seven groups to succeed; documentation-only acceptance requires all seven groups
to be skipped after successful classification. Missing, failure, cancelled,
neutral, malformed, or mixed-mode results fail closed.

After all non-documentation job groups succeed, CI archives tracked source with
`git archive` and its exact checkout SHA for review/reproduction. The archive
contains no runtime data, node_modules, credentials from the runner, or compiled
images. Downloading it is not equivalent to a production deployment.

Sources: https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows
and https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks

Remaining review checkpoint: inspect the candidate diff and actual results,
then use the normal approved merge process. No self-approval, merge, ruleset
mutation or production deployment is performed by this change.
