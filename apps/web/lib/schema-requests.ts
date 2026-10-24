import { attentionOffset, attentionView, isIdentity } from "./quality-attention";
import { schemaView, schemaObservationHref, type SchemaColumn } from "./schema-observation";
import { preserveSchemaExceptionQuery } from "./schema-exception-filter";
import { schemaRequestView, type SchemaRequestView } from "./schema-request-filter";
import { preserveSchemaAttemptQuery } from "./schema-attempt-filter";
import { schemaAttemptsHref } from "./schema-attempts";

export type SchemaRequestOrigin = ({ kind: "REQUESTS"; view: SchemaRequestView; offset: number } | { kind: "OBSERVED_ATTEMPTS"; offset: number }) & { context?: Record<string, string | undefined> };
export function schemaRequestOrigin(query: Record<string, string | undefined>): SchemaRequestOrigin {
  if (query.schemaEvidenceOrigin === "OBSERVED_ATTEMPTS") return { kind: "OBSERVED_ATTEMPTS", offset: attentionOffset(query.schemaAttemptOffset), context: query };
  if (query.schemaEvidenceOrigin !== undefined && query.schemaEvidenceOrigin !== "REQUESTS") throw new Error("证据返回来源无效");
  return { kind: "REQUESTS", view: schemaRequestView(query.schemaRequestView), offset: attentionOffset(query.schemaRequestOffset), context: query };
}
function originQuery(origin: SchemaRequestOrigin): URLSearchParams {
  // Only the existing bounded attention helpers may carry context through evidence.
  const list = schemaRequestListHref(origin);
  const query = new URLSearchParams(list.split("?")[1].split("#")[0]);
  if (origin.kind === "OBSERVED_ATTEMPTS") {
    query.set("schemaEvidenceOrigin", "OBSERVED_ATTEMPTS");
    query.set("schemaAttemptOffset", String(attentionOffset(String(origin.offset))));
  } else {
    query.set("schemaRequestView", schemaRequestView(origin.view));
    query.set("schemaRequestOffset", String(attentionOffset(String(origin.offset))));
  }
  return query;
}
export function schemaRequestListHref(origin: SchemaRequestOrigin): string {
  return origin.kind === "OBSERVED_ATTEMPTS" ? schemaAttemptsHref(origin.context ?? {}, origin.offset) : schemaRequestsHref(origin.context ?? {}, origin.view, origin.offset);
}

