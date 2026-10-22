import "server-only";
import { readFile } from "node:fs/promises";
import { timingSafeEqual } from "node:crypto";

export type AdmissionConfig = { workspace: string; dataset: string; connection: string; principal: string; apiToken: string; gateway: string; apiBase: string; webOrigin: string };
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
export async function sourceAdmissionConfig(): Promise<AdmissionConfig | null> {
  try {
    const path = process.env.FORK_APPROVED_SOURCE_PROFILE;
    const gatewayPath = process.env.FORK_APPROVED_WEB_GATEWAY_TOKEN_FILE;
    const webOrigin = process.env.FORK_APPROVED_WEB_ORIGIN;
    if (!path || !gatewayPath || !webOrigin || new URL(webOrigin).origin !== webOrigin || !["https:", "http:"].includes(new URL(webOrigin).protocol)) return null;
    const p = JSON.parse(await readFile(path, "utf8"));
    if (p.enabled !== true || p.connection?.mode !== "fork_approved" || ![p.connection.workspaceId, p.connection.datasetId, p.connection.id].every((s) => typeof s === "string" && uuid.test(s)) || typeof p.subject !== "string" || !p.subject) return null;
    const apiToken = (await readFile(p.apiTokenFile, "utf8")).trim();
    const gateway = (await readFile(gatewayPath, "utf8")).trim();
    if (!apiToken || !gateway) return null;
    return { workspace: p.connection.workspaceId, dataset: p.connection.datasetId, connection: p.connection.id, principal: p.subject, apiToken, gateway, webOrigin, apiBase: (process.env.PLATFORM_API_BASE_URL ?? "http://127.0.0.1:8080").replace(/\/$/, "") };
  } catch { return null; }
}
// Reuse the existing controlled web gateway contract; no browser-provided actor,
// consumer or upstream credential is accepted. The shared gateway is not IAM.
export function sourceWebAuthorized(headers: Pick<Headers, "get">, c: AdmissionConfig): boolean {
  const supplied = Buffer.from(headers.get("x-delivery-web-gateway") ?? "");
  const expected = Buffer.from(c.gateway);
  return supplied.length === expected.length && timingSafeEqual(supplied, expected) && headers.get("x-authenticated-principal") === c.principal;
}
