import { readFile } from "node:fs/promises";
const manifest = JSON.parse(await readFile("/demo-state/manifest.json", "utf8"));
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
if (manifest.schema !== 1 || !["REVIEW", "READY"].includes(manifest.stage) ||
    ![manifest.workspaceId, manifest.reviewerId, manifest.publisherId, manifest.seedActorId].every((value) => typeof value === "string" && uuid.test(value) && value !== "00000000-0000-0000-0000-000000000000")) {
  throw new Error("Initialize the dedicated demo before starting its console.");
}
Object.assign(process.env, {
  PLATFORM_API_BASE_URL: "http://api:8080", POC_WORKSPACE_ID: manifest.workspaceId,
  POC_INGEST_ACTOR_ID: manifest.seedActorId, POC_ENABLE_INGEST_ACTIONS: "true",
  POC_REVIEWER_ID: manifest.reviewerId, POC_RELEASE_ACTOR_ID: manifest.publisherId,
  POC_ENABLE_REVIEW_ACTIONS: "true", POC_ENABLE_RELEASE_ACTIONS: "true",
  HOSTNAME: "0.0.0.0", PORT: "3000",
});
console.warn("SYNTHETIC LOCAL DEMO ONLY: configured actors are not authentication. Do not expose publicly.");
await import("./server.js");
