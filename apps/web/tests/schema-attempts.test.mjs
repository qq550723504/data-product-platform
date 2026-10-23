import test from "node:test";
import assert from "node:assert/strict";
import { createRequire } from "node:module";
const require = createRequire(import.meta.url);
const { assertSchemaAttemptPage, schemaAttemptsHref } = require("../.schema-tests/schema-attempts.js");
const { schemaRequestOrigin, schemaRequestListHref, schemaRequestHref, schemaRequestObservationHref, schemaRequestsHref } = require("../.schema-tests/schema-requests.js");
const { qualityAttentionHref } = require("../.schema-tests/quality-attention.js");
const { schemaAttentionHref } = require("../.schema-tests/schema-observation.js");
const { schemaExceptionsHref } = require("../.schema-tests/schema-exceptions.js");
const ws="11111111-1111-4111-8111-111111111111", resource="22222222-2222-4222-8222-222222222222", request="33333333-3333-4333-8333-333333333333", attempt="44444444-4444-4444-8444-444444444444", other="55555555-5555-4555-8555-555555555555", stamp="2026-10-22T00:00:00Z";
const item={workspaceId:ws,resourceId:resource,requestId:request,responsibleActorId:ws,attemptCount:2,requestedAt:stamp,attemptId:attempt,registeredAt:stamp,requestObservationCount:31};
const page={items:[item],page:{limit:25,offset:0,total:1},readAt:stamp};

test("attempt collection validates scope, history, attempt identity and complete page counts",()=>{
  assert.equal(assertSchemaAttemptPage(page,ws,25,0),page);
  for(const change of [{workspaceId:resource},{attemptId:""},{registeredAt:"invalid"},{attemptCount:0},{requestObservationCount:0}]) {
    assert.throws(()=>assertSchemaAttemptPage({...page,items:[{...item,...change}]},ws,25,0));
  }
  assert.throws(()=>assertSchemaAttemptPage({...page,page:{...page.page,total:2}},ws,25,0));
  assert.throws(()=>assertSchemaAttemptPage({...page,items:[item,item],page:{...page.page,total:2}},ws,25,0));
  const siblings={...page,items:[item,{...item,attemptId:other}],page:{...page.page,total:2}};
  assert.equal(assertSchemaAttemptPage(siblings,ws,25,0),siblings); // same request, two distinct attempts
});
test("bounded attempt offsets survive all existing section navigation",()=>{
  const q={schemaAttemptOffset:"100",schemaRequestView:"ATTEMPT_RECORDED",schemaRequestOffset:"25",schemaExceptionStatus:"DENIED",schemaExceptionOffset:"50",qualityView:"CLOSED",qualityOffset:"25"};
  for(const href of [qualityAttentionHref(q,"ALL",0),schemaAttentionHref(q,"ALL",0),schemaExceptionsHref(q,"ALL",0),schemaRequestsHref(q,"ALL",0)]) assert.match(href,/schemaAttemptOffset=100/);
  const href=schemaAttemptsHref(q,75);
  for(const pair of ["schemaAttemptOffset=75","schemaRequestView=ATTEMPT_RECORDED","schemaRequestOffset=25","schemaExceptionStatus=DENIED","qualityView=CLOSED"]) assert.ok(href.includes(pair));
  assert.ok(!schemaAttemptsHref({schemaAttemptOffset:"https://evil",returnTo:"https://evil"},0).includes("evil"));
});
test("typed evidence origin preserves selected attempt separately from observation identity",()=>{
  const origin=schemaRequestOrigin({schemaEvidenceOrigin:"OBSERVED_ATTEMPTS",schemaAttemptOffset:"100"});
  assert.equal(schemaRequestListHref(origin),"/attention?schemaAttemptOffset=100#schema-unobserved-attempts");
  assert.match(schemaRequestHref(resource,request,origin,attempt,50,25),/schemaAttemptOffset=100&schemaEvidenceOrigin=OBSERVED_ATTEMPTS&attemptId=.*&attemptOffset=50&observationOffset=25$/);
  const observation=schemaRequestObservationHref(resource,request,other,origin,attempt,50,25);
  assert.match(observation,/observationId=55555555/);
  assert.match(observation,/requestAttemptId=44444444/);
  assert.ok(!new URL(observation,"http://local").searchParams.has("attemptId"));
  assert.throws(()=>schemaRequestOrigin({schemaEvidenceOrigin:"https://evil"}));
  const context=schemaRequestOrigin({schemaEvidenceOrigin:"OBSERVED_ATTEMPTS",schemaAttemptOffset:"100",qualityView:"CLOSED",returnTo:"https://evil"});
  assert.match(schemaRequestListHref(context),/qualityView=CLOSED/);
  assert.ok(!schemaRequestHref(resource,request,context,attempt).includes("evil"));
  assert.throws(()=>schemaRequestObservationHref(resource,request,other,origin,"",0,0));
});
