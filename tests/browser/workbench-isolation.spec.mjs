import { test, expect } from "@playwright/test";
import { ids, fixtureToken, unobservedRequestId, requestAttemptId, requestObservationId } from "./fixture-server.mjs";
const control = "http://127.0.0.1:4400/__control", headers = { "x-fixture-token": fixtureToken };
async function reset(request, readFaults = {}, scenario = "schema-requests") {
  expect((await request.post(control + "/reset", { headers, data: { scenario, readFaults } })).ok()).toBeTruthy();
}
async function calls(request) { return (await (await request.get(control + "/state", { headers })).json()).requests; }
async function pure(page, request) {
  const list = await calls(request);
  expect(list.some(c => c.method !== "GET")).toBe(false);
  expect(list.some(c => /metadata-observation|metadata-bindings|ingestion-source|\/ingestions|\/schema-observations$/.test(c.path))).toBe(false);
  const html = await page.content();
  for (const secret of ["rights-fixture-secret", "private-upstream-sentinel", "review-secret"]) expect(html).not.toContain(secret);
  await expect(page.locator("form")).toHaveCount(0);
}
async function governance(page) {
  await expect(page.getByTestId("quality-attention")).toBeVisible();
  await expect(page.getByTestId("schema-attention")).toBeVisible();
  await expect(page.getByTestId("schema-exceptions")).toBeVisible();
  await expect(page.getByTestId("schema-requests")).toContainText("105");
}

