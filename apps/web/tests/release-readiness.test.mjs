import test from "node:test";
import assert from "node:assert/strict";
const { default: readinessModule } = await import(new URL("../.release-tests/release-readiness.js", import.meta.url).href);
const { evaluateReleaseReadiness, requiredReadinessGates } = readinessModule;

const passChecks = Object.fromEntries(requiredReadinessGates.map((gate) => [gate, "PASS"]));
const ready = { overall: "READY", checks: passChecks, blockers: [] };

test("complete READY contract is publishable", () => {
  assert.deepEqual(evaluateReleaseReadiness("READY", ready), { ready: true, problems: [] });
});

test("empty checks fail closed", () => {
  const result = evaluateReleaseReadiness("READY", { overall: "READY", checks: {}, blockers: [] });
  assert.equal(result.ready, false);
  for (const gate of requiredReadinessGates) assert.ok(result.problems.includes(`GATE_MISSING:${gate}`));
});

test("missing checks object fails closed", () => {
  const result = evaluateReleaseReadiness("READY", { overall: "READY", blockers: [] });
  assert.equal(result.ready, false);
  assert.ok(result.problems.includes("CHECKS_INVALID"));
});

test("malformed checks array fails closed", () => {
  const result = evaluateReleaseReadiness("READY", { overall: "READY", checks: [], blockers: [] });
  assert.equal(result.ready, false);
  assert.ok(result.problems.includes("CHECKS_INVALID"));
});

test("missing blockers fails closed", () => {
  const result = evaluateReleaseReadiness("READY", { overall: "READY", checks: passChecks });
  assert.equal(result.ready, false);
  assert.ok(result.problems.includes("BLOCKERS_INVALID"));
});

test("malformed blockers fail closed", () => {
  const result = evaluateReleaseReadiness("READY", { overall: "READY", checks: passChecks, blockers: "none" });
  assert.equal(result.ready, false);
  assert.ok(result.problems.includes("BLOCKERS_INVALID"));
});

test("contradictory non-empty blockers fail even when all gates PASS", () => {
  const result = evaluateReleaseReadiness("READY", { overall: "READY", checks: passChecks, blockers: ["RIGHTS_EXPIRED"] });
  assert.equal(result.ready, false);
  assert.ok(result.problems.includes("BLOCKERS_PRESENT"));
});

test("non-string blocker fails closed", () => {
  const result = evaluateReleaseReadiness("READY", { overall: "READY", checks: passChecks, blockers: [42] });
  assert.equal(result.ready, false);
  assert.ok(result.problems.includes("BLOCKERS_INVALID"));
});

test("overall NOT_READY fails closed", () => {
  const result = evaluateReleaseReadiness("READY", { ...ready, overall: "NOT_READY" });
  assert.equal(result.ready, false);
  assert.ok(result.problems.includes("READINESS_NOT_READY"));
});

test("release status must itself be READY", () => {
  const result = evaluateReleaseReadiness("DRAFT", ready);
  assert.equal(result.ready, false);
  assert.ok(result.problems.includes("RELEASE_NOT_READY"));
});

for (const gate of requiredReadinessGates) {
  test(`missing required gate ${gate} fails closed`, () => {
    const checks = { ...passChecks };
    delete checks[gate];
    const result = evaluateReleaseReadiness("READY", { ...ready, checks });
    assert.equal(result.ready, false);
    assert.ok(result.problems.includes(`GATE_MISSING:${gate}`));
  });

  test(`non-PASS required gate ${gate} fails closed`, () => {
    const result = evaluateReleaseReadiness("READY", { ...ready, checks: { ...passChecks, [gate]: "FAIL" } });
    assert.equal(result.ready, false);
    assert.ok(result.problems.includes(`GATE_NOT_PASS:${gate}`));
  });
}

test("future gate PASS is accepted without weakening canonical checks", () => {
  const result = evaluateReleaseReadiness("READY", { ...ready, checks: { ...passChecks, externalPublication: "PASS" } });
  assert.equal(result.ready, true);
});

for (const status of ["FAIL", "PENDING", "REVIEW", null, true]) {
  test(`future gate ${String(status)} fails closed`, () => {
    const result = evaluateReleaseReadiness("READY", { ...ready, checks: { ...passChecks, futureGate: status } });
    assert.equal(result.ready, false);
    assert.ok(result.problems.includes("EXTRA_GATE_NOT_PASS:futureGate"));
  });
}

test("invalid readiness body fails closed", () => {
  const result = evaluateReleaseReadiness("READY", null);
  assert.equal(result.ready, false);
  assert.ok(result.problems.includes("READINESS_INVALID"));
});
