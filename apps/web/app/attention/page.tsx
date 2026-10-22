import Link from "next/link";
import { Suspense } from "react";
import { QualityAttention } from "@/components/quality-attention";
import { SchemaAttention } from "@/components/schema-attention";
import { SchemaExceptions } from "@/components/schema-exceptions";
import { SchemaRequests } from "@/components/schema-requests";
import { SchemaAttempts } from "@/components/schema-attempts";
import { ReadUnavailable } from "@/components/read-unavailable";
import { schemaRequestView } from "@/lib/schema-request-filter";
import { schemaExceptionFilter } from "@/lib/schema-exception-filter";
import { schemaView } from "@/lib/schema-observation";
import { attentionOffset, attentionView } from "@/lib/quality-attention";
import { Badge, EmptyState, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { collectAllPages } from "@/lib/pagination";
import { configuredWorkspaceId, platform, type DataProduct } from "@/lib/platform";
import { attentionTotal, readSection, validExecution, validPage, validProduct, validRelease, validReleaseAttention, validReview } from "@/lib/workbench-read";

const PAGE_SIZE = 25;
type Query = Record<string, string | undefined>;
type Offsets = Record<string, number | string>;
type ReleaseKind = "failed" | "validating" | "ready";
const releaseGroups = [
  { key: "failed", status: "FAILED", title: "Release 失败", offset: "failedReleaseOffset", action: "查看 Release" },
  { key: "validating", status: "VALIDATING", title: "Readiness 阻塞", offset: "validatingReleaseOffset", action: "查看 Blockers" },
  { key: "ready", status: "READY", title: "Release 待发布", offset: "readyReleaseOffset", action: "去发布" },
] as const;

function attentionHref(current: Offsets, key: string, offset: number): string {
  const query = new URLSearchParams();
  for (const [name, value] of Object.entries(current)) {
    if (typeof value === "string" || value > 0) query.set(name, String(value));
  }
  if (offset > 0) query.set(key, String(offset)); else query.delete(key);
  return "/attention?" + query;
}

function AttentionPager({ keyName, offset, itemCount, total, current }: { keyName: string; offset: number; itemCount: number; total: number; current: Offsets }) {
  if (total <= PAGE_SIZE && offset === 0) return null;
  return <nav className="pagination attention-pagination" aria-label="待办分页">
    {offset > 0 ? <Link prefetch={false} className="pagination-step" href={attentionHref(current, keyName, Math.max(0, offset - PAGE_SIZE))}>上一页</Link> : <span className="pagination-step pagination-step-disabled" aria-disabled="true">上一页</span>}
    <span className="pagination-summary">{total === 0 ? "0 条" : Math.min(offset + 1, total) + "–" + Math.min(offset + itemCount, total) + " / " + total}</span>
    {offset + itemCount < total ? <Link prefetch={false} className="pagination-step" href={attentionHref(current, keyName, offset + PAGE_SIZE)}>下一页</Link> : <span className="pagination-step pagination-step-disabled" aria-disabled="true">下一页</span>}
  </nav>;
}

function loadAttention(workspace: string, offsets: Offsets) {
  const reviewOffset = Number(offsets.reviewOffset), failureOffset = Number(offsets.failureOffset);
  const reviews = readSection(() => platform.reviews("PENDING", PAGE_SIZE, reviewOffset), p => validPage(p, PAGE_SIZE, reviewOffset, r => validReview(r, workspace)));
  const failures = readSection(() => platform.unresolvedFailedExecutions(PAGE_SIZE, failureOffset), p => validPage(p, PAGE_SIZE, failureOffset, e => validExecution(e, workspace) && e.status === "FAILED"));
  const products = readSection(() => collectAllPages(async (limit, offset) => {
    const p = await platform.products(limit, offset);
    if (!validPage(p, limit, offset, product => validProduct(product, workspace))) throw new Error("product labels unavailable");
    return p;
  }, 100), items => new Set(items.map(p => p.id)).size === items.length);
  const snapshot = readSection(() => platform.releaseAttention(PAGE_SIZE), r => validReleaseAttention(r, PAGE_SIZE));
  const releases = Object.fromEntries(releaseGroups.map(g => {
    const offset = Number(offsets[g.offset]);
    return [g.key, offset === 0 ? snapshot.then(s => s?.[g.key] ?? null) :
      readSection(() => platform.workspaceReleases(g.status, PAGE_SIZE, offset), p => validPage(p, PAGE_SIZE, offset, r => validRelease(r, g.status)))];
  })) as Record<ReleaseKind, Promise<Awaited<ReturnType<typeof platform.workspaceReleases>> | null>>;
  const counts = Object.fromEntries(releaseGroups.map(g => [g.key, releases[g.key].then(async p => p?.page.total ?? (await snapshot)?.[g.key].page.total ?? null)])) as Record<ReleaseKind, Promise<number | null>>;
  return { reviews, failures, products, releases, counts };
}
type Model = ReturnType<typeof loadAttention>;

async function Total({ model }: { model: Model }) {
  const [reviews, failures, failed, validating, ready] = await Promise.all([model.reviews, model.failures, model.counts.failed, model.counts.validating, model.counts.ready]);
  const total = attentionTotal([reviews?.page.total, failures?.page.total, failed, validating, ready]);
  return <span data-testid="attention-total">{total === null ? "审核、执行与发布：UNKNOWN · 未完整获取" : "审核、执行与发布：" + total + " 条"}</span>;
}

function FirstPage({ offsets, keyName, offset }: { offsets: Offsets; keyName: string; offset: number }) {
  return offset > 0 ? <Link prefetch={false} href={attentionHref(offsets, keyName, 0)}>返回该分区首页</Link> : null;
}

async function ReviewQueue({ model, offsets }: { model: Model; offsets: Offsets }) {
  const p = await model.reviews, offset = Number(offsets.reviewOffset);
  return <section className="panel attention-panel" data-testid="review-queue">
    <div className="panel-header"><div><h2>实体审核</h2><span className="panel-meta">{p ? p.page.total + " 条待人工决策" : "数量 UNKNOWN"}</span></div><Link prefetch={false} href="/reviews">进入审核队列</Link></div>
    {!p ? <><ReadUnavailable title="实体审核" /><FirstPage offsets={offsets} keyName="reviewOffset" offset={offset} /></> : <>
      {p.items.length === 0 ? <EmptyState title="本页无审核事项" description="使用下方分页返回有效页，或进入审核队列查看完整记录。" /> :
        <div className="table-card"><table className="data-table"><thead><tr><th>来源</th><th>匹配方式</th><th>置信度</th><th>状态</th></tr></thead><tbody>
          {p.items.map(r => <tr key={r.candidateId}><td className="primary-cell"><strong>{r.sourceName || r.sourceKey}</strong><span>{r.sourceKey}</span></td><td>{r.matchMethod || "—"}</td><td>{r.confidence.toFixed(2)}</td><td><Badge value={r.status} /></td></tr>)}
        </tbody></table></div>}
      <AttentionPager keyName="reviewOffset" offset={offset} itemCount={p.items.length} total={p.page.total} current={offsets} />
    </>}
  </section>;
}

async function FailureQueue({ model, offsets }: { model: Model; offsets: Offsets }) {
  const p = await model.failures, offset = Number(offsets.failureOffset);
  return <section className="panel attention-panel" data-testid="failure-queue">
    <div className="panel-header"><div><h2>生产执行失败</h2><span className="panel-meta">{p ? p.page.total + " 条未解决失败" : "数量 UNKNOWN"}</span></div><Link prefetch={false} href="/production">查看生产历史</Link></div>
    {!p ? <><ReadUnavailable title="未解决执行失败" /><FirstPage offsets={offsets} keyName="failureOffset" offset={offset} /></> : <>
      {p.items.length === 0 ? <EmptyState title="本页没有未解决失败" description="成功 retry 或正在 retry 的执行不会出现在这里。" /> :
        <div className="table-card"><table className="data-table"><thead><tr><th>Workflow</th><th>期间</th><th>错误</th><th>时间</th><th>处理</th></tr></thead><tbody>
          {p.items.map(e => <tr key={e.id}><td className="primary-cell"><strong>{e.workflowName || e.workflowCode}</strong><span>attempt {e.attempt} · {shortId(e.id)}</span></td>
            <td>{e.targetPeriod}</td><td><Badge value={e.errorCode || "FAILED"} /></td><td>{formatDate(e.finishedAt ?? e.createdAt)}</td><td><Link prefetch={false} className="text-link" href={"/production/" + e.id}>查看执行</Link></td></tr>)}
        </tbody></table></div>}
      <AttentionPager keyName="failureOffset" offset={offset} itemCount={p.items.length} total={p.page.total} current={offsets} />
    </>}
  </section>;
}

async function ProductLabel({ products, id }: { products: Promise<DataProduct[] | null>; id: string }) {
  const list = await products;
  return <>{list?.find(p => p.id === id)?.name ?? id + "（名称不可用）"}</>;
}

async function ReleaseQueue({ model, offsets, group }: { model: Model; offsets: Offsets; group: typeof releaseGroups[number] }) {
  const [p, count] = await Promise.all([model.releases[group.key], model.counts[group.key]]);
  const offset = Number(offsets[group.offset]);
  return <section className="panel attention-panel" data-testid={"release-queue-" + group.key}>
    <div className="panel-header"><div><h2>{group.title}</h2><span className="panel-meta">{count === null ? "数量 UNKNOWN" : count + " 个 Release"}</span></div>
      <Badge value={!p ? "UNKNOWN" : count ? group.key === "validating" ? "BLOCKED" : group.status : "CLEAR"} />
    </div>
    {!p ? <><ReadUnavailable title={group.title + "当页"} /><FirstPage offsets={offsets} keyName={group.offset} offset={offset} /></> : <>
      {p.items.length === 0 ? <EmptyState title={"本页没有" + group.title} description="仅说明本次读取的当页；可使用分页查看其他页。" /> :
        <div className="attention-list">{p.items.map(r => <div className="attention-item" key={r.id}>
          <div><strong><Suspense fallback={r.productId + "（名称读取中）"}><ProductLabel products={model.products} id={r.productId} /></Suspense> · {r.releaseNo}</strong>
            <span>{group.key === "validating" ? "等待解除 Release Readiness blockers" : formatDate(r.createdAt)}</span></div>
          <Link prefetch={false} className="text-link" href={group.key === "failed" ? "/products/" + r.productId + "/releases/" + r.id : "/products/" + r.productId + "#releases"}>{group.action}</Link>
        </div>)}</div>}
      <AttentionPager keyName={group.offset} offset={offset} itemCount={p.items.length} total={p.page.total} current={offsets} />
    </>}
  </section>;
}

export default async function AttentionPage({ searchParams }: { searchParams: Promise<Query> }) {
  const workspace = configuredWorkspaceId();
  if (!workspace) return <><PageHeader eyebrow="Attention" title="待办中心" /><SetupRequired /></>;
  const query = await searchParams;
  const offsets: Offsets = {
    schemaRequestView: schemaRequestView(query.schemaRequestView), schemaRequestOffset: attentionOffset(query.schemaRequestOffset),
    schemaExceptionStatus: schemaExceptionFilter(query.schemaExceptionStatus), schemaExceptionOffset: attentionOffset(query.schemaExceptionOffset),
    schemaView: schemaView(query.schemaView), schemaOffset: attentionOffset(query.schemaOffset), schemaAttemptOffset: attentionOffset(query.schemaAttemptOffset),
    qualityView: attentionView(query.qualityView), qualityOffset: attentionOffset(query.qualityOffset),
  };
  for (const name of ["reviewOffset", "failureOffset", "failedReleaseOffset", "validatingReleaseOffset", "readyReleaseOffset"]) offsets[name] = attentionOffset(query[name]);
  const model = loadAttention(workspace, offsets);
  return <>
    <PageHeader eyebrow="Attention" title="待办中心" description="各分区独立读取；历史异常与无观察请求不计为当前未恢复待办。" action={<Suspense fallback={<span>审核、执行与发布：UNKNOWN · 读取中</span>}><Total model={model} /></Suspense>} />
    <Suspense fallback={<section id="quality-attention" data-testid="quality-attention-pending" style={{ marginTop: 18 }}><ReadUnavailable title="质量异常汇总读取中" /></section>}><QualityAttention query={query} /></Suspense>
    <Suspense fallback={<section id="source-schema-attention" data-testid="schema-attention-pending" style={{ marginTop: 18 }}><ReadUnavailable title="源端 schema 漂移汇总读取中" /></section>}><SchemaAttention query={query} /></Suspense>
    <Suspense fallback={<section id="schema-exception-history" data-testid="schema-exceptions-pending" style={{ marginTop: 18 }}><ReadUnavailable title="源端 schema 扫描异常历史读取中" /></section>}><SchemaExceptions query={query} /></Suspense>
    <Suspense fallback={<section id="schema-unobserved-requests" data-testid="schema-requests-pending" style={{ marginTop: 18 }}><ReadUnavailable title="无观察请求读取中" /></section>}><SchemaRequests query={query} /></Suspense>
    <Suspense fallback={<section id="schema-unobserved-attempts" data-testid="schema-attempts-pending" style={{ marginTop: 18 }}><ReadUnavailable title="无观察 attempt 读取中" /></section>}><SchemaAttempts query={query} /></Suspense>
    <p className="panel-meta">各分区按各自读取结果展示，不是同一个冻结快照；产品名称不可用时使用已读取的对象 ID。</p>
    <div className="attention-grid">
      <Suspense fallback={<ReadUnavailable title="实体审核读取中" />}><ReviewQueue model={model} offsets={offsets} /></Suspense>
      <Suspense fallback={<ReadUnavailable title="未解决执行失败读取中" />}><FailureQueue model={model} offsets={offsets} /></Suspense>
      {releaseGroups.map(group => <Suspense key={group.key} fallback={<ReadUnavailable title={group.title + "读取中"} />}><ReleaseQueue model={model} offsets={offsets} group={group} /></Suspense>)}
    </div>
  </>;
}
