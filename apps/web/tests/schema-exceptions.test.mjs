import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const { assertSchemaExceptionPage, schemaExceptionsHref, schemaExceptionEvidenceHref } = require("../.schema-tests/schema-exceptions.js");
const { schemaExceptionFilter } = require("../.schema-tests/schema-exception-filter.js");
const { schemaAttentionHref, assertSchemaPage } = require("../.schema-tests/schema-observation.js");
const { qualityAttentionHref } = require("../.schema-tests/quality-attention.js");
const id = n => `${String(n).padStart(8,"0")}-3490-4349-8349-${String(n).padStart(12,"0")}`;
const stamp = "2026-10-22T00:00:00Z";
function fixture() {
  return { items: [{ workspaceId:id(1),resourceId:id(2),requestId:id(3),observationId:id(4),responsibleActorId:id(5),attemptId:null,status:"DENIED",startedAt:null,completedAt:null,recordedAt:stamp }],
    page:{total:1,limit:25,offset:0},summary:{unavailableCount:0,deniedCount:1,interruptedCount:0},readAt:stamp };
}
test("exception history accepts DENIED with and without an attempt and nullable scan times", () => {
  const page=fixture();
  assert.equal(assertSchemaExceptionPage(page,id(1),"ALL",25,0),page);
  assertSchemaExceptionPage({...page,items:[{...page.items[0],attemptId:id(6)}]},id(1),"DENIED",25,0);
  assert.equal(schemaExceptionFilter("RESOLVED"),"ALL");
});
test("foreign, duplicate, wrong status/filter/count/page and malformed attempt fail closed", () => {
  const page=fixture(), row=page.items[0];
  for(const key of ["workspaceId","resourceId","requestId","observationId","responsibleActorId"]){
    const invalid = key==="workspaceId"?id(9):"bad";
    assert.throws(()=>assertSchemaExceptionPage({...page,items:[{...row,[key]:invalid}]},id(1),"ALL",25,0));
  }
  for(const rowChange of [{status:"DRIFT"},{attemptId:"bad"},{completedAt:"invalid"},{recordedAt:null}]){
    assert.throws(()=>assertSchemaExceptionPage({...page,items:[{...row,...rowChange}]},id(1),"ALL",25,0));
  }
  assert.throws(()=>assertSchemaExceptionPage(page,id(1),"UNAVAILABLE",25,0));
  assert.throws(()=>assertSchemaExceptionPage({...page,page:{...page.page,total:2}},id(1),"ALL",25,0));
  assert.throws(()=>assertSchemaExceptionPage({...page,items:[row,row]},id(1),"ALL",25,0));
  assert.throws(()=>assertSchemaExceptionPage({...page,items:[]},id(1),"ALL",25,0));
  assert.throws(()=>assertSchemaPage({...page,summary:{handledCount:0,unhandledCount:1}},id(1),"ALL",25,0));
});
test("empty page keeps global historical classification counts without inventing resolution", () => {
  const page=fixture();
  assertSchemaExceptionPage({...page,items:[],page:{limit:25,offset:25,total:1}},id(1),"ALL",25,25);
  assertSchemaExceptionPage({...page,items:[],page:{limit:25,offset:0,total:0}},id(1),"INTERRUPTED",25,0);
});
test("exact evidence and return links preserve the history filter/page and existing queue state", () => {
  const query={schemaExceptionStatus:"DENIED",schemaExceptionOffset:"100",qualityView:"CLOSED",qualityOffset:"25",schemaView:"ALL",schemaOffset:"50"};
  const url=new URL(schemaExceptionsHref(query,"INTERRUPTED",25),"http://test");
  assert.equal(url.searchParams.get("schemaExceptionStatus"),"INTERRUPTED");
  assert.equal(url.searchParams.get("schemaExceptionOffset"),"25");
  assert.equal(url.searchParams.get("qualityView"),"CLOSED");
  assert.equal(url.searchParams.get("schemaOffset"),"50");
  for(const href of [qualityAttentionHref(query,"ALL",0),schemaAttentionHref(query,"UNHANDLED",0)]){
    const u=new URL(href,"http://test");
    assert.equal(u.searchParams.get("schemaExceptionStatus"),"DENIED");
    assert.equal(u.searchParams.get("schemaExceptionOffset"),"100");
  }
  const exact=new URL(schemaExceptionEvidenceHref(fixture().items[0],"DENIED",100),"http://test");
  assert.equal(exact.pathname,`/resources/${id(2)}`);
  for(const [key,value] of Object.entries({requestId:id(3),observationId:id(4),schemaExceptionStatus:"DENIED",schemaExceptionOffset:"100"}))assert.equal(exact.searchParams.get(key),value);
});
