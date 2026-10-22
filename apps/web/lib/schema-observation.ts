import { attentionOffset, attentionView, isIdentity } from "./quality-attention";
import { preserveSchemaExceptionQuery } from "./schema-exception-filter";

export type SchemaView = "UNHANDLED" | "ALL";
export type SchemaColumn = { name: string; type: string; constraint: string };
export type SchemaObservation = {
  workspaceId: string; resourceId: string; resourceCode: string; resourceName: string;
  requestId: string; observationId: string; attemptId: string | null; responsibleActorId: string;
  externalFqn: string; ruleRef: string; ruleVersion: string; ruleHash: string; planHash: string;
  status: string; reason: string; schemaHash: string; requestedAt: string;
  completedAt: string | null; recordedAt: string; handlingCount: number;
};
export type SchemaAttentionPage = {
  items: SchemaObservation[];
  page: { limit: number; offset: number; total: number };
  summary: { unhandledCount: number; handledCount: number };
  readAt: string;
};
export type SchemaObservationDetail = SchemaObservation & {
  bindingId: string; externalId: string; connectorHash: string; startedAt: string | null;
  baseline: SchemaColumn[]; columns: SchemaColumn[];
  handling: Array<{ id: string; observationId: string; actorId: string; reason: string; createdAt: string }>;
};

export function schemaView(value?: string): SchemaView { return value === "ALL" ? "ALL" : "UNHANDLED"; }

export function schemaAttentionHref(query: Record<string, string | undefined>, view: SchemaView, offset: number): string {
  const next = new URLSearchParams();
  preserveSchemaExceptionQuery(query, next);
  for (const key of ["reviewOffset", "failureOffset", "failedReleaseOffset", "validatingReleaseOffset", "readyReleaseOffset", "qualityOffset"]) {
    const value = attentionOffset(query[key]);
    if (value) next.set(key, String(value));
  }
  if (query.qualityView) next.set("qualityView", attentionView(query.qualityView));
  next.set("schemaView", view);
  if (offset > 0) next.set("schemaOffset", String(offset));
  return `/attention?${next.toString()}#source-schema-attention`;
}

export function schemaObservationHref(resourceId: string, requestId: string, observationId: string): string {
  if (![resourceId, requestId, observationId].every(isIdentity)) throw new Error("来源观察引用无效");
  return `/resources/${resourceId}?view=source-schema&requestId=${requestId}&observationId=${observationId}`;
}

export function assertSchemaPage(page: SchemaAttentionPage, workspace: string, view: SchemaView, limit: number, offset: number): SchemaAttentionPage {
  const counts = [page.page.total, page.summary.unhandledCount, page.summary.handledCount];
  if (!isIdentity(workspace) || counts.some(n => !Number.isSafeInteger(n) || n < 0)
    || page.page.limit !== limit || page.page.offset !== offset || page.items.length > limit
    || page.page.total !== (view === "ALL" ? page.summary.unhandledCount + page.summary.handledCount : page.summary.unhandledCount)
    || new Set(page.items.map(o => o.observationId)).size !== page.items.length
    || page.items.some(o => o.workspaceId.toLowerCase() !== workspace.toLowerCase() || o.status !== "DRIFT"
      || ![o.resourceId, o.requestId, o.observationId, o.attemptId ?? "", o.responsibleActorId].every(isIdentity)
      || !Number.isSafeInteger(o.handlingCount) || o.handlingCount < 0 || (view === "UNHANDLED" && o.handlingCount !== 0))) {
    throw new Error("来源 schema 漂移汇总不可确认");
  }
  return page;
}

export function assertSchemaObservation(detail: SchemaObservationDetail, workspace: string, resource: string, request: string, observation: string): SchemaObservationDetail {
  if (![workspace, resource, request, observation].every(isIdentity)
    || detail.workspaceId.toLowerCase() !== workspace.toLowerCase()
    || detail.resourceId.toLowerCase() !== resource.toLowerCase()
    || detail.requestId.toLowerCase() !== request.toLowerCase()
    || detail.observationId.toLowerCase() !== observation.toLowerCase()
    || detail.handlingCount !== detail.handling.length
    || detail.handling.some(h => h.observationId.toLowerCase() !== observation.toLowerCase())) {
    throw new Error("观察不属于所选 workspace、资源或扫描请求");
  }
  return detail;
}
