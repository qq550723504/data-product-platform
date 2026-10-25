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

test("Registered batch resumes the same operation after a pre-start crash",async({page,request})=>{
 await scenario(request,"batch-registered",true);
 await page.goto(`http://127.0.0.1:3102/batches/${ids.ingestion}`);
 await page.getByRole("button",{name:"启动抽取",exact:true}).click();
 await expect(page.getByText("运行中",{exact:true})).toBeVisible();
 await page.reload();
 await expect(page.getByRole("button",{name:"启动抽取",exact:true})).toBeDisabled();
 const commands=await writes(request);
 expect(commands.filter(c=>c.path===`/api/v1/ingestions/${ids.ingestion}/dispatch`)).toHaveLength(1);
 expect(commands.some(c=>c.path==="/api/v1/ingestions")).toBe(false);
});

test("Registering batch can query its uncertain outcome without a second start",async({page,request})=>{
 await scenario(request,"batch-registering",true);
 await page.goto(`http://127.0.0.1:3102/batches/${ids.ingestion}`);
 await expect(page.getByRole("button",{name:"启动抽取",exact:true})).toBeDisabled();
 await page.getByRole("button",{name:"核对原运行",exact:true}).click();
 await expect(page.getByRole("status")).toContainText("结果待核实");
 await expect(page.getByRole("button",{name:"原子接纳三份 原始数据",exact:true})).toBeDisabled();
 const commands=await writes(request);
 expect(commands.filter(c=>c.path===`/api/v1/ingestions/${ids.ingestion}/recover`)).toHaveLength(1);
 expect(commands.some(c=>c.path.endsWith("/dispatch")||c.path==="/api/v1/ingestions")).toBe(false);
});

test("Certification profile selection reloads that profile's evidence before prepare and evaluate", async ({ page, request }) => {
  await scenario(request, "multiple-certification-profiles", true);
  await page.goto(`http://127.0.0.1:3102/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=certification&profileId=${ids.goldProfile}`);
  const form = page.locator("form").filter({has: page.getByRole("heading", {name:"执行认证", exact:true})});
  await expect(form.getByRole("button", {name:"准备本次认证证据"})).toBeDisabled();
  await form.getByLabel("认证规则").selectOption(ids.secondProfile);
  await expect(page).toHaveURL(new RegExp(`profileId=${ids.secondProfile}`));
  await expect(form.locator('input[name="evidenceSnapshotId"]')).toHaveValue("");
  await form.getByRole("button", {name:"准备本次认证证据"}).click();
  await expect(form.locator('input[name="evidenceSnapshotId"]')).toHaveValue(ids.secondEvidence);
  await form.getByRole("button", {name:"核验并记录认证决定"}).click();
  await expect.poll(async()=> (await writes(request)).find(c=>c.path.endsWith("/certifications"))?.body).toMatchObject({profileId:ids.secondProfile,evidenceSnapshotId:ids.secondEvidence});
  await expect(form.getByRole("status").last()).toContainText("认证通过");
  const commands = await writes(request);
  const preparation = commands.find(c=>c.path.endsWith("/certification-inputs"));
  const evaluation = commands.find(c=>c.path.endsWith("/certifications"));
  expect(preparation.body.profileId).toBe(ids.secondProfile);
  expect(evaluation.body.profileId).toBe(ids.secondProfile);
  expect(evaluation.body.evidenceSnapshotId).toBe(ids.secondEvidence);
});

test("workbench surfaces actionable review and release items", async ({ page }) => {
  await page.goto("/core/workbench");
  const attention = page.getByTestId("needs-attention");
  await expect(attention).toBeVisible();
  await expect(attention.getByRole("heading", { name: "审核、执行与发布待办", exact: true })).toBeVisible();
  await expect(attention.getByText("实体审核队列", { exact: true })).toBeVisible();
  await expect(attention.getByText("发布记录 待发布", { exact: true })).toBeVisible();
  await expect(attention.getByRole("link", { name: "进入审核", exact: true })).toHaveAttribute("href", "/reviews");
  await expect(attention.getByRole("link", { name: "去发布", exact: true })).toHaveAttribute(
    "href",
    `/products/${ids.product}#releases`,
  );
  await attention.getByRole("link", { name: "去发布", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`/products/${ids.product}#releases$`));
  await expect(page.locator("#releases")).toBeVisible();
});

