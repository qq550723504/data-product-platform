"use client";

import Link from "next/link";
import { useActionState } from "react";
import { publishRelease } from "@/app/products/[id]/actions";
import { Badge, formatDate, shortId } from "@/components/ui";
import type { ProductRelease, ReleaseReadiness } from "@/lib/platform";
import type { ReleaseActionState } from "@/lib/release-command";
import { requiredReleaseGates, releaseReadinessProblem } from "@/lib/release-readiness";

const gateLabels: Record<string, string> = {
  production: "生产结果", dataset: "数据集", rights: "权利", quality: "质量",
  compliance: "合规", contract: "Data Contract", evidence: "证据", delivery: "交付资产",
};

const gateActions: Record<string, string> = {
  production: "若绑定 DatasetVersion 缺少不可变的生产 provenance，重新生产有 Execution lineage 的 DatasetVersion 并创建新 Release",
  dataset: "若绑定 DatasetVersion 已不可用，生产或选择替代版本并创建新 Release",
  rights: "若当前 Release 允许重绑，创建并冻结该 Release 的新 Rights Snapshot 后重新绑定；否则先创建新 Release，再为新 Release 创建并冻结 Rights Snapshot",
  quality: "处理 Quality gate 失败项",
  compliance: "处理 Compliance gate 阻塞项",
  contract: "若 Release 绑定错误，改绑到 ProductVersion 已冻结的 Contract；若冻结 Contract 缺失或错误，创建新 ProductVersion 和新 Release",
  evidence: "为目标 DatasetVersion 补齐 Readiness 所需的 Evidence 关系",
  delivery: "创建包含交付资产的新 ProductVersion，并基于该版本创建新 Release",
};

function gateState(value: unknown): string {
  return typeof value === "string" && value.trim() ? value : "UNKNOWN";
}

function gateSymbol(status: string): string {
  switch (status.toUpperCase()) {
    case "PASS":
      return "✓";
    case "FAIL":
      return "×";
    case "PENDING":
      return "…";
    default:
      return "?";
  }
}

function gateHint(gate: string, status: string): string {
  switch (status.toUpperCase()) {
    case "PASS":
      return "Gate 已通过";
    case "PENDING":
      return "先执行 Release Validation；此 Gate 尚未评估";
    case "FAIL":
      return gateActions[gate] ?? "根据 Core blocker 处理失败条件";
    default:
      return "查看 Core Readiness 详情，确认该 Gate 的当前状态";
  }
}

export type ReleasePanelItem = { release: ProductRelease; readiness: ReleaseReadiness };

function PublishForm({ productId, item, enabled }: { productId: string; item: ReleasePanelItem; enabled: boolean }) {
  const [state, action, pending] = useActionState<ReleaseActionState, FormData>(publishRelease, { ok: false, message: "" });
  const publishable = enabled && item.release.status === "READY" && releaseReadinessProblem(item.readiness) === null;
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
        <small style={{ display: "block", marginTop: 8 }}>只有 Core 同时返回 Release=READY、Readiness=READY、全部 Gate=PASS 且没有阻塞原因时才能发布。</small>
      ) : null}
      {state.message ? <p role={state.ok ? "status" : "alert"}>{state.message}</p> : null}
      {state.refreshRequired ? <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button> : null}
    </form>
  );
}

export function ProductReleasePanel({ productId, items, actionsEnabled }: { productId: string; items: ReleasePanelItem[]; actionsEnabled: boolean }) {
  if (items.length === 0) return <div className="empty-state"><strong>暂无 Release</strong><p>先由 Core 创建冻结了 ProductVersion 与 DatasetVersion 的 Release。</p></div>;
  return (
    <div style={{ display: "grid", gap: 18 }}>
      {items.map((item) => {
        const problem = releaseReadinessProblem(item.readiness);
        return (
          <article className="detail-card" key={item.release.id}>
            <div className="panel-header">
              <div><span className="eyebrow">Release {item.release.releaseNo}</span><h2 style={{ marginTop: 6 }}>{shortId(item.release.id)}</h2></div>
              <div className="badge-row"><Badge value={item.release.status} /><Badge value={item.readiness.overall} /></div>
            </div>
            <p>
              ProductVersion <span className="mono">{shortId(item.release.productVersionId)}</span>
              {item.release.releasedAt ? ` · 发布于 ${formatDate(item.release.releasedAt)}` : ` · 创建于 ${formatDate(item.release.createdAt)}`}
            </p>
            <p style={{ marginTop: 8 }}>
              <Link className="text-link" href={`/products/${productId}/releases/${item.release.id}`}>查看 Release → DatasetVersion → Execution → Evidence 完整证据链 →</Link>
            </p>
            <section className="readiness-checklist" aria-label="Release Readiness Checklist">
              <div className="readiness-checklist-header">
                <div>
                  <span className="eyebrow">Release Readiness</span>
                  <h3>发布门禁清单</h3>
                </div>
                <Badge value={item.readiness.overall} />
              </div>
              <div className="readiness-gates">
                {requiredReleaseGates.map((gate) => {
                  const status = gateState(item.readiness.checks?.[gate]);
                  return (
                    <div
                      className={`readiness-gate readiness-gate-${status.toLowerCase()}`}
                      key={gate}
                      data-testid={`readiness-${gate}`}
                    >
                      <span className="readiness-gate-symbol" aria-hidden="true">{gateSymbol(status)}</span>
                      <div className="readiness-gate-copy">
                        <strong>{gateLabels[gate]}</strong>
                        <small>{gateHint(gate, status)}</small>
                      </div>
                      <Badge value={status} />
                    </div>
                  );
                })}
              </div>
              {problem ? (
                <div className="readiness-blockers" data-testid="readiness-problem">
                  <strong>发布条件尚未完整通过</strong>
                  <p>{problem}</p>
                  {Array.isArray(item.readiness.blockers) && item.readiness.blockers.length > 0 ? (
                    <ul>
                      {item.readiness.blockers
                        .filter((value): value is string => typeof value === "string")
                        .map((value) => <li key={value}><code>{value}</code></li>)}
                    </ul>
                  ) : null}
                  <Link className="text-link" href={`/products/${productId}/releases/${item.release.id}`}>
                    查看完整 Release 证据链与冻结引用 →
                  </Link>
                </div>
              ) : (
                <div className="readiness-ready">
                  <span aria-hidden="true">✓</span>
                  <div><strong>所有 Readiness Gate 已通过</strong><small>Release 已具备发布门禁条件。</small></div>
                </div>
              )}
            </section>
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
                <div style={{ marginTop: 12 }}><strong>DatasetVersion bindings</strong><ul>{item.release.datasets.map((binding) => <li key={`${binding.role}-${binding.datasetVersionId}`}><code>{binding.role}</code> · <span className="mono">{binding.datasetVersionId}</span></li>)}</ul></div>
              ) : null}
              {item.readiness.details && Object.keys(item.readiness.details).length ? <pre className="json-preview">{JSON.stringify(item.readiness.details, null, 2)}</pre> : null}
            </details>
            <PublishForm productId={productId} item={item} enabled={actionsEnabled} />
          </article>
        );
      })}
    </div>
  );
}
