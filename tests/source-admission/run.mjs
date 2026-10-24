// Disposable synthetic acceptance runner. It never launches cmd/api, a worker,
// Label Studio, Docker, a real Core environment or a native storage provider.
import { spawn } from "node:child_process";
import { mkdir, readFile, access } from "node:fs/promises";
import { createWriteStream } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

if (process.env.SOURCE_ADMISSION_ACCEPTANCE !== "1") throw new Error("Set SOURCE_ADMISSION_ACCEPTANCE=1 for the isolated browser test services.");
const dsn = new URL(process.env.TEST_POSTGRES_DSN ?? "invalid:");
if (!(["127.0.0.1", "localhost", "[::1]"].includes(dsn.hostname)) || !dsn.pathname.startsWith("/hfa")) throw new Error("Dedicated loopback hfa test database required.");
const repo = fileURLToPath(new URL("../../", import.meta.url));
const platform = path.join(repo, "apps/platform");
const tests = path.join(repo, "tests/source-admission");
const output = process.env.SOURCE_ADMISSION_E2E_DIR ?? path.join(repo, ".artifacts/source-admission", crypto.randomUUID());
await mkdir(output, { recursive: true });
// An existing readiness file cannot be adopted as proof of a new owned process.
try { await access(path.join(output, "browser-fixture.json")); throw new Error("Use a new acceptance output directory."); } catch (e) { if (e.code !== "ENOENT") throw e; }
const env = { ...process.env, SOURCE_ADMISSION_E2E_DIR: output, SOURCE_ADMISSION_E2E: "1", NEXT_TELEMETRY_DISABLED: "1" };
function child(command, args, cwd, extra = {}, logfile = "runner.log") {
  const log = createWriteStream(path.join(output, logfile), { flags: "a" });
  const c = spawn(command, args, { cwd, env: { ...env, ...extra }, windowsHide: true, stdio: ["ignore", "pipe", "pipe"] });
  c.stdout.pipe(log); c.stderr.pipe(log); c.once("exit", () => log.end()); return c;
}
function done(c) { return new Promise((resolve, reject) => { c.once("error", reject); c.once("exit", (code) => code === 0 ? resolve() : reject(new Error(`Owned test process failed: ${code}`))); }); }
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const binary = path.join(output, "source-fixture.test" + (process.platform === "win32" ? ".exe" : ""));
await done(child("go", ["test", "-c", "-o", binary, "./internal/sourceadmission"], platform));
const harness = child(binary, ["-test.run=^TestBrowserFixture$", "-test.timeout=15m", "-test.v"], path.join(platform, "internal/sourceadmission"), {}, "fixture.log");
let web;
try {
  let meta;
  for (let i = 0; i < 150; i++) { try { meta = JSON.parse(await readFile(path.join(output, "browser-fixture.json"), "utf8")); break; } catch {} if (harness.exitCode !== null) throw new Error("Owned fixture exited before readiness"); await sleep(200); }
  if (!meta) throw new Error("Owned fixture did not become ready");
  web = child(process.execPath, ["node_modules/next/dist/bin/next", "start", "--hostname", "127.0.0.1", "--port", "25482"], path.join(repo, "apps/web"), { FORK_APPROVED_SOURCE_PROFILE: meta.profile, FORK_APPROVED_WEB_GATEWAY_TOKEN_FILE: meta.gatewayTokenFile, FORK_APPROVED_WEB_ORIGIN: "http://127.0.0.1:25482", PLATFORM_API_BASE_URL: meta.api }, "web.log");
  let ready = false;
  for (let i = 0; i < 150; i++) { try { const r = await fetch("http://127.0.0.1:25482/source-admissions"); if (r.ok) { ready = true; break; } } catch {} if (web.exitCode !== null) throw new Error("Owned web process exited before readiness"); await sleep(200); }
  if (!ready) throw new Error("Owned frontend did not become ready");
  await done(child(process.execPath, ["node_modules/@playwright/test/cli.js", "test"], tests, {}, "playwright.log"));
} finally {
  // Stop only the exact children created above; preserve database and evidence.
  if (web && web.exitCode === null) web.kill();
  try { await fetch("http://127.0.0.1:25481/__fixture/control", { method: "POST", headers: { "Content-Type": "application/json", "X-Fixture-Control": "synthetic-hfa-control" }, body: JSON.stringify({ action: "stop" }), signal: AbortSignal.timeout(3000) }); } catch {}
  await Promise.race([done(harness).catch(() => {}), sleep(5000)]);
  if (harness.exitCode === null) harness.kill();
}
