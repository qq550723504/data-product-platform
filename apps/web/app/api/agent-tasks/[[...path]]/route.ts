import { NextRequest, NextResponse } from "next/server";
import { configuredWorkspaceId } from "@/lib/platform";

export const dynamic = "force-dynamic";
async function proxy(request: NextRequest, context: { params: Promise<{ path?: string[] }> }) {
  const path = (await context.params).path ?? [];
  const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
  const command = request.method === "POST";
  const authority = path.length === 1 && path[0] === "authority";
  if (!(path.length === 0 || authority || (uuid.test(path[0]) && (path.length === 1 || (path.length === 2 && ["revisions", "approvals", "actions", "pause", "resume", "stop"].includes(path[1])))))) return NextResponse.json({ error: { code: "INVALID_ROUTE" } }, { status: 404 });
  if (command && (authority || path.length === 1)) return NextResponse.json({ error: { code: "INVALID_ROUTE" } }, { status: 405 });
  const workspace = configuredWorkspaceId();
  if (!workspace) return NextResponse.json({ error: { code: "WORKSPACE_NOT_CONFIGURED" } }, { status: 503 });
  // Writes and authority checks must use the current request's credential.
  // Deployment read credentials must never authorize an anonymous mutation.
  const incoming = request.headers.get("authorization");
  if (command && process.env.AGENT_TASKS_UI_WRITE_ENABLED !== "true") return NextResponse.json({ error: { code: "AGENT_WRITES_DISABLED" } }, { status: 403 });
  if ((command || authority) && !incoming?.startsWith("Bearer ")) return NextResponse.json({ error: { code: "AUTHENTICATION_REQUIRED" } }, { status: 401 });
  if (command) {
    const origin = request.headers.get("origin");
    let validOrigin = false;
    try { validOrigin = !!origin && new URL(origin).host === request.headers.get("host"); } catch { /* malformed origin is denied */ }
    if (!validOrigin) return NextResponse.json({ error: { code: "ORIGIN_DENIED" } }, { status: 403 });
  }
  const token = incoming ?? (process.env.HUMAN_DECISION_API_TOKEN ? `Bearer ${process.env.HUMAN_DECISION_API_TOKEN}` : "");
  const endpoint = authority ? "agent-task-authority" : `agent-tasks${path.length ? `/${path.join("/")}` : ""}`;
  const target = new URL(`/api/v1/workspaces/${workspace}/${endpoint}`, process.env.PLATFORM_API_BASE_URL ?? "http://127.0.0.1:8080");
  target.search = request.nextUrl.search;
  let body: string | undefined;
  if (command) {
    body = await request.text();
    if (new TextEncoder().encode(body).length > 65536) return NextResponse.json({ error: { code: "BODY_TOO_LARGE" } }, { status: 413 });
  }
  try {
    const response = await fetch(target, { method: request.method, cache: "no-store", signal: AbortSignal.timeout(15000), headers: { Authorization: token, "Content-Type": "application/json", ...(command ? { "Idempotency-Key": request.headers.get("idempotency-key") ?? "" } : {}) }, body });
    const data = await response.json();
    if (!response.ok) return NextResponse.json({error:{code:data.code ?? "AGENT_UNAVAILABLE"}}, {status:response.status,headers:{"Cache-Control":"no-store"}});
    if (authority && response.ok) data.writeEnabled = data.writeEnabled === true && process.env.AGENT_TASKS_UI_WRITE_ENABLED === "true";
    return NextResponse.json(data, { status: response.status, headers: { "Cache-Control": "no-store" } });
  } catch {
    return NextResponse.json({ error: { code: "AGENT_RESPONSE_UNKNOWN" } }, { status: 503 });
  }
}
export const GET = proxy;
export const POST = proxy;
