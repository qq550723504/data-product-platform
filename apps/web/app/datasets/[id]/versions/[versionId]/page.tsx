import {businessText,displayText} from '../../../../../lib/business-language.mjs';
import Link from "next/link";
import { assessmentHref, assertReportScope, isIdentity } from "@/lib/quality-attention";
import {
  BackLink,
  Badge,
  DefinitionList,
  EmptyState,
  LoadError,
  PageHeader,
  SetupRequired,
  formatBytes,
  formatDate,
  shortId,
} from "@/components/ui";
import { DatasetVersionInvalidateForm } from "@/components/dataset-version-invalidate-form";
import { QualityCheckForm } from "@/components/quality-check-form";
import { ComplianceCheckForm } from "@/components/compliance-check-form";
import { CertificationForm, CertificationSupersedeForm } from "@/components/certification-form";
import { batchEnabled,certificationReferences,core,trialRawOnly,workspace } from "@/lib/batches";
import { DirectDataDeliveryForm } from "@/components/direct-data-delivery-form";
import { configuredWorkspaceId, platform, PlatformError, type CertificationBlocker, type DatasetCertification, type CertificationProfile } from "@/lib/platform";
import { isQualityAttemptId } from "@/lib/quality-command";
import { readSection, validPage } from "@/lib/workbench-read";
import { isComplianceAttemptId } from "@/lib/compliance-command";

type DatasetVersionView = "overview" | "quality" | "compliance" | "certification" | "eligibility" | "provenance";

type Query = {
  assessmentId?: string;
  view?: string;
  profileId?: string;
  consumer?: string;
  purpose?: string;
  action?: string;
  delivery?: string;
  scopeType?: string;
  scopeRef?: string;
  findingsOffset?: string;
  assessmentOffset?: string;
  certificationOffset?: string;
  certificationAsOf?: string;
  certificationRevision?: string;
  qualityAttemptId?: string;
  complianceAttemptId?: string;
  compliancePolicyRef?: string;
  deliveryAttemptKey?: string;
  deliveryRetryOf?: string;
};

function firstValue(values?: string[]): string {
  return values?.[0] ?? "";
}

function blockerList(blockers: CertificationBlocker[]) {
  if (blockers.length === 0) return <span>—</span>;
  return (
    <ul style={{ margin: 0, paddingLeft: 18 }}>
      {blockers.map((blocker) => (
        <li key={`${blocker.code}:${blocker.detail}`}>
          <strong>{businessText(blocker.code)}</strong> — {displayText(blocker.detail)}
        </li>
      ))}
    </ul>
  );
}

function currentDisposition(certification: DatasetCertification): string {
  if (certification.dispositions.length > 0) {
    return certification.dispositions.map((item) => item.disposition).join(", ");
  }
  return certification.decision === "CERTIFIED" ? "CURRENT" : certification.decision;
}

function applicability(mode?: string, values?: string[]): string {
  if (!mode) return "—";
  if (mode === "ANY") return "ANY";
  return values?.length ? values.join(", ") : "EXPLICIT (empty)";
}

function scopes(values?: Array<{ type: string; ref: string }>): string {
  if (!values?.length) return "—";
  return values.map((item) => `${item.type}:${item.ref}`).join(", ");
}

function displayObserved(value: unknown): string {
  if (value === null || value === undefined) return "—";
  if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return String(value);
  try {
    return JSON.stringify(value);
  } catch {
    return "—";
  }
}

