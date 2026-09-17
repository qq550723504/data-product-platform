import { Badge, EmptyState, LoadError, PageHeader, SetupRequired, formatDate } from "@/components/ui";
import { configuredActorId, configuredWorkspaceId, platform } from "@/lib/platform";
import { reviewCandidateAction } from "./actions";

export default async function ReviewsPage({
  searchParams,
}: {
  searchParams: Promise<{ result?: string; error?: string }>;
}) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader eyebrow="Entity Resolution" title="实体审核" description="低置信度候选进入统一人工审核队列。" /><SetupRequired /></>;
  }

  const actorConfigured = Boolean(configuredActorId());
  const notice = await searchParams;

  try {
    const reviews = await platform.reviews("PENDING");
    return (
      <>
        <PageHeader
          eyebrow="Entity Resolution"
          title="实体审核"
          description="低置信度候选进入统一人工审核。每次确认或拒绝都必须说明判断理由，并由 Core 写入 Evidence 与 Audit。"
          action={<Badge value={reviews.page.total ? "PENDING" : "READY"} />}
        />

        {notice.result ? <div className="callout callout-good"><strong>审核完成</strong><p>{notice.result}</p></div> : null}
        {notice.error ? <div className="callout callout-bad"><strong>审核未提交</strong><p>{notice.error}</p></div> : null}
        {!actorConfigured ? (
          <div className="callout callout-warn" style={{ marginTop: 14 }}>
            <strong>POC Operator 尚未配置</strong>
            <p>设置服务端环境变量 <code>POC_ACTOR_ID</code> 后才能执行人工确认/拒绝；浏览器不会让用户伪造审计身份。</p>
          </div>
        ) : null}

        <div style={{ height: 18 }} />
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
                    来源键 <span className="mono">{review.sourceKey}</span> · Policy {review.policyRef}@{review.policyVersion} · {formatDate(review.createdAt)}
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

                  <form action={reviewCandidateAction} className="review-actions">
                    <input type="hidden" name="candidateId" value={review.candidateId} />
                    <input type="hidden" name="jobId" value={review.jobId} />
                    <label htmlFor={`reason-${review.candidateId}`}>人工判断理由</label>
                    <textarea
                      id={`reason-${review.candidateId}`}
                      name="reason"
                      required
                      minLength={3}
                      maxLength={1000}
                      placeholder="说明确认或拒绝该候选关系的依据…"
                      disabled={!actorConfigured}
                    />
                    <div className="action-row">
                      <button className="button button-primary" type="submit" name="intent" value="confirm" disabled={!actorConfigured}>确认匹配</button>
                      <button className="button button-danger" type="submit" name="intent" value="reject" disabled={!actorConfigured}>拒绝候选</button>
                    </div>
                  </form>
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
