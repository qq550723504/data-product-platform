import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const { schemaObservationHref, schemaAttentionHref, schemaView, assertSchemaPage, assertSchemaObservation } = require("../.schema-tests/schema-observation.js");
const { qualityAttentionHref } = require("../.schema-tests/quality-attention.js");
const id = n => `${String(n).padStart(8,"0")}-3460-4346-8346-${String(n).padStart(12,"0")}`;
const row = { workspaceId:id(1),resourceId:id(2),requestId:id(3),observationId:id(4),attemptId:id(5),responsibleActorId:id(6),status:"DRIFT",handlingCount:0 };
const page = {items:[row],page:{limit:25,offset:0,total:1},summary:{unhandledCount:1,handledCount:0}};

test("exact history links bind resource, request and observation; unsafe identities never navigate",()=>{
  assert.equal(schemaObservationHref(id(2),id(3),id(4)),`/resources/${id(2)}?view=source-schema&requestId=${id(3)}&observationId=${id(4)}`);
  for(const bad of ["latest","../other","?view=other","00000000-0000-0000-0000-000000000000"]){
    for(let i=0;i<3;i++){const values=[id(2),id(3),id(4)];values[i]=bad;assert.throws(()=>schemaObservationHref(...values));}
  }
});
test("exact evidence rejects substitutions of every scope dimension and handling membership",()=>{
  const detail={...row,handling:[]};
  assert.equal(assertSchemaObservation(detail,id(1),id(2),id(3),id(4)),detail);
  for(const key of ["workspaceId","resourceId","requestId","observationId"]){assert.throws(()=>assertSchemaObservation({...detail,[key]:id(9)},id(1),id(2),id(3),id(4)));}
  assert.throws(()=>assertSchemaObservation({...detail,handlingCount:1,handling:[{observationId:id(9)}]},id(1),id(2),id(3),id(4)));
  assert.throws(()=>assertSchemaObservation(detail,id(1),id(2),"",id(4)));
});
test("summary validates workspace, DRIFT-only scope, counts and nonduplicated observations",()=>{
  assert.equal(assertSchemaPage(page,id(1),"UNHANDLED",25,0),page);
  for(const changed of [{workspaceId:id(8)},{requestId:"latest"},{attemptId:null},{status:"MATCH"},{handlingCount:1}]) {
    assert.throws(()=>assertSchemaPage({...page,items:[{...row,...changed}]},id(1),"UNHANDLED",25,0));
  }
  assert.throws(()=>assertSchemaPage({...page,items:[row,row]},id(1),"UNHANDLED",25,0));
  assert.throws(()=>assertSchemaPage({...page,summary:{unhandledCount:0,handledCount:0}},id(1),"UNHANDLED",25,0));
  assert.throws(()=>assertSchemaPage(page,id(1),"UNHANDLED",25,100));
  const handled={...page,items:[{...row,handlingCount:3}],summary:{unhandledCount:0,handledCount:1}};
  assert.equal(assertSchemaPage(handled,id(1),"ALL",25,0),handled);
});
test("schema and quality pagination preserve each other's selections and other queues",()=>{
  const url=new URL(schemaAttentionHref({reviewOffset:"25",qualityView:"CLOSED",qualityOffset:"100",schemaOffset:"75"},"ALL",0),"http://local.test");
  assert.equal(url.searchParams.get("schemaView"),"ALL");assert.equal(url.searchParams.get("schemaOffset"),null);
  assert.equal(url.searchParams.get("qualityView"),"CLOSED");assert.equal(url.searchParams.get("qualityOffset"),"100");assert.equal(url.searchParams.get("reviewOffset"),"25");
  assert.equal(url.hash,"#source-schema-attention");
  const other=new URL(qualityAttentionHref({schemaView:"ALL",schemaOffset:"100"},"FOLLOW_UP",0),"http://local.test");
  assert.equal(other.searchParams.get("schemaView"),"ALL");assert.equal(other.searchParams.get("schemaOffset"),"100");
  assert.equal(schemaView("CLOSED"),"UNHANDLED");assert.equal(schemaView("ALL"),"ALL");
});
