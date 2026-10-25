# #307 D1 v25 host CLI repair

## Goal and boundary

The approved candidate15/v25 host CLI passes an argparse group-name string to inherited maintenance methods that require the frozen group object. Both `archive-root` and `revoke-case` raise `TypeError` before their required work. The independent reproduction is retained in the authorized checkout's `delete-confirmed-v25-review01` evidence. Its original frozen host SHA256 remains `18eb0e8eb022a328d37908d0423a5289b843e5b608ed3437bb06b9c178c46542`.

Resolve the group once through the existing `Host.group` before calling those methods. Reuse the existing root-owner/case-stage maps, clock guards, archive implementation and single-field revocation. No new infrastructure, lifecycle semantics, provider path or authorization fallback is introduced.

## Verification

The CLI regression uses real inherited `archive_root`/`revoke_case` and real temporary receipt/profile files. It verifies byte-preserving archival and journal retention, original-profile preservation and exactly one disabled field, and rejection of active/failed owners, profile drift, unknown stages and incomplete context. External processes are forbidden in those workflow tests. Separate dispatch checks preserve check/H1 behavior and the containment-only context for seal.

The unmodified CLI fails the new workflow regressions with the independently reproduced `TypeError`. The repaired suite passes all seven methods in normal Python and `-O`; raw before/after output and the repair patch are retained in `delete-confirmed-v25-execution01/review-preparation01` in the authorized checkout. Independent re-evaluation is recorded separately.

## Authorization and retained facts

The repair is developed in a separate checkout. The authorized candidate15 checkout, its approved HEAD, candidate/template/source/binaries, 56 approved artifacts, backups, budgets, pins and failure evidence are retained. The same candidate15 heartbeat is PAUSED with no next scheduled run. No Source/Core/helper start, permission enable, provider request, signing, live seal or DELETE was used for this repair.

Passing repair tests does not make the frozen candidate15 executable. Any future executable proposal must bind the repaired host/source and new evidence explicitly and obtain new human confirmation; the prior authorization must not be applied to changed bytes. #332 remains Draft/unmerged and #307 remains OPEN. This change does not create another candidate, execution or window.
