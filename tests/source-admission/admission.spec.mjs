import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
const root = process.env.SOURCE_ADMISSION_E2E_DIR;
const meta = JSON.parse(await readFile(root + "/browser-fixture.json", "utf8"));
async function control(request, action) { const r = await request.post(meta.api + "/__fixture/control", { headers: { "X-Fixture-Control": "synthetic-hfa-control" }, data: { action } }); expect(r.status()).toBe(204); }
const text = {
  "en-US": { title: "Accept annotation source", submit: "Accept / resume original request", lookup: "Check original request", accepted: "Source accepted", denied: "Current access denied.", unknown: "The result is unknown.", publicationConflict: "A newer dataset version was accepted first." },
  "zh-CN": { title: "接纳标注来源", submit: "接纳 / 继续原请求", lookup: "查询原请求", accepted: "来源已接纳", denied: "当前权限不足。", unknown: "结果尚未确认。", publicationConflict: "已有更新的数据版本完成接纳。" },
};
async function fill(page, manifestSha256 = meta.manifestSha256) { await page.locator('input[name="releaseId"]').fill(meta.releaseId); await page.locator('input[name="snapshotId"]').fill(meta.snapshotId); await page.locator('input[name="manifestSha256"]').fill(manifestSha256); }
async function budgetState(request) { const r = await request.get(meta.api + "/__fixture/budget-state", { headers: { "X-Fixture-Control": "synthetic-hfa-control" } }); expect(r.status()).toBe(200); return r.json(); }
for (const lang of ["en-US", "zh-CN"]) {
  test(`${lang}: terminal publication conflict UI and lost-response lookup`, async ({ page }) => {
    const t = text[lang];
    // UI contract only; the actual out-of-order PostgreSQL race is exercised
    // in TestPostgresOutOfOrderSnapshotPublicationTerminatesOriginalKey.
    await page.route("**/api/source-admissions*", async route => {
      const post = route.request().method() === "POST";
      await route.fulfill({ status: post ? 409 : 200, contentType: "application/json", body: JSON.stringify(post ? { error: { code: "SOURCE_PUBLICATION_CONFLICT" } } : { state: "FAILED_FINAL", errorCode: "SOURCE_PUBLICATION_CONFLICT" }) });
    });
    await page.goto(`/source-admissions?lang=${lang}`); await fill(page);
    await page.getByRole("button", { name: t.submit }).click();
    await expect(page.getByRole("status")).toContainText(t.publicationConflict);
    await expect(page.getByRole("button", { name: t.submit })).toBeDisabled();
    const key = await page.getByTestId("request-key").textContent();
    await page.reload(); await page.getByRole("button", { name: t.lookup }).click();
    await expect(page.getByRole("status")).toContainText(t.publicationConflict);
    await expect(page.getByTestId("request-key")).toHaveText(key);
    await expect(page.getByRole("button", { name: t.submit })).toBeDisabled();
    await expect(page.getByTestId("accepted-version")).toHaveCount(0);
  });
  test(`${lang}: real acceptance, replay, reload and stale access`, async ({ page, request }) => {
    const t = text[lang]; await control(request, "enable");
    await page.goto(`/source-admissions?lang=${lang}`); await expect(page.getByRole("heading", { name: t.title })).toBeVisible(); await fill(page);
    await page.getByRole("button", { name: t.submit }).click(); await expect(page.getByRole("status")).toHaveText(t.accepted);
    const version = await page.getByTestId("accepted-version").textContent(); const key = await page.getByTestId("request-key").textContent(); expect(version).toMatch(/^[0-9a-f-]{36}$/);
    await page.getByRole("button", { name: t.submit }).click(); await expect(page.getByTestId("accepted-version")).toHaveText(version);
    await page.reload(); await expect(page.getByTestId("request-key")).toHaveText(key); await page.getByRole("button", { name: t.lookup }).click(); await expect(page.getByTestId("accepted-version")).toHaveText(version);
    await control(request, "deny"); await page.getByRole("button", { name: t.lookup }).click(); await expect(page.getByRole("status")).toContainText(t.denied); await expect(page.getByTestId("accepted-version")).toHaveCount(0); await expect(page.getByRole("button", { name: t.submit })).toBeDisabled();
    await page.screenshot({ path: `${root}/${lang}-stale-access.png`, fullPage: true }); await control(request, "enable");
  });
  test(`${lang}: committed response loss resolves original key`, async ({ page, request }) => {
    const t = text[lang]; await control(request, "enable"); await page.goto(`/source-admissions?lang=${lang}`); await fill(page);
    await page.route("**/api/source-admissions", async route => { const response = await route.fetch(); expect(response.status()).toBe(200); await route.abort("failed"); }, { times: 1 });
    await page.getByRole("button", { name: t.submit }).click(); await expect(page.getByRole("status")).toContainText(t.unknown);
    const key = await page.getByTestId("request-key").textContent(); await page.getByRole("button", { name: t.lookup }).click(); await expect(page.getByRole("status")).toHaveText(t.accepted); await expect(page.getByTestId("request-key")).toHaveText(key);
    await page.screenshot({ path: `${root}/${lang}-accepted-source.png`, fullPage: true });
    const state = await request.get(meta.api + "/__fixture/state", { headers: { "X-Fixture-Control": "synthetic-hfa-control" } }); expect(await state.json()).toMatchObject({ versions: 1, accepted: 1 });
  });
  test(`${lang}: web caller cannot forge actor or bypass gateway`, async ({ page, request }) => {
    await control(request, "enable");
    const missing = await request.post("/api/source-admissions", { headers: { "X-Delivery-Web-Gateway": "wrong", Origin: "http://127.0.0.1:25482", "Idempotency-Key": crypto.randomUUID() }, data: { releaseId: meta.releaseId, snapshotId: meta.snapshotId, manifestSha256: meta.manifestSha256 } }); expect(missing.status()).toBe(403);
    const forged = await request.post("/api/source-admissions", { headers: { Origin: "http://127.0.0.1:25482", "Idempotency-Key": crypto.randomUUID() }, data: { releaseId: meta.releaseId, snapshotId: meta.snapshotId, manifestSha256: meta.manifestSha256, actorId: "forged" } }); expect(forged.status()).toBe(400);
    const csrf = await request.post("/api/source-admissions", { headers: { Origin: "https://foreign.invalid", "Idempotency-Key": crypto.randomUUID() }, data: {} }); expect(csrf.status()).toBe(403);
    await page.goto(`/source-admissions?lang=${lang}`); await expect(page.locator("section[lang='" + lang + "']")).toBeVisible();
  });
  test(`${lang}: real budget rejection, lost response and original-key refresh`, async ({ page, request }) => {
    const t = text[lang];
    const fresh = lang === "zh-CN" ? "新建请求" : "New request";
    const unsupported = lang === "zh-CN" ? "此来源不符合首版纯文本 JSONL 契约。" : "This source does not meet the first-version text JSONL contract.";
    const failed = lang === "zh-CN" ? "原请求已被拒绝；历史保留，请核对来源后使用新的快照。" : "The original request was rejected. History is retained; check the source and use a new snapshot.";
    await control(request, "enable"); await page.goto(`/source-admissions?lang=${lang}`); await fill(page);
    await page.getByRole("button", { name: t.submit }).click(); await expect(page.getByTestId("accepted-version")).toBeVisible();
    await page.getByRole("button", { name: fresh, exact: true }).click(); await expect(page.getByTestId("accepted-version")).toHaveCount(0);
    await control(request, "budget"); await fill(page, meta.budgetManifestSha256);
    const before = await budgetState(request);
    const post = page.waitForResponse(r => r.url().endsWith("/api/source-admissions") && r.request().method() === "POST");
    await page.getByRole("button", { name: t.submit }).click(); expect((await post).status()).toBe(422);
    await expect(page.getByRole("status")).toHaveText(unsupported); await expect(page.getByTestId("accepted-version")).toHaveCount(0);
    const key = await page.getByTestId("request-key").textContent();
    // Terminal replay performs no extra source work; deliberately lose this
    // real backend response, then recover the same terminal operation by key.
    await page.route("**/api/source-admissions", async route => { const r = await route.fetch(); expect(r.status()).toBe(422); await route.abort("failed"); }, { times: 1 });
    await page.getByRole("button", { name: t.submit }).click(); await expect(page.getByRole("status")).toContainText(t.unknown);
    await page.reload(); await expect(page.getByTestId("request-key")).toHaveText(key);
    await page.getByRole("button", { name: t.lookup }).click(); await expect(page.getByRole("status")).toHaveText(failed);
    await expect(page.getByRole("button", { name: t.submit })).toBeDisabled(); await expect(page.getByTestId("accepted-version")).toHaveCount(0);
    const after = await budgetState(request);
    expect(after).toEqual({ ...before, calls: before.calls + 1, costs: before.costs + 1, received: before.received + 1, rejected: before.rejected + 1 });
    await page.screenshot({ path: `${root}/${lang}-budget-original-key.png`, fullPage: true });
    await control(request, "deny"); await page.getByRole("button", { name: t.lookup }).click(); await expect(page.getByRole("status")).toContainText(t.denied);
    expect(await budgetState(request)).toEqual(after); await control(request, "enable");
  });
}
