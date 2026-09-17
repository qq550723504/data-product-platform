import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function EvidencePage() {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Evidence" title="证据中心" /><SetupRequired /></>;
  }

  try {
    const products = await platform.products();
    const releasePages = await Promise.all(
      products.items.map(async (product) => ({ product, releases: await platform.releases(product.id, 20, 0) })),
    );
    const releases = releasePages.flatMap(({ product, releases: page }) =>
      page.items.map((release) => ({ product, release })),
    );

    return (
      <>
        <PageHeader
          eyebrow="Evidence"
          title="证据中心"
          description="从 ProductRelease 进入证据图：DatasetVersion → Execution → Entity Resolution → Evidence / Cost / Audit。"
        />
        <section className="detail-card" style={{ marginBottom: 18 }}>
          <h2>Release 证据链</h2>
          <div className="flow" style={{ marginTop: 16 }}>
            {[
              "ProductRelease",
              "DatasetVersion",
              "Execution",
              "Input DatasetVersion",
              "Data Resource",
              "Evidence",
            ].map((step, index, steps) => (
              <span key={step} className="flow">
                <span className="flow-step">{step}</span>
                {index < steps.length - 1 ? <span className="flow-arrow">→</span> : null}
              </span>
            ))}
          </div>
        </section>

        {releases.length === 0 ? (
          <EmptyState title="暂无可追溯 Release" description="创建 ProductRelease 后，可从这里进入完整生产与证据链。" />
        ) : (
          <div className="table-card">
            <table className="data-table">
              <thead><tr><th>数据产品</th><th>Release</th><th>状态</th><th>Evidence Snapshot</th><th>发布时间</th><th>追溯</th></tr></thead>
              <tbody>{releases.map(({ product, release }) => (
                <tr key={release.id}>
                  <td className="primary-cell"><strong>{product.name}</strong><span>{product.code}</span></td>
                  <td>{release.releaseNo}</td>
                  <td><Badge value={release.status} /></td>
                  <td className="mono">{shortId(release.evidenceSnapshotId)}</td>
                  <td>{formatDate(release.releasedAt ?? release.createdAt)}</td>
                  <td><Link className="text-link" href={`/products/${product.id}/releases/${release.id}`}>查看证据链 →</Link></td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
      </>
    );
  } catch (error) {
    return <><PageHeader eyebrow="Evidence" title="证据中心" /><LoadError error={error} /></>;
  }
}
