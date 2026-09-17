import http from "node:http";

const port = Number(process.env.POC_BROWSER_FIXTURE_PORT ?? 18080);
const workspaceId = "11111111-1111-4111-8111-111111111111";
const reviewerId = "22222222-2222-4222-8222-222222222222";
const productId = "33333333-3333-4333-8333-333333333333";
const releaseId = "44444444-4444-4444-8444-444444444444";
const versionId = "55555555-5555-4555-8555-555555555555";
const candidateId = "66666666-6666-4666-8666-666666666666";
const jobId = "77777777-7777-4777-8777-777777777777";
const entityId = "88888888-8888-4888-8888-888888888888";
const datasetId = "99999999-9999-4999-8999-999999999999";
const datasetVersionId = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const now = "2026-09-17T05:30:00Z";

const requiredChecks = {
  production: "PASS",
  dataset: "PASS",
  rights: "PASS",
  quality: "PASS",
  compliance: "PASS",
  contract: "PASS",
  evidence: "PASS",
  delivery: "PASS",
};

let state = resetState("ready");

function resetState(mode) {
  return {
    mode,
    reviewStatus: "PENDING",
    reviewDecision: null,
    publishCount: 0,
    releaseStatus: "READY",
  };
}

function send(response, status, payload) {
  const body = JSON.stringify(payload);
  response.writeHead(status, { "Content-Type": "application/json", "Content-Length": Buffer.byteLength(body) });
  response.end(body);
}

async function bodyJSON(request) {
  const chunks = [];
  for await (const chunk of request) chunks.push(chunk);
  if (chunks.length === 0) return {};
  return JSON.parse(Buffer.concat(chunks).toString("utf8"));
}

function product() {
  return {
    id: productId,
    workspaceId,
    code: "DP-ENTERPRISE-ACTIVITY",
    name: "企业经营活跃度",
    description: "Browser fixture product",
    domainCode: "PARK",
    lifecycleStatus: "ACTIVE",
    healthStatus: "HEALTHY",
    currentVersionId: versionId,
    currentVersion: "1.0.0",
    latestReleaseId: releaseId,
    latestReleaseNo: "R1",
    latestReleaseStatus: state.releaseStatus,
    createdAt: now,
    updatedAt: now,
  };
}

function release() {
  return {
    id: releaseId,
    productId,
    productVersionId: versionId,
    releaseNo: "R1",
    status: state.releaseStatus,
    contractVersionId: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
    rightsSnapshotId: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
    qualityResultId: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
    complianceResultId: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
    evidenceSnapshotId: state.releaseStatus === "PUBLISHED" ? "ffffffff-ffff-4fff-8fff-ffffffffffff" : undefined,
    datasets: [{ datasetVersionId, role: "PRIMARY" }],
    releaseNotes: "Browser fixture",
    metadata: {},
    createdAt: now,
    releasedAt: state.releaseStatus === "PUBLISHED" ? "2026-09-17T05:31:00Z" : undefined,
  };
}

function readiness() {
  const checks = { ...requiredChecks };
  const blockers = [];
  let overall = "READY";
  if (state.mode === "stale") {
    // Deliberately contradictory payload: old web code trusted overall READY.
    checks.rights = "FAIL";
  }
  if (state.mode === "blocked") {
    checks.evidence = "PENDING";
    blockers.push("EVIDENCE_INCOMPLETE");
    overall = "NOT_READY";
  }
  if (state.mode === "future") {
    checks.externalPublication = "PENDING";
  }
  return { releaseId, overall, checks, blockers, details: { fixtureMode: state.mode } };
}

function review() {
  return {
    candidateId,
    jobId,
    workspaceId,
    sourceKey: "ENT-005",
    sourceName: "深圳星云科技集团有限公司",
    source: { source_company_id: "ENT-005", company_name: "深圳星云科技集团有限公司" },
    normalized: { source_company_id: "ENT-005", normalized_company_name: "深圳星云科技集团有限公司" },
    candidateEntityId: entityId,
    decision: "REVIEW",
    status: state.reviewStatus,
    matchMethod: "FELLEGI_SUNTER",
    matchRuleId: "PROBABILISTIC_CANDIDATE",
    confidence: 0.87,
    engineName: "SPLINK",
    engineVersion: "4.0.17",
    modelVersion: "1.0.0",
    policyRef: "park-company-match",
    policyVersion: "1.0.0",
    createdAt: now,
  };
}

function globalReview() {
  const item = review();
  return {
    id: item.candidateId,
    jobId: item.jobId,
    sourceKey: item.sourceKey,
    sourceName: item.sourceName,
    candidateEntityId: item.candidateEntityId,
    decision: item.decision,
    status: item.status,
    matchMethod: item.matchMethod,
    matchRuleId: item.matchRuleId,
    confidence: item.confidence,
  };
}

