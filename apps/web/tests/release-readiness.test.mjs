import test from "node:test";
import assert from "node:assert/strict";
import guards from "../.release-tests/release-readiness.js";
import commands from "../.release-tests/release-command.js";
const { requiredReleaseGates: gates, releaseReadinessProblem: problem } = guards;
const checks = Object.fromEntries(gates.map((gate) => [gate, "PASS"]));
const ready = { overall: "READY", checks, blockers: [] };
const invalid = [
  ["empty checks", { ...ready, checks: {} }],
  ["missing checks", { overall: "READY", blockers: [] }],
  ["null checks", { ...ready, checks: null }],
  ["array checks", { ...ready, checks: [] }],
  ["missing blockers", { overall: "READY", checks }],
  ["null blockers", { ...ready, blockers: null }],
  ["string blockers", { ...ready, blockers: "none" }],
  ["invalid blocker", { ...ready, blockers: [42] }],
  ["remaining blocker", { ...ready, blockers: ["RIGHTS_REVOKED"] }],
  ["blank blocker", { ...ready, blockers: [""] }],
  ["unknown gate pending", { ...ready, checks: { ...checks, futureGate: "PENDING" } }],
  ...["FAIL", "REVIEW", null, true].map((status) => [`extra gate ${String(status)}`, { ...ready, checks: { ...checks, futureGate: status } }]),
  ...gates.flatMap((gate) => [
    [`missing ${gate}`, { ...ready, checks: Object.fromEntries(Object.entries(checks).filter(([name]) => name !== gate)) }],
    [`failed ${gate}`, { ...ready, checks: { ...checks, [gate]: "FAIL" } }],
  ]),
];
for (const [name, value] of invalid) {
  test(`${name}: shared guard and Server Action boundary fail closed`, async () => {
    assert.notEqual(problem(value), null);
    const workspaceId = "11111111-1111-4111-8111-111111111111";
    const actorId = "22222222-2222-4222-8222-222222222222";
    const productId = "33333333-3333-4333-8333-333333333333";
    const releaseId = "44444444-4444-4444-8444-444444444444";
    const release = { id: releaseId, productId, status: "READY" };
    const responses = [{ items: [{ id: productId, workspaceId }] }, { items: [release] }, release, { ...value, releaseId }];
    const calls = [];
    const request = async (url, init) => {
      calls.push({ url, init });
      assert.notEqual(init.method, "POST", "an inconsistent readiness response must never reach publish");
      return Response.json(responses.shift());
    };
    const form = new FormData();
    form.set("productId", productId); form.set("releaseId", releaseId);
    const result = await commands.executePublish(form, { enabled: true, workspaceId, actorId, apiBaseUrl: "http://core.invalid" }, "fixture-key", request);
    assert.equal(result.ok, false);
    assert.equal(result.refreshRequired, true);
    assert.equal(calls.length, 4);
  });
}
test("complete READY response is accepted", () => assert.equal(problem(ready), null));
test("new passing Core gate is not rejected", () => assert.equal(problem({ ...ready, checks: { ...checks, futureGate: "PASS" } }), null));
test("inherited gates cannot satisfy required fields", () => assert.notEqual(problem({ ...ready, checks: Object.create(checks) }), null));
for (const value of [null, undefined, [], "READY", { ...ready, overall: "NOT_READY" }]) {
  test(`invalid/not-ready response ${JSON.stringify(value)}`, () => assert.notEqual(problem(value), null));
}
