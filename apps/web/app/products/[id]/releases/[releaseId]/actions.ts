"use server";

import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";
import { PlatformError, configuredActorId, configuredWorkspaceId, platform } from "@/lib/platform";

function releaseUrl(productId: string, releaseId: string, kind: "result" | "error", message: string): string {
  const params = new URLSearchParams({ [kind]: message });
  return `/products/${encodeURIComponent(productId)}/releases/${encodeURIComponent(releaseId)}?${params.toString()}`;
}

function errorMessage(error: unknown): string {
  if (error instanceof PlatformError) return error.code ? `${error.code}: ${error.message}` : error.message;
  if (error instanceof Error) return error.message;
  return "ProductRelease 发布失败";
}

export async function publishReleaseAction(formData: FormData): Promise<void> {
  const workspaceId = configuredWorkspaceId();
  const actorId = configuredActorId();
  const productId = String(formData.get("productId") ?? "").trim();
  const releaseId = String(formData.get("releaseId") ?? "").trim();

  if (!workspaceId) redirect(releaseUrl(productId, releaseId, "error", "POC_WORKSPACE_ID 未配置"));
  if (!actorId) redirect(releaseUrl(productId, releaseId, "error", "POC_ACTOR_ID 未配置，无法执行审计发布"));
  if (!productId || !releaseId) redirect("/products?error=Release 信息不完整");

  let failure: string | null = null;
  try {
    const [product, release, readiness] = await Promise.all([
      platform.product(productId),
      platform.release(releaseId),
      platform.readiness(releaseId),
    ]);
    if (product.workspaceId !== workspaceId) throw new Error("DataProduct 不属于当前 POC Workspace");
    if (release.productId !== productId) throw new Error("ProductRelease 不属于当前 DataProduct");
    if (release.status !== "READY") throw new Error(`Release 当前状态为 ${release.status}，只有 READY 可发布`);
    if (readiness.overall !== "READY") throw new Error(`Release Readiness 为 ${readiness.overall}，仍存在发布阻断项`);

    await platform.publishRelease(releaseId, actorId, `poc-publish-${releaseId}`);
  } catch (error) {
    failure = errorMessage(error);
  }

  revalidatePath("/");
  revalidatePath(`/products/${productId}`);
  revalidatePath(`/products/${productId}/releases/${releaseId}`);
  if (failure) redirect(releaseUrl(productId, releaseId, "error", failure));
  redirect(releaseUrl(productId, releaseId, "result", "Release 已发布，EvidenceSnapshot 与引用版本已冻结"));
}
