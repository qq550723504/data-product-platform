"use client";

import { useEffect, useMemo, useState } from "react";

type Recovery = {
  operationId: string;
  profileId: string;
  datasetVersionId: string;
  retryOfDeliveryOperationId?: string | null;
  idempotencyKey: string;
  status: string;
  gateDecision: string;
  consumer: string;
  purpose: string;
  action: string;
  scopeRef: string;
  terminalReason?: string;
};

type Props = {
  datasetId: string;
  versionId: string;
  enabled: boolean;
  allowed: boolean;
  profileId: string;
  consumer: string;
  purpose: string;
  action: string;
  delivery: string;
  scopeType: string;
  scopeRef: string;
  initialKey?: string;
  initialRetryOf?: string;
};

function newKey() {
  return globalThis.crypto?.randomUUID?.() ?? "";
}

export function DirectDataDeliveryForm(props: Props) {
  const [key, setKey] = useState(props.initialKey ?? "");
  const [recovery, setRecovery] = useState<Recovery | null>(null);
  const [retryOf, setRetryOf] = useState<string | undefined>(props.initialRetryOf);
  const [message, setMessage] = useState("");
  const [pending, setPending] = useState(false);

  const directData = props.delivery.trim().toUpperCase() === "DIRECT_DATA";
  const canDeliver = props.enabled && props.allowed && directData && !pending;

  const freezeUrl = (nextKey: string, retryParent = retryOf) => {
    const url = new URL(window.location.href);
    url.searchParams.set("view", "eligibility");
    url.searchParams.set("profileId", props.profileId);
    url.searchParams.set("consumer", props.consumer);
    url.searchParams.set("purpose", props.purpose);
    url.searchParams.set("action", props.action);
    url.searchParams.set("delivery", props.delivery);
    url.searchParams.set("scopeType", props.scopeType);
    if (props.scopeRef) url.searchParams.set("scopeRef", props.scopeRef); else url.searchParams.delete("scopeRef");
    url.searchParams.set("deliveryAttemptKey", nextKey);
    if (retryParent) url.searchParams.set("deliveryRetryOf", retryParent); else url.searchParams.delete("deliveryRetryOf");
    window.history.replaceState(window.history.state, "", url.pathname + "?" + url.searchParams.toString());
  };

  const recover = async (attemptKey: string) => {
    const params = new URLSearchParams({ idempotencyKey: attemptKey, consumer: props.consumer, versionId: props.versionId });
    const response = await fetch(`/api/direct-data-deliveries?${params.toString()}`, { cache: "no-store" });
    if (response.status === 404) {
      setRecovery(null);
      return null;
    }
    if (!response.ok) {
      const payload = await response.json().catch(() => null);
      setMessage(payload?.error?.message ?? "无法恢复 Direct Data attempt。");
      return null;
    }
    const value = await response.json() as Recovery;
    if (value.profileId !== props.profileId || value.consumer !== props.consumer || value.purpose !== props.purpose || value.action !== props.action) {
      setMessage("恢复的 DeliveryOperation 与当前请求上下文不一致。");
      return null;
    }
    setRecovery(value);
    return value;
  };

  useEffect(() => {
    if (!props.initialKey) return;
    void recover(props.initialKey);
  // The request context is frozen in the page URL before the attempt is submitted.
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [props.initialKey]);

  const statusText = useMemo(() => {
    if (!recovery) return null;
    return `${recovery.status} · Operation ${recovery.operationId.slice(0, 8)}${recovery.terminalReason ? " · " + recovery.terminalReason : ""}`;
  }, [recovery]);

  const startFresh = (retryIssued: boolean) => {
    const next = newKey();
    setKey(next);
    const retryParent = retryIssued ? recovery?.operationId : undefined;
    setRetryOf(retryParent);
    setRecovery(null);
    setMessage("");
    freezeUrl(next, retryParent);
  };

  const download = async () => {
    let attemptKey = key;
    if (!attemptKey) {
      attemptKey = newKey();
      setKey(attemptKey);
      freezeUrl(attemptKey);
    } else {
      freezeUrl(attemptKey);
    }
    setPending(true);
    setMessage("");
    try {
      const response = await fetch("/api/direct-data-deliveries", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          versionId: props.versionId,
          profileId: props.profileId,
          consumer: props.consumer,
          purpose: props.purpose,
          action: props.action,
          scopeType: props.scopeType,
          scopeRef: props.scopeRef,
          idempotencyKey: attemptKey,
          ...(retryOf ? { retryOfDeliveryOperationId: retryOf } : {}),
        }),
      });
      if (!response.ok) {
        const payload = await response.json().catch(() => null);
        setMessage(payload?.error?.message ?? "Direct Data 下载失败。");
        await recover(attemptKey);
        return;
      }
      const blob = await response.blob();
      const href = URL.createObjectURL(blob);
      const anchor = document.createElement("a");
      anchor.href = href;
      anchor.download = `dataset-version-${props.versionId}`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(href);
      setMessage("Direct Data 下载已授权并开始。");
      await recover(attemptKey);
    } catch {
      setMessage("下载响应不确定；请刷新恢复当前 idempotency key，不要直接创建新 attempt。");
    } finally {
      setPending(false);
    }
  };

  const terminal = recovery?.status === "ISSUED" || recovery?.status === "BLOCKED" || recovery?.status === "FAILED";
  const buttonDisabled = !canDeliver || (recovery?.status === "ISSUED");

  return (
    <section className="detail-card" style={{ marginTop: 18 }} data-testid="direct-data-delivery">
      <div className="panel-header">
        <div>
          <h3>Direct Data 下载</h3>
          <p>浏览器不会接触 Delivery API token。idempotency key 会在请求前写入 URL；响应丢失后先恢复 Operation，再决定是否创建新 attempt。</p>
        </div>
        <span className="eyebrow">Trusted server proxy</span>
      </div>
      <div className="definition-list">
        <div><dt>Idempotency Key</dt><dd className="mono">{key || "提交时生成"}</dd></div>
        <div><dt>Consumer</dt><dd>{props.consumer}</dd></div>
        <div><dt>Delivery</dt><dd>{props.delivery}</dd></div>
      </div>
      {statusText ? <p role="status" data-testid="delivery-operation-status"><strong>{statusText}</strong></p> : null}
      {message ? <p role="status">{message}</p> : null}
      <button type="button" onClick={download} disabled={buttonDisabled}>
        {pending ? "正在请求下载…" : recovery?.status === "ISSUED" ? "该 attempt 已签发" : "下载 Direct Data"}
      </button>
      {terminal ? (
        <button type="button" style={{ marginLeft: 8 }} onClick={() => startFresh(recovery?.status === "ISSUED")}>
          {recovery?.status === "ISSUED" ? "创建重试下载 attempt" : "创建新的下载 attempt"}
        </button>
      ) : null}
      {!props.enabled ? <small style={{ display: "block", marginTop: 8 }}>Direct Data 下载默认关闭；需由服务端启用并配置 trusted delivery identity。</small> : null}
      {props.enabled && !props.allowed ? <small style={{ display: "block", marginTop: 8 }}>当前 Delivery Eligibility 为 BLOCKED，不能发起下载。</small> : null}
      {props.enabled && !directData ? <small style={{ display: "block", marginTop: 8 }}>当前 Profile 不是 DIRECT_DATA delivery。</small> : null}
    </section>
  );
}
