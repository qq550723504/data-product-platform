import test from "node:test";
import assert from "node:assert/strict";
const { default: pagination } = await import(new URL("../.trace-tests/pagination.js", import.meta.url).href);
const { collectAllPages } = pagination;

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
