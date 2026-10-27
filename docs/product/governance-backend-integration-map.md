# 新版全站后端接入映射与推进记录

> 本文保留早期逐页接入规划与当时结果。2026-10-25的当前演示口径统一见[领导演示能力清单](leader-demo-capabilities-20261025.md)与[交付指南](leader-demo-closeout-20261025.md)；下表的“待接/P1”等是阶段规划，不应覆盖后续已完成的独立只读切片。

## 接入原则与当前边界

本轮先接只读切片，不调用模型、执行引擎或真实交付。原 Agent 计划/确认/意图 API 保留。工作区为 aq1，主 checkout 不修改。

演示 ws-enterprise / VER-* 与 Core UUID 不是同一对象，不能用名称或最新时间猜映射。本轮提供显式 `/core/assets` 入口，服务端工作空间由部署配置和可信 principal 限定；演示空间不参与查询。接入后的错误显示 UNKNOWN，不使用演示记录兜底。Core 数据仍为隔离数据库内既有合成夹具，不是生产数据。

## 逐页映射

以下“可复用”表示当前源码存在相应 contract，不表示新版页面或本隔离实例已启用。P0 为已完成切片；P1 为下一轮只读接入；P2 为需业务 contract 收敛后接入；P3 为通用基础设施或暂保留演示。

| 新版菜单 / 页面 | 可复用能力或差异 | 当前结论 / 优先级 |
|---|---|---|
| 治理总览 | readmodel workspace workbench | 待接真实汇总，不能合并两空间计数 / P1 |
| Agent 工作台、任务详情 | agent-tasks、authority、revisions、approvals、actions、pause/resume/stop | 保留既有接入，dispatch 关闭 |
| 架构搭建流程 | 导航编排，不等同业务对象 | 保留导航，目的页分批接入 / P2 |
| 数据开发流程 | 需求/方案与 Workflow/Execution 不一一对应 | 先明确对象契约 / P2 |
| 沉淀提升流程 | 治理导航，可引用质量/资产/标准入口 | 保留导航 / P2 |
| 我的工作、验收归档 | 演示 workCommand 不是现有 Execution 生命周期 | 需需求办理、交付、业务验收独立 contract / P2 |
| 开发概览 | workspace executions、失败关注项 | 可先只读，不能把保留枚举当已实现状态 / P1 |
| 开发方案、需求清单 | UseCase、WorkflowVersion 语义不同于本地 PLAN | 明确需求/方案归属及冻结依赖 / P2 |
| 数据汇聚、来源登记、汇聚批次 | source-admissions、现有 CSV 接纳能力 | 先读来源与接纳结果；不启动 connector / P1 |
| 数据处理、任务、运行报告 | workspace executions、execution detail、quarantine | 先接历史查询；不创建/重试执行 / P1 |
| 数据发布、发布准备报告 | data-products、product-releases、readiness、traceability | 先接发布历史/检查结果；演示 REL 不能直接替代 ProductRelease / P1 |
| 交付工具箱 | 模板/辅助导航 | 保留演示；实际动作另设 gate / P3 |
| 技术支持 | 运行环境登记不等同真实引擎管理 | 未确认同等公开 contract / P3 |
| 管理概览 | workbench 与治理查询汇总 | 先汇总已接只读数据 / P1 |
| 调研规划 | 业务底稿、制度、计划没有已确认同等 API | 先定义业务 contract / P2 |
| 系统架构 | 主题域/分层设计不等同 runtime 配置 | 保留设计记录；待 contract / P2 |
| 数据模型、实体审核 | entity-match jobs/reviews/mapping decisions；指标模型为另一语义 | 实体审核先读；指标版本 contract 待确认 / P1、P2 |
| 数据质量、监控、整改 | quality-assessments、report、attempt、remediation case | 历史只读 P0；整改命令继续关闭 / P2 |
| 元数据管理、探查、观察 | schema requests/observations、exceptions、attention | 先接观察历史；不触发外部探查 / P1 |
| 数据安全、权利、合规、合约 | Rights/Authorization、Compliance、Contract 独立事实 | 先读依据，不能把三个通过复选框作为真实 gate / P1 |
| 数据资产、版本详情 | 已有 agent-quality-context 复用 catalog/version repositories | P0 已接 `/core/assets`，版本与质量证据同源 |
| 独立认证 | profiles、certification-inputs、certification history、eligibility | P1 先只读；历史认证不等于当前交付许可 |
| 标准建设 | industry-pack rules 不等同本地 STD 发布版本 | 先明确规则定义与平台标准对象关系 / P2 |
| 知识沉淀 | 案例/知识库未确认同等业务 API | 保留演示 / P3 |
| 设置总览 | 不可从本地配置推断部署事实 | P3，先整理配置权威来源 |
| 工作空间管理 | 部署工作空间/可信 principal 与本地 workspace-model 不同 | P2，不能自动迁移本地空间 |
| 空间成员与角色 | 可信 membership/角色授权需服务端来源 | P2，演示成员不成为授权身份 |
| 用户管理 | 优先标准身份提供方，禁止自造登录系统 | P3，复用仓库能力后评估 OIDC |
| 组织机构 | 业务组织与身份提供方组织需划分 | P3，待 contract |
| 角色权限 | 优先现有 principal/capability，后按 capability-map 评估 Casbin | P2，不新增平行授权引擎 |
| 审计日志、活动证据 | Evidence/Audit 对象关联查询与 traceability | P0 已读质量报告引用；全局检索 P1 |
| 运行日志 | application log 不等于 AuditEvent | P3，复用 observability |
| 基础配置 | 本地演示配置不是服务端配置 | P3，避免凭前端改运行环境 |
| 消息、个人资料 | 当前活动生成的本地提醒与演示个人资料 | P2，先明确通知/身份数据源 |
| 取数审批、交付记录、去向、接口发布 | DeliveryOperation/current gate 与本地 GET/DEL 不等价 | P1 评估只读 contract；真实交付 P2 单独授权 |
| 成本、价值、资产地图 | CostEvent/CostAllocation、lineage；业务价值指标需定义 | P1 先读成本与血缘；价值 P2 |

