import {RecordCode} from './record-code';
import {displayText} from '../lib/business-language.mjs';
import Link from "next/link";
import { EmptyState, formatDate } from "./ui";
import { attentionOffset } from "@/lib/quality-attention";
import { unobservedSchemaAttempts } from "@/lib/schema-attempt-client";
import { schemaAttemptsHref } from "@/lib/schema-attempts";
import { schemaRequestHref } from "@/lib/schema-requests";

export async function SchemaAttempts({ compact = false, query = {} }: { compact?: boolean; query?: Record<string, string | undefined> }) {
  const offset = compact ? 0 : attentionOffset(query.schemaAttemptOffset);
  try {
    const r = await unobservedSchemaAttempts(offset, compact ? 1 : 25);
    return <section className="panel" id="schema-unobserved-attempts" data-testid="schema-attempts" style={{ marginTop: 18 }}>
      <div className="panel-header"><div><h2>已有观察请求中的尚无结果的扫描尝试</h2><span className="panel-meta">独立计数：{displayText(r.page.total)} 个 尝试记录</span></div>
        {displayText(compact ? <Link prefetch={false} href={schemaAttemptsHref({}, 0)}>查看尚无结果的扫描尝试</Link> : null)}</div>
      <p>请求已有历史观察，但以下 尝试记录 尚无各自绑定的 扫描结果。仅展示登记事实，不推断运行、排队、超时或恢复，不计入当前未恢复待办；不与尚无结果的扫描请求数相加。</p>
      {compact ? null : <>
        {r.items.length ? <div className="table-card" style={{ overflowX: "auto" }}><table className="data-table"><thead><tr><th>资源 / 历史来源</th><th>精确身份</th><th>请求责任人 / 规则</th><th>登记时间</th><th>查看</th></tr></thead><tbody>
          {r.items.map(a => <tr key={a.attemptId}><td className="primary-cell">{displayText(a.resourceName)}<span><RecordCode value={a.resourceCode}/></span><span>{displayText(a.externalFqn)}</span></td><td className="mono">{displayText(a.attemptId)}<p>request {displayText(a.requestId)}</p></td><td>{displayText(a.responsibleActorId)}<p>{displayText(a.ruleRef)} / {displayText(a.ruleVersion)}</p></td><td>{displayText(formatDate(a.registeredAt))}<p>登记不等于实际扫描开始</p></td><td><Link prefetch={false} href={schemaRequestHref(a.resourceId, a.requestId, { kind: "OBSERVED_ATTEMPTS", offset, context: query }, a.attemptId)}>查看精确 尝试记录</Link></td></tr>)}
        </tbody></table></div> : <EmptyState title={r.page.total ? "本页没有 attempt" : "当前集合没有无观察 attempt"} description="仅说明本次读取的集合，不表示来源健康、成功或已恢复。" />}
        <nav className="pagination" aria-label="尚无结果的扫描尝试 分页">{displayText(offset > 0 ? <Link prefetch={false} href={schemaAttemptsHref(query, Math.max(0, offset - 25))}>上一页</Link> : null)}<span>{displayText(r.page.total ? `${Math.min(offset + 1, r.page.total)}–${Math.min(offset + r.items.length, r.page.total)} / ${r.page.total}` : "0 条")}</span>{displayText(offset + r.items.length < r.page.total ? <Link prefetch={false} href={schemaAttemptsHref(query, offset + 25)}>下一页</Link> : null)}</nav>
      </>}
      <p className="panel-meta">读取时间：{displayText(formatDate(r.readAt))}。仅绑定该 尝试记录 的观察到达后它才退出；历史记录与精确链接继续保留。</p>
    </section>;
  } catch {
    return <section className="callout callout-warn" id="schema-unobserved-attempts" data-testid="schema-attempts-unavailable" style={{ marginTop: 18 }}><h2>尚无结果的扫描尝试 暂不可确认</h2><p>读取未完成、失败、权限不足、凭据未配置或响应无效；数量为 无法确认。其他分区保持独立。</p></section>;
  }
}
