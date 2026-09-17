import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function DatasetsPage() {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Datasets" title="数据集" description="逻辑数据集与不可变版本。" /><SetupRequired /></>;
  }

  try {
    const datasets = await platform.datasets();
    return (
      <>
        <PageHeader
          eyebrow="Datasets"
          title="数据集"
          description="明确区分 RAW、STANDARDIZED、CURATED 与 PRODUCT；每次生产都生成新的不可变 DatasetVersion。"
        />
        {datasets.items.length === 0 ? (
          <EmptyState title="暂无数据集" description="原始数据进入生产链后，先形成 RAW Dataset，再通过执行产生新的标准化或加工版本。" />
        ) : (
          <div className="table-card">
            <table className="data-table">
              <thead><tr><th>数据集</th><th>类型</th><th>生命周期</th><th>当前版本</th><th>更新时间</th></tr></thead>
              <tbody>
                {datasets.items.map((dataset) => (
                  <tr key={dataset.id}>
                    <td className="primary-cell">
                      <Link className="text-link" href={`/datasets/${dataset.id}`}><strong>{dataset.name}</strong></Link>
                      <span>{dataset.code}</span>
                    </td>
                    <td><Badge value={dataset.datasetType} tone="info" /></td>
                    <td><Badge value={dataset.lifecycleStatus} /></td>
                    <td className="mono">{dataset.currentVersionId ? `${dataset.currentVersionId.slice(0, 8)}…` : "—"}</td>
                    <td>{formatDate(dataset.updatedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </>
    );
  } catch (error) {
    return <><PageHeader eyebrow="Datasets" title="数据集" /><LoadError error={error} /></>;
  }
}
