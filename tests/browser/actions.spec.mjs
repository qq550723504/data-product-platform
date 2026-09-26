import { test, expect } from "@playwright/test";
import { ids, fixtureToken } from "./fixture-server.mjs";
const control = "http://127.0.0.1:4400/__control";
const headers = { "x-fixture-token": fixtureToken };
const productPath = `/products/${ids.product}`;
async function scenario(request, value, reset = false) {
  const response = await request.post(`${control}/${reset ? "reset" : "scenario"}`, { headers, data: { scenario: value } });
  expect(response.ok()).toBeTruthy();
}
async function state(request) {
  const response = await request.get(`${control}/state`, { headers });
  expect(response.ok()).toBeTruthy();
  return response.json();
}
async function writes(request) { return (await state(request)).requests.filter((call) => call.method === "POST"); }

test.beforeEach(async ({ request }) => { await scenario(request, "ready", true); });

test("readonly runtime never enables review or publishing", async ({ page, request }) => {
  await page.goto("http://127.0.0.1:3101/reviews");
  await expect(page.getByText("当前为只读审核队列")).toBeVisible();
  await expect(page.getByRole("heading", { name: "测试来源记录", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "确认匹配" })).toHaveCount(0);
  await page.goto(`http://127.0.0.1:3101${productPath}`);
  await expect(page.getByRole("button", { name: "发布 Release", exact: true })).toBeDisabled();
  expect(await writes(request)).toHaveLength(0);
});

for (const [decision, label, status] of [["confirm", "确认匹配", "CONFIRMED"], ["reject", "拒绝匹配", "REJECTED"]]) {
  test(`review ${decision}: mandatory reason, server actor and refreshed queue`, async ({ page, request }) => {
    await page.goto("/reviews");
    const form = page.getByRole("form", { name: "审核 测试来源记录" });
    const button = form.getByRole("button", { name: label });
    await expect(button).toBeDisabled();
    await form.getByLabel("审核理由（必填）").fill("   ");
    await expect(button).toBeDisabled();
    expect(await writes(request)).toHaveLength(0);
    await form.getByLabel("审核理由（必填）").fill("  已核对测试来源  ");
    await button.click();
    await expect(page.getByText("当前页没有待审核候选")).toBeVisible();
    await expect(page.getByRole("status").filter({ hasText: "候选；任务状态：SUCCEEDED" }).first()).toBeVisible();
    const result = await state(request);
    expect(result.candidateStatus).toBe(status);
    const commands = await writes(request);
    expect(commands).toHaveLength(1);
    expect(commands[0].path).toBe(`/api/v1/entity-match-reviews/${ids.candidate}/${decision}`);
    expect(commands[0].authorization).toBe("Bearer review-secret");
    expect(commands[0].actor).toBeUndefined();
    expect(commands[0].body).toEqual({ reason: "已核对测试来源", expectedDecisionId: ids.decision });
  });
}

test("ambiguous entity review requires explicit frozen alternative selection", async ({ page, request }) => {
  await scenario(request, "ambiguous-review");
  await page.goto("/reviews");
  const form = page.getByRole("form", { name: "审核 测试来源记录" });
  const confirm = form.getByRole("button", { name: "确认匹配" });
  const select = form.getByLabel("选择匹配实体（必选）");
  await expect(select).toBeVisible();
  await expect(select.locator("option")).toHaveCount(3);
  await expect(select).toContainText("测试候选 A");
  await expect(select).toContainText("测试候选 B");
  await form.getByLabel("审核理由（必填）").fill("明确选择同名同址实体");
  await expect(confirm).toBeDisabled();
  await select.selectOption(ids.alternative);
  await expect(confirm).toBeEnabled();
  await confirm.click();
  await expect(page.getByRole("status").filter({ hasText: "已选择实体并确认候选；任务状态：SUCCEEDED" })).toBeVisible();
  const commands = await writes(request);
  expect(commands).toHaveLength(1);
  expect(commands[0].body).toEqual({
    reason: "明确选择同名同址实体",
    expectedDecisionId: ids.decision,
    selectedEntityId: ids.alternative,
  });
});

