import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const { readSection, attentionTotal, validSummary, validPage, validReleaseAttention, validReadiness, validExecution, validReview, validProduct } = require("../.trace-tests/workbench-read.js");
const ws = "11111111-1111-4111-8111-111111111111", id = "22222222-2222-4222-8222-222222222222", time = "2026-10-22T00:00:00Z";
const summary = { workspaceId: ws, counts: { dataResources: 0, datasets: 0, dataProducts: 1 }, reviewQueue: { pending: 0, unresolved: 0, conflicts: 0 }, executions: { queued: 0, submitting: 0, running: 0, succeeded: 0, failed: 0 }, releases: { draft: 0, ready: 0, published: 0, failed: 0 }, updatedAt: time };

test("local read failures, invalid responses and deadlines remain unknown without carrying raw errors", async () => {
  const value = { count: 0 };
  assert.equal(await readSection(async () => value, v => v.count === 0), value);
  for (const load of [() => { throw new Error("secret"); }, async () => { throw new Error("secret"); }, async () => null]) {
    assert.equal(await readSection(load, v => v.count === 0), null);
  }
  assert.equal(await readSection(async () => value, () => false), null);
  assert.equal(await readSection(() => new Promise(() => {}), () => true, 2), null);
});

test("incomplete counts never imply zero or a complete total", () => {
  assert.equal(attentionTotal([0, 0, 0, 0, 0]), 0);
  assert.equal(attentionTotal([1, 0, 2, 0, 0]), 3);
  for (const values of [[], [0, null], [0, undefined], [0, -1], [0, NaN], ["0", 1], [Number.MAX_SAFE_INTEGER, 1]]) assert.equal(attentionTotal(values), null);
});

test("the shared summary is usable only with its complete valid counters and workspace", async () => {
  assert.equal(validSummary(summary, ws), true);
  for (const bad of [{ ...summary, workspaceId: id }, { ...summary, reviewQueue: { ...summary.reviewQueue, pending: -1 } }, { ...summary, executions: {} }, { ...summary, counts: null }]) {
    assert.equal(await readSection(async () => bad, s => validSummary(s, ws)), null);
  }
});

test("page membership, offsets and release classifications cannot turn invalid data into an empty result", () => {
  const release = { id, productId: ws, status: "READY", releaseNo: "R1", createdAt: time };
  const empty = { items: [], page: { total: 0, limit: 25, offset: 0 } };
  const ready = { items: [release], page: { total: 1, limit: 25, offset: 0 } };
  assert.equal(validReleaseAttention({ failed: empty, validating: empty, ready }, 25), true);
  assert.equal(validReleaseAttention({ failed: ready, validating: empty, ready }, 25), false);
  assert.equal(validPage({ ...ready, items: [] }, 25, 0, () => true), false);
  assert.equal(validPage({ ...empty, page: { ...empty.page, total: "0" } }, 25, 0, () => true), false);
  assert.equal(validPage({ items: [], page: { total: 1, limit: 25, offset: 25 } }, 25, 25, () => true), true);
  assert.equal(validReadiness({ releaseId: id, overall: "NOT_READY", blockers: ["RIGHTS"] }, id), true);
  assert.equal(validReadiness({ releaseId: ws, overall: "READY", blockers: [] }, id), false);
});

test("only valid scoped primary identities can be retained when auxiliary labels fail", () => {
  const execution = { id, workspaceId: ws, workflowName: "", workflowCode: "W", workflowVersion: "1", targetPeriod: "2026-10", status: "FAILED", errorCode: "FAILED", attempt: 1, createdAt: time };
  assert.equal(validExecution(execution, ws), true);
  assert.equal(validExecution({ ...execution, workspaceId: id }, ws), false);
  assert.equal(validReview({ candidateId: id, workspaceId: ws, status: "PENDING", sourceName: "", sourceKey: "R", matchMethod: "RULE", confidence: 0.8 }, ws), true);
  assert.equal(validProduct({ id, workspaceId: ws, name: "P" }, ws), true);
  assert.equal(validProduct({ id: "../other", workspaceId: ws, name: "P" }, ws), false);
});
