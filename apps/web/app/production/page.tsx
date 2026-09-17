import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function ProductionPage() {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Production" title="数据生产" description="Core Workflow 与 Execution 运行历史。" /><SetupRequired /></>;
  }

  try {
    const executions = await platform.executions();
    return (
      <>
        <PageHeader
          eyebrow="Production"
          title="数据生产"
          description="一次 Execution 固定引用 WorkflowVersion 和输入 DatasetVersion；失败重试会创建新的 Execution，而不是改写旧记录。"
        />
        {executions.items.length === 0 ? (
          <EmptyState title="暂无生产执行" description="开始一次标准化、实体解析或加工任务后，执行历史会在这里保留。" />
        ) : (
          <div className="table-card">
            <table className="data-table">
              <thead><tr><th>Execution</th><th>Workflow</th><th>期间</th><th>状态</th><th>Attempt</th><th>输出 Dataset</th><th>开始时间</th></tr></thead>
              <tbody>
                {executions.items.map((execution) => (
                  <tr key={execution.id}>
                    <td><Link href={`/production/${execution.id}`} className="text-link mono">{shortId(execution.id)}</Link></td>
                    <td className="primary-cell"><strong>{execution.workflowName || execution.workflowCode}</strong><span>{execution.workflowCode} · v{execution.workflowVersion}</span></td>
                    <td>{execution.targetPeriod}</td>
                    <td><Badge value={execution.status} /></td>
                    <td>{execution.attempt}</td>
                    <td className="mono">{shortId(execution.outputDatasetId)}</td>
                    <td>{formatDate(execution.startedAt ?? execution.createdAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </>
    );
  } catch (error) {
    return <><PageHeader eyebrow="Production" title="数据生产" /><LoadError error={error} /></>;
  }
}
