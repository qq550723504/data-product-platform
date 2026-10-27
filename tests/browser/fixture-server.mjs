/** Test-only HTTP fixture. Never imported by the application or exposed beyond loopback. */
import { createServer } from "node:http";
import { pathToFileURL, fileURLToPath } from "node:url";
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import {acceptanceBindingFixture} from '../../apps/web/tests/fixtures/acceptance-bindings.mjs';
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
  secondProfile: "dadadada-dada-4dad-8dad-dadadadadada",
  secondEvidence: "b2b2b2b2-b2b2-4b2b-8b2b-b2b2b2b2b2b2",
  ingestion: "23232323-2323-4232-8232-232323232323",
  goldCertification: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
  goldBinding: "ffffffff-ffff-4fff-8fff-ffffffffffff",
  goldSnapshot: "12121212-1212-4212-8212-121212121212",
  sourceResource: "23232323-2323-4323-8323-232323232323",
  retryParent: "41414141-4141-4141-8141-414141414141",
  retryChild: "43434343-4343-4343-8343-434343434343",
  createdRelease: "45454545-4545-4545-8545-454545454545",
  productAsset: "46464646-4646-4646-8646-464646464646",
  qualityAssessment: "47474747-4747-4747-8747-474747474747",
  complianceResult: "48484848-4848-4848-8848-484848484848",
  deliveryOperation: "49494949-4949-4949-8949-494949494949",
  deliveryRetryOperation: "50505050-5050-4050-8050-505050505050",
  qualityOutputVersion: "53535353-5353-4353-8353-535353535353",
  recheckAssessment: "54545454-5454-4454-8454-545454545454",
};
export const fixtureToken = "local-browser-test-only";
export const unobservedRequestId = n => `${String(n).padStart(8,"0")}-3521-4352-8352-${String(n).padStart(12,"0")}`;
export const requestAttemptId = n => `${String(n).padStart(8,"0")}-3522-4352-8352-${String(n).padStart(12,"0")}`;
export const requestObservationId = n => `${String(n).padStart(8,"0")}-3523-4352-8352-${String(n).padStart(12,"0")}`;
export const observedRequestId = unobservedRequestId(107);
export const observedAttemptId = n => requestAttemptId(1070030+n);
export const observedObservationId = n => requestObservationId(1070030+n);
const stamp = "2026-09-17T00:00:00Z";
const scenarios = new Set(["ready", "empty-checks", "missing-evidence", "null-checks", "blocker", "future-gate", "failed-rights", "pending-gates", "review-conflict", "ambiguous-review", "publish-conflict", "paginated-resources", "paginated-releases", "multiple-certification-profiles","batch-registered","batch-registering", "quality-attention", "quality-attention-unavailable", "quality-evidence-denied", "quality-attention-foreign", "schema-attention", "schema-attention-unavailable", "schema-attention-foreign", "schema-attention-mismatch"]);
scenarios.add("isolation-empty");
scenarios.add("isolation-validating");
scenarios.add("agent-quality-pages");
scenarios.add("leader-demo-quality");
scenarios.add("modern-trace");scenarios.add("modern-trace-foreign");scenarios.add("modern-trace-extra");
for(const s of ['acceptance-sources','acceptance-source-invalid','acceptance-source-foreign','acceptance-context-denied','acceptance-list-denied','acceptance-source-extra'])scenarios.add(s);
for(const s of ['current','expired','revoked','ambiguous','conflict','mismatch','replaced','history'])scenarios.add('acceptance-binding-'+s);
const readFaultKeys = new Set(["summary", "reviews", "executions", "failures", "products", "releases", "readiness", "failed-page", "validating-page", "ready-page", "quality", "schema", "exceptions", "requests", "attempts", "quality-original-report", "quality-latest-report", "quality-history", "quality-latest-summary", "quality-attempt", "agent-context"]);
const readFaultModes = new Set(["fail", "denied", "notfound", "invalid", "hold"]);
for (const s of ["schema-requests", "schema-requests-arrived", "schema-requests-empty", "schema-requests-unavailable", "schema-requests-foreign", "schema-requests-mismatch"]) scenarios.add(s);
for (const scenario of ["schema-attempts", "schema-attempts-arrived", "schema-attempts-empty", "schema-attempts-foreign", "schema-attempts-mismatch"]) scenarios.add(scenario);
function requestItem(n) {
  return {workspaceId:ids.workspace,resourceId:ids.sourceResource,resourceCode:"REQUEST_FIXTURE",resourceName:"无观察请求导航验收",requestId:unobservedRequestId(n),responsibleActorId:ids.actor,
    externalFqn:"historical.request-energy",ruleRef:"fixture/schema-request",ruleVersion:"1",ruleHash:"a".repeat(64),planHash:"b".repeat(64),requestedAt:stamp,attemptCount:n===107?136:n===106?31:n%2};
}
function requestAttempts(n) {
  if(n===107)return Array.from({length:136},(_,i)=>({id:requestAttemptId(1070000+i),requestId:observedRequestId,workspaceId:ids.workspace,registeredAt:stamp,observationId:i<31?requestObservationId(1070000+i):null})).reverse();
  return Array.from({length:requestItem(n).attemptCount},(_,i)=>({id:requestAttemptId(n===106?106000+i:n),requestId:unobservedRequestId(n),workspaceId:ids.workspace,registeredAt:stamp,observationId:n===106?requestObservationId(106000+i):null}));
}
function requestObservations(scenario) {
  const numbers=Array.from({length:31},(_,i)=>106000+i);
  if(scenario==="schema-requests-arrived")numbers.push(105);
  if(scenario.startsWith("schema-attempts"))numbers.push(...Array.from({length:31},(_,i)=>1070000+i));
  if(scenario==="schema-attempts-arrived")numbers.push(1070031);
  return numbers.map(n=>({...requestItem(n>=1070000?107:n===105?105:106),observationId:requestObservationId(n),attemptId:requestAttemptId(n),status:"UNAVAILABLE",reason:`RECORDED_UNAVAILABLE ${n}`,schemaHash:"",startedAt:null,completedAt:null,recordedAt:stamp,handlingCount:0}));
}
export const schemaObservationId = n => `${String(n).padStart(8,"0")}-3460-4346-8346-${String(n).padStart(12,"0")}`;
export const schemaRequestId = n => `${String(n).padStart(8,"0")}-3461-4346-8346-${String(n).padStart(12,"0")}`;
function schemaItems() {
  return Array.from({length:105},(_,index)=>{
    const n=index+1;
    return {workspaceId:ids.workspace,resourceId:ids.sourceResource,resourceCode:"SCHEMA_FIXTURE",resourceName:"schema 漂移导航验收",
      requestId:schemaRequestId(n),observationId:schemaObservationId(n),attemptId:ids.job,responsibleActorId:ids.actor,
      externalFqn:"historical.energy",ruleRef:"fixture/schema",ruleVersion:"1",ruleHash:"a".repeat(64),planHash:"b".repeat(64),
      status:"DRIFT",reason:`SOURCE_SCHEMA_CHANGED ${n}`,schemaHash:"c".repeat(64),requestedAt:stamp,completedAt:stamp,recordedAt:stamp,handlingCount:n===105?3:0};
  });
}
for (const scenario of ["schema-exceptions", "schema-exceptions-unavailable", "schema-exceptions-foreign", "schema-exceptions-mismatch", "schema-exceptions-empty"]) scenarios.add(scenario);
export const exceptionObservationId = n => `${String(n).padStart(8,"0")}-3490-4349-8349-${String(n).padStart(12,"0")}`;
export const exceptionRequestId = n => `${String(n <= 3 ? 1 : n).padStart(8,"0")}-3491-4349-8349-${String(n <= 3 ? 1 : n).padStart(12,"0")}`;
function exceptionItems() {
  return Array.from({length:105},(_,index)=>{
    const n=index+1, status=["UNAVAILABLE","DENIED","INTERRUPTED"][index%3];
    return {workspaceId:ids.workspace,resourceId:ids.sourceResource,resourceCode:"EXCEPTION_FIXTURE",resourceName:"扫描异常历史导航验收",
      requestId:exceptionRequestId(n),observationId:exceptionObservationId(n),attemptId:status==="DENIED"&&n%2===0?null:`${String(n).padStart(8,"0")}-3492-4349-8349-${String(n).padStart(12,"0")}`,responsibleActorId:ids.actor,
      externalFqn:"historical.energy",ruleRef:"fixture/schema",ruleVersion:"1",ruleHash:"a".repeat(64),planHash:"b".repeat(64),
      status,reason:`HISTORICAL_${status} ${n}`,schemaHash:"",requestedAt:stamp,startedAt:null,completedAt:null,recordedAt:stamp};
  });
}
export const attentionCaseId = n => `${String(n).padStart(8, "0")}-3430-4343-8343-${String(n).padStart(12, "0")}`;
function attentionItems() {
  return Array.from({ length: 105 }, (_, index) => {
    const n = index + 1;
    return {
      id: attentionCaseId(n), workspaceId: ids.workspace, datasetId: ids.goldDataset,
      datasetCode: "QUALITY_FIXTURE", datasetName: "质量异常导航验收", versionId: ids.goldVersion, versionNo: 1,
      resourceId: null, assessmentId: ids.goldAssessment, findingId: ids.goldBinding,
      ruleId: "negative-energy", message: `负数用电异常 ${n}`, actorId: null,
      executionId: n === 101 ? ids.job : null, executionStatus: n === 101 ? "FAILED" : null, outputDatasetId: null, outputVersionId: null,
      recheckId: [102, 105].includes(n) ? ids.ingestion : null, recheckOutcome: n === 105 ? "ASSESSED" : n === 102 ? "FAILED" : null,
      recheckAssessmentId: n === 105 ? ids.recheckAssessment : null, recheckGateDecision: n === 105 ? "PASS" : null,
      recheckDatasetId: n === 105 ? ids.goldDataset : null, recheckVersionId: n === 105 ? ids.qualityOutputVersion : null,
      pendingRecheck: n === 105, activeExecution: false, manuallyClosed: n >= 104,
      needsFollowUp: n !== 104, createdAt: stamp,
    };
  });
}
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

