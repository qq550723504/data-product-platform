import test from "node:test";
import assert from "node:assert/strict";
const { default: lookup } = await import(new URL("../.trace-tests/scoped-lookup.js", import.meta.url).href);
const { findAcrossPages } = lookup;

function paged(source, pageSize = 100) {
  return async (limit, offset) => {
    const items = source.slice(offset, offset + Math.min(limit, pageSize));
    return { items, page: { limit, offset, total: source.length } };
  };
}

test("findAcrossPages resolves a record beyond the first page", async () => {
  const source = Array.from({ length: 250 }, (_, index) => ({ id: `id-${index}` }));
  const target = source[220];
  let calls = 0;
  const found = await findAcrossPages(async (limit, offset) => {
    calls += 1;
    return paged(source)(limit, offset);
  }, (item) => item.id === target.id);
  assert.equal(found.id, target.id);
  assert.equal(calls, 3);
});

test("findAcrossPages returns undefined when the record is absent", async () => {
  const source = Array.from({ length: 150 }, (_, index) => ({ id: `id-${index}` }));
  assert.equal(await findAcrossPages(paged(source), (item) => item.id === "missing"), undefined);
});

test("findAcrossPages keeps product scoping for release lookups", async () => {
  const source = [
    { id: "r-1", productId: "p-other" },
    { id: "r-2", productId: "p-target" },
  ];
  assert.equal(await findAcrossPages(paged(source), (item) => item.id === "r-1" && item.productId === "p-target"), undefined);
  const scoped = await findAcrossPages(paged(source), (item) => item.id === "r-2" && item.productId === "p-target");
  assert.equal(scoped.id, "r-2");
});

test("findAcrossPages keeps draining when Core clamps the page size", async () => {
  const source = Array.from({ length: 45 }, (_, index) => ({ id: `id-${index}` }));
  const found = await findAcrossPages(paged(source, 20), (item) => item.id === "id-44", 100);
  assert.equal(found.id, "id-44");
});

test("findAcrossPages surfaces an empty premature page instead of truncating", async () => {
  await assert.rejects(
    findAcrossPages(async (_limit, offset) => ({ items: [], page: { limit: 100, offset, total: 2 } }), () => false),
    /empty page/,
  );
});
