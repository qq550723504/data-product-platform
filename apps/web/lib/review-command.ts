/** Framework-independent review boundary. Actor and workspace come only from the server. */
export type ReviewConfig = {
  enabled: boolean;
  workspaceId?: string | null;
  actorId?: string | null;
  apiBaseUrl: string;
};
export type ReviewJob = {
  id: string;
  workspaceId: string;
  status: string;
  outputDatasetId?: string;
  outputDatasetVersionId?: string;
};
export type ReviewActionState = {
  ok: boolean;
  message: string;
  job?: ReviewJob;
  refreshRequired?: boolean;
};

export class ReviewCommandError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = "ReviewCommandError";
  }
}

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function isReviewId(value: unknown): value is string {
  return typeof value === "string" && uuidPattern.test(value) && value !== "00000000-0000-0000-0000-000000000000";
}
export function reviewConfigurationError(config: ReviewConfig): string | null {
  if (!config.enabled) return "人工审核写入默认关闭；仅在受信任的 POC 环境设置 POC_ENABLE_REVIEW_ACTIONS=true。";
  if (!isReviewId(config.workspaceId)) return "请配置有效的 POC_WORKSPACE_ID。";
  if (!isReviewId(config.actorId)) return "请配置有效的服务端 POC_REVIEWER_ID；不能使用浏览器传入的审核人身份。";
  return null;
}
function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new ReviewCommandError("Core API 返回了无效的数据，请刷新后核对。", "INVALID_RESPONSE");
  }
  return value as Record<string, unknown>;
}
function sameId(a: unknown, b: string): boolean {
  return isReviewId(a) && a.toLowerCase() === b.toLowerCase();
}
function parseJob(value: unknown, jobId: string, workspaceId: string): ReviewJob {
  const job = record(value);
  if (!sameId(job.id, jobId) || !sameId(job.workspaceId, workspaceId)) {
    throw new ReviewCommandError("任务不属于当前工作区，或返回的任务标识不匹配。", "WORKSPACE_MISMATCH");
  }
  if (typeof job.status !== "string" || !job.status.trim()) {
    throw new ReviewCommandError("Core API 未返回有效任务状态。", "INVALID_RESPONSE");
  }
  return {
    id: job.id as string,
    workspaceId: job.workspaceId as string,
    status: job.status,
    outputDatasetId: isReviewId(job.outputDatasetId) ? job.outputDatasetId : undefined,
    outputDatasetVersionId: isReviewId(job.outputDatasetVersionId) ? job.outputDatasetVersionId : undefined,
  };
}

export async function executeReview(
  form: FormData,
  config: ReviewConfig,
  request: typeof fetch = fetch,
): Promise<ReviewActionState> {
  let attemptedWrite = false;
  try {
    const configurationError = reviewConfigurationError(config);
    if (configurationError) throw new ReviewCommandError(configurationError, "REVIEW_NOT_CONFIGURED");
    const workspaceId = config.workspaceId as string;
    const actorId = config.actorId as string;
    const jobId = form.get("jobId");
    const candidateId = form.get("candidateId");
    const decision = form.get("decision");
    const rawReason = form.get("reason");
    if (!isReviewId(jobId) || !isReviewId(candidateId)) {
      throw new ReviewCommandError("任务和候选标识必须是有效 UUID。", "INVALID_ID");
    }
    if (decision !== "confirm" && decision !== "reject") {
      throw new ReviewCommandError("请选择确认或拒绝。", "INVALID_DECISION");
    }
    if (typeof rawReason !== "string" || !rawReason.trim() || rawReason.length > 2000) {
      throw new ReviewCommandError("请填写审核理由，且不要超过 2000 个字符。", "INVALID_REASON");
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
        } catch { /* Do not expose upstream error bodies or SQL details. */ }
        const message = response.status === 409
          ? "候选已被处理或状态已变化，请刷新队列后核对。"
          : `Core API 拒绝了本次操作（HTTP ${response.status}，${code}）。`;
        throw new ReviewCommandError(message, code);
      }
      return response.json();
    }
    // These read endpoints are global in Core: verify scope before calling the command.
    parseJob(await json(`/api/v1/entity-match-jobs/${jobId}`), jobId, workspaceId);
    // Look up the single candidate instead of draining every candidate of the
    // job: a large match job would otherwise transfer the whole queue for each
    // decision, and a candidate beyond the first page would be misjudged absent.
    const candidate = record(await json(`/api/v1/entity-match-reviews/${candidateId}`));
    if (!sameId(candidate.id, candidateId) || !sameId(candidate.jobId, jobId)) {
      throw new ReviewCommandError("候选不属于此任务，已阻止提交。", "CANDIDATE_MISMATCH");
    }
    if (candidate.status !== "PENDING") {
      return { ok: false, message: "此候选已不在待审核状态，请刷新后核对。", refreshRequired: true };
    }
    if (decision === "confirm" && !isReviewId(candidate.candidateEntityId)) {
      throw new ReviewCommandError("此候选没有可确认的实体；不能自动创建或指定另一个实体。", "ENTITY_REQUIRED");
    }
    attemptedWrite = true;
    const result = await json(`/api/v1/entity-match-reviews/${candidateId}/${decision}`, {
      method: "POST",
      headers: { "Content-Type": "application/json", "X-Actor-ID": actorId },
      body: JSON.stringify({ reason: rawReason.trim() }),
    });
    const job = parseJob(result, jobId, workspaceId);
    return { ok: true, message: `${decision === "confirm" ? "已确认" : "已拒绝"}候选；任务状态：${job.status}。`, job };
  } catch (error) {
    const detail = error instanceof ReviewCommandError ? error.message : "暂时无法核实 Core API 的响应。";
    return {
      ok: false,
      message: attemptedWrite ? `${detail} 请求可能已送达，请先刷新核对，不要直接重复提交。` : detail,
      refreshRequired: attemptedWrite,
    };
  }
}
