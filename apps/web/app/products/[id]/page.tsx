import {businessText,displayText} from '../../../lib/business-language.mjs';
import Link from "next/link";
import { BackLink, Badge, DefinitionList, EmptyState, LoadError, PageHeader, Pagination, SetupRequired, formatDate, shortId } from "@/components/ui";
import { ProductReleaseCreateForm, type ReleaseDatasetCandidate } from "@/components/product-release-create-form";
import { ProductReleasePanel, type ReleasePanelItem } from "@/components/product-release-panel";
import { collectAllPages, LIST_PAGE_SIZE, parsePageOffset } from "@/lib/pagination";
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

export default async function ProductDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ offset?: string }>;
}) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader title="数据产品详情" /><SetupRequired /></>;
  }
  const { id } = await params;
  const offset = parsePageOffset((await searchParams).offset);

  try {
    const product = await scopedProduct(id);
    // Release history is immutable and can grow without bound. Render one
    // server-side page at a time so detail/readiness fan-out stays bounded.
    let releasePage = await platform.releases(product.id, LIST_PAGE_SIZE, offset);
    if (releasePage.page.total > 0 && releasePage.items.length === 0 && offset > 0) {
      const lastOffset = Math.floor((releasePage.page.total - 1) / LIST_PAGE_SIZE) * LIST_PAGE_SIZE;
      releasePage = await platform.releases(product.id, LIST_PAGE_SIZE, lastOffset);
    }
    const releasedItems = releasePage.items;
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
    const releaseCandidates: ReleaseDatasetCandidate[] = [];
    if (actionsEnabled && version) {
      const datasetAssets = version.assets.filter((asset) => asset.assetType === "DATASET" && asset.datasetId);
      const candidateCache = new Map<string, Awaited<ReturnType<typeof platform.datasetVersions>>["items"]>();
      const datasetCache = new Map<string, Awaited<ReturnType<typeof platform.dataset>>>();
      for (const asset of datasetAssets) {
        const datasetId = asset.datasetId as string;
        let versions = candidateCache.get(datasetId);
        if (!versions) {
          versions = await collectAllPages((limit, candidateOffset) => platform.datasetVersions(datasetId, limit, candidateOffset));
          candidateCache.set(datasetId, versions);
        }
        let dataset = datasetCache.get(datasetId);
        if (!dataset) {
          dataset = await platform.dataset(datasetId);
          datasetCache.set(datasetId, dataset);
        }
        releaseCandidates.push({
          assetId: asset.id,
          assetName: asset.name,
          datasetId,
          datasetName: dataset.name,
          datasetCode: dataset.code,
          versions: versions
            .filter((candidate) => candidate.status === "READY" || candidate.status === "SUPERSEDED")
            .map((candidate) => ({
              id: candidate.id,
              versionNo: candidate.versionNo,
              status: candidate.status,
            })),
        });
      }
    }

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
          <a className="text-link" href="#releases">发布记录s</a>
        </nav>

        <section className="detail-card" id="definition" style={{ marginBottom: 18 }}>
          <div className="panel-header"><h2>产品定义</h2><span className="eyebrow">Stable Identity</span></div>
          <DefinitionList items={[
            { label: "Product ID", value: <span className="mono">{businessText(product.id)}</span> },
            { label: "Domain", value: product.domainCode || "—" },
            { label: "Project", value: <span className="mono">{displayText(shortId(product.projectId))}</span> },
            { label: "Use Case", value: <span className="mono">{displayText(shortId(product.useCaseId))}</span> },
            { label: "Current Version", value: version ? `${version.version} · ${shortId(version.id)}` : "—" },
            { label: "Latest Release", value: product.latestReleaseNo ? `${product.latestReleaseNo} · ${product.latestReleaseStatus}` : "—" },
            { label: "更新时间", value: formatDate(product.updatedAt) },
          ]} />
          {displayText(version ? <details style={{ marginTop: 16 }}><summary>Version definition snapshot</summary><pre className="json-preview">{JSON.stringify(version.definition, null, 2)}</pre></details> : null)}
        </section>

        <section className="detail-card" id="assets" style={{ marginBottom: 18 }}>
          <div className="panel-header"><h2>交付资产</h2><span className="eyebrow">产品版本 Assets</span></div>
          {!version || version.assets.length === 0 ? (
            <EmptyState title="当前版本没有交付资产" description="发布记录 的 delivery 检查项 会保持阻塞，直到 产品版本 定义了可交付资产。" />
          ) : (
            <div className="table-card">
              <table className="data-table">
                <thead><tr><th>资产</th><th>类型</th><th>数据集</th><th>外部引用</th></tr></thead>
                <tbody>{version.assets.map((asset) => (
                  <tr key={asset.id}>
                    <td className="primary-cell"><strong>{businessText(asset.name)}</strong><span className="mono">{displayText(shortId(asset.id))}</span></td>
                    <td><Badge value={asset.assetType} tone="info" /></td>
                    <td>{asset.datasetId ? <Link className="text-link mono" href={`/datasets/${asset.datasetId}`}>{displayText(shortId(asset.datasetId))}</Link> : "—"}</td>
                    <td className="mono">{displayText(asset.externalRef || "—")}</td>
                  </tr>
                ))}</tbody>
              </table>
            </div>
          )}
        </section>

        <section className="detail-card" id="governance" style={{ marginBottom: 18 }}>
          <div className="panel-header"><h2>治理与生产绑定</h2><span className="eyebrow">Frozen by 产品版本</span></div>
          {version ? <DefinitionList items={[
            { label: "Workflow Version", value: <span className="mono">{displayText(version.workflowVersionId ?? "—")}</span> },
            { label: "Contract Version", value: <span className="mono">{displayText(version.contractVersionId ?? "—")}</span> },
            { label: "Entity Policy", value: version.entityPolicyRef || "—" },
            { label: "Indicator Set", value: version.indicatorSetRef || "—" },
          ]} /> : <EmptyState title="暂无 产品版本" description="请先创建固定的产品版本，再准备发布。" />}
          <p style={{ marginTop: 12 }}>Rights、Quality、Compliance 与 证据记录 以每个 发布记录 的冻结引用和独立 发布检查 检查项 为准，不由浏览器推导。</p>
        </section>

        <section id="releases">
          {displayText(version && actionsEnabled ? (
            <ProductReleaseCreateForm
              productId={product.id}
              productVersionId={version.id}
              candidates={releaseCandidates}
              enabled
            />
          ) : version ? (
            <div className="callout" style={{ marginBottom: 18 }}>
              <strong>发布记录 创建写入未启用</strong>
              <p>当前环境仅供查看，未开放发布准备，也不会加载候选成果版本。</p>
            </div>
          ) : null)}
          <div className="panel-header">
            <div><h2>发布记录 与 发布检查</h2><p>逐项显示 production、dataset、rights、quality、compliance、contract、evidence、delivery 八个 检查项。</p></div>
            <span className="eyebrow">{displayText(releasePage.page.total)} 发布记录s</span>
          </div>
          {displayText(releasePage.page.total > 0 && releases.length === 0 ? (
            <div className="callout callout-warn">
              <strong>该页没有 发布记录</strong>
              <p>当前起始位置超出历史范围，请使用下方分页返回有效页。</p>
            </div>
          ) : (
            <ProductReleasePanel productId={product.id} items={releases} actionsEnabled={actionsEnabled} />
          ))}
          {displayText(releasePage.page.total > 0 ? (
            <Pagination
              basePath={`/products/${product.id}`}
              offset={releasePage.page.offset}
              itemCount={releasePage.items.length}
              total={releasePage.page.total}
              pageSize={LIST_PAGE_SIZE}
              label="发布记录 历史分页"
            />
          ) : null)}
        </section>
      </>
    );
  } catch (error) {
    return <><BackLink href="/products">返回数据产品</BackLink><PageHeader title="数据产品详情" /><LoadError error={error} /></>;
  }
}