test("attention center lists current actionable work", async ({ page }) => {
  await page.goto("/core/workbench");
  await expect(page.locator('a[href="/attention"]').first()).toHaveAttribute("href", "/attention");
  await expect(page.getByTestId("needs-attention").getByRole("link", { name: "查看全部", exact: true })).toHaveAttribute("href", "/attention");

  await page.goto("/attention");
  await expect(page.getByRole("heading", { name: "待办中心", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "实体审核", exact: true })).toBeVisible();
  await expect(page.getByText("测试来源记录", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "进入审核队列", exact: true })).toHaveAttribute("href", "/reviews");

  await expect(page.getByRole("heading", { name: "发布记录 待发布", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "去发布", exact: true })).toHaveAttribute(
    "href",
    `/products/${ids.product}#releases`,
  );
});

test("Product detail creates a DRAFT Release from explicit DatasetVersion bindings", async ({ page, request }) => {
  await page.goto(productPath);
  const form = page.getByRole("form", { name: "创建 发布记录" });
  await form.getByLabel("输出成果版本").selectOption(ids.goldVersion);
  await form.getByLabel("输出成果用途").selectOption("OUTPUT");
  await form.getByRole("textbox", { name: "发布批次号", exact: true }).fill("R2");
  await form.getByRole("textbox", { name: "发布说明", exact: true }).fill("Browser-created release");
  await form.getByRole("button", { name: "创建 发布草稿", exact: true }).click();

  await expect(form.getByRole("status")).toContainText("发布记录 R2 已创建为 草稿");

  const commands = await writes(request);
  const createCalls = commands.filter((call) => call.path === `/api/v1/data-products/${ids.product}/releases`);
  expect(createCalls).toHaveLength(1);
  expect(createCalls[0].actor).toBe(ids.actor);
  expect(createCalls[0].body).toEqual({
    productVersionId: ids.version,
    releaseNo: "R2",
    datasets: [{ datasetVersionId: ids.goldVersion, role: "OUTPUT" }],
    releaseNotes: "Browser-created release",
    metadata: {},
  });
});

test("readonly runtime never enables review or publishing", async ({ page, request }) => {
  await page.goto("http://127.0.0.1:3101/reviews");
  await expect(page.getByText("当前为只读审核队列")).toBeVisible();
  await expect(page.getByRole("heading", { name: "测试来源记录", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "确认匹配" })).toHaveCount(0);
  await page.goto(`http://127.0.0.1:3101${productPath}`);
  await expect(page.getByRole("button", { name: "确认发布", exact: true })).toBeDisabled();
  await expect(page.getByRole("form", { name: "创建 发布记录" })).toHaveCount(0);
  await expect(page.getByText("发布记录 创建写入未启用", { exact: true })).toBeVisible();

  const snapshot = await state(request);
  const gets = snapshot.requests.filter((call) => call.method === "GET").map((call) => call.path);
  expect(gets).not.toContain(`/api/v1/workspaces/${ids.workspace}/datasets/${ids.goldDataset}/versions`);
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
    await expect(page.getByRole("status").filter({ hasText: "候选；任务状态：成功" }).first()).toBeVisible();
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
  await expect(page.getByRole("status").filter({ hasText: "已选择实体并确认候选；任务状态：成功" })).toBeVisible();
  const commands = await writes(request);
  expect(commands).toHaveLength(1);
  expect(commands[0].body).toEqual({
    reason: "明确选择同名同址实体",
    expectedDecisionId: ids.decision,
    selectedEntityId: ids.alternative,
  });
});

