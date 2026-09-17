import { expect, test } from "@playwright/test";

const productId = "55555555-5555-4555-8555-555555555555";
const releaseId = "77777777-7777-4777-8777-777777777777";

test("POC operator can review an entity, inspect readiness, publish and trace evidence", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "数据产品工作台" })).toBeVisible();
  await expect(page.getByText("企业经营活跃度", { exact: true })).toBeVisible();

  await page.goto("/reviews");
  await expect(page.getByRole("heading", { name: "实体审核" })).toBeVisible();
  await expect(page.getByText("深圳前海星河科技有限责任公司", { exact: true })).toBeVisible();
  await page.getByLabel("人工判断理由").fill("同一经营主体，名称与法人信息一致");
  await page.getByRole("button", { name: "确认匹配" }).click();
  await expect(page.getByText("审核完成", { exact: true })).toBeVisible();
  await expect(page.getByText("没有待审核候选", { exact: true })).toBeVisible();

  await page.goto(`/products/${productId}`);
  await expect(page.getByRole("heading", { name: "企业经营活跃度" })).toBeVisible();
  await expect(page.getByText("Release Readiness", { exact: true })).toBeVisible();
  await page.getByRole("link", { name: "R-2026-09", exact: true }).first().click();

  await expect(page).toHaveURL(new RegExp(`/products/${productId}/releases/${releaseId}`));
  await expect(page.getByRole("heading", { name: "发布快照与可信证据" })).toBeVisible();
  await expect(page.getByText("enterprise_activity_product", { exact: true })).toBeVisible();
  await expect(page.getByText("Release Readiness", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "发布 ProductRelease" })).toBeEnabled();
  await page.getByRole("button", { name: "发布 ProductRelease" }).click();

  await expect(page.getByText("发布完成", { exact: true })).toBeVisible();
  await expect(page.getByText("已发布", { exact: true })).toBeVisible();
  await expect(page.getByText("EvidenceSnapshot", { exact: true })).toBeVisible();
  await expect(page.getByText("Manual entity match confirmed", { exact: true })).toBeVisible();

  await page.goto("/evidence");
  await expect(page.getByRole("heading", { name: "证据中心" })).toBeVisible();
  await expect(page.getByText("R-2026-09", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "查看证据链 →" })).toBeVisible();
});
