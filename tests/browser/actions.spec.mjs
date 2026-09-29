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

test("workbench surfaces actionable review and release items", async ({ page }) => {
  await page.goto("/");
  const attention = page.getByTestId("needs-attention");
  await expect(attention).toBeVisible();
  await expect(attention.getByRole("heading", { name: "Needs Attention", exact: true })).toBeVisible();
  await expect(attention.getByText("实体审核队列", { exact: true })).toBeVisible();
  await expect(attention.getByText("Release 待发布", { exact: true })).toBeVisible();
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
  await page.goto("/");
  await expect(page.locator('a[href="/attention"]').first()).toHaveAttribute("href", "/attention");
  await expect(page.getByTestId("needs-attention").getByRole("link", { name: "查看全部", exact: true })).toHaveAttribute("href", "/attention");

  await page.goto("/attention");
  await expect(page.getByRole("heading", { name: "待办中心", exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "实体审核", exact: true })).toBeVisible();
  await expect(page.getByText("测试来源记录", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "进入审核队列", exact: true })).toHaveAttribute("href", "/reviews");

  await expect(page.getByRole("heading", { name: "Release 待发布", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "去发布", exact: true })).toHaveAttribute(
    "href",
    `/products/${ids.product}#releases`,
  );
});

test("Product detail creates a DRAFT Release from explicit DatasetVersion bindings", async ({ page, request }) => {
  await page.goto(productPath);
  const form = page.getByRole("form", { name: "创建 ProductRelease" });
  await form.getByLabel("Gold output DatasetVersion").selectOption(ids.goldVersion);
  await form.getByLabel("Gold output Role").selectOption("OUTPUT");
  await form.getByRole("textbox", { name: "Release No", exact: true }).fill("R2");
  await form.getByRole("textbox", { name: "Release Notes", exact: true }).fill("Browser-created release");
  await form.getByRole("button", { name: "创建 Release Draft", exact: true }).click();

  await expect(form.getByRole("status")).toContainText("Release R2 已创建为 DRAFT");

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
  await expect(page.getByRole("button", { name: "发布 Release", exact: true })).toBeDisabled();
  await expect(page.getByRole("form", { name: "创建 ProductRelease" })).toHaveCount(0);
  await expect(page.getByText("Release 创建写入未启用", { exact: true })).toBeVisible();

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
  await expect(page.getByText("Core Retry 会基于同一冻结输入创建新的 Execution", { exact: false })).toBeVisible();
  await expect(page.getByRole("link", { name: ids.retryParent.slice(0, 8) })).toHaveAttribute("href", `/production/${ids.retryParent}`);

  const versionHref = `/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`;
  await expect(page.getByRole("link", { name: ids.goldVersion.slice(0, 8) }).first()).toHaveAttribute("href", versionHref);
  await expect(page.getByRole("link", { name: ids.goldVersion, exact: true })).toHaveAttribute("href", versionHref);
});

test("failed Execution retry creates and opens a new immutable Execution", async ({ page, request }) => {
  await page.goto(`/production/${ids.job}`);
  await page.getByRole("button", { name: "重试 Execution", exact: true }).click();

  await expect(page).toHaveURL(new RegExp(`/production/${ids.retryChild}$`));
  await expect(page.getByText("Execution 已进入队列", { exact: false })).toBeVisible();
  await expect(page.getByRole("link", { name: ids.job.slice(0, 8) })).toHaveAttribute("href", `/production/${ids.job}`);

  const commands = await writes(request);
  const retryCalls = commands.filter((call) => call.path === `/api/v1/executions/${ids.job}/retry`);
  expect(retryCalls).toHaveLength(1);
  expect(retryCalls[0].actor).toBe(ids.actor);
  expect(retryCalls[0].idempotencyKey).toBe(`ui-retry:${ids.job}`);
});

