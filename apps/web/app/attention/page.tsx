import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

const PAGE_SIZE = 25;

type AttentionSearchParams = {
  reviewOffset?: string;
  failureOffset?: string;
  failedReleaseOffset?: string;
  validatingReleaseOffset?: string;
  readyReleaseOffset?: string;
};

function parseOffset(value?: string): number {
  const parsed = Number(value ?? 0);
  return Number.isSafeInteger(parsed) && parsed >= 0 ? Math.min(parsed, 2147483647) : 0;
}

function attentionHref(
  current: Record<string, number>,
  key: keyof AttentionSearchParams,
  offset: number,
): string {
  const query = new URLSearchParams();
  for (const [name, value] of Object.entries(current)) {
    if (value > 0) query.set(name, String(value));
  }
  if (offset > 0) query.set(key, String(offset));
  else query.delete(key);
  const encoded = query.toString();
  return encoded ? `/attention?${encoded}` : "/attention";
}

function AttentionPager({
  keyName,
  offset,
  itemCount,
  total,
  current,
}: {
  keyName: keyof AttentionSearchParams;
  offset: number;
  itemCount: number;
  total: number;
  current: Record<string, number>;
}) {
  if (total <= PAGE_SIZE && offset === 0) return null;
  const previous = offset > 0 ? Math.max(0, offset - PAGE_SIZE) : null;
  const next = offset + itemCount < total ? offset + PAGE_SIZE : null;
  return (
    <nav className="pagination attention-pagination" aria-label="待办分页">
      {previous === null ? (
        <span className="pagination-step pagination-step-disabled" aria-disabled="true">上一页</span>
      ) : (
        <Link className="pagination-step" href={attentionHref(current, keyName, previous)}>上一页</Link>
      )}
      <span className="pagination-summary">
        {total === 0 ? "0 条" : `${Math.min(offset + 1, total)}–${Math.min(offset + itemCount, total)} / ${total}`}
      </span>
      {next === null ? (
        <span className="pagination-step pagination-step-disabled" aria-disabled="true">下一页</span>
      ) : (
        <Link className="pagination-step" href={attentionHref(current, keyName, next)}>下一页</Link>
      )}
    </nav>
  );
}

