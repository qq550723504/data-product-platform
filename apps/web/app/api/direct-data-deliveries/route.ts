import { timingSafeEqual } from "node:crypto";
import { NextRequest } from "next/server";
import { configuredWorkspaceId } from "@/lib/platform";

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function config() {
  const workspaceId = configuredWorkspaceId();
  const token = process.env.DELIVERY_API_TOKEN?.trim() ?? "";
  const consumer = process.env.DELIVERY_API_CONSUMER_REF?.trim() ?? "";
  const principal = process.env.DELIVERY_API_PRINCIPAL_REF?.trim() ?? "";
  const gatewayToken = process.env.DELIVERY_WEB_GATEWAY_TOKEN?.trim() ?? "";
  const enabled = process.env.POC_ENABLE_DELIVERY_ACTIONS === "true";
  const apiBaseUrl = (process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080").replace(/\/$/, "");
  if (!enabled || !workspaceId || !uuidPattern.test(workspaceId) || !token || !consumer || !principal || !gatewayToken) return null;
  return { workspaceId, token, consumer, principal, gatewayToken, apiBaseUrl };
}

function error(status: number, code: string, message: string, extra: Record<string, unknown> = {}) {
  return Response.json({ error: { code, message }, ...extra }, { status });
}

function safeKey(value: unknown): value is string {
  return typeof value === "string" && value.trim().length > 0 && value.trim().length <= 255;
}
function safeUUID(value: unknown): value is string {
  return typeof value === "string" && uuidPattern.test(value) && value !== "00000000-0000-0000-0000-000000000000";
}
function safeText(value: unknown, max = 512): value is string {
  return typeof value === "string" && value.trim().length > 0 && value.trim().length <= max;
}

function secureEqual(left: string, right: string): boolean {
  const a = Buffer.from(left);
  const b = Buffer.from(right);
  return a.length === b.length && timingSafeEqual(a, b);
}

function authorizeWebCaller(request: NextRequest, cfg: NonNullable<ReturnType<typeof config>>) {
  const gateway = request.headers.get("x-delivery-web-gateway")?.trim() ?? "";
  const principal = request.headers.get("x-authenticated-principal")?.trim() ?? "";
  if (!gateway || !secureEqual(gateway, cfg.gatewayToken)) {
    return error(401, "WEB_CALLER_UNTRUSTED", "Direct Data delivery requires a trusted authenticated web gateway.");
  }
  if (!principal || principal !== cfg.principal) {
    return error(403, "WEB_CALLER_PRINCIPAL_MISMATCH", "Authenticated web principal is not authorized for this Delivery credential.");
  }
  return null;
}

async function upstreamError(response: Response) {
  let code = `HTTP_${response.status}`;
  let message = "Direct Data request failed";
  let operationId: string | undefined;
  let blockers: unknown;
  try {
    const payload = await response.json() as Record<string, unknown>;
    const nested = payload.error && typeof payload.error === "object" ? payload.error as Record<string, unknown> : payload;
    if (typeof nested.code === "string") code = nested.code;
    if (typeof nested.message === "string") message = nested.message;
    if (typeof payload.code === "string") code = payload.code;
    if (typeof payload.message === "string") message = payload.message;
    if (typeof payload.operationId === "string") operationId = payload.operationId;
    blockers = payload.blockers;
  } catch {}
  return error(response.status, code, message, { ...(operationId ? { operationId } : {}), ...(blockers ? { blockers } : {}) });
}

export async function GET(request: NextRequest) {
  const cfg = config();
  if (!cfg) return error(503, "DIRECT_DATA_UI_NOT_CONFIGURED", "Direct Data delivery is not configured.");
  const authError = authorizeWebCaller(request, cfg);
  if (authError) return authError;
  const key = request.nextUrl.searchParams.get("idempotencyKey")?.trim() ?? "";
  const consumer = request.nextUrl.searchParams.get("consumer")?.trim() ?? "";
  const versionId = request.nextUrl.searchParams.get("versionId")?.trim() ?? "";
  if (!safeKey(key) || !safeUUID(versionId)) return error(400, "INVALID_RECOVERY_REQUEST", "A valid idempotency key and DatasetVersion are required.");
  if (consumer !== cfg.consumer) return error(403, "CONSUMER_PRINCIPAL_MISMATCH", "Requested consumer does not match the trusted server binding.");

  try {
    const upstream = await fetch(
      `${cfg.apiBaseUrl}/api/v1/workspaces/${cfg.workspaceId}/direct-data-deliveries/recovery?idempotencyKey=${encodeURIComponent(key)}&consumer=${encodeURIComponent(consumer)}`,
      { cache: "no-store", headers: { Accept: "application/json", Authorization: `Bearer ${cfg.token}` }, signal: AbortSignal.timeout(15000) },
    );
    if (!upstream.ok) return upstreamError(upstream);
    const payload = await upstream.json() as Record<string, unknown>;
    if (payload.workspaceId !== cfg.workspaceId || payload.datasetVersionId !== versionId || payload.consumer !== consumer || payload.idempotencyKey !== key) {
      return error(502, "DIRECT_DATA_RECOVERY_SCOPE_MISMATCH", "Recovered delivery operation does not match the requested context.");
    }
    const result = {
      operationId: payload.operationId,
      profileId: payload.profileId,
      datasetVersionId: payload.datasetVersionId,
      retryOfDeliveryOperationId: payload.retryOfDeliveryOperationId,
      idempotencyKey: payload.idempotencyKey,
      status: payload.status,
      gateDecision: payload.gateDecision,
      consumer: payload.consumer,
      purpose: payload.purpose,
      action: payload.action,
      scopeRef: payload.scopeRef,
      terminalReason: payload.terminalReason,
      createdAt: payload.createdAt,
      updatedAt: payload.updatedAt,
    };
    return Response.json(result, { headers: { "Cache-Control": "no-store" } });
  } catch {
    return error(502, "DIRECT_DATA_RECOVERY_UNAVAILABLE", "Unable to verify the Direct Data delivery attempt.");
  }
}

export async function POST(request: NextRequest) {
  const cfg = config();
  if (!cfg) return error(503, "DIRECT_DATA_UI_NOT_CONFIGURED", "Direct Data delivery is not configured.");
  const authError = authorizeWebCaller(request, cfg);
  if (authError) return authError;

  let body: Record<string, unknown>;
  try {
    body = await request.json() as Record<string, unknown>;
  } catch {
    return error(400, "INVALID_DELIVERY_REQUEST", "Delivery request must be valid JSON.");
  }

  const { versionId, profileId, consumer, purpose, action, scopeType, scopeRef, idempotencyKey, retryOfDeliveryOperationId } = body;
  if (!safeUUID(versionId) || !safeUUID(profileId) || !safeKey(idempotencyKey) ||
      !safeText(consumer, 512) || !safeText(purpose, 512) || !safeText(action, 128) || !safeText(scopeType, 128)) {
    return error(400, "INVALID_DELIVERY_REQUEST", "Direct Data delivery context is incomplete or invalid.");
  }
  if (consumer.trim() !== cfg.consumer) return error(403, "CONSUMER_PRINCIPAL_MISMATCH", "Requested consumer does not match the trusted server binding.");
  if (typeof scopeRef !== "string" || scopeRef.length > 1024) return error(400, "INVALID_DELIVERY_REQUEST", "scopeRef is invalid.");
  if (retryOfDeliveryOperationId !== undefined && retryOfDeliveryOperationId !== "" && !safeUUID(retryOfDeliveryOperationId)) {
    return error(400, "INVALID_DELIVERY_REQUEST", "retryOfDeliveryOperationId must be a UUID.");
  }

  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 60000);
    let upstream: Response;
    try {
      upstream = await fetch(
        `${cfg.apiBaseUrl}/api/v1/workspaces/${cfg.workspaceId}/dataset-versions/${versionId}/deliveries`,
        {
          method: "POST",
          cache: "no-store",
          headers: {
            "Content-Type": "application/json",
            Accept: "application/octet-stream,application/json",
            Authorization: `Bearer ${cfg.token}`,
            "Idempotency-Key": idempotencyKey.trim(),
            "X-Trace-ID": crypto.randomUUID(),
          },
          body: JSON.stringify({
            profileId,
            consumer: consumer.trim(),
            purpose: purpose.trim(),
            action: action.trim(),
            scopeType: scopeType.trim(),
            scopeRef: scopeRef.trim(),
            ...(retryOfDeliveryOperationId ? { retryOfDeliveryOperationId } : {}),
          }),
          signal: controller.signal,
        },
      );
    } finally {
      clearTimeout(timeout);
    }
    if (!upstream.ok) return upstreamError(upstream);
    if (!upstream.body) return error(502, "DIRECT_DATA_EMPTY_PAYLOAD", "Core authorized the delivery but returned no payload.");
    const headers = new Headers({
      "Content-Type": upstream.headers.get("content-type") || "application/octet-stream",
      "Cache-Control": "no-store",
      "Content-Disposition": `attachment; filename="dataset-version-${versionId}.bin"`,
    });
    const operationId = upstream.headers.get("x-delivery-operation-id");
    if (operationId) headers.set("X-Delivery-Operation-Id", operationId);
    const contentLength = upstream.headers.get("content-length");
    if (contentLength) headers.set("Content-Length", contentLength);
    return new Response(upstream.body, { status: 200, headers });
  } catch {
    return error(502, "DIRECT_DATA_DELIVERY_UNCERTAIN", "The delivery request may have reached Core. Recover this idempotency key before creating another attempt.", { refreshRequired: true });
  }
}
