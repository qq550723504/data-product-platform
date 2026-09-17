import test from "node:test";
import assert from "node:assert/strict";
const { default: scope } = await import(new URL("../.trace-tests/trace-scope.js", import.meta.url).href);
const { validateReleaseTraceScope, TraceScopeError } = scope;

const ids = {
  workspace: "11111111-1111-4111-8111-111111111111",
  product: "22222222-2222-4222-8222-222222222222",
  release: "33333333-3333-4333-8333-333333333333",
  version: "44444444-4444-4444-8444-444444444444",
  foreign: "55555555-5555-4555-8555-555555555555",
};

function trace(overrides = {}) {
  return {
    releaseId: ids.release,
    releaseNo: "R1",
    status: "PUBLISHED",
    productId: ids.product,
    productVersionId: ids.version,
    datasetVersions: [],
    executions: [],
    entityMatchJobs: [],
    entityMappings: [],
    evidence: [],
    costEvents: [],
    auditEvents: [],
    ...overrides,
  };
}
const context = { workspaceId: ids.workspace, productId: ids.product, releaseId: ids.release };

test("matching scoped release trace is accepted", () => {
  assert.doesNotThrow(() => validateReleaseTraceScope(trace({
    evidence: [{ workspaceId: ids.workspace }],
    costEvents: [{ workspaceId: ids.workspace }],
    entityMatchJobs: [{ workspaceId: ids.workspace }],
  }), context));
});

for (const [name, value, code] of [
  ["release mismatch", { releaseId: ids.foreign }, "RELEASE_MISMATCH"],
  ["product mismatch", { productId: ids.foreign }, "PRODUCT_MISMATCH"],
  ["foreign evidence", { evidence: [{ workspaceId: ids.foreign }] }, "EVIDENCE_WORKSPACE_MISMATCH"],
  ["foreign cost", { costEvents: [{ workspaceId: ids.foreign }] }, "COST_WORKSPACE_MISMATCH"],
  ["foreign entity job", { entityMatchJobs: [{ workspaceId: ids.foreign }] }, "ENTITY_JOB_WORKSPACE_MISMATCH"],
]) {
  test(`${name} is rejected`, () => {
    assert.throws(
      () => validateReleaseTraceScope(trace(value), context),
      (error) => error instanceof TraceScopeError && error.code === code,
    );
  });
}

test("invalid context identifiers fail closed", () => {
  assert.throws(
    () => validateReleaseTraceScope(trace(), { ...context, workspaceId: "../other" }),
    (error) => error instanceof TraceScopeError && error.code === "INVALID_SCOPE",
  );
});