for (const fault of ["summary", "executions", "failures", "products", "releases"]) {
  test("workbench isolates " + fault + " failure without hiding independent evidence", async ({ page, request }) => {
    await reset(request, { [fault]: "fail" }); await page.goto("/core/workbench");
    await governance(page);
    const count = page.getByTestId("workbench-attention-total");
    await expect(count).toContainText(["summary", "failures", "releases"].includes(fault) ? "无法确认" : "2 条");
    if (fault !== "releases") await expect(page.getByRole("link", { name: "去发布", exact: true })).toBeVisible();
    if (fault === "products") await expect(page.getByTestId("needs-attention")).toContainText(ids.product + "（名称不可用）");
    if (fault === "executions") await expect(page.getByTestId("recent-executions")).toContainText("无法确认");
    if (fault === "summary") await expect(page.getByTestId("workbench-metrics")).toContainText("无法确认");
    await pure(page, request);
  });
}
for (const fault of ["reviews", "failures", "products", "releases"]) {
  test("attention isolates " + fault + " failure and keeps usable queues", async ({ page, request }) => {
    await reset(request, { [fault]: "fail" }); await page.goto("/attention");
    await governance(page);
    await expect(page.getByTestId("attention-total")).toContainText(fault === "products" ? "2 条" : "无法确认");
    if (fault !== "releases") await expect(page.getByTestId("release-queue-ready").getByRole("link", { name: "去发布", exact: true })).toBeVisible();
    if (fault !== "reviews") await expect(page.getByTestId("review-queue")).toContainText("1 条待人工决策");
    if (fault === "products") await expect(page.getByTestId("release-queue-ready")).toContainText(ids.product + "（名称不可用）");
    await pure(page, request);
  });
}
test("denied and malformed host reads fail closed locally on both pages", async ({ page, request }) => {
  for (const mode of ["denied", "invalid"]) {
    await reset(request, { summary: mode, reviews: mode, releases: mode });
    for (const path of ["/core/workbench", "/attention"]) {
      await page.goto(path); await governance(page);
      await expect(page.getByTestId(path === "/core/workbench" ? "workbench-attention-total" : "attention-total")).toContainText("无法确认");
      await pure(page, request);
    }
  }
});
test("each governance failure remains local while original queues stay readable", async ({ page, request }) => {
  const panels = { quality: "quality-attention", schema: "schema-attention", exceptions: "schema-exceptions", requests: "schema-requests" };
  for (const [fault, panel] of Object.entries(panels)) {
    await reset(request, { [fault]: "fail" });
    for (const path of ["/core/workbench", "/attention"]) {
      await page.goto(path);
      await expect(page.getByTestId(panel + "-unavailable")).toContainText("无法确认");
      await expect(page.getByTestId(path === "/core/workbench" ? "workbench-attention-total" : "attention-total")).toContainText("2 条");
      for (const other of Object.values(panels).filter(value => value !== panel)) await expect(page.getByTestId(other)).toBeVisible();
      await pure(page, request);
    }
  }
});
test("all read failures preserve page identity without invented clear results", async ({ page, request }, info) => {
  await reset(request, Object.fromEntries(["summary", "reviews", "executions", "failures", "products", "releases", "quality", "schema", "exceptions", "requests", "attempts"].map(key => [key, "fail"])));
  for (const path of ["/core/workbench", "/attention"]) {
    await page.goto(path);
    await expect(page.getByRole("heading", { level: 1, name: path === "/core/workbench" ? "数据产品工作台" : "待办中心" })).toBeVisible();
    await expect(page.getByTestId(path === "/core/workbench" ? "workbench-attention-total" : "attention-total")).toContainText("无法确认");
    await expect(page.locator(".badge").filter({ hasText: /^(CLEAR|READY)$/ })).toHaveCount(0);
    await expect(page.getByText("Core Read Model 暂时不可用。", { exact: true })).toHaveCount(0);
    await page.screenshot({ path: info.outputPath(path === "/core/workbench" ? "workbench-all-unknown.png" : "attention-all-unknown.png"), fullPage: true });
    await pure(page, request);
  }
});
test("successful empty reads remain distinct from unavailable totals and recover on reload", async ({ page, request }) => {
  await reset(request, {}, "isolation-empty");
  await page.goto("/core/workbench"); await expect(page.getByTestId("workbench-attention-total")).toContainText("0 条");
  await expect(page.getByTestId("workbench-attention-total")).toContainText("暂无待办");
  await expect(page.getByText("当前没有需要人工处理的事项", { exact: true })).toBeVisible();
  await page.goto("/attention"); await expect(page.getByTestId("attention-total")).toContainText("0 条");
  await reset(request, { reviews: "fail" }, "isolation-empty");
  await page.reload(); await expect(page.getByTestId("attention-total")).toContainText("无法确认");
  await reset(request, {}, "isolation-empty");
  await page.reload(); await expect(page.getByTestId("attention-total")).toContainText("0 条");
  await pure(page, request);
});
test("a failed selected release page retains snapshot count and bounded return navigation", async ({ page, request }) => {
  await reset(request, { "ready-page": "fail" });
  await page.goto("/attention?readyReleaseOffset=25&schemaRequestView=ATTEMPT_RECORDED&schemaRequestOffset=100");
  await expect(page.getByTestId("attention-total")).toContainText("2 条");
  const panel = page.getByTestId("release-queue-ready");
  await expect(panel).toContainText("1 个 发布记录"); await expect(panel).toContainText("无法确认");
  await expect(panel.getByRole("link", { name: "返回该分区首页" })).toHaveAttribute("href", /schemaRequestView=ATTEMPT_RECORDED&schemaRequestOffset=100/);
  await panel.getByRole("link", { name: "返回该分区首页" }).click();
  await expect(panel.getByRole("link", { name: "去发布", exact: true })).toHaveAttribute("href", "/products/" + ids.product + "#releases");
  await pure(page, request);
});
test("selected release pages remain independent of a failed shared snapshot", async ({ page, request }) => {
  await reset(request, { releases: "fail" });
  await page.goto("/attention?readyReleaseOffset=25");
  await expect(page.getByTestId("attention-total")).toContainText("无法确认");
  const panel = page.getByTestId("release-queue-ready");
  await expect(panel).toContainText("1 个 发布记录"); await expect(panel.getByTestId("read-unavailable")).toHaveCount(0);
  await expect(page.getByTestId("release-queue-failed")).toContainText("无法确认");
  await pure(page, request);
});
test("readiness failure keeps the known release and its exact navigation", async ({ page, request }) => {
  await reset(request, { readiness: "fail" }, "isolation-validating"); await page.goto("/core/workbench");
  const panel = page.getByTestId("needs-attention");
  await expect(page.getByTestId("workbench-attention-total")).toContainText("2 条");
  await expect(panel).toContainText("发布检查 详情：无法确认");
  await expect(panel.getByRole("link", { name: "查看 阻断原因", exact: true })).toHaveAttribute("href", "/products/" + ids.product + "#releases");
  await pure(page, request);
});
test("pending product labels do not block independent facts or evidence", async ({ page, request }) => {
  for (const path of ["/core/workbench", "/attention"]) {
    await reset(request, { products: "hold" });
    const panel = page.getByTestId(path === "/core/workbench" ? "needs-attention" : "release-queue-ready");
    try {
      await page.goto(path, { waitUntil: "commit" });
      await expect(page.getByRole("link", { name: "去发布", exact: true })).toBeVisible();
      await expect(panel).toContainText(ids.product + "（名称读取中）");
      await governance(page);
      const state = await (await request.get(control + "/state", { headers })).json();
      expect(state.heldReadCount).toBe(1);
    } finally {
      expect((await request.post(control + "/release-reads", { headers })).ok()).toBeTruthy();
    }
    await expect(panel).not.toContainText("名称读取中");
    await pure(page, request);
  }
});
for (const path of ["/core/workbench", "/attention"]) {
  for (const [fault, panel, anchor] of [
    ["quality", "quality-attention", "quality-attention"],
    ["schema", "schema-attention", "source-schema-attention"],
    ["exceptions", "schema-exceptions", "schema-exception-history"],
    ["requests", "schema-requests", "schema-unobserved-requests"],
  ]) {
    test(path + " isolates pending " + fault + " governance GET before it resolves", async ({ page, request }) => {
      await reset(request, { [fault]: "hold" });
      const pending = page.getByTestId(panel + "-pending");
      try {
        await page.goto(path, { waitUntil: "commit", timeout: 5000 });
        await expect(page.getByRole("heading", { level: 1, name: path === "/core/workbench" ? "数据产品工作台" : "待办中心" })).toBeVisible();
        await expect(pending).toContainText("无法确认");
        await expect(pending).toContainText("读取中");
        await expect(pending).not.toContainText(/CLEAR|READY|0 条|0 个/);
        await expect(page.locator("#" + anchor)).toHaveCount(1);
        await expect(page.getByTestId(path === "/core/workbench" ? "workbench-attention-total" : "attention-total")).toContainText("2 条");
        await expect(page.getByRole("link", { name: "去发布", exact: true })).toHaveAttribute("href", "/products/" + ids.product + "#releases");
        for (const other of ["quality-attention", "schema-attention", "schema-exceptions", "schema-requests"].filter(value => value !== panel)) {
          await expect(page.getByTestId(other)).toBeVisible();
        }
        if (path === "/attention" && fault !== "requests") {
          const link = page.getByTestId("schema-requests").locator("tbody tr").filter({ hasText: unobservedRequestId(1) }).getByRole("link", { name: "查看精确请求" });
          await expect(link).toHaveAttribute("href", new RegExp("^/resources/" + ids.sourceResource + "\\?view=source-schema-request&requestId=" + unobservedRequestId(1) + "&"));
        }
        const state = await (await request.get(control + "/state", { headers })).json();
        expect(state.heldReadCount).toBe(1);
        await pure(page, request);
      } finally {
        expect((await request.post(control + "/release-reads", { headers })).ok()).toBeTruthy();
      }
      await expect(pending).toHaveCount(0);
      await expect(page.getByTestId(panel)).toBeVisible();
      await expect(page.locator("#" + anchor)).toHaveCount(1);
      await pure(page, request);
    });
  }
}

