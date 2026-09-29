import test from "node:test";
import assert from "node:assert/strict";
const { default: releases } = await import(new URL("../.release-tests/release-command.js", import.meta.url).href);
const { executeCreateRelease, executePublish } = releases;

const ids = {
  workspace: "11111111-1111-4111-8111-111111111111",
  actor: "22222222-2222-4222-8222-222222222222",
  product: "33333333-3333-4333-8333-333333333333",
  release: "44444444-4444-4444-8444-444444444444",
  version: "55555555-5555-4555-8555-555555555555",
  foreign: "66666666-6666-4666-8666-666666666666",
  dataset: "77777777-7777-4777-8777-777777777777",
  datasetVersion: "88888888-8888-4888-8888-888888888888",
  createdRelease: "99999999-9999-4999-8999-999999999999",
};
const config = { enabled: true, workspaceId: ids.workspace, actorId: ids.actor, apiBaseUrl: "http://core.invalid" };
const product = { id: ids.product, workspaceId: ids.workspace, name: "Enterprise Activity" };
const release = { id: ids.release, productId: ids.product, productVersionId: ids.version, releaseNo: "R1", status: "READY" };
const checks = {
  production: "PASS", dataset: "PASS", rights: "PASS", quality: "PASS",
  compliance: "PASS", contract: "PASS", evidence: "PASS", delivery: "PASS",
};
const readiness = { releaseId: ids.release, overall: "READY", checks, blockers: [] };
const published = { ...release, status: "PUBLISHED", releasedAt: "2026-09-17T04:00:00Z" };

function form(overrides = {}) {
  const data = new FormData();
  for (const [key, value] of Object.entries({ productId: ids.product, releaseId: ids.release, ...overrides })) data.set(key, value);
  return data;
}

