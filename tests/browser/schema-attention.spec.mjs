import { test, expect } from "@playwright/test";
import { ids, fixtureToken, schemaObservationId, schemaRequestId } from "./fixture-server.mjs";
const control="http://127.0.0.1:4400/__control", headers={"x-fixture-token":fixtureToken};
async function reset(request,scenario="schema-attention"){expect((await request.post(`${control}/reset`,{headers,data:{scenario}})).ok()).toBeTruthy();}
async function calls(request){return (await (await request.get(`${control}/state`,{headers})).json()).requests;}
function exact(n=105){return `/resources/${ids.sourceResource}?view=source-schema&requestId=${schemaRequestId(n)}&observationId=${schemaObservationId(n)}`;}
function assertPureHistory(calls){
  expect(calls.some(c=>c.method!=="GET")).toBe(false);
  expect(calls.some(c=>/metadata-observation|metadata-bindings|ingestion-source|\/ingestions|\/schema-observations$/.test(c.path))).toBe(false);
}
test.beforeEach(async({request})=>reset(request));

test("schema attention paginates past 100 without detail fanout then opens exact historical evidence",async({page,request},testInfo)=>{
  await page.goto("/attention?schemaView=ALL&schemaOffset=100&qualityView=CLOSED");
  const panel=page.getByTestId("schema-attention");
  await expect(panel.locator("tbody tr")).toHaveCount(5);
  await expect(panel.getByRole("navigation",{name:"schema 漂移分页"})).toContainText("101–105 / 105");
  const row=panel.locator("tbody tr").filter({hasText:"SOURCE_SCHEMA_CHANGED 105"});
  await expect(row).toContainText("已记录处理（3 条记录）");
  await expect(panel).toContainText("未记录处理 104 条 · 已记录处理 1 条");
  const before=await calls(request);
  expect(before.filter(c=>c.path.endsWith("/source-schema-observations/attention"))).toHaveLength(1);
  expect(before.filter(c=>/schema-observations\/[0-9a-f-]+$/.test(c.path))).toHaveLength(0);
  assertPureHistory(before);
  await page.screenshot({path:testInfo.outputPath("schema-attention.png"),fullPage:true});
  await row.getByRole("link",{name:"查看精确观察"}).click();
  await expect(page).toHaveURL(new RegExp(`observationId=${schemaObservationId(105)}$`));
  const evidence=page.getByTestId("schema-observation-evidence");
  await expect(evidence).toContainText(schemaRequestId(105));
  await expect(evidence).toContainText("historical.energy");
  await expect(evidence).toContainText("historical-provider-id");
  await expect(evidence).toContainText("历史处理 3");
  await expect(evidence.locator("tr").filter({hasText:"energy_kwh"})).toContainText("TEXT / NOT_NULL");
  await expect(evidence.locator("tr").filter({hasText:"energy_kwh"})).toContainText("BIGINT / NULL");
  await expect(page.locator("form")).toHaveCount(0);
  expect(await page.content()).not.toContain("rights-fixture-secret");
  await page.reload();
  await expect(evidence).toBeVisible();
  const after=await calls(request);
  expect(after.filter(c=>c.path.endsWith(`/schema-observations/${schemaObservationId(105)}`))).toHaveLength(2);
  assertPureHistory(after);
  await page.screenshot({path:testInfo.outputPath("schema-exact-evidence.png"),fullPage:true});
});
test("workbench enters unhandled observations and filters preserve quality queue selection",async({page,request})=>{
  await page.goto("/core/workbench");
  const panel=page.getByTestId("schema-attention");
  await expect(panel).toContainText("未记录处理 104 条");
  await panel.getByRole("link",{name:"查看漂移观察"}).click();
  await expect(page).toHaveURL(/schemaView=UNHANDLED#source-schema-attention$/);
  await expect(page.getByTestId("schema-attention").locator("tbody tr")).toHaveCount(25);
  await page.goto("/attention?qualityView=CLOSED&qualityOffset=100&schemaView=ALL&schemaOffset=100");
  await page.getByRole("navigation",{name:"schema 漂移视图"}).getByRole("link",{name:"未记录处理",exact:true}).click();
  await expect(page).toHaveURL(/qualityOffset=100&qualityView=CLOSED&schemaView=UNHANDLED#source-schema-attention$/);
  assertPureHistory(await calls(request));
});
test("schema unavailability and foreign summary stay UNKNOWN while original queues remain visible",async({page,request})=>{
  for(const scenario of ["schema-attention-unavailable","schema-attention-foreign"]){
    await reset(request,scenario);
    await page.goto("/attention");
    await expect(page.getByTestId("schema-attention-unavailable")).toContainText("UNKNOWN");
    await expect(page.getByTestId("quality-attention")).toBeVisible();
    await expect(page.getByRole("heading",{name:"待办中心",exact:true})).toBeVisible();
    assertPureHistory(await calls(request));
  }
});
test("exact mode rejects request mismatch and incomplete IDs without falling back to live resource",async({page,request})=>{
  await reset(request,"schema-attention-mismatch");
  await page.goto(exact());
  await expect(page.getByTestId("schema-observation-error")).toContainText("不属于");
  await expect(page.getByTestId("schema-observation-evidence")).toHaveCount(0);
  await page.goto(`/resources/${ids.sourceResource}?view=source-schema`);
  await expect(page.getByTestId("schema-observation-error")).toContainText("引用必填");
  await expect(page.locator("form")).toHaveCount(0);
  expect((await calls(request)).filter(c=>c.path.endsWith(`/schema-observations/${schemaObservationId(105)}`))).toHaveLength(1);
  assertPureHistory(await calls(request));
});
