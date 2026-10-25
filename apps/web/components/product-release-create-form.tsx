"use client";
import {businessText,displayText} from '../lib/business-language.mjs';

import { useActionState, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import { createReleaseDraft } from "@/app/products/[id]/actions";
import type { CreateReleaseActionState } from "@/lib/release-command";

export type ReleaseDatasetCandidate = {
  assetId: string;
  assetName: string;
  datasetId: string;
  datasetName: string;
  datasetCode: string;
  versions: Array<{ id: string; versionNo: number; status: string }>;
};

type Selection = { versionId: string; role: string };

export function ProductReleaseCreateForm({
  productId,
  productVersionId,
  candidates,
  enabled,
}: {
  productId: string;
  productVersionId: string;
  candidates: ReleaseDatasetCandidate[];
  enabled: boolean;
}) {
  const router = useRouter();
  const [releaseNo, setReleaseNo] = useState("");
  const [releaseNotes, setReleaseNotes] = useState("");
  const [selections, setSelections] = useState<Record<string, Selection>>({});
  const [state, action, pending] = useActionState<CreateReleaseActionState, FormData>(
    createReleaseDraft,
    { ok: false, message: "" },
  );

  const selected = useMemo(
    () => candidates.flatMap((candidate) => {
      const selection = selections[candidate.assetId];
      return selection?.versionId ? [{ candidate, selection }] : [];
    }),
    [candidates, selections],
  );
  const productionTargets = selected.filter((item) => item.selection.role === "PRIMARY" || item.selection.role === "OUTPUT").length;
  const complete = selected.length > 0
    && selected.every((item) => item.selection.role)
    && productionTargets === 1
    && releaseNo.trim().length > 0;
  const locked = pending || state.ok || state.refreshRequired === true;

  if (candidates.length === 0) {
    return (
      <div className="callout callout-warn" style={{ marginBottom: 18 }}>
        <strong>当前 产品版本 没有可用于创建 发布记录 的 DATASET asset</strong>
        <p>发布记录 需要显式冻结 成果版本 bindings；界面不会从其他 数据集 自动猜测版本。</p>
      </div>
    );
  }

  return (
    <form action={action} className="detail-card" style={{ marginBottom: 18 }} aria-label="创建 发布记录">
      <input type="hidden" name="productId" value={productId} />
      <input type="hidden" name="productVersionId" value={productVersionId} />
      {displayText(selected.map(({ candidate, selection }) => (
        <span key={candidate.assetId}>
          <input type="hidden" name="datasetVersionId" value={selection.versionId} />
          <input type="hidden" name="datasetRole" value={selection.role} />
        </span>
      )))}

      <div className="panel-header">
        <div>
          <span className="eyebrow">Explicit frozen bindings</span>
          <h2>创建 发布草稿</h2>
          <p>请选择明确的成果版本及其用途；提交时将重新校验产品版本、工作空间和成果可用性。</p>
        </div>
      </div>

      <div className="definition-list">
        <div>
          <dt>发布批次号</dt>
          <dd><input aria-label="发布批次号" name="releaseNo" value={releaseNo} onChange={(event) => setReleaseNo(event.target.value)} maxLength={64} required /></dd>
        </div>
        <div>
          <dt>发布说明</dt>
          <dd><textarea aria-label="发布说明" name="releaseNotes" value={releaseNotes} onChange={(event) => setReleaseNotes(event.target.value)} maxLength={2000} rows={3} /></dd>
        </div>
      </div>

      <div className="table-card" style={{ marginTop: 16 }}>
        <table className="data-table">
          <thead><tr><th>产品版本 Asset</th><th>成果版本</th><th>Role</th></tr></thead>
          <tbody>
            {candidates.map((candidate) => {
              const selection = selections[candidate.assetId] ?? { versionId: "", role: "" };
              return (
                <tr key={candidate.assetId}>
                  <td className="primary-cell">
                    <strong>{displayText(candidate.assetName)}</strong>
                    <span>{displayText(candidate.datasetName)} · {displayText(candidate.datasetCode)}</span>
                  </td>
                  <td>
                    <select
                      aria-label={businessText(`${candidate.assetName} DatasetVersion`)}
                      value={selection.versionId}
                      onChange={(event) => setSelections((current) => ({
                        ...current,
                        [candidate.assetId]: { ...selection, versionId: event.target.value },
                      }))}
                    >
                      <option value="">不绑定</option>
                      {candidate.versions.map((version) => (
                        <option key={version.id} value={version.id}>
                          v{displayText(version.versionNo)} · {businessText(version.status)} · {displayText(version.id.slice(0, 8))}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <select
                      aria-label={businessText(`${candidate.assetName} Role`)}
                      value={selection.role}
                      disabled={!selection.versionId}
                      onChange={(event) => setSelections((current) => ({
                        ...current,
                        [candidate.assetId]: { ...selection, role: event.target.value },
                      }))}
                    >
                      <option value="">选择 Role</option>
                      <option value="PRIMARY">主成果</option>
                      <option value="OUTPUT">输出成果</option>
                      <option value="INPUT">输入成果</option>
                      <option value="SUPPORTING">SUPPORTING</option>
                    </select>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {displayText(selected.length > 0 && productionTargets !== 1 ? (
        <div className="callout callout-warn" style={{ marginTop: 12 }}>
          <strong>需要且只能有一个生产目标</strong>
          <p>发布绑定中须恰好指定一个主成果或输出成果。</p>
        </div>
      ) : null)}

      <button type="submit" disabled={!enabled || !complete || locked} style={{ marginTop: 14 }}>
        {businessText(pending ? "正在创建，请勿重复操作…" : state.ok ? "Release 已创建" : "创建 Release Draft")}
      </button>
      {displayText(!enabled ? (
        <small style={{ display: "block", marginTop: 8 }}>当前环境未开放发布准备提交。</small>
      ) : (
        <small style={{ display: "block", marginTop: 8 }}>
          如果响应不确定，请保留相同 发布批次号；下一次提交会先核对是否已存在同一冻结 发布记录。
        </small>
      ))}
      {displayText(state.message ? <p role={state.ok ? "status" : "alert"}>{businessText(state.message)}</p> : null)}
      {displayText(state.refreshRequired ? (
        <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button>
      ) : null)}
      {displayText(state.ok && state.release ? (
        <button type="button" onClick={() => router.refresh()}>刷新 发布记录 列表</button>
      ) : null)}
    </form>
  );
}
