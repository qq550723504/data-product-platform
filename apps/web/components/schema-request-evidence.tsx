import Link from "next/link";
import { DefinitionList, LoadError, PageHeader, formatDate } from "./ui";
import { schemaRequestDetail } from "@/lib/schema-request-client";
import { attentionOffset } from "@/lib/quality-attention";
import { schemaRequestOrigin, schemaRequestHref, schemaRequestListHref, schemaRequestObservationHref } from "@/lib/schema-requests";

export async function SchemaRequestEvidence({ resourceId, query }: { resourceId: string; query: Record<string, string | undefined> }) {
  const aOffset = attentionOffset(query.attemptOffset), oOffset = attentionOffset(query.observationOffset);
  try {
    const origin = schemaRequestOrigin(query);
    const d = await schemaRequestDetail(resourceId, query.requestId ?? "", query.attemptId, aOffset, oOffset);
    const href = (attempt: string | undefined, a: number, o: number) => schemaRequestHref(resourceId, d.requestId, origin, attempt, a, o);
    const observationHref = (id: string) => schemaRequestObservationHref(resourceId, d.requestId, id, origin, query.attemptId, aOffset, oOffset);
    return <>
      <Link prefetch={false} href={schemaRequestListHref(origin)}>{origin.kind === "OBSERVED_ATTEMPTS" ? "返回无观察 attempt" : "返回无观察请求"}</Link>
      <PageHeader title="来源 schema 精确请求证据" eyebrow={d.resourceCode} description={`${d.resourceName} · 只读事实`} />
      <section className="detail-card" data-testid="schema-request-evidence"><h2>请求 {d.requestId}</h2>
        <p>此处始终读取原请求及其准确关联事实。登记 attempt 不证明原生调用已经发生，也不确认运行或超时；读取时间不等于扫描时间。后到观察不表示来源当前健康。</p>
        <DefinitionList items={[
          { label: "Workspace", value: d.workspaceId }, { label: "资源 ID", value: d.resourceId }, { label: "扫描请求", value: d.requestId },
          { label: "请求责任人", value: d.responsibleActorId }, { label: "历史来源", value: d.externalFqn }, { label: "历史 binding", value: d.bindingId }, { label: "历史外部 ID", value: d.externalId },
          { label: "规则", value: `${d.ruleRef} / ${d.ruleVersion}` }, { label: "规则 hash", value: d.ruleHash }, { label: "观察契约 hash", value: d.planHash }, { label: "Connector hash", value: d.connectorHash },
          { label: "请求时间", value: formatDate(d.requestedAt) }, { label: "读取时间", value: formatDate(d.readAt) }, { label: "已记录事实", value: `${d.attemptCount} 个 attempt；${d.observations.page.total} 条 observation` },
        ]} />
        {d.observations.page.total ? <p>该请求已有观察，已不属于无观察请求集合；历史链接继续有效。请按具体 observation 查看其结论，不能将退出集合解释为成功或恢复。</p> : <p>本次读取尚无任何 observation；不计入当前未恢复待办。</p>}
        {d.selectedAttempt ? <div data-testid="schema-selected-attempt"><h3>指定 attempt {d.selectedAttempt.id}</h3><p>登记时间 {formatDate(d.selectedAttempt.registeredAt)}；这不是实际扫描开始时间。</p>{d.selectedAttempt.observationId ? <Link prefetch={false} href={observationHref(d.selectedAttempt.observationId)}>查看指定 attempt 的精确观察</Link> : <p>该 attempt 尚无观察结果，实际运行情况 UNKNOWN。</p>}</div> : <p>未指定 attempt；不会自动选择最近一次。</p>}
        <h3>固定基线</h3><div className="table-card"><table className="data-table"><thead><tr><th>列</th><th>类型</th><th>约束</th></tr></thead><tbody>{d.baseline.map(c => <tr key={c.name}><td>{c.name}</td><td>{c.type}</td><td>{c.constraint}</td></tr>)}</tbody></table></div>
        <h3>已登记 attempt</h3>{d.attempts.items.length ? d.attempts.items.map(a => <p key={a.id}><Link prefetch={false} href={href(a.id, aOffset, oOffset)}>查看 attempt {a.id}</Link> · 登记时间 {formatDate(a.registeredAt)}{a.observationId ? <> · <Link prefetch={false} href={observationHref(a.observationId)}>精确观察 {a.observationId}</Link></> : " · 尚无观察结果"}</p>) : <p>{d.attemptCount ? "本页没有 attempt" : "尚未登记 attempt"}</p>}
        <nav className="pagination" aria-label="请求 attempt 分页">{aOffset ? <Link prefetch={false} href={href(query.attemptId, Math.max(0, aOffset - 25), oOffset)}>上一页</Link> : null}<span>{d.attempts.page.total} 个 attempt · offset {aOffset}</span>{aOffset + d.attempts.items.length < d.attempts.page.total ? <Link prefetch={false} href={href(query.attemptId, aOffset + 25, oOffset)}>下一页</Link> : null}</nav>
        <h3>关联 observation</h3>{d.observations.items.length ? d.observations.items.map(o => <p key={o.id}><Link prefetch={false} href={observationHref(o.id)}>查看观察 {o.id}</Link> · {o.status} · {o.reason} · {formatDate(o.recordedAt)}</p>) : <p>{d.observations.page.total ? "本页没有 observation" : "尚无 observation"}</p>}
        <nav className="pagination" aria-label="请求 observation 分页">{oOffset ? <Link prefetch={false} href={href(query.attemptId, aOffset, Math.max(0, oOffset - 25))}>上一页</Link> : null}<span>{d.observations.page.total} 条 observation · offset {oOffset}</span>{oOffset + d.observations.items.length < d.observations.page.total ? <Link prefetch={false} href={href(query.attemptId, aOffset, oOffset + 25)}>下一页</Link> : null}</nav>
        <p>本页不发起扫描、重试、处理或恢复。质量、认证和交付结论沿用各自事实与门禁。</p>
      </section>
    </>;
  } catch (error) {
    return <><PageHeader title="来源 schema 精确请求证据" /><section data-testid="schema-request-error"><LoadError error={error} /></section></>;
  }
}
