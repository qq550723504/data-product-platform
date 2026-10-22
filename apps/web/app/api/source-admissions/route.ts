import { NextRequest } from "next/server";
import { sourceAdmissionConfig, sourceWebAuthorized } from "@/lib/source-admission-server";

export const dynamic = "force-dynamic";
function reject(status: number, code: string) { return Response.json({ error: { code } }, { status, headers: { "Cache-Control": "no-store" } }); }
async function forward(request: NextRequest) {
  const c = await sourceAdmissionConfig();
  if (!c) return reject(503, "SOURCE_NOT_CONFIGURED");
  if (!sourceWebAuthorized(request.headers, c)) return reject(403, "SOURCE_AUTHORITY_DENIED");
  let body: Record<string, unknown> | undefined;
  let key = "";
  if (request.method === "POST") {
    if (request.headers.get("origin") !== c.webOrigin) return reject(403, "SOURCE_AUTHORITY_DENIED");
    try { const text = await request.text(); if (text.length > 8192) throw new Error(); body = JSON.parse(text); } catch { return reject(400, "SOURCE_IDEMPOTENCY_CONFLICT"); }
    if (!body || Array.isArray(body) || Object.keys(body).sort().join(",") !== "manifestSha256,releaseId,snapshotId") return reject(400, "SOURCE_IDEMPOTENCY_CONFLICT");
    key = request.headers.get("idempotency-key") ?? "";
    body = { ...body, datasetId: c.dataset, connectionId: c.connection };
  } else { key = request.nextUrl.searchParams.get("key") ?? ""; }
  if (!key || key.length > 255 || key.trim() !== key) return reject(400, "SOURCE_IDEMPOTENCY_CONFLICT");
  const path = `/api/v1/workspaces/${c.workspace}/source-admissions${request.method === "GET" ? `?key=${encodeURIComponent(key)}` : ""}`;
  try {
    const r = await fetch(c.apiBase + path, { method: request.method, cache: "no-store", redirect: "error", signal: AbortSignal.timeout(90000), headers: { Authorization: `Bearer ${c.apiToken}`, "Content-Type": "application/json", "Idempotency-Key": key }, ...(body ? { body: JSON.stringify(body) } : {}) });
    const data = await r.json();
    // Never forward upstream headers or an arbitrary error message/credential.
    if (!r.ok) return reject(r.status, typeof data.error?.code === "string" ? data.error.code : "SOURCE_TRANSPORT_UNKNOWN");
    return Response.json({ operationId: data.operationId, state: data.state, datasetVersionId: data.datasetVersionId, rowCount: data.rowCount, manifestSha256: data.manifestSha256, errorCode: data.errorCode, goldCertified: false, published: false }, { headers: { "Cache-Control": "no-store" } });
  } catch { return reject(503, "SOURCE_TRANSPORT_UNKNOWN"); }
}
export const GET = forward;
export const POST = forward;
