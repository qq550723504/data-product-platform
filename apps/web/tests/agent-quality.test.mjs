import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFileSync } from "node:fs";
const require = createRequire(import.meta.url);
const {
  agentSelection,
  agentHref,
  agentContextGet,
  validAgentContext,
  readAgentEvidence,
} = require("../.agent-tests/agent-quality.js");
const ws = "11111111-1111-4111-8111-111111111111";
const ds = "22222222-2222-4222-8222-222222222222";
const version = "33333333-3333-4333-8333-333333333333";
const old = "44444444-4444-4444-8444-444444444444";
const newer = "55555555-5555-4555-8555-555555555555";
const attemptId = "66666666-6666-4666-8666-666666666666";
const page = (items, limit = 25, offset = 0, total = items.length) => ({
  items,
  page: { limit, offset, total },
});
const assessment = {
  id: old,
  workspaceId: ws,
  datasetVersionId: version,
  ruleSetRef: "fixture/rules",
  ruleSetVersion: "v1",
  createdAt: "2026-10-23T00:00:00Z",
  gateDecision: "BLOCKED",
};
const report = {
  ...assessment,
  findings: page([]),
  evidence: [],
  auditEvents: [],
};
const selection = agentSelection({
  datasetId: ds,
  versionId: version,
  assessmentId: old,
  attemptId,
});
const context = {
  workspaceId: ws,
  readAt: assessment.createdAt,
  mode: "READ_ONLY",
  dataset: {
    id: ds,
    workspaceId: ws,
    name: "Source",
    code: "SOURCE",
    datasetType: "RAW",
  },
  version: {
    id: version,
    datasetId: ds,
    versionNo: 1,
    status: "READY",
    createdAt: assessment.createdAt,
  },
  template: { configured: false, inputs: [] },
  executionReadiness: "NOT_EVALUATED",
  unverified: [],
};

test("selection rejects ambiguous identities, unsafe pagination and unscoped evidence", () => {
  for (const query of [
    { versionId: "bad" },
    { versionId: [version] },
    { assessmentId: old },
    { offset: "-1" },
    { offset: "2147483648" },
    { offset: "1e2" },
  ])
    assert.throws(() => agentSelection(query));
  const href = agentHref({
    ...selection,
    historyOffset: 25,
    findingOffset: 50,
  });
  const parsed = Object.fromEntries(new URL(href, "http://local").searchParams);
  assert.deepEqual(agentSelection(parsed), {
    ...selection,
    historyOffset: 25,
    findingOffset: 50,
  });
});

test("context credentials are required before fetch; route is fixed and redirects forbidden", async () => {
  let calls = 0;
  await assert.rejects(
    agentContextGet(ws, selection, "http://core", undefined, async () => {
      calls++;
    }),
    /UNKNOWN/,
  );
  assert.equal(calls, 0);
  const result = await agentContextGet(
    ws,
    selection,
    "http://core/",
    "server-only",
    async (url, init) => {
      calls++;
      assert.equal(
        new URL(url).pathname,
        `/api/v1/workspaces/${ws}/agent-quality-context`,
      );
      assert.equal(new URL(url).searchParams.get("versionId"), version);
      assert.equal(init.headers.Authorization, "Bearer server-only");
      assert.equal(init.redirect, "error");
      assert.equal(init.cache, "no-store");
      return Response.json(context);
    },
  );
  assert.equal(calls, 1);
  assert.equal(validAgentContext(result, ws, selection), true);
});
for (const status of [401, 403, 404, 503])
  test(`context ${status} never exposes upstream content or falls back`, async () => {
    let calls = 0;
    await assert.rejects(
      agentContextGet(ws, selection, "http://core", "server-only", async () => {
        calls++;
        return Response.json({ message: "private-token-storage" }, { status });
      }),
      (e) => /UNKNOWN/.test(e.message) && !e.message.includes("private"),
    );
    assert.equal(calls, 1);
  });
