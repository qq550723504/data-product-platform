import { access, cp } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const web = fileURLToPath(new URL("../../apps/web/", import.meta.url));
const candidates = [join(web, ".next/standalone/server.js"), join(web, ".next/standalone/apps/web/server.js")];
let server;
for (const candidate of candidates) {
  try { await access(candidate); server = candidate; break; } catch { /* Try known Next monorepo output layout. */ }
}
if (!server) throw new Error("Build the real Next application before preparing its standalone bundle");
const target = dirname(server);
await cp(join(web, ".next/static"), join(target, ".next/static"), { recursive: true });
try {
  await access(join(web, "public"));
  await cp(join(web, "public"), join(target, "public"), { recursive: true });
} catch (error) {
  if (error.code !== "ENOENT") throw error;
}
console.log(`Prepared standalone server: ${resolve(server)}`);
