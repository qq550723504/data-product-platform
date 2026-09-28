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

const gateDescriptions: Record<string, string> = {
  production: "确认目标 DatasetVersion 有可信生产 Execution、匹配 WorkflowVersion，并具备完整生产血缘。",
  dataset: "确认 Release 绑定的 DatasetVersion 当前仍可读取且没有失效。",
  rights: "确认冻结 RightsSnapshot 与当前产品、用途和授权要求一致。",
  quality: "确认冻结 QualityResult 对应目标 DatasetVersion 且 Gate 可接受。",
  compliance: "确认冻结 ComplianceResult 对应目标 DatasetVersion 且通过合规 Gate。",
  contract: "确认 Release 绑定已发布且匹配当前产品的 Data Contract。",
  evidence: "确认 Release 有足够的 Evidence 支撑并可进入冻结证据快照。",
  delivery: "确认 ProductVersion 定义了可交付资产。",
};

const gateAnchors: Record<string, string> = {
  production: "release-production",
  dataset: "release-dataset",
  rights: "release-governance",
  quality: "release-governance",
  compliance: "release-governance",
  contract: "release-governance",
  evidence: "release-evidence",
  delivery: "release-delivery",
};

function blockerGate(blocker: string): string | null {
  if (blocker.startsWith("PRODUCTION_")) return "production";
  if (blocker.startsWith("DATASET_")) return "dataset";
  if (blocker.startsWith("RIGHTS_") || blocker.startsWith("ENTITLEMENT_")) return "rights";
  if (blocker.startsWith("QUALITY_")) return "quality";
  if (blocker.startsWith("COMPLIANCE_")) return "compliance";
  if (blocker.startsWith("CONTRACT_")) return "contract";
  if (blocker.startsWith("EVIDENCE_")) return "evidence";
  if (blocker.startsWith("DELIVERY_")) return "delivery";
  if (requiredReleaseGates.includes(blocker as (typeof requiredReleaseGates)[number])) return blocker;
  return null;
}

function blockersForGate(blockers: string[], gate: string): string[] {
  return blockers.filter((blocker) => blockerGate(blocker) === gate);
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
            <div className="readiness-checklist" style={{ marginTop: 16 }}>
              {requiredReleaseGates.map((gate) => {
                const status = typeof item.readiness.checks?.[gate] === "string" ? item.readiness.checks[gate] : "UNKNOWN";
                const gateBlockers = blockersForGate(
                  Array.isArray(item.readiness.blockers)
                    ? item.readiness.blockers.filter((value): value is string => typeof value === "string")
                    : [],
                  gate,
                );
                const target = `/products/${productId}/releases/${item.release.id}#${gateAnchors[gate]}`;
                const actionRequired = status !== "PASS" || gateBlockers.length > 0;
                const visualState = actionRequired ? (status === "PENDING" ? "pending" : "fail") : "pass";
                return (
                  <div
                    className={`readiness-check readiness-check-${visualState}`}
                    key={gate}
                    data-testid={`readiness-${gate}`}
                  >
                    <div className="readiness-check-main">
                      <span className="readiness-check-icon" aria-hidden="true">
                        {!actionRequired ? "✓" : status === "PENDING" ? "…" : "!"}
                      </span>
                      <div>
                        <div className="readiness-check-title">
                          <strong>{gateLabels[gate]}</strong>
                          <Badge value={status} />
                          {status === "PASS" && gateBlockers.length > 0 ? <Badge value="BLOCKED" tone="bad" /> : null}
                        </div>
                        <p>{gateDescriptions[gate]}</p>
                        {gateBlockers.length ? (
                          <div className="readiness-blockers">
                            {gateBlockers.map((blocker) => <code key={blocker}>{blocker}</code>)}
                          </div>
                        ) : null}
                      </div>
                    </div>
                    {actionRequired ? <Link className="text-link" href={target}>处理 / 查看依据 →</Link> : null}
                  </div>
                );
              })}
            </div>
            {problem ? (
              <div className="callout callout-warn" style={{ marginTop: 16 }} data-testid="readiness-problem">
                <strong>发布条件尚未完整通过</strong><p>{problem}</p>
                {(() => {
                  const blockers = Array.isArray(item.readiness.blockers)
                    ? item.readiness.blockers.filter((value): value is string => typeof value === "string")
                    : [];
                  const unknown = blockers.filter((blocker) => blockerGate(blocker) === null);
                  return unknown.length ? <p>其他 Core blocker：{unknown.join(" · ")}</p> : null;
                })()}
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
