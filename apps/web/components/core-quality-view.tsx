import {businessText,displayText} from '../lib/business-language.mjs';
import CoreQualityCost from "./core-quality-cost";
import { Badge, BackLink, EmptyState, PageHeader } from "@/components/ui";
import { configuredWorkspaceId, platform } from "@/lib/platform";
import { readSection } from "@/lib/workbench-read";
import { readAgentContext } from "@/lib/agent-quality-server";
import {
  AGENT_PAGE_SIZE,
  agentHref,
  agentSelection,
  readAgentEvidence,
  validAgentContext,
  type AgentSelection,
} from "@/lib/agent-quality";

function Unknown({ children }: { children: React.ReactNode }) {
  return (
    <div className="callout callout-warn" role="status">
      <strong>无法确认</strong>
      <p>{displayText(children)}</p>
    </div>
  );
}
function Pages({
  offset,
  total,
  href,
  label,
}: {
  offset: number;
  total: number;
  href: (offset: number) => string;
  label: string;
}) {
  return (
    <nav className="pagination" aria-label={label}>
      {displayText(offset > 0 ? (
        <a href={href(Math.max(0, offset - AGENT_PAGE_SIZE))}>← 上一页</a>
      ) : (
        <span>已到首页</span>
      ))}
      <span>
        共 {displayText(total)} 条 · 当前 {displayText(offset < total ? offset + 1 : 0)}–
        {displayText(offset < total ? Math.min(offset + AGENT_PAGE_SIZE, total) : 0)}
      </span>
      {displayText(offset + AGENT_PAGE_SIZE < total ? (
        <a href={href(offset + AGENT_PAGE_SIZE)}>下一页 →</a>
      ) : (
        <span>已到末页</span>
      ))}
    </nav>
  );
}
const labels: Record<string, string> = {
  ACCEPTED_BATCH_THREE_RAW: "同一已接纳批次的三份 RAW 输入",
  MATCHING_ENTITY_RESOLUTION_OUTPUT: "与该批次对应的 STANDARDIZED 实体解析结果",
  OUTPUT_DATASET_BINDING: "固定输出数据集绑定",
  CURRENT_EXECUTION_AUTHORITY: "执行时的当前授权",
};

