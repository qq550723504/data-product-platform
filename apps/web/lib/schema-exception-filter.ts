export const schemaExceptionStatuses = ["UNAVAILABLE", "DENIED", "INTERRUPTED"] as const;
export type SchemaExceptionStatus = typeof schemaExceptionStatuses[number];
export type SchemaExceptionFilter = "ALL" | SchemaExceptionStatus;

export function isSchemaExceptionStatus(value: string): value is SchemaExceptionStatus {
  return schemaExceptionStatuses.some(status => status === value);
}

export function schemaExceptionFilter(value?: string): SchemaExceptionFilter {
  return value && isSchemaExceptionStatus(value) ? value : "ALL";
}

// Shared by the existing queue links so changing their filter does not discard
// the independent history selection. No caller-controlled return URL is used.
export function preserveSchemaExceptionQuery(query: Record<string, string | undefined>, next: URLSearchParams): void {
  if (query.schemaExceptionStatus !== undefined) next.set("schemaExceptionStatus", schemaExceptionFilter(query.schemaExceptionStatus));
  const offset = Number(query.schemaExceptionOffset ?? 0);
  if (Number.isSafeInteger(offset) && offset > 0 && offset <= 2147483647) next.set("schemaExceptionOffset", String(offset));
}
