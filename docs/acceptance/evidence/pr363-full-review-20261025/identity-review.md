# Independent identity / acceptance / history review

- Review date: 2026-10-25
- PR: #363
- Frozen head: `8495de4e1bb3100e9cf4e73760caaddc02264153`
- Base: `e0e4f77a576ea4572cf16b3a3475461127f44acc`
- Workspace: `C:/Users/Henry/.codex/worktrees/aq1`
- Outcome: **0 confirmed merge blockers in this assigned scope**. This is a static technical review, not runtime acceptance or approval to merge/deploy/enable writes.

## Findings

No concrete introduced authorization bypass, credential disclosure, historical-fact mutation, unsafe write admission, or required-path failure was found in the 113 assigned changes. No source fixes are requested from this reviewer.

## Technical checks

1. **OIDC and RP trust.** Go uses the ZITADEL resource-server SDK; Web uses openid-client with Code/PKCE, nonce/state and non-repudiation checks. The resolver checks explicit issuer/audience/client/scope and bearer semantics; provider responses are bounded, TLS-verified and redirect-denied. The BFF consumes browser-cookie-keyed handshake state atomically, encrypts RP state, exposes no access token, refreshes authority per request and pins canonical principal/revision. Profile/configuration changes and authority revision changes deny old sessions. No shared-token fallback is reachable in the acceptance identity profile.
2. **Local authority and responsibility.** `ValidateInTx` rejects stale isolation, takes the shared IAM fence on the caller transaction and rechecks current actor/subject/grant, capability, revision and database time. Acceptance readers then take the workspace binding fence and refresh authority after the wait. Identity and acceptance mutation triggers participate in the matching namespace and order. Full responsibility candidates, representation dependencies, actor/subject separation, dispositions and exact confirmer identity are evaluated before caller visibility. Paging never determines eligibility. `CheckResponsibilityInTx` is explicitly a narrow local role Port, not an implemented business Command or complete source/delivery admission gate.
3. **Immutable facts and SQL.** Migrations 74/75/76 preserve distinct business binding facts, mutable revisioned identity projections and ephemeral sessions. Binding headers/dispositions cannot be updated/deleted; target, workspace, Evidence, receiving party and confirmer references have guards; DatasetVersion references use lifecycle admission. Destructive downs fail closed or serialize session drain before dropping ephemeral state. No mutable member/finalize aggregate is introduced by these bindings.
4. **Evidence scope and projection.** The basis reader authorizes and establishes the exact target/caller responsibility before inspecting Evidence. `AcceptanceMaterialReferences` independently joins delivery, DatasetVersion and dataset workspace and requires both explicit relation edges plus matching Evidence workspace. Count and page use the same SQL scope. Inspection returns a bounded integrity reference, not title/metadata/storage URI/source credentials. Envelope integrity, content identity, recipient evidence and source admission remain distinct; materials always remain UNATTESTED/BLOCKED.
5. **Trace and history.** Main API wiring attaches the existing human-decision resolver to the contract, certification, release-history and trace handlers. Existing Web certification/release clients now use the authenticated read transport; acceptance capabilities do not broaden Agent or unrelated Core paths. Scoped trace repositories use one repeatable-read transaction, verify returned ownership and reject corrupt cross-workspace members; HTTP uses an allowlisted projection. Exact history selection has no latest fallback. The new acceptance profile remains GET-only except the private principal confirmation and technical RP session lifecycle.
6. **Documentation authority.** Current binding/basis-read contracts and 1A implementation records describe reachable behavior. Case/Draft/Receipt/Submission/Decision/Archive, trusted enterprise sources and source admission/disposition writes are explicitly future design, unconfigured or disabled. The T/R/G material/source and enablement documents do not authorize runtime work or claim that synthetic identities and Evidence prove business receipt. No conflicting authoritative safety rule requiring a current source-code change was identified.
7. **Harness review.** Reviewed the isolation acknowledgements, fixed independent database/ports, container identity check, generated private profile handling, minimal DB roles, process ownership checks, fixed TLS front door and Browser test boundaries. These were read only; no harness was launched and no profile/credential file was opened.

## Coverage and evidence limits

- **113/113 assigned files reviewed**, including every changed hunk and relevant unchanged dependencies/callers. Existing large unchanged handlers/integration suites were read at the changed hunk and relevant call boundaries; new files were read in full.
- SHA256 recheck: **113/113 matched `candidate.json`** after the review reads. Checked worktree HEAD also equals the frozen head.
- Categories: 50 application/adapter/UI source files; 28 test/fixture files; 15 harness/preparation/start files; 14 documentation/contract files; 6 SQL up/down files.
- Additional dependencies inspected: `apps/platform/cmd/api/main.go`, Evidence acceptance inspection, trace collection/dependencies, Web authenticated read transport, release-trace DTO/validators and affected shared platform callers.
- Uncovered assigned changes: **none**.
- Tests, PostgreSQL, Docker, browser, retained preview, IdP, real sources, models, execution, external providers and current CI: **NOT_RUN by this reviewer**. Historical PASS claims were treated as documents, not independently reproduced evidence. This report cannot establish runtime correctness, enterprise-source trust or deployment readiness.

