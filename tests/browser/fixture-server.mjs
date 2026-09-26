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
  alternative: "abababab-abab-4bab-8bab-abababababab",
  decision: "99999999-9999-4999-8999-999999999999",
  goldDataset: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  goldVersion: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
  goldAssessment: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
  goldProfile: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
  goldCertification: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
  goldBinding: "ffffffff-ffff-4fff-8fff-ffffffffffff",
  goldSnapshot: "12121212-1212-4212-8212-121212121212",
};
export const fixtureToken = "local-browser-test-only";
const stamp = "2026-09-17T00:00:00Z";
const scenarios = new Set(["ready", "empty-checks", "missing-evidence", "null-checks", "blocker", "future-gate", "failed-rights", "review-conflict", "ambiguous-review", "publish-conflict", "paginated-resources"]);
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
      state.requests.push({ method: req.method, path: url.pathname, body, actor: req.headers["x-actor-id"], authorization: req.headers["authorization"], idempotencyKey: req.headers["idempotency-key"] });
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
      const goldDataset = {
        id: ids.goldDataset, workspaceId: ids.workspace, code: "GOLD_BROWSER_FIXTURE",
        name: "Gold 浏览器验收数据集", description: "Synthetic Gold fixture", datasetType: "CURATED",
        lifecycleStatus: "ACTIVE", currentVersionId: ids.goldVersion, createdAt: stamp, updatedAt: stamp,
      };
      const goldVersion = {
        id: ids.goldVersion, datasetId: ids.goldDataset, versionNo: 1, status: "READY",
        schemaVersion: "gold-v1", storageType: "OBJECT", storageUri: "s3://fixture/gold.csv",
        contentType: "text/csv", rowCount: 2, byteSize: 128, checksumAlgorithm: "SHA256",
        checksum: "1".repeat(64), generatedByExecutionId: ids.job, createdAt: stamp, readyAt: stamp,
        invalidationReason: "",
      };
      const goldAssessment = {
        id: ids.goldAssessment, workspaceId: ids.workspace, datasetVersionId: ids.goldVersion,
        ruleSetRef: "gold/quality/annotation-v1", ruleSetVersion: "1",
        ruleSetContentSha256: "2".repeat(64), evaluatorName: "gold-quality", evaluatorVersion: "1",
        gateDecision: "PASS",
        metrics: { formalAssessment: true, productionBindingId: ids.goldBinding, annotationSnapshotId: ids.goldSnapshot },
        dimensionSummary: {
          COMPLETENESS: { dimension: "COMPLETENESS", status: "PASS", ruleCount: 2, evaluatedCount: 2, failedCount: 0 },
          TRACEABILITY: { dimension: "TRACEABILITY", status: "PASS", ruleCount: 1, evaluatedCount: 1, failedCount: 0 },
        },
        createdAt: stamp,
      };
      const goldProfile = {
        id: ids.goldProfile, workspaceId: ids.workspace, profileRef: "gold/dataset-v1",
        code: "GOLD_DATASET", name: "Gold Dataset", version: "1.0.0", contentSha256: "3".repeat(64),
        purpose: { mode: "EXPLICIT", values: ["GOLD-PILOT"] },
        actions: { mode: "EXPLICIT", values: ["USE"] },
        consumers: { mode: "EXPLICIT", values: ["GOLD-PILOT-CONSUMER"] },
        delivery: { mode: "EXPLICIT", values: ["DIRECT_DATA"] },
        requiredQualityDimensions: ["COMPLETENESS", "TRACEABILITY"],
        requiredCriticalRules: ["GOLD-ANNOTATION-COVERAGE", "GOLD-PROVENANCE-COMPLETE"],
        qualityGateRequired: true,
        rights: {
          required: true,
          purpose: { mode: "EXPLICIT", values: ["GOLD-PILOT"] },
          actions: { mode: "EXPLICIT", values: ["USE"] },
          consumers: { mode: "EXPLICIT", values: ["GOLD-PILOT-CONSUMER"] },
          scopes: { mode: "EXPLICIT", values: [{ type: "ALL_RESOURCE", ref: ids.goldDataset }] },
        },
        complianceRequired: false, contractRequired: false, traceabilityRequired: false, evidenceRequired: true,
      };
      const goldCertification = {
        id: ids.goldCertification, workspaceId: ids.workspace, datasetVersionId: ids.goldVersion,
        qualityAssessmentId: ids.goldAssessment, decision: "CERTIFIED", blockers: [],
        reason: "all profile requirements satisfied", issuedAt: stamp,
        effectiveRightsSnapshotId: "34343434-3434-4434-8434-343434343434",
        effectiveRightsSnapshotHash: "4".repeat(64), frozenRightsContextHash: "5".repeat(64),
        evidenceSnapshotId: "56565656-5656-4656-8656-565656565656",
        goldProductionBindingId: ids.goldBinding,
        annotationSnapshotId: ids.goldSnapshot,
        annotationSnapshotRootHash: "6".repeat(64),
        annotationSchemaSha256: "7".repeat(64),
        annotationTaxonomySha256: "8".repeat(64),
        goldProductionBindingRootHash: "9".repeat(64),
        profile: goldProfile, dispositions: [],
      };
      const goldExplanation = {
        workspaceId: ids.workspace,
        outputDatasetVersionId: ids.goldVersion,
        inputDatasetVersionId: "13131313-1313-4313-8313-131313131313",
        inputCertificationId: "14141414-1414-4414-8414-141414141414",
        executionId: ids.job,
        goldProductionBindingId: ids.goldBinding,
        goldProductionBindingRootHash: "9".repeat(64),
        annotationContributionResourceId: "15151515-1515-4515-8515-151515151515",
        campaign: {
          id: "16161616-1616-4616-8616-161616161616",
          status: "ACTIVE",
          purpose: "GOLD-PILOT",
          action: "PROCESS",
          schemaRef: "gold/schema",
          schemaVersion: "1",
          schemaSha256: "7".repeat(64),
          taxonomyRef: "gold/taxonomy",
          taxonomyVersion: "1",
          taxonomySha256: "8".repeat(64),
          expectedTaskCount: 2,
          taskCount: 2,
          resultCount: 2,
          reviewDecisionCount: 2,
          selectedOutputCount: 2,
          createdAt: stamp,
          activatedAt: stamp,
        },
        snapshot: {
          id: ids.goldSnapshot,
          status: "FINALIZED",
          rootHash: "6".repeat(64),
          expectedTaskCount: 2,
          expectedResultCount: 2,
          expectedDecisionCount: 2,
          expectedOutputCount: 2,
          finalizedAt: stamp,
        },
        reviews: [
          {
            taskId: "17171717-1717-4717-8717-171717171717",
            sourceItemRef: "row:1",
            sourceContentSha256: "a".repeat(64),
            decisionId: "18181818-1818-4818-8818-181818181818",
            outcome: "ACCEPT",
            reviewerRef: "reviewer:gold-fixture",
            reason: "evidence is sufficient",
            reviewedResultId: "19191919-1919-4919-8919-191919191919",
            selectedResultId: "19191919-1919-4919-8919-191919191919",
            selectedResultSha256: "b".repeat(64),
            annotationAuthorRef: "annotator:label-studio-17",
            providerBindingRef: "label-studio/project/123",
            externalTaskId: "456",
            externalAnnotationId: "789",
            decisionCreatedAt: stamp,
          },
          {
            taskId: "20202020-2020-4020-8020-202020202020",
            sourceItemRef: "row:2",
            sourceContentSha256: "c".repeat(64),
            decisionId: "21212121-2121-4121-8121-212121212121",
            outcome: "CORRECT",
            reviewerRef: "reviewer:gold-fixture",
            reason: "corrected provider label",
            reviewedResultId: "22222222-2222-4222-8222-222222222223",
            selectedResultId: "23232323-2323-4323-8323-232323232323",
            selectedResultSha256: "d".repeat(64),
            annotationAuthorRef: "annotator:label-studio-17",
            providerBindingRef: "label-studio/project/123",
            externalTaskId: "457",
            externalAnnotationId: "790",
            decisionCreatedAt: stamp,
          },
        ],
        trace: {
          costs: [
            { id: "24242424-2424-4424-8424-242424242424", phase: "HUMAN_REVIEW", subjectType: "ANNOTATION_REVIEW_ATTEMPT", subjectId: "25252525-2525-4525-8525-252525252525", costType: "ANNOTATION_REVIEW", quantity: 1, unit: "review", pricingMode: "ACTUAL", occurredAt: stamp },
            { id: "26262626-2626-4626-8626-262626262626", phase: "GOLD_BUILD", subjectType: "EXECUTION", subjectId: ids.job, costType: "EXECUTION", quantity: 1, unit: "execution", pricingMode: "ACTUAL", occurredAt: stamp },
            { id: "27272727-2727-4727-8727-272727272727", phase: "GOLD_QUALITY", subjectType: "QUALITY_RESULT", subjectId: ids.goldAssessment, costType: "QUALITY_ENGINE_INVOCATION", quantity: 1, unit: "assessment", pricingMode: "ACTUAL", occurredAt: stamp },
            { id: "28282828-2828-4828-8828-282828282828", phase: "GOLD_CERTIFICATION", subjectType: "DATASET_CERTIFICATION", subjectId: ids.goldCertification, costType: "CERTIFICATION_EVALUATION", quantity: 1, unit: "certification", pricingMode: "ACTUAL", occurredAt: stamp },
            { id: "29292929-2929-4929-8929-292929292929", phase: "DIRECT_DATA", subjectType: "DELIVERY_OPERATION", subjectId: "30303030-3030-4030-8030-303030303030", costType: "DIRECT_DATA_DELIVERY_ATTEMPT", quantity: 1, unit: "attempt", pricingMode: "ACTUAL", occurredAt: stamp },
          ],
          evidence: [
            { id: "31313131-3131-4131-8131-313131313131", phase: "HUMAN_REVIEW", subjectType: "ANNOTATION_REVIEW_DECISION", subjectId: "18181818-1818-4818-8818-181818181818", evidenceType: "ANNOTATION_REVIEW_DECISION", relationType: "REVIEW_EVIDENCE", sourceType: "ANNOTATION_REVIEW_DECISION", hashAlgorithm: "SHA256", hashValue: "e".repeat(64), createdAt: stamp },
            { id: "32323232-3232-4232-8232-323232323232", phase: "GOLD_BUILD", subjectType: "GOLD_PRODUCTION_BINDING", subjectId: ids.goldBinding, evidenceType: "GOLD_PRODUCTION_BINDING_FINALIZED", relationType: "GOLD_PRODUCTION", sourceType: "EXECUTION", sourceId: ids.job, hashAlgorithm: "SHA256", hashValue: "f".repeat(64), createdAt: stamp },
            { id: "33333333-3333-4333-8333-333333333333", phase: "GOLD_QUALITY", subjectType: "QUALITY_RESULT", subjectId: ids.goldAssessment, evidenceType: "GOLD_QUALITY_ASSESSMENT", relationType: "QUALITY_EVIDENCE", sourceType: "QUALITY_RESULT", sourceId: ids.goldAssessment, hashAlgorithm: "SHA256", hashValue: "1".repeat(64), createdAt: stamp },
            { id: "34343434-3434-4434-8434-343434343435", phase: "GOLD_CERTIFICATION", subjectType: "DATASET_CERTIFICATION", subjectId: ids.goldCertification, evidenceType: "DATASET_CERTIFICATION", relationType: "CERTIFICATION_EVIDENCE", sourceType: "DATASET_CERTIFICATION", sourceId: ids.goldCertification, hashAlgorithm: "SHA256", hashValue: "2".repeat(64), createdAt: stamp },
            { id: "35353535-3535-4535-8535-353535353535", phase: "DIRECT_DATA", subjectType: "DELIVERY_OPERATION", subjectId: "30303030-3030-4030-8030-303030303030", evidenceType: "DATASETDELIVERYISSUED", relationType: "DELIVERY_EVIDENCE", sourceType: "DELIVERY_OPERATION", hashAlgorithm: "SHA256", hashValue: "3".repeat(64), createdAt: stamp },
          ],
          audit: [
            { id: "36363636-3636-4636-8636-363636363636", phase: "HUMAN_REVIEW", objectType: "ANNOTATION_REVIEW_DECISION", objectId: "18181818-1818-4818-8818-181818181818", action: "ANNOTATION_REVIEWED", actorType: "USER", actorId: ids.actor, traceId: "trace-review", occurredAt: stamp },
            { id: "37373737-3737-4737-8737-373737373737", phase: "GOLD_BUILD", objectType: "GOLD_PRODUCTION_BINDING", objectId: ids.goldBinding, action: "GOLD_PRODUCTION_BINDING_FINALIZED", actorType: "SYSTEM", traceId: "trace-build", occurredAt: stamp },
            { id: "38383838-3838-4838-8838-383838383838", phase: "GOLD_QUALITY", objectType: "QUALITY_RESULT", objectId: ids.goldAssessment, action: "GOLD_QUALITY_ASSESSMENT_COMPLETED", actorType: "SYSTEM", traceId: "trace-quality", occurredAt: stamp },
            { id: "39393939-3939-4939-8939-393939393939", phase: "GOLD_CERTIFICATION", objectType: "DATASET_CERTIFICATION", objectId: ids.goldCertification, action: "DATASET_CERTIFIED", actorType: "USER", actorId: ids.actor, traceId: "trace-certification", occurredAt: stamp },
            { id: "40404040-4040-4040-8040-404040404040", phase: "DIRECT_DATA", objectType: "DELIVERY_OPERATION", objectId: "30303030-3030-4030-8030-303030303030", action: "DATASETDELIVERYISSUED", actorType: "SYSTEM", traceId: "trace-delivery", occurredAt: stamp },
          ],
        },
      };
      const job = { id: ids.job, workspaceId: ids.workspace, status: state.candidateStatus === "PENDING" ? "WAITING_REVIEW" : "SUCCEEDED" };
      const candidate = {
        id: ids.candidate, candidateId: ids.candidate, jobId: ids.job, workspaceId: ids.workspace,
        sourceKey: "fixture-row-1", sourceName: "测试来源记录", source: { name: "测试来源记录" }, normalized: { name: "测试来源记录" },
        ...(state.scenario === "ambiguous-review"
          ? {
              alternatives: [
                { entityId: ids.entity, canonicalKey: "FIXTURE-A", canonicalName: "测试候选 A" },
                { entityId: ids.alternative, canonicalKey: "FIXTURE-B", canonicalName: "测试候选 B" },
              ],
            }
          : { candidateEntityId: ids.entity, alternatives: [] }),
        status: state.candidateStatus, decision: "REVIEW", matchMethod: "RULE",
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
        if (url.pathname === `${workspace}/datasets/${ids.goldDataset}`) return send(200, goldDataset);
        if (url.pathname === `/api/v1/dataset-versions/${ids.goldVersion}`) return send(200, goldVersion);
        if (url.pathname === `/api/v1/dataset-versions/${ids.goldVersion}/quality-assessments`) {
          return send(200, { datasetVersionId: ids.goldVersion, items: [goldAssessment], page: { total: 1, limit: 25, offset: 0 } });
        }
        if (url.pathname === `/api/v1/quality-assessments/${ids.goldAssessment}/report`) {
          return send(200, {
            ...goldAssessment,
            findings: { items: [], page: { total: 0, limit: 50, offset: 0 } },
            evidence: [{ id: "78787878-7878-4878-8878-787878787878", workspaceId: ids.workspace, evidenceType: "GOLD_QUALITY_ASSESSMENT", relationType: "ASSESSMENT_EVIDENCE", sourceType: "QUALITY_ASSESSMENT", sourceId: ids.goldAssessment, hashAlgorithm: "SHA256", hashValue: "a".repeat(64), createdAt: stamp }],
            auditEvents: [],
          });
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/gold-explanation`) {
          return send(200, goldExplanation);
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/certifications`) {
          return send(200, { workspaceId: ids.workspace, datasetVersionId: ids.goldVersion, asOf: stamp, items: [goldCertification] });
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/delivery-eligibility`) {
          return send(200, {
            allowed: true, blockers: [],
            datasetVersion: { status: "READY", allowed: true, blockers: [] },
            certification: { allowed: true, blockers: [], current: goldCertification },
            entitlement: {
              allowed: true, blockers: [],
              checks: [{ dataResourceId: ids.goldDataset, path: "DIRECT_USE", decision: "ALLOWED", reason: "fixture verified rights" }],
            },
          });
        }
        if (url.pathname === `${workspace}/datasets`) return send(200, page([goldDataset]));
        if (url.pathname === `${workspace}/executions`) return send(200, page([]));
        if (url.pathname === `${workspace}/workbench`) return send(200, {
          workspaceId: ids.workspace, counts: { dataResources: 0, datasets: 0, dataProducts: 1 },
          reviewQueue: { pending: state.candidateStatus === "PENDING" ? 1 : 0, unresolved: 0, conflicts: 0 },
          executions: { queued: 0, submitting: 0, running: 0, succeeded: 0, failed: 0 },
          releases: { draft: 0, ready: state.releaseStatus === "READY" ? 1 : 0, published: state.releaseStatus === "PUBLISHED" ? 1 : 0, failed: 0 }, updatedAt: stamp,
        });
      }
      if (req.method === "POST") {
        const reviewPrefix = `/api/v1/entity-match-reviews/${ids.candidate}/`;
        if ([reviewPrefix + "confirm", reviewPrefix + "reject"].includes(url.pathname)) {
          if (req.headers["authorization"] !== "Bearer review-secret") return send(401, { error: { code: "AUTHENTICATION_REQUIRED" } });
          if (typeof body.reason !== "string" || !body.reason.trim()) return send(400, { error: { code: "REVIEW_REASON_REQUIRED" } });
          // A stale or absent token must never replace the decision the reviewer
          // saw; only the exact observed decision is accepted.
          if (body.expectedDecisionId !== ids.decision) return send(409, { error: { code: "ENTITY_MAPPING_DECISION_CONFLICT" } });
          if (state.scenario === "ambiguous-review" && body.selectedEntityId !== ids.alternative) {
            return send(400, { error: { code: "ENTITY_SELECTION_NOT_ALLOWED" } });
          }
          if (state.scenario === "review-conflict" || state.candidateStatus !== "PENDING") return send(409, { error: { code: "REVIEW_CONFLICT" } });
          state.candidateStatus = url.pathname.endsWith("/confirm") ? "CONFIRMED" : "REJECTED";
          return send(200, { ...job, status: "SUCCEEDED" });
        }
        if (url.pathname === `${releasePath}/publish`) {
          if (req.headers["x-actor-id"] !== ids.actor) return send(400, { error: { code: "FIXTURE_ACTOR_REQUIRED" } });
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
