"use client";
import {businessText,displayText} from '../lib/business-language.mjs';

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
    <form action={action} className="detail-card" style={{ marginBottom: 18 }} aria-label="作废 成果版本">
      <input type="hidden" name="datasetId" value={datasetId} />
      <input type="hidden" name="versionId" value={versionId} />
      <div className="panel-header">
        <div>
          <h2>作废当前版本</h2>
          <p>仅可用版本允许作废。原内容和历史会保留，并追加作废原因。</p>
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
        {businessText(pending ? "正在作废，请勿重复操作…" : state.ok ? "已作废" : "作废 DatasetVersion")}
      </button>
      {displayText(!enabled ? (
        <small style={{ display: "block", marginTop: 8 }}>
          当前环境未开放版本作废。
        </small>
      ) : (
        <small style={{ display: "block", marginTop: 8 }}>
          作废会追加一条处置记录，保留原版本内容和历史。
        </small>
      ))}
      {displayText(state.message ? <p role={state.ok ? "status" : "alert"}>{businessText(state.message)}</p> : null)}
      {displayText(state.refreshRequired ? (
        <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button>
      ) : null)}
    </form>
  );
}
