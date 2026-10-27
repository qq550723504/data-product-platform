"use client";
import {businessText,displayText} from '../lib/business-language.mjs';

import Link from "next/link";
import { useActionState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { retryExecution } from "@/app/production/[id]/actions";
import type { ExecutionActionState } from "@/lib/execution-command";

export function ExecutionRetryForm({
  executionId,
  status,
  enabled,
}: {
  executionId: string;
  status: string;
  enabled: boolean;
}) {
  const router = useRouter();
  const [state, action, pending] = useActionState<ExecutionActionState, FormData>(
    retryExecution,
    { ok: false, message: "" },
  );
  const retryable = status === "FAILED" || status === "CANCELLED";
  const locked = pending || state.ok || state.refreshRequired === true;

  useEffect(() => {
    if (state.ok && state.execution?.id) {
      router.push(`/production/${state.execution.id}`);
    }
  }, [router, state.execution?.id, state.ok]);

  if (!retryable) return null;

  return (
    <form action={action} style={{ marginTop: 16 }}>
      <input type="hidden" name="executionId" value={executionId} />
      <button type="submit" disabled={!enabled || locked}>
        {businessText(pending ? "正在创建 Retry…" : state.ok ? "Retry 已创建" : "重试 Execution")}
      </button>
      {displayText(!enabled ? (
        <small style={{ display: "block", marginTop: 8 }}>
          当前环境未开放任务重试。
        </small>
      ) : (
        <small style={{ display: "block", marginTop: 8 }}>
          Retry 会创建新的 待运行 运行记录，并保留当前记录作为不可变历史。
        </small>
      ))}
      {displayText(state.message ? <p role={state.ok ? "status" : "alert"}>{businessText(state.message)}</p> : null)}
      {displayText(state.refreshRequired ? (
        <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button>
      ) : null)}
      {displayText(state.ok && state.execution?.id ? (
        <p><Link className="text-link" href={`/production/${state.execution.id}`}>打开新的 运行记录 →</Link></p>
      ) : null)}
    </form>
  );
}
