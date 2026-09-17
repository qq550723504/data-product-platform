import { test, expect } from "@playwright/test";
import { fixtureToken } from "./fixture-server.mjs";
test("CSV ingestion is read-only by default even after local preview", async ({ page, request }) => {
  const headers = { "x-fixture-token": fixtureToken };
  expect((await request.post("http://127.0.0.1:4400/__control/reset", { headers, data: {} })).ok()).toBeTruthy();
  await page.goto("/ingest");
  await expect(page.getByText("当前为只读接入预览")).toBeVisible();
  const form = page.getByRole("form", { name: "接入 CSV" });
  await form.getByLabel("数据集名称").fill("readonly");
  await form.getByLabel("数据来源说明").fill("synthetic");
  await form.getByLabel("本次处理用途").fill("test");
  await form.getByRole("checkbox").check();
  await form.getByLabel("选择 CSV 文件").setInputFiles({ name: "preview.csv", mimeType: "text/csv", buffer: Buffer.from("source_company_id,company_name\na,b") });
  await expect(page.getByText(/CSV 预检通过/)).toBeVisible();
  await expect(form.getByRole("button", { name: "登记来源并保存 RAW 版本", exact: true })).toBeDisabled();
  const response = await request.get("http://127.0.0.1:4400/__control/state", { headers });
  expect(response.ok()).toBeTruthy();
  expect((await response.json()).requests.filter(call => call.method === "POST")).toHaveLength(0);
});