export default async function CoreQualityView({
  searchParams,
  catalog = false,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
  catalog?: boolean;
}) {
  const basePath = catalog ? "/core/assets" : "/agent/quality";
  const href = (s: Partial<AgentSelection> = {}) => agentHref(s).replace("/agent/quality", basePath);
  let selection: AgentSelection;
  try {
    selection = agentSelection(await searchParams);
  } catch {
    return (
      <>
        <BackLink href={basePath}>重新选择上下文</BackLink>
        <PageHeader title="上下文引用无效" />
        <Unknown>
          请使用明确的数据版本和评测引用。没有读取其他版本来替代。
        </Unknown>
      </>
    );
  }
  const workspace = configuredWorkspaceId() ?? "";
  const context = await readSection(
    () => readAgentContext(selection),
    (c) => validAgentContext(c, workspace, selection),
  );
  // Source scope must be established before reading any version evidence.
  const evidence = context?.version
    ? await readAgentEvidence(workspace, selection, {
        history: platform.qualityAssessments,
        report: platform.qualityReport,
        attempt: platform.qualityAttempt,
      })
    : null;
  const report = evidence?.report;
  const list = context?.datasets ?? context?.versions;
  return (
    <div className="agent-page">
      <BackLink href="/agent">Agent 工作台</BackLink>
      {displayText(catalog && <nav className="capability-links" aria-label="相关业务导航"><a href="/core/assets">资产目录</a><a href="/core/releases">发布历史</a></nav>)}
      <PageHeader
        eyebrow="质量管理"
        title={catalog ? "资产目录" : "质量上下文"}
        description="查看资产版本、质量评测与证据，报告对应当前选定的评测记录。"
        action={<Badge value="只读" tone="info" />}
      />
      <div className="agent-scope">
        <span>
          工作空间 <code>{workspace || "未配置"}</code>
        </span>
        {context && (
          <span>
            读取于{businessText(" ")}
            {displayText(new Date(context.readAt).toLocaleString("zh-CN", {
              timeZone: "Asia/Singapore",
              hour12: false,
            }))}
          </span>
        )}
      </div>
      {!context ? (
        <Unknown>
          暂时无法读取数据，请确认当前工作空间与访问权限，或联系管理员。
          <a className="text-link" href={href(selection)}>
            {businessText(" ")}
            重新读取 →
          </a>
        </Unknown>
      ) : (
        <>
          <nav className="agent-breadcrumb" aria-label="上下文路径">
            <a href={basePath}>数据集</a>
            {displayText(context.dataset && (
              <>
                <span> / </span>
                <a href={href({ datasetId: context.dataset.id })}>
                  {businessText(context.dataset.name)}
                </a>
              </>
            ))}
            {context.version && <span> / v{displayText(context.version.versionNo)}</span>}
          </nav>
          {list && (
            <section className="card">
              <div className="section-heading">
                <div>
                  <h2>
                    {businessText(context.dataset ? "选择明确的数据版本" : "选择数据集")}
                  </h2>
                  <p>仅显示当前授权工作空间中的记录。</p>
                </div>
                <Badge value={`${list.page.total} 条`} />
              </div>
              {list.items.length === 0 ? (
                <EmptyState
                  title="暂无记录"
                  description="当前页未返回可选择的数据。"
                />
              ) : (
                <div className="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>名称 / 版本</th>
                        <th>类型 / 状态</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {displayText(context.datasets?.items.map((d) => (
                        <tr key={d.id}>
                          <td>
                            <strong>{businessText(d.name)}</strong>
                            <details><summary>查看资产编号</summary><code>{d.code}</code></details>
                          </td>
                          <td>
                            <Badge value={d.datasetType} />
                          </td>
                          <td>
                            <a
                              className="text-link"
                              href={href({ datasetId: d.id })}
                            >
                              查看版本 →
                            </a>
                          </td>
                        </tr>
                      )))}
                      {context.versions?.items.map((v) => (
                        <tr key={v.id}>
                          <td>
                            <strong>v{displayText(v.versionNo)}</strong>
                            <div className="muted">{businessText(v.id)}</div>
                          </td>
                          <td>
                            <Badge value={v.status} />
                          </td>
                          <td>
                            <a
                              className="text-link"
                              href={href({
                                datasetId: v.datasetId,
                                versionId: v.id,
                              })}
                            >
                              核实上下文 →
                            </a>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
              <Pages
                offset={selection.offset}
                total={list.page.total}
                label="数据来源分页"
                href={(offset) =>
                  href({ datasetId: selection.datasetId, offset })
                }
              />
            </section>
          )}
          {context.version && (
            <div className="agent-context-grid">
              <div className="agent-main-column">
                <section className="card">
                  {displayText(catalog && <a className="button" href={"/core/certifications?"+new URLSearchParams({datasetId:context.version.datasetId,versionId:context.version.id})}>查看此版本认证历史 →</a>)}
                  <div className="section-heading">
                    <div>
                      <span className="eyebrow">来源记录</span>
                      <h2>
                        {businessText(context.dataset?.name)} · v{displayText(context.version.versionNo)}
                      </h2>
                    </div>
                    <Badge value={context.version.status} />
                  </div>
                  <p>
                    数据版本{businessText(" ")}
                    <code className="agent-id">{businessText(context.version.id)}</code>
                  </p>
                  <p>
                    记录行数：
                    {displayText(context.version.rowCount === undefined
                      ? "UNKNOWN"
                      : context.version.rowCount.toLocaleString("zh-CN"))}
                  </p>
                  <p className="muted">
                    数据版本状态与质量结论分开记录。当前页面不会执行评测或修复。
                  </p>
                </section>
                <section className="card">
                  <div className="section-heading">
                    <div>
                      <h2>评测历史</h2>
                      <p>选择一份评测，查看当时的独立事实。</p>
                    </div>
                    <a className="text-link" href={href(selection)}>
                      刷新
                    </a>
                  </div>
                  {!evidence?.history ? (
                    <Unknown>
                      评测列表读取失败。已明确选择的报告仍独立读取。
                    </Unknown>
                  ) : (
                    <>
                      {!evidence.history.items.length ? (
                        <EmptyState
                          title="暂无评测记录"
                          description="没有评测记录不代表质量通过。"
                        />
                      ) : (
                        <div className="agent-history">
                          {displayText(evidence.history.items.map((a) => (
                            <a
                              title={a.ruleSetRef}
                              key={a.id}
                              className={`agent-history-row ${a.id === selection.assessmentId ? "selected" : ""}`}
                              aria-current={
                                a.id === selection.assessmentId
                                  ? "true"
                                  : undefined
                              }
                              href={href({
                                ...selection,
                                assessmentId: a.id,
                                findingOffset: 0,
                              })}
                            >
                              <span>
                                <strong>质量评测记录</strong>
                                <small>{businessText(a.id)}</small>
                              </span>
                              <Badge value={a.gateDecision} />
                            </a>
                          )))}
                        </div>
                      )}
                      <Pages
                        offset={selection.historyOffset}
                        total={evidence.history.page.total}
                        label="评测历史分页"
                        href={(historyOffset) =>
                          href({ ...selection, historyOffset })
                        }
                      />
                    </>
                  )}
                </section>
                <section className="card" id="assessment-report">
                  <div className="section-heading">
                    <div>
                      <span className="eyebrow">指定评测</span>
                      <h2>评测报告</h2>
                    </div>
                    {displayText(report && <Badge value={report.gateDecision} />)}
                  </div>
                  {!selection.assessmentId ? (
                    <EmptyState
                      title="请选择一份评测"
                      description="点击上方历史记录查看报告，不会默认切换为最新评测。"
                    />
                  ) : !report ? (
                    <Unknown>
                      所选报告 {displayText(selection.assessmentId)}{businessText(" ")}
                      不可确认。可能是读取失败、权限不足或不属于当前版本。没有替换成最新报告。
                    </Unknown>
                  ) : (
                    <>
                      <p>
                        评测 <code className="agent-id">{businessText(report.id)}</code>
                      </p>
                      <p className="muted">
                        规则版本 {displayText(report.ruleSetVersion)} ·{businessText(" ")}
                        {displayText(report.createdAt)}
                      </p>
                      <details><summary>查看评测规则引用</summary><code>{report.ruleSetRef}</code></details>
                      <div className="table-wrap">
                        <table>
                          <thead>
                            <tr>
                              <th>规则 / 发现</th>
                              <th>严重性</th>
                              <th>状态</th>
                            </tr>
                          </thead>
                          <tbody>
                            {report.findings.items.map((f) => (
                              <tr key={f.id}>
                                <td>
                                  <strong>{businessText(f.ruleId)}</strong>
                                  <div className="muted">
                                    {displayText(f.reason || "未记录原因")}
                                  </div>
                                </td>
                                <td>{businessText(f.severity)}</td>
                                <td>
                                  <Badge value={f.status} />
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                      {displayText(!report.findings.page.total && (
                        <p className="muted">
                          本次未返回发现项；请以该评测的实际结论为准。
                        </p>
                      ))}
                      <Pages
                        offset={selection.findingOffset}
                        total={report.findings.page.total}
                        label="发现项分页"
                        href={(findingOffset) =>
                          href({ ...selection, findingOffset })
                        }
                      />
                      <div className="agent-evidence">
                        <div>
                          <h3>证据引用 · {displayText(report.evidence.length)}</h3>
                          {report.evidence.map((e) => (
                            <details key={e.id}><summary>{businessText(e.id)}</summary><p>类型：{displayText(e.evidenceType)} · 关系：{displayText(e.relationType)}</p><p>来源：{displayText(e.sourceType)} / {displayText(e.sourceId||'未返回')}</p><p>摘要：{displayText(e.hashAlgorithm||'未返回')} / {displayText(e.hashValue||'未返回')}</p><p>记录时间：{displayText(e.createdAt)}</p></details>
                          ))}
                          {displayText(!report.evidence.length && (
                            <p className="muted">未返回证据引用</p>
                          ))}
                        </div>
                        <div>
                          <h3>审计引用 · {displayText(report.auditEvents.length)}</h3>
                          {report.auditEvents.map((e) => (
                            <details key={e.id}><summary>{businessText(e.id)}</summary><p>动作：{businessText(e.action)}</p><p>主体：{displayText(e.objectType)} / {displayText(e.objectId)}</p><p>办理类型：{displayText(e.actorType)}</p><p>记录时间：{displayText(e.occurredAt)}</p></details>
                          ))}
                          {displayText(!report.auditEvents.length && (
                            <p className="muted">未返回审计引用</p>
                          ))}
                        </div>
                      </div>
                    </>
                  )}
                </section>
                {displayText(catalog && report && <CoreQualityCost workspace={workspace} version={context.version.id} assessment={report.id}/>)}
                <section className="card">
                  <h2>评测尝试核实</h2>
                  <p className="muted">
                    响应中断时，可按已有评测记录编号 查询事实；不会发起新评测。
                  </p>
                  <form
                    method="get"
                    action={basePath}
                    className="agent-lookup"
                  >
                    {displayText((["historyOffset", "findingOffset"] as const).map(
                      (key) => (
                        <input
                          key={key}
                          type="hidden"
                          name={key}
                          value={selection[key]}
                        />
                      ),
                    ))}
                    <input
                      type="hidden"
                      name="versionId"
                      value={context.version.id}
                    />
                    {displayText(context.dataset && (
                      <input
                        type="hidden"
                        name="datasetId"
                        value={context.dataset.id}
                      />
                    ))}
                    {displayText(selection.assessmentId && (
                      <input
                        type="hidden"
                        name="assessmentId"
                        value={selection.assessmentId}
                      />
                    ))}
                    <label htmlFor="attemptId">评测记录编号</label>
                    <input
                      id="attemptId"
                      name="attemptId"
                      required
                      defaultValue={selection.attemptId ?? ""}
                      placeholder="填写完整记录编号"
                    />
                    <button className="button" type="submit">
                      核实记录
                    </button>
                  </form>
                  {selection.attemptId &&
                    (!evidence?.attempt ? (
                      <Unknown>
                        所选尝试不可确认。没有触发重试或创建新的评测。
                      </Unknown>
                    ) : (
                      <div className="callout">
                        <Badge value={evidence.attempt.outcome} />
                        <p>
                          <code className="agent-id">
                            {businessText(evidence.attempt.id)}
                          </code>
                        </p>
                        {displayText(evidence.attempt.assessmentId ? (
                          <a
                            className="text-link"
                            href={href({
                              ...selection,
                              assessmentId: evidence.attempt.assessmentId,
                              findingOffset: 0,
                            })}
                          >
                            查看该尝试关联的评测 →
                          </a>
                        ) : (
                          <p>尚无关联评测事实。</p>
                        ))}
                      </div>
                    ))}
                </section>
              </div>
              <aside
                className="agent-side-column"
                aria-label="上下文与模板要求"
              >
                <section className="card">
                  <span className="eyebrow">关联信息</span>
                  <h2>当前上下文</h2>
                  <details><summary>查看资产编号</summary><code>{context.dataset?.code}</code></details>
                  <p>
                    <Badge value={context.dataset?.datasetType} />{businessText(" ")}
                    <Badge value={`v${context.version.versionNo}`} />
                  </p>
                  <p className="muted">
                    固定为所选版本。切换评测不会改变数据版本。
                  </p>
                </section>
                <section className="card">
                  <div className="section-heading">
                    <h2>修复模板</h2>
                    <Badge
                      value={context.template.configured ? "已配置" : "未配置"}
                      tone={context.template.configured ? "info" : "warn"}
                    />
                  </div>
                  {context.template.configured ? (
                    <>
                      <p>
                        {businessText(context.template.name)} · {displayText(context.template.version)}
                      </p>
                      <details><summary>查看模板版本与校验摘要</summary><p className="muted">计划校验摘要</p>
                      <code className="agent-id">{context.template.hash}</code>
                      <p className="muted">工作流校验摘要</p>
                      <code className="agent-id">
                        {context.template.workflowHash}
                      </code></details>
                      <ul>
                        {displayText(context.template.inputs.map((i) => (
                          <li key={i.name}>
                            {businessText(i.name)} · {businessText(i.datasetType)}
                            {businessText(i.required ? " · 必需" : "")}
                          </li>
                        )))}
                      </ul>
                    </>
                  ) : (
                    <p>尚未配置已验证的修复模板。</p>
                  )}
                </section>
                <section className="card">
                  <h2>执行条件</h2>
                  <Badge value="NOT EVALUATED" tone="warn" />
                  <p>以下条件尚未在本次查询中核验：</p>
                  <ul>
                    {displayText(context.unverified.map((x) => (
                      <li key={x}>{businessText(labels[x] ?? x)}</li>
                    )))}
                  </ul>
                  <p className="muted">
                    此处的模板配置和历史质量结论均不构成执行授权。
                  </p>
                </section>
              </aside>
            </div>
          )}
        </>
      )}
    </div>
  );
}