test("Dataset detail links Core source resource and current version facts", async ({ page }) => {
  await page.goto(`/datasets/${ids.goldDataset}`);

  await expect(page.getByRole("link", { name: ids.sourceResource.slice(0, 8) })).toHaveAttribute(
    "href",
    `/resources/${ids.sourceResource}`,
  );
  await expect(page.getByRole("link", { name: ids.goldVersion.slice(0, 8) }).first()).toHaveAttribute(
    "href",
    `/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`,
  );
});

test("execution detail exposes navigable frozen lineage and retry semantics", async ({ page }) => {
  await page.goto(`/production/${ids.job}`);

  await expect(page.getByText("FIXTURE_EXECUTION_FAILED", { exact: true })).toBeVisible();
  await expect(page.getByText("重试将沿用相同的固定输入并创建新的运行记录", { exact: false })).toBeVisible();
  await expect(page.getByRole("link", { name: ids.retryParent.slice(0, 8) })).toHaveAttribute("href", `/production/${ids.retryParent}`);

  const versionHref = `/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`;
  await expect(page.getByRole("link", { name: ids.goldVersion.slice(0, 8) }).first()).toHaveAttribute("href", versionHref);
  await expect(page.getByRole("link", { name: ids.goldVersion, exact: true })).toHaveAttribute("href", versionHref);
});

test("failed Execution retry creates and opens a new immutable Execution", async ({ page, request }) => {
  await page.goto(`/production/${ids.job}`);
  await page.getByRole("button", { name: "重试 运行记录", exact: true }).click();

  await expect(page).toHaveURL(new RegExp(`/production/${ids.retryChild}$`));
  await expect(page.getByText("运行记录 已进入队列", { exact: false })).toBeVisible();
  await expect(page.getByRole("link", { name: ids.job.slice(0, 8) })).toHaveAttribute("href", `/production/${ids.job}`);

  const commands = await writes(request);
  const retryCalls = commands.filter((call) => call.path === `/api/v1/executions/${ids.job}/retry`);
  expect(retryCalls).toHaveLength(1);
  expect(retryCalls[0].actor).toBe(ids.actor);
  expect(retryCalls[0].idempotencyKey).toBe(`ui-retry:${ids.job}`);
});

