import Link from "next/link";
import { Suspense } from "react";
import { QualityAttention } from "@/components/quality-attention";
import { SchemaAttention } from "@/components/schema-attention";
import { SchemaExceptions } from "@/components/schema-exceptions";
import { SchemaRequests } from "@/components/schema-requests";
import { ReadUnavailable } from "@/components/read-unavailable";
import { Badge, EmptyState, MetricCard, PageHeader, SetupRequired, formatDate } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";
import { attentionTotal, readSection, validExecution, validPage, validProduct, validReadiness, validReleaseAttention, validSummary } from "@/lib/workbench-read";

function loadWorkbench(workspace: string) {
  return {
    summary: readSection(platform.workbench, s => validSummary(s, workspace)),
    executions: readSection(() => platform.executions(20, 0), p => validPage(p, 20, 0, e => validExecution(e, workspace))),
    failures: readSection(() => platform.unresolvedFailedExecutions(20, 0), p => validPage(p, 20, 0, e => validExecution(e, workspace) && e.status === "FAILED")),
    products: readSection(() => platform.products(50, 0), p => validPage(p, 50, 0, product => validProduct(product, workspace))),
    releases: readSection(() => platform.releaseAttention(10), r => validReleaseAttention(r, 10)),
  };
}
type Model = ReturnType<typeof loadWorkbench>;
async function totalFor(model: Model) {
  const [summary, failures, releases] = await Promise.all([model.summary, model.failures, model.releases]);
  return attentionTotal([summary?.reviewQueue.pending, failures?.page.total, releases?.failed.page.total, releases?.validating.page.total, releases?.ready.page.total]);
}

async function Metrics({ model }: { model: Model }) {
  const s = await model.summary;
  const active = s ? attentionTotal([s.executions.queued, s.executions.submitting, s.executions.running]) : null;
  return <section aria-label="核心指标" data-testid="workbench-metrics">
    {!s ? <ReadUnavailable title="工作台概览" /> : null}
    <div className="metric-grid">
      <MetricCard label="数据资源" value={s?.counts.dataResources ?? "UNKNOWN"} hint="可被生产链引用的业务资源" />
      <MetricCard label="数据集" value={s?.counts.datasets ?? "UNKNOWN"} hint="RAW → STANDARDIZED → CURATED → PRODUCT" />
      <MetricCard label="数据产品" value={s?.counts.dataProducts ?? "UNKNOWN"} hint="稳定业务身份与版本历史" />
      <MetricCard label="待实体审核" value={s?.reviewQueue.pending ?? "UNKNOWN"} hint={s ? s.reviewQueue.unresolved + " 条仍未解析" : "暂不可确认"} />
      <MetricCard label="活跃执行" value={active ?? "UNKNOWN"} hint={s ? s.executions.failed + " 条失败记录" : "暂不可确认"} />
      <MetricCard label="已发布 Release" value={s?.releases.published ?? "UNKNOWN"} hint={s ? s.releases.ready + " 个已具备发布条件" : "暂不可确认"} />
    </div>
  </section>;
}

function UnknownRow({ title }: { title: string }) {
  return <tr><td colSpan={4}><ReadUnavailable title={title} /></td></tr>;
}