## 第一个切片实现

`/core/assets` → 当前授权数据集 → 明确 DatasetVersion → 历史 QualityAssessment → 原评测报告、发现项、Evidence/Audit 引用、已知 attempt 核实。

- 复用 Agent 质量页完整流程，抽为 `components/core-quality-view.tsx`，Agent 原路径继续使用同一组件。所有目录/版本/评测/分页链接保留各自路径，不另建查询状态仓库。
- 复用 `readAgentContext`、`validAgentContext`、`readAgentEvidence` 及现有质量 transport。复用现有 principal 与 workspace 检查，凭据只在服务端使用。
- 隔离预览先前仅挂载 Agent planning handler；本轮挂载已有 agent-quality-context，以及质量 handler 的 `RegisterReadOnly`。真实评测 POST 不注册，quality service 不实例化，不加载 worker/provider。
- 页面只读，不提供认证、发布、交付或评测执行按钮。Core 面板不展示演示重置，原标签页演示历史保留。

## Reuse check

现有仓库已具备目录、版本、质量查询、范围校验、分页及失败隔离；本轮直接复用。无需新增持久化、队列、策略引擎或第三方组件，也不替换 Core System of Record。未引入新的 commodity infrastructure，不新增兼容 shim。

## 后续顺序

2026-10-24 更新：明确质量评测的成本与证据/审计明细已接，见 [本轮验收](governance-quality-cost-evidence-20261024.md)。全局成本汇总和发布 Trace 尚未接入；通用 Trace 的 principal/workspace 缺口需先修复后使用。

2026-10-24 更新：认证历史、工作空间发布历史的只读接入已完成实现与接口回归，详见 [认证和发布历史接入](governance-history-readonly-acceptance-20261024.md)。它不包含 certification evaluate、发布命令、Readiness 实时评估或 Delivery。下面顺序中的这些历史查询已不再是待接接口。

