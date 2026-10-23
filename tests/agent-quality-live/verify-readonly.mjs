// Verify recorded local evidence. This does not connect to Core or the database.
import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
const root = new URL("../../.data/agent-q1-live/", import.meta.url);
const json = async name => JSON.parse((await readFile(new URL(name, root), "utf8")).replace(/^\uFEFF/, ""));
const [before, after, role] = await Promise.all([
  json("evidence/database-before.json"), json("evidence/database-after.json"), json("evidence/database-role.json"),
]);
assert.ok(before.length > 0);
assert.deepEqual(after, before, "Public table row counts and content hashes must remain identical");
assert.equal(role.user, "agent_q1_reader");
assert.equal(role.readOnly, "on");
assert.equal(role.superuser, false);
assert.equal(role.publicSchemaCreate, false);
assert.equal(role.publicTables, before.length);
assert.equal(role.readableTables, role.publicTables);
assert.equal(role.writableTables, 0);
const traceText = await readFile(new URL("core-requests.jsonl", root), "utf8");
assert.ok(!traceText.includes("agent-q1-live-secret"));
const trace = traceText.trim().split("\n").map(JSON.parse);
const requests = trace.filter(e => e.phase === "request");
assert.ok(requests.length > 0);
assert.ok(requests.some(e => e.path.includes("/agent-quality-context")));
assert.ok(requests.some(e => e.path.includes("/quality-assessments/")));
assert.ok(requests.every(e => e.method === "GET"), "Next must not send any business write request");
assert.ok(trace.every(e => Object.keys(e).every(k => ["at", "phase", "method", "path", "status"].includes(k))));
const group = key => Object.fromEntries([...new Set(key)].sort().map(k => [k, key.filter(x => x === k).length]));
const summary = {
  result: "PASS", comparedTables: before.length, changedTables: [],
  coreRequests: requests.length, methods: group(requests.map(e => e.method)),
  routes: group(requests.map(e => e.path.split("?")[0])),
  responseStatuses: group(trace.filter(e => e.phase === "response").map(e => String(e.status))),
  role, note: "Real Core with persisted synthetic historical fixtures; no engine/model execution.",
};
await writeFile(new URL("evidence/readonly-summary.json", root), JSON.stringify(summary, null, 2) + "\n");
console.log(JSON.stringify(summary, null, 2));
console.log("Evidence: " + fileURLToPath(new URL("evidence/readonly-summary.json", root)));