test("readonly runtime does not enable Execution retry", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/production/${ids.job}`);
  await expect(page.getByRole("button", { name: "重试 运行记录", exact: true })).toBeDisabled();
  expect(await writes(request)).toHaveLength(0);
});

test("Evidence Center uses workspace release read model without per-product release fan-out", async ({ page, request }) => {
  await page.goto("/evidence");
  await expect(page.getByRole("heading", { name: "证据中心", exact: true })).toBeVisible();
  await expect(page.getByText("企业数据产品（示例）", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "查看 Trace →", exact: true })).toHaveAttribute(
    "href",
    `/products/${ids.product}/releases/${ids.release}`,
  );

  const snapshot = await state(request);
  const gets = snapshot.requests.filter((call) => call.method === "GET").map((call) => call.path);
  expect(gets).toContain(`/api/v1/workspaces/${ids.workspace}/product-releases`);
  expect(gets).not.toContain(`/api/v1/workspaces/${ids.workspace}/data-products/${ids.product}/releases`);
});

test("Quality Check preserves attempt identity across refresh and does not duplicate execution", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=quality`);

  const form = page.getByRole("form", { name: "运行 质量评测" });
  const button = form.getByRole("button", { name: "运行 质量评测", exact: true });
  await expect(button).toBeEnabled();
  const attemptId = await form.locator('input[name="assessmentAttemptId"]').inputValue();
  expect(attemptId).toMatch(/^[0-9a-f-]{36}$/);
  await button.click();

  await expect(form.getByRole("status").last()).toContainText("质量评测 已完成");
  // Next's action revalidation can briefly restore the pre-submit URL. Verify
  // the original identity after the action settles, then verify refresh recovery.
  await expect(page).toHaveURL(new RegExp(`qualityAttemptId=${attemptId}`));

  let commands = await writes(request);
  let qualityCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/quality-checks`);
  expect(qualityCalls).toHaveLength(1);
  expect(qualityCalls[0].actor).toBeUndefined();
  expect(qualityCalls[0].authorization).toBe("Bearer review-secret");
  expect(qualityCalls[0].body.assessmentAttemptId).toBe(attemptId);
  expect(qualityCalls[0].body.ruleSetRef).toBe("park/quality/enterprise-activity-quality-v1.yaml");

  await page.reload();
  await expect(page).toHaveURL(new RegExp(`qualityAttemptId=${attemptId}`));
  const afterReload = await state(request);
  expect(afterReload.requests.some((call) => call.method === "GET" && call.path === `/api/v1/quality-assessment-attempts/${attemptId}`)).toBe(true);
  const recoveredForm = page.getByRole("form", { name: "运行 质量评测" });
  await expect(recoveredForm.getByTestId("quality-attempt-status")).toContainText("成功");
  await expect(recoveredForm.getByLabel("质量规则集")).toHaveValue("park/quality/enterprise-activity-quality-v1.yaml");
  await expect(recoveredForm.getByLabel("评测引擎")).toHaveValue("native");
  const latestAssessment = page.locator("section").filter({ has: page.getByRole("heading", { name: "最新评测", exact: true }) });
  await expect(latestAssessment.getByText(ids.qualityAssessment, { exact: true })).toBeVisible();

  commands = await writes(request);
  qualityCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/quality-checks`);
  expect(qualityCalls).toHaveLength(1);

  await recoveredForm.getByRole("button", { name: "开始新的 质量评测", exact: true }).click();
  await expect(page).not.toHaveURL(/qualityAttemptId=/);
  await expect(page.getByRole("form", { name: "运行 质量评测" }).getByRole("button", { name: "运行 质量评测", exact: true })).toBeEnabled();

  commands = await writes(request);
  qualityCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/quality-checks`);
  expect(qualityCalls).toHaveLength(1);
});

test("Compliance Check preserves attempt identity across refresh and does not duplicate result", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=compliance`);

  const form = page.getByRole("form", { name: "运行 合规检查" });
  const button = form.getByRole("button", { name: "运行 合规检查", exact: true });
  await expect(button).toBeEnabled();
  const attemptId = await form.locator('input[name="assessmentAttemptId"]').inputValue();
  expect(attemptId).toMatch(/^[0-9a-f-]{36}$/);
  await button.click();

  await expect(form.getByRole("status").last()).toContainText("合规检查 已完成");
  await expect(page).toHaveURL(new RegExp(`complianceAttemptId=${attemptId}`));

  let commands = await writes(request);
  let complianceCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`);
  expect(complianceCalls).toHaveLength(1);
  expect(complianceCalls[0].actor).toBe(ids.actor);
  expect(complianceCalls[0].body.assessmentAttemptId).toBe(attemptId);
  expect(complianceCalls[0].body.policyRef).toBe("park/compliance/enterprise-activity-compliance-v1.yaml");

  await page.reload();
  await expect(page).toHaveURL(new RegExp(`complianceAttemptId=${attemptId}`));
  const afterReload = await state(request);
  expect(afterReload.requests.some((call) => call.method === "GET" && call.path === `/api/v1/compliance-assessment-attempts/${attemptId}`)).toBe(true);

  const recoveredForm = page.getByRole("form", { name: "运行 合规检查" });
  await expect(recoveredForm.getByTestId("compliance-attempt-status")).toContainText("成功");
  await expect(recoveredForm.getByLabel("合规策略")).toHaveValue("park/compliance/enterprise-activity-compliance-v1.yaml");
  await expect(page.getByText(ids.complianceResult, { exact: true })).toBeVisible();

  commands = await writes(request);
  complianceCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`);
  expect(complianceCalls).toHaveLength(1);

  await recoveredForm.getByRole("button", { name: "开始新的 合规检查", exact: true }).click();
  await expect(page).not.toHaveURL(/complianceAttemptId=/);
  await expect(page.getByRole("form", { name: "运行 合规检查" }).getByRole("button", { name: "运行 合规检查", exact: true })).toBeEnabled();

  commands = await writes(request);
  complianceCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`);
  expect(complianceCalls).toHaveLength(1);
});

test("Compliance Check preserves a custom policy with the recoverable attempt", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=compliance`);

  const form = page.getByRole("form", { name: "运行 合规检查" });
  const customPolicy = "custom/compliance/customer-v2.yaml";
  await form.getByLabel("合规策略").fill(customPolicy);
  const attemptId = await form.locator('input[name="assessmentAttemptId"]').inputValue();
  expect(attemptId).toMatch(/^[0-9a-f-]{36}$/);
  await form.getByRole("button", { name: "运行 合规检查", exact: true }).click();

  await expect(form.getByRole("status").last()).toContainText("合规检查 已完成");
  await expect(page).toHaveURL((url) =>
    url.searchParams.get("complianceAttemptId") === attemptId
    && url.searchParams.get("compliancePolicyRef") === customPolicy,
  );

  await page.reload();
  const recoveredForm = page.getByRole("form", { name: "运行 合规检查" });
  await expect(recoveredForm.getByLabel("合规策略")).toHaveValue(customPolicy);
  await expect(recoveredForm.getByTestId("compliance-attempt-status")).toContainText("成功");

  const commands = await writes(request);
  const complianceCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`);
  expect(complianceCalls).toHaveLength(1);
  expect(complianceCalls[0].body.policyRef).toBe(customPolicy);
});

test("readonly runtime never enables Compliance Check", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=compliance`);
  const form = page.getByRole("form", { name: "运行 合规检查" });
  await expect(form.getByRole("button", { name: "运行 合规检查", exact: true })).toBeDisabled();
  await expect(form.getByText("当前环境未开放合规检查提交", { exact: false })).toBeVisible();

  const commands = await writes(request);
  expect(commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`)).toHaveLength(0);
});

test("readonly runtime never enables Quality Check", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=quality`);
  const form = page.getByRole("form", { name: "运行 质量评测" });
  await expect(form.getByRole("button", { name: "运行 质量评测", exact: true })).toBeDisabled();
  await expect(form.getByText("当前环境未开放质量评测提交", { exact: false })).toBeVisible();

  const commands = await writes(request);
  expect(commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/quality-checks`)).toHaveLength(0);
});

test("Direct Data delivery freezes identity, recovers after refresh, and creates a linked retry", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=eligibility`);

  const panel = page.getByTestId("direct-data-delivery");
  const downloadButton = panel.getByRole("button", { name: "下载 数据下载", exact: true });
  await expect(downloadButton).toBeEnabled();

  const firstDownloadPromise = page.waitForEvent("download");
  await downloadButton.click();
  const firstDownload = await firstDownloadPromise;
  expect(firstDownload.suggestedFilename()).toContain(ids.goldVersion);

  await expect(page).toHaveURL((url) => /^[0-9a-f-]{36}$/i.test(url.searchParams.get("deliveryAttemptKey") ?? ""));
  const firstKey = new URL(page.url()).searchParams.get("deliveryAttemptKey");
  expect(firstKey).toBeTruthy();

  let snapshot = await state(request);
  let deliveryCalls = snapshot.requests.filter((call) => call.method === "POST" && call.path === `/api/v1/workspaces/${ids.workspace}/dataset-versions/${ids.goldVersion}/deliveries`);
  expect(deliveryCalls).toHaveLength(1);
  expect(deliveryCalls[0].authorization).toBe("Bearer delivery-secret");
  expect(deliveryCalls[0].gateway).toBeUndefined();
  expect(deliveryCalls[0].authenticatedPrincipal).toBeUndefined();
  expect(deliveryCalls[0].idempotencyKey).toBe(firstKey);
  expect(deliveryCalls[0].body.profileId).toBe(ids.goldProfile);
  expect(deliveryCalls[0].body.consumer).toBe("GOLD-PILOT-CONSUMER");
  expect(deliveryCalls[0].body.retryOfDeliveryOperationId).toBeUndefined();

  await page.reload();
  const recoveredPanel = page.getByTestId("direct-data-delivery");
  await expect(recoveredPanel.getByRole("button", { name: /下载 数据下载|该 尝试记录 已签发/ })).toBeDisabled();
  await expect(recoveredPanel.getByTestId("delivery-operation-status")).toContainText("授权已释放");

  snapshot = await state(request);
  expect(snapshot.requests.some((call) =>
    call.method === "GET"
    && call.path === `/api/v1/workspaces/${ids.workspace}/direct-data-deliveries/recovery`
    && call.authorization === "Bearer delivery-secret"
  )).toBe(true);
  deliveryCalls = snapshot.requests.filter((call) => call.method === "POST" && call.path.endsWith("/deliveries"));
  expect(deliveryCalls).toHaveLength(1);

  await recoveredPanel.getByRole("button", { name: "创建重试下载 尝试记录", exact: true }).click();
  await expect(page).toHaveURL((url) => {
    const nextKey = url.searchParams.get("deliveryAttemptKey");
    return Boolean(nextKey && nextKey !== firstKey);
  });
  const secondKey = new URL(page.url()).searchParams.get("deliveryAttemptKey");
  expect(secondKey).toBeTruthy();
  expect(new URL(page.url()).searchParams.get("deliveryRetryOf")).toBe(ids.deliveryOperation);

  await page.reload();
  await expect(page).toHaveURL((url) =>
    url.searchParams.get("deliveryAttemptKey") === secondKey
    && url.searchParams.get("deliveryRetryOf") === ids.deliveryOperation,
  );

  const secondDownloadPromise = page.waitForEvent("download");
  await page.getByTestId("direct-data-delivery").getByRole("button", { name: "下载 数据下载", exact: true }).click();
  await secondDownloadPromise;
  await page.reload();
  await expect(page.getByTestId("direct-data-delivery").getByTestId("delivery-operation-status")).toContainText("授权已释放");

  snapshot = await state(request);
  deliveryCalls = snapshot.requests.filter((call) => call.method === "POST" && call.path.endsWith("/deliveries"));
  expect(deliveryCalls).toHaveLength(2);
  expect(deliveryCalls[1].idempotencyKey).toBe(secondKey);
  expect(deliveryCalls[1].body.retryOfDeliveryOperationId).toBe(ids.deliveryOperation);
});

