import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function AttentionPage() {
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

  try {
    const [reviews, failures, products, releases] = await Promise.all([
      platform.reviews("PENDING", 50, 0),
      platform.unresolvedFailedExecutions(50, 0),
      platform.products(100, 0),
      platform.releaseAttention(50),
    ]);

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
                <EmptyState title="本页无审核事项" description="进入审核队列查看完整分页。" />
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
                <EmptyState title="没有未解决失败" description="成功 retry 或正在 retry 的执行不会出现在这里。" />
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
                <EmptyState title="没有失败 Release" description="失败 Release 会保留在这里用于定位证据与发布问题。" />
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
                <EmptyState title="没有 Readiness 阻塞" description="VALIDATING 且未通过门禁的 Release 会出现在这里。" />
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
                <EmptyState title="没有待发布 Release" description="通过 Readiness 的 Release 会在这里等待发布动作。" />
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
