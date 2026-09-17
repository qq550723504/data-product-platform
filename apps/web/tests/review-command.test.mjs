import test from "node:test";
import assert from "node:assert/strict";
const { default: review } = await import(new URL("../.review-tests/review-command.js", import.meta.url).href);
const { executeReview } = review;
const ids = {
  workspace: "11111111-1111-4111-8111-111111111111",
  actor: "22222222-2222-4222-8222-222222222222",
  job: "33333333-3333-4333-8333-333333333333",
  candidate: "44444444-4444-4444-8444-444444444444",
  entity: "55555555-5555-4555-8555-555555555555",
  foreign: "66666666-6666-4666-8666-666666666666",
};
const config = { enabled: true, workspaceId: ids.workspace, actorId: ids.actor, apiBaseUrl: "http://core.invalid" };
const job = { id: ids.job, workspaceId: ids.workspace, status: "WAITING_REVIEW" };
const candidate = { id: ids.candidate, jobId: ids.job, status: "PENDING", candidateEntityId: ids.entity };
function form(overrides = {}) {
  const data = new FormData();
  for (const [key, value] of Object.entries({ jobId: ids.job, candidateId: ids.candidate, decision: "confirm", reason: "  已核对来源记录  ", ...overrides })) data.set(key, value);
  return data;
}
function stub(responses = [job, { items: [candidate] }, { ...job, status: "SUCCEEDED" }]) {
  const calls = [];
  const request = async (url, init) => {
    calls.push({ url, init });
    const response = responses.shift();
    if (response instanceof Error) throw response;
    if (response instanceof Response) return response;
    assert.notEqual(response, undefined, "unexpected request / automatic retry");
    return Response.json(response);
  };
  return { calls, request };
}
for (const decision of ["confirm", "reject"]) {
  test(`${decision}: scope checked, server actor and trimmed reason forwarded once`, async () => {
    const transport = stub();
    const result = await executeReview(form({ decision, actorId: ids.foreign, workspaceId: ids.foreign }), config, transport.request);
    assert.equal(result.ok, true);
    assert.equal(result.job.status, "SUCCEEDED");
    assert.equal(transport.calls.length, 3);
    const { url, init } = transport.calls[2];
    assert.equal(url, `http://core.invalid/api/v1/entity-match-reviews/${ids.candidate}/${decision}`);
    assert.equal(init.headers["X-Actor-ID"], ids.actor);
    assert.deepEqual(JSON.parse(init.body), { reason: "已核对来源记录" });
    assert.equal(init.cache, "no-store");
    assert.equal(init.redirect, "error");
    assert.ok(init.signal instanceof AbortSignal);
  });
}
for (const [name, settings] of [
  ["disabled", { enabled: false }], ["missing reviewer", { actorId: null }],
  ["invalid reviewer", { actorId: "bad" }], ["nil reviewer", { actorId: "00000000-0000-0000-0000-000000000000" }],
  ["missing workspace", { workspaceId: null }],
]) {
  test(`${name}: no upstream request`, async () => {
    const transport = stub([]);
    assert.equal((await executeReview(form(), { ...config, ...settings }, transport.request)).ok, false);
    assert.equal(transport.calls.length, 0);
  });
}
for (const [name, fields] of [
  ["blank reason", { reason: "  \n " }], ["long reason", { reason: "a".repeat(2001) }],
  ["path injection", { candidateId: "../other" }], ["invalid job", { jobId: "bad" }],
  ["unknown decision", { decision: "publish" }],
]) {
  test(`${name}: no upstream request`, async () => {
    const transport = stub([]);
    assert.equal((await executeReview(form(fields), config, transport.request)).ok, false);
    assert.equal(transport.calls.length, 0);
  });
}
test("foreign workspace blocks even a valid candidate UUID", async () => {
  const transport = stub([{ ...job, workspaceId: ids.foreign }]);
  assert.equal((await executeReview(form(), config, transport.request)).ok, false);
  assert.equal(transport.calls.length, 1);
});
for (const [name, payload] of [
  ["foreign candidate", { ...candidate, id: ids.foreign }],
  ["foreign candidate job", { ...candidate, jobId: ids.foreign }],
  ["already reviewed", { ...candidate, status: "CONFIRMED" }],
  ["missing entity", { ...candidate, candidateEntityId: null }],
]) {
  test(`${name}: never posts`, async () => {
    const transport = stub([job, { items: [payload] }]);
    assert.equal((await executeReview(form(), config, transport.request)).ok, false);
    assert.equal(transport.calls.length, 2);
  });
}
test("a pending candidate without an entity may be rejected, not confirmed", async () => {
  const transport = stub([job, { items: [{ ...candidate, candidateEntityId: null }] }, job]);
  assert.equal((await executeReview(form({ decision: "reject" }), config, transport.request)).ok, true);
});
test("invalid candidate response blocks the command", async () => {
  const transport = stub([job, { items: [null] }]);
  assert.equal((await executeReview(form(), config, transport.request)).ok, false);
  assert.equal(transport.calls.length, 2);
});
test("nested Core errors surface a safe code, not SQL or internal messages", async () => {
  const transport = stub([job, { items: [candidate] }, Response.json({ error: { code: "REVIEW_CONFLICT", message: "secret SQL" } }, { status: 400 })]);
  const result = await executeReview(form(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.match(result.message, /REVIEW_CONFLICT/);
  assert.doesNotMatch(result.message, /secret SQL/);
});
test("concurrent review conflict requires refresh and never retries", async () => {
  const transport = stub([job, { items: [candidate] }, new Response("conflict", { status: 409 })]);
  const result = await executeReview(form(), config, transport.request);
  assert.equal(result.refreshRequired, true);
  assert.equal(transport.calls.length, 3);
});
test("timeout after POST is ambiguous, not claimed as a failed write or retried", async () => {
  const transport = stub([job, { items: [candidate] }, new Error("timeout")]);
  const result = await executeReview(form(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.match(result.message, /可能已送达/);
  assert.equal(transport.calls.length, 3);
});
test("mismatched command result is not reported as success", async () => {
  const transport = stub([job, { items: [candidate] }, { ...job, id: ids.foreign }]);
  const result = await executeReview(form(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
});
test("read failure cannot trigger a write", async () => {
  const transport = stub([new Error("offline")]);
  assert.equal((await executeReview(form(), config, transport.request)).refreshRequired, false);
  assert.equal(transport.calls.length, 1);
});