test("Direct Data proxy rejects callers outside the trusted web gateway", async () => {
  const response = await fetch(
    `http://127.0.0.1:3100/api/direct-data-deliveries?idempotencyKey=untrusted-key&consumer=GOLD-PILOT-CONSUMER&versionId=${ids.goldVersion}`,
  );
  expect(response.status).toBe(401);
  const payload = await response.json();
  expect(payload.error.code).toBe("WEB_CALLER_UNTRUSTED");
});

test("Direct Data recovery accepts Core-normalized purpose and action", async ({ page }) => {
  const purpose = "  gold-pilot  ";
  const action = "  use  ";
  await page.goto(
    `/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=eligibility&purpose=${encodeURIComponent(purpose)}&action=${encodeURIComponent(action)}`,
  );

  const panel = page.getByTestId("direct-data-delivery");
  const firstDownloadPromise = page.waitForEvent("download");
  await panel.getByRole("button", { name: "下载 数据下载", exact: true }).click();
  await firstDownloadPromise;

  const attemptKey = new URL(page.url()).searchParams.get("deliveryAttemptKey");
  expect(attemptKey).toBeTruthy();

  await page.reload();
  const recoveredPanel = page.getByTestId("direct-data-delivery");
  await expect(recoveredPanel.getByTestId("delivery-operation-status")).toContainText("授权已释放");
  await expect(recoveredPanel.getByRole("button", { name: "创建重试下载 尝试记录", exact: true })).toBeVisible();
});