test("source, template and workspace scope must be comparable", () => {
  for (const changed of [
    { workspaceId: ds },
    { version: { ...context.version, id: old } },
    { dataset: { ...context.dataset, workspaceId: ds } },
    { template: { ...context.template, configured: true } },
    { executionReadiness: "READY" },
  ])
    assert.equal(
      validAgentContext({ ...context, ...changed }, ws, selection),
      false,
    );
});
test("exact old report survives failed history and attempt; no latest lookup", async () => {
  const calls = [];
  const result = await readAgentEvidence(ws, selection, {
    history: async () => {
      calls.push("history");
      throw new Error("private");
    },
    report: async (id, limit, offset) => {
      calls.push(id);
      assert.equal(id, old);
      assert.equal(limit, 25);
      assert.equal(offset, 0);
      return report;
    },
    attempt: async () => {
      calls.push("attempt");
      throw new Error("private");
    },
  });
  assert.equal(result.report.id, old);
  assert.equal(result.history, null);
  assert.equal(result.attempt, null);
  assert.equal(calls.filter((c) => c === old).length, 1);
});
test("report mismatch cannot replace exact identity; known history stays visible", async () => {
  for (const changed of [
    { id: newer },
    { workspaceId: ds },
    { datasetVersionId: ds },
    {
      findings: page([
        { id: newer, resultId: newer, ruleId: "r", status: "FAIL" },
      ]),
    },
    { evidence: [{ id: newer, workspaceId: ds }] },
  ]) {
    const result = await readAgentEvidence(ws, selection, {
      history: async () => ({
        datasetVersionId: version,
        ...page([assessment]),
      }),
      report: async () => ({ ...report, ...changed }),
      attempt: async () => {
        throw new Error("unknown");
      },
    });
    assert.equal(result.report, null);
    assert.equal(result.history.items[0].id, old);
  }
});
test("no selected report triggers no implicit latest or report request", async () => {
  const result = await readAgentEvidence(
    ws,
    agentSelection({ versionId: version }),
    {
      history: async () => ({ datasetVersionId: version, ...page([]) }),
      report: async () => {
        throw new Error("must not call");
      },
      attempt: async () => {
        throw new Error("must not call");
      },
    },
  );
  assert.equal(result.report, null);
  assert.equal(result.history.page.total, 0);
});

test("expanded evidence and audit facts require renderable fields and the exact assessment subject",async()=>{
 const full={...report,evidence:[{id:newer,workspaceId:ws,evidenceType:'QUALITY_RESULT',relationType:'EVIDENCE_FOR',sourceType:'QUALITY_RESULT',sourceId:old,createdAt:assessment.createdAt}],auditEvents:[{id:newer,objectId:old,objectType:'QUALITY_RESULT',action:'QUALITY_CHECK_COMPLETED',actorType:'USER',occurredAt:assessment.createdAt}]};
 const reads={history:async()=>({datasetVersionId:version,...page([assessment])}),report:async()=>full,attempt:async()=>{throw Error('unknown')}};
 assert.equal((await readAgentEvidence(ws,selection,reads)).report.id,old);
 assert.equal((await readAgentEvidence(ws,selection,{...reads,report:async()=>({...full,auditEvents:[{...full.auditEvents[0],objectId:newer}]})})).report,null);
});

test("all template and selected report display fields reject objects before rendering",async()=>{
 const template={configured:true,name:'repair',version:'v1',hash:'a'.repeat(64),workflowHash:'b'.repeat(64),inputs:[]};
 for(const key of ['name','version','hash','workflowHash'])assert.equal(validAgentContext({...context,template:{...template,[key]:{}}},ws,selection),false,key);
 const finding={id:newer,resultId:old,ruleId:'r',severity:'CRITICAL',status:'FAIL',reason:'known reason'};
 const reads={history:async()=>({datasetVersionId:version,...page([assessment])}),report:async()=>report,attempt:async()=>({id:attemptId,workspaceId:ws,datasetVersionId:version,outcome:'SUCCEEDED'})};
 for(const changed of [
  ...['ruleSetVersion','createdAt'].map(key=>({...report,[key]:{}})),
  ...['reason','severity'].map(key=>({...report,findings:page([{...finding,[key]:{}}])})),
 ]){
  const result=await readAgentEvidence(ws,selection,{...reads,report:async()=>changed});
  assert.equal(result.report,null);assert.equal(result.history.items[0].id,old);assert.equal(result.attempt.id,attemptId);
 }
 const result=await readAgentEvidence(ws,selection,{...reads,history:async()=>({datasetVersionId:version,...page([{...assessment,ruleSetRef:{}}])})});
 assert.equal(result.history,null);assert.equal(result.report.id,old);
});

