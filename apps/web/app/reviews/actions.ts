"use server";

import { revalidatePath } from "next/cache";
import { configuredWorkspaceId } from "@/lib/platform";
import { executeReview, type ReviewActionState } from "@/lib/review-command";

export async function reviewCandidate(
  _previous: ReviewActionState,
  form: FormData,
): Promise<ReviewActionState> {
  const result = await executeReview(form, {
    enabled: process.env.POC_ENABLE_REVIEW_ACTIONS === "true",
    workspaceId: configuredWorkspaceId(),
    actorId: process.env.POC_REVIEWER_ID?.trim(),
    apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080",
  });
  if (result.ok || result.refreshRequired) {
    revalidatePath("/reviews");
    revalidatePath("/");
    revalidatePath("/datasets");
  }
  return result;
}
