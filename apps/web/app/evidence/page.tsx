import Link from "next/link";
import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function EvidencePage() {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Evidence" title="证据中心" /><SetupRequired /></>;
  }

  try {
    const products = await platform.products(100, 0);
    const rows = (await Promise.all(products.items.map(async (product) => {
      const releases = await platform.releases(product.id, 20, 0);
      return releases.items.map((release) => ({ product, release }));
    }))).flat().sort((a, b) => Date.parse(b.release.createdAt) - Date.parse(a.release.createdAt));

    return (
      <>
        <PageHeader
          eyebrow="Evidence"
          title="证据中心"
          description="证据不是附件文件夹，而是对权利、来源、加工、质量、合规、成本与发布事实的可追溯证明。"
        />
        <section className="detail-card" style={{ marginBottom: 18 }}>
          <h2>POC 证据链</h2>
          <div className="flow" style={{ marginTop: 16 }}>
            {["ProductRelease", "DatasetVersion", "Execution", "Input DatasetVersion", "Data Resource", "Evidence / Cost / Audit"].map((step, index, steps) => (
              <span key={step} className="flow">
                <span className="flow-step">{step}</span>
                {index < steps.length - 1 ? <span className="flow-arrow">→</span> : null}
              </span>
            ))}
          </div>
          <p style={{ marginTop: 14 }}>Release 必须先从当前 Workspace 的 Data Product 发现；证据页不会要求操作者粘贴任意 UUID。</p>
        </section>

        <div className="panel-header"><h2>可追溯 ProductRelease</h2><span className="eyebrow">{rows.length} Releases</span></div>
        {rows.length === 0 ? (
          <EmptyState title="暂无可追溯 Release" description="当前 Workspace 尚未产生 ProductRelease。" />
        ) : (
          <div className="table-card">
            <table className="data-table">
              <thead><tr><th>数据产品</th><th>Release</th><th>状态</th><th>Evidence Snapshot</th><th>时间</th><th>证据链</th></tr></thead>
              <tbody>{rows.map(({ product, release }) => (
                <tr key={release.id}>
                  <td className="primary-cell"><Link className="text-link" href={`/products/${product.id}`}><strong>{product.name}</strong></Link><span>{product.code}</span></td>
                  <td className="primary-cell"><strong>{release.releaseNo}</strong><span className="mono">{shortId(release.id)}</span></td>
                  <td><Badge value={release.status} /></td>
                  <td className="mono">{shortId(release.evidenceSnapshotId)}</td>
                  <td>{formatDate(release.releasedAt ?? release.createdAt)}</td>
                  <td><Link className="text-link" href={`/products/${product.id}/releases/${release.id}`}>查看 Trace →</Link></td>
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
