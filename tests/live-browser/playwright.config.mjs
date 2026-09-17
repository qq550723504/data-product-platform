import { defineConfig } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { join } from "node:path";

if (process.env.LIVE_BROWSER_ACCEPTANCE !== "1") throw new Error("Run the opt-in Go live acceptance coordinator");
const phase = process.env.LIVE_BROWSER_PHASE;
if (!["review", "publish", "history", "ingest"].includes(phase)) throw new Error("Missing live browser phase");
const root = fileURLToPath(new URL("../../.artifacts/live-browser/", import.meta.url));
export default defineConfig({
  testDir: ".", testMatch: phase === "ingest" ? "ingest.spec.mjs" : "live.spec.mjs", fullyParallel: false, workers: 1, retries: 0,
  forbidOnly: true, timeout: 60000,
  expect: { timeout: 15000 },
  outputDir: join(root, `results-${phase}`),
  reporter: [["list"], ["json", { outputFile: join(root, `report-${phase}.json`) }], ["html", { outputFolder: join(root, `html-${phase}`), open: "never" }]],
  use: { browserName: "chromium", baseURL: "http://127.0.0.1:13100", trace: "on", screenshot: "only-on-failure" },
  webServer: {
    command: "node start-web.mjs", url: "http://127.0.0.1:13100/reviews", reuseExistingServer: false,
    timeout: 30000, gracefulShutdown: { signal: "SIGTERM", timeout: 3000 },
  },
});
