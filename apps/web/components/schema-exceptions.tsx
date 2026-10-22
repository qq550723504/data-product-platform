import Link from "next/link";
import { Badge, EmptyState, formatDate } from "./ui";
import { attentionOffset } from "@/lib/quality-attention";
import { schemaExceptions } from "@/lib/schema-exception-client";
import { schemaExceptionEvidenceHref, schemaExceptionsHref } from "@/lib/schema-exceptions";
import { schemaExceptionFilter, type SchemaExceptionFilter } from "@/lib/schema-exception-filter";

const filters: Array<[SchemaExceptionFilter, string]> = [["ALL", "全部异常"], ["UNAVAILABLE", "扫描不可用"], ["DENIED", "当次拒绝"], ["INTERRUPTED", "扫描中断"]];

export async function SchemaExceptions({ compact = false, query = {} }: { compact?: boolean; query?: Record<string, string | undefined> }) {
  const status = compact ? "ALL" : schemaExceptionFilter(query.schemaExceptionStatus);
  const offset = compact ? 0 : attentionOffset(query.schemaExceptionOffset);
  try {
    const result = await schemaExceptions(status, offset, compact ? 1 : 25);
    const { unavailableCount, deniedCount, interruptedCount } = result.summary;
    return <section className="panel" id="schema-exception-history" style={{ marginTop: 18 }} data-testid="schema-exceptions">
      <div className="panel-header">
        <div><h2>源端 schema 扫描异常历史</h2><span className="panel-meta">UNAVAILABLE {unavailableCount} 条 · DENIED {deniedCount} 条 · INTERRUPTED {interruptedCount} 条</span></div>
        {compact ? <Link prefetch={false} href={schemaExceptionsHref({}, "ALL", 0)}>查看扫描异常历史</Link> : null}
      </div>
      <p>按 Core 已记录的异常观察计数，不是当前未恢复待办数。后续扫描成功仍保留旧异常；没有记录不代表来源健康。尚无观察的请求或 attempt 不在本历史范围内。</p>
      {compact ? <p>已记录 {unavailableCount + deniedCount + interruptedCount} 条扫描异常历史。</p> : <>
        <nav className="detail-tabs" aria-label="扫描异常历史筛选">
          {filters.map(([value, label]) => <Link key={value} prefetch={false} className={`detail-tab ${status === value ? "active" : ""}`} aria-current={status === value ? "page" : undefined} href={schemaExceptionsHref(query, value, 0)}>{label}</Link>)}
        </nav>
        {result.items.length === 0 ? <EmptyState title={result.page.total ? "本页没有历史记录" : "当前筛选没有已登记扫描异常"} description="可切换筛选或返回上一页；此处不提供当前健康或恢复结论。" /> :
          <div className="table-card" style={{ overflowX: "auto" }}>
            <table className="data-table"><thead><tr><th>资源 / 历史来源</th><th>请求责任人</th><th>观察 / 原因</th><th>规则</th><th>记录时间</th><th>查看</th></tr></thead>
              <tbody>{result.items.map(o => <tr key={o.observationId}>
                <td className="primary-cell">{o.resourceName}<span>{o.resourceCode}</span><span>{o.externalFqn}</span></td>
                <td className="mono">{o.responsibleActorId}</td>
                <td><Badge value={o.status} /><p>{o.reason}</p><span>{o.attemptId ? `attempt ${o.attemptId}` : "未绑定 attempt"}</span></td>
                <td>{o.ruleRef} / {o.ruleVersion}</td><td>{formatDate(o.recordedAt)}</td>
                <td><Link prefetch={false} href={schemaExceptionEvidenceHref(o, status, offset)}>查看精确观察</Link></td>
              </tr>)}</tbody>
            </table>
          </div>}
        <nav className="pagination" aria-label="扫描异常历史分页">
          {offset > 0 ? <Link prefetch={false} href={schemaExceptionsHref(query, status, Math.max(0, offset - 25))}>上一页</Link> : null}
          <span>{result.page.total ? `${Math.min(offset + 1, result.page.total)}–${Math.min(offset + result.items.length, result.page.total)} / ${result.page.total}` : "0 条"}</span>
          {offset + result.items.length < result.page.total ? <Link prefetch={false} href={schemaExceptionsHref(query, status, offset + 25)}>下一页</Link> : null}
        </nav>
      </>}
      <p className="panel-meta">读取时间：{formatDate(result.readAt)}。分类计数覆盖三类异常历史，列表总数按当前筛选计算；不合并为资源数或当前任务数。</p>
    </section>;
  } catch {
    return <section className="callout callout-warn" id="schema-exception-history" style={{ marginTop: 18 }} data-testid="schema-exceptions-unavailable">
      <h2>源端 schema 扫描异常历史暂不可确认</h2><p>读取失败、权限不足或凭据未配置；数量与结论为 UNKNOWN。其他待办分区保持独立。</p>
    </section>;
  }
}
