import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";

const require = createRequire(import.meta.url);
const { executeQualityCheck } = require("../.quality-tests/quality-command.js");

const ids = {
  workspace: "11111111-1111-4111-8111-111111111111",
  actor: "22222222-2222-4222-8222-222222222222",
  dataset: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  version: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
  attempt: "cccccccc-cccc-4ccc-8ccc-cccccccccccc",
  assessment: "dddddddd-dddd-4ddd-8ddd-dddddddddddd",
  foreign: "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee",
};

const config = {
  enabled: true,
  workspaceId: ids.workspace,
  actorId: ids.actor,
  apiBaseUrl: "http://core.test",
};

function form() {
  const data = new FormData();
  data.set("datasetId", ids.dataset);
  data.set("versionId", ids.version);
  data.set("assessmentAttemptId", ids.attempt);
  data.set("ruleSetRef", "");
  data.set("engineName", "");
  return data;
}

function stub(responses) {
  const calls = [];
  const request = async (url, init = {}) => {
    calls.push({ url, init });
    const response = responses.shift();
    if (response instanceof Error) throw response;
    if (response instanceof Response) return response;
    assert.notEqual(response, undefined, "unexpected request / blind retry");
    return Response.json(response);
  };
  return { calls, request };
}

const version = { id: ids.version, datasetId: ids.dataset, status: "READY" };

test("new attempt posts exactly once with stable attempt id and server actor", async () => {
  const transport = stub([
    version,
    Response.json({ error: { code: "QUALITY_ASSESSMENT_ATTEMPT_NOT_FOUND" } }, { status: 404 }),
    { id: ids.assessment, workspaceId: ids.workspace, datasetVersionId: ids.version },
  ]);
  const result = await executeQualityCheck(form(), config, transport.request);
  assert.equal(result.ok, true);
  assert.equal(result.assessmentId, ids.assessment);
  assert.equal(result.attemptId, ids.attempt);
  assert.equal(transport.calls.length, 3);
  const write = transport.calls[2];
  assert.equal(write.url, `http://core.test/api/v1/dataset-versions/${ids.version}/quality-checks`);
  assert.equal(write.init.method, "POST");
  assert.equal(write.init.headers["X-Actor-ID"], ids.actor);
  assert.deepEqual(JSON.parse(write.init.body), {
    workspaceId: ids.workspace,
    ruleSetRef: "",
    engineName: "",
    assessmentAttemptId: ids.attempt,
  });
});

test("succeeded attempt recovers assessment without POST", async () => {
  const transport = stub([
    version,
    {
      id: ids.attempt,
      workspaceId: ids.workspace,
      datasetVersionId: ids.version,
      outcome: "SUCCEEDED",
      assessmentId: ids.assessment,
      leaseExpired: false,
    },
  ]);
  const result = await executeQualityCheck(form(), config, transport.request);
  assert.equal(result.ok, true);
  assert.equal(result.assessmentId, ids.assessment);
  assert.equal(transport.calls.length, 2);
  assert.ok(transport.calls.every((call) => call.init.method !== "POST"));
});

test("active attempt never posts", async () => {
  const transport = stub([
    version,
    {
      id: ids.attempt,
      workspaceId: ids.workspace,
      datasetVersionId: ids.version,
      outcome: "IN_PROGRESS",
      leaseExpired: false,
    },
  ]);
  const result = await executeQualityCheck(form(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.equal(transport.calls.length, 2);
  assert.ok(transport.calls.every((call) => call.init.method !== "POST"));
});

test("failed attempt is terminal and never posts", async () => {
  const transport = stub([
    version,
    {
      id: ids.attempt,
      workspaceId: ids.workspace,
      datasetVersionId: ids.version,
      outcome: "FAILED",
      leaseExpired: false,
    },
  ]);
  const result = await executeQualityCheck(form(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, undefined);
  assert.equal(transport.calls.length, 2);
});

test("foreign attempt is blocked before write", async () => {
  const transport = stub([
    version,
    {
      id: ids.attempt,
      workspaceId: ids.foreign,
      datasetVersionId: ids.version,
      outcome: "IN_PROGRESS",
      leaseExpired: false,
    },
  ]);
  const result = await executeQualityCheck(form(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 2);
});

test("non-usable DatasetVersion never looks up or posts attempt", async () => {
  const transport = stub([{ ...version, status: "INVALID" }]);
  const result = await executeQualityCheck(form(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 1);
});

test("timeout after POST preserves attempt identity and never retries", async () => {
  const transport = stub([
    version,
    Response.json({ error: { code: "QUALITY_ASSESSMENT_ATTEMPT_NOT_FOUND" } }, { status: 404 }),
    new Error("timeout"),
  ]);
  const result = await executeQualityCheck(form(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.equal(result.attemptId, ids.attempt);
  assert.match(result.message, /不要生成新 attempt/);
  assert.equal(transport.calls.length, 3);
});

test("disabled configuration fails before any Core request", async () => {
  const transport = stub([]);
  const result = await executeQualityCheck(form(), { ...config, enabled: false }, transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 0);
});
