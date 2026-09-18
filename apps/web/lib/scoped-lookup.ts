import { collectAllPages } from "./pagination";
import type { PageResult } from "./platform";

/**
 * Resolve one record by predicate while draining every page. Scoped detail
 * pages must not assume the record is on the first page: with more than 100
 * products (or 100 releases) a valid URL must still resolve instead of
 * rendering "不存在". `collectAllPages` advances by the returned item count so
 * server-side limit clamping cannot skip immutable history.
 */
export async function findAcrossPages<T>(
  fetchPage: (limit: number, offset: number) => Promise<PageResult<T>>,
  matches: (item: T) => boolean,
  requestedLimit = 100,
): Promise<T | undefined> {
  const items = await collectAllPages(fetchPage, requestedLimit);
  return items.find(matches);
}
