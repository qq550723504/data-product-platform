import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function ResourcesPage() {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Data Resources" title="数据资源" description="业务语义资源，不等同于物理表。" /><SetupRequired /></>;
  }

  try {
    const resources = await platform.resources();
    return (
      <>
        <PageHeader
          eyebrow="Data Resources"
          title="数据资源"
          description="管理可被数据产品生产链引用的业务资源，并观察权利、质量与敏感级别状态。"
        />
        {resources.items.length === 0 ? (
          <EmptyState title="暂无数据资源" description="资源接入后会在这里作为业务语义对象出现，技术绑定仍保持在 Core 边界内。" />
        ) : (
          <div className="table-card">
            <table className="data-table">
              <thead><tr><th>资源</th><th>类型</th><th>权利</th><th>质量</th><th>生命周期</th><th>更新时间</th></tr></thead>
              <tbody>
                {resources.items.map((resource) => (
                  <tr key={resource.id}>
                    <td className="primary-cell">
                      <Link href={`/resources/${resource.id}`} className="text-link"><strong>{resource.name}</strong></Link>
                      <span>{resource.code}{resource.domainCode ? ` · ${resource.domainCode}` : ""}</span>
                    </td>
                    <td>{resource.resourceType}</td>
                    <td><Badge value={resource.rightsStatus} /></td>
                    <td><Badge value={resource.qualityStatus} /></td>
                    <td><Badge value={resource.lifecycleStatus} /></td>
                    <td>{formatDate(resource.updatedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </>
    );
  } catch (error) {
    return <><PageHeader eyebrow="Data Resources" title="数据资源" /><LoadError error={error} /></>;
  }
}