test("Direct Data delivery stays disabled for Core-unsupported explicit scopes", async ({ page, request }) => {
  await page.goto(
    `/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=eligibility&scopeType=OBJECT&scopeRef=${ids.sourceResource}`,
  );
  const panel = page.getByTestId("direct-data-delivery");
  await expect(panel.getByRole("button", { name: "下载 数据下载", exact: true })).toBeDisabled();
  await expect(panel.getByText("数据下载 当前只支持 全部资源 scope", { exact: false })).toBeVisible();

  const snapshot = await state(request);
  expect(snapshot.requests.filter((call) => call.method === "POST" && call.path.endsWith("/deliveries"))).toHaveLength(0);
});

test("readonly runtime never enables Direct Data delivery", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=eligibility`);
  const panel = page.getByTestId("direct-data-delivery");
  await expect(panel.getByRole("button", { name: "下载 数据下载", exact: true })).toBeDisabled();
  await expect(panel.getByText("数据下载 下载默认关闭", { exact: false })).toBeVisible();

  const snapshot = await state(request);
  expect(snapshot.requests.filter((call) => call.method === "POST" && call.path.endsWith("/deliveries"))).toHaveLength(0);
});

test("READY DatasetVersion can be invalidated through the Core command", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`);

  const form = page.getByRole("form", { name: "作废 成果版本" });
  const button = form.getByRole("button", { name: "作废 成果版本", exact: true });
  await expect(button).toBeDisabled();
  await form.getByLabel("作废原因（必填）").fill("  上游来源已撤回  ");
  await expect(button).toBeEnabled();
  await button.click();

  await expect(page.getByText("上游来源已撤回", { exact: true })).toBeVisible();
  await expect(page.getByText("已作废", { exact: true }).first()).toBeVisible();
  await expect(page.getByRole("form", { name: "作废 成果版本" })).toHaveCount(0);

  const commands = await writes(request);
  const invalidateCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/invalidate`);
  expect(invalidateCalls).toHaveLength(1);
  expect(invalidateCalls[0].actor).toBe(ids.actor);
  expect(invalidateCalls[0].body).toEqual({ reason: "上游来源已撤回" });
});

test("readonly runtime never enables DatasetVersion invalidation", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`);
  const form = page.getByRole("form", { name: "作废 成果版本" });
  await form.getByLabel("作废原因（必填）").fill("只读环境不应写入");
  await expect(form.getByRole("button", { name: "作废 成果版本", exact: true })).toBeDisabled();
  expect(await writes(request)).toHaveLength(0);
});

