# Remaining D1 native preparation boundary

This is a new preparation profile, separate from consumed candidate05/v15.
`prepare-remaining-native.py` has only three fixed group choices and no send,
enable, approve, cleanup, hold-release, or bypass action. It reuses the current
fixture provisioner, MinIO SDK/admin SDK, PostgreSQL dump/restore, tar and the
existing full-Core fingerprint collector. No new Core/queue/policy runtime.

- business-races: P1, P2, T1, T2 and two protected controls (6 versions).
- late-hold: H0, H1 A/B, H2 A/B and one native-held control (6 versions).
- mixed: M1 A/B/C share a single immutable DatasetVersion/content hash; C has
  native GOVERNANCE retention; another control has native legal hold (4 versions).

There are eight PREPARED case plans. Controls are excluded. H1/H2 plan members
share their case DatasetVersion. Existing preparation modes and consumed sender
profiles are not widened. Only the fixed seven prospective targets get native
delete IAM statements, each conditioned on its actual version. Both native
DeleteObject and DeleteObjectVersion checks are covered by the OSS evaluator;
bare delete, another version, payload read/write, protection changes and bypass
are denied. Native policy attachment requires an enabled user on this pinned server. It is
attached only inside the closed, deny-all fixture with no sender, then immediately
disabled and authoritatively read back. Native delete and inspection users end disabled. Current send budget
is **zero**, with no platform approval/dispatch/attempt or reader/producer roles.

Run from a clean committed checkout, with cached pinned images, a pre-created
restricted `C:/Users/Henry/.codex/fixture-private/remaining-native01` control
directory and freshly verified, unused `10.234.x.0/24` subnets. Each group is
one-shot. Failure stops owned containers, preserves failed facts and all volumes,
and requires diagnosis; it does not retry provisioning or replay writes.

The verifier uses GET/HEAD only against a restored provider in a network-none
namespace. Its bootstrap journal is physical request evidence, not business
CostEvent or actual processing/DirectData/DELETE acceptance. Initial production
inspection uses the existing typed journal/cost path. The source Core remains
unchanged while full globals/dump/roles/ACL and all 194 tables are restored and
compared. Native archive SHA256 and restored file SHA256 sets are checked before starting the restored
provider; all exact versions, payloads, inventory, protection and disabled users
are read back. Helpers and restored copies remain retained and stopped.

The rendered active gateway is a validated **storage route draft**, never
installed; the installed gateway ends SEALED/DENY_ALL. Its H2-A/M1-B branches
reuse native forward + response loss. It does not implement the required wire
pause, authenticated business HTTP path, worker ownership or controlled owner
termination. Existing v15 sender/recovery binaries reject these new profiles.
Freeze them as reference provenance only. The remaining-case runner, restricted
reader/producer identities, barrier/witness and seven-target sender binding must
be implemented and independently checked before a **new executable candidate**
and final exact user confirmation. A preparation hash is not a deletion permit.

The authoritative ordering, holder/owner semantics and acceptance criteria stay
in `docs/acceptance/issue307-d1-remaining-contract.md`. Preparation receipts must
not relabel any remaining native race as PASS. #323 remains Draft; #307 OPEN.

The fixture/operator failures and finite continuations are retained in
`docs/acceptance/evidence/issue307-d1/remaining-native01`. pg_restore uses the
source role explicitly, comparisons use the available BusyBox SHA256 utilities,
startup observes both Docker log streams, and native IAM equality uses the OSS
Policy.Equals set contract. No failed provisioning or restore write is replayed.
The fixed 808a6960 continuation only verifies the already restored volume.
