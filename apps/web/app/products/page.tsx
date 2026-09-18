import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, Pagination, SetupRequired, formatDate } from "@/components/ui";
import { LIST_PAGE_SIZE, parsePageOffset } from "@/lib/pagination";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function ProductsPage({ searchParams }: { searchParams: Promise<{ offset?: string }> }) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Data Products" title="数据产品" description="稳定业务身份、版本和 Release。" /><SetupRequired /></>;
  }

  const offset = parsePageOffset((await searchParams).offset);
  try {
    const products = await platform.products(LIST_PAGE_SIZE, offset);
    return (
      <>
        <PageHeader
          eyebrow="Data Products"
          title="数据产品"
          description="DataProduct 是稳定业务身份；ProductVersion 冻结定义，ProductRelease 冻结一次实际发布快照。"
        />
        {products.page.total === 0 ? (
          <EmptyState title="暂无数据产品" description="完成生产、质量、合规、权利与 Data Contract 准备后，可创建首个产品版本和 Release。" />
        ) : (
          <>
            {products.items.length === 0 ? (
              <div className="callout callout-warn"><strong>该页没有记录</strong><p>当前偏移超出数据范围，请使用下方分页返回有效页。</p></div>
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
            <Pagination basePath="/products" offset={products.page.offset} itemCount={products.items.length} total={products.page.total} pageSize={LIST_PAGE_SIZE} label="数据产品分页" />
          </>
        )}
      </>
    );
  } catch (error) {
    return <><PageHeader eyebrow="Data Products" title="数据产品" /><LoadError error={error} /></>;
  }
}
