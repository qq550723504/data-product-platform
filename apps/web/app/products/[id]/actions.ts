"use server";

import { randomUUID } from "node:crypto";
import { revalidatePath } from "next/cache";
import { configuredWorkspaceId } from "@/lib/platform";
import {
  executeCreateRelease,
  executePublish,
  type CreateReleaseActionState,
  type ReleaseActionState,
} from "@/lib/release-command";

export async function publishRelease(
  _previous: ReleaseActionState,
  form: FormData,
): Promise<ReleaseActionState> {
  const result = await executePublish(
    form,
    {
      enabled: process.env.POC_ENABLE_RELEASE_ACTIONS === "true",
      workspaceId: configuredWorkspaceId(),
      actorId: process.env.POC_RELEASE_ACTOR_ID?.trim(),
      apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
    },
    randomUUID(),
  );

  const productId = form.get("productId");
  if ((result.ok || result.refreshRequired) && typeof productId === "string") {
    revalidatePath(`/products/${productId}`);
    revalidatePath("/products");
    revalidatePath("/");
  }
  return result;
}


export async function createReleaseDraft(
  _previous: CreateReleaseActionState,
  form: FormData,
): Promise<CreateReleaseActionState> {
  const result = await executeCreateRelease(form, {
    enabled: process.env.POC_ENABLE_RELEASE_ACTIONS === "true",
    workspaceId: configuredWorkspaceId(),
    actorId: process.env.POC_RELEASE_ACTOR_ID?.trim(),
    apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
  });

  const productId = form.get("productId");
  if ((result.ok || result.refreshRequired) && typeof productId === "string") {
    revalidatePath(`/products/${productId}`);
    revalidatePath("/products");
    revalidatePath("/evidence");
    revalidatePath("/attention");
    revalidatePath("/");
  }
  return result;
}
