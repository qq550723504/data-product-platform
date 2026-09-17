# Open-PR consolidation — 2026-09-17

The owner requested processing all currently open PRs before adding features.
Reuse #109 as the final candidate targeting main; do not open another feature PR.
Only close superseded PRs after the combined candidate has passed its own required
checks and has been merged through GitHub's protected PR flow. Preserve branches
and historical commits. A green CI run alone is not a deployment approval.

## Source matrix

| PR | Reviewed source | Included behavior |
| --- | --- | --- |
| #91 | 2de49b775d967edb9d24d8a70b0f997a52a207fd | Full POC, real/browser/demo acceptance, seven-group required gate and current main documentation |
| #94 | f2431685328e65b5315a9be11869f379df55b22f | CSV upload, exact RAW bytes, explicit resolution/manual review, template, live acceptance |
| #95 | d82103e07240a4b6c24df45c21788b8f1c83e026 | Non-finite numeric validation, Splink probability filtering and its CI assertions |
| #96 | 35c48d6e466b5b572ba6a36fccbb0b99bb8e3173 | Deterministic name/address lookup and ambiguous matches entering review without arbitrary assignment |
| #97 | 2db88d3e5a5e41a229eb379860454da81da3fbe8 | Resource/dataset/job workspace checks and STANDARDIZED output constraint, already ported in #109 |
| #109 | 41c188e6fbba6c6429beafece0460838024ce7a5 | Constructor compatibility and no-side-effect HTTP/DB regressions on top of CSV ingestion |

Base main at assembly: c68784f9baad2e35d32dc847e003327a7400345d.
The final candidate preserves current main documentation and the CSV onboarding
instructions rather than replacing either with its stale counterpart.

## Reproduced blocker and resolution

#91's update from main introduced two `export const dynamic` declarations in
`apps/web/app/layout.tsx`. Its run 35209678497 failed TypeScript with TS2451 and
did not execute downstream browser/live acceptance. Keep exactly one declaration
with value `force-dynamic`; do not disable typechecking or static/dynamic runtime
tests. #109 already has that single-declaration implementation.

## Merge requirements

The combined candidate must run Go, Web, Hop smoke, Splink smoke, browser
contracts, live Core and demo lifecycle, with `required` failing on missing,
cancelled or unsuccessful job results. Unrelated Hop runtime steps may remain
skipped under the existing explicit change detector; this is not new Hop runtime
verification. Splink changes must execute their actual runtime checks.

Review unresolved conversations and protect against moving PR heads before the
final merge. No branch-protection changes, self-approval or force push is needed.
The GitHub discussion records the actual final SHA, workflow and merge results;
this document does not claim pending runs have passed.

## Not completed by PR cleanup

Production identity/authorization, entity_mapping workspace-key migration,
Execution reference guards (#110), general production-task UI, arbitrary CSV
mapping and manual selection of an ambiguous canonical entity remain separate
work. Closing a superseded PR must not be represented as completing these items.
