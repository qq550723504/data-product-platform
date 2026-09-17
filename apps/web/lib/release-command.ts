import { evaluateReleaseReadiness } from "./release-readiness";

/** Framework-independent ProductRelease publish boundary. Core remains authoritative for readiness and status. */
export type ReleaseCommandConfig = {
  enabled: boolean;
  workspaceId?: string | null;
  actorId?: string | null;
  apiBaseUrl: string;
};

export type PublishedRelease = {
  id: string;
  productId: string;
  releaseNo: string;
  status: string;
  releasedAt?: string;
};

export type ReleaseActionState = {
  ok: boolean;
  message: string;
  release?: PublishedRelease;
  refreshRequired?: boolean;
};

export class ReleaseCommandError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = "ReleaseCommandError";
  }
}

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function isReleaseId(value: unknown): value is string {
  return typeof value === "string" && uuidPattern.test(value) && value !== "00000000-0000-0000-0000-000000000000";
}

export function releaseConfigurationError(config: ReleaseCommandConfig): string | null {
  if (!config.enabled) return "Release 发布写入默认关闭；仅在受信任的 POC 环境设置 POC_ENABLE_RELEASE_ACTIONS=true。";
  if (!isReleaseId(config.workspaceId)) return "请配置有效的 POC_WORKSPACE_ID。";
  if (!isReleaseId(config.actorId)) return "请配置有效的服务端 POC_RELEASE_ACTOR_ID；不能使用浏览器传入的发布人身份。";
  return null;
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new ReleaseCommandError("Core API 返回了无效的数据，请刷新后核对。", "INVALID_RESPONSE");
  }
  return value as Record<string, unknown>;
}

function sameId(a: unknown, b: string): boolean {
  return isReleaseId(a) && a.toLowerCase() === b.toLowerCase();
}

function itemArray(value: unknown): Record<string, unknown>[] {
  const payload = record(value);
  if (!Array.isArray(payload.items)) {
    throw new ReleaseCommandError("Core API 未返回有效列表。", "INVALID_RESPONSE");
  }
  return payload.items.map(record);
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    throw new ReleaseCommandError("Core API 返回了无法解析的响应。", "INVALID_RESPONSE");
  }
}

export async function executePublish(
  form: FormData,
  config: ReleaseCommandConfig,
  idempotencyKey: string,
  request: typeof fetch = fetch,
): Promise<ReleaseActionState> {
  let attemptedWrite = false;
  try {
    const configurationError = releaseConfigurationError(config);
    if (configurationError) throw new ReleaseCommandError(configurationError, "RELEASE_NOT_CONFIGURED");
    const workspaceId = config.workspaceId as string;
    const actorId = config.actorId as string;
    const productId = form.get("productId");
    const releaseId = form.get("releaseId");
    if (!isReleaseId(productId) || !isReleaseId(releaseId)) {
      throw new ReleaseCommandError("产品和 Release 标识必须是有效 UUID。", "INVALID_ID");
    }
    if (!idempotencyKey.trim() || idempotencyKey.length > 200) {
      throw new ReleaseCommandError("服务端未生成有效幂等键。", "INVALID_IDEMPOTENCY_KEY");
    }

    const base = config.apiBaseUrl.replace(/\/$/, "");
    async function json(path: string, init: RequestInit = {}): Promise<unknown> {
      const response = await request(`${base}${path}`, {
        ...init,
        cache: "no-store",
        redirect: "error",
        signal: AbortSignal.timeout(15000),
        headers: { Accept: "application/json", ...init.headers },
      });
      if (!response.ok) {
        let code = `HTTP_${response.status}`;
        try {
          const body = record(await response.json());
          const error = body.error && typeof body.error === "object" ? record(body.error) : body;
          if (typeof error.code === "string" && /^[A-Z0-9_]{1,80}$/.test(error.code)) code = error.code;
        } catch { /* Never expose provider/SQL detail from an upstream body. */ }
        const message = response.status === 409
          ? `Release 状态已变化或尚未满足发布条件（${code}），请刷新后核对。`
          : `Core API 拒绝了本次操作（HTTP ${response.status}，${code}）。`;
        throw new ReleaseCommandError(message, code);
      }
      return safeJSON(response);
    }

    // Workspace-scoped read models are the preflight authority for this POC UI.
    const products = itemArray(await json(`/api/v1/workspaces/${workspaceId}/data-products?limit=100&offset=0`));
    const product = products.find((item) => sameId(item.id, productId));
    if (!product || !sameId(product.workspaceId, workspaceId)) {
      throw new ReleaseCommandError("产品不属于当前工作区，已阻止发布。", "WORKSPACE_MISMATCH");
    }

    const releases = itemArray(await json(`/api/v1/workspaces/${workspaceId}/data-products/${productId}/releases?limit=100&offset=0`));
    const listedRelease = releases.find((item) => sameId(item.id, releaseId));
    if (!listedRelease || !sameId(listedRelease.productId, productId)) {
      throw new ReleaseCommandError("Release 不属于当前产品，已阻止发布。", "RELEASE_SCOPE_MISMATCH");
    }

    const release = record(await json(`/api/v1/product-releases/${releaseId}`));
    if (!sameId(release.id, releaseId) || !sameId(release.productId, productId)) {
      throw new ReleaseCommandError("Release 返回范围与当前产品不一致，已阻止发布。", "RELEASE_SCOPE_MISMATCH");
    }
    const readiness = record(await json(`/api/v1/product-releases/${releaseId}/readiness`));
    if (!sameId(readiness.releaseId, releaseId)) {
      throw new ReleaseCommandError("Readiness 返回了不匹配的 Release。", "READINESS_MISMATCH");
    }

    const evaluation = evaluateReleaseReadiness(release.status, readiness);
    if (!evaluation.ready) {
      return {
        ok: false,
        message: `Core Release Readiness 不完整或未通过（${evaluation.problems.join(" · ")}），已阻止发布，请刷新后核对。`,
        refreshRequired: true,
      };
    }

    attemptedWrite = true;
    const result = record(await json(`/api/v1/product-releases/${releaseId}/publish`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Actor-ID": actorId,
        "Idempotency-Key": idempotencyKey,
      },
      body: "{}",
    }));
    if (!sameId(result.id, releaseId) || !sameId(result.productId, productId) || result.status !== "PUBLISHED") {
      throw new ReleaseCommandError("发布请求返回了不一致的 Release 状态。", "INVALID_RESPONSE");
    }
    return {
      ok: true,
      message: `Release ${String(result.releaseNo ?? "")} 已发布。`,
      release: {
        id: result.id as string,
        productId: result.productId as string,
        releaseNo: typeof result.releaseNo === "string" ? result.releaseNo : "",
        status: result.status as string,
        releasedAt: typeof result.releasedAt === "string" ? result.releasedAt : undefined,
      },
    };
  } catch (error) {
    const detail = error instanceof ReleaseCommandError ? error.message : "暂时无法核实 Core API 的响应。";
    return {
      ok: false,
      message: attemptedWrite ? `${detail} 请求可能已送达，请先刷新核对，不要直接重复提交。` : detail,
      refreshRequired: attemptedWrite,
    };
  }
}