## Nonblockers / follow-ups

No additional change request is proposed. Existing finite bounds (100 target candidates; bounded Evidence inspection and trace member count) fail closed and are part of the present limited read profile. Enterprise directories, source/admission models, remote revocation acceptance windows and business write Commands remain the documented future scope; their absence is not a regression introduced in the implemented read slice. Disabled backchannel/provider-session code is not advertised as enabled by the acceptance profile.

## Validation proposals for the parent

Use only a newly isolated authorized test environment, with no retained runtime reuse:

- Run the existing acceptance Domain/readmodel/HTTP and principal OIDC/bindings suites. This checks strict envelope parsing, current mapping, wrong client/audience/scope, exact target selection, type-plus-ID history identity, blockers and source/receipt non-promotion.
- Run real PostgreSQL authority, responsibility and business-basis suites with their required isolated DSNs. Require actual pg_locks waits, both revocation orderings, expiry after waits, stale-isolation rejection, immutable-header/disposition checks and exact Evidence dual-relation pagination.
- Run Web IAM plus acceptance-identity/basis/context suites; record which tests use real PostgreSQL versus memory and which are skipped. In particular, preserve distinct claims for RP TLS protocol tests and database drain/concurrency tests.
- Run the existing isolated identity browser suite and privilege proof only if the parent has runtime authority. Verify separate canonical users/spaces, rejected bypass routes, revoked-session non-revival, unknown material details, retained context on navigation, no business writes and unchanged business fingerprints. Do not present synthetic protocol/flow proof as enterprise receipt acceptance.

## Assigned-file audit trail

