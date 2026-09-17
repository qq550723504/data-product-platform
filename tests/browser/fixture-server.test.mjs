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
test("review needs actor and reason", async () => {
  const url = `${base}/api/v1/entity-match-reviews/${ids.candidate}/confirm`;
  assert.equal((await fetch(url, { method: "POST", body: "{}" })).status, 400);
  assert.equal((await fetch(url, { method: "POST", headers: { "X-Actor-ID": ids.actor }, body: "{}" })).status, 400);
  assert.equal((await state()).candidateStatus, "PENDING");
});
test("review mutates only the fixture state and is not accepted twice", async () => {
  const options = { method: "POST", headers: { "X-Actor-ID": ids.actor }, body: '{"reason":"checked"}' };
  const url = `${base}/api/v1/entity-match-reviews/${ids.candidate}/reject`;
  assert.equal((await fetch(url, options)).status, 200);
  assert.equal((await fetch(url, options)).status, 409);
  assert.equal((await state()).candidateStatus, "REJECTED");
});
test("changing scenario preserves request history", async () => {
  await fetch(`${base}/api/v1/product-releases/${ids.release}/readiness`);
  await fetch(`${base}/__control/scenario`, { method: "POST", headers: controlHeaders, body: '{"scenario":"empty-checks"}' });
  const body = await (await fetch(`${base}/api/v1/product-releases/${ids.release}/readiness`)).json();
  assert.deepEqual(body.checks, {});
  assert.equal((await state()).requests.length, 2);
});
test("publish requires idempotency header and stores observed command headers", async () => {
  const url = `${base}/api/v1/product-releases/${ids.release}/publish`;
  assert.equal((await fetch(url, { method: "POST", headers: { "X-Actor-ID": ids.actor }, body: "{}" })).status, 400);
  assert.equal((await fetch(url, { method: "POST", headers: { "X-Actor-ID": ids.actor, "Idempotency-Key": "test-1" }, body: "{}" })).status, 200);
  assert.equal((await state()).releaseStatus, "PUBLISHED");
  assert.equal((await state()).requests.at(-1).idempotencyKey, "test-1");
});
