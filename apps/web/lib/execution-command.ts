export type ExecutionCommandConfig = {
  enabled: boolean;
  workspaceId?: string | null;
  actorId?: string | null;
  apiBaseUrl: string;
};

export type RetriedExecution = {
  id: string;
  workspaceId: string;
  workflowVersionId: string;
  outputDatasetId: string;
  targetPeriod: string;
  status: string;
  attempt: number;
  retryOfExecutionId?: string;
};

export type ExecutionActionState = {
  ok: boolean;
  message: string;
  execution?: RetriedExecution;
  refreshRequired?: boolean;
};

export class ExecutionCommandError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = "ExecutionCommandError";
  }
}

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function isExecutionId(value: unknown): value is string {
  return typeof value === "string" && uuidPattern.test(value) && value !== "00000000-0000-0000-0000-000000000000";
}

export function executionConfigurationError(config: ExecutionCommandConfig): string | null {
  if (!config.enabled) return "Execution 写入默认关闭；仅在受信任的 POC 环境设置 POC_ENABLE_EXECUTION_ACTIONS=true。";
  if (!isExecutionId(config.workspaceId)) return "请配置有效的 POC_WORKSPACE_ID。";
  if (!isExecutionId(config.actorId)) return "请配置有效的服务端 POC_EXECUTION_ACTOR_ID；不能使用浏览器传入的操作人身份。";
  return null;
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new ExecutionCommandError("Core API 返回了无效的数据，请刷新后核对。", "INVALID_RESPONSE");
  }
  return value as Record<string, unknown>;
}

function sameId(a: unknown, b: string): boolean {
  return isExecutionId(a) && a.toLowerCase() === b.toLowerCase();
}

function inputFingerprint(value: unknown): string[] {
  if (!Array.isArray(value)) throw new ExecutionCommandError("Core API 未返回有效的冻结输入。", "INVALID_RESPONSE");
  return value.map(record).map((input) => {
    if (typeof input.name !== "string" || !input.name.trim() || !isExecutionId(input.datasetVersionId)) {
      throw new ExecutionCommandError("Core API 未返回有效的冻结输入。", "INVALID_RESPONSE");
    }
    return `${input.name.trim()}:${String(input.datasetVersionId).toLowerCase()}`;
  }).sort();
}

async function safeJSON(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    throw new ExecutionCommandError("Core API 返回了无法解析的响应。", "INVALID_RESPONSE");
  }
}

export async function executeRetry(
  form: FormData,
  config: ExecutionCommandConfig,
  idempotencyKey: string,
  request: typeof fetch = fetch,
): Promise<ExecutionActionState> {
  let attemptedWrite = false;
  try {
    const configurationError = executionConfigurationError(config);
    if (configurationError) throw new ExecutionCommandError(configurationError, "EXECUTION_NOT_CONFIGURED");
    const workspaceId = config.workspaceId as string;
    const actorId = config.actorId as string;
    const executionId = form.get("executionId");
    if (!isExecutionId(executionId)) {
      throw new ExecutionCommandError("Execution 标识必须是有效 UUID。", "INVALID_ID");
    }
    if (!idempotencyKey.trim() || idempotencyKey.length > 200) {
      throw new ExecutionCommandError("服务端未生成有效幂等键。", "INVALID_IDEMPOTENCY_KEY");
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
          ? `Execution 状态或冻结引用已变化（${code}），请刷新后核对。`
          : `Core API 拒绝了本次操作（HTTP ${response.status}，${code}）。`;
        throw new ExecutionCommandError(message, code);
      }
      return safeJSON(response);
    }

    const source = record(await json(`/api/v1/executions/${executionId}`));
    if (!sameId(source.id, executionId) || !sameId(source.workspaceId, workspaceId)) {
      throw new ExecutionCommandError("Execution 不属于当前工作区，已阻止重试。", "WORKSPACE_MISMATCH");
    }
    if (source.status !== "FAILED" && source.status !== "CANCELLED") {
      return { ok: false, message: "Core 只允许 FAILED 或 CANCELLED Execution 创建 Retry；请刷新后核对当前状态。", refreshRequired: true };
    }
    if (
      !isExecutionId(source.workflowVersionId) ||
      !isExecutionId(source.outputDatasetId) ||
      typeof source.attempt !== "number" ||
      typeof source.targetPeriod !== "string" ||
      typeof source.engineType !== "string"
    ) {
      throw new ExecutionCommandError("Core API 未返回完整的 Execution 冻结事实。", "INVALID_RESPONSE");
    }
    const sourceInputs = inputFingerprint(source.inputs);

    attemptedWrite = true;
    const result = record(await json(`/api/v1/executions/${executionId}/retry`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Actor-ID": actorId,
        "Idempotency-Key": idempotencyKey,
      },
      body: "{}",
    }));

    if (!isExecutionId(result.id) || sameId(result.id, executionId)) {
      throw new ExecutionCommandError("Retry 未返回新的 Execution 标识。", "INVALID_RESPONSE");
    }
    if (
      !sameId(result.workspaceId, workspaceId) ||
      !sameId(result.retryOfExecutionId, executionId) ||
      !sameId(result.workflowVersionId, source.workflowVersionId as string) ||
      !sameId(result.outputDatasetId, source.outputDatasetId as string) ||
      result.targetPeriod !== source.targetPeriod ||
      result.engineType !== source.engineType ||
      !["QUEUED", "SUBMITTING", "RUNNING", "SUCCEEDED", "FAILED", "CANCELLED"].includes(String(result.status)) ||
      result.attempt !== (source.attempt as number) + 1 ||
      JSON.stringify(inputFingerprint(result.inputs)) !== JSON.stringify(sourceInputs)
    ) {
      throw new ExecutionCommandError("Retry 返回的 Execution 与冻结源事实不一致。", "INVALID_RESPONSE");
    }

    return {
      ok: true,
      message: `已创建 Retry Execution（Attempt ${result.attempt}），正在进入队列。`,
      execution: {
        id: result.id as string,
        workspaceId: result.workspaceId as string,
        workflowVersionId: result.workflowVersionId as string,
        outputDatasetId: result.outputDatasetId as string,
        targetPeriod: typeof result.targetPeriod === "string" ? result.targetPeriod : "",
        status: result.status as string,
        attempt: result.attempt as number,
        retryOfExecutionId: result.retryOfExecutionId as string,
      },
    };
  } catch (error) {
    const detail = error instanceof ExecutionCommandError ? error.message : "暂时无法核实 Core API 的响应。";
    return {
      ok: false,
      message: attemptedWrite ? `${detail} 请求可能已送达，请先刷新核对，不要直接重复提交。` : detail,
      refreshRequired: attemptedWrite,
    };
  }
}
