"use client";

import { useActionState, useEffect, useState } from "react";
import { runComplianceCheck } from "@/app/datasets/[id]/versions/[versionId]/actions";
import type { ComplianceResult } from "@/lib/platform";
import type { ComplianceActionState } from "@/lib/compliance-command";

function newAttemptId(): string {
  return globalThis.crypto?.randomUUID?.() ?? "";
}

export function ComplianceCheckForm({
  datasetId,
  versionId,
  versionStatus,
  enabled,
  initialAttemptId,
  initialPolicyRef,
  result,
}: {
  datasetId: string;
  versionId: string;
  versionStatus: string;
  enabled: boolean;
  initialAttemptId?: string;
  initialPolicyRef?: string;
  result?: ComplianceResult | null;
}) {
  const [attemptId, setAttemptId] = useState(initialAttemptId ?? "");
  const [policyRef, setPolicyRef] = useState(result?.policyRef ?? initialPolicyRef ?? "");
  const [state, action, pending] = useActionState<ComplianceActionState, FormData>(
    runComplianceCheck,
    { ok: false, message: "" },
  );

  useEffect(() => {
    if (!attemptId) setAttemptId(newAttemptId());
  }, [attemptId]);

  useEffect(() => {
    if (!state.attemptId) return;
    const url = new URL(window.location.href);
    url.searchParams.set("view", "compliance");
    url.searchParams.set("complianceAttemptId", state.attemptId);
    const effectivePolicyRef = policyRef.trim() || "park/compliance/enterprise-activity-compliance-v1.yaml";
    url.searchParams.set("compliancePolicyRef", effectivePolicyRef);
    window.history.replaceState(window.history.state, "", url.pathname + "?" + url.searchParams.toString());
    if (state.attemptId !== attemptId) setAttemptId(state.attemptId);
  }, [attemptId, policyRef, state.attemptId]);

  const usable = versionStatus === "READY" || versionStatus === "SUPERSEDED";
  const terminal = Boolean(result);
  const canSubmit = enabled && usable && Boolean(attemptId) && !pending && !terminal;

  const rememberAttempt = () => {
    if (!attemptId) return;
    const url = new URL(window.location.href);
    url.searchParams.set("view", "compliance");
    url.searchParams.set("complianceAttemptId", attemptId);
    const effectivePolicyRef = policyRef.trim() || "park/compliance/enterprise-activity-compliance-v1.yaml";
    url.searchParams.set("compliancePolicyRef", effectivePolicyRef);
    window.history.replaceState(window.history.state, "", url.pathname + "?" + url.searchParams.toString());
  };

  const startFreshAttempt = () => {
    const url = new URL(window.location.href);
    url.searchParams.set("view", "compliance");
    url.searchParams.delete("complianceAttemptId");
    url.searchParams.delete("compliancePolicyRef");
    window.location.assign(url.pathname + "?" + url.searchParams.toString());
  };

  return (
    <form
      action={action}
      onSubmit={rememberAttempt}
      className="detail-card"
      style={{ marginBottom: 18 }}
      aria-label="运行 Compliance Check"
    >
      <input type="hidden" name="datasetId" value={datasetId} />
      <input type="hidden" name="versionId" value={versionId} />
      <input type="hidden" name="assessmentAttemptId" value={attemptId} />

      <div className="panel-header">
        <div>
          <span className="eyebrow">Recoverable compliance attempt</span>
          <h3>运行 Compliance Check</h3>
          <p>attemptId 会保留在当前 URL；刷新后先恢复同一个 ComplianceResult，不会盲目创建第二条历史事实。</p>
        </div>
      </div>

      <div className="definition-list">
        <div><dt>Attempt</dt><dd className="mono">{attemptId || "正在生成…"}</dd></div>
        <div>
          <dt>Policy</dt>
          <dd>
            <input
              aria-label="Compliance Policy"
              name="policyRef"
              value={policyRef}
              onChange={(event) => setPolicyRef(event.target.value)}
              maxLength={1024}
              placeholder="留空使用 Core 默认合规策略"
            />
          </dd>
        </div>
      </div>

      {result ? (
        <p role="status" data-testid="compliance-attempt-status">
          <strong>SUCCEEDED · Result {result.id.slice(0, 8)} · {result.gateDecision}</strong>
        </p>
      ) : null}

      <button type="submit" disabled={!canSubmit}>
        {pending ? "正在执行，请勿重复操作…" : terminal ? "Attempt 已结束" : "运行 Compliance Check"}
      </button>
      {terminal ? (
        <button type="button" onClick={startFreshAttempt} style={{ marginLeft: 8 }}>
          开始新的 Compliance Check
        </button>
      ) : null}
      {!enabled ? <small style={{ display: "block", marginTop: 8 }}>Compliance 写入默认关闭；受信任 POC 可由服务端启用。</small> : null}
      {enabled && !usable ? <small style={{ display: "block", marginTop: 8 }}>Core 只允许 READY / SUPERSEDED DatasetVersion 运行 Compliance Check。</small> : null}
      {state.message ? <p role={state.ok ? "status" : "alert"}>{state.message}</p> : null}
      {state.ok ? <button type="button" onClick={() => window.location.reload()}>刷新合规结果</button> : null}
      {state.refreshRequired ? <button type="button" onClick={() => window.location.reload()}>刷新并核对 attempt 状态</button> : null}
    </form>
  );
}
