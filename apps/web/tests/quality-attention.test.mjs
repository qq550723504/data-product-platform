import assert from "node:assert/strict";
import { createRequire } from "node:module";
import test from "node:test";
const require = createRequire(import.meta.url);
const { assessmentHref, assertReportScope, assertCaseScope, attentionOffset, attentionView, qualityAttentionHref, caseHref } = require("../.quality-tests/quality-attention.js");
const ws = "11111111-1111-4111-8111-111111111111";
const version = "22222222-2222-4222-8222-222222222222";
const assessment = "33333333-3333-4333-8333-333333333333";
const dataset = "44444444-4444-4444-8444-444444444444";
const id = "55555555-5555-4555-8555-555555555555";

test("report navigation carries an exact assessment, independently of current/latest", () => {
  assert.equal(assessmentHref(dataset, version, assessment), `/datasets/${dataset}/versions/${version}?view=quality&assessmentId=${assessment}`);
  const report = { id: assessment, workspaceId: ws, datasetVersionId: version, gateDecision: "FAIL" };
  assert.equal(assertReportScope(report, assessment, version, ws), report);
  for (const invalid of [{ ...report, id }, { ...report, workspaceId: id }, { ...report, datasetVersionId: id }]) {
    assert.throws(() => assertReportScope(invalid, assessment, version, ws));
  }
});
test("case focus uses direct identity and rejects workspace/version/identity substitutions", () => {
  assert.equal(caseHref(dataset, version, id), `/datasets/${dataset}/versions/${version}/remediation?caseId=${id}`);
  const detail = { case: { id, workspace_id: ws, version_id: version } };
  assert.equal(assertCaseScope(detail, id, version, ws), detail);
  assert.throws(() => assertCaseScope(detail, assessment, version, ws));
  assert.throws(() => assertCaseScope(detail, id, assessment, ws));
  assert.throws(() => assertCaseScope(detail, id, version, assessment));
});
test("bad identities never become navigation targets", () => {
  for (const bad of ["latest", "../other", "00000000-0000-0000-0000-000000000000", "?consumer=other"]) {
    assert.throws(() => caseHref(dataset, version, bad));
    assert.throws(() => assessmentHref(dataset, version, bad));
  }
});
test("quality view pagination preserves other queues and resets only its own offset", () => {
  const href = qualityAttentionHref({ reviewOffset: "25", failureOffset: "50", qualityOffset: "100" }, "CLOSED", 0);
  const url = new URL(href, "http://local.test");
  assert.equal(url.searchParams.get("reviewOffset"), "25");
  assert.equal(url.searchParams.get("failureOffset"), "50");
  assert.equal(url.searchParams.get("qualityOffset"), null);
  assert.equal(url.searchParams.get("qualityView"), "CLOSED");
  assert.equal(url.hash, "#quality-attention");
  assert.equal(new URL(qualityAttentionHref({}, "ALL", 100), "http://local.test").searchParams.get("qualityOffset"), "100");
});
test("malformed query values cannot produce unsafe page offsets", () => {
  for (const bad of ["-1", "NaN", "Infinity", "2147483648", "0.5"]) assert.equal(attentionOffset(bad), 0);
  assert.equal(attentionOffset("100"), 100);
  assert.equal(attentionView("CLOSED"), "CLOSED");
  assert.equal(attentionView("ALL"), "ALL");
  assert.equal(attentionView("PASS"), "FOLLOW_UP");
});
