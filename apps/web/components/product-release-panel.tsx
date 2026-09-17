"use client";

import Link from "next/link";
import { useActionState } from "react";
import { publishRelease } from "@/app/products/[id]/actions";
import { Badge, formatDate, shortId } from "@/components/ui";
import type { ProductRelease, ReleaseReadiness } from "@/lib/platform";
import type { ReleaseActionState } from "@/lib/release-command";

const gateOrder = ["production", "dataset", "rights", "quality", "compliance", "contract", "evidence", "delivery"];
const gateLabels: Record<string, string> = {
  production: "生产结果",
  dataset: "数据集",
  rights: "权利",
  quality: "质量",
  compliance: "合规",
  contract: "Data Contract",
  evidence: "证据",
  delivery: "交付资产",
};

export type ReleasePanelItem = {
  release: ProductRelease;
  readiness: ReleaseReadiness;
};

function PublishForm({ productId, item, enabled }: { productId: string; item: ReleasePanelItem; enabled: boolean }) {
  const [state, action, pending] = useActionState<ReleaseActionState, FormData>(publishRelease, { ok: false, message: "" });
  const allPass = gateOrder.every((gate) => item.readiness.checks[gate] === "PASS");
  const publishable = enabled && item.release.status === "READY" && item.readiness.overall === "READY" && allPass;
  const locked = pending || state.ok || state.refreshRequired === true;

  return (
    <form action={action} style={{ marginTop: 16 }}>
      <input type="hidden" name="productId" value={productId} />
      <input type="hidden" name="releaseId" value={item.release.id} />
      <button type="submit" disabled={!publishable || locked}>
        {pending ? "正在发布，请勿重复操作…" : item.release.status === "PUBLISHED" ? "已发布" : "发布 Release"}
      </button>
      {!enabled ? <small style={{ display: "block", marginTop: 8 }}>发布写入默认关闭；受信任 POC 可由服务端启用。</small> : null}
      {enabled && !publishable && item.release.status !== "PUBLISHED" ? (
        <small style={{ display: "block", marginTop: 8 }}>只有 Core 同时返回 Release=READY、Readiness=READY 且全部 Gate=PASS 时才能发布。</small>
      ) : null}
      {state.message ? <p role={state.ok ? "status" : "alert"}>{state.message}</p> : null}
      {state.refreshRequired ? <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button> : null}
    </form>
  );
}

export function ProductReleasePanel({ productId, items, actionsEnabled }: { productId: string; items: ReleasePanelItem[]; actionsEnabled: boolean }) {
  if (items.length === 0) {
    return <div className="empty-state"><strong>暂无 Release</strong><p>先由 Core 创建冻结了 ProductVersion 与 DatasetVersion 的 Release。</p></div>;
  }

  return (
    <div style={{ display: "grid", gap: 18 }}>
      {items.map((item) => (
        <article className="detail-card" key={item.release.id}>
          <div className="panel-header">
            <div>
              <span className="eyebrow">Release {item.release.releaseNo}</span>
              <h2 style={{ marginTop: 6 }}>{shortId(item.release.id)}</h2>
            </div>
            <div className="badge-row"><Badge value={item.release.status} /><Badge value={item.readiness.overall} /></div>
          </div>

          <p>
            ProductVersion <span className="mono">{shortId(item.release.productVersionId)}</span>
            {item.release.releasedAt ? ` · 发布于 ${formatDate(item.release.releasedAt)}` : ` · 创建于 ${formatDate(item.release.createdAt)}`}
          </p>
          <p style={{ marginTop: 8 }}>
            <Link className="text-link" href={`/products/${productId}/releases/${item.release.id}`}>查看 Release → DatasetVersion → Execution → Evidence 完整证据链 →</Link>
          </p>

          <div className="metric-grid" style={{ marginTop: 16 }}>
            {gateOrder.map((gate) => (
              <div className="metric-card" key={gate}>
                <span>{gateLabels[gate] ?? gate}</span>
                <Badge value={item.readiness.checks[gate] ?? "PENDING"} />
              </div>
            ))}
          </div>

          {item.readiness.blockers.length > 0 ? (
            <div className="callout callout-warn" style={{ marginTop: 16 }}>
              <strong>当前 Blockers</strong>
              <p>{item.readiness.blockers.join(" · ")}</p>
            </div>
          ) : (
            <div className="callout" style={{ marginTop: 16 }}><strong>所有 Readiness Gate 已通过</strong></div>
          )}

          <details style={{ marginTop: 16 }}>
            <summary>冻结引用与 Readiness 详情</summary>
            <dl className="definition-list" style={{ marginTop: 12 }}>
              <div><dt>Contract Version</dt><dd className="mono">{item.release.contractVersionId ?? "—"}</dd></div>
              <div><dt>Rights Snapshot</dt><dd className="mono">{item.release.rightsSnapshotId ?? "—"}</dd></div>
              <div><dt>Quality Result</dt><dd className="mono">{item.release.qualityResultId ?? "—"}</dd></div>
              <div><dt>Compliance Result</dt><dd className="mono">{item.release.complianceResultId ?? "—"}</dd></div>
              <div><dt>Evidence Snapshot</dt><dd className="mono">{item.release.evidenceSnapshotId ?? "—"}</dd></div>
            </dl>
            {item.release.datasets?.length ? (
              <div style={{ marginTop: 12 }}>
                <strong>DatasetVersion bindings</strong>
                <ul>{item.release.datasets.map((binding) => <li key={`${binding.role}-${binding.datasetVersionId}`}><code>{binding.role}</code> · <span className="mono">{binding.datasetVersionId}</span></li>)}</ul>
              </div>
            ) : null}
            {item.readiness.details && Object.keys(item.readiness.details).length ? (
              <pre className="json-preview">{JSON.stringify(item.readiness.details, null, 2)}</pre>
            ) : null}
          </details>

          <PublishForm productId={productId} item={item} enabled={actionsEnabled} />
        </article>
      ))}
    </div>
  );
}
