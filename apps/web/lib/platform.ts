const apiBaseUrl = (process.env.PLATFORM_API_BASE_URL ?? "http://localhost:8080").replace(/\/$/, "");

export type PageMeta = { limit: number; offset: number; total: number };
export type PageResult<T> = { items: T[]; page: PageMeta };

export type WorkbenchSummary = {
  workspaceId: string;
  counts: { dataResources: number; datasets: number; dataProducts: number };
  reviewQueue: { pending: number; unresolved: number; conflicts: number };
  executions: { queued: number; submitting: number; running: number; succeeded: number; failed: number };
  releases: { draft: number; ready: number; published: number; failed: number };
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
  qualityStatus: string;
  complianceStatus: string;
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

export type EntityMatchJob = {
  id: string;
  workspaceId: string;
  entityTypeId: string;
  inputDatasetVersionId: string;
  outputDatasetId: string;
  outputDatasetVersionId?: string;
  sourceRole: string;
  policyRef: string;
  policyVersion: string;
  status: string;
  autoMatchCount: number;
  reviewCount: number;
  unresolvedCount: number;
  rejectedCount: number;
  errorMessage: string;
};

export type JobReviewCandidate = {
  id: string;
  jobId: string;
  sourceKey: string;
  sourceName: string;
  candidateEntityId?: string;
  decision: string;
  status: string;
  matchMethod: string;
  matchRuleId: string;
  matchEngineName: string;
  matchEngineVersion: string;
  matchModelVersion: string;
  confidence: number;
  source: Record<string, string>;
  normalized: Record<string, string>;
  reviewerReason: string;
  evidenceId?: string;
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

export type ProductDetail = Omit<DataProduct, "currentVersion" | "latestReleaseNo" | "latestReleaseStatus" | "updatedAt"> & {
  ownerId?: string;
  metadata: Record<string, unknown>;
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
  releaseNotes: string;
  createdAt: string;
  releasedAt?: string;
};

export type ProductReleaseDetail = ProductRelease & {
  datasets: Array<{ datasetVersionId: string; role: string }>;
  metadata: Record<string, unknown>;
};

export type ReleaseReadiness = {
  releaseId: string;
  overall: "READY" | "NOT_READY" | string;
  checks: Record<string, "PASS" | "FAIL" | "PENDING" | string>;
  blockers: string[];
  details?: Record<string, unknown>;
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

export type ReleaseTrace = {
  releaseId: string;
  releaseNo: string;
  status: string;
  productId: string;
  productVersionId: string;
  evidenceSnapshot?: {
    id: string;
    rootHash: string;
    integrityValid: boolean;
    manifest: Record<string, unknown>;
    items: Array<Record<string, unknown>>;
    createdAt: string;
  };
  datasetVersions: Array<{
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
  }>;
  executions: Array<{
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
  }>;
  entityMatchJobs: Array<Record<string, unknown>>;
  entityMappings: Array<Record<string, unknown>>;
  evidence: EvidenceItem[];
  costEvents: Array<Record<string, unknown>>;
  auditEvents: Array<Record<string, unknown>>;
};

export class PlatformError extends Error {
  constructor(message: string, readonly status: number, readonly code?: string) {
    super(message);
    this.name = "PlatformError";
  }
}

export function configuredWorkspaceId(): string | null {
  const value = process.env.POC_WORKSPACE_ID?.trim();
  return value || null;
}

export function configuredActorId(): string | null {
  const value = process.env.POC_ACTOR_ID?.trim();
  return value || null;
}

async function parseError(response: Response): Promise<PlatformError> {
  let message = `Core API returned HTTP ${response.status}`;
  let code: string | undefined;
  try {
    const payload = (await response.json()) as { message?: string; detail?: string; code?: string; title?: string };
    message = payload.message ?? payload.detail ?? payload.title ?? message;
    code = payload.code;
  } catch {
    // Preserve a stable UI-facing error when the upstream body is not JSON.
  }
  return new PlatformError(message, response.status, code);
}

async function apiGet<T>(path: string): Promise<T> {
  const response = await fetch(`${apiBaseUrl}${path}`, {
    cache: "no-store",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) throw await parseError(response);
  return (await response.json()) as T;
}

async function apiPost<T>(
  path: string,
  body: unknown,
  options: { actorId?: string | null; idempotencyKey?: string } = {},
): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json", "Content-Type": "application/json" };
  if (options.actorId) headers["X-Actor-ID"] = options.actorId;
  if (options.idempotencyKey) headers["Idempotency-Key"] = options.idempotencyKey;
  const response = await fetch(`${apiBaseUrl}${path}`, {
    method: "POST",
    cache: "no-store",
    headers,
    body: JSON.stringify(body ?? {}),
  });
  if (!response.ok) throw await parseError(response);
  return (await response.json()) as T;
}

function requireWorkspace(): string {
  const workspaceId = configuredWorkspaceId();
  if (!workspaceId) throw new PlatformError("POC_WORKSPACE_ID is not configured", 503, "WORKSPACE_NOT_CONFIGURED");
  return workspaceId;
}

function workspacePath(path: string): string {
  return `/api/v1/workspaces/${encodeURIComponent(requireWorkspace())}${path}`;
}

export const platform = {
  workbench: () => apiGet<WorkbenchSummary>(workspacePath("/workbench")),
  resources: (limit = 100, offset = 0) =>
    apiGet<PageResult<DataResource>>(workspacePath(`/data-resources?limit=${limit}&offset=${offset}`)),
  resource: (id: string) => apiGet<DataResource>(workspacePath(`/data-resources/${encodeURIComponent(id)}`)),
  datasets: (limit = 100, offset = 0) =>
    apiGet<PageResult<Dataset>>(workspacePath(`/datasets?limit=${limit}&offset=${offset}`)),
  dataset: (id: string) => apiGet<Dataset>(workspacePath(`/datasets/${encodeURIComponent(id)}`)),
  datasetVersions: (id: string, limit = 100, offset = 0) =>
    apiGet<PageResult<DatasetVersion>>(workspacePath(`/datasets/${encodeURIComponent(id)}/versions?limit=${limit}&offset=${offset}`)),
  executions: (limit = 100, offset = 0) =>
    apiGet<PageResult<ExecutionSummary>>(workspacePath(`/executions?limit=${limit}&offset=${offset}`)),
  execution: (id: string) => apiGet<ExecutionDetail>(`/api/v1/executions/${encodeURIComponent(id)}`),
  reviews: (status = "", limit = 100, offset = 0) => {
    const filter = status ? `&status=${encodeURIComponent(status)}` : "";
    return apiGet<PageResult<EntityReview>>(workspacePath(`/entity-match-reviews?limit=${limit}&offset=${offset}${filter}`));
  },
  entityMatchJob: (jobId: string) => apiGet<EntityMatchJob>(`/api/v1/entity-match-jobs/${encodeURIComponent(jobId)}`),
  jobReviews: (jobId: string) => apiGet<{ items: JobReviewCandidate[] }>(`/api/v1/entity-match-jobs/${encodeURIComponent(jobId)}/reviews`),
  confirmReview: (candidateId: string, reason: string, actorId: string) =>
    apiPost<EntityMatchJob>(`/api/v1/entity-match-reviews/${encodeURIComponent(candidateId)}/confirm`, { reason }, { actorId }),
  rejectReview: (candidateId: string, reason: string, actorId: string) =>
    apiPost<EntityMatchJob>(`/api/v1/entity-match-reviews/${encodeURIComponent(candidateId)}/reject`, { reason }, { actorId }),
  products: (limit = 100, offset = 0) =>
    apiGet<PageResult<DataProduct>>(workspacePath(`/data-products?limit=${limit}&offset=${offset}`)),
  product: (id: string) => apiGet<ProductDetail>(`/api/v1/data-products/${encodeURIComponent(id)}`),
  productVersion: (id: string) => apiGet<ProductVersion>(`/api/v1/product-versions/${encodeURIComponent(id)}`),
  releases: (productId: string, limit = 100, offset = 0) =>
    apiGet<PageResult<ProductRelease>>(workspacePath(`/data-products/${encodeURIComponent(productId)}/releases?limit=${limit}&offset=${offset}`)),
  release: (id: string) => apiGet<ProductReleaseDetail>(`/api/v1/product-releases/${encodeURIComponent(id)}`),
  readiness: (id: string) => apiGet<ReleaseReadiness>(`/api/v1/product-releases/${encodeURIComponent(id)}/readiness`),
  publishRelease: (id: string, actorId: string | null, idempotencyKey: string) =>
    apiPost<ProductReleaseDetail>(`/api/v1/product-releases/${encodeURIComponent(id)}/publish`, {}, { actorId, idempotencyKey }),
  releaseTrace: (id: string) => apiGet<ReleaseTrace>(`/api/v1/traceability/product-releases/${encodeURIComponent(id)}`),
};
