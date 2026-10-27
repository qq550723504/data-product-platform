# Independent Web delta review — PR #363 technical-review follow-up

Frozen baseline: `8495de4e1bb3100e9cf4e73760caaddc02264153` vs base `e0e4f77a576ea4572cf16b3a3475461127f44acc`.
Baseline report: `.data/pr363-full-review-20261025/web-review.md`, SHA256 `9ace121018ed18df88b46a4e1b834dd846777b637abfeb12e584a7310521e18b`.
This delta review is independent and read-only. Only this report was written; no source/runtime/browser/DB/IdP/Git mutation was performed by this reviewer.

## Initial four-file delta: PASS, no introduced blocker

- `apps/web/app/agent/tasks/workbench.tsx:79`: only the opening and closing repair-editor `main` tags become `section`. The `agent-task-form` class, exact plan/task links, pending recovery, current-plan/control-state/action guards, write-disabled predicates and every child handler are unchanged. Nested main no longer receives the shell's generic `main` sidebar margin. **W-N1 RESOLVED**.
- `docs/product/governance-modern-ui-acceptance.md:3`: an explicit historical/superseded banner identifies the old all-path rewrite/4189 descriptions, states current root-only rewrite with Core/Agent deep links and Actions preserved, and links the current merge contract and delivery guide. The original historical evidence is retained. **W-N2 RESOLVED**.
- `tests/leader-demo/preview.ps1:2`: default DistName changes only to `.next-acceptance-identity-leader-tech-review-20261025`. It still matches the existing directory allowlist. Loopback target, inherited-source clearing, false write flags, occupied-port refusal, owned-process stop identity and all remaining launch code are unchanged.
- `tests/leader-demo/acceptance.mjs:60`: repair-editor replay adds `expect(page.locator('main')).toHaveCount(1)` while retaining the exact task route, plan-history refresh and disabled save assertion. No retry, delay, write or loosened assertion was added.

No auth, immutable-history, synthetic/server-state, credential, feature-flag or command boundary changes are present in the executable delta.

## Fresh evidence independently inspected

- `docs/product/leader-demo-20261025-technical-review/acceptance.json` records 21/21 PASS, 16 PNGs, zero uncaught page errors, zero browser business/API writes and zero external requests. It continues to label realSources UNCONFIGURED, businessWrites DISABLED, models/dispatch NOT_RUN.
- All 16 PNG content hashes match that JSON. All 13 recorded source hashes match the current files; `unchangedDuringCapture=true` and `scope=WORKTREE_AT_CAPTURE_START_NOT_RUNTIME_BUILD_ATTESTATION` are preserved. The recorded HEAD is the frozen baseline with reviewed working-tree changes, not a claim that the new changes were already committed or that source hashes attest the runtime build.
- New `05-repair-readonly.png` was visually inspected using view_image. Relative to the frozen old frame, the repair form now starts at the same content alignment as the page title/back link instead of an additional sidebar offset. The disabled save/input affordances and synthetic/read-only page explanation remain visible. This is a one-frame visual check, not whole-product pixel/responsive/accessibility approval.
- Previous screenshot packs are retained; their existing candidate digests were not replaced.
- This reviewer did not execute the replay or start/stop its preview. Recorded runtime PASS is coordinator-produced evidence checked for content/digest consistency, not an independently repeated run.

## Guides and current staging status

The current changes to `leader-demo-capabilities-20261025.md` and `leader-demo-closeout-20261025.md` were read. They point the current PASS/JSON and 16 screenshot links to the new technical-review bundle, retain synthetic/disabled/NOT_RUN boundaries and name the new isolated build. All those evidence targets exist.

At this first report save, the new closeout link to `docs/product/pr363-full-technical-review-20261025.md` is not yet present. The coordinator has stated that audit/coverage material is still being assembled. This is a pending staging check rather than a final broken-link finding; report closure must check that file and the remaining guide/coverage delta once saved.

## Actual validation and limits

- PASS: static exact diff of the four initial files; current guide delta; 16 screenshot hashes; 13 source hashes; one-frame visual comparison; `git diff --check`.
- NOT_RUN by this reviewer: npm/Go/build/browser/live API/DB/Docker/IdP tests and all preview starts/stops. Existing coordinator build/test/replay logs and latest CI are separate proof obligations.
- Initial executable-delta blockers: 0. Both W-N1/W-N2 are resolved. Full audit/coverage-document closure is pending until its final saved content is available.

## Exact inspected hashes at initial review

