import { test, expect } from "@playwright/test";
import { ids, fixtureToken, observedRequestId, observedAttemptId, observedObservationId, requestObservationId } from "./fixture-server.mjs";
const control="http://127.0.0.1:4400/__control", headers={"x-fixture-token":fixtureToken};
async function reset(request,scenario="schema-attempts",readFaults={}) { expect((await request.post(control+"/reset",{headers,data:{scenario,readFaults}})).ok()).toBeTruthy(); }
async function pure(page,request) {
  const state=await(await request.get(control+"/state",{headers})).json();
  expect(state.requests.some(r=>r.method!=="GET")).toBe(false);
  expect(state.requests.some(r=>/metadata-observation|metadata-bindings|ingestion-source|\/ingestions|\/schema-observations$/.test(r.path))).toBe(false);
  for(const text of ["rights-fixture-secret","private-upstream-sentinel","private-attempt-runtime"])expect(await page.content()).not.toContain(text);
  await expect(page.locator("form")).toHaveCount(0);
}
async function others(page,path) {
  for(const name of ["quality-attention","schema-attention","schema-exceptions","schema-requests"])await expect(page.getByTestId(name)).toBeVisible();
  await expect(page.getByTestId(path==="/core/workbench"?"workbench-attention-total":"attention-total")).toContainText("2 条");
  await expect(page.getByRole("link",{name:"去发布",exact:true})).toHaveAttribute("href","/products/"+ids.product+"#releases");
}
test.beforeEach(async({request})=>reset(request));

