import Link from "next/link";
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
import { configuredWorkspaceId, platform, type CertificationBlocker, type DatasetCertification } from "@/lib/platform";

type Query = {
  profileId?: string;
  consumer?: string;
  purpose?: string;
  action?: string;
  delivery?: string;
  scopeType?: string;
  scopeRef?: string;
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
          <strong>{blocker.code}</strong> — {blocker.detail}
        </li>
      ))}
    </ul>
  );
}

function currentDisposition(certification: DatasetCertification): string {
  if (certification.dispositions.length === 0) return "CURRENT_HISTORY";
  return certification.dispositions.map((item) => item.disposition).join(", ");
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
    return <><PageHeader title="DatasetVersion 详情" /><SetupRequired /></>;
  }
  const { id, versionId } = await params;
  const query = await searchParams;

  try {
    const [dataset, version, quality, history] = await Promise.all([
      platform.dataset(id),
      platform.datasetVersion(versionId),
      platform.qualityAssessments(versionId, 25, 0),
      platform.certificationHistory(versionId),
    ]);

    if (version.datasetId !== dataset.id) {
      throw new Error("DatasetVersion 不属于当前 Dataset");
    }

    const profileMap = new Map(history.items.map((item) => [item.profile.id, item.profile]));
    const profiles = Array.from(profileMap.values());
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
    const canCheck = requested.profileId.trim() !== ""
      && requested.consumer.trim() !== ""
      && requested.purpose.trim() !== ""
      && requested.action.trim() !== ""
      && requested.delivery.trim() !== ""
      && requested.scopeType.trim() !== ""
      && (requested.scopeType.trim().toUpperCase() === "ALL_RESOURCE" || requested.scopeRef.trim() !== "");
    const eligibility = canCheck ? await platform.deliveryEligibility(versionId, requested) : null;
    const latestAssessment = quality.items[0];
    const latestReport = latestAssessment ? await platform.qualityReport(latestAssessment.id, 50, 0) : null;
    const evidenceCertification = eligibility?.certification.current
      ?? history.items.find((item) => item.profile.id === selectedProfile?.id);

    return (
      <>
        <BackLink href={`/datasets/${dataset.id}`}>返回 {dataset.name}</BackLink>
        <PageHeader
          eyebrow={`${dataset.code} / v${version.versionNo}`}
          title="DatasetVersion 质量与认证"
          description="历史认证只说明 issued-at 事实；当前可交付性必须重新检查 DatasetVersion、Certification 与 Rights。"
          action={<div className="badge-row"><Badge value={version.status} /><Badge value={latestAssessment?.gateDecision ?? "NOT_ASSESSED"} /></div>}
        />

        <section className="detail-card" style={{ marginBottom: 18 }}>
          <h2>版本事实</h2>
          <DefinitionList items={[
            { label: "DatasetVersion", value: <span className="mono">{version.id}</span> },
            { label: "状态", value: <Badge value={version.status} /> },
            { label: "行数", value: version.rowCount ?? "—" },
            { label: "大小", value: formatBytes(version.byteSize) },
            { label: "Checksum", value: version.checksum ? <span className="mono">{version.checksum}</span> : "—" },
            { label: "生产执行", value: version.generatedByExecutionId ? <Link href={`/production/${version.generatedByExecutionId}`}>{shortId(version.generatedByExecutionId)}</Link> : "—" },
            { label: "Ready At", value: formatDate(version.readyAt) },
          ]} />
        </section>

        <div className="panel-header"><h2>Quality Assessment</h2><span className="eyebrow">{quality.page.total} Assessments</span></div>
        {!latestAssessment ? (
          <EmptyState title="尚未评测" description="该 DatasetVersion 还没有 QualityAssessment。" />
        ) : (
          <>
            <section className="detail-card" style={{ marginBottom: 18 }}>
              <div className="panel-header"><h3>最新评测</h3><Badge value={latestAssessment.gateDecision} /></div>
              <DefinitionList items={[
                { label: "RuleSet", value: <><span>{latestAssessment.ruleSetRef}</span><br /><span className="mono">{latestAssessment.ruleSetVersion} · {shortId(latestAssessment.ruleSetContentSha256)}</span></> },
                { label: "Evaluator", value: `${latestAssessment.evaluatorName} / ${latestAssessment.evaluatorVersion}` },
                { label: "Assessment", value: <span className="mono">{latestAssessment.id}</span> },
                { label: "评测时间", value: formatDate(latestAssessment.createdAt) },
              ]} />
              <div className="table-card" style={{ marginTop: 14 }}>
                <table className="data-table">
                  <thead><tr><th>维度</th><th>状态</th><th>规则</th><th>已评测</th><th>失败</th></tr></thead>
                  <tbody>
                    {Object.values(latestAssessment.dimensionSummary ?? {}).map((dimension) => (
                      <tr key={dimension.dimension}>
                        <td>{dimension.dimension}</td>
                        <td><Badge value={dimension.status} /></td>
                        <td>{dimension.ruleCount}</td>
                        <td>{dimension.evaluatedCount}</td>
                        <td>{dimension.failedCount}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </section>
            {latestReport ? (
              <section className="detail-card" style={{ marginBottom: 18 }}>
                <div className="panel-header">
                  <h3>Quality Report</h3>
                  <span className="eyebrow">{latestReport.findings.page.total} Findings</span>
                </div>
                {latestReport.findings.items.length === 0 ? (
                  <EmptyState title="无规则明细" description="最新 QualityAssessment 没有 finding。" />
                ) : (
                  <div className="table-card">
                    <table className="data-table">
                      <thead><tr><th>Rule</th><th>维度</th><th>状态</th><th>严重度</th><th>Affected</th><th>原因 / 样例</th></tr></thead>
                      <tbody>
                        {latestReport.findings.items.map((finding) => (
                          <tr key={finding.id}>
                            <td className="mono">{finding.ruleId}</td>
                            <td>{finding.dimension}</td>
                            <td><Badge value={finding.status} /></td>
                            <td>{finding.severity}</td>
                            <td>{displayObserved(finding.affectedCount)}</td>
                            <td>
                              {finding.reason || "—"}
                              {finding.sample !== undefined ? <><br /><span className="mono">{displayObserved(finding.sample)}</span></> : null}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
                <div className="panel-header" style={{ marginTop: 18 }}>
                  <h3>Quality Evidence / Audit</h3>
                  <span className="eyebrow">{latestReport.evidence.length} Evidence · {latestReport.auditEvents.length} Audit</span>
                </div>
                <div className="table-card">
                  <table className="data-table">
                    <thead><tr><th>类型</th><th>引用</th><th>来源 / 动作</th><th>Hash / Trace</th><th>时间</th></tr></thead>
                    <tbody>
                      {latestReport.evidence.map((item) => (
                        <tr key={`evidence:${item.id}`}>
                          <td>Evidence · {item.evidenceType}</td>
                          <td className="mono">{shortId(item.id)}</td>
                          <td>{item.sourceType}{item.sourceId ? ` / ${shortId(item.sourceId)}` : ""}</td>
                          <td className="mono">{item.hashValue ? shortId(item.hashValue) : "—"}</td>
                          <td>{formatDate(item.createdAt)}</td>
                        </tr>
                      ))}
                      {latestReport.auditEvents.map((item) => (
                        <tr key={`audit:${item.id}`}>
                          <td>Audit · {item.actorType}</td>
                          <td className="mono">{shortId(item.id)}</td>
                          <td>{item.action}</td>
                          <td className="mono">{item.traceId || "—"}</td>
                          <td>{formatDate(item.occurredAt)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            ) : null}
            <div className="table-card" style={{ marginBottom: 24 }}>
              <table className="data-table">
                <thead><tr><th>时间</th><th>Gate</th><th>RuleSet</th><th>Assessment</th></tr></thead>
                <tbody>
                  {quality.items.map((assessment) => (
                    <tr key={assessment.id}>
                      <td>{formatDate(assessment.createdAt)}</td>
                      <td><Badge value={assessment.gateDecision} /></td>
                      <td>{assessment.ruleSetRef} <span className="mono">{assessment.ruleSetVersion}</span></td>
                      <td className="mono">{shortId(assessment.id)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </>
        )}

        <div className="panel-header"><h2>Certification 历史</h2><span className="eyebrow">{history.items.length} Facts</span></div>
        {history.items.length === 0 ? (
          <EmptyState title="NOT_CERTIFIED" description="该 DatasetVersion 尚无 DatasetCertification 历史事实。" />
        ) : (
          <div className="table-card" style={{ marginBottom: 24 }}>
            <table className="data-table">
              <thead><tr><th>时间</th><th>结果</th><th>Profile</th><th>历史状态</th><th>依据</th><th>原因 / Blockers</th></tr></thead>
              <tbody>
                {history.items.map((certification) => (
                  <tr key={certification.id}>
                    <td>{formatDate(certification.issuedAt)}</td>
                    <td><Badge value={certification.decision} /></td>
                    <td>
                      <strong>{certification.profile.name}</strong>
                      <span className="mono">{certification.profile.version} · {shortId(certification.profile.contentSha256)}</span>
                    </td>
                    <td><Badge value={currentDisposition(certification)} /></td>
                    <td>
                      <span className="mono">Q {shortId(certification.qualityAssessmentId)}</span><br />
                      <span className="mono">R {shortId(certification.effectiveRightsSnapshotId)}</span><br />
                      <span className="mono">C {shortId(certification.contractVersionId)}</span>
                    </td>
                    <td>{certification.blockers.length ? blockerList(certification.blockers) : certification.reason}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {selectedProfile ? (
          <section className="detail-card" style={{ marginBottom: 24 }}>
            <div className="panel-header"><h2>Rights summary</h2><span className="eyebrow">Frozen certification context</span></div>
            <DefinitionList items={[
              { label: "Rights required", value: <Badge value={selectedProfile.rights.required ? "REQUIRED" : "OPTIONAL"} /> },
              { label: "Purpose coverage", value: applicability(selectedProfile.rights.purpose.mode, selectedProfile.rights.purpose.values) },
              { label: "Action coverage", value: applicability(selectedProfile.rights.actions.mode, selectedProfile.rights.actions.values) },
              { label: "Consumer coverage", value: applicability(selectedProfile.rights.consumers.mode, selectedProfile.rights.consumers.values) },
              { label: "Scope coverage", value: selectedProfile.rights.scopes.mode === "ANY" ? "ANY" : scopes(selectedProfile.rights.scopes.values) },
              { label: "EffectiveRights", value: evidenceCertification?.effectiveRightsSnapshotId ? <span className="mono">{evidenceCertification.effectiveRightsSnapshotId}</span> : "—" },
              { label: "EffectiveRights hash", value: evidenceCertification?.effectiveRightsSnapshotHash ? <span className="mono">{evidenceCertification.effectiveRightsSnapshotHash}</span> : "—" },
              { label: "RightsSnapshot (optional)", value: evidenceCertification?.rightsSnapshotId ? <span className="mono">{evidenceCertification.rightsSnapshotId}</span> : "—" },
            ]} />
            {eligibility?.entitlement.checks.length ? (
              <div className="callout" style={{ marginTop: 14 }}>
                <strong>当前 Rights 重新验证</strong>
                <p>下方 Current Entitlement 以请求的 consumer / purpose / action 对每个实际 source input 重新校验，不把历史认证当成永久授权。</p>
              </div>
            ) : null}
          </section>
        ) : null}

        <section className="detail-card" style={{ marginBottom: 24 }}>
          <div className="panel-header"><h2>Evidence</h2><span className="eyebrow">Frozen references</span></div>
          {!latestReport?.evidence.length && !evidenceCertification?.evidenceSnapshotId && !evidenceCertification?.traceabilityEvidenceId ? (
            <EmptyState title="暂无 Evidence 引用" description="当前版本尚未返回质量或认证 Evidence 引用。" />
          ) : (
            <DefinitionList items={[
              { label: "Quality evidence", value: latestReport?.evidence.length ? latestReport.evidence.map((item) => <div key={item.id}><span className="mono">{shortId(item.id)}</span> · {item.evidenceType}</div>) : "—" },
              { label: "Certification EvidenceSnapshot", value: evidenceCertification?.evidenceSnapshotId ? <span className="mono">{evidenceCertification.evidenceSnapshotId}</span> : "—" },
              { label: "Traceability Evidence", value: evidenceCertification?.traceabilityEvidenceId ? <span className="mono">{evidenceCertification.traceabilityEvidenceId}</span> : "—" },
              { label: "Certification", value: evidenceCertification ? <span className="mono">{evidenceCertification.id}</span> : "—" },
            ]} />
          )}
        </section>

        <div className="panel-header"><h2>Current Delivery Eligibility</h2><span className="eyebrow">Preflight only</span></div>
        {profiles.length === 0 ? (
          <EmptyState title="无法执行当前资格检查" description="需要至少一个 CertificationProfile 历史事实。" />
        ) : (
          <>
            <form method="get" className="detail-card" style={{ marginBottom: 18 }}>
              <div className="definition-list">
                <div><dt>Profile</dt><dd>
                  <select name="profileId" defaultValue={requested.profileId}>
                    {profiles.map((profile) => <option key={profile.id} value={profile.id}>{profile.name} / {profile.version}</option>)}
                  </select>
                </dd></div>
                <div><dt>Consumer</dt><dd><input name="consumer" defaultValue={requested.consumer} required /></dd></div>
                <div><dt>Purpose</dt><dd><input name="purpose" defaultValue={requested.purpose} required /></dd></div>
                <div><dt>Action</dt><dd><input name="action" defaultValue={requested.action} required /></dd></div>
                <div><dt>Delivery</dt><dd><input name="delivery" defaultValue={requested.delivery} required /></dd></div>
                <div><dt>Scope type</dt><dd><input name="scopeType" defaultValue={requested.scopeType} required /></dd></div>
                <div><dt>Scope ref</dt><dd><input name="scopeRef" defaultValue={requested.scopeRef} placeholder="ALL_RESOURCE 时可留空" /></dd></div>
              </div>
              <button type="submit" style={{ marginTop: 14 }}>检查当前可交付性</button>
            </form>

            {!eligibility ? (
              <div className="callout callout-warn"><strong>需要完整 delivery context</strong><p>填写 consumer、purpose、action、delivery 与 normalized scope 后执行当前资格预检；ALL_RESOURCE 的 scope ref 会按每个 source DataResource 解析。</p></div>
            ) : (
              <section className="detail-card">
                <div className="panel-header">
                  <h3>预检结果</h3>
                  <Badge value={eligibility.allowed ? "ALLOWED" : "BLOCKED"} />
                </div>
                <div className="table-card">
                  <table className="data-table">
                    <thead><tr><th>Gate</th><th>结果</th><th>Blockers</th></tr></thead>
                    <tbody>
                      <tr><td>DatasetVersion usability</td><td><Badge value={eligibility.datasetVersion.allowed ? "ALLOWED" : "BLOCKED"} /></td><td>{blockerList(eligibility.datasetVersion.blockers)}</td></tr>
                      <tr><td>Current Certification</td><td><Badge value={eligibility.certification.allowed ? "ALLOWED" : "BLOCKED"} /></td><td>{blockerList(eligibility.certification.blockers)}</td></tr>
                      <tr><td>Current Entitlement</td><td><Badge value={eligibility.entitlement.allowed ? "ALLOWED" : "BLOCKED"} /></td><td>{blockerList(eligibility.entitlement.blockers)}</td></tr>
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
                            <td className="mono">{shortId(check.dataResourceId)}</td>
                            <td>{check.path}</td>
                            <td><Badge value={check.decision} /></td>
                            <td>{check.reason}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : null}
                {!eligibility.allowed ? <div className="callout callout-bad" style={{ marginTop: 14 }}><strong>当前不可交付</strong>{blockerList(eligibility.blockers)}</div> : null}
              </section>
            )}
          </>
        )}
      </>
    );
  } catch (error) {
    return <><BackLink href={`/datasets/${id}`}>返回数据集</BackLink><PageHeader title="DatasetVersion 详情" /><LoadError error={error} /></>;
  }
}
