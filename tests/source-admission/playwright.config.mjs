import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: ".", testMatch: "admission.spec.mjs", workers: 1, fullyParallel: false, retries: 0,
  timeout: 60000, expect: { timeout: 20000 },
  reporter: [["list"], ["json", { outputFile: process.env.SOURCE_ADMISSION_E2E_DIR + "/browser-results.json" }]],
  use: { baseURL: "http://127.0.0.1:25482", browserName: "chromium", trace: "retain-on-failure", screenshot: "only-on-failure", extraHTTPHeaders: { "X-Authenticated-Principal": "approved-source-test", "X-Delivery-Web-Gateway": "synthetic-source-gateway" } },
});
