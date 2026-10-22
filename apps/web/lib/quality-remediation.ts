import "server-only";
import { core, workspace } from "./batches";
import { isIdentity, type AttentionView, type QualityAttentionPage } from "./quality-attention";

export async function qualityAttention(view: AttentionView, offset: number, limit = 25): Promise<QualityAttentionPage> {
  const ws = workspace();
  const query = new URLSearchParams({ view, limit: String(limit), offset: String(offset) });
  const result = await core<QualityAttentionPage>(
    `/workspaces/${encodeURIComponent(ws)}/quality-remediations/attention?${query}`, "GET", undefined, undefined, "human-decision",
  );
  const counts = [result.page.total, result.summary.followUpCount, result.summary.closedCount];
  if (result.page.limit !== limit || result.page.offset !== offset
    || counts.some(n => !Number.isSafeInteger(n) || n < 0) || result.items.length > limit
    || result.items.some(item => item.workspaceId.toLowerCase() !== ws.toLowerCase()
      || ![item.id, item.datasetId, item.versionId, item.assessmentId].every(isIdentity))) {
    throw new Error("质量异常汇总不可确认");
  }
  return result;
}
