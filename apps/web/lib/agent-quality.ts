import { isIdentity, assertReportScope } from "./quality-attention";
import { readSection, validPage } from "./workbench-read";
import type {
  PageResult,
  QualityAssessment,
  QualityAttempt,
  QualityReport,
} from "./platform";
import { qualityGet } from "./quality-transport";

export type AgentDataset = {
  id: string;
  workspaceId: string;
  code: string;
  name: string;
  datasetType: string;
};
export type AgentVersion = {
  id: string;
  datasetId: string;
  versionNo: number;
  status: string;
  rowCount?: number;
  createdAt: string;
};
export type AgentContext = {
  workspaceId: string;
  readAt: string;
  mode: "READ_ONLY";
  dataset?: AgentDataset;
  version?: AgentVersion;
  datasets?: PageResult<AgentDataset>;
  versions?: PageResult<AgentVersion>;
  template: {
    configured: boolean;
    name?: string;
    version?: string;
    hash?: string;
    workflowHash?: string;
    inputs: { name: string; datasetType: string; required: boolean }[];
  };
  executionReadiness: "NOT_EVALUATED";
  unverified: string[];
};
export type AgentSelection = {
  datasetId?: string;
  versionId?: string;
  assessmentId?: string;
  attemptId?: string;
  offset: number;
  historyOffset: number;
  findingOffset: number;
};
export const AGENT_PAGE_SIZE = 25;
const same = (a: string, b: string) =>
  typeof a === "string" && typeof b === "string" &&
  isIdentity(a) && isIdentity(b) && a.toLowerCase() === b.toLowerCase();
const time = (value: string) =>
  typeof value === "string" && Number.isFinite(Date.parse(value));

export function agentSelection(
  query: Record<string, string | string[] | undefined>,
): AgentSelection {
  const result: AgentSelection = {
    offset: 0,
    historyOffset: 0,
    findingOffset: 0,
  };
  for (const key of [
    "datasetId",
    "versionId",
    "assessmentId",
    "attemptId",
  ] as const) {
    if (query[key] !== undefined) {
      const value = query[key];
      if (typeof value !== "string" || !isIdentity(value))
        throw new Error("无效的上下文引用");
      result[key] = value.toLowerCase();
    }
  }
  for (const key of ["offset", "historyOffset", "findingOffset"] as const) {
    if (query[key] !== undefined) {
      const value = query[key];
      if (
        typeof value !== "string" ||
        !/^\d+$/.test(value) ||
        !Number.isSafeInteger(Number(value)) ||
        Number(value) > 2147483647
      )
        throw new Error("无效的分页引用");
      result[key] = Number(value);
    }
  }
  if ((result.assessmentId || result.attemptId) && !result.versionId)
    throw new Error("请指定数据版本");
  return result;
}
export function agentHref(selection: Partial<AgentSelection> = {}): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(selection))
    if (value !== undefined && value !== 0) query.set(key, String(value));
  return `/agent/quality${query.size ? `?${query}` : ""}`;
}
export function agentContextPath(
  workspace: string,
  selection: AgentSelection,
): string {
  if (!isIdentity(workspace)) throw new Error("Workspace 未配置");
  const query = new URLSearchParams({
    limit: String(AGENT_PAGE_SIZE),
    offset: String(selection.offset),
  });
  if (selection.datasetId) query.set("datasetId", selection.datasetId);
  if (selection.versionId) query.set("versionId", selection.versionId);
  return `/api/v1/workspaces/${workspace}/agent-quality-context?${query}`;
}
export async function agentContextGet(
  workspace: string,
  selection: AgentSelection,
  base: string,
  token: string | undefined,
  request: typeof fetch = fetch,
): Promise<AgentContext> {
  return qualityGet<AgentContext>(
    agentContextPath(workspace, selection),
    base,
    token,
    request,
  );
}
export function validAgentContext(
  c: AgentContext,
  workspace: string,
  s: AgentSelection,
): boolean {
  if (!c || !c.template) return false;
  const dataset = (d: AgentDataset) =>
    !!d &&
    same(d.workspaceId, workspace) &&
    isIdentity(d.id) &&
    typeof d.name === "string" &&
    typeof d.code === "string" &&
    typeof d.datasetType === "string";
  const version = (v: AgentVersion) =>
    !!v &&
    isIdentity(v.id) &&
    !!c.dataset &&
    same(v.datasetId, c.dataset.id) &&
    Number.isSafeInteger(v.versionNo) &&
    v.versionNo > 0 &&
    typeof v.status === "string" &&
    time(v.createdAt) &&
    (v.rowCount === undefined ||
      (Number.isSafeInteger(v.rowCount) && v.rowCount >= 0));
  if (
    !same(c.workspaceId, workspace) ||
    c.mode !== "READ_ONLY" ||
    c.executionReadiness !== "NOT_EVALUATED" ||
    !time(c.readAt) ||
    typeof c.template.configured !== "boolean" ||
    !Array.isArray(c.template.inputs) ||
    !Array.isArray(c.unverified) ||
    !c.unverified.every((x) => typeof x === "string")
  )
    return false;
  if (
    c.template.configured &&
    (typeof c.template.name !== "string" || !c.template.name ||
      typeof c.template.version !== "string" || !c.template.version ||
      typeof c.template.hash !== "string" ||
      typeof c.template.workflowHash !== "string" ||
      !/^[a-f0-9]{64}$/.test(c.template.hash ?? "") ||
      !/^[a-f0-9]{64}$/.test(c.template.workflowHash ?? ""))
  )
    return false;
  if (
    !c.template.inputs.every(
      (i) =>
        !!i &&
        typeof i.name === "string" &&
        typeof i.datasetType === "string" &&
        typeof i.required === "boolean",
    )
  )
    return false;
  if (s.versionId)
    return (
      c.datasets === undefined && c.versions === undefined &&
      !!c.dataset &&
      dataset(c.dataset) &&
      !!c.version &&
      version(c.version) &&
      same(c.version.id, s.versionId) &&
      (!s.datasetId || same(c.dataset.id, s.datasetId))
    );
  if (s.datasetId)
    return (
      c.datasets === undefined && c.version === undefined &&
      !!c.dataset &&
      dataset(c.dataset) &&
      same(c.dataset.id, s.datasetId) &&
      !!c.versions &&
      validPage(c.versions, AGENT_PAGE_SIZE, s.offset, version)
    );
  return (
    c.dataset === undefined && c.version === undefined && c.versions === undefined &&
    !!c.datasets && validPage(c.datasets, AGENT_PAGE_SIZE, s.offset, dataset)
  );
}

