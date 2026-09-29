const test = require("node:test");
const assert = require("node:assert/strict");
const { executeInvalidateDatasetVersion } = require("../.dataset-version-tests/dataset-version-command.js");

const ids = {
  workspace: "11111111-1111-4111-8111-111111111111",
  actor: "22222222-2222-4222-8222-222222222222",
  dataset: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  version: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
};

function config(overrides = {}) {
  return {
    enabled: true,
    workspaceId: ids.workspace,
    actorId: ids.actor,
    apiBaseUrl: "http://core.test",
    ...overrides,
  };
}

function form(reason = "  stale upstream source  ") {
  const value = new FormData();
  value.set("datasetId", ids.dataset);
  value.set("versionId", ids.version);
  value.set("reason", reason);
  return value;
}

test("invalidate preflights scope/status and posts trimmed reason with server actor", async () => {
  const calls = [];
  const request = async (url, init = {}) => {
    calls.push({ url, init });
    if (calls.length === 1) return Response.json({ id: ids.dataset, workspaceId: ids.workspace });
    if (calls.length === 2) return Response.json({ id: ids.version, datasetId: ids.dataset, status: "READY" });
    return Response.json({
      id: ids.version,
      datasetId: ids.dataset,
      status: "INVALID",
      invalidatedAt: "2026-09-29T00:00:00Z",
      invalidationReason: "stale upstream source",
    });
  };

  const result = await executeInvalidateDatasetVersion(form(), config(), request);
  assert.equal(result.ok, true);
  assert.equal(calls.length, 3);
  assert.equal(calls[2].url, `http://core.test/api/v1/dataset-versions/${ids.version}/invalidate`);
  assert.equal(calls[2].init.headers["X-Actor-ID"], ids.actor);
  assert.deepEqual(JSON.parse(calls[2].init.body), { reason: "stale upstream source" });
});

test("non-READY version never posts", async () => {
  let calls = 0;
  const request = async () => {
    calls++;
    if (calls === 1) return Response.json({ id: ids.dataset, workspaceId: ids.workspace });
    return Response.json({ id: ids.version, datasetId: ids.dataset, status: "INVALID" });
  };

  const result = await executeInvalidateDatasetVersion(form(), config(), request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.equal(calls, 2);
});

test("blank reason fails before any Core request", async () => {
  let calls = 0;
  const result = await executeInvalidateDatasetVersion(form("   "), config(), async () => {
    calls++;
    return Response.json({});
  });
  assert.equal(result.ok, false);
  assert.equal(calls, 0);
});

test("mismatched invalidate response fails closed after write", async () => {
  let calls = 0;
  const request = async () => {
    calls++;
    if (calls === 1) return Response.json({ id: ids.dataset, workspaceId: ids.workspace });
    if (calls === 2) return Response.json({ id: ids.version, datasetId: ids.dataset, status: "READY" });
    return Response.json({ id: ids.version, datasetId: ids.dataset, status: "READY", invalidationReason: "stale upstream source" });
  };
  const result = await executeInvalidateDatasetVersion(form(), config(), request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.equal(calls, 3);
});

test("disabled configuration fails closed", async () => {
  let calls = 0;
  const result = await executeInvalidateDatasetVersion(form(), config({ enabled: false }), async () => {
    calls++;
    return Response.json({});
  });
  assert.equal(result.ok, false);
  assert.equal(calls, 0);
});
