const apiBaseUrl = (process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080").replace(/\/$/, "");

export type PageMeta = {
  limit: number;
  offset: number;
  total: number;
};

export type PageResult<T> = {
  items: T[];
  page: PageMeta;
};

export type WorkbenchSummary = {
  workspaceId: string;
  counts: {
    dataResources: number;
    datasets: number;
    dataProducts: number;
  };
  reviewQueue: {
    pending: number;
    unresolved: number;
    conflicts: number;
  };
  executions: {
    queued: number;
    submitting: number;
    running: number;
    succeeded: number;
    failed: number;
  };
  releases: {
    draft: number;
    ready: number;
    published: number;
    failed: number;
  };
  updatedAt: string;
};

export type DataResource = {
  id: string;
  workspaceId: string;
  projectId?: string;
  code: string;
  name: string;
  description: string;
  domainCode: string;
  resourceType: string;
  ownerId?: string;
  sensitivityLevel: string;
  rightsStatus: string;
  qualityStatus: string;
  lifecycleStatus: string;
  revision: number;
  createdAt: string;
  updatedAt: string;
};

export type Dataset = {
  id: string;
  workspaceId: string;
  projectId?: string;
  code: string;
  name: string;
  description: string;
  datasetType: "RAW" | "STANDARDIZED" | "CURATED" | "PRODUCT" | string;
  sourceResourceId?: string;
  ownerId?: string;
  currentVersionId?: string;
  lifecycleStatus: string;
  createdAt: string;
  updatedAt: string;
};

export type DatasetVersion = {
  id: string;
  datasetId: string;
  versionNo: number;
  status: string;
  schemaVersion: string;
  storageType: string;
  storageUri: string;
  contentType: string;
  rowCount?: number;
  byteSize?: number;
  checksumAlgorithm: string;
  checksum: string;
  generatedByExecutionId?: string;
  rightsSnapshotId?: string;
  createdAt: string;
  readyAt?: string;
  invalidatedAt?: string;
  invalidationReason: string;
};

export type ExecutionSummary = {
  id: string;
  workspaceId: string;
  workflowVersionId: string;
  workflowCode: string;
  workflowName: string;
  workflowVersion: string;
  outputDatasetId: string;
  outputDatasetVersionId?: string;
  targetPeriod: string;
  status: string;
  attempt: number;
  engineType: string;
  errorCode: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
};

export type ExecutionDetail = {
  id: string;
  workspaceId: string;
  workflowVersionId: string;
  outputDatasetId: string;
  outputDatasetVersionId?: string;
  targetPeriod: string;
  status: string;
  attempt: number;
  retryOfExecutionId?: string;
  engineType: string;
  engineExecutionId: string;
  errorCode: string;
  errorMessage: string;
  metrics: Record<string, unknown>;
  inputs: Array<{ name: string; datasetVersionId: string }>;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
};

export type EntityReview = {
  candidateId: string;
  jobId: string;
  workspaceId: string;
  sourceKey: string;
  sourceName: string;
  source: Record<string, string>;
  normalized: Record<string, string>;
  candidateEntityId?: string;
  decision: string;
  status: string;
  // Optimistic-concurrency token: the mapping decision that was current when the
  // queue was rendered. Sent back so a confirmation cannot replace a decision
  // that changed after the reviewer opened the page.
  currentMappingDecisionId?: string;
  matchMethod: string;
  matchRuleId: string;
  confidence: number;
  engineName: string;
  engineVersion: string;
  modelVersion: string;
  policyRef: string;
  policyVersion: string;
  createdAt: string;
};

export type DataProduct = {
  id: string;
  workspaceId: string;
  projectId?: string;
  useCaseId?: string;
  code: string;
  name: string;
  description: string;
  domainCode: string;
  lifecycleStatus: string;
  healthStatus: string;
  currentVersionId?: string;
  currentVersion: string;
  latestReleaseId?: string;
  latestReleaseNo: string;
  latestReleaseStatus: string;
  createdAt: string;
  updatedAt: string;
};

export type ProductAsset = {
  id: string;
  assetType: string;
  name: string;
  datasetId?: string;
  externalRef: string;
  deliveryConfig: Record<string, unknown>;
  schemaSnapshot: Record<string, unknown>;
};

export type ProductVersion = {
  id: string;
  productId: string;
  version: string;
  workflowVersionId?: string;
  contractVersionId?: string;
  entityPolicyRef: string;
  indicatorSetRef: string;
  definition: Record<string, unknown>;
  assets: ProductAsset[];
  createdAt: string;
};

export type ReleaseDataset = {
  datasetVersionId: string;
  role: string;
};

export type ProductRelease = {
  id: string;
  productId: string;
  productVersionId: string;
  releaseNo: string;
  status: string;
  contractVersionId?: string;
  rightsSnapshotId?: string;
  qualityResultId?: string;
  complianceResultId?: string;
  evidenceSnapshotId?: string;
  datasets?: ReleaseDataset[];
  releaseNotes: string;
  metadata?: Record<string, unknown>;
  createdAt: string;
  releasedAt?: string;
};

export type QualityDimensionSummary = {
  dimension: string;
  status: string;
  ruleCount: number;
  evaluatedCount: number;
  failedCount: number;
};

export type QualityAssessment = {
  id: string;
  workspaceId: string;
  datasetVersionId: string;
  ruleSetRef: string;
  ruleSetVersion: string;
  ruleSetContentSha256: string;
  evaluatorName: string;
  evaluatorVersion: string;
  gateDecision: string;
  metrics: Record<string, unknown>;
  dimensionSummary: Record<string, QualityDimensionSummary>;
  createdAt: string;
};

export type QualityFinding = {
  id: string;
  resultId: string;
  ruleId: string;
  dimension: string;
  severity: string;
  status: string;
  observed?: Record<string, unknown>;
  affectedCount?: unknown;
  sample?: unknown;
  reason?: string;
  createdAt: string;
};

export type EvidenceReference = {
  id: string;
  workspaceId: string;
  evidenceType: string;
  relationType: string;
  sourceType: string;
  sourceId?: string;
  hashAlgorithm?: string;
  hashValue?: string;
  createdAt: string;
  createdBy?: string;
};

export type AuditReference = {
  id: string;
  action: string;
  objectType: string;
  objectId: string;
  actorType: string;
  actorId?: string;
  traceId?: string;
  occurredAt: string;
};

export type QualityReport = QualityAssessment & {
  findings: { items: QualityFinding[]; page: PageMeta };
  evidence: EvidenceReference[];
  auditEvents: AuditReference[];
};

export type Applicability = {
  mode: "ANY" | "EXPLICIT" | string;
  values?: string[];
};

export type ScopeApplicability = {
  mode: "ANY" | "EXPLICIT" | string;
  values?: Array<{ type: string; ref: string }>;
};

export type CertificationProfile = {
  id: string;
  workspaceId: string;
  profileRef: string;
  code: string;
  name: string;
  version: string;
  contentSha256: string;
  purpose: Applicability;
  actions: Applicability;
  consumers: Applicability;
  delivery: Applicability;
  requiredQualityDimensions: string[];
  requiredCriticalRules: string[];
  qualityGateRequired: boolean;
  rights: {
    required: boolean;
    purpose: Applicability;
    actions: Applicability;
    consumers: Applicability;
    scopes: ScopeApplicability;
  };
  complianceRequired: boolean;
  contractRequired: boolean;
  contractCode?: string;
  traceabilityRequired: boolean;
  evidenceRequired: boolean;
};

export type CertificationBlocker = { code: string; detail: string };

export type CertificationDisposition = {
  id: string;
  disposition: string;
  effectiveAt: string;
  reason: string;
  supersededByCertificationId?: string;
  evidenceSnapshotId?: string;
};

export type DatasetCertification = {
  id: string;
  workspaceId: string;
  datasetVersionId: string;
  qualityAssessmentId: string;
  decision: string;
  blockers: CertificationBlocker[];
  reason: string;
  issuedAt: string;
  rightsSnapshotId?: string;
  effectiveRightsSnapshotId?: string;
  effectiveRightsSnapshotHash?: string;
  frozenRightsContextHash?: string;
  complianceResultId?: string;
  contractVersionId?: string;
  traceabilityEvidenceId?: string;
  evidenceSnapshotId?: string;
  goldProductionBindingId?: string;
  annotationSnapshotId?: string;
  annotationSnapshotRootHash?: string;
  annotationSchemaSha256?: string;
  annotationTaxonomySha256?: string;
  goldProductionBindingRootHash?: string;
  profile: CertificationProfile;
  dispositions: CertificationDisposition[];
};

export type CertificationHistory = {
  workspaceId: string;
  datasetVersionId: string;
  asOf: string;
  items: DatasetCertification[];
};

export type DeliveryEligibility = {
  allowed: boolean;
  blockers: CertificationBlocker[];
  datasetVersion: { status: string; allowed: boolean; blockers: CertificationBlocker[] };
  certification: { allowed: boolean; blockers: CertificationBlocker[]; current?: DatasetCertification | null };
  entitlement: {
    allowed: boolean;
    blockers: CertificationBlocker[];
    checks: Array<{
      dataResourceId: string;
      path: string;
      decision: string;
      reason: string;
      authorizationId?: string;
      declarationId?: string;
      bindingId?: string;
    }>;
  };
};

export type ReleaseCheckStatus = "PASS" | "FAIL" | "PENDING" | string;
export type ReleaseReadiness = {
  releaseId: string;
  overall: "READY" | "NOT_READY" | string;
  checks: Record<string, ReleaseCheckStatus>;
  blockers: string[];
  details?: Record<string, unknown>;
};

export class PlatformError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
    this.name = "PlatformError";
  }
}

