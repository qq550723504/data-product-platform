import test, { before, after, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { createFixtureServer, ids, fixtureToken } from "./fixture-server.mjs";
let server, base;
const controlHeaders = { "x-fixture-token": fixtureToken, "Content-Type": "application/json" };
before(async () => {
  server = createFixtureServer();
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  base = `http://127.0.0.1:${server.address().port}`;
});
after(async () => { await new Promise((resolve) => server.close(resolve)); });
beforeEach(async () => { await fetch(`${base}/__control/reset`, { method: "POST", headers: controlHeaders, body: "{}" }); });
async function state() { return (await fetch(`${base}/__control/state`, { headers: controlHeaders })).json(); }
test("control routes require a token", async () => assert.equal((await fetch(`${base}/__control/state`)).status, 403));
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
