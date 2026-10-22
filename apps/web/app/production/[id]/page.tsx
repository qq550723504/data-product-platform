import Link from "next/link";
import { BackLink, Badge, DefinitionList, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { ExecutionRetryForm } from "@/components/execution-retry-form";
import { configuredWorkspaceId, platform } from "@/lib/platform";
import { batchEnabled, core } from "@/lib/batches";

function executionGuidance(status: string): string {
  switch (status) {
    case "FAILED":
      return "该 Execution 是不可变失败事实。Core Retry 会基于同一冻结输入创建新的 Execution，不会改写本记录。";
    case "CANCELLED":
      return "该 Execution 已取消且历史事实保持不变。Core Retry 会基于同一冻结输入创建新的 Execution。";
    case "QUEUED":
      return "Execution 已进入队列，等待 worker 认领；当前记录不需要人工修改状态。";
    case "SUBMITTING":
      return "Core 正在向托管执行引擎提交任务；提交结果由 reconciliation 更新。";
    case "RUNNING":
      return "Execution 正在运行；输出 DatasetVersion 只有在 Core 成功完成并冻结后才会出现。";
    case "SUCCEEDED":
      return "Execution 已成功完成；输出 DatasetVersion 是本次执行冻结的结果事实。";
    default:
      return "Execution 状态由 Core 状态机维护；UI 仅展示当前不可变/受控事实。";
  }
}

export default async function ExecutionDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const workspaceId = configuredWorkspaceId();
  if (!workspaceId) {
    return <><PageHeader title="Execution 详情" /><SetupRequired /></>;
  }
  const { id } = await params;

  try {
    const execution = await platform.execution(id);
    if (execution.workspaceId !== workspaceId) {
      throw new Error("Execution 不属于当前 POC Workspace");
    }
    const quarantine = batchEnabled() ? await core<{items:Array<{id:string;inputName:string;sourceKey:string;reasonCode:string;reasonMessage:string;payload:Record<string,unknown>}>;page:{total:number;offset:number;limit:number}}>(`/workspaces/${workspaceId}/executions/${execution.id}/quarantine`, "GET", undefined, undefined, true) : null;
    return (
      <>
        <BackLink href="/production">返回数据生产</BackLink>
        <PageHeader
          eyebrow={`Execution · ${shortId(execution.id)}`}
          title={`生产执行 ${execution.targetPeriod}`}
          description="Core 冻结本次执行的 WorkflowVersion、输入 DatasetVersion 和最终输出；外部执行 ID 只是运行时引用。"
          action={<Badge value={execution.status} />}
        />

        <section className="section-grid">
          <div className="detail-card">
            <h2>执行事实</h2>
            <DefinitionList items={[
              { label: "WorkflowVersion", value: <span className="mono">{shortId(execution.workflowVersionId)}</span> },
              { label: "Target Period", value: execution.targetPeriod },
              { label: "Attempt", value: execution.attempt },
              { label: "Engine Type", value: execution.engineType || "NATIVE" },
              { label: "Engine Execution", value: <span className="mono">{execution.engineExecutionId || "—"}</span> },
              {
                label: "Retry Of",
                value: execution.retryOfExecutionId
                  ? <Link className="text-link mono" href={`/production/${execution.retryOfExecutionId}`}>{shortId(execution.retryOfExecutionId)}</Link>
                  : "—",
              },
              { label: "开始时间", value: formatDate(execution.startedAt ?? execution.createdAt) },
              { label: "完成时间", value: formatDate(execution.finishedAt) },
            ]} />
            <div className={`callout ${execution.status === "FAILED" ? "callout-bad" : "callout-neutral"}`} style={{ marginTop: 16 }}>
              <strong>当前状态说明</strong>
              <p>{executionGuidance(execution.status)}</p>
            </div>
            {execution.errorCode || execution.errorMessage ? (
              <div className="callout callout-bad" style={{ marginTop: 12 }}>
                <strong>{execution.errorCode || "EXECUTION_FAILED"}</strong>
                <p>{execution.errorMessage || "执行失败"}</p>
              </div>
            ) : null}
            <ExecutionRetryForm
              executionId={execution.id}
              status={execution.status}
              enabled={process.env.POC_ENABLE_EXECUTION_ACTIONS === "true"}
            />
          </div>

          <aside className="detail-card">
            <h2>输出</h2>
            <div className="status-stack" style={{ marginTop: 12 }}>
              <div className="status-row"><span>Output Dataset</span><Link className="text-link mono" href={`/datasets/${execution.outputDatasetId}`}>{shortId(execution.outputDatasetId)}</Link></div>
              <div className="status-row">
                <span>Output Version</span>
                {execution.outputDatasetVersionId ? (
                  <Link className="text-link mono" href={`/datasets/${execution.outputDatasetId}/versions/${execution.outputDatasetVersionId}`}>
                    {shortId(execution.outputDatasetVersionId)}
                  </Link>
                ) : <span className="mono">—</span>}
              </div>
              <div className="status-row"><span>Status</span><Badge value={execution.status} /></div>
            </div>
          </aside>
        </section>

        <section className="detail-card" style={{ marginTop: 18 }}>
          <h2>冻结输入 DatasetVersion</h2>
          {execution.inputs.length === 0 ? <p>无输入版本。</p> : (
            <div className="table-card" style={{ marginTop: 14 }}>
              <table className="data-table">
                <thead><tr><th>端口</th><th>DatasetVersion</th></tr></thead>
                <tbody>{execution.inputs.map((input) => (
                  <tr key={`${input.name}-${input.datasetVersionId}`}>
                    <td>{input.name}</td>
                    <td>
                      {input.datasetId ? (
                        <Link className="text-link mono" href={`/datasets/${input.datasetId}/versions/${input.datasetVersionId}`}>
                          {input.datasetVersionId}
                        </Link>
                      ) : <span className="mono">{input.datasetVersionId}</span>}
                    </td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>

        {quarantine ? <section className="detail-card" style={{marginTop:18}}><h2>隔离记录</h2><p>共 {quarantine.page.total} 条 · 当前显示前 {quarantine.page.limit} 条。修复来源数据后创建新批次和加工执行；已有结果与隔离历史保留。</p>{quarantine.items.length ? <table className="data-table"><thead><tr><th>来源端口 / 记录</th><th>原因</th><th>原值</th></tr></thead><tbody>{quarantine.items.map(q=><tr key={q.id}><td>{q.inputName}<br/>{q.sourceKey}</td><td>{q.reasonCode}<br/>{q.reasonMessage}</td><td><details><summary>查看原记录</summary><pre className="json-preview">{JSON.stringify(q.payload,null,2)}</pre></details></td></tr>)}</tbody></table>:<p>没有隔离记录。</p>}</section>:null}

        {Object.keys(execution.metrics ?? {}).length > 0 ? (
          <section className="detail-card" style={{ marginTop: 18 }}>
            <h2>运行指标</h2>
            <pre className="json-preview">{JSON.stringify(execution.metrics, null, 2)}</pre>
          </section>
        ) : null}
      </>
    );
  } catch (error) {
    return <><BackLink href="/production">返回数据生产</BackLink><PageHeader title="Execution 详情" /><LoadError error={error} /></>;
  }
}