test("readonly runtime does not enable Execution retry", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/production/${ids.job}`);
  await expect(page.getByRole("button", { name: "重试 Execution", exact: true })).toBeDisabled();
  expect(await writes(request)).toHaveLength(0);
});

test("Evidence Center uses workspace release read model without per-product release fan-out", async ({ page, request }) => {
  await page.goto("/evidence");
  await expect(page.getByRole("heading", { name: "证据中心", exact: true })).toBeVisible();
  await expect(page.getByText("浏览器验收产品", { exact: true })).toBeVisible();
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

  const form = page.getByRole("form", { name: "运行 Quality Check" });
  const button = form.getByRole("button", { name: "运行 Quality Check", exact: true });
  await expect(button).toBeEnabled();
  await button.click();

  await expect(page).toHaveURL(/qualityAttemptId=[0-9a-f-]{36}/);
  const attemptId = new URL(page.url()).searchParams.get("qualityAttemptId");
  expect(attemptId).toBeTruthy();
  await expect(form.getByRole("status").last()).toContainText("Quality Check 已完成");

  let commands = await writes(request);
  let qualityCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/quality-checks`);
  expect(qualityCalls).toHaveLength(1);
  expect(qualityCalls[0].actor).toBe(ids.actor);
  expect(qualityCalls[0].body.assessmentAttemptId).toBe(attemptId);
  expect(qualityCalls[0].body.ruleSetRef).toBe("park/quality/enterprise-activity-quality-v1.yaml");

  await page.reload();
  await expect(page).toHaveURL(new RegExp(`qualityAttemptId=${attemptId}`));
  const afterReload = await state(request);
  expect(afterReload.requests.some((call) => call.method === "GET" && call.path === `/api/v1/quality-assessment-attempts/${attemptId}`)).toBe(true);
  const recoveredForm = page.getByRole("form", { name: "运行 Quality Check" });
  await expect(recoveredForm.getByTestId("quality-attempt-status")).toContainText("SUCCEEDED");
  await expect(recoveredForm.getByLabel("Quality Rule Set")).toHaveValue("park/quality/enterprise-activity-quality-v1.yaml");
  await expect(recoveredForm.getByLabel("Quality Engine")).toHaveValue("native");
  await expect(page.getByText(ids.qualityAssessment, { exact: true })).toBeVisible();

  commands = await writes(request);
  qualityCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/quality-checks`);
  expect(qualityCalls).toHaveLength(1);

  await recoveredForm.getByRole("button", { name: "开始新的 Quality Check", exact: true }).click();
  await expect(page).not.toHaveURL(/qualityAttemptId=/);
  await expect(page.getByRole("form", { name: "运行 Quality Check" }).getByRole("button", { name: "运行 Quality Check", exact: true })).toBeEnabled();

  commands = await writes(request);
  qualityCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/quality-checks`);
  expect(qualityCalls).toHaveLength(1);
});

