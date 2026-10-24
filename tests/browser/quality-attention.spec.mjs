import { test, expect } from "@playwright/test";
import { ids, fixtureToken, attentionCaseId } from "./fixture-server.mjs";

const control = "http://127.0.0.1:4400/__control";
const headers = { "x-fixture-token": fixtureToken };
const versionPath = `/datasets/${ids.goldDataset}/versions/${ids.goldVersion}`;
async function reset(request, scenario = "quality-attention", readFaults = {}) {
  expect((await request.post(`${control}/reset`, { headers, data: { scenario, readFaults } })).ok()).toBeTruthy();
}
async function calls(request) {
  const response = await request.get(`${control}/state`, { headers });
  expect(response.ok()).toBeTruthy();
  return (await response.json()).requests;
}
test.beforeEach(async ({ request }) => reset(request));

for (const [fault, offset] of [["quality-history", 0], ["quality-latest-summary", 25], ["quality-attempt", 0], ["quality-latest-report", 0]]) {
  test(`${fault} failure preserves successful exact historical evidence`, async ({ page, request }) => {
    await reset(request, "quality-attention", { [fault]: "fail" });
    const attempt = "abababab-abab-4bab-8bab-abababababab";
    const suffix = fault === "quality-attempt" ? `&qualityAttemptId=${attempt}` : "";
    await page.goto(`${versionPath}?view=quality&assessmentId=${ids.goldAssessment}&assessmentOffset=${offset}${suffix}`);
    await expect(page.getByTestId("quality-report")).toContainText(ids.goldAssessment);
    await expect(page.getByTestId("selected-quality-assessment")).toContainText(ids.goldAssessment);
    await expect(page.getByTestId("quality-evidence-unavailable")).toHaveCount(0);
    if (fault === "quality-history") {
      await expect(page.getByTestId("quality-history-unavailable")).toContainText("UNKNOWN");
      await expect(page.getByRole("heading", { name: "最新评测", exact: true })).toBeVisible();
    }
    if (fault === "quality-latest-summary") await expect(page.getByTestId("quality-summary-unavailable")).toContainText("UNKNOWN");
    if (fault === "quality-attempt") {
      await expect(page.getByTestId("quality-attempt-unavailable")).toContainText(attempt);
      await expect(page.getByRole("form", { name: "运行 Quality Check" })).toHaveCount(0);
    }
    const reads = await calls(request);
    expect(reads.filter(c => c.path.endsWith(`/quality-assessments/${ids.qualityAssessment}/report`))).toHaveLength(0);
    expect(reads.filter(c => c.path.endsWith(`/quality-assessments/${ids.goldAssessment}/report`))).toHaveLength(1);
    expect(reads.every(c => c.method === "GET")).toBe(true);
    expect(await page.content()).not.toContain("private-upstream-sentinel");
  });
}

for (const mode of ["fail", "denied", "notfound", "invalid"]) {
  test(`selected historical report ${mode} stays UNKNOWN without a latest substitute`, async ({ page, request }) => {
    await reset(request, "quality-attention", { "quality-original-report": mode });
    await page.goto(`${versionPath}?view=quality&assessmentId=${ids.goldAssessment}`);
    await expect(page.getByTestId("quality-evidence-unavailable")).toContainText(ids.goldAssessment);
    await expect(page.getByTestId("quality-report")).toHaveCount(0);
    await expect(page.getByRole("heading", { name: "最新评测", exact: true })).toBeVisible();
    await expect(page.getByTestId("quality-history-unavailable")).toHaveCount(0);
    expect((await calls(request)).some(c => c.path.endsWith(`/quality-assessments/${ids.qualityAssessment}/report`))).toBe(false);
    expect((await calls(request)).every(c => c.method === "GET")).toBe(true);
    await reset(request);
    await page.reload();
    await expect(page.getByTestId("quality-report")).toContainText(ids.goldAssessment);
    await expect(page).toHaveURL(new RegExp(`assessmentId=${ids.goldAssessment}`));
  });
}

