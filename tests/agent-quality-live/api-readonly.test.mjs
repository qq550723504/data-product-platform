import test from "node:test";
import assert from "node:assert/strict";
const check = process.env.AGENT_Q1_LIVE_ACK === "ISOLATED_READ_ONLY" ? test : test.skip;
const base = "http://127.0.0.1:18496";
const ws = "11111111-1111-4111-8111-111111111111";
const foreign = "12121212-1212-4212-8212-121212121212";
const dataset = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const version = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
const historical = "cccccccc-cccc-4ccc-8ccc-cccccccccccc";
const context = `/api/v1/workspaces/${ws}/agent-quality-context`;
const headers = { Authorization: "Bearer agent-q1-live-secret" };
async function get(path, expected = 200, supplied = headers) {
  const response = await fetch(base + path, { headers: supplied, redirect: "error", signal: AbortSignal.timeout(10000) });
  assert.equal(response.status, expected, `${path}: ${response.status}`);
  const body = await response.json();
  assert.ok(!JSON.stringify(body).includes("agent-q1-live-secret"));
  return body;
}
check("Core is running with the synthetic read fixture", async () => {
  const c = await get(`${context}?versionId=${version}`);
  assert.equal(c.dataset.code, "AGENT_Q1_CORE"); assert.equal(c.version.id, version);
  assert.equal(c.mode, "READ_ONLY"); assert.equal(c.executionReadiness, "NOT_EVALUATED");
  assert.equal(c.template.name, "energy-quarantine"); assert.equal(c.template.inputs.length, 4);
  assert.ok(!JSON.stringify(c).includes("storageUri")); assert.ok(!JSON.stringify(c).includes("q1-storage-private"));
});
check("missing and forged credentials are rejected", async () => {
  await get(context, 401, {});
  await get(context, 401, { Authorization: "Bearer wrong-fixture-token" });
});
check("principal cannot choose another workspace", async () => {
  await get(`/api/v1/workspaces/${foreign}/agent-quality-context`, 403);
});
check("foreign version and mismatched dataset never expose source facts", async () => {
  await get(`${context}?versionId=a2100002-a101-4101-8101-000000000003`, 404);
  await get(`${context}?versionId=${version}&datasetId=a2100001-a101-4101-8101-000000000001`, 404);
});
check("history page is bounded and retains the exact old assessment", async () => {
  const h = await get(`/api/v1/dataset-versions/${version}/quality-assessments?workspaceId=${ws}&summary=true&limit=25&offset=25`);
  assert.equal(h.page.total, 26); assert.equal(h.items.length, 1); assert.equal(h.items[0].id, historical);
});
check("historical FAIL and findings/evidence/audit survive a newer PASS", async () => {
  const h = await get(`/api/v1/dataset-versions/${version}/quality-assessments?workspaceId=${ws}&summary=true&limit=1&offset=0`);
  assert.equal(h.items[0].gateDecision, "PASS"); assert.notEqual(h.items[0].id, historical);
  const r = await get(`/api/v1/quality-assessments/${historical}/report?workspaceId=${ws}&limit=25&offset=25`);
  assert.equal(r.id, historical); assert.equal(r.datasetVersionId, version); assert.equal(r.gateDecision, "FAIL");
  assert.equal(r.findings.page.total, 26); assert.equal(r.findings.items.length, 1); assert.equal(r.findings.items[0].ruleId, "Q1-SYNTHETIC-26");
  assert.equal(r.evidence.length, 1); assert.equal(r.auditEvents.length, 1);
  assert.equal(r.evidence[0].hashValue, "c390e595f3ef3a813c40006c1395e91c4a229d7168b5a4bb3c3252ca66ee4a44");
});
check("attempt lookup only returns its historical receipt", async () => {
  const a = await get(`/api/v1/quality-assessment-attempts/66666666-6666-4666-8666-666666666666?workspaceId=${ws}`);
  assert.equal(a.outcome, "SUCCEEDED"); assert.equal(a.assessmentId, historical); assert.equal(a.datasetVersionId, version);
});
check("report of another version is identifiable rather than silently rebound", async () => {
  const r = await get(`/api/v1/quality-assessments/a2100003-a101-4101-8101-000000000101/report?workspaceId=${ws}&limit=25&offset=0`);
  assert.equal(r.datasetVersionId, "a2100002-a101-4101-8101-000000000002");
});
check("missing historical report does not return latest", async () => {
  await get(`/api/v1/quality-assessments/99999999-9999-4999-8999-999999999999/report?workspaceId=${ws}`, 404);
});
check("Quality report also enforces the principal workspace", async () => {
  await get(`/api/v1/quality-assessments/${historical}/report?workspaceId=${foreign}`, 403);
});
