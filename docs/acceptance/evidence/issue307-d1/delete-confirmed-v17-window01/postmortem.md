# candidate07/v17 window01: aborted before arming

Execution: `effa0385-9308-42eb-bd66-d0c379b6a989`. Actual fixed-window wake: 2026-10-17 02:00:16Z. Initial approved HEAD `b4d0556ad424e15896577a069bcbd1b236647a07` and PR CI38896130651/13 SUCCESS were verified live; #323 Draft and #307 OPEN were retained.

## Outcome

The attempt stopped at 02:04:12Z before enabling any sender. Native DELETE=0, claim=0, budget=0, wire/case/sender starts=0. No target was retried. The existing heartbeat is PAUSED. This failed window run will not resume automatically, even though the original time interval has remaining minutes.

Static authorization and hashes, nine retained backup hashes, 39 bound owner contracts, three internal networks, original source CIDs/images and exact budget users/empty volumes passed. Only business Core/native were started. Its Core fingerprint matched the frozen checkpoint before verification; its frozen native verifier exited0 and returned PASS for all eight exact versions, content, protection, disabled identities, full single-policy associations and official policy equality. The other two groups and the after-readonly Core fingerprint were not run after the failure.

## Root cause

The host composition written during the window added `result['policy_sha256'] == old_result['policy_sha256']` to its aggregation gate. These are raw native policy serialization hashes. MinIO's Action and Resource collections can serialize in a different order while retaining identical policy meaning.

Three of four policy receipts differed only by Action/Resource list order. All four compare equal using the existing MinIO `policy.ParseConfig` / `Policy.Equals`; the original frozen native verifier already used that maintained OSS comparison and correctly returned PASS. The aggregation error was in the host composition, not an actual privilege expansion or inventory/protection drift. The failed host source and traceback are retained in `host-attempt.py.txt` and `fresh-failure-transcript.txt`.

The offline regression replays all four recorded policies and rejects expanded actions, different resources/version conditions and malformed policy. It does not reopen this attempt or change a frozen binary. For a future attempt, the complete fresh aggregation entry must be submitted and reviewed before its window, including replay of serialization-order and authorization-change counterexamples. Raw hashes remain immutable audit evidence; they must not substitute for semantic policy equality. No live gate was bypassed or changed to continue this run.

## Containment evidence

The reviewed containment tool ran immediately. All32 permission files remain false; all7 case and7 recovery configs were disabled with original bytes retained privately. All59 relevant containers, including the new technical readonly helper, were inspected stopped/PID0/no restart/no published ports; three actual gateway files match the frozen sealed hashes. All resources, volumes and backups remain retained. Cleanup/hold-release/bypass=0.

Business IAM closure confirmed four disabled identities and frozen policies via the trusted fixed `disable-all` helper. Late-hold and mixed native containers had never been started, so no new IAM readback was performed for them. The original `seal-result.json` correctly has `all_steps_confirmed=false` for those two unavailable readbacks; physical closure is confirmed separately. They are not relabeled fresh IAM PASS.

Actual technical provider traffic: 34 readonly GET requests; closure used12 admin GET and4 bounded PUT status=disabled requests. There was no native DELETE, hold release or bypass call. Technical request journals are retained and are not presented as business provider-attempt CostEvents. No business runner, wire or sender ran.

Full D1 remains NOT_ACCEPTED. A future sending attempt requires a newly prepared execution/window/exact confirmation after the aggregation path is corrected and reviewed; this run's failed evidence and authority are not recycled.
