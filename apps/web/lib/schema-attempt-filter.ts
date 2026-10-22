export function preserveSchemaAttemptQuery(query: Record<string, string | undefined>, next: URLSearchParams): void {
  const offset = Number(query.schemaAttemptOffset ?? 0);
  if (Number.isSafeInteger(offset) && offset > 0 && offset <= 2147483647) next.set("schemaAttemptOffset", String(offset));
}