test("Gold DatasetVersion explains frozen production proof and current delivery", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`);
  const proof = page.getByTestId("gold-production-proof");
  await expect(proof).toBeVisible();
  await expect(proof).toContainText("Gold Production Proof");
  await expect(proof).toContainText(ids.goldBinding);
  await expect(proof).toContainText(ids.goldSnapshot);
  await expect(proof).toContainText("7".repeat(64));
  await expect(proof).toContainText("8".repeat(64));
  await expect(proof).toContainText(ids.goldAssessment);
  await expect(proof).toContainText(ids.goldCertification);

  const chain = page.getByTestId("gold-production-chain");
  await expect(chain).toBeVisible();
  await expect(chain).toContainText("Gold Production Chain");
  await expect(chain).toContainText("GOLD-PILOT / PROCESS");
  await expect(chain).toContainText("gold/schema / 1");
  await expect(chain).toContainText("gold/taxonomy / 1");
  await expect(chain).toContainText("row:1");
  await expect(chain).toContainText("annotator:label-studio-17");
  await expect(chain).toContainText("reviewer:gold-fixture");
  await expect(chain).toContainText("evidence is sufficient");
  await expect(chain).toContainText("label-studio/project/123");
  await expect(chain).toContainText("task 456");
  await expect(chain).toContainText("annotation 789");
  await expect(chain).toContainText("row:2");
  await expect(chain).toContainText("corrected provider label");
  await expect(chain).toContainText("task 457");
  await expect(chain).toContainText("annotation 790");

  const trace = page.getByTestId("gold-trace-summary");
  await expect(trace).toBeVisible();
  await expect(trace).toContainText("Gold Cost / Evidence / Audit Trace");
  await expect(trace).toContainText("HUMAN_REVIEW");
  await expect(trace).toContainText("ANNOTATION_REVIEW");
  await expect(trace).toContainText("GOLD_BUILD");
  await expect(trace).toContainText("GOLD_QUALITY");
  await expect(trace).toContainText("QUALITY_ENGINE_INVOCATION");
  await expect(trace).toContainText("GOLD_CERTIFICATION");
  await expect(trace).toContainText("CERTIFICATION_EVALUATION");
  await expect(trace).toContainText("DIRECT_DATA");
  await expect(trace).toContainText("DIRECT_DATA_DELIVERY_ATTEMPT");
  await expect(trace).toContainText("GOLD_PRODUCTION_BINDING_FINALIZED");
  await expect(trace).toContainText("GOLD_QUALITY_ASSESSMENT");
  await expect(trace).toContainText("DATASETDELIVERYISSUED");
  await expect(trace).toContainText("ANNOTATION_REVIEWED");
  await expect(trace).toContainText("trace-build");
  await expect(trace).toContainText("trace-certification");
  await expect(trace).toContainText("trace-delivery");

  await expect(page.getByRole("heading", { name: "Current Delivery Eligibility" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "预检结果" })).toBeVisible();
  const eligibility = page.getByRole("heading", { name: "预检结果" }).locator("..");
  await expect(eligibility).toContainText("ALLOWED");
  await expect(page.getByRole("cell", { name: "DatasetVersion usability" }).locator("..")).toContainText("ALLOWED");
  await expect(page.getByRole("cell", { name: "Current Certification" }).locator("..")).toContainText("ALLOWED");
  await expect(page.getByRole("cell", { name: "Current Entitlement" }).locator("..")).toContainText("ALLOWED");
  await expect(page.getByText("fixture verified rights", { exact: true }).locator("..")).toContainText("ALLOWED");
  expect(await writes(request)).toHaveLength(0);
});

test("populated eight-gate readiness publishes once with a server idempotency key", async ({ page, request }) => {
  await page.goto(productPath);
  const gates = ["production", "dataset", "rights", "quality", "compliance", "contract", "evidence", "delivery"];
  for (const gate of gates) await expect(page.getByTestId(`readiness-${gate}`)).toContainText("PASS");
  await expect(page.getByText("所有 Readiness Gate 已通过")).toBeVisible();
  await page.getByRole("button", { name: "发布 Release", exact: true }).click();
  await expect(page.getByRole("button", { name: "已发布", exact: true })).toBeDisabled();
  const commands = await writes(request);
  expect(commands).toHaveLength(1);
  expect(commands[0].path).toBe(`/api/v1/product-releases/${ids.release}/publish`);
  expect(commands[0].actor).toBe(ids.actor);
  expect(commands[0].idempotencyKey).toMatch(/^[0-9a-f-]{36}$/i);
  await page.reload();
  await expect(page.getByRole("button", { name: "已发布", exact: true })).toBeDisabled();
  expect(await writes(request)).toHaveLength(1);
});

for (const value of ["empty-checks", "missing-evidence", "null-checks", "blocker", "future-gate", "failed-rights"]) {
  test(`${value}: no false success banner or enabled publish button`, async ({ page, request }) => {
    await scenario(request, value);
    await page.goto(productPath);
    await expect(page.getByTestId("readiness-problem")).toBeVisible();
    await expect(page.getByText("所有 Readiness Gate 已通过")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "发布 Release", exact: true })).toBeDisabled();
    expect(await writes(request)).toHaveLength(0);
  });
}

test("readiness changes after render: Server Action rechecks and never posts publish", async ({ page, request }) => {
  await page.goto(productPath);
  const publish = page.getByRole("button", { name: "发布 Release", exact: true });
  await expect(publish).toBeEnabled();
  await scenario(request, "empty-checks");
  await publish.click();
  await expect(page.getByRole("button", { name: "重新加载并核对结果" })).toBeVisible();
  expect(await writes(request)).toHaveLength(0);
});

for (const [value, route, buttonName] of [["review-conflict", "/reviews", "确认匹配"], ["publish-conflict", productPath, "发布 Release"]]) {
  test(`${value}: one attempt and explicit refresh, without automatic retry`, async ({ page, request }) => {
    await scenario(request, value);
    await page.goto(route);
    if (route === "/reviews") await page.getByLabel("审核理由（必填）").fill("并发审核测试");
    await page.getByRole("button", { name: buttonName, exact: true }).click();
    await expect(page.getByRole("button", { name: "重新加载并核对结果" })).toBeVisible();
    await expect(page.getByRole("button", { name: buttonName, exact: true })).toBeDisabled();
    expect(await writes(request)).toHaveLength(1);
  });
}

test("list pages paginate on the server with explicit controls", async ({ page, request }) => {
  await scenario(request, "paginated-resources");
  await page.goto("/resources");
  await expect(page.getByText("第 1 / 2 页 · 本页 25 条 · 共 30 条")).toBeVisible();
  await expect(page.getByRole("link", { name: "资源 1", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "上一页" })).toHaveCount(0);
  await page.getByRole("link", { name: "下一页" }).click();
  await expect(page).toHaveURL(/\/resources\?offset=25$/);
  await expect(page.getByText("第 2 / 2 页 · 本页 5 条 · 共 30 条")).toBeVisible();
  await expect(page.getByRole("link", { name: "资源 26", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "资源 1", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "下一页" })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "上一页" })).toBeVisible();
  // A malformed offset must fall back to the first page, not fail the request.
  await page.goto("/resources?offset=garbage");
  await expect(page.getByText("第 1 / 2 页 · 本页 25 条 · 共 30 条")).toBeVisible();
});
