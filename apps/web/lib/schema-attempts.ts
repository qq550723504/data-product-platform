import { attentionOffset, attentionView, isIdentity } from "./quality-attention";
import { schemaView } from "./schema-observation";
import { preserveSchemaExceptionQuery } from "./schema-exception-filter";
import { preserveSchemaRequestQuery } from "./schema-request-filter";
import type { SchemaRequestItem } from "./schema-requests";

export type SchemaUnobservedAttempt = SchemaRequestItem & {
  attemptId: string; registeredAt: string; requestObservationCount: number;
};
export type SchemaAttemptPage = {
  items: SchemaUnobservedAttempt[]; page: { limit: number; offset: number; total: number }; readAt: string;
};
export function schemaAttemptsHref(query: Record<string, string | undefined>, offset: number): string {
  const next = new URLSearchParams();
  preserveSchemaRequestQuery(query, next);
  preserveSchemaExceptionQuery(query, next);
  for (const key of ["reviewOffset", "failureOffset", "failedReleaseOffset", "validatingReleaseOffset", "readyReleaseOffset", "qualityOffset", "schemaOffset"]) {
    const value = attentionOffset(query[key]); if (value) next.set(key, String(value));
  }
  if (query.qualityView) next.set("qualityView", attentionView(query.qualityView));
  if (query.schemaView) next.set("schemaView", schemaView(query.schemaView));
  next.delete("schemaAttemptOffset");
  if (offset > 0) next.set("schemaAttemptOffset", String(attentionOffset(String(offset))));
  return `/attention?${next}#schema-unobserved-attempts`;
}
export function assertSchemaAttemptPage(p: SchemaAttemptPage, workspace: string, limit: number, offset: number): SchemaAttemptPage {
  const count = (v: number) => Number.isSafeInteger(v) && v >= 0;
  const time = (v: string) => typeof v === "string" && Number.isFinite(Date.parse(v));
  if (!isIdentity(workspace) || !count(p.page.total) || p.page.limit !== limit || p.page.offset !== offset
    || !time(p.readAt) || p.items.length !== Math.min(limit, Math.max(0, p.page.total - offset))
    || new Set(p.items.map(a => a.attemptId)).size !== p.items.length
    || p.items.some(a => ![a.workspaceId, a.resourceId, a.requestId, a.attemptId, a.responsibleActorId].every(isIdentity)
      || a.workspaceId.toLowerCase() !== workspace.toLowerCase() || !time(a.requestedAt) || !time(a.registeredAt)
      || !count(a.attemptCount) || a.attemptCount === 0 || !count(a.requestObservationCount) || a.requestObservationCount === 0)) {
    throw new Error("已有观察请求中的无观察 attempt 汇总不可确认");
  }
  return p;
}
