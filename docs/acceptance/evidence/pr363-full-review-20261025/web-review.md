# Independent Web review — PR #363 frozen baseline

- Review checkout: `C:/Users/Henry/.codex/worktrees/aq1`.
- Frozen HEAD: `8495de4e1bb3100e9cf4e73760caaddc02264153`.
- Compared base: `e0e4f77a576ea4572cf16b3a3475461127f44acc`.
- Assignment: all 152 entries in `.data/pr363-full-review-20261025/web-files.txt` (102 text/config/test/document files; 50 PNG assets).
- Candidate verification: HEAD/base match `candidate.json`; all 152 working-tree SHA256 values match the candidate manifest. Baseline source status was clean.
- Result: **confirmed introduced merge blockers = 0 in this assignment**. Two P3 nonblocking observations below. This is a bounded technical review, not merge, deployment, business-write, live-provider, or complete UX approval.

## Nonblocking observations

### W-N1 / P3 — shell layout selector also indents the repair editor's nested main

- Introduced shell selector: `apps/web/components/governance-demo/styles.css:62`: `.governance-demo main{margin-left:218px;margin-right:0;max-width:1600px;padding-left:48px;padding-right:48px}`.
- Desktop/media variants at lines 64, 65 and 67 likewise target every descendant `main`.
- Cross-owner seam: `apps/web/app/agent/tasks/workbench.tsx:79` returns `<main className="agent-task-form">` for `repairEditor`, inside the shell's existing `<main id="main" ...>` in `components/governance-demo/App.jsx:50`.
- Reproduction from frozen source: open `/agent/tasks/repair?taskId=<existing-task>` at desktop width. Both outer and inner main receive the sidebar margin and shell padding, so the editor is indented a second time relative to the page title. This is also visible in the already-delivered `docs/product/leader-demo-20261025-reviewed-final/05-repair-readonly.png`, the single screenshot inspected visually in this review.
- Impact: layout/landmark defect; the shown required controls remain readable and accessible in the existing frame. No business permission, state, historical fact or credential boundary is changed. Therefore not a merge blocker under AGENTS §17.
- Minimal fix: use a neutral/section wrapper for the inner repair editor (preserving its class/children/handlers), or scope sidebar-layout CSS to the outer shell main. The semantic-wrapper fix is the narrower change.
- Validation proposal: source/render assertion that repair-editor content does not create a nested main; existing Agent repair read-only and action-controller regressions; desktop visual check only when a separately authorized browser/build is available. No preview starts were performed for this review.

### W-N2 / P3 — early acceptance narrative still describes the removed all-path rewrite

- `docs/product/governance-modern-ui-acceptance.md:21` says non-Agent UI routes are rewritten to `/demo`, legacy Core details show an explanation, and legacy Server Components/Actions are held for future integration.
- Current behavior is intentionally different: `apps/web/middleware.ts:6-8` rewrites only `/`; `apps/web/components/shell.tsx:9` keeps every other path in the connected shell; `apps/web/tests/governance-demo/routing.test.mjs:11-14` explicitly retains exact Core identities and existing server-action paths.
- The current authoritative merge contract explicitly records why full-path rewrite was defective and that only root should be rewritten: `docs/product/governance-preview-merge-contract.md:91` (related source inspected, outside assigned ownership).
- Reproduction: compare the early narrative to those exact route predicates. `/reviews`, `/resources/<id>` and `/datasets/<id>/versions/<id>` run the existing Core page rather than the narrative's synthetic replacement. This is intentional current behavior, with per-command feature flags still enforced.
- Minimal fix: mark that early statement as historical/superseded and point to the current root-only route contract; do not reinstate all-path rewrite or block the existing Core API/action paths.
- Not a merge blocker: current authoritative contract, implementation and route regression agree; the inaccurate dated narrative is not the current security implementation.

## Technical review results and seams

