# Governance demo integration

Copied from the user-approved governance-prototype/src on 2026-10-23. Source:
C:/Users/Henry/.codex/visualizations/2026/10/22/01a163ef-ed6d-7d53-bb1d-9d059057244c/governance-prototype

App owns the shared Next shell. Full business demo flows reuse the prototype's React contexts and pure presentation models. CSS is prefixed with .governance-demo and external Google Font loading was removed. Legacy AgentWorkspace/AgentTaskDetail/agent-model were deliberately not retained: the original API-backed Next Agent controller is the sole Agent command path.

Adaptations: Next navigation, truthful Core/demo boundaries, namespaced session-only synthetic recovery/reset, global administration state, and remaining source/batch/entity-review demo screens. JS sources remain JS and are excluded from the TypeScript-specific ESLint rules; pure lifecycle/workspace/session regressions run via npm run test:demo as part of build. App.d.ts is the typed shell boundary. No API or model client is imported by this directory.

The original prototype directory is unchanged. Future frontend work should edit this integrated directory, not create a third copy. See docs/product/governance-modern-ui-acceptance.md for route and acceptance boundaries.

2026-10-27 frontend-only interaction preview: `GOVERNANCE_SETTINGS_INTERACTION_DEMO=true` is a separate, default-off host mode. It retains local `#settings`/`#admin-*` navigation and existing AdminPages/WorkspacePages, shows a single local-data boundary notice, blocks connected routes and all non-read HTTP methods, and keeps the default readonly preview unchanged. The public brand is 创平数据治理平台. Build artifacts are Git archives, not another maintained copy; uncommitted backend administration WIP is excluded. See `docs/product/settings-interaction-preview-20261027.md`.

Core readonly integration: `/core/assets` renders the shared server-side `core-quality-view.tsx` through the same modern shell. Its authenticated workspace and UUID facts never enter demo contexts. Header entry distinguishes the connected surface from local demos; no demo reset is exposed in connected mode. See docs/product/governance-backend-integration-map.md.

`/core/certifications` and `/core/releases` extend this connected surface with authenticated history queries. Certification queries require an explicit version and preserve asOf/revision across pages. Release lists are workspace-wide and do not infer dataset membership. Both paths expose no mutation controls.


Business receipt/acceptance/archive routes (`#business-receipts`, `#business-acceptances`, `#business-archives`) use a separate workspace-scoped DemoProvider key. They never call Core write APIs. Receipt, frozen submission, review decision and archive are separate synthetic facts; return/resubmit and invalidation retain prior facts. The existing Core `/core/archives` is labeled 证据档案 and is not business acceptance. See `docs/product/governance-business-acceptance-demo-20261024.md`.
