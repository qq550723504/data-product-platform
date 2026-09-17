import http from "node:http";

const ids = {
  workspace: "11111111-1111-4111-8111-111111111111",
  actor: "22222222-2222-4222-8222-222222222222",
  candidate: "33333333-3333-4333-8333-333333333333",
  job: "44444444-4444-4444-8444-444444444444",
  product: "55555555-5555-4555-8555-555555555555",
  productVersion: "66666666-6666-4666-8666-666666666666",
  release: "77777777-7777-4777-8777-777777777777",
  dataset: "88888888-8888-4888-8888-888888888888",
  datasetVersion: "99999999-9999-4999-8999-999999999999",
  execution: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  evidence: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
  snapshot: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
  rights: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
  quality: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
  compliance: "ffffffff-ffff-4fff-8fff-ffffffffffff",
  contract: "12121212-1212-4212-8212-121212121212",
};

let candidatePending = true;
let releaseStatus = "READY";

function json(res, status, body) {
  const encoded = JSON.stringify(body);
  res.writeHead(status, {
    "content-type": "application/json",
    "content-length": Buffer.byteLength(encoded),
  });
  res.end(encoded);
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    let body = "";
    req.setEncoding("utf8");
    req.on("data", (chunk) => { body += chunk; });
    req.on("end", () => {
      if (!body) return resolve({});
      try { resolve(JSON.parse(body)); } catch (error) { reject(error); }
    });
    req.on("error", reject);
  });
}

const releaseListItem = () => ({
  id: ids.release,
  productId: ids.product,
  productVersionId: ids.productVersion,
  releaseNo: "R-2026-09",
  status: releaseStatus,
  contractVersionId: ids.contract,
  rightsSnapshotId: ids.rights,
  qualityResultId: ids.quality,
  complianceResultId: ids.compliance,
  evidenceSnapshotId: ids.snapshot,
  releaseNotes: "Enterprise Activity POC release",
  createdAt: "2026-09-17T01:00:00Z",
  releasedAt: releaseStatus === "PUBLISHED" ? "2026-09-17T03:00:00Z" : null,
});

const job = () => ({
  id: ids.job,
  workspaceId: ids.workspace,
  entityTypeId: "13131313-1313-4313-8313-131313131313",
  inputDatasetVersionId: "14141414-1414-4414-8414-141414141414",
  outputDatasetId: ids.dataset,
  outputDatasetVersionId: candidatePending ? null : ids.datasetVersion,
  sourceRole: "REFERENCE",
  policyRef: "park/matching/company-match-policy-v1.yaml",
  policyVersion: "1.0.0",
  status: candidatePending ? "WAITING_REVIEW" : "SUCCEEDED",
  autoMatchCount: 4,
  reviewCount: 1,
  unresolvedCount: 0,
  rejectedCount: 0,
  errorMessage: "",
});

