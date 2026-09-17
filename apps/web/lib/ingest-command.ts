import { inspectCompanyCSV, MAX_CSV_BYTES } from "./csv-inspection";

export type IngestConfig = { enabled: boolean; workspaceId?: string | null; actorId?: string | null; apiBaseUrl: string };
export type IngestState = {
  ok: boolean; message: string; locked?: boolean; operationId?: string;
  resourceId?: string; datasetId?: string; versionId?: string; outputDatasetId?: string;
  checksum?: string; rowCount?: number; jobId?: string; jobStatus?: string;
};
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
function isId(value: unknown): value is string { return typeof value === "string" && uuid.test(value) && value !== "00000000-0000-0000-0000-000000000000"; }
function same(a: unknown, b: string) { return isId(a) && a.toLowerCase() === b.toLowerCase(); }
function object(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("Core 返回格式无效，请核对已创建对象。");
  return value as Record<string, unknown>;
}
export function ingestConfigurationError(config: IngestConfig): string | null {
  if (!config.enabled) return "数据接入写入默认关闭；仅在受信任环境启用 POC_ENABLE_INGEST_ACTIONS。";
  if (!isId(config.workspaceId) || !isId(config.actorId)) return "需要有效的服务端工作区和 POC_INGEST_ACTOR_ID；浏览器不能指定操作人。";
  return null;
}
function configured(config: IngestConfig) {
  const error = ingestConfigurationError(config); if (error) throw new Error(error);
  return { workspaceId: config.workspaceId as string, actorId: config.actorId as string };
}
function transport(config: IngestConfig, request: typeof fetch, beforeWrite: () => void) {
  return async (path: string, body?: Record<string, unknown> | FormData): Promise<Record<string, unknown>> => {
    const multipart = body instanceof FormData;
    if (body) beforeWrite();
    let response: Response;
    try {
      response = await request(`${config.apiBaseUrl.replace(/\/$/, "")}${path}`, {
        method: body ? "POST" : "GET", cache: "no-store", redirect: "error", signal: AbortSignal.timeout(60000),
        headers: { Accept: "application/json", ...(body ? { "X-Actor-ID": config.actorId as string } : {}), ...(!multipart && body ? { "Content-Type": "application/json" } : {}) },
        body: multipart ? body : body ? JSON.stringify(body) : undefined,
      });
    } catch { throw new Error("暂时无法确认 Core 请求结果。"); }
    if (!response.ok) throw new Error(`Core 未确认操作成功（HTTP ${response.status}）。`);
    try { return object(await response.json()); } catch { throw new Error("Core 返回无法核实的响应。"); }
  };
}
type JSONCall = ReturnType<typeof transport>;
async function datasets(json: JSONCall, workspaceId: string): Promise<Record<string, unknown>[]> {
  const all: Record<string, unknown>[] = [];
  for (let page = 0; page < 100; page++) {
    const payload = await json(`/api/v1/workspaces/${workspaceId}/datasets?limit=100&offset=${all.length}`);
    if (!Array.isArray(payload.items)) throw new Error("工作区数据集列表格式无效。");
    const meta = object(payload.page);
    if (!Number.isSafeInteger(meta.total) || (meta.total as number) < 0 || meta.offset !== all.length) throw new Error("工作区分页信息无效。");
    const items = payload.items.map(object);
    if (items.some((item) => !same(item.workspaceId, workspaceId))) throw new Error("数据集不属于当前工作区。");
    all.push(...items);
    if (all.length >= (meta.total as number)) return all;
    if (!items.length) throw new Error("数据集分页不完整，已阻止操作。");
  }
  throw new Error("数据集数量超出本阶段范围，已阻止操作。");
}
function created(value: Record<string, unknown>, workspaceId: string, kind: string, code: string): string {
  if (!isId(value.id) || !same(value.workspaceId, workspaceId) || value.code !== code || (value.datasetType ?? value.resourceType) !== kind) throw new Error("Core 创建结果的工作区、类型或关联标识不一致。");
  return value.id;
}
function failure(error: unknown, state: IngestState, attempted: boolean): IngestState {
  const detail = error instanceof Error ? error.message : "操作结果无法确认。";
  return { ...state, ok: false, locked: attempted || state.locked, message: attempted ? `${detail} 已发送过写入，可能部分成功。请根据下方标识核对资源、版本或审核队列，不要直接重新提交；不会自动重试或删除历史对象。` : detail };
}
export async function importCSV(operationId: string, form: FormData, config: IngestConfig, request: typeof fetch = fetch): Promise<IngestState> {
  let attempted = false;
  const state: IngestState = { ok: false, message: "", operationId };
  try {
    const { workspaceId } = configured(config);
    if (!isId(operationId)) throw new Error("无效接入操作标识。");
    operationId = operationId.toLowerCase(); state.operationId = operationId;
    const name = form.get("name"), source = form.get("source"), purpose = form.get("purpose"), file = form.get("file");
    for (const [label, value, max] of [["名称", name, 120], ["来源", source, 500], ["用途", purpose, 500]] as const) {
      if (typeof value !== "string" || !value.trim() || value.length > max) throw new Error(`${label}必填，且不能超过 ${max} 个字符。`);
    }
    if (form.get("acknowledged") !== "on") throw new Error("请确认数据来源和本次处理权限；此声明不替代正式授权。");
    if (!(file instanceof Blob) || file.size === 0 || file.size > MAX_CSV_BYTES) throw new Error("请选择非空 CSV，最大 512 KiB。");
    const filename = "name" in file && typeof file.name === "string" ? file.name : "";
    if (!/\.csv$/i.test(filename) || filename.length > 180) throw new Error("请选择名称不超过 180 字符的 .csv 文件。");
    const bytes = new Uint8Array(await file.arrayBuffer());
    const inspected = inspectCompanyCSV(bytes);
    const checksum = Array.from(new Uint8Array(await crypto.subtle.digest("SHA-256", bytes))).map((v) => v.toString(16).padStart(2, "0")).join("");
    const json = transport(config, request, () => { attempted = true; });
    const code = `CSV-${operationId}`;
    const existing = (await datasets(json, workspaceId)).find((item) => item.code === code);
    if (existing) return { ...state, datasetId: isId(existing.id) ? existing.id : undefined, locked: true, message: "本次操作已有 RAW 数据集，请先核对，不会再次上传或创建。" };
    const description = JSON.stringify({ source: (source as string).trim(), purpose: (purpose as string).trim(), originalFilename: filename, ingestionOperationId: operationId, declarationOnly: true });
    state.resourceId = created(await json("/api/v1/data-resources", { workspaceId, code, name: (name as string).trim(), description, domainCode: "COMPANY", resourceType: "TABLE_LIKE", sensitivityLevel: "INTERNAL" }), workspaceId, "TABLE_LIKE", code);
    state.datasetId = created(await json("/api/v1/datasets", { workspaceId, code, name: (name as string).trim(), description, datasetType: "RAW", sourceResourceId: state.resourceId }), workspaceId, "RAW", code);
    const multipart = new FormData();
    multipart.set("file", file, `company-import-${operationId}.csv`);
    const version = await json(`/api/v1/datasets/${state.datasetId}/versions`, multipart);
    if (isId(version.id) && same(version.datasetId, state.datasetId)) state.versionId = version.id;
    if (!state.versionId || version.status !== "READY" || version.byteSize !== bytes.length || version.rowCount !== inspected.rowCount || version.checksumAlgorithm !== "SHA256" || version.checksum !== checksum) throw new Error("RAW 版本状态、记录数或原始文件校验和不一致。");
    return { ...state, ok: true, locked: true, rowCount: inspected.rowCount, checksum, message: "原始 CSV 已保存为 RAW 版本，SHA-256 与上传字节一致。请确认下一步的主体解析策略。" };
  } catch (error) { return failure(error, state, attempted); }
}
export async function startImportedResolution(form: FormData, config: IngestConfig, request: typeof fetch = fetch): Promise<IngestState> {
  let attempted = false;
  const state: IngestState = { ok: false, message: "" };
  try {
    const { workspaceId } = configured(config);
    const versionId = form.get("versionId");
    if (!isId(versionId) || form.get("acknowledged") !== "on") throw new Error("需要有效 RAW 版本并确认主体解析策略。");
    const json = transport(config, request, () => { attempted = true; });
    const scoped = await datasets(json, workspaceId);
    const version = await json(`/api/v1/dataset-versions/${versionId}`);
    const raw = scoped.find((item) => same(item.id, String(version.datasetId)) && item.datasetType === "RAW");
    if (!raw || !same(version.id, versionId) || version.status !== "READY") throw new Error("输入必须是当前工作区中可用的 RAW 版本。");
    if (typeof raw.code !== "string" || !raw.code.startsWith("CSV-") || !isId(raw.code.slice(4))) throw new Error("只支持此接入向导创建的 RAW 数据集。");
    const operationId = raw.code.slice(4).toLowerCase();
    const sourceRef = `company-import-${operationId}.csv`;
    if (typeof version.storageUri !== "string") throw new Error("RAW 版本缺少存储引用。");
    const stored = new URL(version.storageUri);
    if (stored.protocol !== "s3:" || decodeURIComponent(stored.pathname.split("/").at(-1) ?? "") !== sourceRef) throw new Error("RAW 来源与接入操作不匹配。");
    state.datasetId = raw.id as string; state.versionId = versionId; state.operationId = operationId;
    const outputCode = `RESOLVED-${versionId.toLowerCase()}`;
    const existing = scoped.find((item) => item.code === outputCode);
    if (existing) return { ...state, outputDatasetId: isId(existing.id) ? existing.id : undefined, locked: true, message: "此版本已有解析输出容器，请核对审核队列与数据集；不会重复创建解析任务。" };
    state.outputDatasetId = created(await json("/api/v1/datasets", { workspaceId, code: outputCode, name: `${String(raw.name).slice(0, 100)} · 主体解析`, datasetType: "STANDARDIZED", description: `Input DatasetVersion: ${versionId}; operation: ${operationId}` }), workspaceId, "STANDARDIZED", outputCode);
    const job = await json("/api/v1/entity-match-jobs", { workspaceId, inputDatasetVersionId: versionId, outputDatasetId: state.outputDatasetId, sourceType: "CSV", sourceRef, sourceRole: "ANCHOR", policyRef: "park/matching/company-match-policy-v1.yaml" });
    if (!isId(job.id) || !same(job.workspaceId, workspaceId) || !same(job.inputDatasetVersionId, versionId) || !same(job.outputDatasetId, state.outputDatasetId) || typeof job.status !== "string") throw new Error("解析任务的范围或关联标识不一致。");
    return { ...state, ok: job.status !== "FAILED", locked: true, jobId: job.id, jobStatus: job.status, message: `Core 已返回主体解析任务，状态：${job.status}。待人工确认、未解析和自动匹配是不同结果，请在审核队列及输出版本中核对。` };
  } catch (error) { return failure(error, state, attempted); }
}
