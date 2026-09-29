export type DatasetVersionCommandConfig = {
  enabled: boolean;
  workspaceId?: string | null;
  actorId?: string | null;
  apiBaseUrl: string;
};

export type InvalidatedDatasetVersion = {
  id: string;
  datasetId: string;
  status: string;
  invalidatedAt?: string;
  invalidationReason: string;
};

export type DatasetVersionActionState = {
  ok: boolean;
  message: string;
  version?: InvalidatedDatasetVersion;
  refreshRequired?: boolean;
};

export class DatasetVersionCommandError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = "DatasetVersionCommandError";
  }
}

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function isDatasetVersionId(value: unknown): value is string {
  return typeof value === "string" && uuidPattern.test(value) && value !== "00000000-0000-0000-0000-000000000000";
}

export function datasetVersionConfigurationError(config: DatasetVersionCommandConfig): string | null {
  if (!config.enabled) return "DatasetVersion 写入默认关闭；仅在受信任的 POC 环境设置 POC_ENABLE_DATASET_ACTIONS=true。";
  if (!isDatasetVersionId(config.workspaceId)) return "请配置有效的 POC_WORKSPACE_ID。";
  if (!isDatasetVersionId(config.actorId)) return "请配置有效的服务端 POC_DATASET_ACTOR_ID；不能使用浏览器传入的操作人身份。";
  return null;
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new DatasetVersionCommandError("Core API 返回了无效的数据，请刷新后核对。", "INVALID_RESPONSE");
  }
  return value as Record<string, unknown>;
}

function sameId(a: unknown, b: string): boolean {
  return isDatasetVersionId(a) && a.toLowerCase() === b.toLowerCase();
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    throw new DatasetVersionCommandError("Core API 返回了无法解析的响应。", "INVALID_RESPONSE");
  }
}

export async function executeInvalidateDatasetVersion(
  form: FormData,
  config: DatasetVersionCommandConfig,
  request: typeof fetch = fetch,
): Promise<DatasetVersionActionState> {
  let attemptedWrite = false;
  try {
    const configurationError = datasetVersionConfigurationError(config);
    if (configurationError) throw new DatasetVersionCommandError(configurationError, "DATASET_VERSION_NOT_CONFIGURED");

    const workspaceId = config.workspaceId as string;
    const actorId = config.actorId as string;
    const datasetId = form.get("datasetId");
    const versionId = form.get("versionId");
    const reasonValue = form.get("reason");
    const reason = typeof reasonValue === "string" ? reasonValue.trim() : "";

    if (!isDatasetVersionId(datasetId) || !isDatasetVersionId(versionId)) {
      throw new DatasetVersionCommandError("Dataset 和 DatasetVersion 标识必须是有效 UUID。", "INVALID_ID");
    }
    if (!reason) {
      throw new DatasetVersionCommandError("作废原因不能为空。", "INVALID_REASON");
    }
    if (reason.length > 500) {
      throw new DatasetVersionCommandError("作废原因不能超过 500 个字符。", "INVALID_REASON");
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
        } catch { /* Never expose raw upstream detail. */ }
        const message = response.status === 409
          ? `DatasetVersion 状态已变化，Core 拒绝本次作废（${code}），请刷新后核对。`
          : `Core API 拒绝了本次操作（HTTP ${response.status}，${code}）。`;
        throw new DatasetVersionCommandError(message, code);
      }
      return safeJSON(response);
    }

    const dataset = record(await json(`/api/v1/workspaces/${workspaceId}/datasets/${datasetId}`));
    if (!sameId(dataset.id, datasetId) || !sameId(dataset.workspaceId, workspaceId)) {
      throw new DatasetVersionCommandError("Dataset 不属于当前工作区，已阻止作废。", "WORKSPACE_MISMATCH");
    }

    const version = record(await json(
      `/api/v1/dataset-versions/${versionId}?workspaceId=${encodeURIComponent(workspaceId)}`,
    ));
    if (!sameId(version.id, versionId) || !sameId(version.datasetId, datasetId)) {
      throw new DatasetVersionCommandError("DatasetVersion 不属于当前 Dataset，已阻止作废。", "DATASET_VERSION_SCOPE_MISMATCH");
    }
    if (version.status !== "READY") {
      return {
        ok: false,
        message: "Core 只允许 READY DatasetVersion 执行作废；请刷新后核对当前状态。",
        refreshRequired: true,
      };
    }

    attemptedWrite = true;
    const result = record(await json(`/api/v1/dataset-versions/${versionId}/invalidate`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Actor-ID": actorId,
      },
      body: JSON.stringify({ reason }),
    }));

    if (
      !sameId(result.id, versionId) ||
      !sameId(result.datasetId, datasetId) ||
      result.status !== "INVALID" ||
      result.invalidationReason !== reason
    ) {
      throw new DatasetVersionCommandError("作废请求返回了不一致的 DatasetVersion 状态。", "INVALID_RESPONSE");
    }

    return {
      ok: true,
      message: "DatasetVersion 已由 Core 标记为 INVALID。",
      version: {
        id: result.id as string,
        datasetId: result.datasetId as string,
        status: result.status as string,
        invalidatedAt: typeof result.invalidatedAt === "string" ? result.invalidatedAt : undefined,
        invalidationReason: result.invalidationReason as string,
      },
    };
  } catch (error) {
    const detail = error instanceof DatasetVersionCommandError ? error.message : "暂时无法核实 Core API 的响应。";
    return {
      ok: false,
      message: attemptedWrite ? `${detail} 请求可能已送达，请先刷新核对，不要直接重复提交。` : detail,
      refreshRequired: attemptedWrite,
    };
  }
}
