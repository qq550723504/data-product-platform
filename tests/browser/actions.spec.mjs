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
    expect(commands[0].actor).toBe(ids.actor);
    expect(commands[0].body).toEqual({ reason: "已核对测试来源", expectedDecisionId: ids.decision });
  });
}

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