test("observed-request attempts page independently and retain exact selected identity after observation arrival",async({page,request},info)=>{
  await page.goto("/attention?schemaAttemptOffset=100&qualityView=CLOSED&schemaRequestView=ATTEMPT_RECORDED");
  const panel=page.getByTestId("schema-attempts");
  await expect(panel).toContainText("105 个 attempt");await expect(panel.locator("tbody tr")).toHaveCount(5);
  await expect(panel).toBeVisible();
  await expect(page.getByTestId("attention-total")).toContainText("2 条");
  const initial=(await(await request.get(control+"/state",{headers})).json()).requests;
  expect(initial.filter(r=>r.path.endsWith("/observed/unobserved-attempts"))).toHaveLength(1);
  expect(initial.filter(r=>/\/schema-requests\//.test(r.path))).toHaveLength(0);
  await panel.screenshot({path:info.outputPath("attempt-list.png")});
  await panel.locator("tbody tr").filter({hasText:observedAttemptId(1)}).getByRole("link",{name:"查看精确 attempt"}).click();
  await expect(page.getByTestId("schema-selected-attempt")).toContainText(observedAttemptId(1));
  await page.getByRole("navigation",{name:"请求 attempt 分页"}).getByRole("link",{name:"下一页"}).click();
  await expect(page).toHaveURL(/attemptOffset=25/);
  await expect(page.getByTestId("schema-selected-attempt")).toContainText(observedAttemptId(1));
  // Viewing a sibling's older observation must return to the original selected attempt.
  await page.getByRole("link",{name:"查看观察 "+requestObservationId(1070000),exact:true}).click();
  await expect(page.getByTestId("schema-observation-evidence")).toContainText(requestObservationId(1070000));
  await page.getByRole("link",{name:"返回精确请求证据",exact:true}).click();
  await expect(page.getByTestId("schema-selected-attempt")).toContainText(observedAttemptId(1));
  await expect(page).toHaveURL(/attemptOffset=25/);
  expect((await request.post(control+"/scenario",{headers,data:{scenario:"schema-attempts-arrived"}})).ok()).toBeTruthy();
  await page.reload();await page.getByRole("link",{name:"查看指定 attempt 的精确观察",exact:true}).click();
  await expect(page.getByTestId("schema-observation-evidence")).toContainText(observedObservationId(1));
  await page.getByRole("link",{name:"返回精确请求证据",exact:true}).click();
  await expect(page.getByTestId("schema-selected-attempt")).toContainText(observedAttemptId(1));
  await page.screenshot({path:info.outputPath("attempt-evidence-after-arrival.png"),fullPage:true});
  await page.getByRole("link",{name:"返回无观察 attempt",exact:true}).click();
  await expect(page).toHaveURL(/schemaAttemptOffset=100#schema-unobserved-attempts$/);
  await expect(page).toHaveURL(/qualityView=CLOSED/);
  await expect(panel).toContainText("104 个 attempt");await expect(panel.locator("tbody tr")).toHaveCount(4);
  await expect(panel).not.toContainText(observedAttemptId(1));await pure(page,request);
});
test("compact and existing section filters keep attempt counts separate and preserve its page",async({page,request})=>{
  await page.goto("/core/workbench");await expect(page.getByTestId("schema-attempts")).toContainText("105 个 attempt");await others(page,"/core/workbench");
  await page.getByRole("link",{name:"查看无观察 attempt",exact:true}).click();
  const panel=page.getByTestId("schema-attempts");
  for(let i=0;i<4;i++)await panel.getByRole("link",{name:"下一页",exact:true}).click();
  await expect(panel.locator("tbody tr")).toHaveCount(5);
  await page.getByRole("navigation",{name:"扫描异常历史筛选"}).getByRole("link",{name:"当次拒绝",exact:true}).click();
  await expect(page).toHaveURL(/schemaAttemptOffset=100/);await expect(panel.locator("tbody tr")).toHaveCount(5);
  await pure(page,request);
});
test("attempt failures and invalid scope stay local on both pages",async({page,request})=>{
  for(const mode of ["fail","denied","invalid"])for(const path of ["/core/workbench","/attention"]){
    await reset(request,"schema-attempts",{attempts:mode});await page.goto(path);
    await expect(page.getByTestId("schema-attempts-unavailable")).toContainText("UNKNOWN");await others(page,path);await pure(page,request);
  }
  await reset(request,"schema-attempts-foreign");await page.goto("/attention");
  await expect(page.getByTestId("schema-attempts-unavailable")).toContainText("UNKNOWN");await pure(page,request);
});
for(const path of ["/core/workbench","/attention"])test(path+" streams independent facts while attempt GET stays held",async({page,request})=>{
  await reset(request,"schema-attempts",{attempts:"hold"});
  try {
    await page.goto(path,{waitUntil:"commit",timeout:5000});
    await expect(page.getByTestId("schema-attempts-pending")).toContainText("UNKNOWN");
    await others(page,path);
    const state=await(await request.get(control+"/state",{headers})).json();expect(state.heldReadCount).toBe(1);
    await pure(page,request);
  } finally {expect((await request.post(control+"/release-reads",{headers})).ok()).toBeTruthy();}
  await expect(page.getByTestId("schema-attempts-pending")).toHaveCount(0);
  await expect(page.getByTestId("schema-attempts")).toContainText("105 个 attempt");
  await expect(page.locator("#schema-unobserved-attempts")).toHaveCount(1);await pure(page,request);
});
test("successful empty attempt collection differs from unavailable and recovers on refresh",async({page,request})=>{
  await reset(request,"schema-attempts-empty");await page.goto("/attention");
  await expect(page.getByTestId("schema-attempts")).toContainText("0 个 attempt");
  await expect(page.getByTestId("schema-attempts")).toContainText("不表示来源健康");
  await reset(request,"schema-attempts-empty",{attempts:"fail"});await page.reload();
  await expect(page.getByTestId("schema-attempts-unavailable")).toContainText("UNKNOWN");
  await reset(request,"schema-attempts-empty");await page.reload();await expect(page.getByTestId("schema-attempts")).toContainText("0 个 attempt");await pure(page,request);
});
test("wrong selected identities and unlisted evidence origins fail without provider fallback",async({page,request})=>{
  const base=`/resources/${ids.sourceResource}?view=source-schema-request&requestId=${observedRequestId}&attemptId=${observedAttemptId(1)}`;
  await page.goto(base+"&schemaEvidenceOrigin=https://evil");await expect(page.getByTestId("schema-request-error")).toBeVisible();
  await reset(request,"schema-attempts-mismatch");await page.goto(base);await expect(page.getByTestId("schema-request-error")).toBeVisible();
  await reset(request);await page.goto(`/resources/${ids.sourceResource}?view=source-schema&requestId=${observedRequestId}&observationId=${requestObservationId(1070000)}&requestEvidence=1&schemaEvidenceOrigin=OBSERVED_ATTEMPTS&requestAttemptId=${ids.job}`);
  await expect(page.getByTestId("schema-observation-error")).toBeVisible();await expect(page.getByTestId("schema-observation-evidence")).toHaveCount(0);await pure(page,request);
});
