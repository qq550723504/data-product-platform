/** Test-only HTTP fixture. Never imported by the application or exposed beyond loopback. */
import { createServer } from "node:http";
import { pathToFileURL } from "node:url";
export const ids = {
  workspace: "11111111-1111-4111-8111-111111111111",
  actor: "22222222-2222-4222-8222-222222222222",
  product: "33333333-3333-4333-8333-333333333333",
  release: "44444444-4444-4444-8444-444444444444",
  version: "55555555-5555-4555-8555-555555555555",
  job: "66666666-6666-4666-8666-666666666666",
  candidate: "77777777-7777-4777-8777-777777777777",
  entity: "88888888-8888-4888-8888-888888888888",
  decision: "99999999-9999-4999-8999-999999999999",
};
export const fixtureToken = "local-browser-test-only";
const stamp = "2026-09-17T00:00:00Z";
const scenarios = new Set(["ready", "empty-checks", "missing-evidence", "null-checks", "blocker", "future-gate", "failed-rights", "review-conflict", "publish-conflict", "paginated-resources"]);
const pass = { production: "PASS", dataset: "PASS", rights: "PASS", quality: "PASS", compliance: "PASS", contract: "PASS", evidence: "PASS", delivery: "PASS" };
function fixtureResources(count) {
  return Array.from({ length: count }, (_, index) => {
    const n = index + 1;
    return {
      id: `${String(n).padStart(8, "0")}-0000-4000-8000-${String(n).padStart(12, "0")}`,
      workspaceId: ids.workspace, code: `FIXTURE_RES_${n}`, name: `资源 ${n}`,
      description: "Synthetic fixture resource", domainCode: "TEST", resourceType: "TABLE",
      sensitivityLevel: "INTERNAL", rightsStatus: "APPROVED", qualityStatus: "PASS",
      lifecycleStatus: "ACTIVE", revision: 1, createdAt: stamp, updatedAt: stamp,
    };
  });
}
function readiness(scenario) {
  const result = { releaseId: ids.release, overall: "READY", checks: { ...pass }, blockers: [], details: {} };
  if (scenario === "empty-checks") result.checks = {};
  if (scenario === "null-checks") result.checks = null;
  if (scenario === "missing-evidence") delete result.checks.evidence;
  if (scenario === "blocker") result.blockers = ["RIGHTS_REVOKED"];
  if (scenario === "future-gate") result.checks.futureGate = "FAIL";
  if (scenario === "failed-rights") result.checks.rights = "FAIL";
  return result;
}
async function bodyOf(req) {
  let text = "";
  for await (const chunk of req) {
    text += chunk;
    if (text.length > 8192) throw new Error("fixture body too large");
  }
  return text ? JSON.parse(text) : {};
}