test("Compliance Check preserves attempt identity across refresh and does not duplicate result", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=compliance`);

  const form = page.getByRole("form", { name: "运行 Compliance Check" });
  const button = form.getByRole("button", { name: "运行 Compliance Check", exact: true });
  await expect(button).toBeEnabled();
  await button.click();

  await expect(page).toHaveURL(/complianceAttemptId=[0-9a-f-]{36}/);
  const attemptId = new URL(page.url()).searchParams.get("complianceAttemptId");
  expect(attemptId).toBeTruthy();
  await expect(form.getByRole("status").last()).toContainText("Compliance Check 已完成");

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

  const recoveredForm = page.getByRole("form", { name: "运行 Compliance Check" });
  await expect(recoveredForm.getByTestId("compliance-attempt-status")).toContainText("SUCCEEDED");
  await expect(recoveredForm.getByLabel("Compliance Policy")).toHaveValue("park/compliance/enterprise-activity-compliance-v1.yaml");
  await expect(page.getByText(ids.complianceResult, { exact: true })).toBeVisible();

  commands = await writes(request);
  complianceCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`);
  expect(complianceCalls).toHaveLength(1);

  await recoveredForm.getByRole("button", { name: "开始新的 Compliance Check", exact: true }).click();
  await expect(page).not.toHaveURL(/complianceAttemptId=/);
  await expect(page.getByRole("form", { name: "运行 Compliance Check" }).getByRole("button", { name: "运行 Compliance Check", exact: true })).toBeEnabled();

  commands = await writes(request);
  complianceCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`);
  expect(complianceCalls).toHaveLength(1);
});

test("Compliance Check preserves a custom policy with the recoverable attempt", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=compliance`);

  const form = page.getByRole("form", { name: "运行 Compliance Check" });
  const customPolicy = "custom/compliance/customer-v2.yaml";
  await form.getByLabel("Compliance Policy").fill(customPolicy);
  await form.getByRole("button", { name: "运行 Compliance Check", exact: true }).click();

  await expect(page).toHaveURL((url) =>
    /^[0-9a-f-]{36}$/i.test(url.searchParams.get("complianceAttemptId") ?? "")
    && url.searchParams.get("compliancePolicyRef") === customPolicy,
  );

  const attemptId = new URL(page.url()).searchParams.get("complianceAttemptId");
  expect(attemptId).toBeTruthy();

  await page.reload();
  const recoveredForm = page.getByRole("form", { name: "运行 Compliance Check" });
  await expect(recoveredForm.getByLabel("Compliance Policy")).toHaveValue(customPolicy);
  await expect(recoveredForm.getByTestId("compliance-attempt-status")).toContainText("SUCCEEDED");

  const commands = await writes(request);
  const complianceCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`);
  expect(complianceCalls).toHaveLength(1);
  expect(complianceCalls[0].body.policyRef).toBe(customPolicy);
});

test("readonly runtime never enables Compliance Check", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=compliance`);
  const form = page.getByRole("form", { name: "运行 Compliance Check" });
  await expect(form.getByRole("button", { name: "运行 Compliance Check", exact: true })).toBeDisabled();
  await expect(form.getByText("Compliance 写入默认关闭", { exact: false })).toBeVisible();

  const commands = await writes(request);
  expect(commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/compliance-checks`)).toHaveLength(0);
});

test("readonly runtime never enables Quality Check", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=quality`);
  const form = page.getByRole("form", { name: "运行 Quality Check" });
  await expect(form.getByRole("button", { name: "运行 Quality Check", exact: true })).toBeDisabled();
  await expect(form.getByText("Quality 写入默认关闭", { exact: false })).toBeVisible();

  const commands = await writes(request);
  expect(commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/quality-checks`)).toHaveLength(0);
});

test("Direct Data delivery freezes identity, recovers after refresh, and creates a linked retry", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=eligibility`);

  const panel = page.getByTestId("direct-data-delivery");
  const downloadButton = panel.getByRole("button", { name: "下载 Direct Data", exact: true });
  await expect(downloadButton).toBeEnabled();

  const firstDownloadPromise = page.waitForEvent("download");
  await downloadButton.click();
  const firstDownload = await firstDownloadPromise;
  expect(firstDownload.suggestedFilename()).toContain(ids.goldVersion);

  await expect(page).toHaveURL((url) => /^[0-9a-f-]{36}$/i.test(url.searchParams.get("deliveryAttemptKey") ?? ""));
  const firstKey = new URL(page.url()).searchParams.get("deliveryAttemptKey");
  expect(firstKey).toBeTruthy();
  await expect(panel.getByTestId("delivery-operation-status")).toContainText("ISSUED");

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
  await expect(recoveredPanel.getByTestId("delivery-operation-status")).toContainText("ISSUED");

  snapshot = await state(request);
  expect(snapshot.requests.some((call) =>
    call.method === "GET"
    && call.path === `/api/v1/workspaces/${ids.workspace}/direct-data-deliveries/recovery`
    && call.authorization === "Bearer delivery-secret"
  )).toBe(true);
  deliveryCalls = snapshot.requests.filter((call) => call.method === "POST" && call.path.endsWith("/deliveries"));
  expect(deliveryCalls).toHaveLength(1);

  await recoveredPanel.getByRole("button", { name: "创建重试下载 attempt", exact: true }).click();
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
  await page.getByTestId("direct-data-delivery").getByRole("button", { name: "下载 Direct Data", exact: true }).click();
  await secondDownloadPromise;
  await expect(page.getByTestId("direct-data-delivery").getByTestId("delivery-operation-status")).toContainText("ISSUED");

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

