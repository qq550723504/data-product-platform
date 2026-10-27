import {businessText,displayText} from '../../../lib/business-language.mjs';
import Link from "next/link";
import { BackLink, Badge, DefinitionList, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { ExecutionRetryForm } from "@/components/execution-retry-form";
import { configuredWorkspaceId, platform } from "@/lib/platform";
import { batchEnabled, core } from "@/lib/batches";

function executionGuidance(status: string): string {
  switch (status) {
    case "FAILED":
      return "本次失败记录会保留。重试将沿用相同的固定输入并创建新的运行记录。";
    case "CANCELLED":
      return "本次运行已取消，历史记录会保留。重试将沿用相同的固定输入并创建新的运行记录。";
    case "QUEUED":
      return "Execution 已进入队列，等待 worker 认领；当前记录不需要人工修改状态。";
    case "SUBMITTING":
      return "正在提交任务，提交结果将由后续核实更新。";
    case "RUNNING":
      return "任务正在运行，成功完成并确认输出后才会生成成果版本。";
    case "SUCCEEDED":
      return "Execution 已成功完成；输出 DatasetVersion 是本次执行冻结的结果事实。";
    default:
      return "当前运行状态由系统维护，历史结果会保留。";
  }
}

export default async function ExecutionDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const workspaceId = configuredWorkspaceId();
  if (!workspaceId) {
    return <><PageHeader title="运行记录 详情" /><SetupRequired /></>;
  }
  const { id } = await params;

  try {
    const execution = await platform.execution(id);
    if (execution.workspaceId !== workspaceId) {
      throw new Error("本次运行不属于当前工作空间");
    }
    const quarantine = batchEnabled() ? await core<{items:Array<{id:string;inputName:string;sourceKey:string;reasonCode:string;reasonMessage:string;payload:Record<string,unknown>}>;page:{total:number;offset:number;limit:number}}>(`/workspaces/${workspaceId}/executions/${execution.id}/quarantine`, "GET", undefined, undefined, true) : null;
    return (
      <>
        <BackLink href="/production">返回数据生产</BackLink>
        <PageHeader
          eyebrow={`Execution · ${shortId(execution.id)}`}
          title={`生产执行 ${execution.targetPeriod}`}
          description="本次运行固定工作流版本、输入版本和最终输出；外部运行编号仅用于追溯。"
          action={<Badge value={execution.status} />}
        />

        <section className="section-grid">
          <div className="detail-card">
            <h2>执行事实</h2>
            <DefinitionList items={[
              { label: "WorkflowVersion", value: <span className="mono">{displayText(shortId(execution.workflowVersionId))}</span> },
              { label: "Target Period", value: execution.targetPeriod },
              { label: "Attempt", value: execution.attempt },
              { label: "Engine Type", value: execution.engineType || "NATIVE" },
              { label: "Engine Execution", value: <span className="mono">{displayText(execution.engineExecutionId || "—")}</span> },
              {
                label: "Retry Of",
                value: execution.retryOfExecutionId
                  ? <Link className="text-link mono" href={`/production/${execution.retryOfExecutionId}`}>{displayText(shortId(execution.retryOfExecutionId))}</Link>
                  : "—",
              },
              { label: "开始时间", value: formatDate(execution.startedAt ?? execution.createdAt) },
              { label: "完成时间", value: formatDate(execution.finishedAt) },
            ]} />
            <div className={`callout ${execution.status === "FAILED" ? "callout-bad" : "callout-neutral"}`} style={{ marginTop: 16 }}>
              <strong>当前状态说明</strong>
              <p>{businessText(executionGuidance(execution.status))}</p>
            </div>
            {execution.errorCode || execution.errorMessage ? (
              <div className="callout callout-bad" style={{ marginTop: 12 }}>
                <strong>{displayText(execution.errorCode || "EXECUTION_FAILED")}</strong>
                <p>{displayText(execution.errorMessage || "执行失败")}</p>
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
              <div className="status-row"><span>Output 数据集</span><Link className="text-link mono" href={`/datasets/${execution.outputDatasetId}`}>{displayText(shortId(execution.outputDatasetId))}</Link></div>
              <div className="status-row">
                <span>Output Version</span>
                {execution.outputDatasetVersionId ? (
                  <Link className="text-link mono" href={`/datasets/${execution.outputDatasetId}/versions/${execution.outputDatasetVersionId}`}>
                    {displayText(shortId(execution.outputDatasetVersionId))}
                  </Link>
                ) : <span className="mono">—</span>}
              </div>
              <div className="status-row"><span>Status</span><Badge value={execution.status} /></div>
            </div>
          </aside>
        </section>

        <section className="detail-card" style={{ marginTop: 18 }}>
          <h2>冻结输入 成果版本</h2>
          {execution.inputs.length === 0 ? <p>无输入版本。</p> : (
            <div className="table-card" style={{ marginTop: 14 }}>
              <table className="data-table">
                <thead><tr><th>端口</th><th>成果版本</th></tr></thead>
                <tbody>{execution.inputs.map((input) => (
                  <tr key={`${input.name}-${input.datasetVersionId}`}>
                    <td>{businessText(input.name)}</td>
                    <td>
                      {input.datasetId ? (
                        <Link className="text-link mono" href={`/datasets/${input.datasetId}/versions/${input.datasetVersionId}`}>
                          {displayText(input.datasetVersionId)}
                        </Link>
                      ) : <span className="mono">{displayText(input.datasetVersionId)}</span>}
                    </td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>

        {quarantine ? <section className="detail-card" style={{marginTop:18}}><h2>隔离记录</h2><p>共 {displayText(quarantine.page.total)} 条 · 当前显示前 {displayText(quarantine.page.limit)} 条。修复来源数据后创建新批次和加工执行；已有结果与隔离历史保留。</p>{quarantine.items.length ? <table className="data-table"><thead><tr><th>来源端口 / 记录</th><th>原因</th><th>原值</th></tr></thead><tbody>{quarantine.items.map(q=><tr key={q.id}><td>{businessText(q.inputName)}<br/>{displayText(q.sourceKey)}</td><td>{displayText(q.reasonCode)}<br/>{displayText(q.reasonMessage)}</td><td><details><summary>查看原记录</summary><pre className="json-preview">{JSON.stringify(q.payload,null,2)}</pre></details></td></tr>)}</tbody></table>:<p>没有隔离记录。</p>}</section>:null}

        {displayText(Object.keys(execution.metrics ?? {}).length > 0 ? (
          <section className="detail-card" style={{ marginTop: 18 }}>
            <h2>运行指标</h2>
            <pre className="json-preview">{JSON.stringify(execution.metrics, null, 2)}</pre>
          </section>
        ) : null)}
      </>
    );
  } catch (error) {
    return <><BackLink href="/production">返回数据生产</BackLink><PageHeader title="运行记录 详情" /><LoadError error={error} /></>;
  }
}