Every entry below is REVIEWED_STATIC, with runtime validation NOT_RUN by this reviewer.
- `apps/platform/internal/certification/transport/http/handler.go`
- `apps/platform/internal/certification/transport/http/history_routes_test.go`
- `apps/platform/internal/contract/domain/acceptance.go`
- `apps/platform/internal/contract/domain/acceptance_bindings.go`
- `apps/platform/internal/contract/domain/acceptance_bindings_test.go`
- `apps/platform/internal/contract/domain/acceptance_test.go`
- `apps/platform/internal/contract/infrastructure/admission.go`
- `apps/platform/internal/contract/infrastructure/basis.go`
- `apps/platform/internal/contract/infrastructure/basis_integration_test.go`
- `apps/platform/internal/contract/infrastructure/binding_guards_integration_test.go`
- `apps/platform/internal/contract/infrastructure/bindings_integration_test.go`
- `apps/platform/internal/contract/infrastructure/identity_targets.go`
- `apps/platform/internal/contract/infrastructure/readonly.go`
- `apps/platform/internal/contract/infrastructure/readonly_integration_test.go`
- `apps/platform/internal/contract/infrastructure/responsibility.go`
- `apps/platform/internal/contract/infrastructure/responsibility_integration_test.go`
- `apps/platform/internal/contract/readmodel/admission.go`
- `apps/platform/internal/contract/readmodel/basis.go`
- `apps/platform/internal/contract/readmodel/binding_history_test.go`
- `apps/platform/internal/contract/readmodel/query.go`
- `apps/platform/internal/contract/transport/http/basis_test.go`
- `apps/platform/internal/contract/transport/http/handler.go`
- `apps/platform/internal/contract/transport/http/readonly.go`
- `apps/platform/internal/contract/transport/http/readonly_test.go`
- `apps/platform/internal/platform/principal/bindings/acceptance_http.go`
- `apps/platform/internal/platform/principal/bindings/acceptance_http_test.go`
- `apps/platform/internal/platform/principal/bindings/authority.go`
- `apps/platform/internal/platform/principal/bindings/authority_integration_test.go`
- `apps/platform/internal/platform/principal/bindings/store.go`
- `apps/platform/internal/platform/principal/oidc/client.go`
- `apps/platform/internal/platform/principal/oidc/client_test.go`
- `apps/platform/internal/platform/principal/oidc/resolver.go`
- `apps/platform/internal/platform/principal/oidc/resolver_test.go`
- `apps/platform/internal/platform/principal/principal.go`
- `apps/platform/internal/readmodel/delivery_history.go`
- `apps/platform/internal/readmodel/transport/http/delivery_history.go`
- `apps/platform/internal/readmodel/transport/http/delivery_history_test.go`
- `apps/platform/internal/readmodel/transport/http/handler.go`
- `apps/platform/internal/readmodel/transport/http/history_routes_test.go`
- `apps/platform/internal/traceability/repository.go`
- `apps/platform/internal/traceability/repository_integration_test.go`
- `apps/platform/internal/traceability/repository_test.go`
- `apps/platform/internal/traceability/scoped.go`
- `apps/platform/internal/traceability/transport/http/handler.go`
- `apps/platform/internal/traceability/transport/http/handler_integration_test.go`
- `apps/platform/internal/traceability/transport/http/handler_security_test.go`
- `apps/platform/tests/acceptancebasisprepare/main.go`
- `apps/platform/tests/acceptancebindingpreview/main.go`
- `apps/platform/tests/acceptancefixture/basis.go`
- `apps/platform/tests/acceptanceidentity/main.go`
- `apps/platform/tests/acceptanceidentity/main_test.go`
- `apps/platform/tests/acceptanceidentityprepare/main.go`
- `apps/web/app/auth/callback/route.ts`
- `apps/web/app/auth/logged-out/route.ts`
- `apps/web/app/auth/login/route.ts`
- `apps/web/app/auth/logout/route.ts`
- `apps/web/app/core/acceptance-basis/page.tsx`
- `apps/web/app/core/acceptance-context/page.tsx`
- `apps/web/app/identity.css`
- `apps/web/app/identity/page.tsx`
- `apps/web/components/acceptance-admission-panel.tsx`
- `apps/web/components/acceptance-identity-shell.tsx`
- `apps/web/components/acceptance-targets.tsx`
- `apps/web/components/core-acceptance-basis.tsx`
- `apps/web/components/core-acceptance-context.tsx`
- `apps/web/lib/acceptance-basis.ts`
- `apps/web/lib/acceptance-context.ts`
- `apps/web/lib/acceptance-identity.ts`
- `apps/web/lib/iam-configuration.ts`
- `apps/web/lib/iam-core.ts`
- `apps/web/lib/iam-logout.ts`
- `apps/web/lib/iam-mode.ts`
- `apps/web/lib/iam-response.ts`
- `apps/web/lib/iam-server.ts`
- `apps/web/lib/iam-store.ts`
- `apps/web/lib/iam-workspace.ts`
- `apps/web/middleware.ts`
- `apps/web/tests/acceptance-basis.test.mjs`
- `apps/web/tests/acceptance-context.test.mjs`
- `apps/web/tests/acceptance-identity.test.mjs`
- `apps/web/tests/fixtures/acceptance-bindings.mjs`
- `apps/web/tests/helpers/oidc-fixture.mjs`
- `apps/web/tests/iam.test.mjs`
- `apps/web/tests/run-iam-tests.mjs`
- `docs/architecture/acceptance-binding-read-contract-v1.md`
- `docs/architecture/acceptance-business-basis-read-v1.md`
- `docs/architecture/business-acceptance-backend-contract-v1.md`
- `docs/architecture/business-acceptance-draft-receipt-admission-v1.md`
- `docs/architecture/business-receipt-trusted-basis-v1.md`
- `docs/contracts/oidc-introspection-v1.json`
- `docs/product/acceptance-business-basis-read-implementation.md`
- `docs/product/acceptance-identity-1a-implementation.md`
- `docs/product/business-receipt-design-review-20261024.md`
- `docs/product/business-receipt-exception-acceptance-matrix-20261024.md`
- `docs/product/business-receipt-page-flow-20261024.md`
- `docs/product/business-receipt-source-paths-audit-20261025.md`
- `docs/product/business-receipt-source-paths-review-20261024.md`
- `docs/product/business-receipt-write-enablement-checklist-20261024.md`
- `migrations/000074_acceptance_bindings.down.sql`
- `migrations/000074_acceptance_bindings.up.sql`
- `migrations/000075_acceptance_identity_authority.down.sql`
- `migrations/000075_acceptance_identity_authority.up.sql`
- `migrations/000076_acceptance_browser_session.down.sql`
- `migrations/000076_acceptance_browser_session.up.sql`
- `tests/acceptance-identity/bootstrap.py`
- `tests/acceptance-identity/browser.mjs`
- `tests/acceptance-identity/configure-idp.py`
- `tests/acceptance-identity/configure-runtime.py`
- `tests/acceptance-identity/https-web.mjs`
- `tests/acceptance-identity/privileges.mjs`
- `tests/acceptance-identity/provision.py`
- `tests/acceptance-identity/start-api.py`
- `tests/acceptance-identity/start-web.py`

