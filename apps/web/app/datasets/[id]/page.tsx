import Link from "next/link";
import { BackLink, Badge, DefinitionList, EmptyState, LoadError, PageHeader, SetupRequired, formatBytes, formatDate, shortId } from "@/components/ui";
import { collectAllPages } from "@/lib/pagination";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function DatasetDetailPage({ params }: { params: Promise<{ id: string }> }) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader title="数据集详情" /><SetupRequired /></>;
  }
  const { id } = await params;

  try {
    const [dataset, versions] = await Promise.all([
      platform.dataset(id),
      // Immutable version history must be drained across pages; a fixed first
      // page would hide older versions once a dataset exceeds 100 versions.
      collectAllPages((limit, offset) => platform.datasetVersions(id, limit, offset)),
    ]);
    return (
      <>
        <BackLink href="/datasets">返回数据集</BackLink>
        <PageHeader
          eyebrow={dataset.code}
          title={dataset.name}
          description={dataset.description || "Dataset 是逻辑生产对象；DatasetVersion 是不可变事实。"}
          action={<div className="badge-row"><Badge value={dataset.datasetType} tone="info" /><Badge value={dataset.lifecycleStatus} /></div>}
        />

        <section className="detail-card" style={{ marginBottom: 18 }}>
          <h2>Dataset 定义</h2>
          <DefinitionList items={[
            { label: "Dataset 类型", value: dataset.datasetType },
            { label: "Source Resource", value: shortId(dataset.sourceResourceId) },
            { label: "Current Version", value: shortId(dataset.currentVersionId) },
            { label: "Project", value: shortId(dataset.projectId) },
            { label: "创建时间", value: formatDate(dataset.createdAt) },
            { label: "更新时间", value: formatDate(dataset.updatedAt) },
          ]} />
        </section>

        <div className="panel-header"><h2>不可变版本历史</h2><span className="eyebrow">{versions.length} Versions</span></div>
        {versions.length === 0 ? (
          <EmptyState title="暂无版本" description="版本一旦生成即冻结；修复数据时应创建新版本，而不是覆盖历史版本。" />
        ) : (
          <div className="table-card">
            <table className="data-table">
              <thead><tr><th>版本</th><th>状态</th><th>行数</th><th>大小</th><th>生产执行</th><th>时间</th></tr></thead>
              <tbody>
                {versions.map((version) => (
                  <tr key={version.id}>
                    <td className="primary-cell"><Link className="text-link" href={`/datasets/${dataset.id}/versions/${version.id}`}><strong>v{version.versionNo}</strong></Link><span className="mono">{shortId(version.id)}</span>{dataset.datasetType === "RAW" && dataset.code.startsWith("CSV-") && version.status === "READY" ? <Link href={`/ingest?version=${version.id}`}>继续主体解析</Link> : null}</td>
                    <td><Badge value={version.status} /></td>
                    <td>{version.rowCount ?? "—"}</td>
                    <td>{formatBytes(version.byteSize)}</td>
                    <td>
                      {version.generatedByExecutionId ? (
                        <Link className="text-link mono" href={`/production/${version.generatedByExecutionId}`}>{shortId(version.generatedByExecutionId)}</Link>
                      ) : "—"}
                    </td>
                    <td>{formatDate(version.readyAt ?? version.createdAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </>
    );
  } catch (error) {
    return <><BackLink href="/datasets">返回数据集</BackLink><PageHeader title="数据集详情" /><LoadError error={error} /></>;
  }
}
