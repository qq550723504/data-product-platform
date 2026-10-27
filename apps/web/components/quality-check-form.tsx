"use client";
import {businessText,displayText} from '../lib/business-language.mjs';

import { useActionState, useEffect, useMemo, useState } from "react";
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
    if (attempt.outcome === "IN_PROGRESS" && attempt.leaseExpired) return "本次评测等待时间已结束，可核实原评测进度。";
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
    const url = new URL(window.location.href);
    url.searchParams.set("view", "quality");
    url.searchParams.delete("qualityAttemptId");
    window.location.assign(url.pathname + "?" + url.searchParams.toString());
  };

  return (
    <form
      action={action}
      onSubmit={rememberAttempt}
      className="detail-card"
      style={{ marginBottom: 18 }}
      aria-label="运行 质量评测"
    >
      <input type="hidden" name="datasetId" value={datasetId} />
      <input type="hidden" name="versionId" value={versionId} />
      <input type="hidden" name="assessmentAttemptId" value={attemptId} />

      <div className="panel-header">
        <div>
          <span className="eyebrow">Recoverable assessment 评测记录</span>
          <h3>运行 质量评测</h3>
          <p>提交前会保存本次评测编号；刷新后继续查询同一次评测，避免重复运行。</p>
        </div>
      </div>

      <div className="definition-list">
        <div><dt>评测记录</dt><dd className="mono">{displayText(attemptId || "正在生成…")}</dd></div>
        <div>
          <dt>Rule Set</dt>
          <dd>
            <input
              aria-label="质量规则集"
              name="ruleSetRef"
              value={ruleSetRef}
              onChange={(event) => setRuleSetRef(event.target.value)}
              maxLength={512}
              placeholder="留空使用默认规则集"
            />
          </dd>
        </div>
        <div>
          <dt>Engine</dt>
          <dd>
            <input
              aria-label="评测引擎"
              name="engineName"
              value={engineName}
              onChange={(event) => setEngineName(event.target.value)}
              maxLength={128}
              placeholder="留空使用默认评测引擎"
            />
          </dd>
        </div>
      </div>

      {displayText(statusText ? <p role="status" data-testid="quality-attempt-status"><strong>{businessText(statusText)}</strong></p> : null)}
      <button type="submit" disabled={!canSubmit}>
        {businessText(pending ? "正在执行，请勿重复操作…" : active ? "Quality Check 运行中" : terminal ? "Attempt 已结束" : "运行 Quality Check")}
      </button>
      {displayText(terminal ? (
        <button type="button" onClick={startFreshAttempt} style={{ marginLeft: 8 }}>
          开始新的 质量评测
        </button>
      ) : null)}
      {displayText(!enabled ? <small style={{ display: "block", marginTop: 8 }}>当前环境未开放质量评测提交。</small> : null)}
      {displayText(enabled && !usable ? <small style={{ display: "block", marginTop: 8 }}>仅可用或已被替代的成果版本可执行质量评测。</small> : null)}
      {displayText(state.message ? <p role={state.ok ? "status" : "alert"}>{businessText(state.message)}</p> : null)}
      {displayText(state.ok ? <button type="button" onClick={() => window.location.reload()}>刷新评测结果</button> : null)}
      {displayText(state.refreshRequired ? <button type="button" onClick={() => window.location.reload()}>刷新并核对 评测记录 状态</button> : null)}
    </form>
  );
}
