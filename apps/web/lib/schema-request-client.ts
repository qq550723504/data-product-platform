import "server-only";
import { core, workspace } from "./batches";
import { isIdentity } from "./quality-attention";
import { assertSchemaRequestPage, assertSchemaRequestDetail, type SchemaRequestPage, type SchemaRequestDetail } from "./schema-requests";
import type { SchemaRequestView } from "./schema-request-filter";

export async function unobservedSchemaRequests(view: SchemaRequestView, offset: number, limit = 25): Promise<SchemaRequestPage> {
  const ws = workspace(); if (!isIdentity(ws)) throw new Error("Workspace 引用无效");
  const query = new URLSearchParams({ view, limit: String(limit), offset: String(offset) });
  return assertSchemaRequestPage(await core<SchemaRequestPage>(`/workspaces/${ws}/source-schema-requests/unobserved?${query}`, "GET", undefined, undefined, true), ws, view, limit, offset);
}
export async function schemaRequestDetail(resource: string, request: string, attempt: string | undefined, attemptOffset: number, observationOffset: number): Promise<SchemaRequestDetail> {
  const ws = workspace();
  if (![ws, resource, request].every(isIdentity) || (attempt !== undefined && !isIdentity(attempt))) throw new Error("完整的精确请求/attempt 引用必填");
  const query = new URLSearchParams({ workspaceId: ws, limit: "25", attemptOffset: String(attemptOffset), observationOffset: String(observationOffset) });
  if (attempt !== undefined) query.set("attemptId", attempt);
  const d = await core<SchemaRequestDetail>(`/data-resources/${resource}/schema-requests/${request}?${query}`, "GET", undefined, undefined, true);
  return assertSchemaRequestDetail(d, ws, resource, request, attempt, attemptOffset, observationOffset);
}
