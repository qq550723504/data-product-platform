import test from "node:test";
import assert from "node:assert/strict";
import { executeRetry } from "../.execution-tests/execution-command.js";

const ids = {
  workspace: "11111111-1111-4111-8111-111111111111",
  actor: "22222222-2222-4222-8222-222222222222",
  source: "33333333-3333-4333-8333-333333333333",
  child: "44444444-4444-4444-8444-444444444444",
  workflow: "55555555-5555-4555-8555-555555555555",
  dataset: "66666666-6666-4666-8666-666666666666",
};

function form() {
  const value = new FormData();
  value.set("executionId", ids.source);
  return value;
}

function config(overrides = {}) {
  return {
    enabled: true,
    workspaceId: ids.workspace,
    actorId: ids.actor,
    apiBaseUrl: "http://core.test",
    ...overrides,
  };
}

test("retry preflights source and posts once with server actor/idempotency", async () => {
  const calls = [];
  const request = async (url, init = {}) => {
    calls.push({ url, init });
    if (url.endsWith(`/api/v1/executions/${ids.source}`)) {
      return Response.json({
        id: ids.source, workspaceId: ids.workspace, workflowVersionId: ids.workflow,
        outputDatasetId: ids.dataset, targetPeriod: "2026-09", status: "FAILED", attempt: 2,
      });
    }
    return Response.json({
      id: ids.child, workspaceId: ids.workspace, workflowVersionId: ids.workflow,
      outputDatasetId: ids.dataset, targetPeriod: "2026-09", status: "QUEUED", attempt: 3,
      retryOfExecutionId: ids.source,
    }, { status: 202 });
  };

  const result = await executeRetry(form(), config(), "retry-key", request);
  assert.equal(result.ok, true);
  assert.equal(result.execution?.id, ids.child);
  assert.equal(calls.length, 2);
  assert.equal(calls[1].init.method, "POST");
  assert.equal(calls[1].init.headers["X-Actor-ID"], ids.actor);
  assert.equal(calls[1].init.headers["Idempotency-Key"], "retry-key");
});

test("non-retryable source never posts", async () => {
  let posts = 0;
  const request = async (_url, init = {}) => {
    if (init.method === "POST") posts++;
    return Response.json({
      id: ids.source, workspaceId: ids.workspace, workflowVersionId: ids.workflow,
      outputDatasetId: ids.dataset, targetPeriod: "2026-09", status: "RUNNING", attempt: 2,
    });
  };
  const result = await executeRetry(form(), config(), "retry-key", request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.equal(posts, 0);
});

test("mismatched child fails closed after write and requires refresh", async () => {
  let call = 0;
  const request = async () => {
    call++;
    if (call === 1) return Response.json({
      id: ids.source, workspaceId: ids.workspace, workflowVersionId: ids.workflow,
      outputDatasetId: ids.dataset, status: "FAILED", attempt: 2,
    });
    return Response.json({
      id: ids.child, workspaceId: ids.workspace, workflowVersionId: ids.workflow,
      outputDatasetId: ids.dataset, status: "QUEUED", attempt: 3,
      retryOfExecutionId: ids.child,
    }, { status: 202 });
  };
  const result = await executeRetry(form(), config(), "retry-key", request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.match(result.message, /请求可能已送达/);
});

test("disabled configuration performs no request", async () => {
  let calls = 0;
  const result = await executeRetry(form(), config({ enabled: false }), "retry-key", async () => {
    calls++;
    throw new Error("should not request");
  });
  assert.equal(result.ok, false);
  assert.equal(calls, 0);
});
