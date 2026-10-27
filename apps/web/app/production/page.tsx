import {displayText} from '../../lib/business-language.mjs';
import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, Pagination, SetupRequired, formatDate, shortId } from "@/components/ui";
import { LIST_PAGE_SIZE, parsePageOffset } from "@/lib/pagination";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function ProductionPage({ searchParams }: { searchParams: Promise<{ offset?: string }> }) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Production" title="数据生产" description="查看工作流及其任务运行历史。" /><SetupRequired /></>;
  }

  const offset = parsePageOffset((await searchParams).offset);
  try {
    const executions = await platform.executions(LIST_PAGE_SIZE, offset);
    return (
      <>
        <PageHeader
          eyebrow="Production"
          title="数据生产"
          description="一次 运行记录 固定引用 工作流版本 和输入 成果版本；失败重试会创建新的 运行记录，而不是改写旧记录。"
        />
        {executions.page.total === 0 ? (
          <EmptyState title="暂无生产执行" description="开始一次标准化、实体解析或加工任务后，执行历史会在这里保留。" />
        ) : (
          <>
            {executions.items.length === 0 ? (
              <div className="callout callout-warn"><strong>该页没有记录</strong><p>当前起始位置超出数据范围，请使用下方分页返回有效页。</p></div>
            ) : (
              <div className="table-card">
                <table className="data-table">
                  <thead><tr><th>运行记录</th><th>工作流</th><th>期间</th><th>状态</th><th>评测记录</th><th>输出 数据集</th><th>开始时间</th></tr></thead>
                  <tbody>
                    {executions.items.map((execution) => (
                      <tr key={execution.id}>
                        <td><Link href={`/production/${execution.id}`} className="text-link mono">{displayText(shortId(execution.id))}</Link></td>
                        <td className="primary-cell"><strong>{displayText(execution.workflowName || execution.workflowCode)}</strong><span>{displayText(execution.workflowCode)} · v{displayText(execution.workflowVersion)}</span></td>
                        <td>{displayText(execution.targetPeriod)}</td>
                        <td><Badge value={execution.status} /></td>
                        <td>{displayText(execution.attempt)}</td>
                        <td className="mono">{displayText(shortId(execution.outputDatasetId))}</td>
                        <td>{displayText(formatDate(execution.startedAt ?? execution.createdAt))}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
            <Pagination basePath="/production" offset={executions.page.offset} itemCount={executions.items.length} total={executions.page.total} pageSize={LIST_PAGE_SIZE} label="生产执行分页" />
          </>
        )}
      </>
    );
  } catch (error) {
    return <><PageHeader eyebrow="Production" title="数据生产" /><LoadError error={error} /></>;
  }
}