export default async function AttentionPage({
  searchParams,
}: {
  searchParams: Promise<AttentionSearchParams>;
}) {
  if (!configuredWorkspaceId()) {
    return (
      <>
        <PageHeader
          eyebrow="Attention"
          title="待办中心"
          description="集中处理需要人工决策、故障跟进或发布动作的当前事项。"
        />
        <SetupRequired />
      </>
    );
  }

  const query = await searchParams;
  const reviewOffset = parseOffset(query.reviewOffset);
  const failureOffset = parseOffset(query.failureOffset);
  const failedReleaseOffset = parseOffset(query.failedReleaseOffset);
  const validatingReleaseOffset = parseOffset(query.validatingReleaseOffset);
  const readyReleaseOffset = parseOffset(query.readyReleaseOffset);
  const offsets = {
    reviewOffset,
    failureOffset,
    failedReleaseOffset,
    validatingReleaseOffset,
    readyReleaseOffset,
  };

  try {
    const [reviews, failures, products, releaseSnapshot] = await Promise.all([
      platform.reviews("PENDING", PAGE_SIZE, reviewOffset),
      platform.unresolvedFailedExecutions(PAGE_SIZE, failureOffset),
      platform.products(100, 0),
      platform.releaseAttention(PAGE_SIZE),
    ]);

    const [failedPage, validatingPage, readyPage] = await Promise.all([
      failedReleaseOffset === 0
        ? Promise.resolve(releaseSnapshot.failed)
        : platform.workspaceReleases("FAILED", PAGE_SIZE, failedReleaseOffset),
      validatingReleaseOffset === 0
        ? Promise.resolve(releaseSnapshot.validating)
        : platform.workspaceReleases("VALIDATING", PAGE_SIZE, validatingReleaseOffset),
      readyReleaseOffset === 0
        ? Promise.resolve(releaseSnapshot.ready)
        : platform.workspaceReleases("READY", PAGE_SIZE, readyReleaseOffset),
    ]);

    const releases = {
      failed: failedPage,
      validating: validatingPage,
      ready: readyPage,
    };
    const productById = new Map(products.items.map((product) => [product.id, product]));
    const total =
      reviews.page.total +
      failures.page.total +
      releases.failed.page.total +
      releases.validating.page.total +
      releases.ready.page.total;

    return (
      <>
        <PageHeader
          eyebrow="Attention"
          title="待办中心"
          description="这里只展示当前仍需处理的事项；已被成功 retry 解决的历史失败不会继续占用待办。"
          action={<Badge value={total ? "ACTION_REQUIRED" : "CLEAR"} />}
        />

        {total === 0 ? (
          <EmptyState
            title="当前没有待办"
            description="实体审核、未解决执行失败、Release Readiness 阻塞与待发布事项会集中出现在这里。"
          />
        ) : (
          <div className="attention-grid">
            <section className="panel attention-panel">
              <div className="panel-header">
                <div>
                  <h2>实体审核</h2>
                  <span className="panel-meta">{reviews.page.total} 条待人工决策</span>
                </div>
                <Link href="/reviews">进入审核队列</Link>
              </div>
              {reviews.items.length === 0 ? (
                <EmptyState title="本页无审核事项" description="使用下方分页返回有效页，或进入审核队列查看完整记录。" />
              ) : (
                <div className="table-card">
                  <table className="data-table">
                    <thead>
                      <tr><th>来源</th><th>匹配方式</th><th>置信度</th><th>状态</th></tr>
                    </thead>
                    <tbody>
                      {reviews.items.map((review) => (
                        <tr key={review.candidateId}>
                          <td className="primary-cell">
                            <strong>{review.sourceName || review.sourceKey}</strong>
                            <span>{review.sourceKey}</span>
                          </td>
                          <td>{review.matchMethod || "—"}</td>
                          <td>{Number.isFinite(review.confidence) ? review.confidence.toFixed(2) : "—"}</td>
                          <td><Badge value={review.status} /></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              <AttentionPager keyName="reviewOffset" offset={reviewOffset} itemCount={reviews.items.length} total={reviews.page.total} current={offsets} />
            </section>

            <section className="panel attention-panel">
              <div className="panel-header">
                <div>
                  <h2>生产执行失败</h2>
                  <span className="panel-meta">{failures.page.total} 条未解决失败</span>
                </div>
                <Link href="/production">查看生产历史</Link>
              </div>
              {failures.items.length === 0 ? (
                <EmptyState title="本页没有未解决失败" description="成功 retry 或正在 retry 的执行不会出现在这里。" />
              ) : (
                <div className="table-card">
                  <table className="data-table">
                    <thead>
                      <tr><th>Workflow</th><th>期间</th><th>错误</th><th>时间</th><th>处理</th></tr>
                    </thead>
                    <tbody>
                      {failures.items.map((execution) => (
                        <tr key={execution.id}>
                          <td className="primary-cell">
                            <strong>{execution.workflowName || execution.workflowCode}</strong>
                            <span>attempt {execution.attempt} · {shortId(execution.id)}</span>
                          </td>
                          <td>{execution.targetPeriod}</td>
                          <td><Badge value={execution.errorCode || "FAILED"} /></td>
                          <td>{formatDate(execution.finishedAt ?? execution.createdAt)}</td>
                          <td><Link className="text-link" href={`/production/${execution.id}`}>查看执行</Link></td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              <AttentionPager keyName="failureOffset" offset={failureOffset} itemCount={failures.items.length} total={failures.page.total} current={offsets} />
            </section>

            <section className="panel attention-panel">
              <div className="panel-header">
                <div>
                  <h2>Release 失败</h2>
                  <span className="panel-meta">{releases.failed.page.total} 个 Release</span>
                </div>
                <Badge value={releases.failed.page.total ? "FAILED" : "CLEAR"} />
              </div>
              {releases.failed.items.length === 0 ? (
                <EmptyState title="本页没有失败 Release" description="使用下方分页返回有效页。" />
              ) : (
                <div className="attention-list">
                  {releases.failed.items.map((release) => (
                    <div className="attention-item" key={release.id}>
                      <div>
                        <strong>{productById.get(release.productId)?.name ?? "Data Product"} · {release.releaseNo}</strong>
                        <span>{formatDate(release.createdAt)}</span>
                      </div>
                      <Link className="text-link" href={`/products/${release.productId}/releases/${release.id}`}>查看 Release</Link>
                    </div>
                  ))}
                </div>
              )}
              <AttentionPager keyName="failedReleaseOffset" offset={failedReleaseOffset} itemCount={releases.failed.items.length} total={releases.failed.page.total} current={offsets} />
            </section>

            <section className="panel attention-panel">
              <div className="panel-header">
                <div>
                  <h2>Readiness 阻塞</h2>
                  <span className="panel-meta">{releases.validating.page.total} 个 Release</span>
                </div>
                <Badge value={releases.validating.page.total ? "BLOCKED" : "CLEAR"} />
              </div>
              {releases.validating.items.length === 0 ? (
                <EmptyState title="本页没有 Readiness 阻塞" description="使用下方分页返回有效页。" />
              ) : (
                <div className="attention-list">
                  {releases.validating.items.map((release) => (
                    <div className="attention-item" key={release.id}>
                      <div>
                        <strong>{productById.get(release.productId)?.name ?? "Data Product"} · {release.releaseNo}</strong>
                        <span>等待解除 Release Readiness blockers</span>
                      </div>
                      <Link className="text-link" href={`/products/${release.productId}#releases`}>查看 Blockers</Link>
                    </div>
                  ))}
                </div>
              )}
              <AttentionPager keyName="validatingReleaseOffset" offset={validatingReleaseOffset} itemCount={releases.validating.items.length} total={releases.validating.page.total} current={offsets} />
            </section>

            <section className="panel attention-panel">
              <div className="panel-header">
                <div>
                  <h2>Release 待发布</h2>
                  <span className="panel-meta">{releases.ready.page.total} 个 Release</span>
                </div>
                <Badge value={releases.ready.page.total ? "READY" : "CLEAR"} />
              </div>
              {releases.ready.items.length === 0 ? (
                <EmptyState title="本页没有待发布 Release" description="使用下方分页返回有效页。" />
              ) : (
                <div className="attention-list">
                  {releases.ready.items.map((release) => (
                    <div className="attention-item" key={release.id}>
                      <div>
                        <strong>{productById.get(release.productId)?.name ?? "Data Product"} · {release.releaseNo}</strong>
                        <span>{formatDate(release.createdAt)}</span>
                      </div>
                      <Link className="text-link" href={`/products/${release.productId}#releases`}>去发布</Link>
                    </div>
                  ))}
                </div>
              )}
              <AttentionPager keyName="readyReleaseOffset" offset={readyReleaseOffset} itemCount={releases.ready.items.length} total={releases.ready.page.total} current={offsets} />
            </section>
          </div>
        )}
      </>
    );
  } catch (error) {
    return (
      <>
        <PageHeader eyebrow="Attention" title="待办中心" description="Core Read Model 暂时不可用。" />
        <LoadError error={error} />
      </>
    );
  }
}
