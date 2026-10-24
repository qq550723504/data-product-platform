import "server-only";
import { core, workspace } from "./batches";
import { isIdentity } from "./quality-attention";
import { assertSchemaAttemptPage, type SchemaAttemptPage } from "./schema-attempts";

export async function unobservedSchemaAttempts(offset: number, limit = 25): Promise<SchemaAttemptPage> {
  const ws = workspace(); if (!isIdentity(ws)) throw new Error("Workspace 引用无效");
  const query = new URLSearchParams({ limit: String(limit), offset: String(offset) });
  return assertSchemaAttemptPage(await core<SchemaAttemptPage>(`/workspaces/${ws}/source-schema-requests/observed/unobserved-attempts?${query}`, "GET", undefined, undefined, true), ws, limit, offset);
}
