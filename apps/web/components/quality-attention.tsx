import Link from "next/link";
import { Badge, EmptyState, formatDate, shortId } from "./ui";
import { qualityAttention } from "@/lib/quality-remediation";
import { assessmentHref, attentionOffset, attentionView, caseHref, qualityAttentionHref } from "@/lib/quality-attention";

export async function QualityAttention({ compact = false, query = {} }: {
  compact?: boolean; query?: Record<string, string | undefined>;
}) {
  const view = attentionView(query.qualityView);
  const offset = attentionOffset(query.qualityOffset);
  try {
    const result = await qualityAttention(compact ? "FOLLOW_UP" : view, compact ? 0 : offset, compact ? 1 : 25);
    return <section className="panel" id="quality-attention" style={{ marginTop: 18 }} data-testid="quality-attention">
      <div className="panel-header">
        <div><h2>质量异常</h2><span className="panel-meta">需跟进 {result.summary.followUpCount} 条 · 已人工关闭 {result.summary.closedCount} 条</span></div>
        {compact ? <Link href="/attention?qualityView=FOLLOW_UP#quality-attention">查看质量异常</Link> : null}
      </div>
      <p>当前 workspace 已登记的用电异常。人工关闭、处理执行、复检结论分别展示；关闭不代表合格或可交付。</p>
      {compact ? <p>{result.summary.followUpCount ? "存在待分派、处理中或待核对的异常。" : "当前没有需跟进的已登记质量异常；未检测不代表质量合格。"}</p> : <>
        <nav className="detail-tabs" style={{ margin: "0 0 14px" }} aria-label="质量异常视图">
          {([["FOLLOW_UP", "需跟进"], ["ALL", "全部"], ["CLOSED", "已人工关闭"]] as const).map(([value, label]) =>
            <Link key={value} className={`detail-tab ${view === value ? "active" : ""}`} href={qualityAttentionHref(query, value, 0)} aria-current={view === value ? "page" : undefined}>{label}</Link>)}
        </nav>
        {result.items.length === 0 ? <EmptyState title={result.page.total ? "本页没有记录" : "当前视图没有已登记异常"} description="使用分页返回有效页；没有登记记录不代表数据质量合格。" /> : <div className="table-card" style={{ overflowX: "auto" }}>
          <table className="data-table" style={{ minWidth: 1040 }}><thead><tr><th>数据集 / 版本</th><th>异常</th><th>处置责任人</th><th>执行</th><th>复检</th><th>人工关闭</th><th>创建时间</th><th>查看</th></tr></thead>
            <tbody>{result.items.map(item => <tr key={item.id}>
              <td className="primary-cell"><Link prefetch={false} href={`/datasets/${item.datasetId}/versions/${item.versionId}`}>{item.datasetName} · v{item.versionNo}</Link><span>{item.datasetCode}</span>{item.resourceId ? <Link prefetch={false} href={`/resources/${item.resourceId}`}>来源资源</Link> : null}</td>
              <td>{item.message || "异常详情待查看"}<br /><span className="mono">{item.ruleId}</span></td>
              <td>{item.actorId ? <span className="mono" title={item.actorId}>{shortId(item.actorId)}</span> : "未分派"}</td>
              <td>{item.executionId ? <Link prefetch={false} href={`/production/${item.executionId}`}><Badge value={item.executionStatus ?? "UNKNOWN"} /></Link> : "未执行"}{item.activeExecution ? <p>已接纳执行尚未终结</p> : null}</td>
              <td>{item.recheckId ? <><Badge value={item.recheckOutcome ?? "UNKNOWN"} />{item.recheckGateDecision ? <Badge value={item.recheckGateDecision} /> : <span> 尚无质量结论</span>}</> : "尚未复检"}{item.pendingRecheck ? <p>存在原复检待核对</p> : null}</td>
              <td>{item.manuallyClosed ? "已人工关闭（不代表合格）" : "未关闭"}{item.manuallyClosed && item.needsFollowUp ? <p>仍有已接纳工作待核对</p> : null}</td>
              <td>{formatDate(item.createdAt)}</td>
              <td><div className="status-stack">
                <Link prefetch={false} href={caseHref(item.datasetId, item.versionId, item.id)}>查看异常</Link>
                <Link prefetch={false} href={assessmentHref(item.datasetId, item.versionId, item.assessmentId)}>原评测与证据</Link>
                {item.recheckAssessmentId && item.recheckDatasetId && item.recheckVersionId ? <Link prefetch={false} href={assessmentHref(item.recheckDatasetId, item.recheckVersionId, item.recheckAssessmentId)}>本次复检与证据</Link> : null}
              </div></td>
            </tr>)}</tbody></table>
        </div>}
        <nav className="pagination" aria-label="质量异常分页">
          {offset > 0 ? <Link href={qualityAttentionHref(query, view, Math.max(0, offset - 25))}>上一页</Link> : null}
          <span>{result.page.total ? `${Math.min(offset + 1, result.page.total)}–${Math.min(offset + result.items.length, result.page.total)} / ${result.page.total}` : "0 条"}</span>
          {offset + result.items.length < result.page.total ? <Link href={qualityAttentionHref(query, view, offset + 25)}>下一页</Link> : null}
        </nav>
      </>}
      <p className="panel-meta">读取时间：{formatDate(result.readAt)}。两类计数可能重叠，不合并为去重任务总数。</p>
    </section>;
  } catch {
    return <section className="callout callout-warn" id="quality-attention" style={{ marginTop: 18 }} data-testid="quality-attention-unavailable">
      <h2>质量异常汇总暂不可确认</h2><p>读取失败、权限不足或受信任凭据未配置；当前数量与结论为 UNKNOWN。请检查配置后刷新，原审核、执行和发布事项保持独立。</p>
    </section>;
  }
}
