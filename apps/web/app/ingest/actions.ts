"use server";
import { revalidatePath } from "next/cache";
import { configuredWorkspaceId } from "@/lib/platform";
import { importCSV, startImportedResolution, type IngestState } from "@/lib/ingest-command";
function configuration() {
  return { enabled: process.env.POC_ENABLE_INGEST_ACTIONS === "true", workspaceId: configuredWorkspaceId(), actorId: process.env.POC_INGEST_ACTOR_ID?.trim(), apiBaseUrl: process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080" };
}
function refresh(result: IngestState) {
  if (result.locked) for (const path of ["/", "/resources", "/datasets", "/reviews"]) revalidatePath(path);
  return result;
}
export async function uploadCSV(operationId: string, _previous: IngestState, form: FormData): Promise<IngestState> {
  return refresh(await importCSV(operationId, form, configuration()));
}
export async function resolveCSV(_previous: IngestState, form: FormData): Promise<IngestState> {
  return refresh(await startImportedResolution(form, configuration()));
}
