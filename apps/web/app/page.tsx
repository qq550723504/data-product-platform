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
    const [summary, executions] = await Promise.all([platform.workbench(), platform.executions(5, 0)]);
    const activeExecutions = summary.executions.queued + summary.executions.submitting + summary.executions.running;

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