test("default latest report failure preserves the known latest summary and history", async ({ page, request }) => {
  await reset(request, "quality-attention", { "quality-latest-report": "denied" });
  await page.goto(`${versionPath}?view=quality`);
  await expect(page.getByTestId("quality-evidence-unavailable")).toContainText(ids.qualityAssessment);
  await expect(page.getByRole("heading", { name: "最新评测", exact: true })).toBeVisible();
  await expect(page.getByTestId("quality-summary-unavailable")).toHaveCount(0);
  await expect(page.getByTestId("quality-history-unavailable")).toHaveCount(0);
  await expect(page.getByTestId("quality-report")).toHaveCount(0);
});

test("Quality credentials stay server-side and denied evidence remains local UNKNOWN with the exact historical link", async ({ page, request }) => {
  await page.goto(`${versionPath}?view=quality&assessmentId=${ids.goldAssessment}`);
  await expect(page.getByTestId("quality-report")).toContainText(ids.goldAssessment);
  const qualityReads = (await calls(request)).filter(c => c.method === "GET" && /\/quality-(assessments|assessment-attempts)/.test(c.path));
  expect(qualityReads.length).toBeGreaterThan(0);
  expect(qualityReads.every(c => c.authorization === "Bearer review-secret")).toBe(true);
  expect(await page.content()).not.toContain("review-secret");
  await reset(request, "quality-evidence-denied");
  await page.reload();
  await expect(page.getByTestId("quality-evidence-unavailable")).toContainText("UNKNOWN");
  await expect(page.getByTestId("quality-evidence-unavailable")).toContainText(ids.goldAssessment);
  await expect(page.getByRole("heading", {name:"版本事实",exact:true})).toBeVisible();
  await expect(page.getByTestId("quality-report")).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`assessmentId=${ids.goldAssessment}`));
  expect((await calls(request)).every(c => c.method === "GET")).toBe(true);
  expect((await calls(request)).filter(c => /\/quality-/.test(c.path)).every(c => c.authorization === "Bearer review-secret")).toBe(true);
});

test("quality attention pages past 100 without fetching case details, then focuses one case read-only", async ({ page, request }, testInfo) => {
  await page.goto("/attention?qualityView=ALL&qualityOffset=100");
  const panel = page.getByTestId("quality-attention");
  await expect(panel.locator("tbody tr")).toHaveCount(5);
  await expect(panel.getByRole("navigation", { name: "质量异常分页" })).toContainText("101–105 / 105");
  const row = panel.locator("tbody tr").filter({ hasText: "负数用电异常 105" });
  await expect(panel.locator("tbody tr").filter({ hasText: "负数用电异常 101" }).getByRole("link", { name: "FAILED", exact: true })).toBeVisible();
  await expect(panel.locator("tbody tr").filter({ hasText: "负数用电异常 102" })).toContainText("FAILED 尚无质量结论");
  await expect(row).toContainText("已人工关闭（不代表合格）");
  await expect(row).toContainText("存在原复检待核对");
  const initial = await calls(request);
  expect(initial.filter(c => c.path.endsWith("/quality-remediations/attention"))).toHaveLength(1);
  expect(initial.filter(c => /quality-remediations\/[0-9a-f-]+$/.test(c.path))).toHaveLength(0);
  expect(initial.some(c => c.method !== "GET")).toBe(false);
  expect(await page.content()).not.toContain("review-secret");
  await page.screenshot({ path: testInfo.outputPath("quality-attention.png"), fullPage: true });

  await row.getByRole("link", { name: "查看异常", exact: true }).click();
  await expect(page).toHaveURL(new RegExp(`caseId=${attentionCaseId(105)}$`));
  await expect(page.getByRole("heading", { name: `异常 ${attentionCaseId(105)}`, exact: true })).toBeVisible();
  await expect(page.getByText("当前为单个异常的只读导航。", { exact: false })).toBeVisible();
  await expect(page.locator("form")).toHaveCount(0);
  await expect(page.getByRole("link", { name: "查看原评测、Finding 与证据" })).toHaveAttribute("href", `${versionPath}?view=quality&assessmentId=${ids.goldAssessment}`);
  const after = await calls(request);
  expect(after.filter(c => c.path.endsWith(`/quality-remediations/${attentionCaseId(105)}`))).toHaveLength(1);
  expect(after.some(c => c.path.endsWith("/quality-remediations") || c.path.endsWith("/batches"))).toBe(false);
  expect(after.some(c => c.method !== "GET")).toBe(false);
});

