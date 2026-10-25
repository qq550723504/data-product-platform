import Link from "next/link";
import { EmptyState, formatDate } from "./ui";
import { schemaAttention } from "@/lib/schema-observation-client";
import { schemaAttentionHref, schemaObservationHref, schemaView } from "@/lib/schema-observation";
import { attentionOffset } from "@/lib/quality-attention";

export async function SchemaAttention({ compact = false, query = {} }: { compact?: boolean; query?: Record<string, string | undefined> }) {
  const view = schemaView(query.schemaView), offset = attentionOffset(query.schemaOffset);
  try {
    const result = await schemaAttention(compact ? "UNHANDLED" : view, compact ? 0 : offset, compact ? 1 : 25);
    return <section className="panel" id="source-schema-attention" style={{ marginTop: 18 }} data-testid="schema-attention">
      <div className="panel-header"><div><h2>源端 schema 漂移观察</h2><span className="panel-meta">未记录处理 {result.summary.unhandledCount} 条 · 已记录处理 {result.summary.handledCount} 条</span></div>
        {compact ? <Link prefetch={false} href="/attention?schemaView=UNHANDLED#source-schema-attention">查看漂移观察</Link> : null}</div>
      <p>仅汇总已登记的源表结构漂移。记录处理或后续匹配成功均不代表旧漂移已恢复，也不构成实时来源健康结论。</p>
      {compact ? <p>{result.summary.unhandledCount ? "存在尚未记录处理的漂移观察。" : "当前没有未记录处理的已登记漂移观察；没有记录不代表所有来源健康。"}</p> : <>
        <nav className="detail-tabs" aria-label="schema 漂移视图">
          {([["UNHANDLED", "未记录处理"], ["ALL", "全部漂移观察"]] as const).map(([value, label]) =>
            <Link prefetch={false} key={value} className={`detail-tab ${view === value ? "active" : ""}`} aria-current={view === value ? "page" : undefined} href={schemaAttentionHref(query, value, 0)}>{label}</Link>)}
        </nav>
        {result.items.length === 0 ? <EmptyState title={result.page.total ? "本页没有记录" : "当前视图没有已登记漂移观察"} description="可切换视图或返回上一页；没有观察记录不代表来源健康。" /> : <div className="table-card" style={{ overflowX: "auto" }}>
          <table className="data-table"><thead><tr><th>资源 / 历史来源</th><th>扫描责任人</th><th>规则 / 原因</th><th>检测完成</th><th>处理记录</th><th>查看</th></tr></thead>
            <tbody>{result.items.map(o => <tr key={o.observationId}>
              <td className="primary-cell">{o.resourceName}<span>{o.resourceCode}</span><span>{o.externalFqn}</span></td>
              <td className="mono">{o.responsibleActorId}</td>
              <td>{o.ruleRef} / {o.ruleVersion}<p>{o.reason}</p></td>
              <td>{formatDate(o.completedAt)}</td><td>{o.handlingCount ? `已记录处理（${o.handlingCount} 条记录）` : "未记录处理"}</td>
              <td><Link prefetch={false} href={schemaObservationHref(o.resourceId, o.requestId, o.observationId)}>查看精确观察</Link></td>
            </tr>)}</tbody></table>
        </div>}
        <nav className="pagination" aria-label="schema 漂移分页">
          {offset > 0 ? <Link prefetch={false} href={schemaAttentionHref(query, view, Math.max(0, offset - 25))}>上一页</Link> : null}
          <span>{result.page.total ? `${Math.min(offset + 1, result.page.total)}–${Math.min(offset + result.items.length, result.page.total)} / ${result.page.total}` : "0 条"}</span>
          {offset + result.items.length < result.page.total ? <Link prefetch={false} href={schemaAttentionHref(query, view, offset + 25)}>下一页</Link> : null}
        </nav>
      </>}
      <p className="panel-meta">读取时间：{formatDate(result.readAt)}。按观察记录计数，不合并为资源数或全局任务总数。</p>
    </section>;
  } catch {
    return <section className="callout callout-warn" id="source-schema-attention" style={{ marginTop: 18 }} data-testid="schema-attention-unavailable">
      <h2>源端 schema 漂移汇总暂不可确认</h2><p>读取失败、权限不足或凭据未配置；数量与结论为 UNKNOWN。其他待办分区保持独立。</p>
    </section>;
  }
}
