# Entity review UI — Issue #75, incremental delivery

This change implements the entity-review portion of #75 only. Product detail,
independent release readiness gates, publishing, evidence navigation, and the
full reference-POC browser acceptance test remain open. Do not close #75 or #13.

## Operator flow

Open `/reviews`, compare original and normalized source records with the candidate
ID and provenance, enter a reason (1–2000 characters), then confirm or reject.
The browser calls a Server Action. Before posting a Core command, the server:

1. Requires an explicitly enabled POC flag and configured workspace/reviewer UUIDs.
2. Fetches the job and verifies its workspace and identity.
3. Fetches that job's candidates and verifies candidate membership and PENDING state.
4. Requires a candidate entity for confirmation; rejection never creates an entity.
5. Calls the existing `/confirm` or `/reject` command with a trimmed reason and
   server-derived `X-Actor-ID`. Core owns state transitions, mapping and evidence.

The returned Core job state is displayed; the queue, workbench and dataset lists
are revalidated. A POST timeout is treated as an unknown outcome, not a proven
failure. No write is automatically retried. Refresh and inspect before retrying.
The queue is paginated and does not equate zero pending candidates to readiness.

## Local / trusted POC configuration

Add to `apps/web/.env.local` (the existing API/workspace settings still apply):

```dotenv
# Default is read-only. Enable only behind a trusted local/network boundary.
POC_ENABLE_REVIEW_ACTIONS=true
# Set to the UUID of the actual operator used for this isolated POC.
POC_REVIEWER_ID=
```

A configured actor is **not authentication or authorization**. Never trust a
reviewer supplied by the browser. Before external or multi-user deployment,
replace the shared POC actor with a verified session identity, enforce reviewer
permissions and workspace membership in Core, and restrict the direct Core API.
The UI's ownership checks do not secure direct calls to global Core endpoints.
Do not expose this console publicly just because the flag exists.

Next.js and eslint-config-next are bumped from 15.2.4 to 15.5.24, the maintained
15.x security release documented on 2026-08-25:
https://nextjs.org/blog/august-2026-security-release
This is a targeted dependency update, not a claim that a complete security audit
has passed. A dependency lockfile is not present in the base web scaffold.

## Validation

`npm run test:reviews` compiles the framework-independent boundary and runs Node
contract tests with an injected fake Core transport (no external service needed).
`npm run build` runs these tests before Next's production build so the existing
web CI build also gates them. Also run `npm run typecheck` and `npm run lint`.

Locally verified for this delivery: 24 transport/validation tests, including
confirm/reject, actor spoofing, workspace/candidate scope, missing reasons,
non-pending candidates, missing entity, malformed responses, 409 and timeout.
These are not live Go/PostgreSQL integration tests or browser E2E tests.

Browser acceptance still required against a real seeded POC:

- Default configuration shows a read-only queue; missing reviewer blocks writes.
- Empty/whitespace reason cannot submit; the focused controls are keyboard usable.
- Confirm/reject reaches Core and removes the candidate after refresh.
- The returned job state and generated version, when present, are visible.
- Concurrent review and timeout show an actionable message, without blind retry.
- Paging can reach pending candidates beyond the first 25 records.
- Inspect Core audit/evidence to verify reviewer, reason, decision and source.

The standalone Next build, lint/typecheck and browser acceptance must be recorded
from actual runs; the 24 tests alone do not establish end-to-end completion.