1. **Modern shell / original routes:** root/demo is the browser-local dynamic shell; connected pages use a stable SSR-compatible shell, preserve Next server children and do not consume demo providers. Original Core URLs remain reachable. Core workbench is a direct re-export at `/core/workbench`; old root regression URLs were updated without removing the original action assertions. Release-detail redirect retains both exact product and release identities for validation.
2. **Local synthetic vs server facts:** `governance-demo` imports no API/model/dispatch client. Local IDs, actor switches and setting changes are explicit demonstrations. Server facts enter only through connected children. Demo context drawer does not send synthetic version IDs into Agent. `syntheticPreview` is provided by server deployment configuration and changes the explanatory boundary for the loopback fixture.
3. **Workspace/session separation:** demo buckets are keyed by workspace; AppContent remounts by workspace ID; transient inputs are component state. Global account/settings buckets remain shared deliberately. Reset removes only the two named demo storage keys, preserving unrelated Agent pending keys. Storage write failures are caught and surfaced as an in-memory warning rather than persistence proof.
4. **Business acceptance history:** receipt correction appends, submission/archive deep-copy their exact inputs, return/resubmit and withdraw retain earlier submission records, terminal cases reject reopening, dispositions are separate from original accepted/archive content, and receipt changes block approval until a new submission. These are local demonstration invariants, not server IAM/DB concurrency evidence. Formal Case/Receipt APIs remain future design.
5. **Core history / trace:** queries use the existing credential-bearing server transport, no-store and safe upstream errors. Exact requested objects are validated against configured workspace/subject; missing or foreign history remains UNKNOWN. Certification asOf/revision anchors survive pagination. Trace uses allowlisted display fields, immutable decision identity and separate frozen snapshot membership. No mutation controls are added to `/core/*` history pages.
6. **Read-only defaults / hidden actions:** inspected existing server-action feature predicates for review, release, DatasetVersion/quality/compliance, certification/ingestion and delivery; they still require explicit true flags and remain outside demo handlers. Inspected related Go delivery-history and trace HTTP transports (GET registrations plus trusted principal/workspace checks before repository reads), config's default `AGENT_TASKS_WRITE_ENABLED=false`, and API registration seam. Full Agent application/worker and identity authority internals remain assigned to other reviewers.
7. **Preview/fixture safety:** leader preview is loopback-only, pins its upstream to 4493, clears inherited business/identity/provider environment names, sets UI/Core business flags false, refuses occupied ports and uses a recorded process identity before stop. Leader fixture rejects every non-GET request and does not expose fixture controls. Older Q1 fixture also rejects business writes regardless of UI configuration. Neither fixture imports an external provider or starts Core/DB/worker. Playwright contract servers intentionally enable synthetic commands only against loopback 4400; this is not real write enablement.
8. **Test/evidence claims:** changed tests preserve precise IDs, pagination boundaries, history content, negative authorization and no-business-write assertions. Leader replay creates a new output directory, refuses existing output, captures source identity before the run and explicitly states it is not runtime build attestation. Both reviewed evidence bundles' screenshot manifests match their PNGs; original/intermediate packs remain historical. Stored PASS records were read as previous evidence, not re-executed or upgraded to this HEAD's runtime/CI proof.
9. **Reuse / dependencies:** implementation reuses Next/React, Phosphor, existing Core queries/transports and in-memory demo state; no parallel queue/workflow engine. Lockfile v3 parses; all 8 dependency and 7 development-dependency root requirements match package.json, with Next/eslint-config-next 15.5.28 and PostCSS 8.5.29 as declared. This was metadata consistency checking, not installation or a dependency vulnerability audit.
10. **Styles/assets:** CSS was examined for scope/external-load and layout seams; the two large demo stylesheets use the governance namespace and contain no url()/@import source. `integration.css` additionally defines the standalone loading class. PNGs were classified and SHA256 checked; only final repair-readonly frame was visually inspected. No complete pixel, responsive or accessibility PASS is claimed.

## Validation actually performed

- PASS: frozen Git HEAD/base and clean source baseline read; all 152 assigned content hashes match candidate manifest.
- PASS: `git diff --check <base> <head> -- apps/web tests/browser tests/leader-demo`.
- PASS: manifest/lockfile root dependency consistency; JSON evidence parse and reviewed-bundle PNG manifest digest comparison.
- PASS: static source/diff review and cross-contract checks described above.
- NOT_RUN: npm/Go/fixture/browser tests, compilation/build, network/live API probes, runtime/preview starts, database, Docker, IdP, real models/dispatch, resets and process stop/start. This review wrote only this report. Prior committed evidence is not current execution proof.
- Exact-head required CI, full server persistence/concurrency, IAM and Agent internal reviews must be combined by the coordinating reviewer; no claim here replaces them.

## Proposed validation after any narrow delta

- Recheck exact changed hashes and diff against this frozen baseline, then check the semantic-wrapper and superseded-history changes independently.
- Keep existing `test:demo`, history/Agent tests and browser-contracts/live-core CI gates; source-only review does not satisfy them.
- Do not start retained previews or run leader/Q1 scripts merely to close these P3 items without separate runtime authority.

## Full assigned file coverage

Each entry below was included in this review. SOURCE/TEST/DOC means assigned diff/source static examination; STYLE means structural/scope review; MANIFEST means configuration/lock/evidence metadata examination; BINARY means digest and relevance classification only. These codes are coverage statements, not execution PASS.