| File | SHA256 |
|---|---|
| `apps/web/app/agent/tasks/workbench.tsx` | `bbf9afbaff4387139c8b47cdd80ae2426bcc4f1f44e40ac4ffff699b026f0064` |
| `docs/product/governance-modern-ui-acceptance.md` | `d1ce79b13f28201455a86d8b49145e39ab50b1a37ad765a0ad0bbdd2038054d4` |
| `tests/leader-demo/preview.ps1` | `dbdaaf08a4a0ecfd4e1a3672809acd617267aef1fa94650d92cc6aecfb860f16` |
| `tests/leader-demo/acceptance.mjs` | `896903d243b69f3681caf3977f0e9a0cf5d9bb7b1f9ba67c8d9b77ee3424948f` |
| `docs/product/leader-demo-capabilities-20261025.md` | `b0464c44768013277d0db3f8f61f5915003baeeb994c2fd98a2f5a39ccf474b9` |
| `docs/product/leader-demo-closeout-20261025.md` | `2267b70967b4eb50c609403a6a15bbe97d0f22e101777c9c7d812863329e0315` |
| `docs/product/leader-demo-20261025-technical-review/acceptance.json` | `01ff1292185bf222767526fbc3274d1a12a21ad9d9a32e124917f1f1f39db4d2` |
| `docs/product/leader-demo-20261025-technical-review/00-about-boundary.png` | `c99f49e3e8df8b594f92c25bc2ef06464b2b320acf9703eb400912cdedf8698a` |
| `docs/product/leader-demo-20261025-technical-review/01-home.png` | `148b5f806bab93909afccd5437d8a066230c9289a5c2625bcf4b5890cfad62c4` |
| `docs/product/leader-demo-20261025-technical-review/02-agent.png` | `77423f1d854df70a725c10e8dc6e26af4db1293ac074fa3bf443af7ad2302dc1` |
| `docs/product/leader-demo-20261025-technical-review/03-quality-focus.png` | `8c5016f9a0efe7604d753b66c2608fb91becbc1748ad1882169fcd2714e66433` |
| `docs/product/leader-demo-20261025-technical-review/03-quality-report.png` | `3fc6023ba61df100bb64f4b881513e7598200d2a388b37754d44d4967043d8fb` |
| `docs/product/leader-demo-20261025-technical-review/04-agent-repair.png` | `03735868a08d32187f1614af834a900446edeeb501da5a134176f73433c62ee7` |
| `docs/product/leader-demo-20261025-technical-review/05-repair-readonly.png` | `446d3539d35a9c5e680a7b62c49175748079e87f15556b8df2a3d0e87823d39a` |
| `docs/product/leader-demo-20261025-technical-review/06-unknown.png` | `e996252d6bf3b0f0039c3b85f064e752e8232df11a53674d055a9472d3ee6749` |
| `docs/product/leader-demo-20261025-technical-review/07-receipt-required.png` | `618f8968ab3b6b833bdfc1d11366777b01c3c8e95c8fa111ef61edf4b7a93cf0` |
| `docs/product/leader-demo-20261025-technical-review/08-receipt.png` | `3e346dd0a5ceb385ca235b6b6ce450970940b9ef0883155909a7b91bb6aff8b1` |
| `docs/product/leader-demo-20261025-technical-review/09-acceptance.png` | `71d63dc4fa1d417426d307028d3a81a58cff59a0c5f92067fac29649b6a8d9e1` |
| `docs/product/leader-demo-20261025-technical-review/10-archive.png` | `d6fb20d57a5301a2c12b26fb43285cfffd1148a8b890cae81449bf50d9cb4d63` |
| `docs/product/leader-demo-20261025-technical-review/11-invalidation.png` | `eda04cdcdf2353a50b75f6e4ef013160c1b2014ae34400b606c366a7516ba90d` |
| `docs/product/leader-demo-20261025-technical-review/12-trace.png` | `dd5e015727973271e05f1e5a46fcdb5059d33de7ef1a8c65ae9d4e1529b66179` |
| `docs/product/leader-demo-20261025-technical-review/13-correction.png` | `6199d13f1f5b8f4733bd494f40e6c903087cf701b442ad1c2ffd3377d02f08b2` |
| `docs/product/leader-demo-20261025-technical-review/14-storage-unavailable.png` | `8846bee70099b937dbf71cb46663804015be2518c4b1f163ba5667bd6e863299` |


## Final delta consistency closure — PASS

The initial pending audit/coverage staging check above is now CLOSED. The saved audit, appended merge-contract section, current capability/closeout links and preserved evidence were independently re-read. Both W-N1 and W-N2 remain RESOLVED; confirmed delta blockers = 0.

### Final audit and coverage checks