async function HomeCount({ model }: { model: Model }) {
  const total = await totalFor(model);
  return <div data-testid="workbench-attention-total"><Badge value={total === null ? "UNKNOWN" : total ? "ACTION_REQUIRED" : "CLEAR"} />
    <p>{total === null ? "总数 UNKNOWN · 未完整获取" : "审核、执行与发布：" + total + " 条"}</p></div>;
}
async function HomeEmpty({ model }: { model: Model }) {
  return (await totalFor(model)) === 0 ? <EmptyState title="当前没有需要人工处理的事项" description="实体审核、失败执行和 Release 阻塞项会集中出现在这里。" /> : null;
}
async function HomeReview({ model }: { model: Model }) {
  const s = await model.summary;
  if (!s) return <UnknownRow title="实体审核计数" />;
  return s.reviewQueue.pending ? <tr><td className="primary-cell"><strong>实体审核队列</strong><span>{s.reviewQueue.pending} 条待人工决策</span></td><td><Badge value="PENDING" /></td>
    <td>{s.reviewQueue.unresolved} 条未解析{s.reviewQueue.conflicts ? " · " + s.reviewQueue.conflicts + " 条冲突" : ""}</td>
    <td><Link prefetch={false} className="text-link" href="/reviews">进入审核</Link></td></tr> : null;
}
async function HomeFailures({ model }: { model: Model }) {
  const p = await model.failures;
  if (!p) return <UnknownRow title="未解决执行失败" />;
  return p.page.total ? <tr><td className="primary-cell"><strong>生产执行失败</strong><span>{p.page.total} 条未解决失败</span></td><td><Badge value="FAILED" /></td>
    <td>{p.items[0] ? (p.items[0].workflowName || p.items[0].workflowCode) + " · " + p.items[0].targetPeriod + (p.items[0].errorCode ? " · " + p.items[0].errorCode : "") : "查看执行列表定位失败原因"}</td>
    <td><Link prefetch={false} className="text-link" href={p.items[0] ? "/production/" + p.items[0].id : "/production"}>查看失败执行</Link></td></tr> : null;
}
async function HomeProductLabel({ model, id }: { model: Model; id: string }) {
  const p = await model.products;
  return <>{p?.items.find(product => product.id === id)?.name ?? id + "（名称不可用）"}</>;
}
async function HomeReadiness({ release }: { release: string }) {
  const r = await readSection(() => platform.releaseReadiness(release), value => validReadiness(value, release));
  return r ? <p>{r.blockers.length ? r.blockers.slice(0, 3).join(" · ") : "本次读取没有 blocker 详情；以精确 Release 页面为准。"}</p> : <ReadUnavailable title="Readiness 详情" />;
}
async function HomeReleases({ model }: { model: Model }) {
  const r = await model.releases;
  if (!r) return <UnknownRow title="发布待办" />;
  return [
    { page: r.failed, title: "Release 失败", badge: "FAILED", link: "查看 Release" },
    { page: r.validating, title: "Release Readiness 阻塞", badge: "VALIDATING", link: "查看 Blockers" },
    { page: r.ready, title: "Release 待发布", badge: "READY", link: "去发布" },
  ].map(g => {
    const release = g.page.items[0];
    return g.page.page.total ? <tr key={g.title}>
      <td className="primary-cell"><strong>{g.title}</strong><span>{g.page.page.total} 个 Release</span></td><td><Badge value={g.badge} /></td>
      <td>{release ? <><Suspense fallback={release.productId + "（名称读取中）"}><HomeProductLabel model={model} id={release.productId} /></Suspense> · {release.releaseNo}</> : "本次读取无详情"}
        {g.badge === "VALIDATING" && release ? <Suspense fallback={<ReadUnavailable title="Readiness 详情读取中" />}><HomeReadiness release={release.id} /></Suspense> : null}</td>
      <td>{release ? <Link prefetch={false} className="text-link" href={g.badge === "FAILED" ? "/products/" + release.productId + "/releases/" + release.id : "/products/" + release.productId + "#releases"}>{g.link}</Link> : null}</td>
    </tr> : null;
  });
}
function WorkbenchAttention({ model }: { model: Model }) {
  return <section className="panel" style={{ marginTop: 18 }} data-testid="needs-attention">
    <div className="panel-header"><h2>审核、执行与发布待办</h2><Link prefetch={false} href="/attention">查看全部</Link></div>
    <Suspense fallback={<ReadUnavailable title="待办总数读取中" />}><HomeCount model={model} /></Suspense>
    <Suspense fallback={null}><HomeEmpty model={model} /></Suspense>
    <div className="table-card"><table className="data-table"><thead><tr><th>事项</th><th>状态</th><th>说明</th><th>处理入口</th></tr></thead><tbody>
      <Suspense fallback={<UnknownRow title="实体审核读取中" />}><HomeReview model={model} /></Suspense>
      <Suspense fallback={<UnknownRow title="未解决执行失败读取中" />}><HomeFailures model={model} /></Suspense>
      <Suspense fallback={<UnknownRow title="发布待办读取中" />}><HomeReleases model={model} /></Suspense>
    </tbody></table></div>
    <p className="panel-meta">各分区按各自读取结果展示，不是同一个冻结快照；质量和 schema 分类不计入上述待办总数。</p>
  </section>;
}

