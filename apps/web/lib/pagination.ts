import type { PageResult } from "./platform";

/**
 * Drain an offset/limit Core read-model endpoint without silently truncating
 * immutable history. The helper advances by the number of records actually
 * returned so server-side limit clamping cannot create gaps.
 */
export async function collectAllPages<T>(
  fetchPage: (limit: number, offset: number) => Promise<PageResult<T>>,
  requestedLimit = 100,
): Promise<T[]> {
  if (!Number.isInteger(requestedLimit) || requestedLimit <= 0) {
    throw new Error("pagination limit must be a positive integer");
  }

  const items: T[] = [];
  let offset = 0;
  let pages = 0;

  for (;;) {
    pages += 1;
    if (pages > 100_000) {
      throw new Error("pagination did not converge");
    }

    const page = await fetchPage(requestedLimit, offset);
    if (!page || !Array.isArray(page.items) || !page.page || !Number.isFinite(page.page.total) || page.page.total < 0) {
      throw new Error("Core API returned invalid pagination metadata");
    }
    if (page.page.offset !== offset) {
      throw new Error(`Core API returned unexpected page offset ${page.page.offset}, expected ${offset}`);
    }

    items.push(...page.items);
    if (items.length >= page.page.total) {
      return items;
    }
    if (page.items.length === 0) {
      throw new Error(`Core API reported ${page.page.total} items but returned an empty page at offset ${offset}`);
    }
    offset += page.items.length;
  }
}
