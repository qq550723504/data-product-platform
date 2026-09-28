import Link from "next/link";
import { Badge, EmptyState, LoadError, MetricCard, PageHeader, SetupRequired, formatDate } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function WorkbenchPage() {
  const workspaceId = configuredWorkspaceId();
  if (!workspaceId) {
    return (
      <>
        <PageHeader
          eyebrow="Workbench"
          title="数据产品工作台"
          description="从场景、资源、数据集和执行记录进入数据产品生产链，而不是从外部引擎开始。"
        />
        <SetupRequired />
      </>
    );
  }

  try {
    const [summary, executions, unresolvedFailures, products, releaseAttention] = await Promise.all([
      platform.workbench(),
      platform.executions(20, 0),
      platform.unresolvedFailedExecutions(20, 0),
      platform.products(50, 0),
      platform.releaseAttention(10),
    ]);
    const activeExecutions = summary.executions.queued + summary.executions.submitting + summary.executions.running;
    const failedExecutions = unresolvedFailures.items;
    const productById = new Map(products.items.map((product) => [product.id, product]));
    const failedReleases = releaseAttention.failed;
    const validatingReleases = releaseAttention.validating;
    const readyReleases = releaseAttention.ready;
    const failedRelease = failedReleases.items[0];
    const validatingRelease = validatingReleases.items[0];
    const readyRelease = readyReleases.items[0];
    const failedReleaseProduct = failedRelease ? productById.get(failedRelease.productId) : undefined;
    const validatingReleaseProduct = validatingRelease ? productById.get(validatingRelease.productId) : undefined;
    const readyReleaseProduct = readyRelease ? productById.get(readyRelease.productId) : undefined;
    const validatingReadiness = validatingRelease ? await platform.releaseReadiness(validatingRelease.id) : null;
    const attentionCount =
      summary.reviewQueue.pending +
      unresolvedFailures.page.total +
      failedReleases.page.total +
      validatingReleases.page.total +
      readyReleases.page.total;

    return (
      <>
        <PageHeader
          eyebrow="Workbench"
          title="数据产品工作台"
          description="观察资源、生产、实体审核与发布状态。当前 POC 聚焦企业经营活跃度这一条可追溯生产链。"
          action={<span className="mono">Workspace · {workspaceId.slice(0, 8)}…</span>}
        />

        <section className="metric-grid" aria-label="核心指标">
          <MetricCard label="数据资源" value={summary.counts.dataResources} hint="可被生产链引用的业务资源" />
          <MetricCard label="数据集" value={summary.counts.datasets} hint="RAW → STANDARDIZED → CURATED → PRODUCT" />
          <MetricCard label="数据产品" value={summary.counts.dataProducts} hint="稳定业务身份与版本历史" />
          <MetricCard label="待实体审核" value={summary.reviewQueue.pending} hint={`${summary.reviewQueue.unresolved} 条仍未解析`} />
          <MetricCard label="活跃执行" value={activeExecutions} hint={`${summary.executions.failed} 条失败记录`} />
          <MetricCard label="已发布 Release" value={summary.releases.published} hint={`${summary.releases.ready} 个已具备发布条件`} />
        </section>

        <section className="panel" style={{ marginTop: 18 }} data-testid="needs-attention">
          <div className="panel-header">
            <h2>Needs Attention</h2>
            <Badge value={attentionCount ? "ACTION_REQUIRED" : "CLEAR"} />
          </div>
          {attentionCount === 0 ? (
            <EmptyState
              title="当前没有需要人工处理的事项"
              description="实体审核、失败执行和 Release 阻塞项会集中出现在这里。"
            />
          ) : (
            <div className="table-card">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>事项</th>
                    <th>状态</th>
                    <th>说明</th>
                    <th>处理入口</th>
                  </tr>
                </thead>
                <tbody>
                  {summary.reviewQueue.pending > 0 ? (
                    <tr>
                      <td className="primary-cell">
                        <strong>实体审核队列</strong>
                        <span>{summary.reviewQueue.pending} 条待人工决策</span>
                      </td>
                      <td><Badge value="PENDING" /></td>
                      <td>
                        {summary.reviewQueue.unresolved} 条未解析
                        {summary.reviewQueue.conflicts ? ` · ${summary.reviewQueue.conflicts} 条冲突` : ""}
                      </td>
                      <td><Link className="text-link" href="/reviews">进入审核</Link></td>
                    </tr>
                  ) : null}

                  {unresolvedFailures.page.total > 0 ? (
                    <tr>
                      <td className="primary-cell">
                        <strong>生产执行失败</strong>
                        <span>{unresolvedFailures.page.total} 条未解决失败</span>
                      </td>
                      <td><Badge value="FAILED" /></td>
                      <td>
                        {failedExecutions[0]
                          ? `${failedExecutions[0].workflowName || failedExecutions[0].workflowCode} · ${failedExecutions[0].targetPeriod}${failedExecutions[0].errorCode ? ` · ${failedExecutions[0].errorCode}` : ""}`
                          : "查看执行列表定位失败原因"}
                      </td>
                      <td>
                        <Link
                          className="text-link"
                          href={failedExecutions[0] ? `/production/${failedExecutions[0].id}` : "/production"}
                        >
                          查看失败执行
                        </Link>
                      </td>
                    </tr>
                  ) : null}

                  {failedReleases.page.total > 0 ? (
                    <tr>
                      <td className="primary-cell">
                        <strong>Release 失败</strong>
                        <span>{failedReleases.page.total} 个 Release 处于失败状态</span>
                      </td>
                      <td><Badge value="FAILED" /></td>
                      <td>{failedRelease ? `${failedReleaseProduct?.name ?? "Data Product"} · ${failedRelease.releaseNo}` : "查看数据产品定位失败 Release"}</td>
                      <td>
                        <Link
                          className="text-link"
                          href={failedRelease
                            ? `/products/${failedRelease.productId}/releases/${failedRelease.id}`
                            : "/products"}
                        >
                          查看 Release
                        </Link>
                      </td>
                    </tr>
                  ) : null}

                  {validatingReleases.page.total > 0 ? (
                    <tr>
                      <td className="primary-cell">
                        <strong>Release Readiness 阻塞</strong>
                        <span>{validatingReleases.page.total} 个 Release 仍在 VALIDATING</span>
                      </td>
                      <td><Badge value="BLOCKED" /></td>
                      <td>
                        {validatingReadiness?.blockers.length
                          ? validatingReadiness.blockers.slice(0, 3).join(" · ")
                          : validatingRelease
                            ? `${validatingReleaseProduct?.name ?? "Data Product"} · ${validatingRelease.releaseNo}`
                            : "查看 Release Readiness"}
                      </td>
                      <td>
                        <Link
                          className="text-link"
                          href={validatingRelease
                            ? `/products/${validatingRelease.productId}#releases`
                            : "/products"}
                        >
                          查看 Blockers
                        </Link>
                      </td>
                    </tr>
                  ) : null}

                  {readyReleases.page.total > 0 ? (
                    <tr>
                      <td className="primary-cell">
                        <strong>Release 待发布</strong>
                        <span>{readyReleases.page.total} 个 Release 已通过 Readiness</span>
                      </td>
                      <td><Badge value="READY" /></td>
                      <td>{readyRelease ? `${readyReleaseProduct?.name ?? "Data Product"} · ${readyRelease.releaseNo}` : "已有 Release 可进入发布动作"}</td>
                      <td>
                        <Link
                          className="text-link"
                          href={readyRelease
                            ? `/products/${readyRelease.productId}#releases`
                            : "/products"}
                        >
                          去发布
                        </Link>
                      </td>
                    </tr>
                  ) : null}
                </tbody>
              </table>
            </div>
          )}
        </section>

        <section className="section-grid">
          <div className="panel">
            <div className="panel-header">
              <h2>最近生产执行</h2>
              <Link href="/production">查看全部</Link>
            </div>
            {executions.items.length === 0 ? (
              <EmptyState title="尚无执行记录" description="创建 Workflow Execution 后，这里会出现输入版本、运行状态与输出版本。" />
            ) : (
              <div className="table-card">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>Workflow</th>
                      <th>期间</th>
                      <th>状态</th>
                      <th>创建时间</th>
                    </tr>
                  </thead>
                  <tbody>
                    {executions.items.map((execution) => (
                      <tr key={execution.id}>
                        <td className="primary-cell">
                          <Link className="text-link" href={`/production/${execution.id}`}>
                            <strong>{execution.workflowName || execution.workflowCode}</strong>
                          </Link>
                          <span>v{execution.workflowVersion} · attempt {execution.attempt}</span>
                        </td>
                        <td>{execution.targetPeriod}</td>
                        <td><Badge value={execution.status} /></td>
                        <td>{formatDate(execution.createdAt)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>

          <aside className="panel">
            <div className="panel-header"><h2>参考生产链</h2></div>
            <div className="flow" aria-label="数据产品生产流程">
              {[
                "Data Resource",
                "Raw Dataset",
                "标准化",
                "实体解析",
                "加工",
                "质量/合规",
                "Data Contract",
                "Data Product",
                "Release",
              ].map((step, index, steps) => (
                <span key={step} className="flow">
                  <span className="flow-step">{step}</span>
                  {index < steps.length - 1 ? <span className="flow-arrow">→</span> : null}
                </span>
              ))}
            </div>
            <div className="status-stack" style={{ marginTop: 18 }}>
              <div className="status-row"><span>实体审核队列</span><Badge value={summary.reviewQueue.pending ? "PENDING" : "READY"} /></div>
              <div className="status-row"><span>生产执行</span><Badge value={activeExecutions ? "RUNNING" : "READY"} /></div>
              <div className="status-row"><span>Release</span><Badge value={summary.releases.failed ? "FAILED" : "READY"} /></div>
            </div>
          </aside>
        </section>
      </>
    );
  } catch (error) {
    return (
      <>
        <PageHeader eyebrow="Workbench" title="数据产品工作台" description="Core Read Model 暂时不可用。" />
        <LoadError error={error} />
      </>
    );
  }
}
