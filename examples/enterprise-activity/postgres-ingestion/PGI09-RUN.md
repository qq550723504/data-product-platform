# #298 / PGI-09 real OpenMetadata and Edge execution — 2026-10-11

The provider/browser technical checks passed on an isolated checkout of latest
`main` **eff620bd658209d3434beb7ae4029faf0202583c**. The
[fixed report](results/2026-10-11/pgi09/report.json) records 21 bounded checks,
actual identities, image digests, normalized source hashes and ten Core
checkpoints. **Human explanation remains deferred / NOT_RUN** by user request;
this does not mark all of PGI-09 or #298 complete.

Real OpenMetadata **2.0.5**, revision
`c96845e6131c934dc993d9d3e8bae020365f09a7`, used the
[official release](https://github.com/open-metadata/OpenMetadata/releases/tag/2.0.5-release)
PostgreSQL Compose. Only its database, Elasticsearch, migration and server
services ran, with distinct containers/network/volumes and a loopback server
port. The prior B3 trial was left running unchanged. Three fixed synthetic
TABLE entries were authored through native OpenMetadata APIs; this does
**not** prove automated connector discovery or ingestion.

## Actual browser and independent Core observations

| Action | Actual result |
|---|---|
| Edge prepared, started, recovered and admitted a new batch | `08e7e2c7-bbb4-4a37-bba4-e4ba6ef400be` ACCEPTED; three immutable RAW versions, 6/10/14 rows, original checksums |
| Source detail before binding | UNKNOWN; Core source/schema/owner/classification and accepted batch visible |
| Refresh against healthy authenticated OpenMetadata | OBSERVED; lease table ID `af273cb8-447f-446b-8d6d-d058f12509d7`, native version `0.1`; stable binding ID `9ce38578-8f00-471f-847e-8e01f6895368` |
| Native provider description edit, then reload | Current observation `0.2`, confirmed cache still `0.1`; observation alone did not write cache |
| Stop the actual OpenMetadata server container | STALE, current revision/time unknown; confirmed version/time and object identity retained. Browser refresh failed with METADATA_PROVIDER_UNAVAILABLE |
| Restart the same container with retained volumes | OBSERVED `0.2` with original external/binding identities; cache remained `0.1` until explicit refresh |
| Explicit browser refresh, then reload | Cache became `0.2`; same binding ID/resource/FQN/external ID persisted |
| Core quality/certification during outage and after recovery | Quality `24b97f1b-b768-4993-9670-317b67300445` remained FAIL; certification `8fc5fe3a-e4e2-4d55-b487-304cafd24987` remained REJECTED |
| Delivery eligibility during actual outage and after recovery | Exactly equal BLOCKED response, CERTIFICATION_NOT_CURRENT; browser download disabled |
| Independent SQL comparison | All 15 selected Core tables' complete row hashes/counts equaled the post-quality/certification baseline at all ten checkpoints |

The failed certification also lacks rights/compliance/contract/traceability
prerequisites. This is not a quality-only isolated negative. The trial contains
no published ProductRelease or new successful delivery; prior B3 positive
evidence remains separately scoped. Existing metadata ProductReleased retry
tests passed against real PostgreSQL, but do not constitute browser acceptance
of a published release recovering from this outage.

Screenshots: [initial UNKNOWN](results/2026-10-11/pgi09/unknown-main.png),
[healthy binding](results/2026-10-11/pgi09/healthy-fixed.png),
[current versus cached revision](results/2026-10-11/pgi09/current-version-fixed.png),
[actual outage and failed refresh](results/2026-10-11/pgi09/outage-fixed.png),
[reconnect before refresh](results/2026-10-11/pgi09/reconnected-observation.png),
[persisted recovery](results/2026-10-11/pgi09/reconnected-refreshed.png),
[REJECTED history](results/2026-10-11/pgi09/rejected-after-reconnect.png),
[delivery still blocked](results/2026-10-11/pgi09/blocked-after-reconnect.png).

## Reproduced defects and minimum repair

1. Main accepted an unauthenticated binding POST with **201**. POST and list GET
   now share observation's trusted workspace/resource authorization. The same
   live POST returns **403**. Fifteen denial cases cover missing token, actor
   header without token, foreign workspace, missing workspace and nonexistent
   resource across all three routes. The existing static authorizer resolves
   the actor from server configuration; client actor headers are not authority.
2. Main reported cached `0.1` as OBSERVED after the provider changed to `0.2`.
   Observation now returns the current adapter result and separately reports
   the confirmed cache. STALE does not label cache as a current revision.
3. Main's FQN conflict UPDATE moved the same stable binding to another resource
   with **201**. PostgreSQL conflict update now permits only the existing
   resource; reassignment returns **409 METADATA_BINDING_CONFLICT**. A real
   concurrent two-workspace test proves one owner/one rejection, with stable
   same-owner refresh identity.

This reuses the current metadata Port/OpenMetadata Adapter, ResourceBinding,
static authorizer, PostgreSQL unique constraint and B3 runtime/browser gateway.
OpenMetadata supplies the catalog and PostgreSQL supplies serialization; a
second catalog, IAM/policy engine, queue or workflow runtime would duplicate
existing capabilities. No Core business facts, migrations, certification or
delivery gates, or OSS fork changed. Owner/classification remain Core facts;
Steward remains visibly unknown.

Full `go test ./...` and `go vet ./...` passed. Metadata ran with `-race` against
a separately migrated real PostgreSQL test DB
([output](results/2026-10-11/pgi09/metadata-postgres-race.txt)). All 204 web Command
tests, lint/type checking and production build passed. Ordinary Go tests may
skip PostgreSQL without TEST_POSTGRES_DSN; the explicit real run supplies it.
Local results do not replace candidate CI/review or merged-main CI.

## Reproduce the bounded trial

Use the existing B1/B2/B3 Docker/Go/Node/Python environment and PyYAML to read
official Compose. Use a fresh ignored directory and distinct project name.
`prepare` checks the fixed official Compose SHA-256. `seed` uses the official
local quickstart account and saves its token only in the ignored fixture dir.

```powershell
python examples/enterprise-activity/postgres-ingestion/pgi09_openmetadata.py prepare --output .data/pgi09-om-fresh --project dpp-pgi09-fresh
docker compose -p dpp-pgi09-fresh -f .data/pgi09-om-fresh/compose.json up -d
# Initial startup includes migrations and native catalog seeding. Once healthy:
python examples/enterprise-activity/postgres-ingestion/pgi09_openmetadata.py health --output .data/pgi09-om-fresh
python examples/enterprise-activity/postgres-ingestion/pgi09_openmetadata.py seed --output .data/pgi09-om-fresh
Push-Location apps/platform
go build -o ../../.data/pgi-b3-pgi09-api.exe ./cmd/api
go build -o ../../.data/pgi-b3-pgi09-worker.exe ./cmd/worker
go build -o ../../.data/pgi-b3-pgi09-fixture.exe ./cmd/ingestion-browser-fixture
Pop-Location
$taskOmState = Get-Content .data/pgi09-om-fresh/state.json -Raw | ConvertFrom-Json
python examples/enterprise-activity/postgres-ingestion/b3_runtime.py --output .data/pgi09-core-fresh --binary-prefix pgi-b3-pgi09 --metadata-url ($taskOmState.url + '/api') --metadata-token-file .data/pgi09-om-fresh/token
```

The optional metadata flags feed only the Core API environment; the browser
receives neither provider token nor password. Without these flags, the original
B3 unavailable-provider fixture remains available. Use the existing
[B3 prerequisite preparation](B3-RUN.md#inspect-or-reproduce) with the matching
runtime/binary prefix, then operate in Edge. This run used
`park/quality/enterprise-activity-quality-v1.yaml`, registered `native-quality`
and the actual certification Command.

On lease source detail, refresh the binding. Edit the native table description
with `/api/v1/tables/{id}` JSON Patch, then reload without refreshing to compare
current/cache revisions. Snapshot Core before testing the lifecycle. Stop and
restart only this server with retained volumes:

```powershell
docker stop --timeout 5 dpp-pgi09-fresh-openmetadata-server
# Edge: reload source, attempt refresh, inspect the same quality/history/gate.
docker start dpp-pgi09-fresh-openmetadata-server
# Wait for native health, reload, explicitly refresh binding, then reload again.
```

Compare complete sorted Core row hashes, RAW checksums, binding identity and
gate output across the steps; the report lists the exact tables/facts. Keep
tokens, private environment/DSN files and full container environment dumps
outside artifacts. Production IAM/admission/deployment, arbitrary connectors
and the deferred human explanation remain outside this technical acceptance.

## Ready review repair validation

Ready review of `019497690d` completed on 2026-10-11 at 08:01:05 UTC and found
two required-path defects. Migration 000053 now requires a nonempty confirmed
external ID and uniquely owns `(provider, entity_type, external_id)`; same-owner
rename/replay updates the name on the original binding. Another resource cannot
take that identity under a different name. Existing name uniqueness also rejects
replacement identities and colliding renames with 409. Invalid pre-production
rows fail migration; there is no identity backfill or compatibility fallback.
Resource lookup errors now return a generic 503 before provider work; successful
lookups outside the workspace or for absent resources still return 404.

The [RED output](results/2026-10-11/pgi09/ready-review-red.txt) reproduced two
concurrent owners under different names and incorrect 404s at all three metadata
HTTP routes. The [GREEN output](results/2026-10-11/pgi09/ready-review-green.txt)
uses a freshly migrated real PostgreSQL DB with `-race`, covering rename/replay,
cross-workspace conflicts, name/identity collision, missing identity, and the
503/404 distinction. Full Go tests and vet passed after the grouped repair.

The new API binary and retained real OpenMetadata/browser health path are bound
in [the additional report](results/2026-10-11/pgi09/ready-review.json) and
[Edge screenshot](results/2026-10-11/pgi09/ready-review-health.png). Browser refresh
retained the binding/provider IDs and revision 0.2; another-resource refresh
returned 409; all 15 Core table hashes match the original trial baseline.
The native table `name` JSON Patch returned unchanged name/FQN/ID/version in this
2.0.5 trial, and the original lease name was verified afterward. Native catalog
rename is therefore NOT_RUN; stable-ID rename ownership is proven by the real
PostgreSQL tests. The original 21-check report remains historical evidence for
the earlier binary. Human explanation is still deferred / NOT_RUN.
