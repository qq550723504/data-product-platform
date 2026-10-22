import "server-only";
import { core, workspace } from "./batches";
import { isIdentity } from "./quality-attention";
import { assertSchemaExceptionPage, type SchemaExceptionPage } from "./schema-exceptions";
import type { SchemaExceptionFilter } from "./schema-exception-filter";

export async function schemaExceptions(status: SchemaExceptionFilter, offset: number, limit = 25): Promise<SchemaExceptionPage> {
  const ws = workspace();
  if (!isIdentity(ws)) throw new Error("Workspace 引用无效");
  const query = new URLSearchParams({ status, limit: String(limit), offset: String(offset) });
  const result = await core<SchemaExceptionPage>(`/workspaces/${ws}/source-schema-observations/exceptions?${query}`, "GET", undefined, undefined, true);
  return assertSchemaExceptionPage(result, ws, status, limit, offset);
}