export function configuredWorkspaceId(): string | null {
  const value = process.env.POC_WORKSPACE_ID?.trim();
  return value || null;
}

async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    cache: "no-store",
    headers: { Accept: "application/json" },
  });

  if (!response.ok) {
    let message = `Core API returned HTTP ${response.status}`;
    let code: string | undefined;
    try {
      const payload = (await response.json()) as { message?: string; detail?: string; code?: string };
      message = payload.message ?? payload.detail ?? message;
      code = payload.code;
    } catch {
      // Keep a stable UI-facing error when the upstream body is not JSON.
    }
    throw new PlatformError(message, response.status, code);
  }

  return (await response.json()) as T;
}

function requireWorkspace(): string {
  const workspaceId = configuredWorkspaceId();
  if (!workspaceId) {
    throw new PlatformError("POC_WORKSPACE_ID is not configured", 503, "WORKSPACE_NOT_CONFIGURED");
  }
  return workspaceId;
}

function workspacePath(path: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(requireWorkspace())}${path}`;
}

export const platform = {
  workbench: () => apiGet<WorkbenchSummary>(workspacePath("/workbench")),
  resources: (limit = 100, offset = 0) =>
    apiGet<PageResult<DataResource>>(workspacePath(`/data-resources?limit=${limit}&offset=${offset}`)),
  resource: (id: string) =>
    apiGet<DataResource>(workspacePath(`/data-resources/${encodeURIComponent(id)}`)),
  datasets: (limit = 100, offset = 0) =>
    apiGet<PageResult<Dataset>>(workspacePath(`/datasets?limit=${limit}&offset=${offset}`)),
  dataset: (id: string) =>
    apiGet<Dataset>(workspacePath(`/datasets/${encodeURIComponent(id)}`)),
  datasetVersions: (id: string, limit = 100, offset = 0) =>
    apiGet<PageResult<DatasetVersion>>(
      workspacePath(`/datasets/${encodeURIComponent(id)}/versions?limit=${limit}&offset=${offset}`),
    ),
  datasetVersion: (versionId: string) =>
    apiGet<DatasetVersion>(
      `/api/v1/dataset-versions/${encodeURIComponent(versionId)}?workspaceId=${encodeURIComponent(requireWorkspace())}`,
    ),
  qualityAssessments: (versionId: string, limit = 25, offset = 0) =>
    apiGet<{ datasetVersionId: string; items: QualityAssessment[]; page: PageMeta }>(
      `/api/v1/dataset-versions/${encodeURIComponent(versionId)}/quality-assessments?workspaceId=${encodeURIComponent(requireWorkspace())}&limit=${limit}&offset=${offset}&summary=true`,
    ),
  qualityReport: (assessmentId: string, limit = 50, offset = 0) =>
    apiGet<QualityReport>(
      `/api/v1/quality-assessments/${encodeURIComponent(assessmentId)}/report?workspaceId=${encodeURIComponent(requireWorkspace())}&limit=${limit}&offset=${offset}`,
    ),
  certificationHistory: (versionId: string) =>
    apiGet<CertificationHistory>(
      workspacePath(`/dataset-versions/${encodeURIComponent(versionId)}/certifications`),
    ),
  deliveryEligibility: (
    versionId: string,
    input: { profileId: string; consumer: string; purpose: string; action: string; delivery: string; scopeType: string; scopeRef?: string },
  ) => {
    const query = new URLSearchParams({
      profileId: input.profileId,
      consumer: input.consumer,
      purpose: input.purpose,
      action: input.action,
      delivery: input.delivery,
      scopeType: input.scopeType,
    });
    if (input.scopeRef) query.set("scopeRef", input.scopeRef);
    return apiGet<DeliveryEligibility>(
      workspacePath(`/dataset-versions/${encodeURIComponent(versionId)}/delivery-eligibility?${query.toString()}`),
    );
  },
  executions: (limit = 100, offset = 0) =>
    apiGet<PageResult<ExecutionSummary>>(workspacePath(`/executions?limit=${limit}&offset=${offset}`)),
  execution: (id: string) => apiGet<ExecutionDetail>(`/api/v1/executions/${encodeURIComponent(id)}`),
  reviews: (status = "", limit = 100, offset = 0) => {
    const filter = status ? `&status=${encodeURIComponent(status)}` : "";
    return apiGet<PageResult<EntityReview>>(
      workspacePath(`/entity-match-reviews?limit=${limit}&offset=${offset}${filter}`),
    );
  },
  products: (limit = 100, offset = 0) =>
    apiGet<PageResult<DataProduct>>(workspacePath(`/data-products?limit=${limit}&offset=${offset}`)),
  releases: (productId: string, limit = 100, offset = 0) =>
    apiGet<PageResult<ProductRelease>>(
      workspacePath(`/data-products/${encodeURIComponent(productId)}/releases?limit=${limit}&offset=${offset}`),
    ),
  productVersion: (versionId: string) =>
    apiGet<ProductVersion>(`/api/v1/product-versions/${encodeURIComponent(versionId)}`),
  release: (releaseId: string) =>
    apiGet<ProductRelease>(`/api/v1/product-releases/${encodeURIComponent(releaseId)}`),
  releaseReadiness: (releaseId: string) =>
    apiGet<ReleaseReadiness>(`/api/v1/product-releases/${encodeURIComponent(releaseId)}/readiness`),
};
