import { BackLink, Badge, DefinitionList, LoadError, PageHeader, SetupRequired, formatDate, shortId } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function ResourceDetailPage({ params }: { params: Promise<{ id: string }> }) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader title="数据资源详情" /><SetupRequired /></>;
  }
  const { id } = await params;
  try {
    const resource = await platform.resource(id);
    return (
      <>
        <BackLink href="/resources">返回数据资源</BackLink>
        <PageHeader
          eyebrow={resource.code}
          title={resource.name}
          description={resource.description || "业务语义资源详情"}
          action={<Badge value={resource.lifecycleStatus} />}
        />
        <section className="section-grid">
          <div className="detail-card">
            <h2>资源属性</h2>
            <DefinitionList items={[
              { label: "资源类型", value: resource.resourceType },
              { label: "业务域", value: resource.domainCode || "—" },
              { label: "敏感级别", value: resource.sensitivityLevel || "—" },
              { label: "Revision", value: resource.revision },
              { label: "Owner", value: shortId(resource.ownerId) },
              { label: "Project", value: shortId(resource.projectId) },
              { label: "创建时间", value: formatDate(resource.createdAt) },
              { label: "更新时间", value: formatDate(resource.updatedAt) },
            ]} />
          </div>
          <aside className="detail-card">
            <h2>治理状态</h2>
            <div className="status-stack" style={{ marginTop: 12 }}>
              <div className="status-row"><span>Rights</span><Badge value={resource.rightsStatus} /></div>
              <div className="status-row"><span>Quality</span><Badge value={resource.qualityStatus} /></div>
              <div className="status-row"><span>Lifecycle</span><Badge value={resource.lifecycleStatus} /></div>
            </div>
          </aside>
        </section>
      </>
    );
  } catch (error) {
    return <><BackLink href="/resources">返回数据资源</BackLink><PageHeader title="数据资源详情" /><LoadError error={error} /></>;
  }
}
