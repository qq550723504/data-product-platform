"use client";

import { useActionState, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { invalidateDatasetVersion } from "@/app/datasets/[id]/versions/[versionId]/actions";
import type { DatasetVersionActionState } from "@/lib/dataset-version-command";

export function DatasetVersionInvalidateForm({
  datasetId,
  versionId,
  status,
  enabled,
}: {
  datasetId: string;
  versionId: string;
  status: string;
  enabled: boolean;
}) {
  const router = useRouter();
  const [reason, setReason] = useState("");
  const [state, action, pending] = useActionState<DatasetVersionActionState, FormData>(
    invalidateDatasetVersion,
    { ok: false, message: "" },
  );

  const invalidatable = status === "READY";
  const locked = pending || state.ok || state.refreshRequired === true;
  const canSubmit = enabled && invalidatable && reason.trim().length > 0 && !locked;

  useEffect(() => {
    if (state.ok) router.refresh();
  }, [router, state.ok]);

  if (!invalidatable) return null;

  return (
    <form action={action} className="detail-card" style={{ marginBottom: 18 }} aria-label="作废 DatasetVersion">
      <input type="hidden" name="datasetId" value={datasetId} />
      <input type="hidden" name="versionId" value={versionId} />
      <div className="panel-header">
        <div>
          <h2>作废当前版本</h2>
          <p>Core 只允许 READY → INVALID。内容和历史不会被覆盖；作废原因会成为版本事实。</p>
        </div>
      </div>
      <label style={{ display: "grid", gap: 8 }}>
        <span>作废原因（必填）</span>
        <textarea
          name="reason"
          value={reason}
          onChange={(event) => setReason(event.target.value)}
          maxLength={500}
          required
          rows={3}
        />
      </label>
      <button type="submit" disabled={!canSubmit} style={{ marginTop: 12 }}>
        {pending ? "正在作废，请勿重复操作…" : state.ok ? "已作废" : "作废 DatasetVersion"}
      </button>
      {!enabled ? (
        <small style={{ display: "block", marginTop: 8 }}>
          DatasetVersion 写入默认关闭；受信任 POC 可由服务端启用。
        </small>
      ) : (
        <small style={{ display: "block", marginTop: 8 }}>
          此操作不会删除或改写版本内容，只会调用 Core 的显式 invalidate 命令。
        </small>
      )}
      {state.message ? <p role={state.ok ? "status" : "alert"}>{state.message}</p> : null}
      {state.refreshRequired ? (
        <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button>
      ) : null}
    </form>
  );
}
