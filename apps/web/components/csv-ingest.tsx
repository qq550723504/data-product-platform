"use client";
import Link from "next/link";
import { useActionState, useRef, useState } from "react";
import { uploadCSV, resolveCSV } from "@/app/ingest/actions";
import { inspectCompanyCSV, type CSVInspection } from "@/lib/csv-inspection";
import type { IngestState } from "@/lib/ingest-command";
import styles from "./csv-ingest.module.css";
const initial: IngestState = { ok: false, message: "" };
function Result({ state }: { state: IngestState }) {
  return <div className={styles.result}>
    {state.message ? <p role={state.ok ? "status" : "alert"}>{state.message}</p> : null}
    {state.operationId ? <small>接入标识：<code>{state.operationId}</code></small> : null}
    {state.resourceId ? <Link href={`/resources/${state.resourceId}`}>查看已登记资源</Link> : null}
    {state.datasetId ? <Link href={`/datasets/${state.datasetId}`}>查看 RAW 数据集与版本</Link> : null}
    {state.versionId ? <><small>RAW 版本：<code>{state.versionId}</code></small><Link href={`/ingest?version=${state.versionId}`}>保存此入口，稍后继续解析</Link></> : null}
    {state.checksum ? <small>原始文件 SHA-256：<code>{state.checksum}</code></small> : null}
    {state.outputDatasetId ? <Link href={`/datasets/${state.outputDatasetId}`}>查看解析输出数据集</Link> : null}
    {state.jobId ? <small>解析任务：<code>{state.jobId}</code> · {state.jobStatus}</small> : null}
    {state.jobId || (state.locked && !state.ok) ? <Link href="/reviews">进入实体审核队列</Link> : null}
  </div>;
}
function Resolution({ versionId, enabled }: { versionId: string; enabled: boolean }) {
  const [acknowledged, setAcknowledged] = useState(false);
  const [state, action, pending] = useActionState(resolveCSV, initial);
  const locked = pending || state.locked === true;
  return <section className="detail-card">
    <span className="eyebrow">02 · Entity Resolution</span><h2>启动主体解析</h2>
    <p className={styles.meta}>输入版本：<code>{versionId}</code>。提交前会由服务端重新核对工作区、RAW 类型和版本状态。</p>
    <p>当前模板：主体基础信息（COMPANY），策略 <code>company-match-policy-v1</code>，来源角色 <code>ANCHOR</code>。</p>
    <p className={styles.meta}>基准来源允许规则引擎建立新主体；低置信候选进入人工审核，不会自动替你确认。解析成功也不代表允许发布。</p>
    <form action={action} className={styles.form} aria-label="启动主体解析">
      <input type="hidden" name="versionId" value={versionId} />
      <label className={styles.check}><input type="checkbox" name="acknowledged" checked={acknowledged} disabled={locked} onChange={(event) => setAcknowledged(event.target.checked)} />确认以此文件作为主体基准来源，使用上述策略。</label>
      <button disabled={!enabled || !acknowledged || locked} type="submit">{pending ? "正在启动解析…" : "启动解析并进入审核"}</button>
    </form>
    <Result state={state} />
  </section>;
}
export function CSVIngest({ operationId, enabled, resumeVersionId }: { operationId: string; enabled: boolean; resumeVersionId?: string }) {
  const [inspection, setInspection] = useState<CSVInspection | null>(null);
  const [error, setError] = useState("");
  const [inspecting, setInspecting] = useState(false);
  const [name, setName] = useState("");
  const [source, setSource] = useState("");
  const [purpose, setPurpose] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  const generation = useRef(0);
  const [state, action, pending] = useActionState(async (previous: IngestState, form: FormData) => {
    const result = await uploadCSV(operationId, previous, form);
    if (!result.locked) setInspection(null);
    return result;
  }, initial);
  const locked = pending || state.locked === true;
  const versionId = resumeVersionId ?? (state.ok ? state.versionId : undefined);
  async function inspect(file?: File) {
    const current = ++generation.current;
    setInspection(null); setError(""); setInspecting(true);
    try {
      if (!file) return;
      if (file.size > 512 * 1024) throw new Error("文件不能超过 512 KiB。");
      const result = inspectCompanyCSV(new Uint8Array(await file.arrayBuffer()));
      if (generation.current === current) setInspection(result);
    } catch (cause) {
      if (generation.current === current) setError(cause instanceof Error ? cause.message : "CSV 预检失败。");
    } finally { if (generation.current === current) setInspecting(false); }
  }
  return <div className={styles.flow}>
    {!resumeVersionId ? <section className="detail-card">
      <span className="eyebrow">01 · Original CSV</span><h2>预检并保存原始文件</h2>
      <p className={styles.meta}>UTF-8（支持 BOM）、逗号分隔，最多 512 KiB / 1000 条记录。必需字段：<code>source_company_id</code>、<code>company_name</code>。</p>
      <p className={styles.meta}>可选字段：<code>unified_social_credit_code</code>、<code>legal_representative</code>、<code>registered_address</code>、<code>entry_date</code>、<code>company_status</code>。V1 不提供任意字段映射或 Excel 导入。</p>
      <p className={styles.meta}><a href="/templates/company-import-v1.csv" download="company-import-v1.csv">下载 CSV 示例模板（合成数据）</a> · 请替换示例记录。信用代码留空仅用于字段演示，不保证产生可确认的匹配。</p>
      <form action={action} className={styles.form} aria-label="接入 CSV">
        <label>数据集名称<input name="name" required maxLength={120} value={name} disabled={locked} onChange={(event) => setName(event.target.value)} /></label>
        <label>数据来源说明<textarea name="source" required maxLength={500} rows={2} value={source} disabled={locked} onChange={(event) => setSource(event.target.value)} placeholder="说明文件的提供方或生成方式，不要填写密钥。" /></label>
        <label>本次处理用途<textarea name="purpose" required maxLength={500} rows={2} value={purpose} disabled={locked} onChange={(event) => setPurpose(event.target.value)} /></label>
        <label>选择 CSV 文件<input type="file" name="file" accept=".csv,text/csv" required disabled={locked} onChange={(event) => { void inspect(event.target.files?.[0]); }} /></label>
        {inspecting ? <p role="status">正在本地预检文件…</p> : null}
        {error ? <p role="alert">{error}</p> : null}
        {inspection ? <div>
          <strong>CSV 预检通过：{inspection.rowCount} 条记录，{inspection.headers.length} 个字段{inspection.hasBOM ? "（UTF-8 BOM）" : ""}</strong>
          <div className={styles.table}><table className="data-table"><caption>前 5 条原始记录预览</caption><thead><tr>{inspection.headers.map((header) => <th key={header}>{header}</th>)}</tr></thead><tbody>{inspection.preview.map((row, index) => <tr key={index}>{row.map((value, cell) => <td key={cell}>{value}</td>)}</tr>)}</tbody></table></div>
        </div> : null}
        <label className={styles.check}><input name="acknowledged" type="checkbox" checked={acknowledged} disabled={locked} onChange={(event) => setAcknowledged(event.target.checked)} />确认这是合成数据或已获准在此环境处理的数据；上述声明不替代正式授权。</label>
        <button type="submit" disabled={!enabled || locked || inspecting || !inspection || !acknowledged || !name.trim() || !source.trim() || !purpose.trim()}>{pending ? "正在保存原始版本…" : "登记来源并保存 RAW 版本"}</button>
      </form>
      <Result state={state} />
      <p className={styles.meta}>离开页面不会撤销已写入的数据。遇到失败先核对对象标识；同一操作不会盲目重新创建，历史版本不会被覆盖。</p>
    </section> : <Link href="/ingest">返回新文件接入</Link>}
    {versionId ? <Resolution key={versionId} versionId={versionId} enabled={enabled} /> : null}
  </div>;
}
