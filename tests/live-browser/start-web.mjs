import { readFile, access } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

if (process.env.LIVE_BROWSER_ACCEPTANCE !== "1") throw new Error("Opt-in live acceptance only");
const manifest = JSON.parse(await readFile(process.env.LIVE_BROWSER_MANIFEST, "utf8"));
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
for (const key of ["workspaceId", "reviewerId", "publisherId"]) {
  if (!uuid.test(manifest[key] ?? "")) throw new Error(`Invalid test identity: ${key}`);
}
if (process.env.LIVE_BROWSER_PHASE === "ingest" && !uuid.test(manifest.ingestActorId ?? "")) throw new Error("Invalid ingestion actor");
// Fixed loopback targets, no existing deployment URL and no browser-supplied actor.
Object.assign(process.env, {
  NODE_ENV: "production", HOSTNAME: "127.0.0.1", PORT: "13100",
  PLATFORM_API_BASE_URL: "http://127.0.0.1:18080", POC_WORKSPACE_ID: manifest.workspaceId,
  POC_ENABLE_INGEST_ACTIONS: process.env.LIVE_BROWSER_PHASE === "ingest" ? "true" : "false", POC_INGEST_ACTOR_ID: manifest.ingestActorId ?? "",
  POC_ENABLE_REVIEW_ACTIONS: "true", HUMAN_DECISION_API_TOKEN: process.env.HUMAN_DECISION_API_TOKEN,
  POC_ENABLE_RELEASE_ACTIONS: "true", POC_RELEASE_ACTOR_ID: manifest.publisherId,
  NEXT_TELEMETRY_DISABLED: "1",
});
if (!process.env.HUMAN_DECISION_API_TOKEN) throw new Error("Missing live Human Decision credential");
const web = fileURLToPath(new URL("../../apps/web/", import.meta.url));
let server;
for (const path of [join(web, ".next/standalone/server.js"), join(web, ".next/standalone/apps/web/server.js")]) {
  try { await access(path); server = path; break; } catch { /* Try the other supported bundle layout. */ }
}
if (!server) throw new Error("Missing built standalone server");
process.chdir(dirname(server));
await import(pathToFileURL(server).href);