function fixtureReleases(count) {
  return Array.from({ length: count }, (_, index) => {
    const n = index + 1;
    return {
      id: `${String(n).padStart(8, "0")}-1111-4111-8111-${String(n).padStart(12, "0")}`,
      productId: ids.product,
      productVersionId: ids.version,
      releaseNo: `R${n}`,
      status: "READY",
      createdAt: new Date(Date.parse(stamp) + index * 1000).toISOString(),
      releaseNotes: "Synthetic paginated release fixture",
      datasets: [],
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
  if (scenario === "pending-gates") {
    result.overall = "NOT_READY";
    for (const gate of ["rights", "quality", "compliance", "contract", "evidence", "delivery"]) result.checks[gate] = "PENDING";
  }
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

export function createFixtureServer(initialScenario = "ready") {
  if (!scenarios.has(initialScenario)) throw new Error("Unknown initial fixture scenario");
  let state;
  const heldReads = new Set();
  const releaseReads = () => { for (const resolve of heldReads) resolve(); heldReads.clear(); };
  const reset = (scenario = "ready") => {
    releaseReads();
    state = { scenario, candidateStatus: scenario === "isolation-empty" ? "REJECTED" : "PENDING", releaseStatus: (scenario === "isolation-empty" || scenario.startsWith("modern-trace")) ? "PUBLISHED" : scenario === "isolation-validating" ? "VALIDATING" : "READY", datasetVersionStatus: "READY", invalidationReason: "", retryCreated: false, releaseCreated: false, qualityAttemptId: "", qualityCompleted: scenario.startsWith("quality-attention"), complianceAttemptId: "", compliancePolicyRef: "", complianceCompleted: false, deliveryAttempts: {}, deliveryCount: 0, requests: [], readFaults: {} };
  };
  reset(initialScenario);
  return createServer(async (req, res) => {
    const url = new URL(req.url, "http://127.0.0.1");
    const send = (status, value) => {
      res.writeHead(status, { "Content-Type": "application/json", "Cache-Control": "no-store" });
      res.end(JSON.stringify(value));
    };
    const sendBytes = (status, value, headers = {}) => {
      res.writeHead(status, { "Content-Type": "text/csv", "Cache-Control": "no-store", ...headers });
      res.end(value);
    };
    try {
      if (url.pathname === "/__health" && req.method === "GET") return send(200, { fixture: true });
      if (url.pathname.startsWith("/__control/")) {
        if (req.headers["x-fixture-token"] !== fixtureToken) return send(403, { error: "fixture control token required" });
        if (url.pathname === "/__control/state" && req.method === "GET") return send(200, { ...state, heldReadCount: heldReads.size });
        if (url.pathname === "/__control/release-reads" && req.method === "POST") { releaseReads(); return send(200, { released: true }); }
        if (req.method === "POST" && ["/__control/reset", "/__control/scenario"].includes(url.pathname)) {
          const { scenario = "ready", readFaults = {} } = await bodyOf(req);
          if (!scenarios.has(scenario)) return send(400, { error: "unknown fixture scenario" });
          if (!readFaults || Array.isArray(readFaults) || typeof readFaults !== "object" || Object.entries(readFaults).some(([key, value]) => !readFaultKeys.has(key) || !readFaultModes.has(value))) return send(400, { error: "invalid read fault" });
          if (url.pathname.endsWith("/reset")) reset(scenario); else state.scenario = scenario;
          if (scenario === "agent-quality-pages") state.qualityCompleted = true;
          state.readFaults = readFaults;
          return send(200, { scenario });
        }
        return send(404, { error: "unknown control route" });
      }
      const body = req.method === "POST" ? await bodyOf(req) : undefined;
      state.requests.push({ method: req.method, path: url.pathname, body, actor: req.headers["x-actor-id"], authorization: req.headers["authorization"], idempotencyKey: req.headers["idempotency-key"], gateway: req.headers["x-delivery-web-gateway"], authenticatedPrincipal: req.headers["x-authenticated-principal"] });
      const workspace = `/api/v1/workspaces/${ids.workspace}`;
      if(state.scenario.startsWith('acceptance-') && req.method==='GET' && (url.pathname===`${workspace}/contract-versions` || url.pathname===`${workspace}/acceptance-context` || url.pathname.startsWith('/api/v1/contract-versions/'))){
        if(req.headers.authorization!=='Bearer review-secret')return send(401,{code:'AUTHENTICATION_REQUIRED'});
        const standard=n=>({id:n===0?ids.version:`${String(n).padStart(8,'0')}-8100-4100-8100-${String(n).padStart(12,'0')}`,workspaceId:ids.workspace,contractId:ids.product,name:'合成验收标准 '+n,version:`1.0.${n}`,status:'PUBLISHED',sourceSha256:'a'.repeat(64),createdAt:stamp,publishedAt:stamp,acceptanceSource:{status:'SUPPORTED',schemaVersion:'1',setHash:'b'.repeat(64),criteria:[{id:'C1',clause:'3.1',text:'核对明确成果范围',required:true,method:'MANUAL_CHECK',evidenceCategory:'RECEIPT'}]},...(state.scenario==='acceptance-source-extra'?{document:{secret:'private-acceptance-source-sentinel'},sourceRef:'private-acceptance-location'}:{})});
        const standards=Array.from({length:26},(_,n)=>standard(n));
        if(url.pathname===`${workspace}/contract-versions`){if(state.scenario==='acceptance-list-denied')return send(403,{code:'WORKSPACE_AUTHORITY_DENIED'});const offset=Number(url.searchParams.get('offset')||0);return send(200,{workspaceId:ids.workspace,items:standards.slice(offset,offset+25),page:{limit:25,offset,total:26}});}
        if(url.pathname.startsWith('/api/v1/contract-versions/')){if(url.searchParams.get('workspaceId')!==ids.workspace)return send(403,{});const v=standards.find(v=>url.pathname.endsWith('/'+v.id));if(!v)return send(404,{});if(state.scenario==='acceptance-source-foreign')return send(200,{...v,workspaceId:ids.product});if(state.scenario==='acceptance-source-invalid')return send(200,{...v,acceptanceSource:{status:'UNSUPPORTED_SOURCE',schemaVersion:'',setHash:'',criteria:[]}});return send(200,v);}
        if(state.scenario==='acceptance-context-denied')return send(403,{});
        if(url.searchParams.get('deliveryOperationId')!==ids.deliveryOperation||url.searchParams.get('datasetVersionId')!==ids.goldVersion)return send(404,{});
        const selected=url.searchParams.get('contractVersionId');const items=selected?standards.filter(v=>v.id===selected):standards.slice(0,25);if(selected&&!items.length)return send(404,{});
        const context=acceptanceBindingFixture({ids,stamp,standards,selected,scenario:state.scenario,offset:Number(url.searchParams.get('bindingOffset')||0),selectedBinding:url.searchParams.get('bindingId')});
        return context?send(200,context):send(404,{});
      }
      const qualityRoute = /^\/api\/v1\/(?:quality-(?:results|assessments|assessment-attempts)\/|dataset-versions\/[^/]+\/(?:quality-assessments|quality-checks|gold-quality-checks)(?:\/|$))/.test(url.pathname);
      if (qualityRoute) {
        if (req.headers.authorization !== "Bearer review-secret") return send(401, { error: { code: "AUTHENTICATION_REQUIRED" } });
        if ((req.method === "GET" ? url.searchParams.get("workspaceId") : body?.workspaceId) !== ids.workspace) return send(403, { error: { code: "QUALITY_AUTHORITY_DENIED" } });
        if (req.method === "GET" && state.scenario === "quality-evidence-denied") return send(403, { error: { code: "QUALITY_AUTHORITY_DENIED" } });
      }
      const releasePath = `/api/v1/product-releases/${ids.release}`;
      // Token-controlled fault injection applies only to fixed read routes.
      const faultRoutes = {
        [`${workspace}/workbench`]: "summary", [`${workspace}/entity-match-reviews`]: "reviews",
        [`${workspace}/executions`]: "executions", [`${workspace}/attention/failed-executions`]: "failures",
        [`${workspace}/data-products`]: "products", [`${workspace}/attention/releases`]: "releases",
        [`${releasePath}/readiness`]: "readiness", [`${workspace}/quality-remediations/attention`]: "quality",
        [`${workspace}/source-schema-observations/attention`]: "schema", [`${workspace}/source-schema-observations/exceptions`]: "exceptions",
        [`${workspace}/source-schema-requests/unobserved`]: "requests",
        [`${workspace}/source-schema-requests/observed/unobserved-attempts`]: "attempts",
        [`/api/v1/quality-assessments/${ids.goldAssessment}/report`]: "quality-original-report",
        [`/api/v1/quality-assessments/${ids.qualityAssessment}/report`]: "quality-latest-report",
        [`${workspace}/agent-quality-context`]: "agent-context",
      };
      const pageFault = url.pathname === `${workspace}/product-releases` ? { FAILED: "failed-page", VALIDATING: "validating-page", READY: "ready-page" }[url.searchParams.get("status")] : undefined;
      const qualityFault = url.pathname === `/api/v1/dataset-versions/${ids.goldVersion}/quality-assessments`
        ? (url.searchParams.get("limit") === "1" ? "quality-latest-summary" : "quality-history")
        : /^\/api\/v1\/quality-assessment-attempts\//.test(url.pathname) ? "quality-attempt" : undefined;
      const fault = req.method === "GET" ? state.readFaults[qualityFault ?? pageFault ?? faultRoutes[url.pathname]] : undefined;
      if (fault === "fail" || fault === "denied" || fault === "notfound") return send(fault === "denied" ? 403 : fault === "notfound" ? 404 : 503, { code: "FIXTURE_READ_UNAVAILABLE", message: "private-upstream-sentinel" });
      if (fault === "invalid") return send(200, {});
      if (fault === "hold") await new Promise(resolve => heldReads.add(resolve));
      if(url.pathname.startsWith('/api/v1/traceability/product-releases/')&&req.method==='GET'){
        if(req.headers.authorization!=='Bearer review-secret')return send(401,{code:'AUTHENTICATION_REQUIRED'});
        if(url.searchParams.get('workspaceId')!==ids.workspace)return send(403,{code:'WORKSPACE_AUTHORITY_DENIED'});
        if(!state.scenario.startsWith('modern-trace')||url.pathname!==`/api/v1/traceability/product-releases/${ids.release}`)return send(404,{code:'PRODUCT_RELEASE_NOT_FOUND'});
        return send(200,{workspaceId:ids.workspace,releaseId:ids.release,releaseNo:'SYNTHETIC_TRACE',status:'PUBLISHED',productId:ids.product,productVersionId:ids.version,
          evidenceSnapshot:{id:ids.goldSnapshot,rootHash:'a'.repeat(64),integrityValid:true,createdAt:stamp,items:[{evidenceId:ids.secondEvidence,category:'TEST_EVIDENCE'}]},
          datasetVersions:[{id:ids.goldVersion,datasetId:ids.goldDataset,datasetCode:'SYNTHETIC_GOLD',datasetType:'CURATED',versionNo:1,status:'READY',releaseRole:'OUTPUT',...(state.scenario==='modern-trace-extra'?{storageUri:'private-trace-storage-sentinel'}:{})}],
          executions:[],entityMatchJobs:[],entityMappings:[{id:ids.candidate,decisionId:ids.decision,entityId:ids.entity,...(state.scenario==='modern-trace-extra'?{sourceKey:'private-trace-source-key'}:{})}],
          evidence:[{id:ids.secondEvidence,workspaceId:state.scenario==='modern-trace-foreign'?ids.product:ids.workspace,evidenceType:'TEST_EVIDENCE',hashValue:'b'.repeat(64),integrityValid:true}],
          costEvents:[{id:ids.deliveryOperation,workspaceId:ids.workspace,costType:'TEST_COST',quantity:1,unit:'attempt',amount:'0.000000000000000001',currency:'USD',pricingMode:'ACTUAL'}],
          auditEvents:[{id:ids.goldBinding,action:'PRODUCT_RELEASE_PUBLISHED',objectType:'PRODUCT_RELEASE',objectId:ids.release,occurredAt:stamp}]});
      }

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
      const createdRelease = {
        id: ids.createdRelease, productId: ids.product, productVersionId: ids.version, releaseNo: "R2",
        status: "DRAFT", createdAt: stamp,
        datasets: [{ datasetVersionId: ids.goldVersion, role: "OUTPUT" }],
        releaseNotes: "Browser-created release",
      };
      const goldDataset = {
        id: ids.goldDataset, workspaceId: ids.workspace, code: "GOLD_BROWSER_FIXTURE",
        name: state.scenario === "leader-demo-quality" ? "园区能耗质量演示数据集" : "Gold 浏览器验收数据集", description: "Synthetic Gold fixture", datasetType: "CURATED",
        sourceResourceId: ids.sourceResource, lifecycleStatus: "ACTIVE", currentVersionId: ids.goldVersion, createdAt: stamp, updatedAt: stamp,
      };
      const goldVersion = {
        id: ids.goldVersion, datasetId: ids.goldDataset, versionNo: 1, status: state.datasetVersionStatus,
        schemaVersion: "gold-v1", storageType: "OBJECT", storageUri: "s3://fixture/gold.csv",
        contentType: "text/csv", rowCount: 2, byteSize: 128, checksumAlgorithm: "SHA256",
        checksum: "1".repeat(64), generatedByExecutionId: ids.job, createdAt: stamp, readyAt: stamp,
        ...(state.datasetVersionStatus === "INVALID" ? { invalidatedAt: stamp } : {}),
        invalidationReason: state.invalidationReason,
      };
      const goldAssessment = {
        id: ids.goldAssessment, workspaceId: ids.workspace, datasetVersionId: ids.goldVersion,
        ruleSetRef: state.scenario === "leader-demo-quality" ? "park/quality/enterprise-activity-quality-v1.yaml" : "gold/quality/annotation-v1", ruleSetVersion: "1",
        ruleSetContentSha256: "2".repeat(64), evaluatorName: state.scenario === "leader-demo-quality" ? "synthetic-native" : "gold-quality", evaluatorVersion: "1",
        gateDecision: state.scenario === "leader-demo-quality" ? "FAIL" : "PASS",
        metrics: state.scenario === "leader-demo-quality" ? { synthetic: true, totalRows: 2, failedRows: 1 } : { formalAssessment: true, productionBindingId: ids.goldBinding, annotationSnapshotId: ids.goldSnapshot },
        dimensionSummary: state.scenario === "leader-demo-quality" ? {
          VALIDITY: { dimension: "VALIDITY", status: "FAIL", ruleCount: 1, evaluatedCount: 1, failedCount: 1 },
          TRACEABILITY: { dimension: "TRACEABILITY", status: "PASS", ruleCount: 1, evaluatedCount: 1, failedCount: 0 },
        } : {
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
        if (/^\/api\/v1\/workspaces\/[^/]+\/agent-quality-context$/.test(url.pathname)) {
          if (req.headers.authorization !== "Bearer review-secret") return send(401, { code: "AUTHENTICATION_REQUIRED" });
          if (url.pathname !== `${workspace}/agent-quality-context`) return send(403, { code: "QUALITY_AUTHORITY_DENIED" });
          const datasetId = url.searchParams.get("datasetId"), versionId = url.searchParams.get("versionId");
          if ((datasetId && datasetId !== ids.goldDataset) || (versionId && versionId !== ids.goldVersion)) return send(404, { code: "SOURCE_NOT_FOUND" });
          const { storageUri, ...safeVersion } = goldVersion;
          void storageUri;
          const context = {
            workspaceId: ids.workspace, readAt: stamp, mode: "READ_ONLY", executionReadiness: "NOT_EVALUATED",
            template: { configured: true, name: "浏览器测试模板", version: "1.0.0", hash: "a".repeat(64), workflowHash: "b".repeat(64), inputs: [
              { name: "company", datasetType: "RAW", required: true }, { name: "tax", datasetType: "RAW", required: true }, { name: "energy", datasetType: "RAW", required: true }, { name: "resolved", datasetType: "STANDARDIZED", required: true },
            ] }, unverified: ["ACCEPTED_BATCH_THREE_RAW", "MATCHING_ENTITY_RESOLUTION_OUTPUT", "OUTPUT_DATASET_BINDING", "CURRENT_EXECUTION_AUTHORITY"],
          };
          if (versionId) return send(200, { ...context, dataset: goldDataset, version: safeVersion });
          if (datasetId) return send(200, { ...context, dataset: goldDataset, versions: page([safeVersion]) });
          return send(200, { ...context, datasets: page([goldDataset]) });
        }
        if (url.pathname === `${workspace}/source-schema-requests/observed/unobserved-attempts`) {
          if(req.headers.authorization!=="Bearer rights-fixture-secret")return send(403,{});
          const all=state.scenario.startsWith("schema-attempts")&&state.scenario!=="schema-attempts-empty"?Array.from({length:105},(_,i)=>({...requestItem(107),attemptId:observedAttemptId(i+1),registeredAt:stamp,requestObservationCount:state.scenario==="schema-attempts-arrived"?32:31})).reverse().filter(a=>state.scenario!=="schema-attempts-arrived"||a.attemptId!==observedAttemptId(1)):[];
          if(state.scenario==="schema-attempts-foreign")all[0].workspaceId=ids.goldDataset;
          return send(200,{...page(all),readAt:stamp});
        }
        if (url.pathname === `${workspace}/source-schema-requests/unobserved`) {
          if(req.headers.authorization!=="Bearer rights-fixture-secret")return send(403,{});
          if(state.scenario==="schema-requests-unavailable")return send(503,{code:"SCHEMA_REQUESTS_UNAVAILABLE"});
          const all=state.scenario.startsWith("schema-requests")&&state.scenario!=="schema-requests-empty"?Array.from({length:105},(_,i)=>requestItem(i+1)).filter(r=>state.scenario!=="schema-requests-arrived"||r.requestId!==unobservedRequestId(105)):[];
          if(state.scenario==="schema-requests-foreign")all[0].workspaceId=ids.goldDataset;
          const view=url.searchParams.get("view")??"ALL";
          if(!["ALL","NO_ATTEMPT","ATTEMPT_RECORDED"].includes(view))return send(400,{});
          return send(200,{...page(all.filter(r=>view==="ALL"||(view==="NO_ATTEMPT"?r.attemptCount===0:r.attemptCount>0))),summary:{noAttemptCount:all.filter(r=>!r.attemptCount).length,attemptRecordedCount:all.filter(r=>r.attemptCount).length},readAt:stamp});
        }
        if (url.pathname.startsWith(`/api/v1/data-resources/${ids.sourceResource}/schema-requests/`)) {
          if(req.headers.authorization!=="Bearer rights-fixture-secret")return send(403,{});
          if(url.searchParams.get("workspaceId")!==ids.workspace)return send(404,{});
          const n=Array.from({length:107},(_,i)=>i+1).find(n=>url.pathname.endsWith(`/${unobservedRequestId(n)}`));
          if(!n)return send(404,{});
          const attempts=requestAttempts(n);
          if(n===105&&state.scenario==="schema-requests-arrived")attempts[0].observationId=requestObservationId(105);
          if(n===107&&state.scenario==="schema-attempts-arrived")attempts.find(a=>a.id===observedAttemptId(1)).observationId=observedObservationId(1);
          const selected=url.searchParams.get("attemptId"),chosen=attempts.find(a=>a.id===selected);
          if(chosen&&state.scenario==="schema-attempts-mismatch")chosen.requestId=unobservedRequestId(1);
          if(selected&&!chosen)return send(404,{});
          const observations=requestObservations(state.scenario).filter(o=>o.requestId===unobservedRequestId(n)).map(o=>({id:o.observationId,requestId:o.requestId,attemptId:o.attemptId,status:o.status,reason:o.reason,recordedAt:o.recordedAt}));
          const detailPage=(items,key)=>{const limit=Number(url.searchParams.get("limit")??25),offset=Number(url.searchParams.get(key)??0);return{items:items.slice(offset,offset+limit),page:{limit,offset,total:items.length}};};
          return send(200,{...requestItem(n),requestId:state.scenario==="schema-requests-mismatch"?unobservedRequestId(1):unobservedRequestId(n),bindingId:ids.goldBinding,externalId:"historical-request-provider-id",connectorHash:"d".repeat(64),baseline:[{name:"energy_kwh",type:"TEXT",constraint:"NOT_NULL"}],selectedAttempt:chosen??null,attempts:detailPage(attempts,"attemptOffset"),observations:detailPage(observations,"observationOffset"),readAt:stamp});
        }
        if (url.pathname === `${workspace}/source-schema-observations/exceptions`) {
          if (req.headers.authorization !== "Bearer rights-fixture-secret") return send(403, {});
          if (state.scenario === "schema-exceptions-unavailable") return send(503,{code:"SCHEMA_EXCEPTIONS_UNAVAILABLE"});
          const all=state.scenario.startsWith("schema-exceptions")&&state.scenario!=="schema-exceptions-empty"?exceptionItems():[];
          if(state.scenario==="schema-exceptions-foreign")all[0].workspaceId=ids.goldDataset;
          const status=url.searchParams.get("status")??"ALL";
          if(!["ALL","UNAVAILABLE","DENIED","INTERRUPTED"].includes(status))return send(400,{code:"INVALID_SCHEMA_EXCEPTION_STATUS"});
          const items=all.filter(o=>status==="ALL"||o.status===status);
          return send(200,{...page(items),summary:{unavailableCount:all.filter(o=>o.status==="UNAVAILABLE").length,deniedCount:all.filter(o=>o.status==="DENIED").length,interruptedCount:all.filter(o=>o.status==="INTERRUPTED").length},readAt:stamp});
        }
        if (url.pathname === `${workspace}/source-schema-observations/attention`) {
          if (req.headers.authorization !== "Bearer rights-fixture-secret") return send(403, {});
          if (state.scenario === "schema-attention-unavailable") return send(503,{code:"SCHEMA_ATTENTION_UNAVAILABLE"});
          const all=state.scenario.startsWith("schema-attention")?schemaItems():[];
          if(state.scenario==="schema-attention-foreign")all[0].workspaceId=ids.goldDataset;
          const items=all.filter(o=>url.searchParams.get("view")==="ALL"||o.handlingCount===0);
          return send(200,{...page(items),summary:{unhandledCount:all.filter(o=>!o.handlingCount).length,handledCount:all.filter(o=>o.handlingCount>0).length},readAt:stamp});
        }
        if (url.pathname.startsWith(`/api/v1/data-resources/${ids.sourceResource}/schema-observations/`)) {
          if(req.headers.authorization!=="Bearer rights-fixture-secret")return send(403,{});
          const item=[...schemaItems(),...exceptionItems(),...requestObservations(state.scenario)].find(o=>url.pathname.endsWith(`/${o.observationId}`));
          if(!item)return send(404,{});
          const exception=item.status!=="DRIFT";
          return send(200,{...item,requestId:state.scenario==="schema-attention-mismatch"?schemaRequestId(1):state.scenario==="schema-exceptions-mismatch"?exceptionRequestId(1):item.requestId,
            bindingId:ids.goldBinding,externalId:item.ruleRef==="fixture/schema-request"?"historical-request-provider-id":"historical-provider-id",connectorHash:"d".repeat(64),startedAt:exception?null:stamp,
            baseline:[{name:"energy_kwh",type:"TEXT",constraint:"NOT_NULL"}],columns:exception?[]:[{name:"energy_kwh",type:"BIGINT",constraint:"NULL"},{name:"new_optional",type:"TEXT",constraint:"NULL"}],handlingCount:item.handlingCount??0,
            handling:Array.from({length:item.handlingCount??0},(_,i)=>({id:schemaObservationId(201+i),observationId:item.observationId,actorId:ids.actor,reason:`历史处理 ${i+1}`,createdAt:stamp}))});
        }
        if (url.pathname === `${workspace}/quality-remediations/attention`) {
          if (req.headers.authorization !== "Bearer review-secret") return send(403, {});
          if (state.scenario === "quality-attention-unavailable") return send(503, { code: "QUALITY_ATTENTION_UNAVAILABLE" });
          const all = state.scenario.startsWith("quality-attention") ? attentionItems() : [];
          const view = url.searchParams.get("view") ?? "FOLLOW_UP";
          const items = all.filter(i => view === "ALL" || (view === "CLOSED" ? i.manuallyClosed : i.needsFollowUp));
          return send(200, { ...page(items), summary: { followUpCount: all.filter(i => i.needsFollowUp).length, closedCount: all.filter(i => i.manuallyClosed).length }, readAt: stamp });
        }
        if (url.pathname.startsWith(`${workspace}/quality-remediations/`)) {
          if (req.headers.authorization !== "Bearer review-secret") return send(403, {});
          const item = attentionItems().find(i => url.pathname.endsWith(`/${i.id}`));
          if (!item) return send(404, {});
          return send(200, {
            case: { id: item.id, workspace_id: ids.workspace, version_id: state.scenario === "quality-attention-foreign" ? ids.version : ids.goldVersion, assessment_id: ids.goldAssessment, finding_id: ids.goldBinding, detection_rule_id: "negative-energy", plan_ref: "fixture", plan_version: "1", workflow_hash: "a".repeat(64) },
            finding: { message: item.message, observed: { affectedCount: 1, total: 10 } },
            assignments: [], attempts: [], rechecks: [], closure: item.manuallyClosed ? { reason: "fixture closure" } : null,
          });
        }
        if (url.pathname === `/api/v1/dataset-versions/${ids.qualityOutputVersion}`) return send(200, { ...goldVersion, id: ids.qualityOutputVersion, versionNo: 2 });
        if (url.pathname === `/api/v1/dataset-versions/${ids.qualityOutputVersion}/quality-assessments`) return send(200, { datasetVersionId: ids.qualityOutputVersion, ...page([{ ...goldAssessment, id: ids.recheckAssessment, datasetVersionId: ids.qualityOutputVersion }]) });
        if (url.pathname === `${workspace}/dataset-versions/${ids.qualityOutputVersion}/certifications`) return send(200, { workspaceId: ids.workspace, datasetVersionId: ids.qualityOutputVersion, asOf: stamp, ...page([]), profiles: [] });
        if (url.pathname === `/api/v1/quality-assessments/${ids.recheckAssessment}/report`) return send(200, {
          ...goldAssessment, id: ids.recheckAssessment, datasetVersionId: ids.qualityOutputVersion,
          findings: { ...page([]) }, evidence: [], auditEvents: [],
        });
        if (url.pathname === `/api/v1/ingestions/${ids.ingestion}`) {
          if (req.headers.authorization !== "Bearer rights-fixture-secret") return send(401, {});
          return send(200, {id:ids.ingestion,state:state.batchDispatched?"RUNNING":state.scenario==="batch-registered"?"REGISTERED":"REGISTERING",attemptId:ids.job,remoteId:state.scenario==="batch-registered"?"known-remote-run":"",request:{workspaceId:ids.workspace,batchKey:"Recoverable batch",connectionRef:"fixture-source",instanceRef:"fixture-instance",templateHash:"a".repeat(64),schemaHash:"b".repeat(64),slots:[]}});
        }
        if (url.pathname === `${workspace}/certification-profiles`) {
          if (req.headers.authorization !== "Bearer rights-fixture-secret") return send(401, {});
          return send(200, { items: [goldProfile, { ...goldProfile, id: ids.secondProfile, name: "Second profile" }] });
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/certification-inputs`) {
          if (req.headers.authorization !== "Bearer rights-fixture-secret") return send(401, {});
          const profile = url.searchParams.get("profileId");
          if (profile === ids.secondProfile && !state.secondProfilePrepared) return send(200, {});
          return send(200, { evidenceSnapshotId: profile === ids.secondProfile ? ids.secondEvidence : ids.goldSnapshot });
        }
        if (url.pathname === `${workspace}/data-products`) return send(200, page([product]));
        if (url.pathname === `/api/v1/data-products/${ids.product}`) return send(200, product);
        if (url.pathname === `${workspace}/product-releases`) {
          const status = url.searchParams.get("status") ?? "";
          const available = [release, ...(state.releaseCreated ? [createdRelease] : [])];
          const items = status ? available.filter((item) => item.status === status) : available;
          return send(200, {workspaceId:ids.workspace,...page(items)});
        }
        if (url.pathname === `${workspace}/attention/releases`) {
          const limit = Math.max(1, Number(url.searchParams.get("limit")) || 10);
          const group = (status) => {
            const items = state.releaseStatus === status ? [release] : [];
            return { items: items.slice(0, limit), page: { total: items.length, limit, offset: 0 } };
          };
          return send(200, {
            failed: group("FAILED"),
            validating: group("VALIDATING"),
            ready: group("READY"),
          });
        }
        if (url.pathname === `${workspace}/data-products/${ids.product}/releases`) {
          if (state.scenario === "paginated-releases") return send(200, page(fixtureReleases(30)));
          return send(200, page([release, ...(state.releaseCreated ? [createdRelease] : [])]));
        }
        if (state.scenario === "paginated-releases") {
          const releaseMatch = url.pathname.match(/^\/api\/v1\/product-releases\/([0-9a-f-]+)$/);
          if (releaseMatch) {
            const item = fixtureReleases(30).find((candidate) => candidate.id === releaseMatch[1]);
            if (item) return send(200, item);
          }
          const readinessMatch = url.pathname.match(/^\/api\/v1\/product-releases\/([0-9a-f-]+)\/readiness$/);
          if (readinessMatch) {
            const item = fixtureReleases(30).find((candidate) => candidate.id === readinessMatch[1]);
            if (item) return send(200, { ...readiness(state.scenario), releaseId: item.id });
          }
        }
        if (url.pathname === releasePath) return send(200, release);
        if (url.pathname === `${releasePath}/readiness`) return send(200, readiness(state.scenario));
        if (url.pathname === `/api/v1/product-releases/${ids.createdRelease}` && state.releaseCreated) return send(200, createdRelease);
        if (url.pathname === `/api/v1/product-releases/${ids.createdRelease}/readiness` && state.releaseCreated) {
          return send(200, { releaseId: ids.createdRelease, overall: "NOT_READY", checks: { ...pass, rights: "PENDING", quality: "PENDING", compliance: "PENDING", contract: "PENDING", evidence: "PENDING" }, blockers: ["rights", "quality", "compliance", "contract", "evidence"], details: {} });
        }
        if (url.pathname === `/api/v1/product-versions/${ids.version}`) return send(200, {
          id: ids.version, productId: ids.product, version: "1.0.0", definition: {},
          assets: [{ id: ids.productAsset, assetType: "DATASET", name: "Gold output", datasetId: ids.goldDataset, externalRef: "", deliveryConfig: {}, schemaSnapshot: {} }],
        });
        if (url.pathname === `${workspace}/entity-match-reviews`) return send(200, page(state.candidateStatus === "PENDING" ? [candidate] : []));
        if (url.pathname === `/api/v1/entity-match-jobs/${ids.job}`) return send(200, job);
        if (url.pathname === `/api/v1/entity-match-reviews/${ids.candidate}`) return send(200, candidate);
        if (url.pathname === `/api/v1/entity-match-jobs/${ids.job}/reviews`) return send(200, { items: [candidate] });
        if (url.pathname === `${workspace}/data-resources`) return send(200, page(state.scenario === "paginated-resources" ? fixtureResources(30) : []));
        if (url.pathname === `${workspace}/datasets/${ids.goldDataset}`) return send(200, goldDataset);
        if (url.pathname === `${workspace}/datasets/${ids.goldDataset}/versions`) return send(200, page([goldVersion]));
        if (url.pathname === `/api/v1/dataset-versions/${ids.goldVersion}`) return send(200, goldVersion);
        if (url.pathname === `/api/v1/dataset-versions/${ids.goldVersion}/quality-assessments`) {
          const generatedAssessment = {
            ...goldAssessment,
            id: ids.qualityAssessment,
            ruleSetRef: "park/quality/enterprise-activity-quality-v1.yaml",
            createdAt: "2026-09-29T08:00:00Z",
          };
          const items = state.scenario === "leader-demo-quality"
            ? [goldAssessment, { ...goldAssessment, id: ids.qualityAssessment, gateDecision: "PASS", createdAt: stamp, metrics: { synthetic: true, totalRows: 2, failedRows: 0 }, dimensionSummary: { VALIDITY: { dimension: "VALIDITY", status: "PASS", ruleCount: 1, evaluatedCount: 1, failedCount: 0 } } }]
            : state.scenario === "agent-quality-pages"
            ? [generatedAssessment, ...Array.from({length:24},(_,i)=>({...goldAssessment,id:`a1100003-a101-4101-8101-${String(i+1).padStart(12,"0")}`,ruleSetRef:`fixture/history-${i+1}`})), goldAssessment]
            : state.qualityCompleted ? [generatedAssessment, goldAssessment] : [goldAssessment];
          return send(200, { datasetVersionId: ids.goldVersion, ...page(items) });
        }
        const qualityAttemptMatch = url.pathname.match(/^\/api\/v1\/quality-assessment-attempts\/([0-9a-f-]+)$/);
        if (qualityAttemptMatch) {
          const selectedAttempt = state.scenario === "agent-quality-pages" ? ids.job : state.qualityAttemptId;
          if (!selectedAttempt || qualityAttemptMatch[1] !== selectedAttempt) {
            return send(404, { error: { code: "QUALITY_ASSESSMENT_ATTEMPT_NOT_FOUND" } });
          }
          return send(200, {
            id: selectedAttempt,
            workspaceId: ids.workspace,
            datasetVersionId: ids.goldVersion,
            ruleSetRef: "park/quality/enterprise-activity-quality-v1.yaml",
            engineName: "native",
            engineVersion: "1",
            outcome: state.scenario === "agent-quality-pages" || state.qualityCompleted ? "SUCCEEDED" : "IN_PROGRESS",
            ...(state.scenario === "agent-quality-pages" ? { assessmentId: ids.goldAssessment } : state.qualityCompleted ? { assessmentId: ids.qualityAssessment } : {}),
            leaseExpiresAt: "2026-09-29T09:00:00Z",
            leaseExpired: false,
          });
        }
        const complianceAttemptMatch = url.pathname.match(/^\/api\/v1\/compliance-assessment-attempts\/([0-9a-f-]+)$/);
        if (complianceAttemptMatch) {
          if (!state.complianceAttemptId || complianceAttemptMatch[1] !== state.complianceAttemptId) {
            return send(404, { error: { code: "COMPLIANCE_ASSESSMENT_ATTEMPT_NOT_FOUND" } });
          }
          return send(200, {
            id: ids.complianceResult,
            assessmentAttemptId: state.complianceAttemptId,
            workspaceId: ids.workspace,
            datasetVersionId: ids.goldVersion,
            policyRef: state.compliancePolicyRef || "park/compliance/enterprise-activity-compliance-v1.yaml",
            policyVersion: "1.0.0",
            gateDecision: "PASS",
            summary: { piiFields: 0 },
            findings: [],
            createdAt: "2026-09-29T08:30:00Z",
          });
        }
        if (url.pathname === `/api/v1/quality-assessments/${ids.qualityAssessment}/report` && state.scenario === "leader-demo-quality") {
          return send(200, { ...goldAssessment, id: ids.qualityAssessment, gateDecision: "PASS", createdAt: stamp, metrics: { synthetic: true, totalRows: 2, failedRows: 0 }, dimensionSummary: { VALIDITY: { dimension: "VALIDITY", status: "PASS", ruleCount: 1, evaluatedCount: 1, failedCount: 0 } }, findings: page([]), evidence: [], auditEvents: [] });
        }
        if (url.pathname === `/api/v1/quality-assessments/${ids.qualityAssessment}/report` && state.qualityCompleted) {
          return send(200, {
            ...goldAssessment,
            id: ids.qualityAssessment,
            ruleSetRef: "park/quality/enterprise-activity-quality-v1.yaml",
            createdAt: "2026-09-29T08:00:00Z",
            findings: page([]),
            evidence: [],
            auditEvents: [],
          });
        }
        if (url.pathname === `/api/v1/quality-assessments/${ids.goldAssessment}/report`) {
          const findings = state.scenario === "leader-demo-quality" ? [
            { id: ids.goldBinding, resultId: ids.goldAssessment, ruleId: "negative-energy", dimension: "VALIDITY", severity: "ERROR", status: "FAIL", reason: "合成能耗记录为负值，须人工审阅修复范围；本预览未执行修复", createdAt: stamp },
            { id: "a1100004-a101-4101-8101-000000000002", resultId: ids.goldAssessment, ruleId: "source-reference", dimension: "TRACEABILITY", severity: "INFO", status: "PASS", reason: "合成记录保留来源引用；不代表真实原件核验", createdAt: stamp },
          ] : state.scenario === "agent-quality-pages" ? Array.from({length:26},(_,i)=>({
            id:`a1100004-a101-4101-8101-${String(i+1).padStart(12,"0")}`,resultId:ids.goldAssessment,
            ruleId:`FIXTURE-${String(i+1).padStart(2,"0")}`,dimension:"COMPLETENESS",severity:"INFO",status:"PASS",reason:`分页验收记录 ${i+1}`,createdAt:stamp,
          })) : [];
          return send(200, {
            ...goldAssessment,
            ...(state.scenario === "quality-attention-foreign" ? { datasetVersionId: ids.version } : {}),
            findings: page(findings),
            evidence: [{ id: "78787878-7878-4878-8878-787878787878", workspaceId: ids.workspace, evidenceType: "GOLD_QUALITY_ASSESSMENT", relationType: "ASSESSMENT_EVIDENCE", sourceType: "QUALITY_ASSESSMENT", sourceId: ids.goldAssessment, hashAlgorithm: "SHA256", hashValue: "a".repeat(64), createdAt: stamp }],
            auditEvents: [],
          });
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/gold-explanation`) {
          return send(200, goldExplanation);
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/certifications`) {
          const limit = Math.max(1, Number(url.searchParams.get("limit")) || 25);
          const offset = Math.max(0, Number(url.searchParams.get("offset")) || 0);
          const items = [goldCertification].slice(offset, offset + limit);
          return send(200, {
            workspaceId: ids.workspace,
            datasetVersionId: ids.goldVersion,
            asOf: stamp,
            items,
            profiles: [goldProfile],
            page: { total: 1, limit, offset, anchorRevision: 0 },
          });
        }
        if (url.pathname === `${workspace}/direct-data-deliveries/recovery`) {
          if (req.headers["authorization"] !== "Bearer delivery-secret") return send(401, { error: { code: "CALLER_IDENTITY_UNTRUSTED" } });
          if (url.searchParams.get("consumer") !== "GOLD-PILOT-CONSUMER") return send(403, { error: { code: "CONSUMER_PRINCIPAL_MISMATCH" } });
          const attempt = state.deliveryAttempts[url.searchParams.get("idempotencyKey") ?? ""];
          if (!attempt) return send(404, { error: { code: "DIRECT_DATA_DELIVERY_NOT_FOUND" } });
          return send(200, attempt);
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/delivery-eligibility`) {
          return send(200, {
            allowed: true, blockers: [],
            datasetVersion: { status: "READY", allowed: true, blockers: [] },
            certification: { allowed: true, blockers: [], current: goldCertification },
            entitlement: {
              allowed: true, blockers: [],
              checks: [{ dataResourceId: ids.sourceResource, path: "DIRECT_USE", decision: "ALLOWED", reason: "fixture verified rights" }],
            },
          });
        }
        if (url.pathname === `${workspace}/datasets`) return send(200, page([goldDataset]));
        if (url.pathname === `${workspace}/executions`) return send(200, page([]));
        if (url.pathname === `/api/v1/executions/${ids.job}`) {
          return send(200, {
            id: ids.job,
            workspaceId: ids.workspace,
            workflowVersionId: "42424242-4242-4242-8242-424242424242",
            outputDatasetId: ids.goldDataset,
            outputDatasetVersionId: ids.goldVersion,
            targetPeriod: "2026-09",
            status: "FAILED",
            attempt: 2,
            retryOfExecutionId: ids.retryParent,
            engineType: "NATIVE",
            engineExecutionId: "fixture-engine-execution",
            errorCode: "FIXTURE_EXECUTION_FAILED",
            errorMessage: "fixture failure",
            metrics: { rowsRead: 2 },
            inputs: [{ name: "source", datasetId: ids.goldDataset, datasetVersionId: ids.goldVersion }],
            createdAt: stamp,
            startedAt: stamp,
            finishedAt: stamp,
          });
        }
        if (url.pathname === `/api/v1/executions/${ids.retryChild}` && state.retryCreated) {
          return send(200, {
            id: ids.retryChild,
            workspaceId: ids.workspace,
            workflowVersionId: "42424242-4242-4242-8242-424242424242",
            outputDatasetId: ids.goldDataset,
            targetPeriod: "2026-09",
            status: "QUEUED",
            attempt: 3,
            retryOfExecutionId: ids.job,
            engineType: "NATIVE",
            engineExecutionId: "",
            errorCode: "",
            errorMessage: "",
            metrics: {},
            inputs: [{ name: "source", datasetId: ids.goldDataset, datasetVersionId: ids.goldVersion }],
            createdAt: stamp,
          });
        }
        if (url.pathname === `${workspace}/attention/failed-executions`) return send(200, page([]));
        if (url.pathname === `${workspace}/workbench`) return send(200, {
          workspaceId: ids.workspace, counts: { dataResources: 0, datasets: 0, dataProducts: 1 },
          reviewQueue: { pending: state.candidateStatus === "PENDING" ? 1 : 0, unresolved: 0, conflicts: 0 },
          executions: { queued: 0, submitting: 0, running: 0, succeeded: 0, failed: 0 },
          releases: { draft: 0, ready: state.releaseStatus === "READY" ? 1 : 0, published: state.releaseStatus === "PUBLISHED" ? 1 : 0, failed: 0 }, updatedAt: stamp,
        });
      }
      if (req.method === "POST") {
        if (url.pathname === `/api/v1/ingestions/${ids.ingestion}/dispatch`) {
          if (req.headers.authorization !== "Bearer rights-fixture-secret") return send(401, {});
          if(state.scenario!=="batch-registered"||state.batchDispatched)return send(409,{code:"INVALID_STATE"});
          state.batchDispatched=true;return send(200,{id:ids.ingestion,state:"RUNNING"});
        }
        if (url.pathname === `/api/v1/ingestions/${ids.ingestion}/recover`) {
          if (req.headers.authorization !== "Bearer rights-fixture-secret") return send(401, {});
          return send(503,{code:"OUTCOME_UNKNOWN"});
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/certification-inputs`) {
          if (req.headers.authorization !== "Bearer rights-fixture-secret") return send(401, {});
          if (body.profileId !== ids.secondProfile || body.qualityAssessmentId !== ids.goldAssessment) return send(400, {code:"PROFILE_INPUT_MISMATCH"});
          state.secondProfilePrepared = true;
          return send(200, { evidenceSnapshotId: ids.secondEvidence });
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/certifications`) {
          if (req.headers.authorization !== "Bearer rights-fixture-secret") return send(401, {});
          if (body.profileId !== ids.secondProfile || body.evidenceSnapshotId !== ids.secondEvidence) return send(400, {code:"PROFILE_EVIDENCE_MISMATCH"});
          return send(200, {id: ids.goldCertification, decision:"CERTIFIED", blockers:[]});
        }
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
        if (url.pathname === `/api/v1/dataset-versions/${ids.goldVersion}/quality-checks`) {
          if (req.headers.authorization !== "Bearer review-secret") return send(401, { error: { code: "AUTHENTICATION_REQUIRED" } });
          if (body.workspaceId !== ids.workspace) return send(400, { error: { code: "DATASET_WORKSPACE_MISMATCH" } });
          if (typeof body.assessmentAttemptId !== "string" || !body.assessmentAttemptId) return send(400, { error: { code: "MISSING_ASSESSMENT_ATTEMPT_ID" } });
          if (body.ruleSetRef !== "park/quality/enterprise-activity-quality-v1.yaml") return send(400, { error: { code: "INVALID_RULE_SET" } });
          if (state.qualityAttemptId && state.qualityAttemptId !== body.assessmentAttemptId) return send(409, { error: { code: "QUALITY_ASSESSMENT_ATTEMPT_CONFLICT" } });
          state.qualityAttemptId = body.assessmentAttemptId;
          state.qualityCompleted = true;
          return send(201, {
            ...goldAssessment,
            id: ids.qualityAssessment,
            ruleSetRef: body.ruleSetRef,
            createdAt: "2026-09-29T08:00:00Z",
          });
        }
        if (url.pathname === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`) {
          if (req.headers["x-actor-id"] !== ids.actor) return send(400, { error: { code: "FIXTURE_ACTOR_REQUIRED" } });
          if (body.workspaceId !== ids.workspace) return send(400, { error: { code: "DATASET_WORKSPACE_MISMATCH" } });
          if (typeof body.assessmentAttemptId !== "string" || !body.assessmentAttemptId) return send(400, { error: { code: "MISSING_ASSESSMENT_ATTEMPT_ID" } });
          if (typeof body.policyRef !== "string" || !body.policyRef.trim()) return send(400, { error: { code: "INVALID_POLICY_REF" } });
          if (state.complianceAttemptId && state.complianceAttemptId !== body.assessmentAttemptId) return send(409, { error: { code: "COMPLIANCE_ASSESSMENT_ATTEMPT_CONFLICT" } });
          state.complianceAttemptId = body.assessmentAttemptId;
          state.compliancePolicyRef = body.policyRef;
          state.complianceCompleted = true;
          return send(201, {
            id: ids.complianceResult,
            assessmentAttemptId: state.complianceAttemptId,
            workspaceId: ids.workspace,
            datasetVersionId: ids.goldVersion,
            policyRef: state.compliancePolicyRef,
            policyVersion: "1.0.0",
            gateDecision: "PASS",
            summary: { piiFields: 0 },
            findings: [],
            createdAt: "2026-09-29T08:30:00Z",
          });
        }
        if (url.pathname === `${workspace}/dataset-versions/${ids.goldVersion}/deliveries`) {
          if (req.headers["authorization"] !== "Bearer delivery-secret") return send(401, { error: { code: "CALLER_IDENTITY_UNTRUSTED" } });
          const key = req.headers["idempotency-key"];
          if (typeof key !== "string" || !key) return send(400, { error: { code: "INVALID_IDEMPOTENCY_KEY" } });
          if (
            body.profileId !== ids.goldProfile
            || body.consumer?.trim() !== "GOLD-PILOT-CONSUMER"
            || body.purpose?.trim().toUpperCase() !== "GOLD-PILOT"
            || body.action?.trim().toUpperCase() !== "USE"
            || body.scopeType?.trim().toUpperCase() !== "ALL_RESOURCE"
          ) {
            return send(400, { error: { code: "INVALID_DELIVERY_FIXTURE" } });
          }
          const existing = state.deliveryAttempts[key];
          if (existing) return send(409, { code: "DIRECT_DATA_REPLAY_REQUIRES_NEW_ATTEMPT", operationId: existing.operationId, status: existing.status, message: "use a new attempt" });
          if (body.retryOfDeliveryOperationId) {
            const prior = Object.values(state.deliveryAttempts).find((item) => item.operationId === body.retryOfDeliveryOperationId);
            if (!prior || prior.status !== "ISSUED" || prior.profileId !== body.profileId) return send(400, { error: { code: "INVALID_RETRY_OF_DELIVERY_OPERATION_ID" } });
          }
          state.deliveryCount += 1;
          const operationId = state.deliveryCount === 1 ? ids.deliveryOperation : ids.deliveryRetryOperation;
          state.deliveryAttempts[key] = {
            operationId,
            profileId: ids.goldProfile,
            workspaceId: ids.workspace,
            datasetVersionId: ids.goldVersion,
            ...(body.retryOfDeliveryOperationId ? { retryOfDeliveryOperationId: body.retryOfDeliveryOperationId } : {}),
            idempotencyKey: key,
            status: "ISSUED",
            gateDecision: "ALLOWED",
            principalRef: "browser-principal",
            consumer: "GOLD-PILOT-CONSUMER",
            purpose: "GOLD-PILOT",
            action: "USE",
            scopeRef: ids.goldVersion,
            deliveryChannel: "DIRECT_DATA",
            deliveryMode: "DIRECT_DATA",
            terminalReason: "",
            createdAt: stamp,
            updatedAt: stamp,
          };
          return sendBytes(200, "company_id,score\nCOMPANY-001,88\n", {
            "X-Delivery-Operation-Id": operationId,
            "Content-Disposition": "attachment; filename=fixture.csv",
          });
        }
        if (url.pathname === `/api/v1/data-products/${ids.product}/releases`) {
          if (req.headers["x-actor-id"] !== ids.actor) return send(400, { error: { code: "FIXTURE_ACTOR_REQUIRED" } });
          if (body.productVersionId !== ids.version || body.releaseNo !== "R2") return send(400, { error: { code: "INVALID_RELEASE_FIXTURE" } });
          if (!Array.isArray(body.datasets) || body.datasets.length !== 1 || body.datasets[0].datasetVersionId !== ids.goldVersion || body.datasets[0].role !== "OUTPUT") {
            return send(400, { error: { code: "INVALID_RELEASE_DATASETS" } });
          }
          if (state.releaseCreated) return send(400, { error: { code: "PRODUCT_RELEASE_CREATE_FAILED" } });
          state.releaseCreated = true;
          return send(201, createdRelease);
        }
        if (url.pathname === `/api/v1/dataset-versions/${ids.goldVersion}/invalidate`) {
          if (req.headers["x-actor-id"] !== ids.actor) return send(400, { error: { code: "FIXTURE_ACTOR_REQUIRED" } });
          if (state.datasetVersionStatus !== "READY") return send(409, { error: { code: "DATASET_VERSION_INVALID_TRANSITION" } });
          if (typeof body.reason !== "string" || !body.reason.trim()) return send(400, { error: { code: "INVALID_REASON" } });
          state.datasetVersionStatus = "INVALID";
          state.invalidationReason = body.reason.trim();
          return send(200, {
            ...goldVersion,
            status: "INVALID",
            invalidatedAt: stamp,
            invalidationReason: state.invalidationReason,
          });
        }
        if (url.pathname === `/api/v1/executions/${ids.job}/retry`) {
          if (req.headers["x-actor-id"] !== ids.actor) return send(400, { error: { code: "FIXTURE_ACTOR_REQUIRED" } });
          if (!req.headers["idempotency-key"]) return send(400, { error: { code: "FIXTURE_KEY_REQUIRED" } });
          state.retryCreated = true;
          return send(202, {
            id: ids.retryChild,
            workspaceId: ids.workspace,
            workflowVersionId: "42424242-4242-4242-8242-424242424242",
            outputDatasetId: ids.goldDataset,
            targetPeriod: "2026-09",
            status: "QUEUED",
            attempt: 3,
            retryOfExecutionId: ids.job,
            engineType: "NATIVE",
            metrics: {},
            inputs: [{ name: "source", datasetVersionId: ids.goldVersion }],
            createdAt: stamp,
          });
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
  const tokenPath = fileURLToPath(new URL("../../.data/browser-fixture-rights-token", import.meta.url));
  mkdirSync(dirname(tokenPath), { recursive: true });
  writeFileSync(tokenPath, "rights-fixture-secret", {mode: 0o600}); // synthetic test token only
  const server = createFixtureServer();
  server.listen(4400, "127.0.0.1", () => console.log("Test-only Core fixture on 127.0.0.1:4400"));
  process.on("SIGTERM", () => server.close(() => process.exit(0)));
}