function stub(responses = [
  { items: [product], page: { limit: 100, offset: 0, total: 1 } },
  { items: [release], page: { limit: 100, offset: 0, total: 1 } },
  release,
  readiness,
  published,
]) {
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

test("publish: scope/readiness checked and server actor/idempotency forwarded once", async () => {
  const transport = stub();
  const result = await executePublish(form({ actorId: ids.foreign, workspaceId: ids.foreign }), config, "publish-once-1", transport.request);
  assert.equal(result.ok, true);
  assert.equal(result.release.status, "PUBLISHED");
  assert.equal(transport.calls.length, 5);
  const { url, init } = transport.calls[4];
  assert.equal(url, `http://core.invalid/api/v1/product-releases/${ids.release}/publish`);
  assert.equal(init.method, "POST");
  assert.equal(init.headers["X-Actor-ID"], ids.actor);
  assert.equal(init.headers["Idempotency-Key"], "publish-once-1");
  assert.equal(init.cache, "no-store");
  assert.equal(init.redirect, "error");
  assert.ok(init.signal instanceof AbortSignal);
});

for (const [name, settings] of [
  ["disabled", { enabled: false }],
  ["missing actor", { actorId: null }],
  ["invalid actor", { actorId: "bad" }],
  ["nil actor", { actorId: "00000000-0000-0000-0000-000000000000" }],
  ["missing workspace", { workspaceId: null }],
]) {
  test(`${name}: no upstream request`, async () => {
    const transport = stub([]);
    const result = await executePublish(form(), { ...config, ...settings }, "key", transport.request);
    assert.equal(result.ok, false);
    assert.equal(transport.calls.length, 0);
  });
}

for (const [name, fields, key] of [
  ["invalid product", { productId: "bad" }, "key"],
  ["path injection release", { releaseId: "../other" }, "key"],
  ["nil release", { releaseId: "00000000-0000-0000-0000-000000000000" }, "key"],
  ["blank idempotency", {}, "   "],
]) {
  test(`${name}: no upstream request`, async () => {
    const transport = stub([]);
    const result = await executePublish(form(fields), config, key, transport.request);
    assert.equal(result.ok, false);
    assert.equal(transport.calls.length, 0);
  });
}

test("foreign workspace product blocks before release lookup", async () => {
  const transport = stub([{ items: [{ ...product, workspaceId: ids.foreign }], page: {} }]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 1);
});

test("missing product in workspace blocks before release lookup", async () => {
  const transport = stub([{ items: [], page: {} }]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 1);
});

test("release not in scoped product history blocks before global release read", async () => {
  const transport = stub([{ items: [product] }, { items: [] }]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 2);
});

test("release scoped to another product is blocked", async () => {
  const transport = stub([{ items: [product] }, { items: [{ ...release, productId: ids.foreign }] }]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 2);
});

test("global release response mismatch blocks readiness and write", async () => {
  const transport = stub([{ items: [product] }, { items: [release] }, { ...release, productId: ids.foreign }]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 3);
});

test("readiness response mismatch blocks write", async () => {
  const transport = stub([{ items: [product] }, { items: [release] }, release, { ...readiness, releaseId: ids.foreign }]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 4);
});

for (const [name, releaseValue, readinessValue] of [
  ["release is DRAFT", { ...release, status: "DRAFT" }, readiness],
  ["overall is NOT_READY", release, { ...readiness, overall: "NOT_READY" }],
  ["rights gate failed", release, { ...readiness, checks: { ...checks, rights: "FAIL" }, blockers: ["RIGHTS_BLOCKING"] }],
  ["evidence gate pending", release, { ...readiness, checks: { ...checks, evidence: "PENDING" }, blockers: ["EVIDENCE_INCOMPLETE"] }],
]) {
  test(`${name}: never posts`, async () => {
    const transport = stub([{ items: [product] }, { items: [releaseValue] }, releaseValue, readinessValue]);
    const result = await executePublish(form(), config, "key", transport.request);
    assert.equal(result.ok, false);
    assert.equal(result.refreshRequired, true);
    assert.equal(transport.calls.length, 4);
  });
}

test("409 after publish requires refresh and never retries", async () => {
  const transport = stub([
    { items: [product] }, { items: [release] }, release, readiness,
    Response.json({ error: { code: "PRODUCT_RELEASE_NOT_READY", message: "internal detail" } }, { status: 409 }),
  ]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.match(result.message, /PRODUCT_RELEASE_NOT_READY/);
  assert.doesNotMatch(result.message, /internal detail/);
  assert.equal(transport.calls.length, 5);
});

test("timeout after POST is an unknown outcome and is never retried", async () => {
  const transport = stub([{ items: [product] }, { items: [release] }, release, readiness, new Error("timeout")]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.match(result.message, /可能已送达/);
  assert.equal(transport.calls.length, 5);
});

test("invalid JSON after POST is an unknown outcome", async () => {
  const transport = stub([{ items: [product] }, { items: [release] }, release, readiness, new Response("not-json", { status: 200 })]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.equal(transport.calls.length, 5);
});

test("mismatched published release is not reported as success", async () => {
  const transport = stub([{ items: [product] }, { items: [release] }, release, readiness, { ...published, id: ids.foreign }]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.equal(transport.calls.length, 5);
});

test("read failure cannot trigger a write", async () => {
  const transport = stub([new Error("offline")]);
  const result = await executePublish(form(), config, "key", transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, false);
  assert.equal(transport.calls.length, 1);
});


function createForm(overrides = {}) {
  const data = new FormData();
  const values = {
    productId: ids.product,
    productVersionId: ids.version,
    releaseNo: "R2",
    releaseNotes: "  release notes  ",
    ...overrides,
  };
  for (const [key, value] of Object.entries(values)) data.set(key, value);
  data.append("datasetVersionId", ids.datasetVersion);
  data.append("datasetRole", "OUTPUT");
  return data;
}

const currentProduct = {
  id: ids.product,
  workspaceId: ids.workspace,
  currentVersionId: ids.version,
};
const currentProductVersion = {
  id: ids.version,
  productId: ids.product,
  assets: [{ id: ids.foreign, assetType: "DATASET", datasetId: ids.dataset }],
};
const usableDatasetVersion = {
  id: ids.datasetVersion,
  datasetId: ids.dataset,
  status: "READY",
};
const createdRelease = {
  id: ids.createdRelease,
  productId: ids.product,
  productVersionId: ids.version,
  releaseNo: "R2",
  status: "DRAFT",
  datasets: [{ datasetVersionId: ids.datasetVersion, role: "OUTPUT" }],
};

function createStub(responses) {
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

test("create release: preflights frozen facts and posts explicit bindings once", async () => {
  const transport = createStub([
    currentProduct,
    currentProductVersion,
    usableDatasetVersion,
    { items: [], page: { total: 0, limit: 100, offset: 0 } },
    createdRelease,
  ]);
  const result = await executeCreateRelease(createForm(), config, transport.request);
  assert.equal(result.ok, true);
  assert.equal(result.release.id, ids.createdRelease);
  assert.equal(transport.calls.length, 5);
  const write = transport.calls[4];
  assert.equal(write.url, `http://core.invalid/api/v1/data-products/${ids.product}/releases`);
  assert.equal(write.init.method, "POST");
  assert.equal(write.init.headers["X-Actor-ID"], ids.actor);
  assert.deepEqual(JSON.parse(write.init.body), {
    productVersionId: ids.version,
    releaseNo: "R2",
    datasets: [{ datasetVersionId: ids.datasetVersion, role: "OUTPUT" }],
    releaseNotes: "release notes",
    metadata: {},
  });
});

test("create release: same releaseNo with identical frozen facts recovers without POST", async () => {
  const transport = createStub([
    currentProduct,
    currentProductVersion,
    usableDatasetVersion,
    { items: [{ id: ids.createdRelease, productId: ids.product, releaseNo: "R2" }], page: { total: 1 } },
    createdRelease,
  ]);
  const result = await executeCreateRelease(createForm(), config, transport.request);
  assert.equal(result.ok, true);
  assert.match(result.message, /已存在/);
  assert.equal(transport.calls.length, 5);
  assert.ok(transport.calls.every((call) => call.init?.method !== "POST"));
});

test("create release: stale ProductVersion stops before binding reads", async () => {
  const transport = createStub([{ ...currentProduct, currentVersionId: ids.foreign }]);
  const result = await executeCreateRelease(createForm(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.equal(transport.calls.length, 1);
});

test("create release: DatasetVersion outside ProductVersion assets is blocked", async () => {
  const transport = createStub([
    currentProduct,
    currentProductVersion,
    { ...usableDatasetVersion, datasetId: ids.foreign },
  ]);
  const result = await executeCreateRelease(createForm(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 3);
  assert.ok(transport.calls.every((call) => call.init?.method !== "POST"));
});

test("create release: exactly one PRIMARY or OUTPUT is required before any Core request", async () => {
  const data = createForm();
  data.append("datasetVersionId", ids.foreign);
  data.append("datasetRole", "PRIMARY");
  const transport = createStub([]);
  const result = await executeCreateRelease(data, config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(transport.calls.length, 0);
});

test("create release: timeout after POST is ambiguous and never retried", async () => {
  const transport = createStub([
    currentProduct,
    currentProductVersion,
    usableDatasetVersion,
    { items: [], page: { total: 0 } },
    new Error("timeout"),
  ]);
  const result = await executeCreateRelease(createForm(), config, transport.request);
  assert.equal(result.ok, false);
  assert.equal(result.refreshRequired, true);
  assert.match(result.message, /相同 Release No/);
  assert.equal(transport.calls.length, 5);
});
