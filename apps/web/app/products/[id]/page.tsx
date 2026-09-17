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
import { configuredWorkspaceId, platform, type ReleaseReadiness, type ReleaseTrace } from "@/lib/platform";

const sections = [
  ["dataset", "Dataset"],
  ["workflow", "Workflow"],
  ["contract", "Contract"],
  ["rights", "Rights"],
  ["quality", "Quality"],
  ["compliance", "Compliance"],
  ["cost", "Cost"],
  ["evidence", "Evidence"],
  ["releases", "Releases"],
] as const;

async function optionalReleaseContext(releaseId?: string): Promise<{ readiness: ReleaseReadiness | null; trace: ReleaseTrace | null }> {
  if (!releaseId) return { readiness: null, trace: null };
  const [readiness, trace] = await Promise.all([
    platform.readiness(releaseId).catch(() => null),
    platform.releaseTrace(releaseId).catch(() => null),
  ]);
  return { readiness, trace };
}

function check(readiness: ReleaseReadiness | null, name: string): string {
  return readiness?.checks?.[name] ?? "PENDING";
}

export default async function ProductDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const workspaceId = configuredWorkspaceId();
  if (!workspaceId) return <><PageHeader title="数据产品详情" /><SetupRequired /></>;
  const { id } = await params;

  try {
    const [product, releases] = await Promise.all([platform.product(id), platform.releases(id)]);
    if (product.workspaceId !== workspaceId) throw new Error("DataProduct 不属于当前 POC Workspace");

    const version = product.currentVersionId ? await platform.productVersion(product.currentVersionId) : null;
    const latestRelease = releases.items[0];
    const { readiness, trace } = await optionalReleaseContext(latestRelease?.id);

    return (
      <>
        <BackLink href="/products">返回数据产品</BackLink>
        <PageHeader
          eyebrow={product.code}
          title={product.name}
          description={product.description || "DataProduct 是稳定业务身份；版本和 Release 保留不可变生产事实。"}
          action={<div className="badge-row"><Badge value={product.lifecycleStatus} /><Badge value={product.healthStatus} /></div>}
        />

        <nav className="section-tabs" aria-label="数据产品详情分区">
          {sections.map(([href, label]) => <a href={`#${href}`} key={href}>{label}</a>)}
        </nav>

        <section className="section-grid" style={{ marginTop: 18 }}>
          <div className="detail-card">
            <h2>产品定义</h2>
            <DefinitionList items={[
              { label: "业务域", value: product.domainCode || "—" },
              { label: "Current Version", value: version?.version || "—" },
              { label: "Owner", value: shortId(product.ownerId) },
              { label: "Use Case", value: shortId(product.useCaseId) },
              { label: "Project", value: shortId(product.projectId) },
              { label: "创建时间", value: formatDate(product.createdAt) },
            ]} />
          </div>
          <aside className="detail-card">
            <h2>最新 Release</h2>
            {latestRelease ? (
              <div className="status-stack" style={{ marginTop: 12 }}>
                <div className="status-row"><span>Release</span><Link className="text-link" href={`/products/${id}/releases/${latestRelease.id}`}>{latestRelease.releaseNo}</Link></div>
                <div className="status-row"><span>Status</span><Badge value={latestRelease.status} /></div>
                <div className="status-row"><span>Readiness</span><Badge value={readiness?.overall ?? "PENDING"} /></div>
                <div className="status-row"><span>Evidence</span><span>{trace?.evidence.length ?? 0}</span></div>
              </div>
            ) : <p className="muted-copy">尚未创建 ProductRelease。</p>}
          </aside>
        </section>

        <section className="detail-card anchored-section" id="dataset">
          <div className="panel-header"><h2>Dataset / Delivery Assets</h2><Badge value={check(readiness, "dataset")} /></div>
          {version?.assets?.length ? (
            <div className="table-card">
              <table className="data-table">
                <thead><tr><th>Asset</th><th>类型</th><th>Dataset</th><th>外部引用</th></tr></thead>
                <tbody>{version.assets.map((asset) => (
                  <tr key={asset.id}>
                    <td className="primary-cell"><strong>{asset.name}</strong><span className="mono">{shortId(asset.id)}</span></td>
                    <td><Badge value={asset.assetType} tone="info" /></td>
                    <td>{asset.datasetId ? <Link className="text-link mono" href={`/datasets/${asset.datasetId}`}>{shortId(asset.datasetId)}</Link> : "—"}</td>
                    <td className="mono">{asset.externalRef || "—"}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          ) : <EmptyState title="当前版本暂无 Delivery Asset" description="Release Readiness 的 delivery 检查会保持阻断，直到 ProductVersion 定义至少一种可交付资产。" />}
        </section>

        <section className="detail-card anchored-section" id="workflow">
          <div className="panel-header"><h2>Workflow</h2><Badge value={check(readiness, "production")} /></div>
          <DefinitionList items={[
            { label: "WorkflowVersion", value: <span className="mono">{shortId(version?.workflowVersionId)}</span> },
            { label: "Entity Policy", value: version?.entityPolicyRef || "—" },
            { label: "Indicator Set", value: version?.indicatorSetRef || "—" },
            { label: "执行数量（Release Trace）", value: trace?.executions.length ?? 0 },
          ]} />
        </section>

        <section className="detail-card anchored-section" id="contract">
          <div className="panel-header"><h2>Data Contract</h2><Badge value={check(readiness, "contract")} /></div>
          <DefinitionList items={[
            { label: "ProductVersion Contract", value: <span className="mono">{shortId(version?.contractVersionId)}</span> },
            { label: "Release Contract", value: <span className="mono">{shortId(latestRelease?.contractVersionId)}</span> },
          ]} />
        </section>

        <section className="governance-grid">
          <div className="detail-card anchored-section" id="rights">
            <div className="panel-header"><h2>Rights</h2><Badge value={check(readiness, "rights")} /></div>
            <p className="muted-copy">Rights Snapshot 固定本次 Release 可使用、加工、产品化与发布的权利边界。</p>
            <span className="mono">{latestRelease?.rightsSnapshotId ?? "尚未绑定"}</span>
          </div>
          <div className="detail-card anchored-section" id="quality">
            <div className="panel-header"><h2>Quality</h2><Badge value={check(readiness, "quality")} /></div>
            <p className="muted-copy">质量门独立于合规门，允许 PASS / PASS_WITH_WARNING 等产品级决策。</p>
            <span className="mono">{latestRelease?.qualityResultId ?? "尚未绑定"}</span>
          </div>
          <div className="detail-card anchored-section" id="compliance">
            <div className="panel-header"><h2>Compliance</h2><Badge value={check(readiness, "compliance")} /></div>
            <p className="muted-copy">合规结果固定敏感识别、最小必要、处理动作与复核结论。</p>
            <span className="mono">{latestRelease?.complianceResultId ?? "尚未绑定"}</span>
          </div>
        </section>

        <section className="governance-grid">
          <div className="detail-card anchored-section" id="cost">
            <div className="panel-header"><h2>Cost</h2><span className="eyebrow">Trace</span></div>
            <strong className="large-number">{trace?.costEvents.length ?? 0}</strong>
            <p className="muted-copy">CostEvent 与 Execution 关联；生产成本与会计可资本化成本保持概念隔离。</p>
          </div>
          <div className="detail-card anchored-section" id="evidence">
            <div className="panel-header"><h2>Evidence</h2><Badge value={check(readiness, "evidence")} /></div>
            <strong className="large-number">{trace?.evidence.length ?? 0}</strong>
            <p className="muted-copy">Release 发布时冻结 EvidenceSnapshot，历史证据不随当前状态漂移。</p>
          </div>
          <div className="detail-card">
            <div className="panel-header"><h2>Delivery</h2><Badge value={check(readiness, "delivery")} /></div>
            <strong className="large-number">{version?.assets.length ?? 0}</strong>
            <p className="muted-copy">Dataset、API、Report、Dashboard 等交付形态属于 ProductAsset。</p>
          </div>
        </section>

        <section className="detail-card anchored-section" id="releases">
          <div className="panel-header"><h2>Product Releases</h2><span className="eyebrow">{releases.page.total} Releases</span></div>
          {releases.items.length === 0 ? <EmptyState title="暂无 Release" description="创建 Release 后，系统将把产品版本与实际 DatasetVersion、权利、质量、合规和证据快照绑定。" /> : (
            <div className="table-card">
              <table className="data-table">
                <thead><tr><th>Release</th><th>状态</th><th>ProductVersion</th><th>Evidence Snapshot</th><th>发布时间</th></tr></thead>
                <tbody>{releases.items.map((release) => (
                  <tr key={release.id}>
                    <td><Link className="text-link" href={`/products/${id}/releases/${release.id}`}>{release.releaseNo}</Link></td>
                    <td><Badge value={release.status} /></td>
                    <td className="mono">{shortId(release.productVersionId)}</td>
                    <td className="mono">{shortId(release.evidenceSnapshotId)}</td>
                    <td>{formatDate(release.releasedAt ?? release.createdAt)}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>
      </>
    );
  } catch (error) {
    return <><BackLink href="/products">返回数据产品</BackLink><PageHeader title="数据产品详情" /><LoadError error={error} /></>;
  }
}
