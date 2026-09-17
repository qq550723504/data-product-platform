"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import {
  PlatformError,
  configuredActorId,
  configuredWorkspaceId,
  platform,
} from "@/lib/platform";

function resultUrl(kind: "result" | "error", message: string): string {
  const params = new URLSearchParams({ [kind]: message });
  return `/reviews?${params.toString()}`;
}

function errorMessage(error: unknown): string {
  if (error instanceof PlatformError) {
    return error.code ? `${error.code}: ${error.message}` : error.message;
  }
  if (error instanceof Error) return error.message;
  return "实体审核失败";
}

export async function reviewCandidateAction(formData: FormData): Promise<void> {
  const workspaceId = configuredWorkspaceId();
  const actorId = configuredActorId();
  const candidateId = String(formData.get("candidateId") ?? "").trim();
  const jobId = String(formData.get("jobId") ?? "").trim();
  const reason = String(formData.get("reason") ?? "").trim();
  const intent = String(formData.get("intent") ?? "").trim();

  if (!workspaceId) redirect(resultUrl("error", "POC_WORKSPACE_ID 未配置"));
  if (!actorId) redirect(resultUrl("error", "POC_ACTOR_ID 未配置，无法执行审计动作"));
  if (!candidateId || !jobId) redirect(resultUrl("error", "审核候选信息不完整"));
  if (!reason) redirect(resultUrl("error", "人工审核必须填写判断理由"));
  if (intent !== "confirm" && intent !== "reject") redirect(resultUrl("error", "未知审核动作"));

  let failure: string | null = null;
  try {
    const [job, candidates] = await Promise.all([
      platform.entityMatchJob(jobId),
      platform.jobReviews(jobId),
    ]);
    if (job.workspaceId !== workspaceId) {
      throw new Error("审核任务不属于当前 POC Workspace");
    }
    const candidate = candidates.items.find((item) => item.id === candidateId);
    if (!candidate || candidate.jobId !== jobId) {
      throw new Error("候选记录不属于当前审核任务");
    }
    if (candidate.status !== "PENDING") {
      throw new Error(`候选记录当前状态为 ${candidate.status}，不能重复审核`);
    }

    if (intent === "confirm") {
      await platform.confirmReview(candidateId, reason, actorId);
    } else {
      await platform.rejectReview(candidateId, reason, actorId);
    }
  } catch (error) {
    failure = errorMessage(error);
  }

  revalidatePath("/reviews");
  revalidatePath("/");
  if (failure) redirect(resultUrl("error", failure));
  redirect(resultUrl("result", intent === "confirm" ? "匹配关系已确认并写入审计记录" : "候选关系已拒绝并写入审计记录"));
}
