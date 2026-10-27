import test, { before, after, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { createFixtureServer, ids, fixtureToken, exceptionObservationId, exceptionRequestId, unobservedRequestId, requestAttemptId, requestObservationId, observedRequestId, observedAttemptId, observedObservationId } from "./fixture-server.mjs";
let server, base;
const controlHeaders = { "x-fixture-token": fixtureToken, "Content-Type": "application/json" };
before(async () => {
  server = createFixtureServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  base = `http://127.0.0.1:${server.address().port}`;
});
after(async () => { await new Promise((resolve) => server.close(resolve)); });
beforeEach(async () => { await fetch(`${base}/__control/reset`, { method: "POST", headers: controlHeaders, body: "{}" }); });

test("Quality fault injection rejects one exact read and preserves the other authenticated report", async () => {
  const response = await fetch(`${base}/__control/reset`, { method: "POST", headers: controlHeaders, body: JSON.stringify({ scenario: "quality-attention", readFaults: { "quality-original-report": "notfound" } }) });
  assert.equal(response.status, 200);
  const headers = { Authorization: "Bearer review-secret" };
  const exact = `${base}/api/v1/quality-assessments/${ids.goldAssessment}/report?workspaceId=${ids.workspace}`;
  const latest = `${base}/api/v1/quality-assessments/${ids.qualityAssessment}/report?workspaceId=${ids.workspace}`;
  assert.equal((await fetch(exact, { headers })).status, 404);
  assert.equal((await fetch(latest, { headers })).status, 200);
  assert.equal((await fetch(latest)).status, 401);
});
async function state() { return (await fetch(`${base}/__control/state`, { headers: controlHeaders })).json(); }

test("Agent context fixture is scoped, authenticated and records only GET queries", async () => {
  const path = `/api/v1/workspaces/${ids.workspace}/agent-quality-context?limit=25&offset=0`;
  assert.equal((await fetch(base + path)).status, 401);
  const headers = { Authorization: "Bearer review-secret" };
  const catalog = await (await fetch(base + path, { headers })).json();
  assert.equal(catalog.datasets.page.total, 1);
  const context = await (await fetch(base + path + `&versionId=${ids.goldVersion}`, { headers })).json();
  assert.equal(context.version.id, ids.goldVersion);
  assert.equal(context.executionReadiness, "NOT_EVALUATED");
  assert.equal(context.version.storageUri, undefined);
  assert.equal((await fetch(base + path + `&versionId=${ids.version}`, { headers })).status, 404);
  assert.equal((await fetch(base + path.replace(ids.workspace, ids.actor), { headers })).status, 403);
  assert.ok((await state()).requests.every(r => r.method === "GET"));
});

test("Agent acceptance fixture has independent history/finding pages and an exact recoverable attempt", async () => {
  await fetch(`${base}/__control/reset`, { method: "POST", headers: controlHeaders, body: JSON.stringify({scenario:"agent-quality-pages"}) });
  const headers={ Authorization:"Bearer review-secret" };
  const history=await(await fetch(`${base}/api/v1/dataset-versions/${ids.goldVersion}/quality-assessments?workspaceId=${ids.workspace}&limit=25&offset=25`,{headers})).json();
  assert.equal(history.page.total,26);assert.equal(history.items.length,1);assert.equal(history.items[0].id,ids.goldAssessment);
  const report=await(await fetch(`${base}/api/v1/quality-assessments/${ids.goldAssessment}/report?workspaceId=${ids.workspace}&limit=25&offset=25`,{headers})).json();
  assert.equal(report.id,ids.goldAssessment);assert.equal(report.findings.page.total,26);assert.equal(report.findings.items.length,1);assert.equal(report.findings.items[0].ruleId,"FIXTURE-26");
  const attempt=await(await fetch(`${base}/api/v1/quality-assessment-attempts/${ids.job}?workspaceId=${ids.workspace}`,{headers})).json();
  assert.equal(attempt.outcome,"SUCCEEDED");assert.equal(attempt.assessmentId,ids.goldAssessment);
  assert.ok((await state()).requests.every(r=>r.method==="GET"));
});

test("observed request fixture counts attempts once and keeps selected identity after its observation arrives",async()=>{
  const headers={Authorization:"Bearer rights-fixture-secret"};
  await fetch(`${base}/__control/reset`,{method:"POST",headers:controlHeaders,body:JSON.stringify({scenario:"schema-attempts"})});
  const list=`${base}/api/v1/workspaces/${ids.workspace}/source-schema-requests/observed/unobserved-attempts?limit=25&offset=100`;
  assert.equal((await fetch(list)).status,403);
  const page=await(await fetch(list,{headers})).json();assert.equal(page.page.total,105);assert.equal(page.items.length,5);
  assert.equal(new Set(page.items.map(a=>a.attemptId)).size,5);assert.ok(page.items.every(a=>a.requestId===observedRequestId&&a.requestObservationCount===31));
  const exact=`${base}/api/v1/data-resources/${ids.sourceResource}/schema-requests/${observedRequestId}?workspaceId=${ids.workspace}&attemptId=${observedAttemptId(1)}&attemptOffset=25`;
  const before=await(await fetch(exact,{headers})).json();assert.equal(before.selectedAttempt.observationId,null);assert.ok(!before.attempts.items.some(a=>a.id===observedAttemptId(1)));
  await fetch(`${base}/__control/scenario`,{method:"POST",headers:controlHeaders,body:JSON.stringify({scenario:"schema-attempts-arrived"})});
  const after=await(await fetch(exact,{headers})).json();assert.equal(after.selectedAttempt.id,observedAttemptId(1));assert.equal(after.selectedAttempt.observationId,observedObservationId(1));
  const remaining=await(await fetch(list,{headers})).json();assert.equal(remaining.page.total,104);assert.ok(remaining.items.every(a=>a.attemptId!==observedAttemptId(1)));
});

test("unobserved request fixture separates registered facts and retains exact identity after arrival",async()=>{
  const headers={Authorization:"Bearer rights-fixture-secret"};
  await fetch(`${base}/__control/reset`,{method:"POST",headers:controlHeaders,body:JSON.stringify({scenario:"schema-requests"})});
  const list=`${base}/api/v1/workspaces/${ids.workspace}/source-schema-requests/unobserved?limit=25&offset=100`;
  assert.equal((await fetch(list)).status,403);
  const page=await(await fetch(list,{headers})).json();assert.equal(page.page.total,105);assert.equal(page.items.length,5);assert.deepEqual(page.summary,{noAttemptCount:52,attemptRecordedCount:53});
  const exact=`${base}/api/v1/data-resources/${ids.sourceResource}/schema-requests/${unobservedRequestId(105)}?workspaceId=${ids.workspace}&attemptId=${requestAttemptId(105)}`;
  assert.equal((await fetch(exact.replace(requestAttemptId(105),requestAttemptId(104)),{headers})).status,404);
  const prior=await(await fetch(exact,{headers})).json();assert.equal(prior.selectedAttempt.observationId,null);assert.equal(prior.observations.page.total,0);
  await fetch(`${base}/__control/scenario`,{method:"POST",headers:controlHeaders,body:JSON.stringify({scenario:"schema-requests-arrived"})});
  const after=await(await fetch(exact,{headers})).json();assert.equal(after.selectedAttempt.id,prior.selectedAttempt.id);assert.equal(after.selectedAttempt.observationId,requestObservationId(105));assert.equal(after.observations.items[0].requestId,prior.requestId);
  assert.equal((await(await fetch(list,{headers})).json()).page.total,104);
});

test("exception history fixture preserves classification counts and exact nullable evidence", async () => {
  await fetch(`${base}/__control/reset`, {method:"POST",headers:controlHeaders,body:JSON.stringify({scenario:"schema-exceptions"})});
  const list=`${base}/api/v1/workspaces/${ids.workspace}/source-schema-observations/exceptions`;
  assert.equal((await fetch(list)).status,403);
  const headers={Authorization:"Bearer rights-fixture-secret"};
  const all=await (await fetch(`${list}?status=ALL&limit=25&offset=100`,{headers})).json();
  assert.equal(all.page.total,105);
  assert.equal(all.items.length,5);
  assert.deepEqual(all.summary,{unavailableCount:35,deniedCount:35,interruptedCount:35});
  const denied=await (await fetch(`${list}?status=DENIED&limit=25&offset=25`,{headers})).json();
  assert.equal(denied.page.total,35);
  assert.equal(denied.items.length,10);
  assert.ok(denied.items.every(o=>o.status==="DENIED"));
  const exact=await (await fetch(`${base}/api/v1/data-resources/${ids.sourceResource}/schema-observations/${exceptionObservationId(104)}`,{headers})).json();
  assert.equal(exact.requestId,exceptionRequestId(104));
  assert.equal(exact.attemptId,null);
  assert.equal(exact.completedAt,null);
  assert.deepEqual(exact.columns,[]);
  assert.deepEqual(exact.handling,[]);
});
test("control routes require a token", async () => assert.equal((await fetch(`${base}/__control/state`)).status, 403));

test("read faults are explicit token-controlled fixed routes and leave independent data readable", async () => {
  const options = { method: "POST", headers: controlHeaders, body: JSON.stringify({ scenario: "schema-requests", readFaults: { summary: "fail", products: "invalid" } }) };
  assert.equal((await fetch(`${base}/__control/reset`, { ...options, headers: {} })).status, 403);
  assert.equal((await fetch(`${base}/__control/reset`, options)).status, 200);
  const workspace = `${base}/api/v1/workspaces/${ids.workspace}`;
  assert.equal((await fetch(`${workspace}/workbench`)).status, 503);
  assert.deepEqual(await (await fetch(`${workspace}/data-products`)).json(), {});
  const r = await fetch(`${workspace}/source-schema-requests/unobserved`, { headers: { Authorization: "Bearer rights-fixture-secret" } });
  assert.equal((await r.json()).page.total, 105);
  assert.equal((await fetch(`${base}/__control/reset`, { ...options, body: JSON.stringify({ readFaults: { arbitrary: "fail" } }) })).status, 400);
  assert.equal((await fetch(`${base}/__control/reset`, { ...options, body: JSON.stringify({ readFaults: { summary: "typo" } }) })).status, 400);
});
test("unknown scenario is rejected rather than silently returning ready", async () => {
  assert.equal((await fetch(`${base}/__control/scenario`, { method: "POST", headers: controlHeaders, body: '{"scenario":"typo"}' })).status, 400);
});
test("unknown API routes fail explicitly", async () => assert.equal((await fetch(`${base}/api/v1/not-implemented`)).status, 501));
test("targeted candidate lookup returns one candidate, not a job queue", async () => {
  const response = await fetch(`${base}/api/v1/entity-match-reviews/${ids.candidate}`);
  assert.equal(response.status, 200);
  const body = await response.json();
  assert.equal(body.id, ids.candidate);
  assert.equal(body.jobId, ids.job);
  assert.equal(body.items, undefined);
  assert.equal(body.currentMappingDecisionId, ids.decision);
});
test("review needs trusted credential and reason", async () => {
  const url = `${base}/api/v1/entity-match-reviews/${ids.candidate}/confirm`;
  assert.equal((await fetch(url, { method: "POST", body: "{}" })).status, 401);
  assert.equal((await fetch(url, { method: "POST", headers: { Authorization: "Bearer review-secret" }, body: "{}" })).status, 400);
  assert.equal((await state()).candidateStatus, "PENDING");
});
test("review mutates only the fixture state and is not accepted twice", async () => {
  const options = { method: "POST", headers: { Authorization: "Bearer review-secret" }, body: JSON.stringify({ reason: "checked", expectedDecisionId: ids.decision }) };
  const url = `${base}/api/v1/entity-match-reviews/${ids.candidate}/reject`;
  assert.equal((await fetch(url, options)).status, 200);
  assert.equal((await fetch(url, options)).status, 409);
  assert.equal((await state()).candidateStatus, "REJECTED");
});
test("review refuses a stale or absent decision token instead of overwriting", async () => {
  const url = `${base}/api/v1/entity-match-reviews/${ids.candidate}/confirm`;
  const headers = { Authorization: "Bearer review-secret", "Content-Type": "application/json" };
  assert.equal((await fetch(url, { method: "POST", headers, body: JSON.stringify({ reason: "checked" }) })).status, 409);
  assert.equal((await fetch(url, { method: "POST", headers, body: JSON.stringify({ reason: "checked", expectedDecisionId: "00000000-0000-0000-0000-000000000001" }) })).status, 409);
  assert.equal((await state()).candidateStatus, "PENDING");
});
test("changing scenario preserves request history", async () => {
  await fetch(`${base}/api/v1/product-releases/${ids.release}/readiness`);
  await fetch(`${base}/__control/scenario`, { method: "POST", headers: controlHeaders, body: '{"scenario":"empty-checks"}' });
  const body = await (await fetch(`${base}/api/v1/product-releases/${ids.release}/readiness`)).json();
  assert.deepEqual(body.checks, {});
  assert.equal((await state()).requests.length, 2);
});
test("paginated resource list slices by server-side offset", async () => {
  await fetch(`${base}/__control/scenario`, { method: "POST", headers: controlHeaders, body: '{"scenario":"paginated-resources"}' });
  const first = await (await fetch(`${base}/api/v1/workspaces/${ids.workspace}/data-resources?limit=25&offset=0`)).json();
  assert.equal(first.page.total, 30);
  assert.equal(first.page.limit, 25);
  assert.equal(first.page.offset, 0);
  assert.equal(first.items.length, 25);
  assert.equal(first.items[0].name, "资源 1");
  assert.equal(first.items.at(-1).name, "资源 25");
  const second = await (await fetch(`${base}/api/v1/workspaces/${ids.workspace}/data-resources?limit=25&offset=25`)).json();
  assert.equal(second.page.total, 30);
  assert.equal(second.page.offset, 25);
  assert.deepEqual(second.items.map((item) => item.name), ["资源 26", "资源 27", "资源 28", "资源 29", "资源 30"]);
});

test("default resource list stays empty so write tests see no phantom rows", async () => {
  const body = await (await fetch(`${base}/api/v1/workspaces/${ids.workspace}/data-resources`)).json();
  assert.equal(body.page.total, 0);
  assert.deepEqual(body.items, []);
});

test("publish requires idempotency header and stores observed command headers", async () => {
  const url = `${base}/api/v1/product-releases/${ids.release}/publish`;
  assert.equal((await fetch(url, { method: "POST", headers: { "X-Actor-ID": ids.actor }, body: "{}" })).status, 400);
  assert.equal((await fetch(url, { method: "POST", headers: { "X-Actor-ID": ids.actor, "Idempotency-Key": "test-1" }, body: "{}" })).status, 200);
  assert.equal((await state()).releaseStatus, "PUBLISHED");
  assert.equal((await state()).requests.at(-1).idempotencyKey, "test-1");
});

test('modern Trace fixture requires exact workspace and credential and excludes secret fields',async()=>{
 await fetch(base+'/__control/reset',{method:'POST',headers:controlHeaders,body:JSON.stringify({scenario:'modern-trace'})});
 const path=`/api/v1/traceability/product-releases/${ids.release}?workspaceId=${ids.workspace}`;
 assert.equal((await fetch(base+path)).status,401);
 const headers={Authorization:'Bearer review-secret'};
 assert.equal((await fetch(base+path.replace(ids.workspace,ids.product),{headers})).status,403);
 const response=await fetch(base+path,{headers});assert.equal(response.status,200);const t=await response.json();assert.equal(t.releaseId,ids.release);assert.equal(t.workspaceId,ids.workspace);assert.equal(t.entityMappings[0].decisionId,ids.decision);assert.equal(t.costEvents[0].amount,'0.000000000000000001');
 assert.doesNotMatch(JSON.stringify(t),/metadata|sourceKey|storageUri|review-secret/);
});
