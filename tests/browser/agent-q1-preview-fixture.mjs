// Local synthetic acceptance fixture. No database, provider, worker or model.
import { createServer } from "node:http";
import { createFixtureServer } from "./fixture-server.mjs";

const fixture = createFixtureServer();
const server = createServer((request, response) => {
  const path = new URL(request.url, "http://127.0.0.1").pathname;
  // Only token-protected fixture scenario controls may mutate test data.
  // Every business endpoint is read-only, regardless of web UI configuration.
  if (request.method !== "GET" && !path.startsWith("/__control/")) {
    response.writeHead(405, { "Content-Type": "application/json", "Cache-Control": "no-store", Allow: "GET" });
    response.end(JSON.stringify({ code: "READ_ONLY_PREVIEW" }));
    return;
  }
  fixture.emit("request", request, response);
});
server.listen(4459, "127.0.0.1", () => console.log("Agent Q1 read-only synthetic fixture: http://127.0.0.1:4459"));
process.on("SIGTERM", () => server.close(() => process.exit(0)));
