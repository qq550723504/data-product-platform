import {test,expect} from "@playwright/test";
import {ids,fixtureToken,unobservedRequestId,requestAttemptId,requestObservationId} from "./fixture-server.mjs";
const control="http://127.0.0.1:4400/__control",headers={"x-fixture-token":fixtureToken};
async function reset(request,scenario="schema-requests"){expect((await request.post(`${control}/reset`,{headers,data:{scenario}})).ok()).toBeTruthy();}
async function calls(request){return(await(await request.get(`${control}/state`,{headers})).json()).requests;}
function pure(list){expect(list.some(c=>c.method!=="GET")).toBe(false);expect(list.some(c=>/metadata-observation|metadata-bindings|ingestion-source|\/ingestions|\/schema-observations$/.test(c.path))).toBe(false);}

async function requestPageReady(page,offset=0,view="NO_ATTEMPT"){
  const suffix=offset?`&schemaRequestOffset=${offset}`:"";
  await expect(page).toHaveURL(new RegExp(`/attention\\?schemaRequestView=${view}${suffix}#schema-unobserved-requests$`));
  const panel=page.getByTestId("schema-requests");
  await expect(page.getByRole("navigation",{name:"无观察请求筛选"}).getByRole("link",{name:view==="ALL"?"全部无观察请求":"尚未登记 attempt",exact:true})).toHaveAttribute("aria-current","page");
  await expect(panel.locator("tbody tr")).toHaveCount(25);
  // Both the old ALL page and the new filtered page have 25 rows. Require the
  // committed pagination/context href before continuing to another section.
  await expect(panel.getByRole("link",{name:"下一页",exact:true})).toHaveAttribute("href",new RegExp(`schemaRequestView=${view}&schemaRequestOffset=${offset+25}#`));
  await expect(page.getByRole("navigation",{name:"扫描异常历史筛选"}).getByRole("link",{name:"当次拒绝",exact:true})).toHaveAttribute("href",new RegExp(`schemaRequestView=${view}${suffix}&schemaExceptionStatus=DENIED#`));
  return panel;
}
async function selectNoAttempt(page){
  await page.getByRole("navigation",{name:"无观察请求筛选"}).getByRole("link",{name:"尚未登记 attempt",exact:true}).click();
  return requestPageReady(page);
}
test.beforeEach(async({request})=>reset(request));
test("unobserved pagination and exact attempt remain readable after observation arrives",async({page,request},info)=>{
  await page.goto("/attention?schemaRequestView=ALL&schemaRequestOffset=100");
  await expect(page.getByText("审核、执行与发布：2 条",{exact:true})).toBeVisible();
  const panel=page.getByTestId("schema-requests");await expect(panel.locator("tbody tr")).toHaveCount(5);
  await expect(panel).toContainText("尚未登记 attempt 52 个 · 已登记 attempt 53 个");
  const initial=await calls(request);expect(initial.filter(c=>c.path.endsWith("/source-schema-requests/unobserved"))).toHaveLength(1);expect(initial.filter(c=>/\/schema-requests\//.test(c.path))).toHaveLength(0);
  await page.screenshot({path:info.outputPath("schema-requests.png"),fullPage:true});
  await panel.locator("tbody tr").filter({hasText:unobservedRequestId(105)}).getByRole("link",{name:"查看精确请求"}).click();
  const evidence=page.getByTestId("schema-request-evidence");await expect(evidence).toContainText("historical-request-provider-id");await expect(evidence).toContainText("未指定 attempt");
  await evidence.getByRole("link",{name:`查看 attempt ${requestAttemptId(105)}`,exact:true}).click();
  await expect(page.getByTestId("schema-selected-attempt")).toContainText(requestAttemptId(105));
  await expect(page.getByTestId("schema-selected-attempt")).toContainText("实际运行情况 UNKNOWN");
  await request.post(`${control}/scenario`,{headers,data:{scenario:"schema-requests-arrived"}});
  await page.reload();await expect(evidence).toContainText("该请求已有观察，已不属于无观察请求集合");
  await expect(page.getByTestId("schema-selected-attempt")).toContainText(requestAttemptId(105));
  await page.screenshot({path:info.outputPath("schema-request-evidence.png"),fullPage:true});
  await page.getByRole("link",{name:"查看指定 attempt 的精确观察"}).click();
  await expect(page.getByTestId("schema-observation-evidence")).toContainText(requestObservationId(105));
  await page.getByRole("link",{name:"返回精确请求证据",exact:true}).click();await expect(page.getByTestId("schema-selected-attempt")).toContainText(requestAttemptId(105));await page.getByRole("link",{name:"返回无观察请求",exact:true}).click();
  await expect(page).toHaveURL(/schemaRequestView=ALL&schemaRequestOffset=100#schema-unobserved-requests$/);await expect(panel.locator("tbody tr")).toHaveCount(4);
  await expect(page.locator("form")).toHaveCount(0);expect(await page.content()).not.toContain("rights-fixture-secret");pure(await calls(request));
});
test("independent counts and filters survive other queue navigation",async({page,request})=>{
  await page.goto("/core/workbench");await expect(page.getByTestId("schema-requests")).toContainText("105 个无观察请求");await expect(page.getByTestId("needs-attention")).not.toContainText("105");
  await page.getByRole("link",{name:"查看无观察请求",exact:true}).click();
  await requestPageReady(page,0,"ALL");
  const panel=await selectNoAttempt(page);await panel.getByRole("link",{name:"下一页",exact:true}).click();
  await requestPageReady(page,25);
  // Both pages contain 25 rows. Count alone can pass on the old page and
  // clicking its sibling filter would cancel the still-pending navigation.
  await expect(page).toHaveURL(/schemaRequestView=NO_ATTEMPT&schemaRequestOffset=25/);
  await expect(panel.getByRole("navigation",{name:"无观察请求分页",exact:true})).toContainText("26–50 / 52");
  const denied=page.getByRole("navigation",{name:"扫描异常历史筛选"}).getByRole("link",{name:"当次拒绝",exact:true});
  await expect(denied).toHaveAttribute("href",/schemaRequestView=NO_ATTEMPT&schemaRequestOffset=25/);
  await denied.click();
  await expect(page).toHaveURL(/schemaRequestView=NO_ATTEMPT&schemaRequestOffset=25/);await expect(panel.locator("tbody tr")).toHaveCount(25);pure(await calls(request));
});
test("delayed filter response cannot make an old 25-row page navigation-ready",async({page,request})=>{
  await page.goto("/attention?schemaRequestView=ALL#schema-unobserved-requests");
  const panel=page.getByTestId("schema-requests");await expect(panel.locator("tbody tr")).toHaveCount(25);
  let release;const barrier=new Promise(resolve=>{release=resolve;});let held=false,ready=false,selection;
  const target=url=>url.pathname==="/attention"&&url.searchParams.get("schemaRequestView")==="NO_ATTEMPT"&&!url.searchParams.has("schemaRequestOffset");
  const handler=async route=>{
    if(route.request().headers()["rsc"]!=="1")return route.continue();
    const response=await route.fetch();held=true;await barrier;await route.fulfill({response});
  };
  await page.route(target,handler);
  try{
    selection=selectNoAttempt(page).then(value=>{ready=true;return value;});selection.catch(()=>{});
    await expect.poll(()=>held).toBe(true);
    await expect(page).toHaveURL(/schemaRequestView=ALL#schema-unobserved-requests$/);
    await expect(page.getByRole("navigation",{name:"无观察请求筛选"}).getByRole("link",{name:"全部无观察请求",exact:true})).toHaveAttribute("aria-current","page");
    await expect(panel.locator("tbody tr")).toHaveCount(25);
    await expect(panel.getByRole("link",{name:"下一页",exact:true})).toHaveAttribute("href",/schemaRequestView=ALL&schemaRequestOffset=25#/);
    expect(ready).toBe(false);
    release();await selection;
    await panel.getByRole("link",{name:"下一页",exact:true}).click();await requestPageReady(page,25);
    await page.getByRole("navigation",{name:"扫描异常历史筛选"}).getByRole("link",{name:"当次拒绝",exact:true}).click();
    await expect(page).toHaveURL(/schemaRequestView=NO_ATTEMPT&schemaRequestOffset=25&schemaExceptionStatus=DENIED/);
    await expect(panel.locator("tbody tr")).toHaveCount(25);pure(await calls(request));
  }finally{
    release();if(selection)await selection.catch(()=>{});await page.unroute(target,handler);
  }
});
test("request detail pages all attempts and observations without replacing selected attempt",async({page,request})=>{
  await page.goto(`/resources/${ids.sourceResource}?view=source-schema-request&requestId=${unobservedRequestId(106)}&attemptId=${requestAttemptId(106000)}&attemptOffset=25&observationOffset=25`);
  const evidence=page.getByTestId("schema-request-evidence");await expect(evidence).toContainText("31 个 attempt；31 条 observation");await expect(page.getByTestId("schema-selected-attempt")).toContainText(requestAttemptId(106000));
  await expect(evidence.getByRole("link",{name:/^查看 attempt /})).toHaveCount(6);await expect(evidence.getByRole("link",{name:/^查看观察 /})).toHaveCount(6);await expect(page.locator("form")).toHaveCount(0);pure(await calls(request));
});
test("unavailable, foreign and mismatched request data fail locally without fallback",async({page,request})=>{
  for(const scenario of ["schema-requests-unavailable","schema-requests-foreign"]){await reset(request,scenario);await page.goto("/attention");await expect(page.getByTestId("schema-requests-unavailable")).toContainText("UNKNOWN");await expect(page.getByTestId("schema-exceptions")).toBeVisible();pure(await calls(request));}
  await reset(request,"schema-requests-mismatch");await page.goto(`/resources/${ids.sourceResource}?view=source-schema-request&requestId=${unobservedRequestId(105)}`);await expect(page.getByTestId("schema-request-error")).toContainText("不属于");await expect(page.getByTestId("schema-request-evidence")).toHaveCount(0);pure(await calls(request));
  await reset(request,"schema-requests-empty");await page.goto("/attention");await expect(page.getByTestId("schema-requests")).toContainText("当前筛选没有无观察请求");await expect(page.getByTestId("schema-requests")).toContainText("不表示来源健康或已经恢复");
});