test("precise attempt and arriving observation round trip survives an unrelated release failure", async ({ page, request }, info) => {
  await reset(request, { releases: "fail" });
  await page.goto("/attention?schemaRequestView=ALL&schemaRequestOffset=100");
  await expect(page.getByTestId("attention-total")).toContainText("无法确认");
  await page.getByTestId("schema-requests").locator("tbody tr").filter({ hasText: unobservedRequestId(105) }).getByRole("link", { name: "查看精确请求" }).click();
  await page.getByRole("link", { name: "查看 尝试记录 " + requestAttemptId(105), exact: true }).click();
  await expect(page.getByTestId("schema-selected-attempt")).toContainText(requestAttemptId(105));
  await request.post(control + "/scenario", { headers, data: { scenario: "schema-requests-arrived", readFaults: { releases: "fail" } } });
  await page.reload(); await page.getByRole("link", { name: "查看指定 尝试记录 的精确观察" }).click();
  await expect(page.getByTestId("schema-observation-evidence")).toContainText(requestObservationId(105));
  await page.getByRole("link", { name: "返回精确请求证据", exact: true }).click();
  await expect(page.getByTestId("schema-selected-attempt")).toContainText(requestAttemptId(105));
  await page.getByRole("link", { name: "返回尚无结果的扫描请求", exact: true }).click();
  await expect(page).toHaveURL(/schemaRequestView=ALL&schemaRequestOffset=100#schema-unobserved-requests$/);
  await expect(page.getByTestId("schema-requests").locator("tbody tr")).toHaveCount(4);
  await expect(page.getByTestId("attention-total")).toContainText("无法确认");
  await page.screenshot({ path: info.outputPath("attention-partial-with-evidence.png"), fullPage: true });
  await pure(page, request);
});
