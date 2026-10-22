import { preserveSchemaExceptionQuery } from "./schema-exception-filter";

export type AttentionView = "FOLLOW_UP" | "CLOSED" | "ALL";
export type QualityAttentionItem = {
  id: string; workspaceId: string; datasetId: string; datasetCode: string; datasetName: string;
  versionId: string; versionNo: number; resourceId: string | null;
  assessmentId: string; findingId: string; ruleId: string; message: string;
  actorId: string | null; executionId: string | null; executionStatus: string | null;
  outputDatasetId: string | null; outputVersionId: string | null;
  recheckId: string | null; recheckOutcome: string | null; recheckAssessmentId: string | null;
  recheckGateDecision: string | null; recheckDatasetId: string | null; recheckVersionId: string | null;
  pendingRecheck: boolean; activeExecution: boolean; manuallyClosed: boolean; needsFollowUp: boolean;
  createdAt: string;
};
export type QualityAttentionPage = {
  items: QualityAttentionItem[];
  page: { limit: number; offset: number; total: number };
  summary: { followUpCount: number; closedCount: number };
  readAt: string;
};

export function isIdentity(value: string): boolean {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value)
    && value !== "00000000-0000-0000-0000-000000000000";
}

export function attentionView(value?: string): AttentionView {
  return value === "CLOSED" || value === "ALL" ? value : "FOLLOW_UP";
}

export function attentionOffset(value?: string): number {
  const parsed = Number(value ?? 0);
  return Number.isSafeInteger(parsed) && parsed >= 0 && parsed <= 2147483647 ? parsed : 0;
}

export function qualityAttentionHref(query: Record<string, string | undefined>, view: AttentionView, offset: number): string {
  const next = new URLSearchParams();
  preserveSchemaExceptionQuery(query, next);
  for (const key of ["reviewOffset", "failureOffset", "failedReleaseOffset", "validatingReleaseOffset", "readyReleaseOffset"]) {
    const value = attentionOffset(query[key]);
    if (value) next.set(key, String(value));
  }
  next.set("qualityView", view);
  if (query.schemaView) next.set("schemaView", query.schemaView === "ALL" ? "ALL" : "UNHANDLED");
  const schemaOffset = attentionOffset(query.schemaOffset);
  if (schemaOffset) next.set("schemaOffset", String(schemaOffset));
  if (offset > 0) next.set("qualityOffset", String(offset));
  return `/attention?${next.toString()}#quality-attention`;
}

export function caseHref(datasetId: string, versionId: string, caseId: string): string {
  if (![datasetId, versionId, caseId].every(isIdentity)) throw new Error("异常引用无效");
  return `/datasets/${datasetId}/versions/${versionId}/remediation?caseId=${caseId}`;
}

export function assessmentHref(datasetId: string, versionId: string, assessmentId: string): string {
  if (![datasetId, versionId, assessmentId].every(isIdentity)) throw new Error("评测引用无效");
  return `/datasets/${datasetId}/versions/${versionId}?view=quality&assessmentId=${assessmentId}`;
}

export function assertReportScope<T extends { id: string; workspaceId: string; datasetVersionId: string }>(
  report: T, assessmentId: string, versionId: string, workspaceId: string,
): T {
  if (![assessmentId, versionId, workspaceId].every(isIdentity)
    || report.id.toLowerCase() !== assessmentId.toLowerCase()
    || report.workspaceId.toLowerCase() !== workspaceId.toLowerCase()
    || report.datasetVersionId.toLowerCase() !== versionId.toLowerCase()) {
    throw new Error("评测不属于所选版本或 workspace");
  }
  return report;
}

export function assertCaseScope<T extends { case: { id: string; workspace_id: string; version_id: string } }>(
  detail: T, caseId: string, versionId: string, workspaceId: string,
): T {
  if (![caseId, versionId, workspaceId].every(isIdentity)
    || detail.case.id.toLowerCase() !== caseId.toLowerCase()
    || detail.case.workspace_id.toLowerCase() !== workspaceId.toLowerCase()
    || detail.case.version_id.toLowerCase() !== versionId.toLowerCase()) {
    throw new Error("异常不属于所选版本或 workspace");
  }
  return detail;
}
