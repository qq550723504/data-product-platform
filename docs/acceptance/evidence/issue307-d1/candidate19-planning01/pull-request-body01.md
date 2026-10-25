The retained v28 business dispatches blocked a new bounded execution: P2 had a confirmed DENIED attempt and consumed budget, while T1/T2 had no send claim. This change closes them through the existing reconciliation and trusted NOT_SENT Commands, retains the original attempts, costs, budgets and pins, and prepares candidate19/v29 with a new execution and three independent empty budgets.

The frozen proposal covers only P2 → T1 → T2 → H2A → M1A → M1B, max one attempt each and six total. P1 remains excluded; eleven survivor versions and all prior evidence/environments/backups remain. The proposed window is 2026-10-22 Singapore14:00–14:40. This preparation grants no DELETE or scheduling authority. #332 remains Draft/unmerged and #307 remains OPEN.

Validation:

- Actual-schema PostgreSQL closure regression:48 bounded cases, failed/invented proof rejection and same-key replay without another HEAD.
- Real source closure and independent result review:append-only releases/revocations, one journaled exact HEAD, preserved original row multisets/pins/budgets; no DELETE.
- Three actual native source/restore preparation groups:298 paired technical read requests, nine new backup hashes, isolated business reruns and source Core unchanged.
- Fixed complete native entry:17 versions,41 disabled identities,9 fresh NoSuchUser,149 paired requests, source Core before=after.
- Final cold readback:820 known CIDs stopped,21 old budgets unchanged,3 new budgets empty,57 actual backup files retained.
- Independent source tests:8 Go packages and67 Python methods pass; one explicit Windows symlink privilege skip. Frozen packet and exact final PR HEAD CI have separate review/check receipts.

Authoritative scope is the bounded source merge contract, candidate19 packet, v29 confirmation and runbook. Blockers are limited to authorization/credential boundaries, immutable history/cost, conflicting contracts, required path/build/CI or concurrency/idempotency violations. Future hardening and full D1 acceptance are separate. No Ready, merge, old authorization reuse, cleanup, hold release or bypass is performed.