const candidate = () => ({
  id: ids.candidate,
  jobId: ids.job,
  sourceKey: "ENT-005",
  sourceName: "深圳前海星河科技有限责任公司",
  candidateEntityId: "15151515-1515-4515-8515-151515151515",
  decision: "REVIEW",
  status: candidatePending ? "PENDING" : "CONFIRMED",
  matchMethod: "FELLEGI_SUNTER",
  matchRuleId: "PROBABILISTIC_CANDIDATE",
  matchEngineName: "SPLINK",
  matchEngineVersion: "4.0.17",
  matchModelVersion: "1.0.0",
  confidence: 0.8732,
  source: { source_company_id: "ENT-005", company_name: "深圳前海星河科技有限责任公司" },
  normalized: { normalized_company_name: "深圳前海星河科技有限公司", legal_representative: "张三" },
  reviewerReason: candidatePending ? "" : "同一经营主体，名称与法人信息一致",
  evidenceId: candidatePending ? null : ids.evidence,
});

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", "http://127.0.0.1:18080");
  const path = url.pathname;

  if (path === "/healthz") return json(res, 200, { ok: true });

  if (req.method === "GET" && path === `/api/v1/workspaces/${ids.workspace}/workbench`) {
    return json(res, 200, {
      workspaceId: ids.workspace,
      counts: { dataResources: 3, datasets: 5, dataProducts: 1 },
      reviewQueue: { pending: candidatePending ? 1 : 0, unresolved: 0, conflicts: 0 },
      executions: { queued: 0, submitting: 0, running: 0, succeeded: 3, failed: 0 },
      releases: { draft: 0, ready: releaseStatus === "READY" ? 1 : 0, published: releaseStatus === "PUBLISHED" ? 1 : 0, failed: 0 },
      updatedAt: "2026-09-17T03:00:00Z",
    });
  }

  if (req.method === "GET" && path === `/api/v1/workspaces/${ids.workspace}/executions`) {
    return json(res, 200, {
      items: [{
        id: ids.execution,
        workspaceId: ids.workspace,
        workflowVersionId: "16161616-1616-4616-8616-161616161616",
        workflowCode: "enterprise-activity",
        workflowName: "企业经营活跃度",
        workflowVersion: "1.0.0",
        outputDatasetId: ids.dataset,
        outputDatasetVersionId: ids.datasetVersion,
        targetPeriod: "2026-09",
        status: "SUCCEEDED",
        attempt: 1,
        engineType: "HOP",
        errorCode: "",
        createdAt: "2026-09-17T01:20:00Z",
        startedAt: "2026-09-17T01:20:01Z",
        finishedAt: "2026-09-17T01:20:03Z",
      }],
      page: { limit: Number(url.searchParams.get("limit") ?? 100), offset: 0, total: 1 },
    });
  }

  if (req.method === "GET" && path === `/api/v1/workspaces/${ids.workspace}/entity-match-reviews`) {
    const item = candidate();
    return json(res, 200, {
      items: candidatePending ? [{
        candidateId: item.id,
        jobId: item.jobId,
        workspaceId: ids.workspace,
        sourceKey: item.sourceKey,
        sourceName: item.sourceName,
        source: item.source,
        normalized: item.normalized,
        candidateEntityId: item.candidateEntityId,
        decision: item.decision,
        status: item.status,
        matchMethod: item.matchMethod,
        matchRuleId: item.matchRuleId,
        confidence: item.confidence,
        engineName: item.matchEngineName,
        engineVersion: item.matchEngineVersion,
        modelVersion: item.matchModelVersion,
        policyRef: "park/matching/company-match-policy-v1.yaml",
        policyVersion: "1.0.0",
        createdAt: "2026-09-17T01:10:00Z",
      }] : [],
      page: { limit: 100, offset: 0, total: candidatePending ? 1 : 0 },
    });
  }

  if (req.method === "GET" && path === `/api/v1/entity-match-jobs/${ids.job}`) return json(res, 200, job());
  if (req.method === "GET" && path === `/api/v1/entity-match-jobs/${ids.job}/reviews`) return json(res, 200, { items: [candidate()] });

  if (req.method === "POST" && (path === `/api/v1/entity-match-reviews/${ids.candidate}/confirm` || path === `/api/v1/entity-match-reviews/${ids.candidate}/reject`)) {
    if (req.headers["x-actor-id"] !== ids.actor) return json(res, 400, { code: "REVIEWER_REQUIRED", message: "actor required" });
    const body = await readBody(req);
    if (!String(body.reason ?? "").trim()) return json(res, 400, { code: "REVIEW_REASON_REQUIRED", message: "reason required" });
    candidatePending = false;
    return json(res, 200, job());
  }

  if (req.method === "GET" && path === `/api/v1/workspaces/${ids.workspace}/data-products`) {
    return json(res, 200, {
      items: [{
        id: ids.product,
        workspaceId: ids.workspace,
        code: "PARK-ENTERPRISE-ACTIVITY",
        name: "企业经营活跃度",
        description: "企业融资风险辅助数据产品",
        domainCode: "PARK",
        lifecycleStatus: releaseStatus === "PUBLISHED" ? "ACTIVE" : "READY",
        healthStatus: "HEALTHY",
        currentVersionId: ids.productVersion,
        currentVersion: "1.0.0",
        latestReleaseId: ids.release,
        latestReleaseNo: "R-2026-09",
        latestReleaseStatus: releaseStatus,
        createdAt: "2026-09-17T00:00:00Z",
        updatedAt: "2026-09-17T03:00:00Z",
      }],
      page: { limit: 100, offset: 0, total: 1 },
    });
  }

  if (req.method === "GET" && path === `/api/v1/data-products/${ids.product}`) {
    return json(res, 200, {
      id: ids.product,
      workspaceId: ids.workspace,
      projectId: null,
      useCaseId: "17171717-1717-4717-8717-171717171717",
      code: "PARK-ENTERPRISE-ACTIVITY",
      name: "企业经营活跃度",
      description: "企业融资风险辅助数据产品",
      domainCode: "PARK",
      ownerId: ids.actor,
      lifecycleStatus: "READY",
      healthStatus: "HEALTHY",
      currentVersionId: ids.productVersion,
      latestReleaseId: ids.release,
      metadata: {},
      createdAt: "2026-09-17T00:00:00Z",
    });
  }

  if (req.method === "GET" && path === `/api/v1/product-versions/${ids.productVersion}`) {
    return json(res, 200, {
      id: ids.productVersion,
      productId: ids.product,
      version: "1.0.0",
      workflowVersionId: "16161616-1616-4616-8616-161616161616",
      contractVersionId: ids.contract,
      entityPolicyRef: "park/matching/company-match-policy-v1.yaml",
      indicatorSetRef: "park/indicators/enterprise-activity-v1.yaml",
      definition: { purpose: "enterprise financing risk support" },
      assets: [{
        id: "18181818-1818-4818-8818-181818181818",
        assetType: "DATASET",
        name: "企业经营活跃度月度数据集",
        datasetId: ids.dataset,
        externalRef: "",
        deliveryConfig: { format: "CSV" },
        schemaSnapshot: {},
      }],
      createdAt: "2026-09-17T00:30:00Z",
    });
  }

  if (req.method === "GET" && path === `/api/v1/workspaces/${ids.workspace}/data-products/${ids.product}/releases`) {
    return json(res, 200, { items: [releaseListItem()], page: { limit: 100, offset: 0, total: 1 } });
  }

  if (req.method === "GET" && path === `/api/v1/product-releases/${ids.release}`) {
    return json(res, 200, {
      ...releaseListItem(),
      datasets: [{ datasetVersionId: ids.datasetVersion, role: "PRIMARY" }],
      metadata: { period: "2026-09" },
    });
  }

  if (req.method === "GET" && path === `/api/v1/product-releases/${ids.release}/readiness`) {
    return json(res, 200, {
      releaseId: ids.release,
      overall: "READY",
      checks: {
        production: "PASS", dataset: "PASS", rights: "PASS", quality: "PASS",
        compliance: "PASS", contract: "PASS", evidence: "PASS", delivery: "PASS",
      },
      blockers: [],
      details: { rights: { effectiveActions: ["PRODUCTIZE", "PUBLISH"] } },
    });
  }

  if (req.method === "POST" && path === `/api/v1/product-releases/${ids.release}/publish`) {
    if (!req.headers["idempotency-key"]) return json(res, 400, { code: "IDEMPOTENCY_KEY_REQUIRED", message: "key required" });
    if (req.headers["x-actor-id"] !== ids.actor) return json(res, 400, { code: "INVALID_ACTOR_ID", message: "actor required" });
    releaseStatus = "PUBLISHED";
    return json(res, 200, {
      ...releaseListItem(),
      datasets: [{ datasetVersionId: ids.datasetVersion, role: "PRIMARY" }],
      metadata: { period: "2026-09" },
    });
  }

  if (req.method === "GET" && path === `/api/v1/traceability/product-releases/${ids.release}`) {
    return json(res, 200, {
      releaseId: ids.release,
      releaseNo: "R-2026-09",
      status: releaseStatus,
      productId: ids.product,
      productVersionId: ids.productVersion,
      evidenceSnapshot: {
        id: ids.snapshot,
        rootHash: "6f8a0c80c0ffee1234567890abcdef1234567890abcdef1234567890abcdef12",
        integrityValid: true,
        manifest: { releaseNo: "R-2026-09" },
        items: [],
        createdAt: "2026-09-17T02:00:00Z",
      },
      datasetVersions: [{
        id: ids.datasetVersion,
        datasetId: ids.dataset,
        datasetCode: "enterprise_activity_product",
        datasetType: "PRODUCT",
        versionNo: 7,
        status: "READY",
        releaseRole: "PRIMARY",
        storageUri: "s3://poc/enterprise_activity_product/v7.csv",
        checksumAlgorithm: "SHA256",
        checksumValue: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
        generatedByExecutionId: ids.execution,
      }],
      executions: [{
        id: ids.execution,
        workflowVersionId: "16161616-1616-4616-8616-161616161616",
        workflowVersion: "1.0.0",
        workflowDefinitionHash: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
        status: "SUCCEEDED",
        attempt: 1,
        engineType: "HOP",
        engineExecutionId: "hop-run-poc",
        targetPeriod: "2026-09",
        outputDatasetVersionId: ids.datasetVersion,
        metrics: { outputRowCount: 5 },
      }],
      entityMatchJobs: [{ id: ids.job, policyVersion: "1.0.0", status: "SUCCEEDED" }],
      entityMappings: [{ id: "19191919-1919-4919-8919-191919191919", matchMethod: "FELLEGI_SUNTER", confidence: 0.8732, status: "CONFIRMED" }],
      evidence: [{
        id: ids.evidence,
        workspaceId: ids.workspace,
        evidenceType: "ENTITY_MATCH_REVIEW",
        title: "Manual entity match confirmed",
        sourceType: "ENTITY_MATCH_CANDIDATE",
        sourceId: ids.candidate,
        hashAlgorithm: "SHA256",
        hashValue: "abcdef",
        integrityValid: true,
        metadata: { reviewerReason: "同一经营主体，名称与法人信息一致" },
        relationType: "SUPPORTS",
        createdAt: "2026-09-17T01:15:00Z",
        createdBy: ids.actor,
      }],
      costEvents: [{ id: "20202020-2020-4020-8020-202020202020", costType: "PROCESSING_EXECUTION", quantity: 1 }],
      auditEvents: [{ id: "21212121-2121-4121-8121-212121212121", action: "PRODUCT_RELEASE_READY", objectType: "PRODUCT_RELEASE" }],
    });
  }

  json(res, 404, { code: "MOCK_NOT_FOUND", message: `${req.method} ${path}` });
});

server.listen(18080, "127.0.0.1", () => {
  console.log("mock Core API listening on 127.0.0.1:18080");
});
