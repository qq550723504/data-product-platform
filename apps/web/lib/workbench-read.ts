import type { DataProduct, EntityReview, ExecutionSummary, PageResult, ProductRelease, ReleaseReadiness, WorkbenchSummary, WorkspaceReleaseAttention } from "./platform";
import { isIdentity } from "./quality-attention";

// A failed read carries no invented value and never exposes an upstream error.
// The deadline bounds this page's wait; it is not a business/runtime timeout.
export async function readSection<T>(load: () => Promise<T>, valid: (value: T) => boolean, timeoutMs = 60_000): Promise<T | null> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const value = await Promise.race([
      Promise.resolve().then(load),
      new Promise<never>((_, reject) => { timer = setTimeout(() => reject(new Error("read unavailable")), timeoutMs); }),
    ]);
    return valid(value) ? value : null;
  } catch {
    return null;
  } finally {
    clearTimeout(timer);
  }
}

export const validCount = (value: number) => Number.isSafeInteger(value) && value >= 0;
const text = (value: string) => typeof value === "string";
const time = (value: string) => text(value) && Number.isFinite(Date.parse(value));
const same = (a: string, b: string) => isIdentity(a) && isIdentity(b) && a.toLowerCase() === b.toLowerCase();

export function attentionTotal(counts: Array<number | null | undefined>): number | null {
  if (!counts.length || !counts.every((value): value is number => value !== null && value !== undefined && validCount(value))) return null;
  const total = counts.reduce((sum, value) => sum + value, 0);
  return validCount(total) ? total : null;
}

export function validSummary(s: WorkbenchSummary, workspace: string): boolean {
  return same(s.workspaceId, workspace) && time(s.updatedAt)
    && [s.counts.dataResources, s.counts.datasets, s.counts.dataProducts,
      s.reviewQueue.pending, s.reviewQueue.unresolved, s.reviewQueue.conflicts,
      s.executions.queued, s.executions.submitting, s.executions.running, s.executions.succeeded, s.executions.failed,
      s.releases.draft, s.releases.ready, s.releases.published, s.releases.failed].every(validCount)
    && attentionTotal([s.executions.queued, s.executions.submitting, s.executions.running]) !== null;
}

export function validPage<T>(p: PageResult<T>, limit: number, offset: number, item: (value: T) => boolean): boolean {
  return Array.isArray(p.items) && validCount(p.page.total) && p.page.limit === limit && p.page.offset === offset
    && p.items.length === Math.min(limit, Math.max(0, p.page.total - offset)) && p.items.every(item);
}

export function validExecution(e: ExecutionSummary, workspace: string): boolean {
  return isIdentity(e.id) && same(e.workspaceId, workspace) && text(e.workflowName) && text(e.workflowCode)
    && text(e.workflowVersion) && text(e.targetPeriod) && text(e.status) && text(e.errorCode)
    && validCount(e.attempt) && time(e.createdAt) && (e.finishedAt === undefined || time(e.finishedAt));
}

export function validReview(r: EntityReview, workspace: string): boolean {
  return isIdentity(r.candidateId) && same(r.workspaceId, workspace) && r.status === "PENDING"
    && text(r.sourceName) && text(r.sourceKey) && text(r.matchMethod) && Number.isFinite(r.confidence);
}

export function validProduct(p: DataProduct, workspace: string): boolean {
  return isIdentity(p.id) && same(p.workspaceId, workspace) && text(p.name);
}

export function validRelease(r: ProductRelease, status: string): boolean {
  return isIdentity(r.id) && isIdentity(r.productId) && r.status === status && text(r.releaseNo) && time(r.createdAt);
}

export function validReleaseAttention(r: WorkspaceReleaseAttention, limit: number): boolean {
  return validPage(r.failed, limit, 0, value => validRelease(value, "FAILED"))
    && validPage(r.validating, limit, 0, value => validRelease(value, "VALIDATING"))
    && validPage(r.ready, limit, 0, value => validRelease(value, "READY"));
}

export function validReadiness(r: ReleaseReadiness, release: string): boolean {
  return same(r.releaseId, release) && text(r.overall) && Array.isArray(r.blockers) && r.blockers.every(text);
}
