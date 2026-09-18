import test from "node:test";
import assert from "node:assert/strict";
const { default: pagination } = await import(new URL("../.trace-tests/pagination.js", import.meta.url).href);
const { collectAllPages, parsePageOffset, paginationWindow, LIST_PAGE_SIZE } = pagination;

test("collectAllPages drains every page using returned item count", async () => {
  const source = Array.from({ length: 205 }, (_, index) => ({ id: index + 1 }));
  const calls = [];
  const items = await collectAllPages(async (limit, offset) => {
    calls.push({ limit, offset });
    // Simulate a server that clamps the requested 100 to 60.
    const pageItems = source.slice(offset, offset + 60);
    return { items: pageItems, page: { limit: 60, offset, total: source.length } };
  }, 100);
  assert.deepEqual(items, source);
  assert.deepEqual(calls.map((call) => call.offset), [0, 60, 120, 180]);
});

test("collectAllPages accepts an empty collection", async () => {
  const items = await collectAllPages(async (_limit, offset) => ({ items: [], page: { limit: 100, offset, total: 0 } }));
  assert.deepEqual(items, []);
});

test("collectAllPages rejects an empty page before reported total", async () => {
  await assert.rejects(
    collectAllPages(async (_limit, offset) => ({ items: [], page: { limit: 100, offset, total: 2 } })),
    /reported 2 items but returned an empty page/,
  );
});

test("collectAllPages rejects an unexpected offset", async () => {
  await assert.rejects(
    collectAllPages(async () => ({ items: [{ id: 1 }], page: { limit: 100, offset: 1, total: 1 } })),
    /unexpected page offset/,
  );
});

test("collectAllPages rejects invalid limits before calling Core", async () => {
  let called = false;
  await assert.rejects(
    collectAllPages(async () => { called = true; return { items: [], page: { limit: 1, offset: 0, total: 0 } }; }, 0),
    /positive integer/,
  );
  assert.equal(called, false);
});

test("parsePageOffset normalizes query values and rejects garbage", () => {
  assert.equal(parsePageOffset(undefined), 0);
  assert.equal(parsePageOffset(""), 0);
  assert.equal(parsePageOffset("0"), 0);
  assert.equal(parsePageOffset("25"), 25);
  assert.equal(parsePageOffset(" 50 "), 50);
  assert.equal(parsePageOffset(["75", "9"]), 75);
  assert.equal(parsePageOffset("-1"), 0);
  assert.equal(parsePageOffset("1.5"), 0);
  assert.equal(parsePageOffset("1e3"), 0);
  assert.equal(parsePageOffset("abc"), 0);
  assert.equal(parsePageOffset("999999999999999999999"), 0);
  assert.equal(parsePageOffset("999999999999"), 2147483647);
  assert.equal(parsePageOffset("4294967296"), 2147483647);
});

test("paginationWindow exposes explicit prev/next offsets", () => {
  assert.equal(LIST_PAGE_SIZE, 25);
  const first = paginationWindow(0, 25, 205, LIST_PAGE_SIZE);
  assert.deepEqual(
    { previousOffset: first.previousOffset, nextOffset: first.nextOffset, page: first.page, pageCount: first.pageCount },
    { previousOffset: null, nextOffset: 25, page: 1, pageCount: 9 },
  );
  assert.equal(first.summary, "第 1 / 9 页 · 本页 25 条 · 共 205 条");

  const middle = paginationWindow(50, 25, 205, LIST_PAGE_SIZE);
  assert.deepEqual(
    { previousOffset: middle.previousOffset, nextOffset: middle.nextOffset, page: middle.page },
    { previousOffset: 25, nextOffset: 75, page: 3 },
  );

  const last = paginationWindow(200, 5, 205, LIST_PAGE_SIZE);
  assert.deepEqual(
    { previousOffset: last.previousOffset, nextOffset: last.nextOffset, page: last.page, pageCount: last.pageCount },
    { previousOffset: 175, nextOffset: null, page: 9, pageCount: 9 },
  );
});

test("paginationWindow advances by returned rows and never loops on an empty page", () => {
  // Core clamps a requested 100 down to 60: advance by what came back, not by pageSize.
  const clamped = paginationWindow(60, 60, 205, 100);
  assert.equal(clamped.nextOffset, 120);
  assert.equal(clamped.previousOffset, 0);

  // An out-of-range offset returns no rows: no next link, and the page never exceeds pageCount.
  const stale = paginationWindow(500, 0, 205, LIST_PAGE_SIZE);
  assert.equal(stale.nextOffset, null);
  assert.equal(stale.previousOffset, 475);
  assert.equal(stale.page, 9);
  assert.equal(stale.pageCount, 9);
});

test("paginationWindow tolerates invalid metadata without throwing", () => {
  const empty = paginationWindow(0, 0, 0, 0);
  assert.deepEqual(
    { previousOffset: empty.previousOffset, nextOffset: empty.nextOffset, page: empty.page, pageCount: empty.pageCount },
    { previousOffset: null, nextOffset: null, page: 1, pageCount: 1 },
  );
  const negative = paginationWindow(-5, -1, -3, LIST_PAGE_SIZE);
  assert.equal(negative.previousOffset, null);
  assert.equal(negative.nextOffset, null);
  assert.equal(negative.summary, "第 1 / 1 页 · 本页 0 条 · 共 0 条");
});
