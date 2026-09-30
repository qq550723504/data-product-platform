import { defineConfig } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { ids } from "./fixture-server.mjs";
const webRoot = fileURLToPath(new URL("../../apps/web/", import.meta.url));
const webEnv = {
  PLATFORM_API_BASE_URL: "http://127.0.0.1:4400", POC_WORKSPACE_ID: ids.workspace,
  POC_ENABLE_INGEST_ACTIONS: "false", POC_INGEST_ACTOR_ID: "",
  HUMAN_DECISION_API_TOKEN: "review-secret", DELIVERY_API_TOKEN: "delivery-secret", DELIVERY_API_CONSUMER_REF: "GOLD-PILOT-CONSUMER", DELIVERY_API_PRINCIPAL_REF: "browser-principal", DELIVERY_WEB_GATEWAY_TOKEN: "gateway-secret", POC_RELEASE_ACTOR_ID: ids.actor, POC_EXECUTION_ACTOR_ID: ids.actor, POC_DATASET_ACTOR_ID: ids.actor, POC_QUALITY_ACTOR_ID: ids.actor, POC_COMPLIANCE_ACTOR_ID: ids.actor, NEXT_TELEMETRY_DISABLED: "1",
};
function consoleServer(port, enabled) {
  return {
    command: `node node_modules/next/dist/bin/next start --hostname 127.0.0.1 --port ${port}`,
    cwd: webRoot, url: `http://127.0.0.1:${port}/reviews`, reuseExistingServer: false, timeout: 60000,
    env: { ...webEnv, POC_ENABLE_REVIEW_ACTIONS: String(enabled), POC_ENABLE_RELEASE_ACTIONS: String(enabled), POC_ENABLE_EXECUTION_ACTIONS: String(enabled), POC_ENABLE_DATASET_ACTIONS: String(enabled), POC_ENABLE_QUALITY_ACTIONS: String(enabled), POC_ENABLE_COMPLIANCE_ACTIONS: String(enabled), POC_ENABLE_DELIVERY_ACTIONS: String(enabled) },
  };
}
export default defineConfig({
  testDir: ".", testMatch: ["actions.spec.mjs", "ingest-readonly.spec.mjs"], fullyParallel: false, workers: 1,
  forbidOnly: Boolean(process.env.CI), retries: 0, timeout: 30000,
  expect: { timeout: 10000 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: { browserName: "chromium", baseURL: "http://127.0.0.1:3100", trace: "retain-on-failure", screenshot: "only-on-failure", extraHTTPHeaders: { "X-Authenticated-Principal": "browser-principal", "X-Delivery-Web-Gateway": "gateway-secret" } },
  webServer: [
    { command: "node fixture-server.mjs", env: { BROWSER_FIXTURE: "1" }, url: "http://127.0.0.1:4400/__health", reuseExistingServer: false },
    consoleServer(3100, true), consoleServer(3101, false),
  ],
});
