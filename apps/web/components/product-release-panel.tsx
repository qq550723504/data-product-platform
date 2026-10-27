"use client";
import {businessText,displayText} from '../lib/business-language.mjs';

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
      return gateActions[gate] ?? "请按阻断原因处理未通过项";
    default:
      return "请查看发布检查详情，确认各项检查状态";
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
        {businessText(pending ? "正在发布，请勿重复操作…" : item.release.status === "PUBLISHED" ? "已发布" : "发布 Release")}
      </button>
      {displayText(!enabled ? <small style={{ display: "block", marginTop: 8 }}>当前环境未开放发布提交。</small> : null)}
      {displayText(enabled && !publishable && item.release.status !== "PUBLISHED" ? (
        <small style={{ display: "block", marginTop: 8 }}>发布准备和检查均须就绪，所有必需检查通过且无阻断项，才能发布。</small>
      ) : null)}
      {displayText(state.message ? <p role={state.ok ? "status" : "alert"}>{businessText(state.message)}</p> : null)}
      {displayText(state.refreshRequired ? <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button> : null)}
    </form>
  );
}

export function ProductReleasePanel({ productId, items, actionsEnabled }: { productId: string; items: ReleasePanelItem[]; actionsEnabled: boolean }) {
  if (items.length === 0) return <div className="empty-state"><strong>暂无 发布记录</strong><p>请先创建发布记录，固定产品版本和所含成果版本。</p></div>;
  return (
    <div style={{ display: "grid", gap: 18 }}>
      {items.map((item) => {
        const problem = releaseReadinessProblem(item.readiness);
        return (
          <article className="detail-card" key={item.release.id}>
            <div className="panel-header">
              <div><span className="eyebrow">发布记录 {displayText(item.release.releaseNo)}</span><h2 style={{ marginTop: 6 }}>{displayText(shortId(item.release.id))}</h2></div>
              <div className="badge-row"><Badge value={item.release.status} /><Badge value={item.readiness.overall} /></div>
            </div>
            <p>
              产品版本 <span className="mono">{displayText(shortId(item.release.productVersionId))}</span>
              {displayText(item.release.releasedAt ? ` · 发布于 ${formatDate(item.release.releasedAt)}` : ` · 创建于 ${formatDate(item.release.createdAt)}`)}
            </p>
            <p style={{ marginTop: 8 }}>
              <Link className="text-link" href={`/products/${productId}/releases/${item.release.id}`}>查看 发布记录 → 成果版本 → 运行记录 → 证据记录 完整证据链 →</Link>
            </p>
            <section className="readiness-checklist" aria-label="发布记录 发布检查 Checklist">
              <div className="readiness-checklist-header">
                <div>
                  <span className="eyebrow">发布记录 发布检查</span>
                  <h3>发布门禁清单</h3>
                </div>
                <Badge value={item.readiness.overall} />
              </div>
              <div className="readiness-gates">
                {[
                  ...requiredReleaseGates,
                  ...Object.keys(item.readiness.checks ?? {}).filter((gate) => !(requiredReleaseGates as readonly string[]).includes(gate)),
                ].map((gate) => {
                  const status = gateState(item.readiness.checks?.[gate]);
                  return (
                    <div
                      className={`readiness-gate readiness-gate-${status.toLowerCase()}`}
                      key={gate}
                      data-testid={`readiness-${gate}`}
                    >
                      <span className="readiness-gate-symbol" aria-hidden="true">{displayText(gateSymbol(status))}</span>
                      <div className="readiness-gate-copy">
                        <strong>{displayText(gateLabels[gate] ?? gate)}</strong>
                        <small>{businessText(gateHint(gate, status))}</small>
                      </div>
                      <Badge value={status} />
                    </div>
                  );
                })}
              </div>
              {problem ? (
                <div className="readiness-blockers" data-testid="readiness-problem">
                  <strong>发布条件尚未完整通过</strong>
                  <p>{displayText(problem)}</p>
                  {Array.isArray(item.readiness.blockers) && item.readiness.blockers.length > 0 ? (
                    <ul>
                      {displayText(item.readiness.blockers
                        .filter((value): value is string => typeof value === "string")
                        .map((value) => <li key={value}><code>{value}</code></li>))}
                    </ul>
                  ) : null}
                  <Link className="text-link" href={`/products/${productId}/releases/${item.release.id}`}>
                    查看完整 发布记录 证据链与冻结引用 →
                  </Link>
                </div>
              ) : (
                <div className="readiness-ready">
                  <span aria-hidden="true">✓</span>
                  <div><strong>所有 发布检查 检查项 已通过</strong><small>发布记录 已具备发布门禁条件。</small></div>
                </div>
              )}
            </section>
            <details style={{ marginTop: 16 }}>
              <summary>冻结引用与 发布检查 详情</summary>
              <dl className="definition-list" style={{ marginTop: 12 }}>
                <div><dt>Contract Version</dt><dd className="mono">{displayText(item.release.contractVersionId ?? "—")}</dd></div>
                <div><dt>Rights Snapshot</dt><dd className="mono">{displayText(item.release.rightsSnapshotId ?? "—")}</dd></div>
                <div><dt>Quality 结果记录</dt><dd className="mono">{displayText(item.release.qualityResultId ?? "—")}</dd></div>
                <div><dt>Compliance 结果记录</dt><dd className="mono">{displayText(item.release.complianceResultId ?? "—")}</dd></div>
                <div><dt>证据记录 Snapshot</dt><dd className="mono">{displayText(item.release.evidenceSnapshotId ?? "—")}</dd></div>
              </dl>
              {item.release.datasets?.length ? (
                <div style={{ marginTop: 12 }}><strong>成果版本 bindings</strong><ul>{item.release.datasets.map((binding) => <li key={`${binding.role}-${binding.datasetVersionId}`}><code>{binding.role}</code> · <span className="mono">{displayText(binding.datasetVersionId)}</span></li>)}</ul></div>
              ) : null}
              {displayText(item.readiness.details && Object.keys(item.readiness.details).length ? <pre className="json-preview">{JSON.stringify(item.readiness.details, null, 2)}</pre> : null)}
            </details>
            <PublishForm productId={productId} item={item} enabled={actionsEnabled} />
          </article>
        );
      })}
    </div>
  );
}
