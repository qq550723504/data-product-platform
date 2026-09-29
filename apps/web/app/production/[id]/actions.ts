"use server";

import { randomUUID } from "node:crypto";
import { revalidatePath } from "next/cache";
import { configuredWorkspaceId } from "@/lib/platform";
import { executeRetry, type ExecutionActionState } from "@/lib/execution-command";

export async function retryExecution(
  _previous: ExecutionActionState,
  form: FormData,
): Promise<ExecutionActionState> {
  const result = await executeRetry(
    form,
    {
      enabled: process.env.POC_ENABLE_EXECUTION_ACTIONS === "true",
      workspaceId: configuredWorkspaceId(),
      actorId: process.env.POC_EXECUTION_ACTOR_ID?.trim(),
      apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
    },
    randomUUID(),
  );

  const executionId = form.get("executionId");
  if ((result.ok || result.refreshRequired) && typeof executionId === "string") {
    revalidatePath(`/production/${executionId}`);
    revalidatePath("/production");
    revalidatePath("/attention");
    revalidatePath("/");
  }
  return result;
}