test("original evidence opens the exact historical assessment and preserves latest quality", async ({ page, request }) => {
  await page.goto("/attention?qualityView=ALL&qualityOffset=100");
  await page.getByTestId("quality-attention").getByRole("link", { name: "原评测与证据", exact: true }).last().click();
  await expect(page.getByTestId("selected-quality-assessment")).toContainText(ids.goldAssessment);
  const report = page.getByTestId("quality-report");
  await expect(report).toContainText(ids.goldAssessment);
  await expect(report).toContainText("GOLD_QUALITY_ASSESSMENT");
  const latest = page.locator("section").filter({ has: page.getByRole("heading", { name: "最新评测", exact: true }) });
  await expect(latest).toContainText(ids.qualityAssessment);
  expect((await calls(request)).some(c => c.method !== "GET")).toBe(false);
});

test("workbench enters follow-up view and recheck evidence opens its output version", async ({ page, request }) => {
  const speculativeReads = [];
  page.on("request", request => {
    const url = new URL(request.url());
    if (url.pathname === "/attention" && url.searchParams.has("qualityView")
      && [...url.searchParams.keys()].every(key => key === "qualityView" || key === "_rsc")
      && request.headers()["next-router-prefetch"]) speculativeReads.push(request.url());
  });
  await page.goto("/core/workbench");
  const compact = page.getByTestId("quality-attention");
  await expect(compact).toContainText("需跟进 104 条");
  await compact.getByRole("link", { name: "查看质量异常" }).click();
  await expect(page).toHaveURL(/qualityView=FOLLOW_UP#quality-attention$/);
  const filters = page.getByRole("navigation", { name: "质量异常视图" });
  const panel = page.getByTestId("quality-attention");
  for (const [view, label, count] of [["CLOSED", "已人工关闭", 2], ["ALL", "全部", 25], ["CLOSED", "已人工关闭", 2]]) {
    await filters.getByRole("link", { name: label, exact: true }).click();
    await expect(page).toHaveURL(new RegExp(`qualityView=${view}#quality-attention$`));
    await expect(filters.getByRole("link", { name: label, exact: true })).toHaveAttribute("aria-current", "page");
    await expect(panel.locator("tbody tr")).toHaveCount(count);
  }
  const link = panel.getByRole("link", { name: "本次复检与证据", exact: true });
  await expect(link).toHaveAttribute("href", `/datasets/${ids.goldDataset}/versions/${ids.qualityOutputVersion}?view=quality&assessmentId=${ids.recheckAssessment}`);
  await link.click();
  await expect(page.getByTestId("quality-report")).toContainText(ids.recheckAssessment);
  await expect(page.getByTestId("selected-quality-assessment")).toContainText(ids.recheckAssessment);
  expect(speculativeReads).toHaveLength(0);
  expect((await calls(request)).some(c => c.method !== "GET")).toBe(false);
});

test("failed quality read stays UNKNOWN while existing attention remains visible", async ({ page, request }) => {
  await reset(request, "quality-attention-unavailable");
  for (const path of ["/core/workbench", "/attention"]) {
    await page.goto(path);
    await expect(page.getByTestId("quality-attention-unavailable")).toContainText("UNKNOWN");
    await expect(page.getByText("Release 待发布", { exact: true })).toBeVisible();
    await expect(page.getByRole("link", { name: "去发布", exact: true })).toBeVisible();
  }
  expect((await calls(request)).some(c => c.method !== "GET")).toBe(false);
});

test("foreign case or assessment scope fails without a latest-result fallback", async ({ page, request }) => {
  await reset(request, "quality-attention-foreign");
  await page.goto(`${versionPath}/remediation?caseId=${attentionCaseId(105)}`);
  await expect(page.getByText("异常不属于所选版本或 workspace", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: `异常 ${attentionCaseId(105)}`, exact: true })).toHaveCount(0);
  await page.goto(`${versionPath}?view=quality&assessmentId=${ids.goldAssessment}`);
  await expect(page.getByTestId("quality-evidence-unavailable")).toContainText("UNKNOWN");
  await expect(page.getByTestId("quality-report")).toHaveCount(0);
  expect((await calls(request)).some(c => c.method !== "GET")).toBe(false);
});
