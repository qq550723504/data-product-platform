import Link from "next/link";
import {
  BackLink,
  Badge,
  DefinitionList,
  EmptyState,
  LoadError,
  PageHeader,
  SetupRequired,
  formatDate,
  shortId,
} from "@/components/ui";
import { configuredActorId, configuredWorkspaceId, platform } from "@/lib/platform";
import { publishReleaseAction } from "./actions";

const readinessChecks = [
  ["production", "Production", "目标生产结果已形成"],
  ["dataset", "Dataset", "绑定版本可用且不可变"],
  ["rights", "Rights", "权利快照允许产品化与发布"],
  ["quality", "Quality", "质量门已通过"],
  ["compliance", "Compliance", "合规门已通过"],
  ["contract", "Contract", "Data Contract 已发布且匹配"],
  ["evidence", "Evidence", "发布所需证据完整"],
  ["delivery", "Delivery", "存在可交付 ProductAsset"],
] as const;

export default async function ReleaseDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string; releaseId: string }>;
  searchParams: Promise<{ result?: string; error?: string }>;
}) {
  const workspaceId = configuredWorkspaceId();
  if (!workspaceId) return <><PageHeader title="ProductRelease 详情" /><SetupRequired /></>;
  const actorConfigured = Boolean(configuredActorId());
  const { id: productId, releaseId } = await params;
  const notice = await searchParams;

  try {
    const [product, release, readiness, trace] = await Promise.all([
      platform.product(productId),
      platform.release(releaseId),
      platform.readiness(releaseId),
      platform.releaseTrace(releaseId),
    ]);
    if (product.workspaceId !== workspaceId) throw new Error("DataProduct 不属于当前 POC Workspace");
    if (release.productId !== productId || trace.productId !== productId) throw new Error("ProductRelease 与 DataProduct 关系不一致");

    const canPublish = release.status === "READY" && readiness.overall === "READY";

    return (
      <>
        <BackLink href={`/products/${productId}`}>返回 {product.name}</BackLink>
        <PageHeader
          eyebrow={`ProductRelease · ${release.releaseNo}`}
          title="发布快照与可信证据"
          description="Release 把 ProductVersion、实际 DatasetVersion、权利、质量、合规与证据快照冻结在同一历史事实中。"
          action={<div className="badge-row"><Badge value={release.status} /><Badge value={readiness.overall} /></div>}
        />

        {notice.result ? <div className="callout callout-good"><strong>发布完成</strong><p>{notice.result}</p></div> : null}
        {notice.error ? <div className="callout callout-bad"><strong>操作未完成</strong><p>{notice.error}</p></div> : null}

        <section className="section-grid">
          <div className="detail-card">
            <h2>Release 定义</h2>
            <DefinitionList items={[
              { label: "Release No", value: release.releaseNo },
              { label: "ProductVersion", value: <span className="mono">{shortId(release.productVersionId)}</span> },
              { label: "ContractVersion", value: <span className="mono">{shortId(release.contractVersionId)}</span> },
              { label: "RightsSnapshot", value: <span className="mono">{shortId(release.rightsSnapshotId)}</span> },
              { label: "QualityResult", value: <span className="mono">{shortId(release.qualityResultId)}</span> },
              { label: "ComplianceResult", value: <span className="mono">{shortId(release.complianceResultId)}</span> },
              { label: "EvidenceSnapshot", value: <span className="mono">{shortId(release.evidenceSnapshotId)}</span> },
              { label: "发布时间", value: formatDate(release.releasedAt) },
            ]} />
          </div>
          <aside className="detail-card publish-card">
            <div className="panel-header"><h2>发布控制</h2><Badge value={readiness.overall} /></div>
            <p className="muted-copy">只有所有独立 Gate 都为 PASS、Release 状态为 READY 时，发布动作才会开放。</p>
            {release.status === "PUBLISHED" ? (
              <div className="callout callout-good"><strong>已发布</strong><p>该 Release 已成为不可变发布事实；后续变化应创建新的 Release。</p></div>
            ) : (
              <form action={publishReleaseAction}>
                <input type="hidden" name="productId" value={productId} />
                <input type="hidden" name="releaseId" value={releaseId} />
                <button className="button button-primary button-full" type="submit" disabled={!canPublish || !actorConfigured}>
                  发布 ProductRelease
                </button>
              </form>
            )}
            {!actorConfigured ? <p className="form-help">需要服务端 <code>POC_ACTOR_ID</code> 才能执行审计发布。</p> : null}
            {!canPublish && release.status !== "PUBLISHED" ? <p className="form-help">当前仍有 Readiness 阻断项，按钮保持禁用。</p> : null}
          </aside>
        </section>

        <section className="detail-card" style={{ marginTop: 18 }}>
          <div className="panel-header"><h2>Release Readiness</h2><Badge value={readiness.overall} /></div>
          <div className="readiness-grid">
            {readinessChecks.map(([key, label, description]) => (
              <div className="readiness-check" key={key}>
                <div><strong>{label}</strong><span>{description}</span></div>
                <Badge value={readiness.checks[key] ?? "PENDING"} />
              </div>
            ))}
          </div>
          {readiness.blockers.length > 0 ? (
            <div className="callout callout-warn" style={{ marginTop: 16 }}>
              <strong>阻断项</strong>
              <div className="blocker-list">{readiness.blockers.map((blocker) => <code key={blocker}>{blocker}</code>)}</div>
            </div>
          ) : null}
          {readiness.details && Object.keys(readiness.details).length > 0 ? <pre className="json-preview">{JSON.stringify(readiness.details, null, 2)}</pre> : null}
        </section>

        <section className="detail-card" style={{ marginTop: 18 }}>
          <div className="panel-header"><h2>DatasetVersion 血缘</h2><span className="eyebrow">{trace.datasetVersions.length} Versions</span></div>
          {trace.datasetVersions.length === 0 ? <EmptyState title="暂无版本血缘" description="Release 尚未绑定 DatasetVersion。" /> : (
            <div className="table-card">
              <table className="data-table">
                <thead><tr><th>Dataset</th><th>类型</th><th>版本</th><th>Release Role</th><th>状态</th><th>生成 Execution</th><th>Checksum</th></tr></thead>
                <tbody>{trace.datasetVersions.map((dataset) => (
                  <tr key={dataset.id}>
                    <td className="primary-cell"><Link className="text-link" href={`/datasets/${dataset.datasetId}`}><strong>{dataset.datasetCode}</strong></Link><span className="mono">{shortId(dataset.id)}</span></td>
                    <td><Badge value={dataset.datasetType} tone="info" /></td>
                    <td>v{dataset.versionNo}</td>
                    <td>{dataset.releaseRole || "UPSTREAM"}</td>
                    <td><Badge value={dataset.status} /></td>
                    <td>{dataset.generatedByExecutionId ? <Link className="text-link mono" href={`/production/${dataset.generatedByExecutionId}`}>{shortId(dataset.generatedByExecutionId)}</Link> : "—"}</td>
                    <td className="mono">{dataset.checksumValue ? `${dataset.checksumValue.slice(0, 12)}…` : "—"}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>

        <section className="detail-card" style={{ marginTop: 18 }}>
          <div className="panel-header"><h2>Execution Trace</h2><span className="eyebrow">{trace.executions.length} Executions</span></div>
          {trace.executions.length === 0 ? <EmptyState title="无生产执行" description="当前 Release 血缘中没有由 Workflow Execution 生成的版本。" /> : (
            <div className="table-card">
              <table className="data-table">
                <thead><tr><th>Execution</th><th>WorkflowVersion</th><th>期间</th><th>Engine</th><th>状态</th><th>Attempt</th></tr></thead>
                <tbody>{trace.executions.map((execution) => (
                  <tr key={execution.id}>
                    <td><Link className="text-link mono" href={`/production/${execution.id}`}>{shortId(execution.id)}</Link></td>
                    <td className="primary-cell"><strong>{execution.workflowVersion}</strong><span className="mono">{execution.workflowDefinitionHash ? `${execution.workflowDefinitionHash.slice(0, 12)}…` : "—"}</span></td>
                    <td>{execution.targetPeriod}</td>
                    <td>{execution.engineType}</td>
                    <td><Badge value={execution.status} /></td>
                    <td>{execution.attempt}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>

        <section className="governance-grid">
          <div className="detail-card">
            <div className="panel-header"><h2>Entity Resolution</h2><span className="eyebrow">Trace</span></div>
            <strong className="large-number">{trace.entityMappings.length}</strong>
            <p className="muted-copy">{trace.entityMatchJobs.length} 个 MatchJob，保留策略版本、人工复核和映射证据。</p>
          </div>
          <div className="detail-card">
            <div className="panel-header"><h2>Cost Events</h2><span className="eyebrow">Ledger</span></div>
            <strong className="large-number">{trace.costEvents.length}</strong>
            <p className="muted-copy">生产执行成本与数据产品经济分析可追溯，但不直接等同于会计资本化金额。</p>
          </div>
          <div className="detail-card">
            <div className="panel-header"><h2>Audit Events</h2><span className="eyebrow">History</span></div>
            <strong className="large-number">{trace.auditEvents.length}</strong>
            <p className="muted-copy">谁在什么时间做了什么决策，与 Evidence 的“证明什么”保持分离。</p>
          </div>
        </section>

        <section className="detail-card" style={{ marginTop: 18 }}>
          <div className="panel-header"><h2>Evidence</h2><span className="eyebrow">{trace.evidence.length} Records</span></div>
          {trace.evidenceSnapshot ? (
            <div className="snapshot-banner">
              <div><span>EvidenceSnapshot</span><strong className="mono">{shortId(trace.evidenceSnapshot.id)}</strong></div>
              <div><span>Root Hash</span><strong className="mono">{trace.evidenceSnapshot.rootHash ? `${trace.evidenceSnapshot.rootHash.slice(0, 18)}…` : "—"}</strong></div>
              <Badge value={trace.evidenceSnapshot.integrityValid ? "PASS" : "FAIL"} />
            </div>
          ) : null}
          {trace.evidence.length === 0 ? <EmptyState title="暂无 Evidence" description="加工、质量、合规、审核与发布证据会沿对象关系进入 Evidence Graph。" /> : (
            <div className="table-card" style={{ marginTop: 14 }}>
              <table className="data-table">
                <thead><tr><th>证据</th><th>类型</th><th>来源</th><th>关系</th><th>完整性</th><th>时间</th></tr></thead>
                <tbody>{trace.evidence.map((evidence) => (
                  <tr key={`${evidence.id}-${evidence.relationType}`}>
                    <td className="primary-cell"><strong>{evidence.title || evidence.evidenceType}</strong><span className="mono">{shortId(evidence.id)}</span></td>
                    <td>{evidence.evidenceType}</td>
                    <td>{evidence.sourceType || "—"} <span className="mono">{shortId(evidence.sourceId)}</span></td>
                    <td>{evidence.relationType}</td>
                    <td><Badge value={evidence.integrityValid ? "PASS" : "FAIL"} /></td>
                    <td>{formatDate(evidence.createdAt)}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>
      </>
    );
  } catch (error) {
    return <><BackLink href={`/products/${productId}`}>返回数据产品</BackLink><PageHeader title="ProductRelease 详情" /><LoadError error={error} /></>;
  }
}