1. 扩展认证/发布历史、成本与证据只读路径，逐模块补足当前 principal/workspace contract。
2. 收敛需求、方案、标准、业务验收的真实对象 contract，再选择单一受控写切片。
3. 模型、外部连接、评测/处理执行、真实交付继续保持关闭，各自单独设计与验收。

## 本轮验证结果

- 完整隔离 Next 构建 PASS，含 26 项演示回归、已有 Agent/质量及其他前端回归、编译、类型检查。日志 `.data/governance-core-readonly-build.log`。
- Go quality HTTP、agentquality query/adapter、preview 编译测试 PASS。新增只读注册回归验证评测执行 POST 不可达，查询未认证时先拒绝，不访问 repository。
- `tests/agent-task-preview/readonly-api.test.mjs`：当前隔离实例验证认证、跨 workspace、非法 UUID、不存在对象、明确报告主体和被禁用的评测命令，及既有 Agent r4 / REQUESTED / dispatchEnabled=false。日志 `.data/governance-core-readonly-api.log`。
- Edge 实走 PASS：顶部 Core 只读资产 → 6 个服务端数据集 → energy_raw → v1 `4c208c9b-d4d7-42ad-a119-101637d99e77` → 明确评测 `d3667496-f421-497c-be69-e5dec6bb569b`。显示 2 个发现项、1 个证据引用、1 个审计引用。未选择评测时不默认查询最新报告。
- Edge 失败隔离与刷新 PASS：不存在的 attempt 显示 UNKNOWN，所选版本、明确报告和已知证据保留；最终构建刷新后身份保持。Core 面板无本地重置入口。测试没有创建新评测或意图。
- 数据库计数仍为 CostEvent=7、Execution=0、源 QualityAssessment=1、修复 assignment=0，沿用原夹具，没有真实执行或新增成本。
- 当前截图已保存并查看：`.data/governance-browser-acceptance/14-core-catalog.png`、`15-core-report.png`、`16-core-unknown.png`；16 同名 txt 保存可见事实。

限制：这是第一个查询切片，不代表认证、发布、交付等整站已接后端。数据源为隔离测试 Core；真实身份提供方、多空间切换、其他模块写入、真实模型和执行均未启用。本轮没有新增数据库 migration 或业务状态。


2026-10-24 发布 Trace 更新：身份/工作空间与同一只读快照边界、新版发布证据链页面已实现，自动化回归通过。浏览器与重启后接口验收 NOT_RUN，不能视为本轮已完成接入验收。详见 [发布证据链只读接入](governance-release-trace-readonly-20261024.md)。


2026-10-24 交付/归档更新：`/core/deliveries` 已接现有 DeliveryOperation 历史及 gate/transition 只读投影；`/core/archives` 已接 EvidenceSnapshot 冻结档案。业务签收/验收通过尚无正式后端实现，不能用归档证据替代。独立库 API 与 Edge 验收、指纹无写入验证 PASS，见 [本轮交付与证据归档验收](governance-delivery-history-readonly-20261024.md)。前述 Trace 浏览器 NOT_RUN 是历史快照，已由上一轮独立只读验收文档及本轮实际路径覆盖。

2026-10-24 业务接入设计更新：[后端契约草案](../architecture/business-acceptance-backend-contract-v1.md) 定义责任、标准来源、签收、冻结提交、验收决定、归档和作废的目标边界；[页面与切片计划](governance-business-backend-plan-20261024.md) 给出现有 API、缺口及优先级。两份均为设计，拟议 API/表尚未实现，不改变本预览只读范围或 PR #363 的演示合约。

2026-10-24 第 0 切片更新：[标准与责任只读接入](governance-acceptance-s0-readonly-20261024.md) 已补合约版本 GET 的可信身份/workspace、结构化标准校验、安全列表/上下文及新版 `/core/acceptance-context`。责任源未接入时明确返回未配置和阻断，不用演示角色替代。正式签收、标准确认、验收决定和归档写接口仍未实施。