async function RecentExecutions({ model }: { model: Model }) {
  const executions = await model.executions;
  return <section className="panel" data-testid="recent-executions">
    <div className="panel-header"><h2>最近生产执行</h2><Link prefetch={false} href="/production">查看全部</Link></div>
    {!executions ? <ReadUnavailable title="最近生产执行" /> : executions.items.length === 0 ?
      <EmptyState title="尚无执行记录" description="创建 Workflow Execution 后，这里会出现输入版本、运行状态与输出版本。" /> :
      <div className="table-card"><table className="data-table"><thead><tr><th>Workflow</th><th>期间</th><th>状态</th><th>创建时间</th></tr></thead><tbody>
        {executions.items.map(e => <tr key={e.id}><td className="primary-cell"><Link prefetch={false} className="text-link" href={"/production/" + e.id}><strong>{e.workflowName || e.workflowCode}</strong></Link><span>v{e.workflowVersion} · attempt {e.attempt}</span></td>
          <td>{e.targetPeriod}</td><td><Badge value={e.status} /></td><td>{formatDate(e.createdAt)}</td></tr>)}
      </tbody></table></div>}
  </section>;
}

async function ReferenceChain({ model }: { model: Model }) {
  const s = await model.summary;
  const active = s ? attentionTotal([s.executions.queued, s.executions.submitting, s.executions.running]) : null;
  return <aside className="panel">
    <div className="panel-header"><h2>参考生产链</h2></div>
    <div className="flow" aria-label="数据产品生产流程">
      {["Data Resource", "Raw Dataset", "标准化", "实体解析", "加工", "质量/合规", "Data Contract", "Data Product", "Release"].map((step, index) => <span key={step} className="flow"><span className="flow-step">{step}</span>{index < 8 ? <span className="flow-arrow">→</span> : null}</span>)}
    </div>
    <div className="status-stack" style={{ marginTop: 18 }}>
      <div className="status-row"><span>实体审核队列</span><Badge value={s ? s.reviewQueue.pending ? "PENDING" : "READY" : "UNKNOWN"} /></div>
      <div className="status-row"><span>生产执行</span><Badge value={active === null ? "UNKNOWN" : active ? "RUNNING" : "READY"} /></div>
      <div className="status-row"><span>Release</span><Badge value={s ? s.releases.failed ? "FAILED" : "READY" : "UNKNOWN"} /></div>
    </div>
  </aside>;
}

export default function WorkbenchPage() {
  const workspace = configuredWorkspaceId();
  if (!workspace) return <><PageHeader eyebrow="Workbench" title="数据产品工作台" /><SetupRequired /></>;
  const model = loadWorkbench(workspace);
  return <>
    <PageHeader eyebrow="Workbench" title="数据产品工作台" description="观察资源、生产、实体审核与发布状态。各分区独立读取，未知结果明确标记。" action={<span className="mono">Workspace · {workspace.slice(0, 8)}…</span>} />
    <Suspense fallback={<ReadUnavailable title="工作台概览读取中" />}><Metrics model={model} /></Suspense>
    <Suspense fallback={<section id="quality-attention" data-testid="quality-attention-pending" style={{ marginTop: 18 }}><ReadUnavailable title="质量异常汇总读取中" /></section>}><QualityAttention compact /></Suspense>
    <Suspense fallback={<section id="source-schema-attention" data-testid="schema-attention-pending" style={{ marginTop: 18 }}><ReadUnavailable title="源端 schema 漂移汇总读取中" /></section>}><SchemaAttention compact /></Suspense>
    <Suspense fallback={<section id="schema-exception-history" data-testid="schema-exceptions-pending" style={{ marginTop: 18 }}><ReadUnavailable title="源端 schema 扫描异常历史读取中" /></section>}><SchemaExceptions compact /></Suspense>
    <Suspense fallback={<section id="schema-unobserved-requests" data-testid="schema-requests-pending" style={{ marginTop: 18 }}><ReadUnavailable title="无观察请求读取中" /></section>}><SchemaRequests compact /></Suspense>
    <WorkbenchAttention model={model} />
    <section className="section-grid">
      <Suspense fallback={<ReadUnavailable title="最近生产执行读取中" />}><RecentExecutions model={model} /></Suspense>
      <Suspense fallback={<ReadUnavailable title="参考生产链计数读取中" />}><ReferenceChain model={model} /></Suspense>
    </section>
  </>;
}
