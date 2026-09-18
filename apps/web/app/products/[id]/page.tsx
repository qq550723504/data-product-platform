import Link from "next/link";
import { BackLink, Badge, DefinitionList, EmptyState, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { ProductReleasePanel, type ReleasePanelItem } from "@/components/product-release-panel";
import { collectAllPages } from "@/lib/pagination";
import { configuredWorkspaceId, platform, type DataProduct } from "@/lib/platform";
import { findAcrossPages } from "@/lib/scoped-lookup";

async function scopedProduct(id: string): Promise<DataProduct> {
  // A valid product URL must resolve even when the workspace has more than the
  // first page of products; a first-page-only lookup reports "不存在" wrongly.
  const product = await findAcrossPages(
    (limit, offset) => platform.products(limit, offset),
    (item) => item.id.toLowerCase() === id.toLowerCase(),
  );
  if (!product) throw new Error("当前 Workspace 中不存在此数据产品。");
  return product;
}

export default async function ProductDetailPage({ params }: { params: Promise<{ id: string }> }) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader title="数据产品详情" /><SetupRequired /></>;
  }
  const { id } = await params;

  try {
    const product = await scopedProduct(id);
    // Release history is immutable and unbounded; drain every page so older
    // releases are not silently dropped from the panel.
    const releasedItems = await collectAllPages((limit, offset) => platform.releases(product.id, limit, offset));
    const version = product.currentVersionId ? await platform.productVersion(product.currentVersionId) : null;
    if (version && version.productId.toLowerCase() !== product.id.toLowerCase()) {
      throw new Error("当前 ProductVersion 与数据产品不匹配。");
    }

    const releases: ReleasePanelItem[] = await Promise.all(releasedItems.map(async (summary) => {
      const [release, readiness] = await Promise.all([
        platform.release(summary.id),
        platform.releaseReadiness(summary.id),
      ]);
      if (release.productId.toLowerCase() !== product.id.toLowerCase() || readiness.releaseId.toLowerCase() !== release.id.toLowerCase()) {
        throw new Error("Release/Readiness 返回范围与当前数据产品不匹配。");
      }
      return { release, readiness };
    }));

    const actionsEnabled = process.env.POC_ENABLE_RELEASE_ACTIONS === "true" && Boolean(process.env.POC_RELEASE_ACTOR_ID?.trim());

    return (
      <>
        <BackLink href="/products">返回数据产品</BackLink>
        <PageHeader
          eyebrow={product.code}
          title={product.name}
          description={product.description || "DataProduct 是稳定业务身份；ProductVersion 冻结定义，ProductRelease 冻结实际发布快照。"}
          action={<div className="badge-row"><Badge value={product.lifecycleStatus} /><Badge value={product.healthStatus} /></div>}
        />

        <nav className="badge-row" aria-label="产品详情章节" style={{ marginBottom: 18 }}>
          <a className="text-link" href="#definition">产品定义</a>
          <a className="text-link" href="#assets">交付资产</a>
          <a className="text-link" href="#governance">治理绑定</a>
          <a className="text-link" href="#releases">Releases</a>
        </nav>

        <section className="detail-card" id="definition" style={{ marginBottom: 18 }}>
          <div className="panel-header"><h2>产品定义</h2><span className="eyebrow">Stable Identity</span></div>
          <DefinitionList items={[
            { label: "Product ID", value: <span className="mono">{product.id}</span> },
            { label: "Domain", value: product.domainCode || "—" },
            { label: "Project", value: <span className="mono">{shortId(product.projectId)}</span> },
            { label: "Use Case", value: <span className="mono">{shortId(product.useCaseId)}</span> },
            { label: "Current Version", value: version ? `${version.version} · ${shortId(version.id)}` : "—" },
            { label: "Latest Release", value: product.latestReleaseNo ? `${product.latestReleaseNo} · ${product.latestReleaseStatus}` : "—" },
            { label: "更新时间", value: formatDate(product.updatedAt) },
          ]} />
          {version ? <details style={{ marginTop: 16 }}><summary>Version definition snapshot</summary><pre className="json-preview">{JSON.stringify(version.definition, null, 2)}</pre></details> : null}
        </section>

        <section className="detail-card" id="assets" style={{ marginBottom: 18 }}>
          <div className="panel-header"><h2>交付资产</h2><span className="eyebrow">ProductVersion Assets</span></div>
          {!version || version.assets.length === 0 ? (
            <EmptyState title="当前版本没有交付资产" description="Release 的 delivery Gate 会保持阻塞，直到 ProductVersion 定义了可交付资产。" />
          ) : (
            <div className="table-card">
              <table className="data-table">
                <thead><tr><th>资产</th><th>类型</th><th>Dataset</th><th>外部引用</th></tr></thead>
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
          )}
        </section>

        <section className="detail-card" id="governance" style={{ marginBottom: 18 }}>
          <div className="panel-header"><h2>治理与生产绑定</h2><span className="eyebrow">Frozen by ProductVersion</span></div>
          {version ? <DefinitionList items={[
            { label: "Workflow Version", value: <span className="mono">{version.workflowVersionId ?? "—"}</span> },
            { label: "Contract Version", value: <span className="mono">{version.contractVersionId ?? "—"}</span> },
            { label: "Entity Policy", value: version.entityPolicyRef || "—" },
            { label: "Indicator Set", value: version.indicatorSetRef || "—" },
          ]} /> : <EmptyState title="暂无 ProductVersion" description="先由 Core 创建不可变 ProductVersion，再准备 Release。" />}
          <p style={{ marginTop: 12 }}>Rights、Quality、Compliance 与 Evidence 以每个 Release 的冻结引用和独立 Readiness Gate 为准，不由浏览器推导。</p>
        </section>

        <section id="releases">
          <div className="panel-header">
            <div><h2>ProductRelease 与 Readiness</h2><p>逐项显示 production、dataset、rights、quality、compliance、contract、evidence、delivery 八个 Gate。</p></div>
            <span className="eyebrow">{releases.length} Releases</span>
          </div>
          <ProductReleasePanel productId={product.id} items={releases} actionsEnabled={actionsEnabled} />
        </section>
      </>
    );
  } catch (error) {
    return <><BackLink href="/products">返回数据产品</BackLink><PageHeader title="数据产品详情" /><LoadError error={error} /></>;
  }
}
