"use client";

import { useActionState, useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { runQualityCheck } from "@/app/datasets/[id]/versions/[versionId]/actions";
import type { QualityAttempt } from "@/lib/platform";
import type { QualityActionState } from "@/lib/quality-command";

function newAttemptId(): string {
  return globalThis.crypto?.randomUUID?.() ?? "";
}

export function QualityCheckForm({
  datasetId,
  versionId,
  versionStatus,
  enabled,
  initialAttemptId,
  attempt,
}: {
  datasetId: string;
  versionId: string;
  versionStatus: string;
  enabled: boolean;
  initialAttemptId?: string;
  attempt?: QualityAttempt | null;
}) {
  const router = useRouter();
  const [attemptId, setAttemptId] = useState(initialAttemptId ?? "");
  const [ruleSetRef, setRuleSetRef] = useState(attempt?.ruleSetRef ?? "");
  const [engineName, setEngineName] = useState(attempt?.engineName ?? "");
  const [state, action, pending] = useActionState<QualityActionState, FormData>(
    runQualityCheck,
    { ok: false, message: "" },
  );

  useEffect(() => {
    if (!attemptId) setAttemptId(newAttemptId());
  }, [attemptId]);

  useEffect(() => {
    if (!state.attemptId) return;
    const url = new URL(window.location.href);
    url.searchParams.set("view", "quality");
    url.searchParams.set("qualityAttemptId", state.attemptId);
    window.history.replaceState(window.history.state, "", url.pathname + "?" + url.searchParams.toString());
    if (state.attemptId !== attemptId) setAttemptId(state.attemptId);
  }, [attemptId, state.attemptId]);

  const usable = versionStatus === "READY" || versionStatus === "SUPERSEDED";
  const terminal = attempt?.outcome === "SUCCEEDED" || attempt?.outcome === "FAILED";
  const active = attempt?.outcome === "IN_PROGRESS" && !attempt.leaseExpired;
  const canSubmit = enabled && usable && Boolean(attemptId) && !pending && !terminal && !active;

  const statusText = useMemo(() => {
    if (!attempt) return null;
    if (attempt.outcome === "SUCCEEDED") return "SUCCEEDED · Assessment " + (attempt.assessmentId?.slice(0, 8) ?? "—");
    if (attempt.outcome === "FAILED") return "FAILED · 该 attempt 已终止；重新评测会创建新的 attempt";
    if (attempt.outcome === "IN_PROGRESS" && attempt.leaseExpired) return "IN_PROGRESS · lease 已过期，可用同一 attemptId 让 Core reconcile";
    if (attempt.outcome === "IN_PROGRESS") return "IN_PROGRESS · 正在运行，不会重复提交";
    return attempt.outcome;
  }, [attempt]);

  const rememberAttempt = () => {
    if (!attemptId) return;
    const url = new URL(window.location.href);
    url.searchParams.set("view", "quality");
    url.searchParams.set("qualityAttemptId", attemptId);
    window.history.replaceState(window.history.state, "", url.pathname + "?" + url.searchParams.toString());
  };

  const startFreshAttempt = () => {
    const nextAttemptId = newAttemptId();
    const url = new URL(window.location.href);
    url.searchParams.set("view", "quality");
    url.searchParams.delete("qualityAttemptId");
    setAttemptId(nextAttemptId);
    window.history.replaceState(window.history.state, "", url.pathname + "?" + url.searchParams.toString());
    router.refresh();
  };

  return (
    <form
      action={action}
      onSubmit={rememberAttempt}
      className="detail-card"
      style={{ marginBottom: 18 }}
      aria-label="运行 Quality Check"
    >
      <input type="hidden" name="datasetId" value={datasetId} />
      <input type="hidden" name="versionId" value={versionId} />
      <input type="hidden" name="assessmentAttemptId" value={attemptId} />

      <div className="panel-header">
        <div>
          <span className="eyebrow">Recoverable assessment attempt</span>
          <h3>运行 Quality Check</h3>
          <p>attemptId 会在提交前写入当前 URL；刷新后继续查询同一个 Core attempt，不会盲目重跑。</p>
        </div>
      </div>

      <div className="definition-list">
        <div><dt>Attempt</dt><dd className="mono">{attemptId || "正在生成…"}</dd></div>
        <div>
          <dt>Rule Set</dt>
          <dd>
            <input
              aria-label="Quality Rule Set"
              name="ruleSetRef"
              value={ruleSetRef}
              onChange={(event) => setRuleSetRef(event.target.value)}
              maxLength={512}
              placeholder="留空使用 Core 默认规则集"
            />
          </dd>
        </div>
        <div>
          <dt>Engine</dt>
          <dd>
            <input
              aria-label="Quality Engine"
              name="engineName"
              value={engineName}
              onChange={(event) => setEngineName(event.target.value)}
              maxLength={128}
              placeholder="留空使用 Core 默认引擎"
            />
          </dd>
        </div>
      </div>

      {statusText ? <p role="status" data-testid="quality-attempt-status"><strong>{statusText}</strong></p> : null}
      <button type="submit" disabled={!canSubmit}>
        {pending ? "正在执行，请勿重复操作…" : active ? "Quality Check 运行中" : terminal ? "Attempt 已结束" : "运行 Quality Check"}
      </button>
      {terminal ? (
        <button type="button" onClick={startFreshAttempt} style={{ marginLeft: 8 }}>
          开始新的 Quality Check
        </button>
      ) : null}
      {!enabled ? <small style={{ display: "block", marginTop: 8 }}>Quality 写入默认关闭；受信任 POC 可由服务端启用。</small> : null}
      {enabled && !usable ? <small style={{ display: "block", marginTop: 8 }}>Core 只允许 READY / SUPERSEDED DatasetVersion 运行 Quality Check。</small> : null}
      {state.message ? <p role={state.ok ? "status" : "alert"}>{state.message}</p> : null}
      {state.ok ? <button type="button" onClick={() => window.location.reload()}>刷新评测结果</button> : null}
      {state.refreshRequired ? <button type="button" onClick={() => window.location.reload()}>刷新并核对 attempt 状态</button> : null}
    </form>
  );
}
