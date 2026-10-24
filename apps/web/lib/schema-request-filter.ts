import { preserveSchemaAttemptQuery } from "./schema-attempt-filter";

export type SchemaRequestView = "ALL" | "NO_ATTEMPT" | "ATTEMPT_RECORDED";
export function schemaRequestView(value?: string): SchemaRequestView {
  return value === "NO_ATTEMPT" || value === "ATTEMPT_RECORDED" ? value : "ALL";
}
export function preserveSchemaRequestQuery(query: Record<string, string | undefined>, next: URLSearchParams): void {
  preserveSchemaAttemptQuery(query, next);
  if (query.schemaRequestView !== undefined) next.set("schemaRequestView", schemaRequestView(query.schemaRequestView));
  const offset = Number(query.schemaRequestOffset ?? 0);
  if (Number.isSafeInteger(offset) && offset > 0 && offset <= 2147483647) next.set("schemaRequestOffset", String(offset));
}
