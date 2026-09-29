"use server";

import { revalidatePath } from "next/cache";
import { configuredWorkspaceId } from "@/lib/platform";
import {
  executeInvalidateDatasetVersion,
  type DatasetVersionActionState,
} from "@/lib/dataset-version-command";
import { executeQualityCheck, type QualityActionState } from "@/lib/quality-command";

export async function invalidateDatasetVersion(
  _previous: DatasetVersionActionState,
  form: FormData,
): Promise<DatasetVersionActionState> {
  const result = await executeInvalidateDatasetVersion(form, {
    enabled: process.env.POC_ENABLE_DATASET_ACTIONS === "true",
    workspaceId: configuredWorkspaceId(),
    actorId: process.env.POC_DATASET_ACTOR_ID?.trim(),
    apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
  });

  const datasetId = form.get("datasetId");
  const versionId = form.get("versionId");
  if ((result.ok || result.refreshRequired) && typeof datasetId === "string" && typeof versionId === "string") {
    revalidatePath(`/datasets/${datasetId}/versions/${versionId}`);
    revalidatePath(`/datasets/${datasetId}`);
    revalidatePath("/datasets");
    revalidatePath("/products");
    revalidatePath("/attention");
    revalidatePath("/");
  }
  return result;
}


export async function runQualityCheck(
  _previous: QualityActionState,
  form: FormData,
): Promise<QualityActionState> {
  const result = await executeQualityCheck(form, {
    enabled: process.env.POC_ENABLE_QUALITY_ACTIONS === "true",
    workspaceId: configuredWorkspaceId(),
    actorId: process.env.POC_QUALITY_ACTOR_ID?.trim(),
    apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
  });

  const datasetId = form.get("datasetId");
  const versionId = form.get("versionId");
  if ((result.ok || result.refreshRequired) && typeof datasetId === "string" && typeof versionId === "string") {
    revalidatePath(`/datasets/${datasetId}/versions/${versionId}`);
    revalidatePath(`/datasets/${datasetId}`);
    revalidatePath("/datasets");
    revalidatePath("/products");
    revalidatePath("/");
  }
  return result;
}
