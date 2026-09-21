import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import commands from "../.ingest-tests/ingest-command.js";
const { importCSV, startImportedResolution: start } = commands;
const ids = Object.fromEntries(["workspace", "actor", "operation", "resource", "raw", "version", "output", "job", "foreign"].map((name, i) => [name, `${String(i+1).repeat(8)}-${String(i+1).repeat(4)}-4${String(i+1).repeat(3)}-8${String(i+1).repeat(3)}-${String(i+1).repeat(12)}`]));
const config = { enabled: true, workspaceId: ids.workspace, actorId: ids.actor, apiBaseUrl: "http://core.invalid" };
const code = `CSV-${ids.operation}`;
const text = '\ufeff"source_company_id","company_name"\r\na,"合成,主体"\r\n';
const bytes = Buffer.from(text), checksum = createHash("sha256").update(bytes).digest("hex");
const raw = { id: ids.raw, workspaceId: ids.workspace, datasetType: "RAW", code, name: "synthetic" };
const resource = { id: ids.resource, workspaceId: ids.workspace, resourceType: "TABLE_LIKE", code };
const version = { id: ids.version, datasetId: ids.raw, status: "READY", rowCount: 1, byteSize: bytes.length, checksumAlgorithm: "SHA256", checksum, storageUri: `s3://test/datasets/file/company-import-${ids.operation}.csv` };
const output = { id: ids.output, workspaceId: ids.workspace, datasetType: "STANDARDIZED", code: `RESOLVED-${ids.version}` };
const job = { id: ids.job, workspaceId: ids.workspace, inputDatasetVersionId: ids.version, outputDatasetId: ids.output, status: "WAITING_REVIEW" };
const page = (items, total = items.length, offset = 0) => ({ items, page: { total, offset, limit: 100 } });
function form(overrides = {}) {
  const result = new FormData();
  for (const [key, value] of Object.entries({name:"测试文件", source:"合成生成", purpose:"接入测试", acknowledged:"on", ...overrides})) result.set(key, value);
  result.set("file", new Blob([bytes], { type:"text/csv" }), "input.csv"); return result;
}
function resolution(overrides = {}) {
  const result = new FormData();
  for (const [key,value] of Object.entries({versionId:ids.version, acknowledged:"on",...overrides})) result.set(key,value);
  return result;
}
function stub(values) {
  const calls = [];
  return { calls, request: async (url, init) => {
    calls.push({url,init}); const value = values.shift();
    assert.notEqual(value, undefined, "unexpected request or automatic retry");
    if (value instanceof Error) throw value;
    return value instanceof Response ? value : Response.json(value);
  }};
}
test("upload preserves original bytes, scopes records and ignores browser actor/workspace", async () => {
  const t = stub([page([]),resource,raw,version]);
  const result = await importCSV(ids.operation, form({ actorId:ids.foreign, workspaceId:ids.foreign }), config,t.request);
  assert.equal(result.ok,true); assert.equal(result.checksum,checksum); assert.equal(result.versionId,ids.version);
  assert.equal(t.calls.length,4);
  for (const call of t.calls.slice(1)) { assert.equal(call.init.headers["X-Actor-ID"],ids.actor); assert.equal(call.init.redirect,"error"); }
  assert.equal(JSON.parse(t.calls[1].init.body).workspaceId,ids.workspace);
  const file=t.calls[3].init.body.get("file"); assert.deepEqual(Buffer.from(await file.arrayBuffer()),bytes);
  assert.equal(file.name,`company-import-${ids.operation}.csv`);
  assert.equal(t.calls.some(c=>c.url.includes("entity-match-jobs")),false);
});
for(const [name,settings] of [["disabled",{enabled:false}],["actor missing",{actorId:null}],["workspace missing",{workspaceId:null}],["actor invalid",{actorId:"bad"}]]) {
  test(`${name} prevents all network access`,async()=>{
    const t=stub([]);assert.equal((await importCSV(ids.operation,form(),{...config,...settings},t.request)).ok,false);
    assert.equal((await start(resolution(),{...config,...settings},t.request)).ok,false);assert.equal(t.calls.length,0);
  });
}
for(const fields of [{name:" "},{source:""},{purpose:" "},{acknowledged:"no"},{name:"a".repeat(121)}]) {
  test(`invalid form ${JSON.stringify(fields)} never writes`,async()=>{
    const t=stub([]); assert.equal((await importCSV(ids.operation,form(fields),config,t.request)).ok,false);assert.equal(t.calls.length,0);
  });
}
for(const [name,fileName,body] of [["extension","file.xlsx",bytes],["empty","file.csv",Buffer.alloc(0)],["invalid CSV","file.csv",Buffer.from("a,b\nc,d")],["size","file.csv",Buffer.alloc(512*1024+1)]]) {
  test(`${name} blocked on server regardless of browser preview`,async()=>{
    const t=stub([]), f=form();f.set("file",new Blob([body]),fileName);
    assert.equal((await importCSV(ids.operation,f,config,t.request)).ok,false);assert.equal(t.calls.length,0);
  });
}
test("same operation with existing RAW never writes again",async()=>{
  const t=stub([page([raw])]);const result=await importCSV(ids.operation,form(),config,t.request);
  assert.equal(result.locked,true);assert.equal(result.datasetId,ids.raw);assert.equal(t.calls.length,1);
});
for(const [name,last] of [["timeout",new Error("secret network detail")],["HTTP",new Response("secret SQL",{status:500})]]) {
  test(`${name}: partial object identities survive, no blind retry`,async()=>{
    const t=stub([page([]),resource,raw,last]);const result=await importCSV(ids.operation,form(),config,t.request);
    assert.equal(result.ok,false);assert.equal(result.locked,true);assert.equal(result.resourceId,ids.resource);assert.equal(result.datasetId,ids.raw);
    assert.match(result.message,/可能部分成功/);assert.doesNotMatch(result.message,/secret/);assert.equal(t.calls.length,4);
  });
}
test("mismatched RAW checksum is not success",async()=>{
  const t=stub([page([]),resource,raw,{...version,checksum:"0".repeat(64)}]);const result=await importCSV(ids.operation,form(),config,t.request);
  assert.equal(result.ok,false);assert.equal(result.versionId,ids.version);assert.equal(result.locked,true);
});
test("foreign creation response stops the chain",async()=>{
  const t=stub([page([]),{...resource,workspaceId:ids.foreign}]);assert.equal((await importCSV(ids.operation,form(),config,t.request)).ok,false);assert.equal(t.calls.length,2);
});
test("start verifies input scope and fixes policy/source role on server",async()=>{
  const t=stub([page([raw]),version,output,job]);const result=await start(resolution({actorId:ids.foreign,policyRef:"../../evil",sourceRole:"evil"}),config,t.request);
  assert.equal(result.ok,true);assert.equal(result.jobId,ids.job);assert.equal(t.calls.length,4);
  assert.match(t.calls[1].url,new RegExp(`/api/v1/dataset-versions/${ids.version}\\?workspaceId=${ids.workspace}import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import commands from "../.ingest-tests/ingest-command.js";
const { importCSV, startImportedResolution: start } = commands;
const ids = Object.fromEntries(["workspace", "actor", "operation", "resource", "raw", "version", "output", "job", "foreign"].map((name, i) => [name, `${String(i+1).repeat(8)}-${String(i+1).repeat(4)}-4${String(i+1).repeat(3)}-8${String(i+1).repeat(3)}-${String(i+1).repeat(12)}`]));
const config = { enabled: true, workspaceId: ids.workspace, actorId: ids.actor, apiBaseUrl: "http://core.invalid" };
const code = `CSV-${ids.operation}`;
const text = '\ufeff"source_company_id","company_name"\r\na,"合成,主体"\r\n';
const bytes = Buffer.from(text), checksum = createHash("sha256").update(bytes).digest("hex");
const raw = { id: ids.raw, workspaceId: ids.workspace, datasetType: "RAW", code, name: "synthetic" };
const resource = { id: ids.resource, workspaceId: ids.workspace, resourceType: "TABLE_LIKE", code };
const version = { id: ids.version, datasetId: ids.raw, status: "READY", rowCount: 1, byteSize: bytes.length, checksumAlgorithm: "SHA256", checksum, storageUri: `s3://test/datasets/file/company-import-${ids.operation}.csv` };
const output = { id: ids.output, workspaceId: ids.workspace, datasetType: "STANDARDIZED", code: `RESOLVED-${ids.version}` };
const job = { id: ids.job, workspaceId: ids.workspace, inputDatasetVersionId: ids.version, outputDatasetId: ids.output, status: "WAITING_REVIEW" };
const page = (items, total = items.length, offset = 0) => ({ items, page: { total, offset, limit: 100 } });
function form(overrides = {}) {
  const result = new FormData();
  for (const [key, value] of Object.entries({name:"测试文件", source:"合成生成", purpose:"接入测试", acknowledged:"on", ...overrides})) result.set(key, value);
  result.set("file", new Blob([bytes], { type:"text/csv" }), "input.csv"); return result;
}
function resolution(overrides = {}) {
  const result = new FormData();
  for (const [key,value] of Object.entries({versionId:ids.version, acknowledged:"on",...overrides})) result.set(key,value);
  return result;
}
function stub(values) {
  const calls = [];
  return { calls, request: async (url, init) => {
    calls.push({url,init}); const value = values.shift();
    assert.notEqual(value, undefined, "unexpected request or automatic retry");
    if (value instanceof Error) throw value;
    return value instanceof Response ? value : Response.json(value);
  }};
}
test("upload preserves original bytes, scopes records and ignores browser actor/workspace", async () => {
  const t = stub([page([]),resource,raw,version]);
  const result = await importCSV(ids.operation, form({ actorId:ids.foreign, workspaceId:ids.foreign }), config,t.request);
  assert.equal(result.ok,true); assert.equal(result.checksum,checksum); assert.equal(result.versionId,ids.version);
  assert.equal(t.calls.length,4);
  for (const call of t.calls.slice(1)) { assert.equal(call.init.headers["X-Actor-ID"],ids.actor); assert.equal(call.init.redirect,"error"); }
  assert.equal(JSON.parse(t.calls[1].init.body).workspaceId,ids.workspace);
  const file=t.calls[3].init.body.get("file"); assert.deepEqual(Buffer.from(await file.arrayBuffer()),bytes);
  assert.equal(file.name,`company-import-${ids.operation}.csv`);
  assert.equal(t.calls.some(c=>c.url.includes("entity-match-jobs")),false);
});
for(const [name,settings] of [["disabled",{enabled:false}],["actor missing",{actorId:null}],["workspace missing",{workspaceId:null}],["actor invalid",{actorId:"bad"}]]) {
  test(`${name} prevents all network access`,async()=>{
    const t=stub([]);assert.equal((await importCSV(ids.operation,form(),{...config,...settings},t.request)).ok,false);
    assert.equal((await start(resolution(),{...config,...settings},t.request)).ok,false);assert.equal(t.calls.length,0);
  });
}
for(const fields of [{name:" "},{source:""},{purpose:" "},{acknowledged:"no"},{name:"a".repeat(121)}]) {
  test(`invalid form ${JSON.stringify(fields)} never writes`,async()=>{
    const t=stub([]); assert.equal((await importCSV(ids.operation,form(fields),config,t.request)).ok,false);assert.equal(t.calls.length,0);
  });
}
for(const [name,fileName,body] of [["extension","file.xlsx",bytes],["empty","file.csv",Buffer.alloc(0)],["invalid CSV","file.csv",Buffer.from("a,b\nc,d")],["size","file.csv",Buffer.alloc(512*1024+1)]]) {
  test(`${name} blocked on server regardless of browser preview`,async()=>{
    const t=stub([]), f=form();f.set("file",new Blob([body]),fileName);
    assert.equal((await importCSV(ids.operation,f,config,t.request)).ok,false);assert.equal(t.calls.length,0);
  });
}
test("same operation with existing RAW never writes again",async()=>{
  const t=stub([page([raw])]);const result=await importCSV(ids.operation,form(),config,t.request);
  assert.equal(result.locked,true);assert.equal(result.datasetId,ids.raw);assert.equal(t.calls.length,1);
});
for(const [name,last] of [["timeout",new Error("secret network detail")],["HTTP",new Response("secret SQL",{status:500})]]) {
  test(`${name}: partial object identities survive, no blind retry`,async()=>{
    const t=stub([page([]),resource,raw,last]);const result=await importCSV(ids.operation,form(),config,t.request);
    assert.equal(result.ok,false);assert.equal(result.locked,true);assert.equal(result.resourceId,ids.resource);assert.equal(result.datasetId,ids.raw);
    assert.match(result.message,/可能部分成功/);assert.doesNotMatch(result.message,/secret/);assert.equal(t.calls.length,4);
  });
}
test("mismatched RAW checksum is not success",async()=>{
  const t=stub([page([]),resource,raw,{...version,checksum:"0".repeat(64)}]);const result=await importCSV(ids.operation,form(),config,t.request);
  assert.equal(result.ok,false);assert.equal(result.versionId,ids.version);assert.equal(result.locked,true);
});
test("foreign creation response stops the chain",async()=>{
  const t=stub([page([]),{...resource,workspaceId:ids.foreign}]);assert.equal((await importCSV(ids.operation,form(),config,t.request)).ok,false);assert.equal(t.calls.length,2);
});
test("start verifies input scope and fixes policy/source role on server",async()=>{
  const t=stub([page([raw]),version,output,job]);const result=await start(resolution({actorId:ids.foreign,policyRef:"../../evil",sourceRole:"evil"}),config,t.request);
));
  const body=JSON.parse(t.calls[3].init.body);
  assert.equal(body.workspaceId,ids.workspace);assert.equal(body.policyRef,"park/matching/company-match-policy-v1.yaml");assert.equal(body.sourceRole,"ANCHOR");
  assert.equal(body.sourceRef,`company-import-${ids.operation}.csv`);assert.equal(t.calls[3].init.headers["X-Actor-ID"],ids.actor);
});
for(const [name,rows,payload] of [
 ["foreign workspace",[{...raw,workspaceId:ids.foreign}],version],
 ["foreign version dataset",[raw],{...version,datasetId:ids.foreign}],
 ["non RAW",[{...raw,datasetType:"CURATED"}],version],
 ["invalidated version",[raw],{...version,status:"INVALIDATED"}],
 ["wrong source",[raw],{...version,storageUri:"s3://test/foreign.csv"}],
 ["wrong version identity",[raw],{...version,id:ids.foreign}],
]) test(`${name} never posts a parse job`,async()=>{
  const t=stub([page(rows),payload]);assert.equal((await start(resolution(),config,t.request)).ok,false);
  assert.equal(t.calls.filter(c=>c.init.method==="POST").length,0);
});
test("reserved output prevents repeating job creation",async()=>{
  const t=stub([page([raw,output]),version]);const result=await start(resolution(),config,t.request);
  assert.equal(result.locked,true);assert.equal(result.outputDatasetId,ids.output);assert.equal(t.calls.length,2);
});
test("job POST timeout keeps output identity and locks, not retry",async()=>{
  const t=stub([page([raw]),version,output,new Error("timeout")]);const result=await start(resolution(),config,t.request);
  assert.equal(result.ok,false);assert.equal(result.locked,true);assert.equal(result.outputDatasetId,ids.output);assert.equal(t.calls.length,4);
});
test("wrong job scope is not reported as success",async()=>{
  const t=stub([page([raw]),version,output,{...job,workspaceId:ids.foreign}]);assert.equal((await start(resolution(),config,t.request)).ok,false);
});
test("input enumeration drains more than the first page",async()=>{
  const other={...raw,id:ids.foreign,code:"other"}; const t=stub([page([other],2,0),page([raw],2,1),version,output,job]);
  assert.equal((await start(resolution(),config,t.request)).ok,true);assert.match(t.calls[1].url,/offset=1$/);
});
test("empty incomplete page never reaches a write",async()=>{
  const t=stub([page([],1)]);assert.equal((await start(resolution(),config,t.request)).ok,false);assert.equal(t.calls.length,1);
});
