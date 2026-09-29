"use client";

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
        {pending ? "正在创建 Retry…" : state.ok ? "Retry 已创建" : "重试 Execution"}
      </button>
      {!enabled ? (
        <small style={{ display: "block", marginTop: 8 }}>
          Execution 写入默认关闭；受信任 POC 可由服务端启用。
        </small>
      ) : (
        <small style={{ display: "block", marginTop: 8 }}>
          Retry 会创建新的 QUEUED Execution，并保留当前记录作为不可变历史。
        </small>
      )}
      {state.message ? <p role={state.ok ? "status" : "alert"}>{state.message}</p> : null}
      {state.refreshRequired ? (
        <button type="button" onClick={() => window.location.reload()}>重新加载并核对结果</button>
      ) : null}
      {state.ok && state.execution?.id ? (
        <p><Link className="text-link" href={`/production/${state.execution.id}`}>打开新的 Execution →</Link></p>
      ) : null}
    </form>
  );
}
