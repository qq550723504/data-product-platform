import type { PageResult } from "./platform";

/** Console list pages render one server-side page at a time. */
export const LIST_PAGE_SIZE = 25;

// Core rejects offsets beyond the PostgreSQL int4 range.
const MAX_PAGE_OFFSET = 2_147_483_647;

/**
 * Normalize a `?offset=` query value into a safe non-negative integer. Missing,
 * negative, fractional, non-numeric or out-of-range values fall back to the
 * first page instead of failing the request or forwarding garbage to Core.
 */
export function parsePageOffset(value: string | string[] | undefined): number {
  const raw = Array.isArray(value) ? value[0] : value;
  if (typeof raw !== "string") return 0;
  const trimmed = raw.trim();
  if (!/^\d+$/.test(trimmed)) return 0;
  const parsed = Number(trimmed);
  if (!Number.isSafeInteger(parsed) || parsed < 0) return 0;
  return Math.min(parsed, MAX_PAGE_OFFSET);
}

export type PaginationWindow = {
  previousOffset: number | null;
  nextOffset: number | null;
  page: number;
  pageCount: number;
  summary: string;
};

/**
 * Derive explicit pagination controls from the Core-reported total. The next
 * offset advances by the number of rows Core actually returned, so a clamped
 * page size cannot skip records or loop on the same page.
 */
export function paginationWindow(
  offset: number,
  itemCount: number,
  total: number,
  pageSize: number,
): PaginationWindow {
  const safeOffset = Number.isSafeInteger(offset) && offset >= 0 ? offset : 0;
  const safeCount = Number.isSafeInteger(itemCount) && itemCount >= 0 ? itemCount : 0;
  const safeTotal = Number.isSafeInteger(total) && total >= 0 ? total : 0;
  const safeSize = Number.isSafeInteger(pageSize) && pageSize > 0 ? pageSize : LIST_PAGE_SIZE;

  const pageCount = Math.max(1, Math.ceil(safeTotal / safeSize));
  const page = Math.min(pageCount, Math.floor(safeOffset / safeSize) + 1);
  const previousOffset = safeOffset > 0 ? Math.max(0, safeOffset - safeSize) : null;
  const nextOffset = safeCount > 0 && safeOffset + safeCount < safeTotal ? safeOffset + safeCount : null;

  return {
    previousOffset,
    nextOffset,
    page,
    pageCount,
    summary: `第 ${page} / ${pageCount} 页 · 本页 ${safeCount} 条 · 共 ${safeTotal} 条`,
  };
}

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
