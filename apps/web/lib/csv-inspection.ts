/** Bounded, strict CSV inspection. It never rewrites the uploaded bytes. */
export const MAX_CSV_BYTES = 512 * 1024;
export const MAX_CSV_ROWS = 1000;
export type CSVInspection = { headers: string[]; rowCount: number; preview: string[][]; hasBOM: boolean };
export class CSVError extends Error {
  constructor(message: string, readonly line: number, readonly column: number) {
    super(`第 ${line} 行，第 ${column} 列：${message}`);
    this.name = "CSVError";
  }
}
export function inspectCompanyCSV(bytes: Uint8Array): CSVInspection {
  if (!bytes.length || bytes.length > MAX_CSV_BYTES) throw new Error("文件必须非空且不超过 512 KiB。");
  let text: string;
  try { text = new TextDecoder("utf-8", { fatal: true }).decode(bytes); }
  catch { throw new Error("仅支持 UTF-8 CSV；请先转换编码，不要直接上传 Excel 工作簿。"); }
  const hasBOM = bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf;
  if (/[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/.test(text)) throw new Error("文件包含不支持的控制字符。");
  let line = 1, column = 1, startLine = 1, field = "", quoted = false, afterQuote = false, started = false;
  let fields: string[] = [], headers: string[] | undefined, rowCount = 0;
  const preview: string[][] = [], keys = new Set<string>();
  const fail = (message: string): never => { throw new CSVError(message, line, column); };
  const pushField = () => {
    if (field.length > 4096) fail("单个字段不能超过 4096 个字符。");
    fields.push(field); field = ""; afterQuote = false;
    if (fields.length > 64) fail("字段数量不能超过 64。");
  };
  const pushRow = () => {
    if (!started && !fields.length && !field && !afterQuote) { startLine = line + 1; return; }
    pushField();
    if (!headers) {
      headers = fields.map((value) => value.trim());
      if (headers.some((value) => !value)) fail("表头不能为空。");
      if (new Set(headers).size !== headers.length) fail("存在重复表头。");
      for (const required of ["source_company_id", "company_name"]) {
        if (!headers.includes(required)) fail(`缺少必需字段 ${required}。`);
      }
    } else {
      if (fields.length !== headers.length) throw new CSVError(`应有 ${headers.length} 个字段，实际 ${fields.length} 个。`, startLine, 1);
      for (const required of ["source_company_id", "company_name"]) {
        if (!fields[headers.indexOf(required)].trim()) throw new CSVError(`${required} 不能为空。`, startLine, headers.indexOf(required) + 1);
      }
      const key = fields[headers.indexOf("source_company_id")].trim();
      if (keys.has(key)) throw new CSVError("source_company_id 重复。", startLine, headers.indexOf("source_company_id") + 1);
      keys.add(key);
      rowCount++;
      if (rowCount > MAX_CSV_ROWS) fail("本阶段最多接入 1000 条记录，请拆分文件。");
      if (preview.length < 5) preview.push(fields);
    }
    fields = []; started = false; startLine = line + 1;
  };
  for (let i = 0; i < text.length; i++, column++) {
    const c = text[i];
    if (quoted) {
      if (c === '"') {
        if (text[i + 1] === '"') { field += '"'; i++; column++; }
        else { quoted = false; afterQuote = true; }
      } else { field += c; if (c === "\n") { line++; column = 0; } }
    } else if (c === ",") { pushField(); started = true; }
    else if (c === "\n" || c === "\r") {
      if (c === "\r") { if (text[i + 1] !== "\n") fail("换行请使用 LF 或 CRLF。"); i++; }
      pushRow(); line++; column = 0;
    } else if (c === '"' && !field && !afterQuote) { quoted = true; started = true; }
    else {
      if (afterQuote || c === '"') fail("引号格式无效；字段中的引号应写成两个双引号。");
      field += c; started = true;
    }
    if (field.length > 4096) fail("单个字段不能超过 4096 个字符。");
  }
  if (quoted) fail("带引号的字段没有闭合。");
  pushRow();
  if (!headers || !rowCount) throw new Error("CSV 必须包含表头和至少一条数据。");
  return { headers, rowCount, preview, hasBOM };
}
