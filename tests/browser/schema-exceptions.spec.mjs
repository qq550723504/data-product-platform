import { test, expect } from "@playwright/test";
import { ids, fixtureToken, exceptionObservationId, exceptionRequestId } from "./fixture-server.mjs";
const control="http://127.0.0.1:4400/__control", headers={"x-fixture-token":fixtureToken};
async function reset(request,scenario="schema-exceptions"){expect((await request.post(`${control}/reset`,{headers,data:{scenario}})).ok()).toBeTruthy();}
async function calls(request){return (await (await request.get(`${control}/state`,{headers})).json()).requests;}
function assertPureHistory(list){
  expect(list.some(c=>c.method!=="GET")).toBe(false);
  expect(list.some(c=>/metadata-observation|metadata-bindings|ingestion-source|\/ingestions|\/schema-observations$/.test(c.path))).toBe(false);
}
test.beforeEach(async({request})=>reset(request));

test("exception history paginates past 100 and binds exact nullable historical evidence",async({page,request},testInfo)=>{
  await page.goto("/attention?schemaExceptionStatus=ALL&schemaExceptionOffset=100&qualityView=CLOSED&schemaView=ALL");
  const panel=page.getByTestId("schema-exceptions");
  await expect(panel.locator("tbody tr")).toHaveCount(5);
  await expect(panel.getByRole("navigation",{name:"扫描异常历史分页"})).toContainText("101–105 / 105");
  await expect(panel).toContainText("暂不可用 35 条 · 拒绝 35 条 · 已中断 35 条");
  await expect(panel).toContainText("不等于当前未恢复数量");
  const row=panel.locator("tbody tr").filter({hasText:"HISTORICAL_DENIED 104"});
  await expect(row).toContainText("未绑定 尝试记录");
  const before=await calls(request);
  expect(before.filter(c=>c.path.endsWith("/source-schema-observations/exceptions"))).toHaveLength(1);
  expect(before.filter(c=>/schema-observations\/[0-9a-f-]+$/.test(c.path))).toHaveLength(0);
  await page.screenshot({path:testInfo.outputPath("schema-exceptions.png"),fullPage:true});
  await row.getByRole("link",{name:"查看精确观察"}).click();
  await expect(page).toHaveURL(new RegExp(`observationId=${exceptionObservationId(104)}&schemaExceptionStatus=ALL&schemaExceptionOffset=100$`));
  const evidence=page.getByTestId("schema-observation-evidence");
  await expect(evidence).toContainText(exceptionRequestId(104));
  await expect(evidence).toContainText("historical-provider-id");
  await expect(evidence).toContainText("未绑定 尝试记录");
  await expect(evidence.locator("tr").filter({hasText:"energy_kwh"})).toContainText("无法确认");
  await expect(evidence).not.toContainText("本次无此列");
  await expect(evidence.getByRole("heading",{name:"处理记录",exact:true})).toHaveCount(0);
  await expect(page.locator("form")).toHaveCount(0);
  expect(await page.content()).not.toContain("rights-fixture-secret");
  await page.reload();
  await expect(evidence).toBeVisible();
  await page.screenshot({path:testInfo.outputPath("schema-exception-evidence.png"),fullPage:true});
  await page.getByRole("link",{name:"返回扫描异常历史"}).click();
  await expect(page).toHaveURL(/schemaExceptionStatus=ALL&schemaExceptionOffset=100#schema-exception-history$/);
  await expect(page.getByTestId("schema-exceptions").locator("tbody tr")).toHaveCount(5);
  assertPureHistory(await calls(request));
});

test("workbench keeps history separate from current queues and filters survive other queue navigation",async({page,request})=>{
  await page.goto("/core/workbench");
  const compact=page.getByTestId("schema-exceptions");
  await expect(compact).toContainText("已记录 105 条扫描异常历史");
  await expect(page.getByTestId("needs-attention")).not.toContainText("105");
  await compact.getByRole("link",{name:"查看扫描异常历史"}).click();
  await page.getByRole("navigation",{name:"扫描异常历史筛选"}).getByRole("link",{name:"当次拒绝",exact:true}).click();
  const panel=page.getByTestId("schema-exceptions");
  await expect(panel.locator("tbody tr")).toHaveCount(25);
  await expect(panel.locator("tbody tr")).toContainText(Array(25).fill("DENIED"));
  const next=panel.getByRole("link",{name:"下一页",exact:true});
  await expect(next).toHaveAttribute("href",/schemaExceptionStatus=DENIED&schemaExceptionOffset=25/);
  await Promise.all([
    page.waitForResponse(response=>response.request().isNavigationRequest()&&/schemaExceptionStatus=DENIED&schemaExceptionOffset=25/.test(response.url())),
    next.click(),
  ]);
  await expect(page).toHaveURL(/schemaExceptionStatus=DENIED&schemaExceptionOffset=25#schema-exception-history$/);
  await expect(panel.locator("tbody tr")).toHaveCount(10);
  const row=panel.locator("tbody tr").filter({hasText:"HISTORICAL_DENIED 104"});
  await row.getByRole("link",{name:"查看精确观察"}).click();
  await page.getByRole("link",{name:"返回扫描异常历史"}).click();
  await expect(page).toHaveURL(/schemaExceptionStatus=DENIED&schemaExceptionOffset=25#schema-exception-history$/);
  await page.getByRole("navigation",{name:"数据结构 漂移视图"}).getByRole("link",{name:"全部漂移观察",exact:true}).click();
  await expect(page).toHaveURL(/schemaExceptionStatus=DENIED&schemaExceptionOffset=25/);
  await expect(panel.locator("tbody tr")).toHaveCount(10);
  assertPureHistory(await calls(request));
});

test("history failures, foreign data and successful empty reads never assert current health",async({page,request})=>{
  for(const scenario of ["schema-exceptions-unavailable","schema-exceptions-foreign"]){
    await reset(request,scenario);
    await page.goto("/attention");
    await expect(page.getByTestId("schema-exceptions-unavailable")).toContainText("无法确认");
    await expect(page.getByTestId("schema-attention")).toBeVisible();
    await expect(page.getByTestId("quality-attention")).toBeVisible();
    assertPureHistory(await calls(request));
  }
  await reset(request,"schema-exceptions-empty");
  await page.goto("/attention");
  await expect(page.getByTestId("schema-exceptions")).toContainText("当前筛选没有已登记扫描异常");
  await expect(page.getByTestId("schema-exceptions")).toContainText("不提供当前健康或恢复结论");
});

test("exception exact mode rejects a wrong request without live fallback",async({page,request})=>{
  await reset(request,"schema-exceptions-mismatch");
  await page.goto(`/resources/${ids.sourceResource}?view=source-schema&requestId=${exceptionRequestId(104)}&observationId=${exceptionObservationId(104)}`);
  await expect(page.getByTestId("schema-observation-error")).toContainText("不属于");
  await expect(page.getByTestId("schema-observation-evidence")).toHaveCount(0);
  await expect(page.locator("form")).toHaveCount(0);
  assertPureHistory(await calls(request));
});