| File | Coverage | Candidate SHA256 |
|---|---|---|
| `apps/web/.eslintrc.json` | MANIFEST | `38dc07d6063b008965c2a85d1d9770c215918bf801a5684433184aed0929ce8b` |
| `apps/web/.gitignore` | MANIFEST | `8157ccca55add4ae77b803144a24c012d0e8b7f284e0c11c606740f4d3da281a` |
| `apps/web/app/core/archives/page.tsx` | SOURCE | `6c59229bd89daded03d4e0e78f16750d10dbbb2e6b1fa99d65e3a104b1e6ffa2` |
| `apps/web/app/core/assets/page.tsx` | SOURCE | `3717bff45e2643b505d7f60e131c2d31912b2c70868093d5947eae57ac4f4e4b` |
| `apps/web/app/core/certifications/page.tsx` | SOURCE | `72cb3d5bc28ca15937bda625e56260251e7373e830c83639f7afe533ceaf9198` |
| `apps/web/app/core/deliveries/page.tsx` | SOURCE | `7f95a71e90721d0c87aba7a652cc95de0968e16f8048782a3669a307cd09938b` |
| `apps/web/app/core/release-trace/page.tsx` | SOURCE | `a83644e9c79199b784247e71f3706925d688a80b9f7ff7ec20de918f3edb1ed7` |
| `apps/web/app/core/releases/page.tsx` | SOURCE | `a0f01d9e27b4a21881ae869b39e09719e22ba441d656c52f0cd41bba70cf4d3f` |
| `apps/web/app/core/workbench/page.tsx` | SOURCE | `be0dce8c3d9068b91d21bdd6b4078234ae82c64b49fee29351a2b3480f61f617` |
| `apps/web/app/demo/page.tsx` | SOURCE | `3ea2ee7ef51c86b784d2cbb8cc8e58f17609ce81d349077d87d08b768fb2fad4` |
| `apps/web/app/globals.css` | STYLE | `1759e4cb837f4bf37b2308e2c4999df97f3447f667a5921ee4a435cbbe2592d2` |
| `apps/web/app/layout.tsx` | SOURCE | `0f871213641738a45c8bc2d0aeb22bf472864646a148a1dd79048f450dc70e2f` |
| `apps/web/app/products/[id]/releases/[releaseId]/page.tsx` | SOURCE | `19063a01083bae649383768be9dcb7ba93d9ee042c84c92d2c4afb6109333824` |
| `apps/web/components/core-delivery-history.tsx` | SOURCE | `41f394566ce0746765ae15af511af63f04e191066cec92230cd8ef301c31820b` |
| `apps/web/components/core-history-records.tsx` | SOURCE | `26ecc78d7fb200b155d75fec4edb40aed33530cea66b75148f4fe120f7e4278e` |
| `apps/web/components/core-history-view.tsx` | SOURCE | `6eba73567261ff6ad79d00e5520bf719f214916034c352665ecf5cebe46d8989` |
| `apps/web/components/governance-demo/AdminPages.jsx` | SOURCE | `1dbf9974cdfa153db38afebb7ad50201f5ab17a62cdf3435ed0a4e6e9a6a0305` |
| `apps/web/components/governance-demo/AgentContextSidebar.jsx` | SOURCE | `37b426cc1cc2590e8557d050824a24d304008ed59fba2fd98961f622e533675e` |
| `apps/web/components/governance-demo/App.d.ts` | SOURCE | `37cee98e1ba3e07ab16d83e38dd4613133753fcfe2969de5477664237b7c07e2` |
| `apps/web/components/governance-demo/App.jsx` | SOURCE | `0f94adeff21d63ef80e1cabb5527e29388871d32842ab0f0e3768ef1a26fd30c` |
| `apps/web/components/governance-demo/BusinessAcceptancePages.jsx` | SOURCE | `e87b73918fed065d8d4d8e6394832f9c5e29a23cf15d7b058237f596dd521b7b` |
| `apps/web/components/governance-demo/DefinitionPages.jsx` | SOURCE | `16b31bccc456307237cd713c5f883e935482c7f0f709b2e5d87c15818c28e256` |
| `apps/web/components/governance-demo/DemoPages.jsx` | SOURCE | `f65e02c8865a88d3d0fc380421c467389db3e1c6d4dabd971353a8649cb5a66d` |
| `apps/web/components/governance-demo/DemoState.jsx` | SOURCE | `9f61fda09d7d69f31dcb7bc5c641ac494d69d62a094a3c21138cb8a5b1ac9508` |
| `apps/web/components/governance-demo/GovernanceWorkspace.jsx` | SOURCE | `d2c33b0ef76fa74a0c6f210a46342758eeef3c3172017a6cfe50abac37a115c3` |
| `apps/web/components/governance-demo/PipelineContext.jsx` | SOURCE | `21f224e164c5fbe4b8963f081cd49f07ed0aa2cc796a6a136e2f2f60d2307410` |
| `apps/web/components/governance-demo/PipelineWorkspace.jsx` | SOURCE | `bb0eceeb674be7b4aac676eb8ce89009b3164cf9020b2489e1f4868c22aad5cb` |
| `apps/web/components/governance-demo/README.md` | SOURCE | `053f157f069051009785ddb79fa225e3dc649447009d7a9c9e2da5fa6715fa82` |
| `apps/web/components/governance-demo/SectionPage.jsx` | SOURCE | `27bac790782cdc546175d064833ec747354684db8e033c0d3cf0b43c75efd0f5` |
| `apps/web/components/governance-demo/SupplementalWorkspace.jsx` | SOURCE | `5062f2144ccb80297985fae2ee0f4a916c3605f817b28487b8d4aa71084caadf` |
| `apps/web/components/governance-demo/WorkPages.jsx` | SOURCE | `6d4c871d1fa59290be7055ed14d1885b6bf29ac1db4da5c6bc20ef67363fec66` |
| `apps/web/components/governance-demo/WorkspaceContext.jsx` | SOURCE | `6271abf2fab2c5ab227f2745a772da73b91a0e10a57721085d7352b54019e263` |
| `apps/web/components/governance-demo/WorkspacePages.jsx` | SOURCE | `045384ec5b2e2f0996880eea2ca2821164bd5043797df0784585a9267d657ac4` |
| `apps/web/components/governance-demo/agent.css` | STYLE | `de6c9d47b1d9762997b074011ba8b965216ae45028af4325a7317fea21ed2b39` |
| `apps/web/components/governance-demo/business-acceptance-model.mjs` | SOURCE | `812c52f8f27776a342fb8d9e4a57e2b5d53436e79d62376687a0fb160ff4278b` |
| `apps/web/components/governance-demo/demo-context.mjs` | SOURCE | `2ee2b4008780f640e78be3d7d4b80e60afb76b34e5c229253a8d7df79c8a289a` |
| `apps/web/components/governance-demo/demo-navigation.mjs` | SOURCE | `683b9f04da5eca89d69ad8a3712697da5ca01f0135d5067d31a74e2d9951c01c` |
| `apps/web/components/governance-demo/demo-session.mjs` | SOURCE | `ef622f1405563bd0d5b89f1bbd87e7d50a101ec911f43e2493f0035c4b6f3875` |
| `apps/web/components/governance-demo/integration.css` | STYLE | `9ee44f258eccdc9ecec982885f95953c65b3cbe00e8aff059bb258d5a028b293` |
| `apps/web/components/governance-demo/lifecycle-model.mjs` | SOURCE | `a6fa553ac21dd009b092cc1a1745e70d0200acef53f57feae4baed49c6d3be11` |
| `apps/web/components/governance-demo/pipeline-model.mjs` | SOURCE | `c28084a18cf55b6087038d34c6235d9f5f2261eecb52943178a90b341f3aef5c` |
| `apps/web/components/governance-demo/styles.css` | STYLE | `141ab9a0ed2497c1337e0a58656b41e265de1f10dea701f48adfba9bc7cca901` |
| `apps/web/components/governance-demo/ui.jsx` | SOURCE | `5005e6a727a2613e0fa315fa34b852960067c9914eb5f7dc189a71a2e4ebffab` |
| `apps/web/components/governance-demo/workspace-model.mjs` | SOURCE | `3acc47d69da8d7357ccaf470aa58931af877a77ef90daa23e30794d4b148efb6` |
| `apps/web/components/governance-demo/workspace-seeds.mjs` | SOURCE | `501d18882ea7409149c38591f9feb6589e6640e89c74cd03e00a3972aecbde6e` |
| `apps/web/components/schema-exceptions.tsx` | SOURCE | `f4d93e8dbbecc8cdd41762c7533db04a9cc060460ce31b7f2dcac0fa7defd18b` |
| `apps/web/components/shell.tsx` | SOURCE | `e95bfe44958588e4b2a54aadaeb55d1912cb04b840869c5a8039b267e0dc2ea0` |
| `apps/web/components/ui.tsx` | SOURCE | `e1c3301a77f042d7c9e3a22a8efd17392f0369cb5ec5942589a9a247a212052a` |
| `apps/web/lib/core-history.ts` | SOURCE | `8a476e4ad20be25d21547ffbe653609c12bc22504bde5868d1b177f18306d1ed` |
| `apps/web/lib/delivery-history.ts` | SOURCE | `d011606b1be263e488d3acd6066a6c6ae2966e9742a9070fabbca2b44e0bf481` |
| `apps/web/lib/platform.ts` | SOURCE | `8ca8944b149505ad9c8dc8ad1e563d3139c8ecf35d5ca6c935ad0ac2ced29c64` |
| `apps/web/lib/quality-cost.ts` | SOURCE | `027aa8d294ca4284995cfda100b4a31f0e00a88a94a661162d36d1e5f2345200` |
| `apps/web/lib/release-trace.ts` | SOURCE | `58256ee9bf6bc80716fc1e39cd8e8aeb0b00a02c5dac7be4b87e39ce865184ac` |
| `apps/web/lib/traceability.ts` | SOURCE | `7a222c3a0ab6eb0561acc1bfa3b2b0286df725261fd15c1d65f19aee8b05def2` |
| `apps/web/package-lock.json` | MANIFEST | `2d4906e5b191e3d060a557d139c7daf9caea9c19026f2204921f9878a4bef94b` |
| `apps/web/package.json` | MANIFEST | `64c771c7872d7c5fcfdec4781ed1577fa72bfeeb38d9eca00f160e02d8b73e79` |
| `apps/web/public/design/brand.png` | BINARY | `a6432f8abe7ebd528a3619a23d429bed87c44ddc7c99ee7193ecf61960b095e9` |
| `apps/web/tests/core-history.test.mjs` | TEST | `3cdaec44596ac8d7e14c897fc1613c54cd56d733f9138100f79ec106d41ec795` |
| `apps/web/tests/delivery-history.test.mjs` | TEST | `72854bb81dad2add095684a6acc748a941994695bd459e7240761a677f9bfafc` |
| `apps/web/tests/governance-demo/business-acceptance.test.mjs` | TEST | `01173e2398dbeb3efa7b1963053a7393b1b211f5b3246dc8099de31284a777fd` |
| `apps/web/tests/governance-demo/lifecycle-model.test.mjs` | TEST | `6ffc17119adade9c30a4702de811a2024e1064e8d3586b03d619deea69f89b50` |
| `apps/web/tests/governance-demo/navigation.test.mjs` | TEST | `2784d8faedd9c95984a6f0e67cf4a80f3e7d46effd0695ad6403a3467513b6af` |
| `apps/web/tests/governance-demo/page-render.test.mjs` | TEST | `25061f8b811bfea7e0874d00f4d05ae427b167f219004ef823aa131def3c5f21` |
| `apps/web/tests/governance-demo/pipeline-model.test.mjs` | TEST | `846aaf476d524dd551e77880889d8b09379d9463a4624b625ceda7412620e24c` |
| `apps/web/tests/governance-demo/routing.test.mjs` | TEST | `5de9ceaaa7a727f8e9d7e614aa4ca0e2eeb4f09171b82fe098b3b0d8f7120501` |
| `apps/web/tests/governance-demo/session.test.mjs` | TEST | `5a1d4822e91ea5066be0f0d85414d0557e3d2bac70ffe4705c5ff749b3a5e611` |
| `apps/web/tests/governance-demo/workspace-model.test.mjs` | TEST | `2cbd3a8b9986f9f62f2411f7cb46821484c639c323af924f491c7c34db7fe8ef` |
| `docs/product/governance-business-acceptance-demo-20261024.md` | DOC | `fa13d7e6870e31cb8531441299de3bbdfdb7bccc4041a9340e83a6751507199a` |
| `docs/product/governance-business-backend-plan-20261024.md` | DOC | `5167b568c7d10fbe72fc4b3ca3170470caf5d8ab6971c8dbb142b2e2006c2b43` |
| `docs/product/governance-business-exceptions-20261024.md` | DOC | `f70807987b2123ffcbe306cb588896451751067a19c0859a35d8308eb177e141` |
| `docs/product/governance-business-review-20261024.md` | DOC | `353e4849aa7b3d9afce335595629044dfe9d0d5e48b1fd8cf493d2771bb5123b` |
| `docs/product/governance-modern-ui-acceptance.md` | DOC | `cfd0be540af4e80a79e4b0b2dd285799f6c4e1125657d9b64e69ec472e98b103` |
| `docs/product/governance-modern-ui-audit-20261023.md` | DOC | `a64e849fdfb390d6417361c7e52d973f3f5c1a6e065aea4fe9d0b05169e389fa` |
| `docs/product/leader-demo-20261025-reviewed-final/00-about-boundary.png` | BINARY | `c99f49e3e8df8b594f92c25bc2ef06464b2b320acf9703eb400912cdedf8698a` |
| `docs/product/leader-demo-20261025-reviewed-final/01-home.png` | BINARY | `148b5f806bab93909afccd5437d8a066230c9289a5c2625bcf4b5890cfad62c4` |
| `docs/product/leader-demo-20261025-reviewed-final/02-agent.png` | BINARY | `77423f1d854df70a725c10e8dc6e26af4db1293ac074fa3bf443af7ad2302dc1` |
| `docs/product/leader-demo-20261025-reviewed-final/03-quality-focus.png` | BINARY | `8c5016f9a0efe7604d753b66c2608fb91becbc1748ad1882169fcd2714e66433` |
| `docs/product/leader-demo-20261025-reviewed-final/03-quality-report.png` | BINARY | `3fc6023ba61df100bb64f4b881513e7598200d2a388b37754d44d4967043d8fb` |
| `docs/product/leader-demo-20261025-reviewed-final/04-agent-repair.png` | BINARY | `03735868a08d32187f1614af834a900446edeeb501da5a134176f73433c62ee7` |
| `docs/product/leader-demo-20261025-reviewed-final/05-repair-readonly.png` | BINARY + SINGLE_FRAME_VISUAL | `5702034b2cc8b4ffdd3e2a4fe792795c48a36b9856867aed87eafa173bb4df1c` |
| `docs/product/leader-demo-20261025-reviewed-final/06-unknown.png` | BINARY | `e996252d6bf3b0f0039c3b85f064e752e8232df11a53674d055a9472d3ee6749` |
| `docs/product/leader-demo-20261025-reviewed-final/07-receipt-required.png` | BINARY | `618f8968ab3b6b833bdfc1d11366777b01c3c8e95c8fa111ef61edf4b7a93cf0` |
| `docs/product/leader-demo-20261025-reviewed-final/08-receipt.png` | BINARY | `0da1878f3be6476526a2b55da8a0bac95b1679497755f11fdb35ccf45c6b026b` |
| `docs/product/leader-demo-20261025-reviewed-final/09-acceptance.png` | BINARY | `51e4df57f929525d71c5a85e787a638becd8cef5f701b2731f253bfcd6ad1052` |
| `docs/product/leader-demo-20261025-reviewed-final/10-archive.png` | BINARY | `bac5428a393488e0a713c4ad6891a8040809b422723fddbc3f6d98673171f5c5` |
| `docs/product/leader-demo-20261025-reviewed-final/11-invalidation.png` | BINARY | `aa2f02376eede0fde307b6f553841ae394d91c43c5cbca4329299f61e2416612` |
| `docs/product/leader-demo-20261025-reviewed-final/12-trace.png` | BINARY | `591b5323db3427911085a2d3c57e5f83960aa03fe38751d86027e9e511e54133` |
| `docs/product/leader-demo-20261025-reviewed-final/13-correction.png` | BINARY | `ae33e9e8d52a4350e4fcafb61c3045067a6b509e6ce5e1ab5d2fb366082f7641` |
| `docs/product/leader-demo-20261025-reviewed-final/14-storage-unavailable.png` | BINARY | `a3b9793b1c58783a00890937fcb9f516df096befc4aad0b5f06aa661d5f54d12` |
| `docs/product/leader-demo-20261025-reviewed-final/acceptance.json` | MANIFEST | `e89c1ec3c2663cc818b9cfbbc2cb392293857806b07ea95c9aca0d8c6a2bda00` |
| `docs/product/leader-demo-20261025-reviewed/00-about-boundary.png` | BINARY | `c99f49e3e8df8b594f92c25bc2ef06464b2b320acf9703eb400912cdedf8698a` |
| `docs/product/leader-demo-20261025-reviewed/01-home.png` | BINARY | `148b5f806bab93909afccd5437d8a066230c9289a5c2625bcf4b5890cfad62c4` |
| `docs/product/leader-demo-20261025-reviewed/02-agent.png` | BINARY | `77423f1d854df70a725c10e8dc6e26af4db1293ac074fa3bf443af7ad2302dc1` |
| `docs/product/leader-demo-20261025-reviewed/03-quality-focus.png` | BINARY | `8c5016f9a0efe7604d753b66c2608fb91becbc1748ad1882169fcd2714e66433` |
| `docs/product/leader-demo-20261025-reviewed/03-quality-report.png` | BINARY | `3fc6023ba61df100bb64f4b881513e7598200d2a388b37754d44d4967043d8fb` |
| `docs/product/leader-demo-20261025-reviewed/04-agent-repair.png` | BINARY | `03735868a08d32187f1614af834a900446edeeb501da5a134176f73433c62ee7` |
| `docs/product/leader-demo-20261025-reviewed/05-repair-readonly.png` | BINARY | `5702034b2cc8b4ffdd3e2a4fe792795c48a36b9856867aed87eafa173bb4df1c` |
| `docs/product/leader-demo-20261025-reviewed/06-unknown.png` | BINARY | `e996252d6bf3b0f0039c3b85f064e752e8232df11a53674d055a9472d3ee6749` |
| `docs/product/leader-demo-20261025-reviewed/07-receipt-required.png` | BINARY | `a6b5a85f11ae8bd474f523c71afe15549b3c5501962ab1f681f723809d6c8d4e` |
| `docs/product/leader-demo-20261025-reviewed/08-receipt.png` | BINARY | `7e15fef40fc39a0797e2a7466f2c6a4d6381105301adb6360733eaf89eb85367` |
| `docs/product/leader-demo-20261025-reviewed/09-acceptance.png` | BINARY | `460f27befb18ffd5e0f93dd2e2d95d48d9fd949aa01c99c817465b277e409831` |
| `docs/product/leader-demo-20261025-reviewed/10-archive.png` | BINARY | `cfb87831550ba6c4bb9cf585b32257a49bac798d6b77d85a466b26a9b4bca483` |
| `docs/product/leader-demo-20261025-reviewed/11-invalidation.png` | BINARY | `c0949899551b5bc3721e6b367d890c2b0561cacf795f49cf74e5e58fefe71b0a` |
| `docs/product/leader-demo-20261025-reviewed/12-trace.png` | BINARY | `ca20dc8cbd28266b90989a7849e04b629a75bfb80ba75af9166cd0cc2f52e65d` |
| `docs/product/leader-demo-20261025-reviewed/13-correction.png` | BINARY | `133c973076ad3dee3122713cd29cbd15216dd0d15ac92eedca9f48eac9a4c21c` |
| `docs/product/leader-demo-20261025-reviewed/14-storage-unavailable.png` | BINARY | `d88b33d3c67c9522080832a8f8eb3774633366b69bfde21539f6095e37d1c86e` |
| `docs/product/leader-demo-20261025-reviewed/acceptance.json` | MANIFEST | `e7090d099deac7c7ab394fdc4daa0117d0fae05c16c7be02604e4d25269938b4` |
| `docs/product/leader-demo-20261025/01-home.png` | BINARY | `148b5f806bab93909afccd5437d8a066230c9289a5c2625bcf4b5890cfad62c4` |
| `docs/product/leader-demo-20261025/02-agent.png` | BINARY | `b4ca5a3122ecb7a202e94c36bcf527dadf986eb1f7dfe98fc752a028ac2010d7` |
| `docs/product/leader-demo-20261025/03-quality-focus.png` | BINARY | `e7aebbe67b03ea41698d7daa3b8ce8651f5e205f12efb25aab4afc379d192a3e` |
| `docs/product/leader-demo-20261025/03-quality-report.png` | BINARY | `d98d8bddad92c973999c2e8416ea6dd0782dc88a30d823cfaff7c8739e85e68a` |
| `docs/product/leader-demo-20261025/04-agent-repair.png` | BINARY | `03735868a08d32187f1614af834a900446edeeb501da5a134176f73433c62ee7` |
| `docs/product/leader-demo-20261025/05-repair-readonly.png` | BINARY | `5702034b2cc8b4ffdd3e2a4fe792795c48a36b9856867aed87eafa173bb4df1c` |
| `docs/product/leader-demo-20261025/06-unknown.png` | BINARY | `28e86319c84c958458cf29b8e24f4685bb71080f396000acae776fffc65fd55c` |
| `docs/product/leader-demo-20261025/07-receipt-required.png` | BINARY | `a6b5a85f11ae8bd474f523c71afe15549b3c5501962ab1f681f723809d6c8d4e` |
| `docs/product/leader-demo-20261025/08-receipt.png` | BINARY | `3660dfd86795eb3b892e0270b70ccdfdafc615e0043d4bb485012bac38a4c619` |
| `docs/product/leader-demo-20261025/09-acceptance.png` | BINARY | `813ff373ad5ce4598f48986e7480bf9cb506c2be5f8770dcdef461ec6e05ad6b` |
| `docs/product/leader-demo-20261025/10-archive.png` | BINARY | `e63a00632e62960702ee8bb68dc47519d06b349baee0ecf9a3c80f917a1bf7ef` |
| `docs/product/leader-demo-20261025/11-invalidation.png` | BINARY | `6553c87a96f7c82d3921cbbc4c94f65e9f72dc8010d68732d076b9860f937267` |
| `docs/product/leader-demo-20261025/12-trace.png` | BINARY | `cf020df4b7fcbd4b0cc6747488c5cf0fd7f7d667559f20c241065dd04d0ed7fa` |
| `docs/product/leader-demo-20261025/13-correction.png` | BINARY | `3935d84c72746ad5426500361eb52f9056dbae6cb9a1f3fbc8f8d137c3fa74c4` |
| `docs/product/leader-demo-20261025/acceptance.json` | MANIFEST | `de8757697b431b4729b2b14053f7361a71e475a26ced34e3a94d36d2b597184d` |
| `docs/product/leader-demo-capabilities-20261025.md` | DOC | `283845cfa498b7d444878873242723675f96e98299e76f48cce9ee7c80c2ca71` |
| `docs/product/leader-demo-closeout-20261025.md` | DOC | `94b4e894db1147d91e49a0997a7889899ee21b5ed34432c85c3ab183c3b02c2e` |
| `docs/product/leader-demo-review-20261025.md` | DOC | `713258a9473e95e11bf9c997c3ea521341faded10cf7d3136b75dc7500d7ec5d` |
| `docs/product/leader-demo-review-before-20261025/about-message.png` | BINARY | `5f15ba699819631f359129fa2d6adc3834831c1e80dbc910bd0de97bfa520335` |
| `docs/product/leader-demo-review-before-20261025/missing-report.png` | BINARY | `08634022d890477ad96aea0abd855913d731d4dcad5a72e4f0d3c096b3d45d74` |
| `docs/product/leader-demo-review-before-20261025/storage-message.png` | BINARY | `b2139cf966e55623f176159dc9bbc7124fb0b24e7912a90c47e9a4ebc73adbfd` |
| `docs/product/leader-demo-talk-track-20261025.md` | DOC | `a4890b1491d59fe097aebc7dbb8c50a436bd05122cbc2f7addd35441431e4331` |
| `tests/browser/acceptance-context.spec.mjs` | TEST | `c6357494ead2060455c20f1f14b5a41fee2cec681c5ef03abb6b54f54ba02712` |
| `tests/browser/actions.spec.mjs` | TEST | `e5841b84b645a4188cbd13f3a050d456b75ebb3fb8328d7c854151c2d652ea98` |
| `tests/browser/agent-q1-preview-fixture.mjs` | TEST | `63e0eb63835649da7a72e76495f983271d09ec7e8a8df2a01e1c69d25da63b3e` |
| `tests/browser/business-acceptance-exceptions.spec.mjs` | TEST | `f188c532a2bcd9902fb891309dfbc17ab0de5106effbf0fca652231474812cc9` |
| `tests/browser/business-acceptance.spec.mjs` | TEST | `74942bc16fa91550da558c035659465e7edd1d059ebbc26efe74e09e27d601a9` |
| `tests/browser/fixture-server.mjs` | TEST | `54e30612cef84c65dd4de4c64a7378bf94dcbe3a47d6335743a716a610e74aec` |
| `tests/browser/fixture-server.test.mjs` | TEST | `ae42339618170a90bb2617827a3478c5d0d6a5617aba9e645f9368d1ca5edb44` |
| `tests/browser/modern-core.spec.mjs` | TEST | `28edcfa47cd8545ad251015d8152c9a7343897f7a6451f5c064419654575741b` |
| `tests/browser/package.json` | MANIFEST | `71519aed74b8c26412d7be28b28fe0a83c108cb7930f6c762308c418d57aec17` |
| `tests/browser/playwright.config.mjs` | TEST | `e28cf965bb5136104571694d945bf1034357897f3e3b245bb9238ac83c0ecc33` |
| `tests/browser/quality-attention.spec.mjs` | TEST | `c1b59b1954b90e99779d755d7fb41261a78665d43059de73401db93319533a6a` |
| `tests/browser/schema-attempts.spec.mjs` | TEST | `d61b536ab2eed7b70e609cfdddb28a68f30eeced11d0043f5767220b1305286a` |
| `tests/browser/schema-attention.spec.mjs` | TEST | `3a9c3a443c0630d57c770030ee444464bd311b3e6aa39480ad878167eb69f5e9` |
| `tests/browser/schema-exceptions.spec.mjs` | TEST | `2e4fbad7a82cc1cab4b61c8248a28b8ef42a4867d581e65146a94a03cabd3a0c` |
| `tests/browser/schema-requests.spec.mjs` | TEST | `85904c5701d0b17fe451bfd18dd415f33b1832c9f91238fad95ab0c3a1cece6c` |
| `tests/browser/start-agent-q1-preview.ps1` | TEST | `d51b5a694797403c23ac54a5cf24a974ce862a562c3024364f3ccf969a788b42` |
| `tests/browser/workbench-isolation.spec.mjs` | TEST | `6d648e11c9357f2efba93bf1319dda5c21b8348197879a4ed3b00a7910a73497` |
| `tests/leader-demo/acceptance.mjs` | TEST | `baad81b2af41d27f779f38d513eb98b511476fe1d420474ed6997e62abea2af9` |
| `tests/leader-demo/preview.ps1` | TEST | `36831e1a3fb4f7d1b90cd81b1af3961e77d6f97af5b8422a9a1248dd099ba332` |
| `tests/leader-demo/readonly-fixture.mjs` | TEST | `6d7cc098b705eb4c331294a582547f9df6a716470427c381a68d53681ca122f4` |
| `tests/leader-demo/readonly-fixture.test.mjs` | TEST | `b925c76fca2f2baff11821b51ef06a4aa8c1f91cbadcb6fd822aedcde1b615a2` |
| `tests/leader-demo/replay-output.mjs` | TEST | `3c3be4fb0ef8aeeeed96fbc44d12363fa4c0df065a5efeee8234700f2172d284` |
| `tests/leader-demo/replay-output.test.mjs` | TEST | `6489d7be2cac25813e1a5e6e9cb21e7f18dde51e74371ec64beaba33bf6aff65` |

