import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function ProductsPage() {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Data Products" title="数据产品" description="稳定业务身份、版本和 Release。" /><SetupRequired /></>;
  }

  try {
    const products = await platform.products();
    return (
      <>
        <PageHeader
          eyebrow="Data Products"
          title="数据产品"
          description="DataProduct 是稳定业务身份；ProductVersion 冻结定义，ProductRelease 冻结一次实际发布快照。"
        />
        {products.items.length === 0 ? (
          <EmptyState title="暂无数据产品" description="完成生产、质量、合规、权利与 Data Contract 准备后，可创建首个产品版本和 Release。" />
        ) : (
          <div className="table-card">
            <table className="data-table">
              <thead><tr><th>产品</th><th>生命周期</th><th>健康度</th><th>当前版本</th><th>最新 Release</th><th>更新时间</th></tr></thead>
              <tbody>
                {products.items.map((product) => (
                  <tr key={product.id}>
                    <td className="primary-cell">
                      <Link className="text-link" href={`/products/${product.id}`}><strong>{product.name}</strong></Link>
                      <span>{product.code}{product.domainCode ? ` · ${product.domainCode}` : ""}</span>
                    </td>
                    <td><Badge value={product.lifecycleStatus} /></td>
                    <td><Badge value={product.healthStatus} /></td>
                    <td>{product.currentVersion || "—"}</td>
                    <td><div className="badge-row"><span>{product.latestReleaseNo || "—"}</span>{product.latestReleaseStatus ? <Badge value={product.latestReleaseStatus} /> : null}</div></td>
                    <td>{formatDate(product.updatedAt)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </>
    );
  } catch (error) {
    return <><PageHeader eyebrow="Data Products" title="数据产品" /><LoadError error={error} /></>;
  }
}