async function renderQuality({source=context,history=assessment,selected=report}={}){
 const ts=require('typescript'),React=require('react');
 const sourceCode=readFileSync(new URL('../components/core-quality-view.tsx',import.meta.url),'utf8');
 const code=ts.transpileModule(sourceCode,{compilerOptions:{jsx:ts.JsxEmit.ReactJSX,module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText;
 const calls=[];
 const modules={
  'react/jsx-runtime':require('react/jsx-runtime'),
  './core-quality-cost':{default:()=>null},
  '@/components/ui':{
   Badge:({value})=>React.createElement('span',null,value),
   BackLink:({children,href})=>React.createElement('a',{href},children),
   EmptyState:({title,description})=>React.createElement('p',null,title,description),
   PageHeader:({title})=>React.createElement('h1',null,title),
  },
  '@/lib/platform':{configuredWorkspaceId:()=>ws,platform:{
   qualityAssessments:async()=>({datasetVersionId:version,...page([history])}),
   qualityReport:async id=>{calls.push(id);return selected;},
   qualityAttempt:async()=>({id:attemptId,workspaceId:ws,datasetVersionId:version,outcome:'SUCCEEDED'}),
  }},
  '@/lib/workbench-read':require('../.agent-tests/workbench-read.js'),
  '@/lib/agent-quality-server':{readAgentContext:async()=>source},
  '@/lib/agent-quality':require('../.agent-tests/agent-quality.js'),
 };
 const mod={exports:{}};new Function('require','module','exports',code)(name=>{assert.ok(name in modules,name);return modules[name];},mod,mod.exports);
 const element=await mod.exports.default({searchParams:Promise.resolve({datasetId:ds,versionId:version,assessmentId:old,attemptId})});
 return {html:require('react-dom/server').renderToStaticMarkup(element),calls};
}

test('actual CoreQualityView renders malformed report/history as local UNKNOWN with known siblings intact',async()=>{
 const finding={id:newer,resultId:old,ruleId:'r',severity:'CRITICAL',status:'FAIL',reason:'known reason'};
 for(const selected of [
  {...report,ruleSetVersion:{}},
  ...['reason','severity'].map(key=>({...report,findings:page([{...finding,[key]:{}}])})),
 ]){
  const {html,calls}=await renderQuality({selected});
  assert.match(html,/UNKNOWN/);assert.match(html,/所选报告/);
  assert.ok(html.includes(version));assert.match(html,/fixture\/rules/);assert.match(html,/SUCCEEDED/);
  assert.deepEqual(calls,[old]);
 }
 const {html,calls}=await renderQuality({history:{...assessment,ruleSetRef:{}}});
 assert.match(html,/评测列表读取失败/);assert.match(html,/fixture\/rules/);assert.match(html,/v1/);assert.ok(html.includes(version));assert.deepEqual(calls,[old]);
});

test('actual CoreQualityView rejects malformed configured template before rendering and reads no unrelated evidence',async()=>{
 for(const key of ['name','version']){
  const {html,calls}=await renderQuality({source:{...context,template:{configured:true,name:'repair',version:'v1',hash:'a'.repeat(64),workflowHash:'b'.repeat(64),inputs:[],[key]:{}}}});
  assert.match(html,/UNKNOWN/);assert.deepEqual(calls,[]);
 }
});