function job() {
  return {
    id: jobId,
    workspaceId,
    entityTypeId: "12121212-1212-4212-8212-121212121212",
    inputDatasetVersionId: "13131313-1313-4313-8313-131313131313",
    outputDatasetId: datasetId,
    outputDatasetVersionId: state.reviewStatus === "PENDING" ? undefined : datasetVersionId,
    sourceRole: "REFERENCE",
    policyRef: "park/matching/company-match-policy-v1.yaml",
    policyVersion: "1.0.0",
    status: state.reviewStatus === "PENDING" ? "WAITING_REVIEW" : "SUCCEEDED",
    autoMatchCount: 0,
    reviewCount: 1,
    unresolvedCount: 0,
    rejectedCount: state.reviewStatus === "REJECTED" ? 1 : 0,
    errorMessage: "",
  };
}

const server = http.createServer(async (request, response) => {
  const url = new URL(request.url ?? "/", `http://${request.headers.host ?? `127.0.0.1:${port}`}`);

  if (request.method === "POST" && url.pathname === "/__test/reset") {
    const payload = await bodyJSON(request);
    state = resetState(typeof payload.mode === "string" ? payload.mode : "ready");
    return send(response, 200, { ok: true, state });
  }
  if (request.method === "GET" && url.pathname === "/__test/state") {
    return send(response, 200, state);
  }

  if (request.method === "GET" && url.pathname === `/api/v1/workspaces/${workspaceId}/entity-match-reviews`) {
    const items = state.reviewStatus === "PENDING" ? [review()] : [];
    return send(response, 200, { items, page: { limit: 25, offset: 0, total: items.length } });
  }
  if (request.method === "GET" && url.pathname === `/api/v1/entity-match-jobs/${jobId}`) {
    return send(response, 200, job());
  }
  if (request.method === "GET" && url.pathname === `/api/v1/entity-match-jobs/${jobId}/reviews`) {
    return send(response, 200, { items: [globalReview()] });
  }
  if (request.method === "POST" && (url.pathname === `/api/v1/entity-match-reviews/${candidateId}/confirm` || url.pathname === `/api/v1/entity-match-reviews/${candidateId}/reject`)) {
    if (state.reviewStatus !== "PENDING") return send(response, 409, { error: { code: "ENTITY_MATCH_REVIEW_CONFLICT" } });
    if (request.headers["x-actor-id"] !== reviewerId) return send(response, 400, { error: { code: "REVIEWER_REQUIRED" } });
    const payload = await bodyJSON(request);
    if (typeof payload.reason !== "string" || !payload.reason.trim()) return send(response, 400, { error: { code: "REVIEW_REASON_REQUIRED" } });
    state.reviewDecision = url.pathname.endsWith("/confirm") ? "confirm" : "reject";
    state.reviewStatus = state.reviewDecision === "confirm" ? "CONFIRMED" : "REJECTED";
    return send(response, 200, job());
  }

  if (request.method === "GET" && url.pathname === `/api/v1/workspaces/${workspaceId}/data-products`) {
    return send(response, 200, { items: [product()], page: { limit: 100, offset: 0, total: 1 } });
  }
  if (request.method === "GET" && url.pathname === `/api/v1/workspaces/${workspaceId}/data-products/${productId}/releases`) {
    return send(response, 200, { items: [release()], page: { limit: 100, offset: 0, total: 1 } });
  }
  if (request.method === "GET" && url.pathname === `/api/v1/product-versions/${versionId}`) {
    return send(response, 200, {
      id: versionId,
      productId,
      version: "1.0.0",
      workflowVersionId: "14141414-1414-4414-8414-141414141414",
      contractVersionId: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
      entityPolicyRef: "park-company-match@1.0.0",
      indicatorSetRef: "park-enterprise-activity@1.0.0",
      definition: { reference: "browser-fixture" },
      assets: [{ id: "15151515-1515-4515-8515-151515151515", assetType: "DATASET", name: "enterprise_activity", datasetId, externalRef: "", deliveryConfig: {}, schemaSnapshot: {} }],
      createdAt: now,
    });
  }
  if (request.method === "GET" && url.pathname === `/api/v1/product-releases/${releaseId}`) {
    return send(response, 200, release());
  }
  if (request.method === "GET" && url.pathname === `/api/v1/product-releases/${releaseId}/readiness`) {
    return send(response, 200, readiness());
  }
  if (request.method === "POST" && url.pathname === `/api/v1/product-releases/${releaseId}/publish`) {
    state.publishCount += 1;
    if (state.mode === "conflict") return send(response, 409, { error: { code: "PRODUCT_RELEASE_NOT_READY", message: "fixture private detail" } });
    if (state.releaseStatus !== "READY") return send(response, 409, { error: { code: "PRODUCT_RELEASE_NOT_READY" } });
    if (!request.headers["idempotency-key"] || request.headers["x-actor-id"] !== reviewerId) return send(response, 400, { error: { code: "PUBLISH_HEADERS_REQUIRED" } });
    state.releaseStatus = "PUBLISHED";
    return send(response, 200, release());
  }

  return send(response, 404, { error: { code: "FIXTURE_NOT_FOUND", path: url.pathname } });
});

server.listen(port, "127.0.0.1", () => {
  process.stdout.write(`fixture Core listening on http://127.0.0.1:${port}\n`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