test("Gold DatasetVersion explains frozen production proof and current delivery", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`);
  await expect(page.getByRole("link", { name: "Overview" })).toHaveAttribute("aria-current", "page");
  await expect(page.getByText("下一步", { exact: true })).toBeVisible();

  await page.getByRole("link", { name: "Provenance", exact: true }).click();
  await expect(page).toHaveURL(/view=provenance/);
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
  await expect(chain).toContainText("成果生产依据");
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
  await expect(trace).toContainText("Gold Cost / 证据记录 / Audit Trace");
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

  await page.getByRole("link", { name: "Eligibility", exact: true }).click();
  await expect(page).toHaveURL(/view=eligibility/);
  await expect(page.getByRole("heading", { name: "Current Delivery Eligibility" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "预检结果" })).toBeVisible();
  const eligibility = page.getByRole("heading", { name: "预检结果" }).locator("..");
  await expect(eligibility).toContainText("允许");
  await expect(page.getByRole("cell", { name: "成果版本 usability" }).locator("..")).toContainText("允许");
  await expect(page.getByRole("cell", { name: "Current Certification" }).locator("..")).toContainText("允许");
  await expect(page.getByRole("cell", { name: "Current Entitlement" }).locator("..")).toContainText("允许");
  await expect(page.getByText("fixture verified rights", { exact: true }).locator("..")).toContainText("允许");
  await expect(page.getByRole("link", { name: ids.sourceResource.slice(0, 8) })).toHaveAttribute(
    "href",
    `/resources/${ids.sourceResource}`,
  );
  expect(await writes(request)).toHaveLength(0);
});

test("populated eight-gate readiness publishes once with a server idempotency key", async ({ page, request }) => {
  await page.goto(productPath);
  const gates = ["production", "dataset", "rights", "quality", "compliance", "contract", "evidence", "delivery"];
  await expect(page.getByRole("region", { name: "发布记录 发布检查 Checklist" })).toBeVisible();
  for (const gate of gates) await expect(page.getByTestId(`readiness-${gate}`)).toContainText("通过");
  await expect(page.getByText("所有 发布检查 检查项 已通过")).toBeVisible();
  await page.getByRole("button", { name: "确认发布", exact: true }).click();
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
    await expect(page.getByRole("region", { name: "发布记录 发布检查 Checklist" })).toBeVisible();
    if (value === "future-gate") {
      await expect(page.getByTestId("readiness-futureGate")).toContainText("未通过");
    }
    await expect(page.getByText("所有 发布检查 检查项 已通过")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "确认发布", exact: true })).toBeDisabled();
    expect(await writes(request)).toHaveLength(0);
  });
}

test("pending readiness gates ask for validation before remediation", async ({ page, request }) => {
  await scenario(request, "pending-gates");
  await page.goto(productPath);
  for (const gate of ["rights", "quality", "compliance", "contract", "evidence", "delivery"]) {
    const row = page.getByTestId(`readiness-${gate}`);
    await expect(row).toContainText("待处理");
    await expect(row).toContainText("先执行 发布记录 核验；此 检查项 尚未评估");
  }
  await expect(page.getByRole("button", { name: "确认发布", exact: true })).toBeDisabled();
  expect(await writes(request)).toHaveLength(0);
});

test("readiness changes after render: Server Action rechecks and never posts publish", async ({ page, request }) => {
  await page.goto(productPath);
  const publish = page.getByRole("button", { name: "确认发布", exact: true });
  await expect(publish).toBeEnabled();
  await scenario(request, "empty-checks");
  await publish.click();
  await expect(page.getByRole("button", { name: "重新加载并核对结果" })).toBeVisible();
  expect(await writes(request)).toHaveLength(0);
});

for (const [value, route, buttonName] of [["review-conflict", "/reviews", "确认匹配"], ["publish-conflict", productPath, "确认发布"]]) {
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

test("product detail paginates release history before loading readiness", async ({ page, request }) => {
  await scenario(request, "paginated-releases");
  await page.goto(`/products/${ids.product}`);

  await expect(page.getByText("第 1 / 2 页 · 本页 25 条 · 共 30 条")).toBeVisible();
  await expect(page.getByText("发布记录 R1", { exact: true })).toBeVisible();
  await expect(page.getByText("发布记录 R25", { exact: true })).toBeVisible();
  await expect(page.getByText("发布记录 R26", { exact: true })).toHaveCount(0);

  await page.getByRole("link", { name: "下一页" }).click();
  await expect(page).toHaveURL(new RegExp(`/products/${ids.product}\\?offset=25$`));
  await expect(page.getByText("第 2 / 2 页 · 本页 5 条 · 共 30 条")).toBeVisible();
  await expect(page.getByText("发布记录 R26", { exact: true })).toBeVisible();
  await expect(page.getByText("发布记录 R30", { exact: true })).toBeVisible();
  await expect(page.getByText("发布记录 R1", { exact: true })).toHaveCount(0);

  await page.goto(`/products/${ids.product}?offset=1000`);
  await expect(page.getByText("第 2 / 2 页 · 本页 5 条 · 共 30 条")).toBeVisible();
  await expect(page.getByText("发布记录 R26", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "上一页" })).toHaveAttribute(
    "href",
    `/products/${ids.product}?offset=0`,
  );
});

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
