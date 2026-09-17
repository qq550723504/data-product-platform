import Link from "next/link";
import { BackLink, Badge, DefinitionList, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function ExecutionDetailPage({ params }: { params: Promise<{ id: string }> }) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader title="Execution 详情" /><SetupRequired /></>;
  }
  const { id } = await params;

  try {
    const execution = await platform.execution(id);
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
              { label: "Retry Of", value: <span className="mono">{shortId(execution.retryOfExecutionId)}</span> },
              { label: "开始时间", value: formatDate(execution.startedAt ?? execution.createdAt) },
              { label: "完成时间", value: formatDate(execution.finishedAt) },
            ]} />
            {execution.errorCode || execution.errorMessage ? (
              <div className="callout callout-bad" style={{ marginTop: 16 }}>
                <strong>{execution.errorCode || "EXECUTION_FAILED"}</strong>
                <p>{execution.errorMessage || "执行失败"}</p>
              </div>
            ) : null}
          </div>

          <aside className="detail-card">
            <h2>输出</h2>
            <div className="status-stack" style={{ marginTop: 12 }}>
              <div className="status-row"><span>Output Dataset</span><Link className="text-link mono" href={`/datasets/${execution.outputDatasetId}`}>{shortId(execution.outputDatasetId)}</Link></div>
              <div className="status-row"><span>Output Version</span><span className="mono">{shortId(execution.outputDatasetVersionId)}</span></div>
              <div className="status-row"><span>Status</span><Badge value={execution.status} /></div>
            </div>
          </aside>
        </section>

        <section className="detail-card" style={{ marginTop: 18 }}>
          <h2>冻结输入 DatasetVersion</h2>
          {execution.inputs.length === 0 ? <p>无输入版本。</p> : (
            <div className="table-card" style={{ marginTop: 14 }}>
              <table className="data-table">
                <thead><tr><th>端口</th><th>DatasetVersion ID</th></tr></thead>
                <tbody>{execution.inputs.map((input) => (
                  <tr key={`${input.name}-${input.datasetVersionId}`}><td>{input.name}</td><td className="mono">{input.datasetVersionId}</td></tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>

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
