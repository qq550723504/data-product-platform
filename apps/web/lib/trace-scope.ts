import type { ReleaseTrace } from "./traceability";

const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export class TraceScopeError extends Error {
  constructor(message: string, readonly code: string) {
    super(message);
    this.name = "TraceScopeError";
  }
}

function normalized(value: string): string {
  return value.toLowerCase();
}

function validId(value: string): boolean {
  return uuidPattern.test(value) && value !== "00000000-0000-0000-0000-000000000000";
}

export function validateReleaseTraceScope(
  trace: ReleaseTrace,
  context: { workspaceId: string; productId: string; releaseId: string },
): void {
  if (!validId(context.workspaceId) || !validId(context.productId) || !validId(context.releaseId)) {
    throw new TraceScopeError("Trace scope context contains an invalid identifier.", "INVALID_SCOPE");
  }
  if (!validId(trace.releaseId) || normalized(trace.releaseId) !== normalized(context.releaseId)) {
    throw new TraceScopeError("Trace release does not match the scoped ProductRelease.", "RELEASE_MISMATCH");
  }
  if (!validId(trace.productId) || normalized(trace.productId) !== normalized(context.productId)) {
    throw new TraceScopeError("Trace product does not match the scoped Data Product.", "PRODUCT_MISMATCH");
  }
  const expectedWorkspace = normalized(context.workspaceId);
  if (trace.evidence.some((item) => !validId(item.workspaceId) || normalized(item.workspaceId) !== expectedWorkspace)) {
    throw new TraceScopeError("Trace contains Evidence from another workspace.", "EVIDENCE_WORKSPACE_MISMATCH");
  }
  if (trace.costEvents.some((item) => !validId(item.workspaceId) || normalized(item.workspaceId) !== expectedWorkspace)) {
    throw new TraceScopeError("Trace contains CostEvent from another workspace.", "COST_WORKSPACE_MISMATCH");
  }
  if (trace.entityMatchJobs.some((item) => !validId(item.workspaceId) || normalized(item.workspaceId) !== expectedWorkspace)) {
    throw new TraceScopeError("Trace contains EntityMatchJob from another workspace.", "ENTITY_JOB_WORKSPACE_MISMATCH");
  }
}