export type SchemaRequestItem = {
  workspaceId: string; resourceId: string; resourceCode: string; resourceName: string; requestId: string;
  responsibleActorId: string; externalFqn: string; ruleRef: string; ruleVersion: string; ruleHash: string;
  planHash: string; requestedAt: string; attemptCount: number;
};
type Page = { limit: number; offset: number; total: number };
export type SchemaRequestPage = {
  items: SchemaRequestItem[]; page: Page;
  summary: { noAttemptCount: number; attemptRecordedCount: number }; readAt: string;
};
export type SchemaRequestAttempt = { id: string; workspaceId: string; requestId: string; registeredAt: string; observationId: string | null };
export type SchemaRequestObservation = { id: string; requestId: string; attemptId: string | null; status: string; reason: string; recordedAt: string };
export type SchemaRequestDetail = SchemaRequestItem & {
  bindingId: string; externalId: string; connectorHash: string; baseline: SchemaColumn[];
  selectedAttempt: SchemaRequestAttempt | null;
  attempts: { items: SchemaRequestAttempt[]; page: Page };
  observations: { items: SchemaRequestObservation[]; page: Page }; readAt: string;
};
const count = (n: number) => Number.isSafeInteger(n) && n >= 0;
const time = (v: string) => typeof v === "string" && Number.isFinite(Date.parse(v));
const same = (a: string, b: string) => a.toLowerCase() === b.toLowerCase();
function validItem(item: SchemaRequestItem, workspace: string): boolean {
  return [item.workspaceId, item.resourceId, item.requestId, item.responsibleActorId].every(isIdentity)
    && same(item.workspaceId, workspace) && count(item.attemptCount) && time(item.requestedAt);
}
function validPage(page: Page, length: number, limit: number, offset: number): boolean {
  return count(page.total) && page.limit === limit && page.offset === offset
    && length === Math.min(limit, Math.max(0, page.total - offset));
}
export function schemaRequestsHref(query: Record<string, string | undefined>, view: SchemaRequestView, offset: number): string {
  const next = new URLSearchParams();
  preserveSchemaAttemptQuery(query, next);
  preserveSchemaExceptionQuery(query, next);
  for (const key of ["reviewOffset", "failureOffset", "failedReleaseOffset", "validatingReleaseOffset", "readyReleaseOffset", "qualityOffset", "schemaOffset"]) {
    const value = attentionOffset(query[key]); if (value) next.set(key, String(value));
  }
  if (query.qualityView) next.set("qualityView", attentionView(query.qualityView));
  if (query.schemaView) next.set("schemaView", schemaView(query.schemaView));
  next.set("schemaRequestView", schemaRequestView(view));
  if (offset > 0) next.set("schemaRequestOffset", String(attentionOffset(String(offset))));
  return `/attention?${next}#schema-unobserved-requests`;
}
export function schemaRequestHref(resource: string, request: string, origin: SchemaRequestOrigin, attempt?: string, attemptOffset = 0, observationOffset = 0): string {
  if (![resource, request].every(isIdentity) || (attempt !== undefined && !isIdentity(attempt))) throw new Error("精确请求/attempt 引用无效");
  const query = new URLSearchParams({ view: "source-schema-request", requestId: request });
  originQuery(origin).forEach((value, key) => query.set(key, value));
  if (attempt !== undefined) query.set("attemptId", attempt);
  if (attemptOffset) query.set("attemptOffset", String(attentionOffset(String(attemptOffset))));
  if (observationOffset) query.set("observationOffset", String(attentionOffset(String(observationOffset))));
  return `/resources/${resource}?${query}`;
}
export function assertSchemaRequestPage(p: SchemaRequestPage, workspace: string, view: SchemaRequestView, limit: number, offset: number): SchemaRequestPage {
  const { noAttemptCount, attemptRecordedCount } = p.summary;
  const total = view === "ALL" ? noAttemptCount + attemptRecordedCount : view === "NO_ATTEMPT" ? noAttemptCount : attemptRecordedCount;
  if (!isIdentity(workspace) || ![noAttemptCount, attemptRecordedCount].every(count) || p.page.total !== total
    || !validPage(p.page, p.items.length, limit, offset) || !time(p.readAt)
    || new Set(p.items.map(r => r.requestId)).size !== p.items.length
    || p.items.some(r => !validItem(r, workspace) || (view === "NO_ATTEMPT" && r.attemptCount !== 0) || (view === "ATTEMPT_RECORDED" && r.attemptCount === 0))) throw new Error("无观察请求汇总不可确认");
  return p;
}
export function schemaRequestObservationHref(resource: string, request: string, observation: string, origin: SchemaRequestOrigin, selectedAttempt: string | undefined, attemptOffset: number, observationOffset: number): string {
  if (selectedAttempt !== undefined && !isIdentity(selectedAttempt)) throw new Error("精确返回 attempt 引用无效");
  const query = new URLSearchParams({ requestEvidence: "1" });
  originQuery(origin).forEach((value, key) => query.set(key, value));
  if (selectedAttempt !== undefined) query.set("requestAttemptId", selectedAttempt);
  query.set("attemptOffset", String(attentionOffset(String(attemptOffset))));
  query.set("observationOffset", String(attentionOffset(String(observationOffset))));
  return `${schemaObservationHref(resource, request, observation)}&${query}`;
}
export function assertSchemaRequestDetail(d: SchemaRequestDetail, workspace: string, resource: string, request: string, attempt: string | undefined, attemptOffset: number, observationOffset: number): SchemaRequestDetail {
  const validAttempt = (a: SchemaRequestAttempt) => [a.id, a.workspaceId, a.requestId].every(isIdentity)
    && same(a.workspaceId, workspace) && same(a.requestId, request) && time(a.registeredAt) && (a.observationId === null || isIdentity(a.observationId));
  if (![workspace, resource, request].every(isIdentity) || !validItem(d, workspace) || !same(d.resourceId, resource) || !same(d.requestId, request)
    || !isIdentity(d.bindingId) || !time(d.readAt) || !Array.isArray(d.baseline)
    || d.attemptCount !== d.attempts.page.total || !validPage(d.attempts.page, d.attempts.items.length, 25, attemptOffset)
    || !validPage(d.observations.page, d.observations.items.length, 25, observationOffset)
    || new Set(d.attempts.items.map(a => a.id)).size !== d.attempts.items.length
    || new Set(d.observations.items.map(o => o.id)).size !== d.observations.items.length
    || d.attempts.items.some(a => !validAttempt(a))
    || (attempt === undefined ? d.selectedAttempt !== null : !isIdentity(attempt) || !d.selectedAttempt || !validAttempt(d.selectedAttempt) || !same(d.selectedAttempt.id, attempt))
    || d.observations.items.some(o => !isIdentity(o.id) || !same(o.requestId, request) || (o.attemptId !== null && !isIdentity(o.attemptId))
      || !["MATCH", "DRIFT", "UNAVAILABLE", "DENIED", "INTERRUPTED"].includes(o.status) || !time(o.recordedAt))) throw new Error("请求/attempt 不属于所选 workspace、资源或扫描请求");
  return d;
}