type QualityReads = {
  history: (
    version: string,
    limit: number,
    offset: number,
  ) => Promise<{
    datasetVersionId: string;
    items: QualityAssessment[];
    page: PageResult<QualityAssessment>["page"];
  }>;
  report: (id: string, limit: number, offset: number) => Promise<QualityReport>;
  attempt: (id: string) => Promise<QualityAttempt>;
};
export async function readAgentEvidence(
  workspace: string,
  s: AgentSelection,
  reads: QualityReads,
) {
  const version = s.versionId;
  if (!version) return { history: null, report: null, attempt: null };
  // Explicit report reads are independent of the history list. No latest fallback.
  const [history, report, attempt] = await Promise.all([
    readSection(
      () => reads.history(version, AGENT_PAGE_SIZE, s.historyOffset),
      (h) =>
        same(h.datasetVersionId, version) &&
        validPage(
          h,
          AGENT_PAGE_SIZE,
          s.historyOffset,
          (a) =>
            same(a.workspaceId, workspace) &&
            same(a.datasetVersionId, version) &&
            isIdentity(a.id) &&
            typeof a.ruleSetRef === "string" &&
            typeof a.gateDecision === "string",
        ),
    ),
    s.assessmentId
      ? readSection(
          () => reads.report(s.assessmentId!, AGENT_PAGE_SIZE, s.findingOffset),
          (r) => {
            assertReportScope(r, s.assessmentId!, version, workspace);
            return (
              typeof r.gateDecision === "string" &&
              typeof r.ruleSetRef === "string" &&
              typeof r.ruleSetVersion === "string" &&
              time(r.createdAt) &&
              validPage(
                r.findings,
                AGENT_PAGE_SIZE,
                s.findingOffset,
                (f) =>
                  isIdentity(f.id) &&
                  same(f.resultId, r.id) &&
                  typeof f.ruleId === "string" &&
                  typeof f.severity === "string" &&
                  (f.reason === undefined || typeof f.reason === "string") &&
                  typeof f.status === "string",
              ) &&
              Array.isArray(r.evidence) &&
              r.evidence.every(
                (e) => isIdentity(e.id) && same(e.workspaceId, workspace) && typeof e.evidenceType==='string' && typeof e.relationType==='string' && typeof e.sourceType==='string' && (e.sourceId == null || (typeof e.sourceId==='string' && isIdentity(e.sourceId))) && (e.hashAlgorithm===undefined || typeof e.hashAlgorithm==='string') && (e.hashValue===undefined || typeof e.hashValue==='string') && time(e.createdAt),
              ) &&
              Array.isArray(r.auditEvents) &&
              r.auditEvents.every((e) => isIdentity(e.id) && same(e.objectId,r.id) && typeof e.action==='string' && typeof e.objectType==='string' && typeof e.actorType==='string' && time(e.occurredAt))
            );
          },
        )
      : Promise.resolve(null),
    s.attemptId
      ? readSection(
          () => reads.attempt(s.attemptId!),
          (a) =>
            same(a.id, s.attemptId!) &&
            same(a.workspaceId, workspace) &&
            same(a.datasetVersionId, version) &&
            typeof a.outcome === "string" &&
            (a.assessmentId == null || (typeof a.assessmentId === "string" && isIdentity(a.assessmentId))),
        )
      : Promise.resolve(null),
  ]);
  return { history, report, attempt };
}
