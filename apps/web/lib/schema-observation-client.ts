import "server-only";
import { core, workspace } from "./batches";
import { isIdentity } from "./quality-attention";
import { assertSchemaObservation, assertSchemaPage, type SchemaAttentionPage, type SchemaObservationDetail, type SchemaView } from "./schema-observation";

export async function schemaAttention(view: SchemaView, offset: number, limit = 25): Promise<SchemaAttentionPage> {
  const ws = workspace();
  if (!isIdentity(ws)) throw new Error("Workspace 引用无效");
  const query = new URLSearchParams({ view, limit: String(limit), offset: String(offset) });
  const result = await core<SchemaAttentionPage>(`/workspaces/${ws}/source-schema-observations/attention?${query}`, "GET", undefined, undefined, true);
  return assertSchemaPage(result, ws, view, limit, offset);
}

export async function schemaObservation(resource: string, request: string, observation: string): Promise<SchemaObservationDetail> {
  if (![resource, request, observation].every(isIdentity)) throw new Error("完整的精确观察引用必填");
  const ws = workspace();
  if (!isIdentity(ws)) throw new Error("Workspace 引用无效");
  const result = await core<SchemaObservationDetail>(`/data-resources/${resource}/schema-observations/${observation}?workspaceId=${ws}`, "GET", undefined, undefined, true);
  return assertSchemaObservation(result, ws, resource, request, observation);
}