export function createFixtureServer() {
  let state;
  const reset = (scenario = "ready") => {
    state = { scenario, candidateStatus: "PENDING", releaseStatus: "READY", requests: [] };
  };
  reset();
  return createServer(async (req, res) => {
    const url = new URL(req.url, "http://127.0.0.1");
    const send = (status, value) => {
      res.writeHead(status, { "Content-Type": "application/json", "Cache-Control": "no-store" });
      res.end(JSON.stringify(value));
    };
    try {
      if (url.pathname === "/__health" && req.method === "GET") return send(200, { fixture: true });
      if (url.pathname.startsWith("/__control/")) {
        if (req.headers["x-fixture-token"] !== fixtureToken) return send(403, { error: "fixture control token required" });
        if (url.pathname === "/__control/state" && req.method === "GET") return send(200, state);
        if (req.method === "POST" && ["/__control/reset", "/__control/scenario"].includes(url.pathname)) {
          const { scenario = "ready" } = await bodyOf(req);
          if (!scenarios.has(scenario)) return send(400, { error: "unknown fixture scenario" });
          if (url.pathname.endsWith("/reset")) reset(scenario); else state.scenario = scenario;
          return send(200, { scenario });
        }
        return send(404, { error: "unknown control route" });
      }
      const body = req.method === "POST" ? await bodyOf(req) : undefined;
      state.requests.push({ method: req.method, path: url.pathname, body, actor: req.headers["x-actor-id"], idempotencyKey: req.headers["idempotency-key"] });
      const workspace = `/api/v1/workspaces/${ids.workspace}`;
      const releasePath = `/api/v1/product-releases/${ids.release}`;
      const product = {
        id: ids.product, workspaceId: ids.workspace, code: "BROWSER_FIXTURE", name: "浏览器验收产品",
        description: "Synthetic fixture — not production data", domainCode: "TEST", lifecycleStatus: "ACTIVE", healthStatus: "HEALTHY",
        currentVersionId: ids.version, currentVersion: "1.0.0", latestReleaseId: ids.release,
        latestReleaseNo: "R1", latestReleaseStatus: state.releaseStatus, createdAt: stamp, updatedAt: stamp,
      };
      const release = {
        id: ids.release, productId: ids.product, productVersionId: ids.version, releaseNo: "R1",
        status: state.releaseStatus, createdAt: stamp, datasets: [], releaseNotes: "Synthetic browser fixture",
        ...(state.releaseStatus === "PUBLISHED" ? { releasedAt: stamp } : {}),
      };
      const job = { id: ids.job, workspaceId: ids.workspace, status: state.candidateStatus === "PENDING" ? "WAITING_REVIEW" : "SUCCEEDED" };
      const candidate = {
        id: ids.candidate, candidateId: ids.candidate, jobId: ids.job, workspaceId: ids.workspace,
        sourceKey: "fixture-row-1", sourceName: "测试来源记录", source: { name: "测试来源记录" }, normalized: { name: "测试来源记录" },
        candidateEntityId: ids.entity, status: state.candidateStatus, decision: "REVIEW", matchMethod: "RULE",
        matchRuleId: "fixture-rule", confidence: 0.8, engineName: "fixture", engineVersion: "1", modelVersion: "1",
        policyRef: "test/policy", policyVersion: "1", createdAt: stamp,
        // The reviewer observes the mapping decision that is current when the
        // queue is rendered; the form submits it as the concurrency token.
        ...(state.candidateStatus === "PENDING" ? { currentMappingDecisionId: ids.decision } : {}),
      };
      function page(items) {
        const limit = Math.max(1, Number(url.searchParams.get("limit")) || 100);
        const offset = Math.max(0, Number(url.searchParams.get("offset")) || 0);
        return { items: items.slice(offset, offset + limit), page: { total: items.length, limit, offset } };
      }
      if (req.method === "GET") {
        if (url.pathname === `${workspace}/data-products`) return send(200, page([product]));
        if (url.pathname === `${workspace}/data-products/${ids.product}/releases`) return send(200, page([release]));
        if (url.pathname === releasePath) return send(200, release);
        if (url.pathname === `${releasePath}/readiness`) return send(200, readiness(state.scenario));
        if (url.pathname === `/api/v1/product-versions/${ids.version}`) return send(200, { id: ids.version, productId: ids.product, version: "1.0.0", definition: {}, assets: [] });
        if (url.pathname === `${workspace}/entity-match-reviews`) return send(200, page(state.candidateStatus === "PENDING" ? [candidate] : []));
        if (url.pathname === `/api/v1/entity-match-jobs/${ids.job}`) return send(200, job);
        if (url.pathname === `/api/v1/entity-match-reviews/${ids.candidate}`) return send(200, candidate);
        if (url.pathname === `/api/v1/entity-match-jobs/${ids.job}/reviews`) return send(200, { items: [candidate] });
        if (url.pathname === `${workspace}/data-resources`) return send(200, page(state.scenario === "paginated-resources" ? fixtureResources(30) : []));
        if (["/datasets", "/executions"].some((suffix) => url.pathname === workspace + suffix)) return send(200, page([]));
        if (url.pathname === `${workspace}/workbench`) return send(200, {
          workspaceId: ids.workspace, counts: { dataResources: 0, datasets: 0, dataProducts: 1 },
          reviewQueue: { pending: state.candidateStatus === "PENDING" ? 1 : 0, unresolved: 0, conflicts: 0 },
          executions: { queued: 0, submitting: 0, running: 0, succeeded: 0, failed: 0 },
          releases: { draft: 0, ready: state.releaseStatus === "READY" ? 1 : 0, published: state.releaseStatus === "PUBLISHED" ? 1 : 0, failed: 0 }, updatedAt: stamp,
        });
      }
      if (req.method === "POST") {
        if (req.headers["x-actor-id"] !== ids.actor) return send(400, { error: { code: "FIXTURE_ACTOR_REQUIRED" } });
        const reviewPrefix = `/api/v1/entity-match-reviews/${ids.candidate}/`;
        if ([reviewPrefix + "confirm", reviewPrefix + "reject"].includes(url.pathname)) {
          if (typeof body.reason !== "string" || !body.reason.trim()) return send(400, { error: { code: "REVIEW_REASON_REQUIRED" } });
          // A stale or absent token must never replace the decision the reviewer
          // saw; only the exact observed decision is accepted.
          if (body.expectedDecisionId !== ids.decision) return send(409, { error: { code: "ENTITY_MAPPING_DECISION_CONFLICT" } });
          if (state.scenario === "review-conflict" || state.candidateStatus !== "PENDING") return send(409, { error: { code: "REVIEW_CONFLICT" } });
          state.candidateStatus = url.pathname.endsWith("/confirm") ? "CONFIRMED" : "REJECTED";
          return send(200, { ...job, status: "SUCCEEDED" });
        }
        if (url.pathname === `${releasePath}/publish`) {
          if (!req.headers["idempotency-key"]) return send(400, { error: { code: "FIXTURE_KEY_REQUIRED" } });
          if (state.scenario === "publish-conflict" || state.scenario !== "ready" || state.releaseStatus !== "READY") return send(409, { error: { code: "PRODUCT_RELEASE_NOT_READY" } });
          state.releaseStatus = "PUBLISHED";
          return send(200, { ...release, status: "PUBLISHED", releasedAt: stamp });
        }
      }
      return send(501, { error: { code: "UNIMPLEMENTED_FIXTURE_ROUTE" } });
    } catch {
      return send(400, { error: { code: "INVALID_FIXTURE_REQUEST" } });
    }
  });
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  if (process.env.BROWSER_FIXTURE !== "1") throw new Error("Set BROWSER_FIXTURE=1 for test-only startup");
  const server = createFixtureServer();
  server.listen(4400, "127.0.0.1", () => console.log("Test-only Core fixture on 127.0.0.1:4400"));
  process.on("SIGTERM", () => server.close(() => process.exit(0)));
}
