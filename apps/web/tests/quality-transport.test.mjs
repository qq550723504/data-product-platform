import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const { qualityGet, QualityReadError } = require("../.quality-tests/quality-transport.js");
const { executeQualityCheck } = require("../.quality-tests/quality-command.js");

test("missing Quality credential never makes an anonymous read", async () => {
  let calls = 0;
  await assert.rejects(qualityGet("/api/v1/quality-assessments/exact/report?workspaceId=ws", "http://core.test", " ", async () => { calls++; }), /UNKNOWN/);
  assert.equal(calls, 0);
});

for (const status of [401, 403, 404, 503]) {
  test(`Quality ${status} preserves exact URL and never falls back or reveals upstream body`, async () => {
    const path = "/api/v1/quality-assessments/historical-id/report?workspaceId=ws&offset=50";
    let calls = 0;
    await assert.rejects(qualityGet(path, "http://core.test/", "read-secret", async (url, init) => {
      calls++;
      assert.equal(url, `http://core.test${path}`);
      assert.equal(init.headers.Authorization, "Bearer read-secret");
      assert.equal(init.redirect, "error");
      return Response.json({ message: "read-secret and protected report" }, { status });
    }), error => error instanceof QualityReadError && error.status === status && /UNKNOWN/.test(error.message) && !/secret|protected/.test(error.message));
    assert.equal(calls, 1);
  });
}

test("missing command credential blocks before all Core requests", async () => {
  let calls = 0;
  const result = await executeQualityCheck(new FormData(), { enabled: true, workspaceId: "11111111-1111-4111-8111-111111111111", apiBaseUrl: "http://core.test" }, async () => { calls++; });
  assert.equal(result.ok, false);
  assert.equal(calls, 0);
});
