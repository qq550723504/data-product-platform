import Link from "next/link";
import { EmptyState, formatDate } from "./ui";
import { attentionOffset } from "@/lib/quality-attention";
import { unobservedSchemaRequests } from "@/lib/schema-request-client";
import { schemaRequestHref, schemaRequestsHref } from "@/lib/schema-requests";
import { schemaRequestView, type SchemaRequestView } from "@/lib/schema-request-filter";

const filters: Array<[SchemaRequestView, string]> = [["ALL", "全部无观察请求"], ["NO_ATTEMPT", "尚未登记 attempt"], ["ATTEMPT_RECORDED", "已登记 attempt"]];
export async function SchemaRequests({ compact = false, query = {} }: { compact?: boolean; query?: Record<string, string | undefined> }) {
  const view = compact ? "ALL" : schemaRequestView(query.schemaRequestView), offset = compact ? 0 : attentionOffset(query.schemaRequestOffset);
  try {
    const r = await unobservedSchemaRequests(view, offset, compact ? 1 : 25);
    return <section className="panel" id="schema-unobserved-requests" data-testid="schema-requests" style={{ marginTop: 18 }}>
      <div className="panel-header"><div><h2>源端 schema 尚无观察结果的请求</h2><span className="panel-meta">尚未登记 attempt {r.summary.noAttemptCount} 个 · 已登记 attempt {r.summary.attemptRecordedCount} 个</span></div>
        {compact ? <Link prefetch={false} href={schemaRequestsHref({}, "ALL", 0)}>查看无观察请求</Link> : null}</div>
      <p>按尚无任何 observation 的请求独立计数，不计入当前未恢复待办。登记 attempt 仅证明身份已记录，不提供运行、超时或恢复结论；已有任意观察的重试请求不在此集合。</p>
      {compact ? <p>当前读取共有 {r.page.total} 个无观察请求。</p> : <>
        <nav className="detail-tabs" aria-label="无观察请求筛选">{filters.map(([value, label]) => <Link prefetch={false} key={value} className={`detail-tab ${view === value ? "active" : ""}`} aria-current={view === value ? "page" : undefined} href={schemaRequestsHref(query, value, 0)}>{label}</Link>)}</nav>
        {r.items.length ? <div className="table-card" style={{ overflowX: "auto" }}><table className="data-table"><thead><tr><th>资源 / 历史来源</th><th>请求责任人</th><th>已记录事实</th><th>规则</th><th>请求时间</th><th>查看</th></tr></thead><tbody>
          {r.items.map(item => <tr key={item.requestId}><td className="primary-cell">{item.resourceName}<span>{item.resourceCode}</span><span>{item.externalFqn}</span></td><td className="mono">{item.responsibleActorId}</td><td>{item.attemptCount ? `已登记 ${item.attemptCount} 个 attempt，尚无观察结果` : "尚未登记 attempt"}<p className="mono">{item.requestId}</p></td><td>{item.ruleRef} / {item.ruleVersion}</td><td>{formatDate(item.requestedAt)}</td><td><Link prefetch={false} href={schemaRequestHref(item.resourceId, item.requestId, view, offset)}>查看精确请求</Link></td></tr>)}
        </tbody></table></div> : <EmptyState title={r.page.total ? "本页没有请求" : "当前筛选没有无观察请求"} description="仅说明本次读取的集合；不表示来源健康或已经恢复。" />}
        <nav className="pagination" aria-label="无观察请求分页">{offset > 0 ? <Link prefetch={false} href={schemaRequestsHref(query, view, Math.max(0, offset - 25))}>上一页</Link> : null}<span>{r.page.total ? `${Math.min(offset + 1, r.page.total)}–${Math.min(offset + r.items.length, r.page.total)} / ${r.page.total}` : "0 条"}</span>{offset + r.items.length < r.page.total ? <Link prefetch={false} href={schemaRequestsHref(query, view, offset + 25)}>下一页</Link> : null}</nav>
      </>}
      <p className="panel-meta">读取时间：{formatDate(r.readAt)}。任意观察到达后请求退出此集合，历史仍保留，退出不表示成功或恢复。</p>
    </section>;
  } catch {
    return <section className="callout callout-warn" id="schema-unobserved-requests" data-testid="schema-requests-unavailable" style={{ marginTop: 18 }}><h2>无观察请求暂不可确认</h2><p>读取失败、权限不足或凭据未配置；数量为 UNKNOWN。其他分区保持独立。</p></section>;
  }
}
