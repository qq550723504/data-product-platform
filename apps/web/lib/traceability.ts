const apiBaseUrl = (process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080").replace(/\/$/, "");

export type SnapshotItem = { evidenceId: string; category: string };
export type EvidenceSnapshotTrace = {
  id: string;
  rootHash: string;
  integrityValid: boolean;
  manifest: Record<string, unknown>;
  items: SnapshotItem[];
  createdAt: string;
};

export type DatasetVersionTrace = {
  id: string;
  datasetId: string;
  datasetCode: string;
  datasetType: string;
  sourceResourceId?: string;
  versionNo: number;
  status: string;
  releaseRole?: string;
  storageUri?: string;
  checksumAlgorithm?: string;
  checksumValue?: string;
  generatedByExecutionId?: string;
};

export type ExecutionTrace = {
  id: string;
  workflowVersionId: string;
  workflowVersion: string;
  workflowDefinitionHash: string;
  status: string;
  attempt: number;
  engineType: string;
  engineExecutionId?: string;
  targetPeriod: string;
  outputDatasetVersionId?: string;
  metrics: Record<string, unknown>;
};

export type EntityMatchJobTrace = {
  id: string;
  workspaceId: string;
  entityTypeId: string;
  inputDatasetVersionId: string;
  outputDatasetVersionId?: string;
  sourceType: string;
  sourceRef: string;
  policyRef: string;
  policyVersion: string;
  status: string;
};

export type EntityMappingTrace = {
  id: string;
  decisionId: string;
  entityId: string;
  sourceType: string;
  sourceRef: string;
  sourceKey: string;
  sourceName?: string;
  matchMethod: string;
  matchRuleId?: string;
  matchPolicyVersion: string;
  confidence: number;
  status: string;
  reviewedBy?: string;
  reviewedAt?: string;
  reviewerReason?: string;
  evidenceId?: string;
  sourceOrigin: string;
  sourceJobId?: string;
  decidedAt: string;
};

export type EvidenceItem = {
  id: string;
  workspaceId: string;
  evidenceType: string;
  title?: string;
  sourceType?: string;
  sourceId?: string;
  storageUri?: string;
  hashAlgorithm?: string;
  hashValue?: string;
  integrityValid: boolean;
  metadata: Record<string, unknown>;
  relationType: string;
  createdAt: string;
  createdBy?: string;
};

export type CostEvent = {
  id: string;
  workspaceId: string;
  executionId?: string;
  costType: string;
  quantity: number;
  unit: string;
  amount?: number;
  currency?: string;
  pricingMode: string;
  metadata: Record<string, unknown>;
  occurredAt: string;
};

export type AuditEventTrace = {
  id: string;
  action: string;
  objectType: string;
  objectId: string;
  actorType: string;
  actorId?: string;
  reason?: string;
  metadata: Record<string, unknown>;
  occurredAt: string;
};

export type ReleaseTrace = {
  releaseId: string;
  releaseNo: string;
  status: string;
  productId: string;
  productVersionId: string;
  evidenceSnapshot?: EvidenceSnapshotTrace;
  datasetVersions: DatasetVersionTrace[];
  executions: ExecutionTrace[];
  entityMatchJobs: EntityMatchJobTrace[];
  entityMappings: EntityMappingTrace[];
  evidence: EvidenceItem[];
  costEvents: CostEvent[];
  auditEvents: AuditEventTrace[];
};

export class TraceabilityError extends Error {
  constructor(message: string, readonly status: number, readonly code?: string) {
    super(message);
    this.name = "TraceabilityError";
  }
}

export async function productReleaseTrace(releaseId: string): Promise<ReleaseTrace> {
  const response = await fetch(`${apiBaseUrl}/api/v1/traceability/product-releases/${encodeURIComponent(releaseId)}`, {
    cache: "no-store",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    let code: string | undefined;
    try {
      const payload = await response.json() as { code?: string; error?: { code?: string } };
      code = payload.error?.code ?? payload.code;
    } catch { /* Keep stable UI error; never surface raw database/provider detail. */ }
    throw new TraceabilityError(`Release traceability unavailable (HTTP ${response.status}${code ? `, ${code}` : ""})`, response.status, code);
  }
  return await response.json() as ReleaseTrace;
}
