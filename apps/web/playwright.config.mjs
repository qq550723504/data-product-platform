import { defineConfig } from "@playwright/test";

const workspaceId = "11111111-1111-4111-8111-111111111111";
const actorId = "22222222-2222-4222-8222-222222222222";
const core = "http://127.0.0.1:18080";

export default defineConfig({
  testDir: "./tests/browser",
  testMatch: /poc-actions\.spec\.mjs/,
  workers: 1,
  fullyParallel: false,
  retries: 0,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  reporter: [["line"], ["html", { outputFolder: "playwright-report", open: "never" }]],
  outputDir: "test-results/playwright",
  use: {
    baseURL: "http://127.0.0.1:3001",
    browserName: "chromium",
    headless: true,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  webServer: [
    {
      command: "node tests/browser/fixture-core.mjs",
      url: `${core}/__test/state`,
      reuseExistingServer: false,
      timeout: 30_000,
    },
    {
      command: "npm start -- --hostname 127.0.0.1 --port 3001",
      url: "http://127.0.0.1:3001/reviews",
      reuseExistingServer: false,
      timeout: 30_000,
      env: {
        PLATFORM_API_BASE_URL: core,
        POC_WORKSPACE_ID: workspaceId,
        POC_ENABLE_REVIEW_ACTIONS: "true",
        POC_REVIEWER_ID: actorId,
        POC_ENABLE_RELEASE_ACTIONS: "true",
        POC_RELEASE_ACTOR_ID: actorId,
      },
    },
    {
      command: "npm start -- --hostname 127.0.0.1 --port 3002",
      url: "http://127.0.0.1:3002/reviews",
      reuseExistingServer: false,
      timeout: 30_000,
      env: {
        PLATFORM_API_BASE_URL: core,
        POC_WORKSPACE_ID: workspaceId,
        POC_ENABLE_REVIEW_ACTIONS: "false",
        POC_ENABLE_RELEASE_ACTIONS: "false",
      },
    },
  ],
});
