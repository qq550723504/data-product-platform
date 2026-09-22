import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";

const phase = process.env.LIVE_BROWSER_PHASE;
const data = JSON.parse(await readFile(process.env.LIVE_BROWSER_MANIFEST, "utf8"));
const productPath = `/products/${data.productId}`;
const tracePath = `${productPath}/releases/${data.releaseId}`;

function releaseCard(page, id) {
  return page.locator("article").filter({ has: page.locator(`input[name="releaseId"][value="${id}"]`) });
}
async function clickAction(page, button, path) {
  const response = page.waitForResponse((res) => res.request().method() === "POST" && new URL(res.url()).pathname === path);
  await button.click();
  expect((await response).status(), "real Next Server Action response").toBe(200);
}

test(`real Core browser phase: ${phase}`, async ({ page }, testInfo) => {
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("response", (response) => {
    if (response.status() >= 500) errors.push(`HTTP ${response.status()} ${response.url()}`);
  });
  if (phase === "review") {
    expect(data.candidateIds.length).toBeGreaterThan(0);
    await page.goto("/reviews");
    await expect(page.getByRole("heading", { name: "实体审核", exact: true })).toBeVisible();
    for (const id of data.candidateIds) {
      const form = page.locator("form").filter({ has: page.locator(`input[name="candidateId"][value="${id}"]`) });
      await expect(form).toBeVisible();
      const reason = form.getByLabel("审核理由（必填）");
      const confirm = form.getByRole("button", { name: "确认匹配", exact: true });
      await expect(confirm).toBeDisabled();
      await reason.fill("   ");
      await expect(confirm).toBeDisabled();
      await reason.fill(data.reason);
      await clickAction(page, confirm, "/reviews");
      await expect(form).toHaveCount(0);
    }
    await expect(page.getByText("当前页没有待审核候选", { exact: true })).toBeVisible();
    await page.reload();
    await expect(page.getByText("当前页没有待审核候选", { exact: true })).toBeVisible();
  } else if (phase === "publish") {
    await page.goto(productPath);
    const blocked = releaseCard(page, data.blockedReleaseId);
    await expect(blocked.getByRole("button", { name: "发布 Release", exact: true })).toBeDisabled();
    const card = releaseCard(page, data.releaseId);
    const gates = card.locator(".metric-grid .metric-card");
    await expect(gates).toHaveCount(8);
    for (let index = 0; index < 8; index++) await expect(gates.nth(index)).toContainText("PASS");
    await expect(card.getByText("所有 Readiness Gate 已通过", { exact: true })).toBeVisible();
    await clickAction(page, card.getByRole("button", { name: "发布 Release", exact: true }), productPath);
    await expect(card.getByRole("button", { name: "已发布", exact: true })).toBeDisabled();
    await card.getByRole("link", { name: /查看 Release/ }).click();
    await expect(page).toHaveURL(new RegExp(`${tracePath}$`));
    await expect(page.getByRole("heading", { name: "ProductRelease 证据链", exact: true })).toBeVisible();
    await expect(page.getByText(data.reason, { exact: true }).first()).toBeVisible();
    await expect(page.getByText("PRODUCT_RELEASE_PUBLISHED", { exact: true })).toBeVisible();
  } else if (phase === "certified") {
    expect(data.deliveredSha256).toBe(data.datasetChecksum);
    const query = new URLSearchParams({
      profileId: data.certificationProfileId,
      consumer: "LICENSED_BANK",
      purpose: "REGULATORY_REPORTING",
      action: "READ",
      delivery: "DIRECT_DATA",
      scopeType: "ALL_RESOURCE",
      scopeRef: data.outputVersionId,
    });
    const versionPath = `/datasets/${data.datasetId}/versions/${data.outputVersionId}?${query.toString()}`;
    await page.goto(versionPath);
    await expect(page.getByRole("heading", { name: "DatasetVersion 质量与认证", exact: true })).toBeVisible();
    await expect(page.getByText(data.outputVersionId, { exact: true })).toBeVisible();
    await expect(page.getByText(data.datasetChecksum, { exact: true })).toBeVisible();

    await expect(page.getByRole("heading", { name: "Quality Assessment", exact: true })).toBeVisible();
    await expect(page.getByText(data.qualityAssessmentId, { exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Quality Report", exact: true })).toBeVisible();
    for (const dimension of data.qualityDimensions) {
      await expect(page.getByText(dimension, { exact: true }).first()).toBeVisible();
    }

    await expect(page.getByRole("heading", { name: "Certification 历史", exact: true })).toBeVisible();
    await expect(page.getByText(data.certificationProfileName, { exact: true })).toBeVisible();
    await expect(page.getByRole("heading", { name: "Rights summary", exact: true })).toBeVisible();
    await expect(page.getByText(data.effectiveRightsSnapshotId, { exact: true })).toBeVisible();
    await expect(page.getByText(data.rightsSnapshotId, { exact: true })).toBeVisible();

    await expect(page.getByRole("heading", { name: "Evidence", exact: true })).toBeVisible();
    await expect(page.getByText(data.certificationEvidenceSnapshotId, { exact: true })).toBeVisible();
    await expect(page.getByText(data.certificationId, { exact: true })).toBeVisible();

    await expect(page.getByRole("heading", { name: "Current Delivery Eligibility", exact: true })).toBeVisible();
    const eligibility = page.locator("section.detail-card").filter({
      has: page.getByRole("heading", { name: "预检结果", exact: true }),
    });
    await expect(eligibility).toBeVisible();
    await expect(eligibility.getByText("DatasetVersion usability", { exact: true })).toBeVisible();
    await expect(eligibility.getByText("Current Certification", { exact: true })).toBeVisible();
    await expect(eligibility.getByText("Current Entitlement", { exact: true })).toBeVisible();
    await expect(eligibility.getByText("ALLOWED", { exact: true })).toHaveCount(4);
  } else if (phase === "history") {
    // New browser context + fresh standalone process, reading persisted history.
    await page.goto("/evidence");
    await page.locator(`a[href="${tracePath}"]`).click();
    await expect(page.getByText(data.snapshotId, { exact: true })).toBeVisible();
    await expect(page.getByText(data.rootHash, { exact: true })).toBeVisible();
    await expect(page.getByText("INTEGRITY_OK", { exact: true })).toBeVisible();
    for (const name of ["DatasetVersion Lineage", "Entity Resolution", "Evidence Records", "Cost Ledger", "Audit Trail"]) {
      await expect(page.getByRole("heading", { name, exact: true })).toBeVisible();
    }
    await expect(page.getByText(data.reason, { exact: true }).first()).toBeVisible();
    // The Release must show the immutable decision that produced it...
    await expect(page.getByText(data.decisionId, { exact: true })).toBeVisible();
    // ...not the post-release correction that only moved the current mapping.
    await expect(page.getByText(data.currentMappingReason, { exact: true })).toHaveCount(0);
    await page.goto(productPath);
    await expect(releaseCard(page, data.releaseId).getByRole("button", { name: "已发布", exact: true })).toBeDisabled();
    await expect(releaseCard(page, data.blockedReleaseId).getByRole("button", { name: "发布 Release", exact: true })).toBeDisabled();
  } else {
    throw new Error("Unknown live test phase");
  }
  expect(errors, "browser JavaScript/HTTP failures").toEqual([]);
  await testInfo.attach(`live-${phase}-screen`, { body: await page.screenshot({ fullPage: true }), contentType: "image/png" });
});