- `docs/product/pr363-full-technical-review-20261025.md` accurately distinguishes three independent reviewers from four assignment groups, original static review from coordinator execution, 328 text changes from 50 binary assets, and current implementation from enterprise-source/business-write/dispatch authority.
- `candidate-coverage.json`: 378 entries, no duplicated paths; all paths, original working-tree SHA256 values and owners exactly match the preserved freeze candidate. Each frozen Git blob ID matches `git ls-tree -r 8495de4e1bb3100e9cf4e73760caaddc02264153`. Group counts are identity 113, Agent 54, integration 59, Web 152. Delta files and new evidence are reviewed separately rather than silently replacing frozen records.
- Four published baseline reports are byte-for-byte copies of their original local reports. This review only checks their preservation and summary consistency; it does not claim to redo the other reviewers' source assignments.
- A digest-label precision issue was raised during this delta check and corrected by the coordinator: `b2f4422eb8b8bbe03d1a7c08072cb1df1b5b7e8b2dfadc5b883490640bacbc66` is explicitly the CRLF-to-LF-normalized candidate payload. The preserved `candidate-original.json` is byte-identical to the frozen local file, raw Windows-byte SHA256 `e2b5c9774703ed1d0d8ffb44a7454e9b19ef2bc7fc9df45e33a6abe22c61bf80`. Both digests were recomputed independently with the stated transformation; no JSON reorder is needed.
- Relative targets in the new audit, appended merge contract, current capability list and closeout guide all exist. Current JSON/16-image links point at `leader-demo-20261025-technical-review`; previous final/intermediate evidence packs remain retained as historical records.
- The new audit/guide states the precise new isolated build default and single-main fix. It does not broaden source admission, identity, case/receipt write, model, worker or owner-execution scope.

### Coordinator validation records, independently read but not run here

- Parsed coordinator-produced Go JSON: 5 package PASS records and 89 test/subtest PASS records; 0 fail, 0 skip.
- Read coordinator-produced Web IAM log tail: 19 tests, 17 pass, 2 real PostgreSQL skips, 0 fail. Read Web Agent log tail: 20 pass, 0 fail, 0 skip.
- Fresh Edge evidence remains 21/21 recorded PASS, with 16 PNG digests and 13 current source digests matching. New frame 05 confirms the layout correction. The one-main assertion and disabled-save check remain in the replay source.
- Audit CI statements are coordinator-read historical exact-baseline evidence; this reviewer did not call GitHub/CI APIs. New-commit required CI remains an independent subsequent gate, as the audit states.
- Final `git diff --check` PASS. No additional executable/backend/flag changes beyond the reviewed wrapper tags, replay assertion and isolated build-name default were introduced during this closure.

### Final verdict and limits

**PASS for this narrow delta and its published evidence consistency. W-N1 resolved; W-N2 resolved; no confirmed auth/history/write-boundary regression.** Baseline report remains the 152-file static source review. This final result is not merge/deployment permission, enterprise-source or formal receipt acceptance, full visual/responsive/accessibility verification, runtime build attestation, or an independent replay of coordinator tests.

Only this delta report was written by this reviewer. No source, browser session, service, database, Docker, IdP, provider, model, dispatch or Git mutation was performed.

## Final inspected document/evidence hashes

| File | SHA256 |
|---|---|
| `docs/product/pr363-full-technical-review-20261025.md` | `ad07941230c7f97b75249ad808875e2b2f5fa0c8a8ea1ec66ca3d9b28d3e1f0b` |
| `docs/product/governance-preview-merge-contract.md` | `18e5485c96fab0d1727be9c05b173c24115ceb5ce30ed9f2b44cea024fff0bd9` |
| `docs/product/leader-demo-capabilities-20261025.md` | `b0464c44768013277d0db3f8f61f5915003baeeb994c2fd98a2f5a39ccf474b9` |
| `docs/product/leader-demo-closeout-20261025.md` | `2267b70967b4eb50c609403a6a15bbe97d0f22e101777c9c7d812863329e0315` |
| `docs/acceptance/evidence/pr363-full-review-20261025/candidate-original.json` | `e2b5c9774703ed1d0d8ffb44a7454e9b19ef2bc7fc9df45e33a6abe22c61bf80` |
| `docs/acceptance/evidence/pr363-full-review-20261025/candidate-coverage.json` | `9e75a5cc9b884454267e1f7594040d66d0d66daf7d748d0a2d808138934c650c` |
| `docs/acceptance/evidence/pr363-full-review-20261025/identity-review.md` | `ece7d630e2217c56ce7cc3fa264d5041fb138e419fb1b224ba5ae28dfd016596` |
| `docs/acceptance/evidence/pr363-full-review-20261025/agent-review.md` | `030427fc6c95e0e6b519c6a925b5b8a249e242ecb5369c97fb8aa2ae5ebbda4d` |
| `docs/acceptance/evidence/pr363-full-review-20261025/integration-review.md` | `a2faffa9903387ac069d5e80da00aad000dc42329ef32a7e21a76438d5d58f7a` |
| `docs/acceptance/evidence/pr363-full-review-20261025/web-review.md` | `9ace121018ed18df88b46a4e1b834dd846777b637abfeb12e584a7310521e18b` |
| `docs/acceptance/evidence/pr363-full-review-20261025/local-validation.json` | `8737c8a825aeaedcd820200d1606219784e66a619e9fa5ebeaa11036e9af508e` |

