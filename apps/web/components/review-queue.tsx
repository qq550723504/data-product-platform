"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useActionState, useState } from "react";
import { reviewCandidate } from "@/app/reviews/actions";
import { Badge, EmptyState, formatDate } from "@/components/ui";
import type { EntityReview } from "@/lib/platform";
import type { ReviewActionState } from "@/lib/review-command";
import styles from "./review-queue.module.css";

function ReviewForm({ review, onResult }: { review: EntityReview; onResult: (result: ReviewActionState) => void }) {
  const [reason, setReason] = useState("");
  const [selectedEntityId, setSelectedEntityId] = useState(review.candidateEntityId ?? "");
  const [state, action, pending] = useActionState(async (previous: ReviewActionState, form: FormData) => {
    const result = await reviewCandidate(previous, form);
    onResult(result);
    return result;
  }, { ok: false, message: "" });
  const locked = pending || state.ok || state.refreshRequired === true;
  return (
    <form action={action} className={styles.form} aria-label={`审核 ${review.sourceName || review.sourceKey}`}>
      <input type="hidden" name="candidateId" value={review.candidateId} />
      <input type="hidden" name="jobId" value={review.jobId} />
      {/* The token is what the reviewer actually saw, not a value read at submit. */}
      <input type="hidden" name="expectedDecisionId" value={review.currentMappingDecisionId ?? ""} />
      {!review.candidateEntityId && review.alternatives.length > 0 ? <>
        <label htmlFor={`entity-${review.candidateId}`}>选择匹配实体（必选）</label>
        <select id={`entity-${review.candidateId}`} name="selectedEntityId" required value={selectedEntityId}
          onChange={(event) => setSelectedEntityId(event.target.value)} disabled={locked}>
          <option value="">请选择冻结候选实体</option>
          {review.alternatives.map((alternative) => <option key={alternative.entityId} value={alternative.entityId}>
            {alternative.canonicalName || alternative.entityId}{alternative.canonicalKey ? ` · ${alternative.canonicalKey}` : ""}
          </option>)}
        </select>
      </> : null}
      <label htmlFor={`reason-${review.candidateId}`}>审核理由（必填）</label>
      <textarea id={`reason-${review.candidateId}`} name="reason" required maxLength={2000} rows={3}
        value={reason} onChange={(event) => setReason(event.target.value)} disabled={locked}
        placeholder="说明核对了哪些来源、为什么确认或拒绝此候选。" />
      <div className={styles.buttons}>
        <button type="submit" name="decision" value="confirm"
          disabled={locked || !reason.trim() || (!review.candidateEntityId && !selectedEntityId)}>确认匹配</button>
        <button type="submit" name="decision" value="reject" disabled={locked || !reason.trim()}>拒绝匹配</button>
        {pending ? <span role="status">正在提交，请勿重复操作…</span> : null}
      </div>
      {!review.candidateEntityId && review.alternatives.length === 0 ? <small>没有可选择的冻结候选实体，不能确认；拒绝也不会自动新建实体。</small> : null}
      {!review.candidateEntityId && review.alternatives.length > 0 ? <small>只能从本次匹配时冻结的候选集合中选择；提交时 Core 会再次验证实体仍为当前工作区同类型的 ACTIVE 实体。</small> : null}
      {state.message ? <p role={state.ok ? "status" : "alert"}>{state.message}</p> : null}
      {state.refreshRequired ? <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button> : null}
    </form>
  );
}

export function ReviewQueue({ items, actionsEnabled }: { items: EntityReview[]; actionsEnabled: boolean }) {
  const router = useRouter();
  const [notice, setNotice] = useState<ReviewActionState | null>(null);
  function onResult(result: ReviewActionState) {
    setNotice(result);
    if (result.ok || result.refreshRequired) router.refresh();
  }
  return (
    <>
      {notice ? <div className={`callout ${notice.ok ? "" : "callout-warn"}`} role={notice.ok ? "status" : "alert"}>
        <strong>{notice.message}</strong>
        {notice.job ? <p>任务 <span className="mono">{notice.job.id}</span> · <Badge value={notice.job.status} /></p> : null}
        {notice.job?.outputDatasetId && notice.job.outputDatasetVersionId ? <p>
          已生成版本 <span className="mono">{notice.job.outputDatasetVersionId}</span> · <Link href={`/datasets/${notice.job.outputDatasetId}`}>查看数据集版本</Link>
        </p> : null}
      </div> : null}
      <div className={styles.buttons}><button type="button" onClick={() => router.refresh()}>刷新队列</button></div>
      {items.length === 0 ? <EmptyState title="当前页没有待审核候选" description="未解析或冲突记录不因待审队列为空而视为已处理；请结合工作台查看任务状态。" /> : (
        <div className="table-card">
          {items.map((review) => (
            <article className="review-card" key={review.candidateId}>
              <div>
                <div className="badge-row"><Badge value={review.status} /><Badge value={review.decision} tone="info" /></div>
                <h3>{review.sourceName || review.sourceKey}</h3>
                <p>来源键 <span className="mono">{review.sourceKey}</span> · {formatDate(review.createdAt)}</p>
                <p>任务 <span className="mono">{review.jobId}</span></p>
                <details><summary>原始来源记录</summary><pre className="json-preview">{JSON.stringify(review.source, null, 2)}</pre></details>
                <details open><summary>规范化记录</summary><pre className="json-preview">{JSON.stringify(review.normalized, null, 2)}</pre></details>
                {actionsEnabled ? <ReviewForm review={review} onResult={onResult} /> : null}
              </div>
              <div>
                <div className="review-score"><span className="eyebrow">Confidence</span><strong>{Number.isFinite(review.confidence) && review.confidence >= 0 && review.confidence <= 1 ? `${(review.confidence * 100).toFixed(1)}%` : "未提供"}</strong></div>
                <dl className={styles.provenance}>
                  <dt>匹配方法</dt><dd>{review.matchMethod || "—"}</dd>
                  <dt>匹配规则</dt><dd>{review.matchRuleId || "—"}</dd>
                  <dt>候选实体</dt><dd>{review.candidateEntityId || "—"}</dd>
                  <dt>冻结备选</dt><dd>{review.alternatives.length ? review.alternatives.map((item) => item.canonicalName || item.entityId).join(" / ") : "—"}</dd>
                  <dt>策略引用</dt><dd>{review.policyRef || "—"}</dd>
                  <dt>策略版本</dt><dd>{review.policyVersion || "—"}</dd>
                  <dt>诊断引擎</dt><dd>{review.engineName || "未提供"}</dd>
                  <dt>引擎版本</dt><dd>{review.engineVersion || "—"}</dd>
                  <dt>模型版本</dt><dd>{review.modelVersion || "—"}</dd>
                </dl>
                <small>分数和引擎信息是诊断依据，不代替人工判断。</small>
              </div>
            </article>
          ))}
        </div>
      )}
    </>
  );
}
