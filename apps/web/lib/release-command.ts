import { releaseReadinessProblem } from "./release-readiness";

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

export type ReleaseDatasetBinding = {
  datasetVersionId: string;
  role: "PRIMARY" | "INPUT" | "OUTPUT" | "SUPPORTING";
};

export type CreatedRelease = {
  id: string;
  productId: string;
  productVersionId: string;
  releaseNo: string;
  status: string;
  datasets: ReleaseDatasetBinding[];
};

export type CreateReleaseActionState = {
  ok: boolean;
  message: string;
  release?: CreatedRelease;
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
    if (release.status !== "READY" || readiness.overall !== "READY") {
      return { ok: false, message: "Core 尚未将此 Release 判定为 READY；发布按钮不会绕过任何 Gate。", refreshRequired: true };
    }
    const readinessProblem = releaseReadinessProblem(readiness);
    if (readinessProblem) {
      // A summary without every required gate and an empty blockers list is not sufficient.
      return { ok: false, message: readinessProblem, refreshRequired: true };
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


const releaseRoles = new Set(["PRIMARY", "INPUT", "OUTPUT", "SUPPORTING"]);

function normalizeBindings(form: FormData): ReleaseDatasetBinding[] {
  const versionIds = form.getAll("datasetVersionId");
  const roles = form.getAll("datasetRole");
  if (versionIds.length === 0 || versionIds.length !== roles.length) {
    throw new ReleaseCommandError("至少需要一个完整的 DatasetVersion binding。", "INVALID_DATASET_BINDINGS");
  }
  const bindings = versionIds.map((versionId, index) => {
    const role = typeof roles[index] === "string" ? roles[index].trim().toUpperCase() : "";
    if (!isReleaseId(versionId) || !releaseRoles.has(role)) {
      throw new ReleaseCommandError("DatasetVersion binding 包含无效的版本或 Role。", "INVALID_DATASET_BINDINGS");
    }
    return {
      datasetVersionId: versionId.toLowerCase(),
      role: role as ReleaseDatasetBinding["role"],
    };
  });
  const productionTargets = bindings.filter((binding) => binding.role === "PRIMARY" || binding.role === "OUTPUT");
  if (productionTargets.length !== 1) {
    throw new ReleaseCommandError("Release 必须且只能有一个 PRIMARY 或 OUTPUT DatasetVersion。", "INVALID_DATASET_BINDINGS");
  }
  const keys = bindings.map((binding) => `${binding.datasetVersionId}:${binding.role}`);
  if (new Set(keys).size !== keys.length) {
    throw new ReleaseCommandError("Release 不能包含重复的 DatasetVersion binding。", "INVALID_DATASET_BINDINGS");
  }
  return bindings;
}

function responseBindings(value: unknown): ReleaseDatasetBinding[] {
  if (!Array.isArray(value)) throw new ReleaseCommandError("Core API 未返回有效的 Release Dataset bindings。", "INVALID_RESPONSE");
  return value.map(record).map((binding) => {
    const role = typeof binding.role === "string" ? binding.role.trim().toUpperCase() : "";
    if (!isReleaseId(binding.datasetVersionId) || !releaseRoles.has(role)) {
      throw new ReleaseCommandError("Core API 返回了无效的 Release Dataset binding。", "INVALID_RESPONSE");
    }
    return {
      datasetVersionId: String(binding.datasetVersionId).toLowerCase(),
      role: role as ReleaseDatasetBinding["role"],
    };
  });
}

function bindingFingerprint(bindings: ReleaseDatasetBinding[]): string[] {
  return bindings.map((binding) => `${binding.datasetVersionId}:${binding.role}`).sort();
}

export async function executeCreateRelease(
  form: FormData,
  config: ReleaseCommandConfig,
  request: typeof fetch = fetch,
): Promise<CreateReleaseActionState> {
  let attemptedWrite = false;
  try {
    const configurationError = releaseConfigurationError(config);
    if (configurationError) throw new ReleaseCommandError(configurationError, "RELEASE_NOT_CONFIGURED");
    const workspaceId = (config.workspaceId as string).toLowerCase();
    const actorId = config.actorId as string;
    const productId = form.get("productId");
    const productVersionId = form.get("productVersionId");
    const releaseNoValue = form.get("releaseNo");
    const releaseNotesValue = form.get("releaseNotes");
    const releaseNo = typeof releaseNoValue === "string" ? releaseNoValue.trim() : "";
    const releaseNotes = typeof releaseNotesValue === "string" ? releaseNotesValue.trim() : "";
    if (!isReleaseId(productId) || !isReleaseId(productVersionId)) {
      throw new ReleaseCommandError("产品和 ProductVersion 标识必须是有效 UUID。", "INVALID_ID");
    }
    if (!releaseNo || releaseNo.length > 64) {
      throw new ReleaseCommandError("Release No 必填且不能超过 64 个字符。", "INVALID_RELEASE_NO");
    }
    if (releaseNotes.length > 2000) {
      throw new ReleaseCommandError("Release Notes 不能超过 2000 个字符。", "INVALID_RELEASE_NOTES");
    }
    const bindings = normalizeBindings(form);
    const expectedFingerprint = bindingFingerprint(bindings);

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
        } catch { /* Never expose raw provider/SQL detail. */ }
        throw new ReleaseCommandError(`Core API 拒绝了本次操作（HTTP ${response.status}，${code}）。`, code);
      }
      return safeJSON(response);
    }

    const product = record(await json(`/api/v1/data-products/${productId}`));
    if (!sameId(product.id, productId) || !sameId(product.workspaceId, workspaceId)) {
      throw new ReleaseCommandError("产品不属于当前工作区，已阻止创建 Release。", "WORKSPACE_MISMATCH");
    }
    if (!sameId(product.currentVersionId, productVersionId)) {
      return {
        ok: false,
        message: "当前 ProductVersion 已变化；请刷新页面后基于最新冻结版本创建 Release。",
        refreshRequired: true,
      };
    }

    const productVersion = record(await json(`/api/v1/product-versions/${productVersionId}`));
    if (!sameId(productVersion.id, productVersionId) || !sameId(productVersion.productId, productId)) {
      throw new ReleaseCommandError("ProductVersion 不属于当前产品，已阻止创建 Release。", "PRODUCT_VERSION_SCOPE_MISMATCH");
    }
    const assets = Array.isArray(productVersion.assets) ? productVersion.assets.map(record) : [];
    const allowedDatasetIds = new Set(
      assets
        .filter((asset) => asset.assetType === "DATASET" && isReleaseId(asset.datasetId))
        .map((asset) => String(asset.datasetId).toLowerCase()),
    );
    if (allowedDatasetIds.size === 0) {
      throw new ReleaseCommandError("当前 ProductVersion 没有 DATASET asset，无法从此 UI 创建 Release。", "NO_DATASET_ASSETS");
    }

    for (const binding of bindings) {
      const version = record(await json(
        `/api/v1/dataset-versions/${binding.datasetVersionId}?workspaceId=${encodeURIComponent(workspaceId)}`,
      ));
      if (!sameId(version.id, binding.datasetVersionId) || !isReleaseId(version.datasetId)) {
        throw new ReleaseCommandError("DatasetVersion 返回范围无效，已阻止创建 Release。", "DATASET_VERSION_SCOPE_MISMATCH");
      }
      if (!allowedDatasetIds.has(String(version.datasetId).toLowerCase())) {
        throw new ReleaseCommandError("所选 DatasetVersion 不属于当前 ProductVersion 的 DATASET assets。", "DATASET_VERSION_SCOPE_MISMATCH");
      }
      if (version.status !== "READY" && version.status !== "SUPERSEDED") {
        return {
          ok: false,
          message: "所选 DatasetVersion 已不再可用于 Release；请刷新后重新选择。",
          refreshRequired: true,
        };
      }
    }

    let releaseOffset = 0;
    let existingSummary: Record<string, unknown> | undefined;
    while (!existingSummary) {
      const page = record(await json(
        `/api/v1/workspaces/${workspaceId}/data-products/${productId}/releases?limit=100&offset=${releaseOffset}`,
      ));
      if (!Array.isArray(page.items)) {
        throw new ReleaseCommandError("Core API 未返回有效 Release 列表。", "INVALID_RESPONSE");
      }
      const items = page.items.map(record);
      existingSummary = items.find((release) => typeof release.releaseNo === "string" && release.releaseNo.trim() === releaseNo);
      const meta = record(page.page);
      const total = typeof meta.total === "number" ? meta.total : items.length;
      const offset = typeof meta.offset === "number" ? meta.offset : releaseOffset;
      if (existingSummary || offset + items.length >= total || items.length === 0) break;
      releaseOffset = offset + items.length;
    }
    if (existingSummary) {
      if (!isReleaseId(existingSummary.id)) {
        throw new ReleaseCommandError("Core Release 列表返回了无效标识。", "INVALID_RESPONSE");
      }
      const existing = record(await json(`/api/v1/product-releases/${existingSummary.id}`));
      if (
        !sameId(existing.productId, productId) ||
        !sameId(existing.productVersionId, productVersionId) ||
        existing.releaseNo !== releaseNo ||
        JSON.stringify(bindingFingerprint(responseBindings(existing.datasets))) !== JSON.stringify(expectedFingerprint)
      ) {
        throw new ReleaseCommandError("相同 Release No 已被不同的冻结内容占用。", "RELEASE_NO_CONFLICT");
      }
      return {
        ok: true,
        message: `Release ${releaseNo} 已存在；已恢复到同一个冻结 Release。`,
        release: {
          id: existing.id as string,
          productId: existing.productId as string,
          productVersionId: existing.productVersionId as string,
          releaseNo,
          status: typeof existing.status === "string" ? existing.status : "",
          datasets: responseBindings(existing.datasets),
        },
      };
    }

    attemptedWrite = true;
    const result = record(await json(`/api/v1/data-products/${productId}/releases`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Actor-ID": actorId,
      },
      body: JSON.stringify({
        productVersionId,
        releaseNo,
        datasets: bindings,
        releaseNotes,
        metadata: {},
      }),
    }));
    if (
      !isReleaseId(result.id) ||
      !sameId(result.productId, productId) ||
      !sameId(result.productVersionId, productVersionId) ||
      result.releaseNo !== releaseNo ||
      result.status !== "DRAFT" ||
      JSON.stringify(bindingFingerprint(responseBindings(result.datasets))) !== JSON.stringify(expectedFingerprint)
    ) {
      throw new ReleaseCommandError("创建请求返回了不一致的 Release 冻结事实。", "INVALID_RESPONSE");
    }
    return {
      ok: true,
      message: `Release ${releaseNo} 已创建为 DRAFT。`,
      release: {
        id: result.id as string,
        productId: result.productId as string,
        productVersionId: result.productVersionId as string,
        releaseNo,
        status: result.status as string,
        datasets: responseBindings(result.datasets),
      },
    };
  } catch (error) {
    const detail = error instanceof ReleaseCommandError ? error.message : "暂时无法核实 Core API 的响应。";
    return {
      ok: false,
      message: attemptedWrite
        ? `${detail} 请求可能已送达；请使用相同 Release No 刷新核对后再决定是否重试。`
        : detail,
      refreshRequired: attemptedWrite,
    };
  }
}
