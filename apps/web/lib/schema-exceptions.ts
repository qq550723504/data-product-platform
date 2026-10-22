import { attentionOffset, attentionView, isIdentity } from "./quality-attention";
import { schemaObservationHref, schemaView, type SchemaObservation } from "./schema-observation";
import { isSchemaExceptionStatus, schemaExceptionFilter, type SchemaExceptionFilter, type SchemaExceptionStatus } from "./schema-exception-filter";

export type SchemaException = Omit<SchemaObservation, "handlingCount" | "status"> & {
  status: SchemaExceptionStatus;
  startedAt: string | null;
};
export type SchemaExceptionPage = {
  items: SchemaException[];
  page: { limit: number; offset: number; total: number };
  summary: { unavailableCount: number; deniedCount: number; interruptedCount: number };
  readAt: string;
};

export function schemaExceptionsHref(query: Record<string, string | undefined>, status: SchemaExceptionFilter, offset: number): string {
  const next = new URLSearchParams();
  for (const key of ["reviewOffset", "failureOffset", "failedReleaseOffset", "validatingReleaseOffset", "readyReleaseOffset", "qualityOffset", "schemaOffset"]) {
    const value = attentionOffset(query[key]);
    if (value) next.set(key, String(value));
  }
  if (query.qualityView) next.set("qualityView", attentionView(query.qualityView));
  if (query.schemaView) next.set("schemaView", schemaView(query.schemaView));
  next.set("schemaExceptionStatus", schemaExceptionFilter(status));
  if (offset > 0) next.set("schemaExceptionOffset", String(offset));
  return `/attention?${next.toString()}#schema-exception-history`;
}

export function schemaExceptionEvidenceHref(item: SchemaException, status: SchemaExceptionFilter, offset: number): string {
  return `${schemaObservationHref(item.resourceId, item.requestId, item.observationId)}&schemaExceptionStatus=${status}&schemaExceptionOffset=${offset}`;
}

export function assertSchemaExceptionPage(page: SchemaExceptionPage, workspace: string, status: SchemaExceptionFilter, limit: number, offset: number): SchemaExceptionPage {
  const summary = page.summary;
  const counts = [summary.unavailableCount, summary.deniedCount, summary.interruptedCount];
  const expected = status === "ALL" ? counts.reduce((a, b) => a + b, 0)
    : status === "UNAVAILABLE" ? summary.unavailableCount : status === "DENIED" ? summary.deniedCount : summary.interruptedCount;
  const validTime = (value: string | null) => value === null || (typeof value === "string" && Number.isFinite(Date.parse(value)));
  if (!isIdentity(workspace) || [...counts, page.page.total].some(n => !Number.isSafeInteger(n) || n < 0)
    || page.page.total !== expected || page.page.limit !== limit || page.page.offset !== offset
    || page.items.length !== Math.min(limit, Math.max(0, page.page.total - offset))
    || !validTime(page.readAt) || page.readAt === null
    || new Set(page.items.map(o => o.observationId)).size !== page.items.length
    || page.items.some(o => o.workspaceId.toLowerCase() !== workspace.toLowerCase()
      || !isSchemaExceptionStatus(o.status) || (status !== "ALL" && o.status !== status)
      || ![o.resourceId, o.requestId, o.observationId, o.responsibleActorId].every(isIdentity)
      || (o.attemptId !== null && !isIdentity(o.attemptId))
      || !validTime(o.startedAt) || !validTime(o.completedAt) || !validTime(o.recordedAt) || o.recordedAt === null)) {
    throw new Error("来源 schema 扫描异常历史不可确认");
  }
  return page;
}