export default async function DatasetVersionDetailPage({
  params,
  searchParams,
}: {
  params: Promise<{ id: string; versionId: string }>;
  searchParams: Promise<Query>;
}) {
  if (!configuredWorkspaceId()) {
    return <><PageHeader title="成果版本 详情" /><SetupRequired /></>;
  }
  const { id, versionId } = await params;
  const query = await searchParams;
  const findingsOffset = Math.max(0, Number.parseInt(query.findingsOffset ?? "0", 10) || 0);
  const assessmentOffset = Math.max(0, Number.parseInt(query.assessmentOffset ?? "0", 10) || 0);
  const certificationOffset = Math.max(0, Number.parseInt(query.certificationOffset ?? "0", 10) || 0);
  const view: DatasetVersionView = ["quality", "compliance", "certification", "eligibility", "provenance"].includes(query.view ?? "")
    ? query.view as DatasetVersionView
    : "overview";
  const findingsLimit = 50;
  const assessmentLimit = 25;
  const certificationLimit = 25;
  const viewHref = (target: DatasetVersionView) => {
    const next = new URLSearchParams();
    for (const [queryKey, value] of Object.entries(query)) {
      if (value && !["findingsOffset", "assessmentOffset", "certificationOffset", "certificationAsOf", "certificationRevision"].includes(queryKey)) {
        next.set(queryKey, value);
      }
    }
    if (target === "overview") next.delete("view");
    else next.set("view", target);
    const suffix = next.toString();
    return `/datasets/${id}/versions/${versionId}${suffix ? `?${suffix}` : ""}`;
  };
  const pageHref = (key: "findingsOffset" | "assessmentOffset" | "certificationOffset", offset: number) => {
    const next = new URLSearchParams();
    for (const [queryKey, value] of Object.entries(query)) {
      if (value) next.set(queryKey, value);
    }
    if (offset > 0) next.set(key, String(offset));
    else next.delete(key);
    const suffix = next.toString();
    return `/datasets/${id}/versions/${versionId}${suffix ? `?${suffix}` : ""}`;
  };

  try {
    const readQualityList = (limit: number, offset: number) => readSection(
      () => platform.qualityAssessments(versionId, limit, offset),
      page => page.datasetVersionId.toLowerCase() === versionId.toLowerCase()
        && validPage(page, limit, offset, assessment => isIdentity(assessment.id)
          && assessment.workspaceId.toLowerCase() === configuredWorkspaceId()!.toLowerCase()
          && assessment.datasetVersionId.toLowerCase() === versionId.toLowerCase()),
    );
    const [dataset, version, quality, latestQuality, history] = await Promise.all([
      platform.dataset(id),
      platform.datasetVersion(versionId),
      readQualityList(assessmentLimit, assessmentOffset),
      readQualityList(1, 0),
      platform.certificationHistory(
        versionId,
        certificationLimit,
        certificationOffset,
        query.certificationAsOf ?? "",
        query.certificationRevision === undefined ? undefined : Math.max(0, Number.parseInt(query.certificationRevision, 10) || 0),
        query.profileId ?? "",
      ),
    ]);

    if (version.datasetId !== dataset.id) {
      throw new Error("DatasetVersion 不属于当前 Dataset");
    }

    const availableProfiles = batchEnabled() && !trialRawOnly() ? (await core<{items:CertificationProfile[]}>(`/workspaces/${workspace()}/certification-profiles`,"GET",undefined,undefined,true)).items : [];
    const profileMap = new Map([...availableProfiles,...history.profiles].map((profile) => [profile.id, profile]));
    const profiles = [...profileMap.values()];
    const selectedProfile = profileMap.get(query.profileId ?? "") ?? profiles[0];
    const profileScope = selectedProfile?.rights.scopes.values?.[0];
    const requested = {
      profileId: selectedProfile?.id ?? "",
      consumer: query.consumer ?? firstValue(selectedProfile?.consumers.values),
      purpose: query.purpose ?? firstValue(selectedProfile?.purpose.values),
      action: query.action ?? firstValue(selectedProfile?.actions.values),
      delivery: query.delivery ?? firstValue(selectedProfile?.delivery.values),
      scopeType: query.scopeType ?? profileScope?.type ?? "ALL_RESOURCE",
      scopeRef: query.scopeRef ?? profileScope?.ref ?? "",
    };
    const certificationPageHref = (offset: number) => {
      const next = new URLSearchParams();
      for (const [queryKey, value] of Object.entries(query)) {
        if (value) next.set(queryKey, value);
      }
      for (const [key, value] of Object.entries(requested)) {
        if (value) next.set(key, value);
      }
      if (offset > 0) next.set("certificationOffset", String(offset));
      else next.delete("certificationOffset");
      next.set("certificationAsOf", history.asOf);
      next.set("certificationRevision", String(history.page.anchorRevision));
      const suffix = next.toString();
      return `/datasets/${id}/versions/${versionId}${suffix ? `?${suffix}` : ""}`;
    };
    const canCheck = requested.profileId.trim() !== ""
      && requested.consumer.trim() !== ""
      && requested.purpose.trim() !== ""
      && requested.action.trim() !== ""
      && requested.delivery.trim() !== ""
      && requested.scopeType.trim() !== ""
      && (requested.scopeType.trim().toUpperCase() === "ALL_RESOURCE" || requested.scopeRef.trim() !== "");
    // A delivery ambiguity must not prevent viewing/correcting immutable
    // certification history or reading quality/compliance results.
    const eligibility = canCheck && view === "eligibility" ? await platform.deliveryEligibility(versionId, requested) : null;
    const qualityAttemptId = isQualityAttemptId(query.qualityAttemptId) ? query.qualityAttemptId : undefined;
    const qualityAttempt = qualityAttemptId ? await readSection(
      () => platform.qualityAttempt(qualityAttemptId),
      attempt => attempt.id.toLowerCase() === qualityAttemptId.toLowerCase()
        && attempt.workspaceId.toLowerCase() === configuredWorkspaceId()!.toLowerCase()
        && attempt.datasetVersionId.toLowerCase() === version.id.toLowerCase(),
    ) : null;
    const attemptUnknown = Boolean(qualityAttemptId) && qualityAttempt === null;
    let complianceResult = null;
    const complianceAttemptId = isComplianceAttemptId(query.complianceAttemptId) ? query.complianceAttemptId : undefined;
    const compliancePolicyRef = typeof query.compliancePolicyRef === "string" && query.compliancePolicyRef.trim().length <= 1024
      ? query.compliancePolicyRef.trim()
      : undefined;
    if (complianceAttemptId) {
      try {
        complianceResult = await platform.complianceAttempt(complianceAttemptId);
        if (complianceResult.datasetVersionId.toLowerCase() !== version.id.toLowerCase()) {
          throw new Error("Compliance attempt 不属于当前 DatasetVersion。");
        }
      } catch (error) {
        if (!(error instanceof PlatformError && error.status === 404)) throw error;
      }
    }
    const latestSummary = latestQuality;
    const summaryUnknown = latestSummary === null;
    const latestAssessment = latestSummary?.items[0];
    const selectedAssessmentId = view === "quality" ? query.assessmentId : undefined;
    if (selectedAssessmentId && !isIdentity(selectedAssessmentId)) throw new Error("评测引用无效");
    // Report selection is reading only: latestAssessment remains the source for
    // current summaries and certification defaults below.
    const reportId = view === "quality" ? selectedAssessmentId ?? latestAssessment?.id
      : view === "provenance" ? latestAssessment?.id : undefined;
    // An explicit historical report has no dependency on the current list or
    // latest report. Failed explicit selection never falls back to latest.
    const displayReport = reportId ? await readSection(
      () => platform.qualityReport(reportId, findingsLimit, findingsOffset),
      report => Boolean(assertReportScope(report, reportId, version.id, configuredWorkspaceId()!)),
    ) : null;
    const reportUnknown = Boolean(reportId) && displayReport === null;
    const evidenceCertification = eligibility?.certification.current
      ?? history.items.find((item) => item.profile.id === selectedProfile?.id);
    const goldExplanation = evidenceCertification?.profile.profileRef === "gold/dataset-v1"
      ? await platform.goldExplanation(versionId)
      : null;

    const datasetActionsEnabled =
      process.env.POC_ENABLE_DATASET_ACTIONS === "true" && Boolean(process.env.POC_DATASET_ACTOR_ID?.trim());
    const qualityActionsEnabled =
      process.env.POC_ENABLE_QUALITY_ACTIONS === "true" && Boolean(process.env.HUMAN_DECISION_API_TOKEN?.trim());
    const complianceActionsEnabled =
      process.env.POC_ENABLE_COMPLIANCE_ACTIONS === "true" && Boolean(process.env.POC_COMPLIANCE_ACTOR_ID?.trim());
    const deliveryActionsEnabled =
      process.env.POC_ENABLE_DELIVERY_ACTIONS === "true"
      && Boolean(process.env.DELIVERY_API_TOKEN?.trim())
      && Boolean(process.env.DELIVERY_API_CONSUMER_REF?.trim())
      && Boolean(process.env.DELIVERY_API_PRINCIPAL_REF?.trim())
      && Boolean(process.env.DELIVERY_WEB_GATEWAY_TOKEN?.trim())
      && process.env.DELIVERY_API_CONSUMER_REF?.trim() === requested.consumer.trim();

    return (
      <>
        <BackLink href={`/datasets/${dataset.id}`}>返回 {businessText(dataset.name)}</BackLink>
        <PageHeader
          eyebrow={`${dataset.code} / v${version.versionNo}`}
          title="成果版本 质量与认证"
          description="历史认证只说明 issued-at 事实；当前可交付性必须重新检查 成果版本、Certification 与 Rights。"
          action={<div className="badge-row"><Badge value={version.status} /><Badge value={summaryUnknown ? "UNKNOWN" : latestAssessment?.gateDecision ?? "NOT_ASSESSED"} /></div>}
        />

        <nav className="detail-tabs" aria-label="成果版本 详情视图">
          {([
            ["overview", "Overview"],
            ["quality", "Quality"],
            ["compliance", "Compliance"],
            ["certification", "Certification"],
            ["eligibility", "Eligibility"],
            ["provenance", "Provenance"],
          ] as Array<[DatasetVersionView, string]>).map(([target, label]) => (
            <Link
              key={target}
              href={viewHref(target)}
              className={`detail-tab ${view === target ? "active" : ""}`}
              aria-current={view === target ? "page" : undefined}
            >
              {displayText(label)}
            </Link>
          ))}
        </nav>

        <section className="detail-card" style={{ marginBottom: 18 }}>
          <h2>版本事实</h2>
          <DefinitionList items={[
            { label: "DatasetVersion", value: <span className="mono">{businessText(version.id)}</span> },
            { label: "状态", value: <Badge value={version.status} /> },
            { label: "行数", value: version.rowCount ?? "—" },
            { label: "大小", value: formatBytes(version.byteSize) },
            { label: "Checksum", value: version.checksum ? <span className="mono">{displayText(version.checksum)}</span> : "—" },
            { label: "生产执行", value: version.generatedByExecutionId ? <Link href={`/production/${version.generatedByExecutionId}`}>{displayText(shortId(version.generatedByExecutionId))}</Link> : "—" },
            { label: "Ready At", value: formatDate(version.readyAt) },
            { label: "Invalidated At", value: formatDate(version.invalidatedAt) },
            { label: "Invalidation Reason", value: version.invalidationReason || "—" },
          ]} />
        </section>

        <DatasetVersionInvalidateForm
          datasetId={dataset.id}
          versionId={version.id}
          status={version.status}
          enabled={datasetActionsEnabled}
        />

        {view === "overview" ? (
          <>
            <section className="status-overview" aria-label="成果版本 当前状态摘要">
              <article className="status-overview-card">
                <span>Version</span>
                <strong><Badge value={version.status} /></strong>
                <small>{displayText(version.rowCount ?? "—")} rows · {displayText(formatBytes(version.byteSize))}</small>
              </article>
              <article className="status-overview-card">
                <span>Quality</span>
                <strong><Badge value={summaryUnknown ? "UNKNOWN" : latestAssessment?.gateDecision ?? "NOT_ASSESSED"} /></strong>
                <small>{displayText(quality === null ? "评测历史 UNKNOWN" : `${quality.page.total} assessments`)}</small>
              </article>
              <article className="status-overview-card">
                <span>Certification</span>
                <strong>
                  <Badge value={
                    eligibility
                      ? (eligibility.certification.allowed ? "CURRENT" : "NOT_CURRENT")
                      : (history.page.total ? "HISTORY_ONLY" : "NOT_CERTIFIED")
                  } />
                </strong>
                <small>{displayText(history.page.total)} immutable facts</small>
              </article>
              <article className="status-overview-card">
                <span>Delivery</span>
                <strong><Badge value={eligibility ? (eligibility.allowed ? "ALLOWED" : "BLOCKED") : "NOT_CHECKED"} /></strong>
                <small>{businessText(selectedProfile?.name ?? "No profile selected")}</small>
                {eligibility ? (
                  <small className="status-context">
                    {displayText(requested.consumer)} · {displayText(requested.purpose)} · {businessText(requested.action)} · {displayText(requested.delivery)} · {displayText(requested.scopeType)}{displayText(requested.scopeRef ? `:${requested.scopeRef}` : "")}
                  </small>
                ) : null}
              </article>
            </section>
            <section className="detail-card">
              <div className="panel-header"><h2>下一步</h2><span className="eyebrow">Drill down</span></div>
              <div className="quick-links">
                <Link href={viewHref("quality")}>查看 Quality 评测 →</Link>
                {displayText(dataset.datasetType === "RAW" ? <Link href={`/datasets/${id}/versions/${versionId}/remediation`}>质量异常分派、处理与复检 →</Link> : null)}
                <Link href={viewHref("compliance")}>运行 / 恢复 合规检查 →</Link>
                <Link href={viewHref("certification")}>查看 Certification 历史 →</Link>
                <Link href={viewHref("eligibility")}>检查当前可交付性 →</Link>
                <Link href={viewHref("provenance")}>查看 Provenance / Gold proof →</Link>
              </div>
            </section>
          </>
        ) : null}

        {view === "quality" ? (
          <>
        <div className="panel-header"><h2>Quality 评测记录</h2><span className="eyebrow">{displayText(quality === null ? "UNKNOWN" : `${quality.page.total} Assessments`)}</span></div>
        {selectedAssessmentId ? <div className="callout" data-testid="selected-quality-assessment"><strong>指定评测：{displayText(selectedAssessmentId)}</strong><p>下方报告绑定此评测；最新质量摘要、认证输入和当前交付判断保持独立。</p></div> : null}
        {attemptUnknown ? <section className="callout" data-testid="quality-attempt-unavailable"><h3>Quality 评测记录 无法确认</h3><p className="mono">{displayText(qualityAttemptId)}</p><p>暂时无法确认此 评测记录，不发起重复评测。</p></section> : <QualityCheckForm
          key={qualityAttemptId ?? "new"}
          datasetId={dataset.id}
          versionId={version.id}
          versionStatus={version.status}
          enabled={qualityActionsEnabled}
          initialAttemptId={qualityAttemptId}
          attempt={qualityAttempt}
        />}
        {summaryUnknown ? <section className="callout" data-testid="quality-summary-unavailable"><h3>最新评测摘要 无法确认</h3></section> : !latestAssessment ? (
          <EmptyState title="尚未评测" description="该 成果版本 还没有 质量评测。" />
        ) : (
            <section className="detail-card" style={{ marginBottom: 18 }}>
              <div className="panel-header"><h3>最新评测</h3><Badge value={latestAssessment.gateDecision} /></div>
              <DefinitionList items={[
                { label: "RuleSet", value: <><span>{displayText(latestAssessment.ruleSetRef)}</span><br /><span className="mono">{displayText(latestAssessment.ruleSetVersion)} · {displayText(shortId(latestAssessment.ruleSetContentSha256))}</span></> },
                { label: "Evaluator", value: `${latestAssessment.evaluatorName} / ${latestAssessment.evaluatorVersion}` },
                { label: "Assessment", value: <span className="mono">{businessText(latestAssessment.id)}</span> },
                { label: "评测时间", value: formatDate(latestAssessment.createdAt) },
              ]} />
              <div className="table-card" style={{ marginTop: 14 }}>
                <table className="data-table">
                  <thead><tr><th>维度</th><th>状态</th><th>规则</th><th>已评测</th><th>失败</th></tr></thead>
                  <tbody>
                    {Object.values(latestAssessment.dimensionSummary ?? {}).map((dimension) => (
                      <tr key={dimension.dimension}>
                        <td>{displayText(dimension.dimension)}</td>
                        <td><Badge value={dimension.status} /></td>
                        <td>{displayText(dimension.ruleCount)}</td>
                        <td>{displayText(dimension.evaluatedCount)}</td>
                        <td>{displayText(dimension.failedCount)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
        )}
            {reportUnknown ? <section className="callout" data-testid="quality-evidence-unavailable"><h3>Quality 报告 无法确认</h3><p className="mono">{displayText(reportId)}</p><p>暂时无法确认此评测的证据，请稍后重试。未使用其他评测替代。</p></section> : null}
            {displayReport ? (
              <section className="detail-card" style={{ marginBottom: 18 }} data-testid="quality-report">
                <div className="panel-header">
                  <h3>Quality Report</h3>
                  <span className="eyebrow">{displayText(displayReport.findings.page.total)} Findings</span>
                </div>
                <p>报告评测 <span className="mono">{businessText(displayReport.id)}</span> · {displayText(displayReport.ruleSetRef)} · <Badge value={displayReport.gateDecision} /></p>
                {displayReport.findings.items.length === 0 ? (
                  <EmptyState title="无规则明细" description="所选 质量评测 没有 finding。" />
                ) : (
                  <div className="table-card">
                    <table className="data-table">
                      <thead><tr><th>Rule</th><th>维度</th><th>状态</th><th>严重度</th><th>Affected</th><th>原因 / 样例</th></tr></thead>
                      <tbody>
                        {displayReport.findings.items.map((finding) => (
                          <tr key={finding.id}>
                            <td className="mono">{businessText(finding.ruleId)}</td>
                            <td>{displayText(finding.dimension)}</td>
                            <td><Badge value={finding.status} /></td>
                            <td>{businessText(finding.severity)}</td>
                            <td>{displayText(displayObserved(finding.affectedCount))}</td>
                            <td>
                              {displayText(finding.reason || "—")}
                              {finding.sample !== undefined ? <><br /><span className="mono">{displayText(displayObserved(finding.sample))}</span></> : null}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
                {displayReport.findings.page.total > findingsLimit ? (
                  <div className="panel-header" style={{ marginTop: 14 }}>
                    <span className="eyebrow">
                      {displayText(displayReport.findings.page.offset + 1)}–{displayText(Math.min(displayReport.findings.page.offset + displayReport.findings.items.length, displayReport.findings.page.total))}
                      {businessText(" / ")}{displayText(displayReport.findings.page.total)}
                    </span>
                    <div className="badge-row">
                      {displayText(displayReport.findings.page.offset > 0 ? (
                        <Link href={pageHref("findingsOffset", Math.max(0, displayReport.findings.page.offset - findingsLimit))}>上一页</Link>
                      ) : null)}
                      {displayText(displayReport.findings.page.offset + displayReport.findings.items.length < displayReport.findings.page.total ? (
                        <Link href={pageHref("findingsOffset", displayReport.findings.page.offset + findingsLimit)}>下一页</Link>
                      ) : null)}
                    </div>
                  </div>
                ) : null}
                <div className="panel-header" style={{ marginTop: 18 }}>
                  <h3>Quality 证据记录 / Audit</h3>
                  <span className="eyebrow">{displayText(displayReport.evidence.length)} 证据记录 · {displayText(displayReport.auditEvents.length)} Audit</span>
                </div>
                <div className="table-card">
                  <table className="data-table">
                    <thead><tr><th>类型</th><th>引用</th><th>来源 / 动作</th><th>Hash / Trace</th><th>时间</th></tr></thead>
                    <tbody>
                      {displayReport.evidence.map((item) => (
                        <tr key={`evidence:${item.id}`}>
                          <td>证据记录 · {displayText(item.evidenceType)}</td>
                          <td className="mono">{displayText(shortId(item.id))}</td>
                          <td>{displayText(item.sourceType)}{displayText(item.sourceId ? ` / ${shortId(item.sourceId)}` : "")}</td>
                          <td className="mono">{displayText(item.hashValue ? shortId(item.hashValue) : "—")}</td>
                          <td>{displayText(formatDate(item.createdAt))}</td>
                        </tr>
                      ))}
                      {displayReport.auditEvents.map((item) => (
                        <tr key={`audit:${item.id}`}>
                          <td>Audit · {displayText(item.actorType)}</td>
                          <td className="mono">{displayText(shortId(item.id))}</td>
                          <td>{businessText(item.action)}</td>
                          <td className="mono">{displayText(item.traceId || "—")}</td>
                          <td>{displayText(formatDate(item.occurredAt))}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            ) : null}
            {quality === null ? <section className="callout" data-testid="quality-history-unavailable"><h3>Quality 评测历史 无法确认</h3></section> : <>
            <div className="table-card" style={{ marginBottom: 24 }}>
              <table className="data-table">
                <thead><tr><th>时间</th><th>检查项</th><th>RuleSet</th><th>评测记录</th></tr></thead>
                <tbody>
                  {quality.items.map((assessment) => (
                    <tr key={assessment.id}>
                      <td>{displayText(formatDate(assessment.createdAt))}</td>
                      <td><Badge value={assessment.gateDecision} /></td>
                      <td>{displayText(assessment.ruleSetRef)} <span className="mono">{displayText(assessment.ruleSetVersion)}</span></td>
                      <td className="mono"><Link href={assessmentHref(dataset.id, version.id, assessment.id)}>{displayText(shortId(assessment.id))}</Link></td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {quality.page.total > assessmentLimit ? (
              <div className="panel-header" style={{ marginBottom: 24 }}>
                <span className="eyebrow">
                  {displayText(quality.page.offset + 1)}–{displayText(Math.min(quality.page.offset + quality.items.length, quality.page.total))}
                  {businessText(" / ")}{displayText(quality.page.total)}
                </span>
                <div className="badge-row">
                  {displayText(quality.page.offset > 0 ? (
                    <Link href={pageHref("assessmentOffset", Math.max(0, quality.page.offset - assessmentLimit))}>上一页</Link>
                  ) : null)}
                  {displayText(quality.page.offset + quality.items.length < quality.page.total ? (
                    <Link href={pageHref("assessmentOffset", quality.page.offset + assessmentLimit)}>下一页</Link>
                  ) : null)}
                </div>
              </div>
            ) : null}
          </>
        }

          </>
        ) : null}

        {view === "compliance" ? (
          <>
            <div className="panel-header">
              <h2>Compliance 评测记录</h2>
              <span className="eyebrow">{businessText(complianceResult ? "Recovered result" : "No selected attempt")}</span>
            </div>
            <ComplianceCheckForm
              key={complianceAttemptId ?? "new"}
              datasetId={dataset.id}
              versionId={version.id}
              versionStatus={version.status}
              enabled={complianceActionsEnabled}
              initialAttemptId={complianceAttemptId}
              initialPolicyRef={compliancePolicyRef}
              result={complianceResult}
            />
            {complianceResult ? (
              <>
                <section className="detail-card" style={{ marginBottom: 18 }}>
                  <div className="panel-header">
                    <h3>Compliance 结果记录</h3>
                    <Badge value={complianceResult.gateDecision} />
                  </div>
                  <DefinitionList items={[
                    { label: "Result", value: <span className="mono">{businessText(complianceResult.id)}</span> },
                    { label: "Attempt", value: <span className="mono">{displayText(complianceResult.assessmentAttemptId)}</span> },
                    { label: "Policy", value: <><span>{displayText(complianceResult.policyRef)}</span><br /><span className="mono">{displayText(complianceResult.policyVersion)}</span></> },
                    { label: "评测时间", value: formatDate(complianceResult.createdAt) },
                  ]} />
                </section>
                <div className="table-card" style={{ marginBottom: 24 }}>
                  <table className="data-table">
                    <thead><tr><th>Field</th><th>Category</th><th>Action</th><th>Status</th><th>Message</th></tr></thead>
                    <tbody>
                      {complianceResult.findings.length === 0 ? (
                        <tr><td colSpan={5}>无 Compliance finding。</td></tr>
                      ) : complianceResult.findings.map((finding) => (
                        <tr key={finding.id}>
                          <td>{displayText(finding.fieldName)}</td>
                          <td>{displayText(finding.category || "—")}</td>
                          <td>{businessText(finding.action)}</td>
                          <td><Badge value={finding.status} /></td>
                          <td>{businessText(finding.message || "—")}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </>
            ) : (
              <EmptyState title="尚未选择 Compliance 评测记录" description="运行一次 合规检查，或使用带 合规检查编号 的链接恢复已有结果。" />
            )}
          </>
        ) : null}

        {view === "certification" ? (
          <>
        {displayText(batchEnabled() && !trialRawOnly() && !summaryUnknown ? <CertificationForm key={selectedProfile?.id??"none"} datasetId={id} versionId={versionId} qualityId={latestAssessment?.id??""} profileId={selectedProfile?.id??""} profiles={profiles} canPrepare={Boolean(version.generatedByExecutionId)} references={await certificationReferences(versionId,selectedProfile?.id??"",latestAssessment?.id??"")} /> : null)}
        {displayText(batchEnabled() && !trialRawOnly() ? <CertificationSupersedeForm datasetId={id} versionId={versionId} certifications={history.items.filter(c=>c.profile.id===selectedProfile?.id).map(c=>({id:c.id,decision:c.decision,issuedAt:c.issuedAt,current:c.decision==="CERTIFIED"&&c.dispositions.length===0}))}/> : null)}
        <div className="panel-header"><h2>Certification 历史</h2><span className="eyebrow">{displayText(history.page.total)} Facts</span></div>
        {history.items.length === 0 ? (
          <EmptyState title="认证未通过" description="该 成果版本 尚无 数据集Certification 历史事实。" />
        ) : (
          <div className="table-card" style={{ marginBottom: 24 }}>
            <table className="data-table">
              <thead><tr><th>时间</th><th>结果</th><th>Profile</th><th>历史状态</th><th>依据</th><th>原因 / 阻断原因</th></tr></thead>
              <tbody>
                {history.items.map((certification) => (
                  <tr key={certification.id}>
                    <td>{displayText(formatDate(certification.issuedAt))}</td>
                    <td><Badge value={certification.decision} /></td>
                    <td>
                      <strong>{businessText(certification.profile.name)}</strong>
                      <span className="mono">{displayText(certification.profile.version)} · {displayText(shortId(certification.profile.contentSha256))}</span>
                    </td>
                    <td><Badge value={currentDisposition(certification)} /></td>
                    <td>
                      <span className="mono">Q {displayText(shortId(certification.qualityAssessmentId))}</span><br />
                      <span className="mono">R {displayText(shortId(certification.effectiveRightsSnapshotId))}</span><br />
                      <span className="mono">C {displayText(shortId(certification.contractVersionId))}</span>
                    </td>
                    <td>{displayText(certification.blockers.length ? blockerList(certification.blockers) : certification.reason)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {history.page.total > certificationLimit ? (
          <div className="panel-header" style={{ marginBottom: 24 }}>
            <span className="eyebrow">
              {displayText(history.page.offset + 1)}–{displayText(Math.min(history.page.offset + history.items.length, history.page.total))}
              {businessText(" / ")}{displayText(history.page.total)}
            </span>
            <div className="badge-row">
              {displayText(history.page.offset > 0 ? (
                <Link href={certificationPageHref(Math.max(0, history.page.offset - certificationLimit))}>上一页</Link>
              ) : null)}
              {displayText(history.page.offset + history.items.length < history.page.total ? (
                <Link href={certificationPageHref(history.page.offset + history.items.length)}>下一页</Link>
              ) : null)}
            </div>
          </div>
        ) : null}

          </>
        ) : null}

        {view === "provenance" ? (
          <>
        {evidenceCertification?.profile.profileRef === "gold/dataset-v1" ? (
          <section className="detail-card" style={{ marginBottom: 24 }} data-testid="gold-production-proof">
            <div className="panel-header">
              <h2>Gold Production Proof</h2>
              <span className="eyebrow">Frozen annotation facts</span>
            </div>
            <div className="callout">
              <strong>这是 Gold 成果版本 的冻结生产证明</strong>
              <p>这些引用来自 immutable 数据集Certification，不读取 Label Studio 当前状态，也不会被后续 annotation/provider mutation 改写。</p>
            </div>
            <DefinitionList items={[
              { label: "GoldProductionBinding", value: evidenceCertification.goldProductionBindingId ? <span className="mono">{displayText(evidenceCertification.goldProductionBindingId)}</span> : "—" },
              { label: "Binding root", value: evidenceCertification.goldProductionBindingRootHash ? <span className="mono">{displayText(evidenceCertification.goldProductionBindingRootHash)}</span> : "—" },
              { label: "AnnotationSnapshot", value: evidenceCertification.annotationSnapshotId ? <span className="mono">{displayText(evidenceCertification.annotationSnapshotId)}</span> : "—" },
              { label: "Snapshot root", value: evidenceCertification.annotationSnapshotRootHash ? <span className="mono">{displayText(evidenceCertification.annotationSnapshotRootHash)}</span> : "—" },
              { label: "Schema SHA-256", value: evidenceCertification.annotationSchemaSha256 ? <span className="mono">{displayText(evidenceCertification.annotationSchemaSha256)}</span> : "—" },
              { label: "Taxonomy SHA-256", value: evidenceCertification.annotationTaxonomySha256 ? <span className="mono">{displayText(evidenceCertification.annotationTaxonomySha256)}</span> : "—" },
              { label: "Formal Gold Quality", value: <><span className="mono">{displayText(evidenceCertification.qualityAssessmentId)}</span>{displayText(latestAssessment?.ruleSetRef === "gold/quality/annotation-v1" ? <> · <Badge value={latestAssessment.gateDecision} /></> : null)}</> },
              { label: "Gold Certification", value: <span className="mono">{businessText(evidenceCertification.id)}</span> },
            ]} />
          </section>
        ) : null}

        {goldExplanation ? (
          <section className="detail-card" style={{ marginBottom: 24 }} data-testid="gold-production-chain">
            <div className="panel-header">
              <h2>成果生产依据</h2>
              <span className="eyebrow">冻结业务记录</span>
            </div>
            <DefinitionList items={[
              { label: "Input DatasetVersion", value: <span className="mono">{displayText(goldExplanation.inputDatasetVersionId)}</span> },
              { label: "Input Certification", value: <span className="mono">{displayText(goldExplanation.inputCertificationId)}</span> },
              { label: "Annotation Contribution", value: <span className="mono">{displayText(goldExplanation.annotationContributionResourceId)}</span> },
              { label: "Annotation Campaign", value: <><span className="mono">{businessText(goldExplanation.campaign.id)}</span> · <Badge value={goldExplanation.campaign.status} /></> },
              { label: "Purpose / Action", value: `${goldExplanation.campaign.purpose} / ${goldExplanation.campaign.action}` },
              { label: "Schema", value: <><span>{displayText(goldExplanation.campaign.schemaRef)} / {displayText(goldExplanation.campaign.schemaVersion)}</span><br /><span className="mono">{displayText(goldExplanation.campaign.schemaSha256)}</span></> },
              { label: "Taxonomy", value: <><span>{displayText(goldExplanation.campaign.taxonomyRef)} / {displayText(goldExplanation.campaign.taxonomyVersion)}</span><br /><span className="mono">{displayText(goldExplanation.campaign.taxonomySha256)}</span></> },
              { label: "Tasks / Results / Reviews / Outputs", value: `${goldExplanation.campaign.taskCount} / ${goldExplanation.campaign.resultCount} / ${goldExplanation.campaign.reviewDecisionCount} / ${goldExplanation.campaign.selectedOutputCount}` },
              { label: "FINALIZED Snapshot", value: <><span className="mono">{businessText(goldExplanation.snapshot.id)}</span> · <Badge value={goldExplanation.snapshot.status} /><br /><span className="mono">{displayText(goldExplanation.snapshot.rootHash)}</span></> },
              { label: "Gold Build Execution", value: <Link href={`/production/${goldExplanation.executionId}`}>{displayText(goldExplanation.executionId)}</Link> },
            ]} />

            <div className="panel-header" style={{ marginTop: 18 }}>
              <h3>标注与人工审核</h3>
              <span className="eyebrow">{displayText(goldExplanation.reviews.length)} frozen decisions</span>
            </div>
            <div className="table-card">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Source</th>
                    <th>Annotation</th>
                    <th>Review</th>
                    <th>Selected 结果记录</th>
                    <th>Provider provenance</th>
                  </tr>
                </thead>
                <tbody>
                  {goldExplanation.reviews.map((review) => (
                    <tr key={review.decisionId}>
                      <td>
                        <strong>{displayText(review.sourceItemRef)}</strong><br />
                        <span className="mono">{displayText(shortId(review.sourceContentSha256))}</span>
                      </td>
                      <td>
                        {displayText(review.annotationAuthorRef || "—")}<br />
                        <span className="mono">{displayText(review.reviewedResultId ? shortId(review.reviewedResultId) : "—")}</span>
                      </td>
                      <td>
                        <Badge value={review.outcome} /><br />
                        <span>{displayText(review.reviewerRef)}</span><br />
                        <span>{displayText(review.reason)}</span><br />
                        <span>{displayText(formatDate(review.decisionCreatedAt))}</span>
                      </td>
                      <td>
                        <span className="mono">{displayText(review.selectedResultId ? shortId(review.selectedResultId) : "—")}</span><br />
                        <span className="mono">{displayText(review.selectedResultSha256 ? shortId(review.selectedResultSha256) : "—")}</span>
                      </td>
                      <td>
                        <span className="mono">{displayText(review.providerBindingRef || "—")}</span><br />
                        <span className="mono">task {displayText(review.externalTaskId || "—")}</span><br />
                        <span className="mono">annotation {displayText(review.externalAnnotationId || "—")}</span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </section>
        ) : null}

        {goldExplanation ? (
          <section className="detail-card" style={{ marginBottom: 24 }} data-testid="gold-trace-summary">
            <div className="panel-header">
              <h2>Gold Cost / 证据记录 / Audit Trace</h2>
              <span className="eyebrow">
                {displayText(goldExplanation.trace.costs.length)} Cost · {displayText(goldExplanation.trace.evidence.length)} 证据记录 · {displayText(goldExplanation.trace.audit.length)} Audit
              </span>
            </div>
            <div className="callout">
              <strong>按生产阶段聚合的可追溯引用</strong>
              <p>这里只展示有界的 subject / hash / actor / trace 引用，不加载底层大 metadata；用于解释 Annotation、Review、Build、Quality、Certification 与 Delivery 的成本和证据链。</p>
            </div>

            <div className="panel-header" style={{ marginTop: 18 }}>
              <h3>Cost</h3>
              <span className="eyebrow">{displayText(goldExplanation.trace.costs.length)} events</span>
            </div>
            {goldExplanation.trace.costs.length ? (
              <div className="table-card">
                <table className="data-table">
                  <thead><tr><th>Phase</th><th>Cost</th><th>Subject</th><th>Quantity</th><th>Amount</th><th>Time</th></tr></thead>
                  <tbody>
                    {goldExplanation.trace.costs.map((item) => (
                      <tr key={item.id}>
                        <td>{businessText(item.phase)}</td>
                        <td>{displayText(item.costType)}<br /><span className="mono">{displayText(item.pricingMode)}</span></td>
                        <td>{displayText(item.subjectType)}<br /><span className="mono">{displayText(shortId(item.subjectId))}</span></td>
                        <td>{displayText(item.quantity)} {displayText(item.unit)}</td>
                        <td>{displayText(item.amount === undefined ? "—" : String(item.amount) + (item.currency ? " " + item.currency : ""))}</td>
                        <td>{displayText(formatDate(item.occurredAt))}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : <EmptyState title="暂无 Cost 引用" description="当前 Gold 链没有可展示的成本事件。" />}

            <div className="panel-header" style={{ marginTop: 18 }}>
              <h3>证据记录</h3>
              <span className="eyebrow">{displayText(goldExplanation.trace.evidence.length)} refs</span>
            </div>
            {goldExplanation.trace.evidence.length ? (
              <div className="table-card">
                <table className="data-table">
                  <thead><tr><th>Phase</th><th>证据记录</th><th>Subject</th><th>Relation</th><th>Hash</th><th>Time</th></tr></thead>
                  <tbody>
                    {goldExplanation.trace.evidence.map((item) => (
                      <tr key={item.phase + ":" + item.id + ":" + item.subjectId}>
                        <td>{businessText(item.phase)}</td>
                        <td>{displayText(item.evidenceType)}<br /><span className="mono">{displayText(shortId(item.id))}</span></td>
                        <td>{displayText(item.subjectType)}<br /><span className="mono">{displayText(shortId(item.subjectId))}</span></td>
                        <td>{displayText(item.relationType)}</td>
                        <td className="mono">{displayText(item.hashValue ? shortId(item.hashValue) : "—")}</td>
                        <td>{displayText(formatDate(item.createdAt))}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : <EmptyState title="暂无 证据记录 引用" description="当前 Gold 链没有可展示的 证据记录 relation。" />}

            <div className="panel-header" style={{ marginTop: 18 }}>
              <h3>Audit</h3>
              <span className="eyebrow">{displayText(goldExplanation.trace.audit.length)} refs</span>
            </div>
            {goldExplanation.trace.audit.length ? (
              <div className="table-card">
                <table className="data-table">
                  <thead><tr><th>Phase</th><th>Action</th><th>Object</th><th>Actor</th><th>Trace</th><th>Time</th></tr></thead>
                  <tbody>
                    {goldExplanation.trace.audit.map((item) => (
                      <tr key={item.id}>
                        <td>{businessText(item.phase)}</td>
                        <td>{businessText(item.action)}</td>
                        <td>{displayText(item.objectType)}<br /><span className="mono">{displayText(shortId(item.objectId))}</span></td>
                        <td>{displayText(item.actorType)}<br /><span className="mono">{displayText(item.actorId ? shortId(item.actorId) : "—")}</span></td>
                        <td className="mono">{displayText(item.traceId || "—")}</td>
                        <td>{displayText(formatDate(item.occurredAt))}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : <EmptyState title="暂无 Audit 引用" description="当前 Gold 链没有可展示的 AuditEvent。" />}
          </section>
        ) : null}


        <section className="detail-card" style={{ marginBottom: 24 }} data-testid="generic-provenance-summary">
          <div className="panel-header"><h2>证据记录 / Traceability</h2><span className="eyebrow">Frozen references</span></div>
          {displayText(reportUnknown || summaryUnknown ? <p data-testid="quality-provenance-unavailable">Quality 证据记录 无法确认；其他已知认证引用保持展示。</p> : null)}
          {!reportUnknown && !summaryUnknown && !displayReport?.evidence.length && !evidenceCertification?.evidenceSnapshotId && !evidenceCertification?.traceabilityEvidenceId ? (
            <EmptyState title="暂无 Provenance 引用" description="当前 成果版本 暂无可展示的质量 证据记录、证据快照 或 Traceability 证据记录。" />
          ) : (
            <DefinitionList items={[
              { label: "Quality evidence", value: reportUnknown || summaryUnknown ? "UNKNOWN" : displayReport?.evidence.length ? displayReport.evidence.map((item) => <div key={item.id}><span className="mono">{displayText(shortId(item.id))}</span> · {displayText(item.evidenceType)}</div>) : "—" },
              { label: "Certification EvidenceSnapshot", value: evidenceCertification?.evidenceSnapshotId ? <span className="mono">{displayText(evidenceCertification.evidenceSnapshotId)}</span> : "—" },
              { label: "Traceability Evidence", value: evidenceCertification?.traceabilityEvidenceId ? <span className="mono">{displayText(evidenceCertification.traceabilityEvidenceId)}</span> : "—" },
              { label: "Certification", value: evidenceCertification ? <span className="mono">{businessText(evidenceCertification.id)}</span> : "—" },
            ]} />
          )}
        </section>
          </>
        ) : null}

        {view === "eligibility" ? (
          <>
        {selectedProfile ? (
          <section className="detail-card" style={{ marginBottom: 24 }}>
            <div className="panel-header"><h2>Rights summary</h2><span className="eyebrow">Frozen certification context</span></div>
            <DefinitionList items={[
              { label: "Rights required", value: <Badge value={selectedProfile.rights.required ? "REQUIRED" : "OPTIONAL"} /> },
              { label: "Purpose coverage", value: applicability(selectedProfile.rights.purpose.mode, selectedProfile.rights.purpose.values) },
              { label: "Action coverage", value: applicability(selectedProfile.rights.actions.mode, selectedProfile.rights.actions.values) },
              { label: "Consumer coverage", value: applicability(selectedProfile.rights.consumers.mode, selectedProfile.rights.consumers.values) },
              { label: "Scope coverage", value: selectedProfile.rights.scopes.mode === "ANY" ? "ANY" : scopes(selectedProfile.rights.scopes.values) },
              { label: "EffectiveRights", value: evidenceCertification?.effectiveRightsSnapshotId ? <span className="mono">{displayText(evidenceCertification.effectiveRightsSnapshotId)}</span> : "—" },
              { label: "EffectiveRights hash", value: evidenceCertification?.effectiveRightsSnapshotHash ? <span className="mono">{displayText(evidenceCertification.effectiveRightsSnapshotHash)}</span> : "—" },
              { label: "RightsSnapshot (optional)", value: evidenceCertification?.rightsSnapshotId ? <span className="mono">{displayText(evidenceCertification.rightsSnapshotId)}</span> : "—" },
            ]} />
            {displayText(eligibility?.entitlement.checks.length ? (
              <div className="callout" style={{ marginTop: 14 }}>
                <strong>当前 Rights 重新验证</strong>
                <p>下方 Current Entitlement 以请求的 consumer / purpose / action 对每个实际 source input 重新校验，不把历史认证当成永久授权。</p>
              </div>
            ) : null)}
          </section>
        ) : null}

        <div className="panel-header"><h2>Current Delivery Eligibility</h2><span className="eyebrow">Preflight only</span></div>
        {profiles.length === 0 ? (
          <EmptyState title="无法执行当前资格检查" description="需要至少一个 CertificationProfile 历史事实。" />
        ) : (
          <>
            <form method="get" className="detail-card" style={{ marginBottom: 18 }}>
              <input type="hidden" name="view" value="eligibility" />
              <div className="definition-list">
                <div><dt>Profile</dt><dd>
                  <select name="profileId" defaultValue={requested.profileId}>
                    {profiles.map((profile) => <option key={profile.id} value={profile.id}>{businessText(profile.name)} / {displayText(profile.version)}</option>)}
                  </select>
                </dd></div>
                <div><dt>Consumer</dt><dd><input name="consumer" defaultValue={requested.consumer} required /></dd></div>
                <div><dt>Purpose</dt><dd><input name="purpose" defaultValue={requested.purpose} required /></dd></div>
                <div><dt>Action</dt><dd><input name="action" defaultValue={requested.action} required /></dd></div>
                <div><dt>Delivery</dt><dd><input name="delivery" defaultValue={requested.delivery} required /></dd></div>
                <div><dt>Scope type</dt><dd><input name="scopeType" defaultValue={requested.scopeType} required /></dd></div>
                <div><dt>Scope ref</dt><dd><input name="scopeRef" defaultValue={requested.scopeRef} placeholder="全部资源 时可留空" /></dd></div>
              </div>
              <button type="submit" style={{ marginTop: 14 }}>检查当前可交付性</button>
            </form>

            {!eligibility ? (
              <div className="callout callout-warn"><strong>需要完整 delivery context</strong><p>填写 consumer、purpose、action、delivery 与 normalized scope 后执行当前资格预检；全部资源 的 scope ref 会按每个 source 数据资源 解析。</p></div>
            ) : (
              <section className="detail-card">
                <div className="panel-header">
                  <h3>预检结果</h3>
                  <Badge value={eligibility.allowed ? "ALLOWED" : "BLOCKED"} />
                </div>
                <div className="table-card">
                  <table className="data-table">
                    <thead><tr><th>检查项</th><th>结果</th><th>阻断原因</th></tr></thead>
                    <tbody>
                      <tr><td>成果版本 usability</td><td><Badge value={eligibility.datasetVersion.allowed ? "ALLOWED" : "BLOCKED"} /></td><td>{displayText(blockerList(eligibility.datasetVersion.blockers))}</td></tr>
                      <tr><td>Current Certification</td><td><Badge value={eligibility.certification.allowed ? "ALLOWED" : "BLOCKED"} /></td><td>{displayText(blockerList(eligibility.certification.blockers))}</td></tr>
                      <tr><td>Current Entitlement</td><td><Badge value={eligibility.entitlement.allowed ? "ALLOWED" : "BLOCKED"} /></td><td>{displayText(blockerList(eligibility.entitlement.blockers))}</td></tr>
                    </tbody>
                  </table>
                </div>
                {eligibility.entitlement.checks.length ? (
                  <div className="table-card" style={{ marginTop: 14 }}>
                    <table className="data-table">
                      <thead><tr><th>Source Resource</th><th>Path</th><th>Decision</th><th>Reason</th></tr></thead>
                      <tbody>
                        {eligibility.entitlement.checks.map((check) => (
                          <tr key={`${check.dataResourceId}:${check.path}`}>
                            <td><Link className="text-link mono" href={`/resources/${check.dataResourceId}`}>{displayText(shortId(check.dataResourceId))}</Link></td>
                            <td>{displayText(check.path)}</td>
                            <td><Badge value={check.decision} /></td>
                            <td>{displayText(check.reason)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : null}
                {!eligibility.allowed ? <div className="callout callout-bad" style={{ marginTop: 14 }}><strong>当前不可交付</strong>{displayText(blockerList(eligibility.blockers))}</div> : null}
                <DirectDataDeliveryForm
                  datasetId={dataset.id}
                  versionId={version.id}
                  enabled={deliveryActionsEnabled}
                  allowed={eligibility.allowed}
                  profileId={requested.profileId}
                  consumer={requested.consumer}
                  purpose={requested.purpose}
                  action={requested.action}
                  delivery={requested.delivery}
                  scopeType={requested.scopeType}
                  scopeRef={requested.scopeRef}
                  initialKey={typeof query.deliveryAttemptKey === "string" && query.deliveryAttemptKey.trim().length <= 255 ? query.deliveryAttemptKey.trim() : undefined}
                  initialRetryOf={typeof query.deliveryRetryOf === "string" && /^[0-9a-f-]{36}$/i.test(query.deliveryRetryOf) ? query.deliveryRetryOf : undefined}
                />
              </section>
            )}
          </>
        )}
          </>
        ) : null}
      </>
    );
  } catch (error) {
    return <><BackLink href={`/datasets/${id}`}>返回数据集</BackLink><PageHeader title="成果版本 详情" /><LoadError error={error} /></>;
  }
}
