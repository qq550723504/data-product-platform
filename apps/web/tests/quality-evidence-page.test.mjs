import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import ts from "typescript";

const require = createRequire(import.meta.url);
const attention = require("../.quality-tests/quality-attention.js");
const command = require("../.quality-tests/quality-command.js");
const { QualityReadError } = require("../.quality-tests/quality-transport.js");
const workspace = "11111111-1111-4111-8111-111111111111";
const dataset = "22222222-2222-4222-8222-222222222222";
const version = "33333333-3333-4333-8333-333333333333";
const latest = "44444444-4444-4444-8444-444444444444";
const historical = "55555555-5555-4555-8555-555555555555";
const attempt = "66666666-6666-4666-8666-666666666666";

function evaluate(file, imports) {
  const source = readFileSync(new URL(file, import.meta.url), "utf8");
  const code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX } }).outputText;
  const module = { exports: {} };
  new Function("require", "module", "exports", code)(name => imports[name], module, module.exports);
  return module.exports;
}
const reads = evaluate("../lib/workbench-read.ts", { "./quality-attention": attention });
const report = id => ({ id, workspaceId: workspace, datasetVersionId: version, gateDecision: "PASS", dimensionSummary: {}, findings: { items: [], page: { total: 0 } }, evidence: [], auditEvents: [] });

async function render({ fault, status = 503, query = {} } = {}) {
  const calls = [];
  const fail = () => { throw new QualityReadError(status); };
  const platform = {
    dataset: async () => ({ id: dataset }),
    datasetVersion: async () => ({ id: version, datasetId: dataset, status: "READY" }),
    qualityAssessments: async (_, limit, offset) => {
      if (fault === (limit === 1 ? "summary" : "history")) fail();
      return { datasetVersionId: version, items: [report(latest), report(historical)].slice(offset, offset + limit), page: { total: 2, limit, offset } };
    },
    certificationHistory: async () => ({ profiles: [], items: [], page: { total: 0 } }),
    qualityReport: async id => { calls.push(id); if (fault === id) fail(); return report(id); },
    qualityAttempt: async () => { if (fault === "attempt") fail(); return { id: attempt, workspaceId: workspace, datasetVersionId: fault === "foreign-attempt" ? dataset : version }; },
  };
  const ui = new Proxy({}, { get: () => value => String(value ?? "") });
  const jsx = (type, props) => ({ type, props });
  const page = evaluate("../app/datasets/[id]/versions/[versionId]/page.tsx", {
    "react/jsx-runtime": { jsx, jsxs: jsx, Fragment: "fragment" }, "next/link": { default: "Link" },
    "@/components/ui": ui,
    "@/lib/platform": { platform, configuredWorkspaceId: () => workspace },
    "@/lib/quality-attention": attention, "@/lib/quality-command": command,
    "@/lib/workbench-read": reads, "@/lib/compliance-command": { isComplianceAttemptId: () => false },
    "@/lib/batches": { batchEnabled: () => false, trialRawOnly: () => false },
    "@/components/quality-check-form": { QualityCheckForm: "QualityCheckForm" },
    "@/components/dataset-version-invalidate-form": {}, "@/components/compliance-check-form": {},
    "@/components/certification-form": {}, "@/components/direct-data-delivery-form": {},
  }).default;
  const tree = await page({ params: Promise.resolve({ id: dataset, versionId: version }), searchParams: Promise.resolve({ view: "quality", assessmentId: historical, ...query }) });
  const nodes = [];
  const walk = node => { if (Array.isArray(node)) node.forEach(walk); else if (node?.props) { nodes.push(node); walk(node.props.children); } };
  walk(tree);
  return { calls, nodes, has: id => nodes.some(n => n.props["data-testid"] === id) };
}

for (const fault of ["history", "summary", "attempt", "foreign-attempt", latest]) {
  test(`independent ${fault} failure retains the exact successful historical report`, async () => {
    const result = await render({ fault, query: { assessmentOffset: "25", qualityAttemptId: attempt } });
    assert.equal(result.has("quality-report"), true);
    assert.equal(result.has("selected-quality-assessment"), true);
    assert.equal(result.has("quality-evidence-unavailable"), false);
    assert.deepEqual(result.calls, [historical]);
    if (fault === "history") assert.equal(result.has("quality-history-unavailable"), true);
    if (fault === "summary") assert.equal(result.has("quality-summary-unavailable"), true);
    if (fault.includes("attempt")) {
      assert.equal(result.has("quality-attempt-unavailable"), true);
      assert.equal(result.nodes.some(n => n.type === "QualityCheckForm"), false);
    }
  });
}
for (const status of [401, 403, 404, 503]) {
  test(`historical ${status} stays UNKNOWN without loading latest as a substitute`, async () => {
    const result = await render({ fault: historical, status });
    assert.equal(result.has("quality-report"), false);
    assert.equal(result.has("quality-evidence-unavailable"), true);
    assert.equal(result.has("quality-summary-unavailable"), false);
    assert.equal(result.has("quality-history-unavailable"), false);
    assert.deepEqual(result.calls, [historical]);
  });
}
test("first history page failure does not erase the independently successful latest summary", async () => {
  const result = await render({ fault: "history" });
  assert.equal(result.has("quality-history-unavailable"), true);
  assert.equal(result.has("quality-summary-unavailable"), false);
  assert.equal(result.has("quality-report"), true);
  assert.deepEqual(result.calls, [historical]);
});
test("latest report failure is local when no explicit assessment is selected", async () => {
  const result = await render({ fault: latest, query: { assessmentId: undefined } });
  assert.equal(result.has("quality-evidence-unavailable"), true);
  assert.equal(result.has("quality-summary-unavailable"), false);
  assert.equal(result.has("quality-history-unavailable"), false);
  assert.deepEqual(result.calls, [latest]);
});
test("provenance quality failure remains UNKNOWN and does not invent empty evidence", async () => {
  const result = await render({ fault: latest, query: { view: "provenance" } });
  assert.equal(result.has("quality-provenance-unavailable"), true);
  assert.equal(result.has("generic-provenance-summary"), true);
});
