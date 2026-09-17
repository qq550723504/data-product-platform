import { defineConfig } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { ids } from "./fixture-server.mjs";
const webRoot = fileURLToPath(new URL("../../apps/web/", import.meta.url));
const webEnv = {
  PLATFORM_API_BASE_URL: "http://127.0.0.1:4400", POC_WORKSPACE_ID: ids.workspace,
  POC_REVIEWER_ID: ids.actor, POC_RELEASE_ACTOR_ID: ids.actor, NEXT_TELEMETRY_DISABLED: "1",
};
function consoleServer(port, enabled) {
  return {
    command: `node node_modules/next/dist/bin/next start --hostname 127.0.0.1 --port ${port}`,
    cwd: webRoot, url: `http://127.0.0.1:${port}/reviews`, reuseExistingServer: false, timeout: 60000,
    env: { ...webEnv, POC_ENABLE_REVIEW_ACTIONS: String(enabled), POC_ENABLE_RELEASE_ACTIONS: String(enabled) },
  };
}
export default defineConfig({
  testDir: ".", testMatch: "actions.spec.mjs", fullyParallel: false, workers: 1,
  forbidOnly: Boolean(process.env.CI), retries: 0, timeout: 30000,
  expect: { timeout: 10000 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: { browserName: "chromium", baseURL: "http://127.0.0.1:3100", trace: "retain-on-failure", screenshot: "only-on-failure" },
  webServer: [
    { command: "node fixture-server.mjs", env: { BROWSER_FIXTURE: "1" }, url: "http://127.0.0.1:4400/__health", reuseExistingServer: false },
    consoleServer(3100, true), consoleServer(3101, false),
  ],
});
