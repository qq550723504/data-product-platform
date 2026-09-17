/** CI-only lifecycle acceptance of the shipped local demo, using synthetic records. */
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdirSync, writeFileSync, realpathSync } from "node:fs";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import { projectName } from "./demo.mjs";
const require = createRequire(new URL("../../tests/live-browser/package.json", import.meta.url));
const { chromium, expect } = require("@playwright/test");
const root = realpathSync(fileURLToPath(new URL("../../", import.meta.url)));
const artifacts = `${root}/.artifacts/demo`;
mkdirSync(artifacts, { recursive: true });
writeFileSync(`${artifacts}/verification.json`, JSON.stringify({ verified: false, status: "started" }));
function cli(args, success = true) {
  const result = spawnSync(process.execPath, ["deploy/demo/demo.mjs", ...args], { cwd: root, encoding: "utf8", timeout: 20 * 60 * 1000, maxBuffer: 64 * 1024 * 1024 });
  if (result.error) throw result.error;
  writeFileSync(`${artifacts}/${args[0]}-${Date.now()}.log`, (result.stdout || "") + (result.stderr || ""));
  if (success) assert.equal(result.status, 0, `${args.join(" ")}: ${result.stderr}`);
  else assert.notEqual(result.status, 0, "unsafe/premature command unexpectedly succeeded");
  return result.stdout.trim();
}
const status = () => JSON.parse(cli(["status"]));
const initial = status();
assert.equal(initial.jobStatus, "WAITING_REVIEW");
cli(["advance"], false);
cli(["reset"], false);
assert.deepEqual(status(), initial, "premature advance or unconfirmed reset changed the demo");
cli(["up"]);
assert.deepEqual(status(), initial, "second up recreated data or identity");

let browser, context, page;
try {
  browser = await chromium.launch();
  context = await browser.newContext();
  await context.tracing.start({ screenshots: true, snapshots: true });
  page = await context.newPage();
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(initial.reviewURL);
  await expect(page.getByRole("heading", { name: "实体审核", exact: true })).toBeVisible();
  await page.screenshot({ path: `${artifacts}/01-review.png`, fullPage: true });
  const reason = `DEMO_BROWSER_REVIEW_${initial.manifest.workspaceId}`;
  const forms = page.getByRole("form", { name: /^审核 / });
  let reviews = 0;
  while (await forms.count()) {
    assert.ok(reviews < 20, "unexpected review loop");
    const before = await forms.count();
    const form = forms.first();
    await expect(form.getByRole("button", { name: "确认匹配" })).toBeDisabled();
    await form.getByLabel("审核理由（必填）").fill(reason);
    await form.getByRole("button", { name: "确认匹配" }).click();
    await expect(forms).toHaveCount(before - 1);
    reviews++;
  }
  assert.ok(reviews > 0);
  cli(["advance"]);
  const ready = status();
  assert.equal(ready.releaseStatus, "READY");
  cli(["advance"]);
  assert.deepEqual(status(), ready, "second advance duplicated production/release");
  cli(["verify"], false);
  await page.goto(ready.productURL);
  const blocked = page.locator("article").filter({ hasText: "Release R-DEMO-BLOCKED" });
  const release = page.locator("article").filter({ hasText: "Release R-DEMO-READY" });
  await expect(blocked.getByRole("button", { name: "发布 Release", exact: true })).toBeDisabled();
  await expect(release.getByRole("button", { name: "发布 Release", exact: true })).toBeEnabled();
  await page.screenshot({ path: `${artifacts}/02-ready-and-blocked.png`, fullPage: true });
  await release.getByRole("button", { name: "发布 Release", exact: true }).click();
  await expect(release.getByRole("button", { name: "已发布", exact: true })).toBeDisabled();
  const verified = JSON.parse(cli(["verify"]));
  assert.equal(verified.verified, true);
  assert.equal(verified.checksummedVersions, 5);
  assert.match(verified.rootHash, /^[0-9a-f]{64}$/);
  const published = status();
  assert.equal(published.releaseStatus, "PUBLISHED");
  await context.tracing.stop({ path: `${artifacts}/review-publish-trace.zip` });
  await context.close();
  await browser.close();
  browser = null;

  cli(["down"]);
  cli(["up"]);
  assert.deepEqual(status(), published, "down/up changed persisted business state");
  assert.deepEqual(JSON.parse(cli(["verify"])), verified, "snapshot/hash/record counts changed after restart");
  browser = await chromium.launch();
  context = await browser.newContext();
  await context.tracing.start({ screenshots: true, snapshots: true });
  page = await context.newPage();
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(published.traceURL);
  await expect(page.getByRole("heading", { name: "ProductRelease 证据链", exact: true })).toBeVisible();
  await expect(page.getByText(reason, { exact: true }).first()).toBeVisible();
  await expect(page.getByText(verified.rootHash, { exact: true }).first()).toBeVisible();
  await page.screenshot({ path: `${artifacts}/03-restarted-trace.png`, fullPage: true });
  await context.tracing.stop({ path: `${artifacts}/restart-trace.zip` });
  await context.close();
  assert.deepEqual(errors, [], "uncaught browser runtime errors");

  const ids = spawnSync("docker", ["ps", "-q", "--filter", `label=com.docker.compose.project=${projectName(root)}`], { encoding: "utf8" });
  assert.equal(ids.status, 0);
  const inspect = spawnSync("docker", ["inspect", ...ids.stdout.trim().split(/\s+/)], { encoding: "utf8" });
  assert.equal(inspect.status, 0);
  const bindings = JSON.parse(inspect.stdout).flatMap((item) => Object.values(item.NetworkSettings.Ports || {}).flat().filter(Boolean));
  assert.deepEqual(bindings, [{ HostIp: "127.0.0.1", HostPort: "3180" }], "unexpected host-exposed service port");
  writeFileSync(`${artifacts}/verification.json`, JSON.stringify({ ...verified, reviewedCandidates: reviews, duplicateUpUnchanged: true, duplicateAdvanceUnchanged: true, downUpPreservedHistory: true, unconfirmedResetRejected: true, onlyLoopbackConsolePublished: true, counts: published.counts }, null, 2));
  console.log("DEMO_LIFECYCLE_VERIFIED", JSON.stringify(verified));
} catch (error) {
  if (page && !page.isClosed()) await page.screenshot({ path: `${artifacts}/failure.png`, fullPage: true }).catch(() => {});
  if (context) await context.tracing.stop({ path: `${artifacts}/failure-trace.zip` }).catch(() => {});
  throw error;
} finally {
  if (browser) await browser.close();
}
