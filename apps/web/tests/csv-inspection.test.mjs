import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import csv from "../.ingest-tests/csv-inspection.js";
const inspect = (text) => csv.inspectCompanyCSV(new TextEncoder().encode(text));
const header = "source_company_id,company_name";
for (const bom of ["", "\ufeff"]) for (const newline of ["\n", "\r\n"]) {
  test(`CSV ${JSON.stringify(bom + newline)} preserves byte inputs and previews quoted fields`, () => {
    const bytes = new TextEncoder().encode(`${bom}"source_company_id","company_name"${newline}A,"测试,主体"${newline}B,"a""b${newline}c"${newline}`);
    const before = bytes.slice();
    const result = csv.inspectCompanyCSV(bytes);
    assert.equal(result.rowCount, 2); assert.equal(result.hasBOM, !!bom);
    assert.equal(result.preview[0][1], "测试,主体"); assert.equal(result.preview[1][1], `a"b${newline}c`);
    assert.deepEqual(bytes, before);
  });
}
for (const [name, input, message] of [
  ["empty", "", /非空/], ["header only", header, /至少一条/],
  ["missing field", "id,name\na,b", /source_company_id/],
  ["duplicate header", `${header}, company_name\na,b,c`, /重复表头/],
  ["empty header", `${header},\na,b,c`, /表头不能为空/],
  ["ragged row", `${header}\na,b,c`, /第 2 行/],
  ["blank key", `${header}\n ,b`, /source_company_id/],
  ["blank name", `${header}\na,`, /company_name/],
  ["duplicate key", `${header}\na,b\n a ,c`, /重复/],
  ["unclosed quote", `${header}\na,"b`, /没有闭合/],
  ["bare quote", `${header}\na,b"c`, /引号格式/],
  ["trailing quote text", `${header}\na,"b" c`, /引号格式/],
  ["bare CR", `${header}\ra,b`, /LF/],
  ["control byte", `${header}\na,b\u0000`, /控制字符/],
  ["oversized field", `${header}\na,${"b".repeat(4097)}`, /4096/],
  ["too many columns", `${header},${Array.from({length:63},(_,i)=>`col${i}`).join(",")}\na,b`, /64/],
  ["too many rows", `${header}\n${Array.from({length:1001},(_,i)=>`${i},n`).join("\n")}`, /1000/],
]) test(`${name} is rejected`, () => assert.throws(() => inspect(input), message));
test("invalid UTF-8 is rejected, not replacement-decoded", () => assert.throws(() => csv.inspectCompanyCSV(Uint8Array.from([255, 254])), /UTF-8/));
test("oversized bytes are rejected before parsing", () => assert.throws(() => csv.inspectCompanyCSV(new Uint8Array(csv.MAX_CSV_BYTES + 1)), /512/));
test("preview is bounded to five records while every record is validated", () => {
  const result = inspect(`${header}\n${Array.from({length:10},(_,i)=>`${i},n`).join("\n")}`);
  assert.equal(result.preview.length, 5); assert.equal(result.rowCount, 10);
});
test("blank physical lines match Go CSV behavior", () => assert.equal(inspect(`\n${header}\n\na,b\n\n`).rowCount, 1));

// Exercise the exact asset offered by the production console, not a second copy.
test("downloadable synthetic template satisfies the declared CSV format", () => {
  const bytes = readFileSync(new URL("../public/templates/company-import-v1.csv", import.meta.url));
  const original = Buffer.from(bytes);
  const result = csv.inspectCompanyCSV(bytes);
  assert.equal(result.hasBOM, true);
  assert.equal(result.rowCount, 2);
  assert.deepEqual(result.headers, ["source_company_id", "company_name", "unified_social_credit_code", "legal_representative", "registered_address", "entry_date", "company_status"]);
  assert.deepEqual(result.preview.map((row) => row[0]), ["DEMO-001", "DEMO-002"]);
  assert.ok(result.preview.every((row) => row[1].startsWith("合成示例") && row[2] === ""));
  assert.deepEqual(bytes, original);
});
