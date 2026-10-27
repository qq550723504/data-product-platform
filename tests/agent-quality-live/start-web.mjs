// Real standalone Next -> real Core. Logging observes requests, never mocks them.
import { channel } from "node:diagnostics_channel";
import { appendFileSync, existsSync } from "node:fs";
import { fileURLToPath, pathToFileURL } from "node:url";
import { dirname, join } from "node:path";

if (process.env.AGENT_Q1_LIVE_ACK !== "ISOLATED_READ_ONLY") throw new Error("Explicit isolated read-only startup required");
const core = "http://127.0.0.1:18496";
const trace = process.env.AGENT_Q1_READ_TRACE;
if (!trace || process.env.PLATFORM_API_BASE_URL !== core) throw new Error("Fixed real Core and trace required");
function observe(phase, value) {
  const request = value.request;
  if (String(request.origin) !== core) return;
  appendFileSync(trace, JSON.stringify({ at: new Date().toISOString(), phase, method: request.method, path: request.path,
    ...(value.response ? { status: value.response.statusCode } : {}) }) + "\n");
}
channel("undici:request:create").subscribe(value => observe("request", value));
channel("undici:request:headers").subscribe(value => observe("response", value));
const web = fileURLToPath(new URL("../../apps/web/", import.meta.url));
const server = join(web, ".next-agent-q1-live/standalone/server.js");
if (!existsSync(server)) throw new Error("Build standalone web first");
process.chdir(dirname(server));
await import(pathToFileURL(server).href);
