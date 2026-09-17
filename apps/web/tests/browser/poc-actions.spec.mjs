import { test, expect } from "@playwright/test";

const fixture = "http://127.0.0.1:18080";
const productId = "33333333-3333-4333-8333-333333333333";

async function reset(request, mode = "ready") {
  const response = await request.post(`${fixture}/__test/reset`, { data: { mode } });
  expect(response.ok()).toBeTruthy();
}

async function fixtureState(request) {
  const response = await request.get(`${fixture}/__test/state`);
  expect(response.ok()).toBeTruthy();
  return response.json();
}

test.beforeEach(async ({ request }) => {
  await reset(request, "ready");
});

test("review reason is mandatory and confirm uses the real Next Server Action", async ({ page, request }) => {
  await page.goto("/reviews");
  const form = page.getByRole("form", { name: "审核 深圳星云科技集团有限公司" });
  const confirm = form.getByRole("button", { name: "确认匹配" });
  const reject = form.getByRole("button", { name: "拒绝匹配" });
  await expect(confirm).toBeDisabled();
  await expect(reject).toBeDisabled();

  await form.getByLabel("审核理由（必填）").fill("核对规范化名称、候选实体与来源记录后确认匹配");
  await expect(confirm).toBeEnabled();
  await confirm.click();

  await expect(page.getByRole("status").filter({ hasText: /已确认候选；任务状态：SUCCEEDED/ }).first()).toBeVisible();
  await expect(page.getByText("当前页没有待审核候选")).toBeVisible();
  const state = await fixtureState(request);
  expect(state.reviewDecision).toBe("confirm");
  expect(state.reviewStatus).toBe("CONFIRMED");
});

test("reject uses the same guarded review path", async ({ page, request }) => {
  await page.goto("/reviews");
  const form = page.getByRole("form", { name: "审核 深圳星云科技集团有限公司" });
  await form.getByLabel("审核理由（必填）").fill("来源与候选实体不一致，拒绝此映射");
  await form.getByRole("button", { name: "拒绝匹配" }).click();
  await expect(page.getByRole("status").filter({ hasText: /已拒绝候选；任务状态：SUCCEEDED/ }).first()).toBeVisible();
  const state = await fixtureState(request);
  expect(state.reviewDecision).toBe("reject");
  expect(state.reviewStatus).toBe("REJECTED");
});

test("readonly deployment renders provenance but no review form", async ({ page }) => {
  await page.goto("http://127.0.0.1:3002/reviews");
  await expect(page.getByText("当前为只读审核队列")).toBeVisible();
  await expect(page.getByText("深圳星云科技集团有限公司")).toBeVisible();
  await expect(page.getByLabel("审核理由（必填）")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "确认匹配" })).toHaveCount(0);
});

test("complete readiness enables publish and submits exactly once", async ({ page, request }) => {
  await page.goto(`/products/${productId}`);
  await expect(page.getByText("所有 Release Readiness 条件已完整通过")).toBeVisible();
  const publish = page.getByRole("button", { name: "发布 Release" });
  await expect(publish).toBeEnabled();
  await publish.click();
  await expect(page.getByRole("status").filter({ hasText: /Release R1 已发布/ }).first()).toBeVisible();
  const state = await fixtureState(request);
  expect(state.publishCount).toBe(1);
  expect(state.releaseStatus).toBe("PUBLISHED");
});

test("contradictory stale readiness fails closed in the hydrated UI", async ({ page, request }) => {
  await reset(request, "stale");
  await page.goto(`/products/${productId}`);
  await expect(page.getByText(/GATE_NOT_PASS:rights/)).toBeVisible();
  await expect(page.getByRole("button", { name: "发布 Release" })).toBeDisabled();
  const state = await fixtureState(request);
  expect(state.publishCount).toBe(0);
});

test("future non-PASS gate also fails closed", async ({ page, request }) => {
  await reset(request, "future");
  await page.goto(`/products/${productId}`);
  await expect(page.getByText(/EXTRA_GATE_NOT_PASS:externalPublication/)).toBeVisible();
  await expect(page.getByRole("button", { name: "发布 Release" })).toBeDisabled();
  const state = await fixtureState(request);
  expect(state.publishCount).toBe(0);
});

test("Core publish conflict is surfaced without an automatic retry", async ({ page, request }) => {
  await reset(request, "conflict");
  await page.goto(`/products/${productId}`);
  const publish = page.getByRole("button", { name: "发布 Release" });
  await expect(publish).toBeEnabled();
  await publish.click();
  await expect(page.getByRole("alert").filter({ hasText: /PRODUCT_RELEASE_NOT_READY/ }).first()).toBeVisible();
  await expect(page.getByRole("button", { name: "重新加载并核对结果" })).toBeVisible();
  const state = await fixtureState(request);
  expect(state.publishCount).toBe(1);
  expect(state.releaseStatus).toBe("READY");
});
