# Required acceptance gate

Follow-up to #91. The active main-protection ruleset (23543992, read on
2026-09-17) requires the GitHub Actions check named `required`. Keep that name;
do not weaken or bypass the repository ruleset.

The ci workflow now calls three existing workflows at the same commit via
`workflow_call`: browser contracts, live Core acceptance, and demo lifecycle.
`required` depends on these plus the four existing job groups. Its `always()`
condition lets it report failures when a dependency fails or is skipped, and
the executable policy accepts only an explicit `success` for every dependency.
A cancelled run is not proof of acceptance. No polling across unrelated runs,
privileged workflow_run, write token, or historical green status is used.

The called workflows retain manual dispatch for diagnostics, but no longer
start duplicate PR/push runs. A manually dispatched child alone cannot satisfy
`required`. The ordinary parent CI handles main and the temporary #91 base
branch so the next incremental slice can be reviewed before #91 merges.

The policy regression suite covers missing, failure, cancelled, skipped, neutral,
malformed and inherited results. These tests validate the gate program; they do
not replace an actual all-services run or independently establish branch-rule
administration. The existing Hop/Splink path-aware runtime checks remain as-is;
a successful unrelated smoke wrapper does not establish a new engine run.

After all job groups succeed, CI archives tracked source with `git archive` and
its exact checkout SHA for review/reproduction. The archive contains no runtime
data, node_modules, credentials from the runner, or compiled images. Downloading
it is not equivalent to a production deployment.

Sources: https://docs.github.com/en/actions/how-tos/reuse-automations/reuse-workflows
and https://docs.github.com/en/pull-requests/how-tos/merge-and-close-pull-requests/troubleshooting-required-status-checks

Remaining review checkpoint: inspect the candidate diff and actual results,
then use the normal approved merge process. No self-approval, merge, ruleset
mutation or production deployment is performed by this change.
