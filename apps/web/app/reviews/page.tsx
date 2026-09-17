import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";

export default async function ReviewsPage() {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Entity Resolution" title="实体审核" description="低置信度候选进入统一人工审核队列。" /><SetupRequired /></>;
  }

  try {
    const reviews = await platform.reviews("PENDING");
    return (
      <>
        <PageHeader
          eyebrow="Entity Resolution"
          title="实体审核"
          description="这里呈现需要人判断的候选关系。匹配引擎、模型和分数是诊断证据，不是产品导航层。确认/拒绝操作将在下一阶段接入。"
          action={<Badge value={reviews.page.total ? "PENDING" : "READY"} />}
        />

        {reviews.items.length === 0 ? (
          <EmptyState title="没有待审核候选" description="确定性强键、规则匹配和概率候选均已完成决策，或当前没有实体解析任务。" />
        ) : (
          <div className="table-card">
            {reviews.items.map((review) => (
              <article className="review-card" key={review.candidateId}>
                <div>
                  <div className="badge-row" style={{ marginBottom: 10 }}>
                    <Badge value={review.status} />
                    <Badge value={review.decision} tone="info" />
                  </div>
                  <h3>{review.sourceName || review.sourceKey}</h3>
                  <p>
                    来源键 <span className="mono">{review.sourceKey}</span> · Policy {review.policyVersion} · {formatDate(review.createdAt)}
                  </p>
                  <div className="json-preview">{JSON.stringify(review.normalized, null, 2)}</div>
                </div>
                <div>
                  <div className="review-score">
                    <span className="eyebrow">Confidence</span>
                    <strong>{(review.confidence * 100).toFixed(1)}%</strong>
                  </div>
                  <div className="status-stack" style={{ marginTop: 14 }}>
                    <div className="status-row"><span>匹配方法</span><strong>{review.matchMethod || "—"}</strong></div>
                    <div className="status-row"><span>规则</span><span className="mono">{review.matchRuleId || "—"}</span></div>
                    <div className="status-row"><span>候选实体</span><span className="mono">{review.candidateEntityId ? `${review.candidateEntityId.slice(0, 8)}…` : "—"}</span></div>
                    <div className="status-row"><span>诊断引擎</span><span>{review.engineName || "RULES"}{review.engineVersion ? ` ${review.engineVersion}` : ""}</span></div>
                    <div className="status-row"><span>模型版本</span><span>{review.modelVersion || "—"}</span></div>
                  </div>
                </div>
              </article>
            ))}
          </div>
        )}
      </>
    );
  } catch (error) {
    return <><PageHeader eyebrow="Entity Resolution" title="实体审核" /><LoadError error={error} /></>;
  }
}