test("readonly runtime never enables Direct Data delivery", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/datasets/${ids.goldDataset}/versions/${ids.goldVersion}?view=eligibility`);
  const panel = page.getByTestId("direct-data-delivery");
  await expect(panel.getByRole("button", { name: "下载 Direct Data", exact: true })).toBeDisabled();
  await expect(panel.getByText("Direct Data 下载默认关闭", { exact: false })).toBeVisible();

  const snapshot = await state(request);
  expect(snapshot.requests.filter((call) => call.method === "POST" && call.path.endsWith("/deliveries"))).toHaveLength(0);
});

test("READY DatasetVersion can be invalidated through the Core command", async ({ page, request }) => {
  await page.goto(`/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`);

  const form = page.getByRole("form", { name: "作废 DatasetVersion" });
  const button = form.getByRole("button", { name: "作废 DatasetVersion", exact: true });
  await expect(button).toBeDisabled();
  await form.getByLabel("作废原因（必填）").fill("  上游来源已撤回  ");
  await expect(button).toBeEnabled();
  await button.click();

  await expect(page.getByText("上游来源已撤回", { exact: true })).toBeVisible();
  await expect(page.getByText("INVALID", { exact: true }).first()).toBeVisible();
  await expect(page.getByRole("form", { name: "作废 DatasetVersion" })).toHaveCount(0);

  const commands = await writes(request);
  const invalidateCalls = commands.filter((call) => call.path === `/api/v1/dataset-versions/${ids.goldVersion}/invalidate`);
  expect(invalidateCalls).toHaveLength(1);
  expect(invalidateCalls[0].actor).toBe(ids.actor);
  expect(invalidateCalls[0].body).toEqual({ reason: "上游来源已撤回" });
});

test("readonly runtime never enables DatasetVersion invalidation", async ({ page, request }) => {
  await page.goto(`http://127.0.0.1:3101/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`);
  const form = page.getByRole("form", { name: "作废 DatasetVersion" });
  await form.getByLabel("作废原因（必填）").fill("只读环境不应写入");
  await expect(form.getByRole("button", { name: "作废 DatasetVersion", exact: true })).toBeDisabled();
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

  await page.getByRole("link", { name: "Eligibility", exact: true }).click();
  await expect(page).toHaveURL(/view=eligibility/);
  await expect(page.getByRole("heading", { name: "Current Delivery Eligibility" })).toBeVisible();
  await expect(page.getByRole("heading", { name: "预检结果" })).toBeVisible();
  const eligibility = page.getByRole("heading", { name: "预检结果" }).locator("..");
  await expect(eligibility).toContainText("ALLOWED");
  await expect(page.getByRole("cell", { name: "DatasetVersion usability" }).locator("..")).toContainText("ALLOWED");
  await expect(page.getByRole("cell", { name: "Current Certification" }).locator("..")).toContainText("ALLOWED");
  await expect(page.getByRole("cell", { name: "Current Entitlement" }).locator("..")).toContainText("ALLOWED");
  await expect(page.getByText("fixture verified rights", { exact: true }).locator("..")).toContainText("ALLOWED");
  await expect(page.getByRole("link", { name: ids.sourceResource.slice(0, 8) })).toHaveAttribute(
    "href",
    `/resources/${ids.sourceResource}`,
  );
  expect(await writes(request)).toHaveLength(0);
});

test("populated eight-gate readiness publishes once with a server idempotency key", async ({ page, request }) => {
  await page.goto(productPath);
  const gates = ["production", "dataset", "rights", "quality", "compliance", "contract", "evidence", "delivery"];
  await expect(page.getByRole("region", { name: "Release Readiness Checklist" })).toBeVisible();
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
    await expect(page.getByRole("region", { name: "Release Readiness Checklist" })).toBeVisible();
    if (value === "future-gate") {
      await expect(page.getByTestId("readiness-futureGate")).toContainText("FAIL");
    }
    await expect(page.getByText("所有 Readiness Gate 已通过")).toHaveCount(0);
    await expect(page.getByRole("button", { name: "发布 Release", exact: true })).toBeDisabled();
    expect(await writes(request)).toHaveLength(0);
  });
}

test("pending readiness gates ask for validation before remediation", async ({ page, request }) => {
  await scenario(request, "pending-gates");
  await page.goto(productPath);
  for (const gate of ["rights", "quality", "compliance", "contract", "evidence", "delivery"]) {
    const row = page.getByTestId(`readiness-${gate}`);
    await expect(row).toContainText("PENDING");
    await expect(row).toContainText("先执行 Release Validation；此 Gate 尚未评估");
  }
  await expect(page.getByRole("button", { name: "发布 Release", exact: true })).toBeDisabled();
  expect(await writes(request)).toHaveLength(0);
});

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

test("product detail paginates release history before loading readiness", async ({ page, request }) => {
  await scenario(request, "paginated-releases");
  await page.goto(`/products/${ids.product}`);

  await expect(page.getByText("第 1 / 2 页 · 本页 25 条 · 共 30 条")).toBeVisible();
  await expect(page.getByText("Release R1", { exact: true })).toBeVisible();
  await expect(page.getByText("Release R25", { exact: true })).toBeVisible();
  await expect(page.getByText("Release R26", { exact: true })).toHaveCount(0);

  await page.getByRole("link", { name: "下一页" }).click();
  await expect(page).toHaveURL(new RegExp(`/products/${ids.product}\\?offset=25$`));
  await expect(page.getByText("第 2 / 2 页 · 本页 5 条 · 共 30 条")).toBeVisible();
  await expect(page.getByText("Release R26", { exact: true })).toBeVisible();
  await expect(page.getByText("Release R30", { exact: true })).toBeVisible();
  await expect(page.getByText("Release R1", { exact: true })).toHaveCount(0);

  await page.goto(`/products/${ids.product}?offset=1000`);
  await expect(page.getByText("第 2 / 2 页 · 本页 5 条 · 共 30 条")).toBeVisible();
  await expect(page.getByText("Release R26", { exact: true })).toBeVisible();
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
