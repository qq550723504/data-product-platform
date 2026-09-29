"use client";

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
        <strong>当前 ProductVersion 没有可用于创建 Release 的 DATASET asset</strong>
        <p>Release 需要显式冻结 DatasetVersion bindings；界面不会从其他 Dataset 自动猜测版本。</p>
      </div>
    );
  }

  return (
    <form action={action} className="detail-card" style={{ marginBottom: 18 }} aria-label="创建 ProductRelease">
      <input type="hidden" name="productId" value={productId} />
      <input type="hidden" name="productVersionId" value={productVersionId} />
      {selected.map(({ candidate, selection }) => (
        <span key={candidate.assetId}>
          <input type="hidden" name="datasetVersionId" value={selection.versionId} />
          <input type="hidden" name="datasetRole" value={selection.role} />
        </span>
      ))}

      <div className="panel-header">
        <div>
          <span className="eyebrow">Explicit frozen bindings</span>
          <h2>创建 Release Draft</h2>
          <p>显式选择 DatasetVersion 和 Role；Core 会再次校验 ProductVersion、Workspace 与版本可用性。</p>
        </div>
      </div>

      <div className="definition-list">
        <div>
          <dt>Release No</dt>
          <dd><input name="releaseNo" value={releaseNo} onChange={(event) => setReleaseNo(event.target.value)} maxLength={64} required /></dd>
        </div>
        <div>
          <dt>Release Notes</dt>
          <dd><textarea name="releaseNotes" value={releaseNotes} onChange={(event) => setReleaseNotes(event.target.value)} maxLength={2000} rows={3} /></dd>
        </div>
      </div>

      <div className="table-card" style={{ marginTop: 16 }}>
        <table className="data-table">
          <thead><tr><th>ProductVersion Asset</th><th>DatasetVersion</th><th>Role</th></tr></thead>
          <tbody>
            {candidates.map((candidate) => {
              const selection = selections[candidate.assetId] ?? { versionId: "", role: "" };
              return (
                <tr key={candidate.assetId}>
                  <td className="primary-cell">
                    <strong>{candidate.assetName}</strong>
                    <span>{candidate.datasetName} · {candidate.datasetCode}</span>
                  </td>
                  <td>
                    <select
                      aria-label={`${candidate.assetName} DatasetVersion`}
                      value={selection.versionId}
                      onChange={(event) => setSelections((current) => ({
                        ...current,
                        [candidate.assetId]: { ...selection, versionId: event.target.value },
                      }))}
                    >
                      <option value="">不绑定</option>
                      {candidate.versions.map((version) => (
                        <option key={version.id} value={version.id}>
                          v{version.versionNo} · {version.status} · {version.id.slice(0, 8)}
                        </option>
                      ))}
                    </select>
                  </td>
                  <td>
                    <select
                      aria-label={`${candidate.assetName} Role`}
                      value={selection.role}
                      disabled={!selection.versionId}
                      onChange={(event) => setSelections((current) => ({
                        ...current,
                        [candidate.assetId]: { ...selection, role: event.target.value },
                      }))}
                    >
                      <option value="">选择 Role</option>
                      <option value="PRIMARY">PRIMARY</option>
                      <option value="OUTPUT">OUTPUT</option>
                      <option value="INPUT">INPUT</option>
                      <option value="SUPPORTING">SUPPORTING</option>
                    </select>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {selected.length > 0 && productionTargets !== 1 ? (
        <div className="callout callout-warn" style={{ marginTop: 12 }}>
          <strong>需要且只能有一个生产目标</strong>
          <p>Core 要求全部 bindings 中恰好一个 Role 为 PRIMARY 或 OUTPUT。</p>
        </div>
      ) : null}

      <button type="submit" disabled={!enabled || !complete || locked} style={{ marginTop: 14 }}>
        {pending ? "正在创建，请勿重复操作…" : state.ok ? "Release 已创建" : "创建 Release Draft"}
      </button>
      {!enabled ? (
        <small style={{ display: "block", marginTop: 8 }}>Release 写入默认关闭；受信任 POC 可由服务端启用。</small>
      ) : (
        <small style={{ display: "block", marginTop: 8 }}>
          如果响应不确定，请保留相同 Release No；下一次提交会先核对是否已存在同一冻结 Release。
        </small>
      )}
      {state.message ? <p role={state.ok ? "status" : "alert"}>{state.message}</p> : null}
      {state.refreshRequired ? (
        <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button>
      ) : null}
      {state.ok && state.release ? (
        <button type="button" onClick={() => router.refresh()}>刷新 Release 列表</button>
      ) : null}
    </form>
  );
}
