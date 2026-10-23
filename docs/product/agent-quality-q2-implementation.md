# Agent Q2 委托与计划持久化

2026-10-23，诊断计划前后端切片已验收，修复计划 API 已验收，修复绑定页面尚待实现。复用 Q1 的隔离 checkout，不修改 Q1 数据库、进程或构建。当前权威契约为 `docs/architecture/agent-quality-backend-contract-v1.md`；本文限定 Q2 范围，不替代后续执行契约。

## 有限实施边界

- 新增 QUALITY_DIAGNOSIS 委托、完整原子冻结的计划修订、DIAGNOSIS / REPAIR 两阶段确认、稳定动作意图及暂停/恢复/停止记录。
- 新增命令的同键同内容重放返回原收据；同键异内容、过期 task revision、旧计划/确认和重复动作槽位拒绝。修订、确认和受理竞争同一个 task lock。
- 所有历史计划、成员、确认、意图与收据不可覆盖；DRAFT 只存在于创建事务，不能提交；成员插入与冻结竞争同一个 plan parent lock。
- 计划和确认只证明已保存的办理范围。Q2 不注册动作消费者，不调用 Detect、Assign、Execute、Retry 或 Recheck；动作事件保留显式未满足的 dispatch obligation，不能当作已消费。
- REPAIR 计划通过既有 Remediation owner 的准备查询复用同批 receipt、解析结果、workflow 和输出绑定规则；不复制一套行业 validator。引用现存 case 仅是显式规划依据，不冒充 Agent 已完成检测的收据。
- 不新增报告、模型、通用运行时、队列、IAM、发布或交付。RETRY 绑定与实际步骤派发属于 Q3/Q4。存在未决动作时先核实/停止，不用新计划掩盖它。

## 复用与身份边界

复用现有 pgx/transaction.Manager、principal.Resolver、JCS、Audit、Evidence 和版本化 Outbox 路由。新增表拥有的是本平台委托、确认范围和动作幂等语义。现有 WorkflowVersion/Execution 不拥有这些事实；成熟通用编排方案 Temporal 不解决当前平台的冻结/确认 invariant，且现有队列/outbox 已足够，因此不引入第二套基础设施。

发现写入口缺口：现有 Web 的 Quality 只读查询使用部署端固定凭据，尚无访客身份。Q2 写入必须使用当前请求携带的可信 Bearer，而不能从部署凭据匿名代办。读写开关分别控制；缺少可信请求身份时展示只读状态。

本地交互验收已使用现有 Caddy 镜像的标准 Basic Auth 和 reverse_proxy，代理认证后向 Next 注入固定试点 Bearer；覆盖来自浏览器的同名授权头并保留来源 Host/port。Core 最终验证 principal 与 workspace。Caddy 只作为测试入口，其临时合成登录不代表多人 SSO。正式开放沿用后续标准身份方案。验收账号只写 Agent 及其 Audit/Evidence/Outbox 表；为 PostgreSQL 行锁授予 case/job 的 id 列 UPDATE 权限，但 case immutable guard 和预览专用 job trigger 拒绝实际 owner mutation。没有 owner Command 路由，不启动 worker。

## 当前进展与验证状态

已实现领域模型与 JCS 内容冻结、migration 73、Repository、有限 Commands、受认证的 HTTP API、默认关闭的写开关和版本化 Outbox 路由。迁移只应用于新建的独立验收库及测试临时库。Next 页面支持诊断委托创建、计划修订、确认、意图保存、暂停/恢复/停止、历史只读和 URL 刷新恢复。未核实的提交在 sessionStorage 保存原键与原内容，刷新后必须先重试原请求。

- PASS：Agent 领域 5 组测试，覆盖内容绑定、JCS、诊断范围、修复来源与动作顺序、修订号及暂停/停止门禁。
- PASS：Agent HTTP 身份、关闭写入、伪造 actor、重复 JSON 键、跨命令字段和非法 body 检查。
- PASS：真实 PostgreSQL migration 73 空库 down/up、有历史 down 拒绝、header/成员 mutation 拒绝、DRAFT 无法提交；第二连接先启动且看不到未提交草稿，冻结提交后其晚到成员写入被拒绝。
- PASS：真实 PostgreSQL 同键并发创建、同键异内容拒绝、过期 revision 的原键重放、旧确认失效、动作槽唯一、未决动作阻止修订、跨 actor/workspace、停止与动作受理争用同一修订号，以及缺失 Outbox 路由导致事务整体回滚。
- PASS：每次命令收据与 Audit/Evidence/Outbox 同事务记录；动作事件保留 `agent-action-dispatch` 未满足义务，未注册消费者。
- PASS：既有 Remediation 合成测试夹具中，Agent REPAIR 计划冻结、独立 REPAIR 确认、ASSIGN_SELF 意图可保存；无实际 assignment/Execution owner 收据时不能继续 EXECUTE_REPAIR。准备查询不增加质量引擎调用。该测试复用原测试的内存对象存储及受控 fake engine；不代表接入真实引擎。
- PASS：完整 Next build（含已有前端回归套件）、独立 typecheck/lint、受影响 Go 包测试及 diff whitespace 检查。未配置 DSN 的其他集成用例跳过，不计为真实数据库验收。
- PASS：真实浏览器创建、刷新、确认 V1、修订 V2 后重新确认、保存意图、暂停/恢复/停止、历史只读、未决动作限制；CDP 临时断网后恢复网络、刷新及原请求重试；最终 Core/Next 重启后记录仍可读取。网络模拟已恢复。
- PASS：独立预览 API 3 组验收，包括 6 个同键并发请求、意图原键重放、重复槽拒绝、停止取消、直连 Next 匿名写入拒绝及跨空间/伪造 actor 拒绝。
- PASS：REPAIR 独立绑定页接入既有 revisions API；复用诊断委托完整提交控制器，适配 REPAIR 依赖及大小写 InputBinding contract；现存修复计划编辑保持 REPAIR，不退回诊断。
- PASS：浏览器完整依赖保存、缺失输入拒绝且保留字段、503 后刷新原键重试恢复、取消无写入、编辑回填、独立修复确认、确认刷新恢复、ASSIGN_SELF 意图保存、刷新后防重复和历史只读。
- 尚未开放：模型、业务派发、Q3/Q4 owner receipts、执行重试和报告。绑定仍需明确 UUID 及模板输入名，检索选择体验可后续完善。

隔离预览使用真实 Agent Application/HTTP 和 PostgreSQL，专用验收 host 只注册 Agent 路由；不启动模型、质量引擎、执行器、Redis、对象存储或 worker。此前诊断验收时该空间 QualityAssessment/Execution/CostEvent 为 0；本轮明确生成合成修复夹具后，浏览器/API 操作前后均为 QualityAssessment=1、Execution=0、CostEvent=7、该 case assignment=0。7 条成本来自既有受控测试夹具准备，不能算作 Agent 执行或宣称总成本为 0。原 Remediation 回归测试使用其他随机工作空间的合成夹具，不能与预览办理成果混淆。未修改 Q1 数据库、原进程或构建，也没有合并或部署。主 checkout 仍只有原先两份未跟踪设计文档。

入口与重现步骤见 `tests/agent-task-preview/README.md`。本轮证据在 `.data/agent-q2-preview/`：`postgres-test.log`、`remediation-test.log`、`go-tests.log`、`api-test.log`、`evidence/closed-task.jpg`；前端完整构建日志为 `.data/agent-q2-build.log`。本地临时文件不提交。

## 2026-10-23 新版 Agent 界面接入

Agent 工作台、任务详情、上下文侧栏及独立修复绑定页已迁移到已确认原型的暖白/翡翠绿设计。复用现有完整 TaskWorkbench 提交控制器、Core API、CAS、冻结计划、幂等键与待核实请求恢复；没有把原型 React 内存状态当作服务端事实。仅 Agent 路由采用新壳，其他业务中心尚未整体迁移。图标复用原型采用的 @phosphor-icons/react 2.1.10（MIT），没有新增基础设施。

新版真实浏览器委托 ef6e94ef-23fb-4afe-8296-b9960ce86127：创建诊断、缺失修复输入拒绝并保留字段、补齐绑定冻结 V2 REPAIR、独立确认、保存 ASSIGN_SELF、刷新恢复均通过。最终 r4，计划 08f396bb-bcf8-4b57-bb3a-e42dc606179c，意图 787a579f-3936-4bc4-ad34-b4d7e66e9cf9 保持 REQUESTED / 未派发。重复确认与意图按钮禁用，V1 历史只读；不存在任务显示读取失败且不回退新建表单。意图与历史标签通过 URL 刷新恢复。

完整 Next build 与 API 4 组回归通过，日志 .data/agent-modern-build.log、.data/agent-modern-api.log；截图 .data/agent-q2-preview/evidence/agent-modern-workbench.jpg、agent-modern-detail.jpg。验收后固定空间 Cost=7、Execution=0、来源 Quality=1、case assignment=0，与操作前一致。仍不接模型或真实派发；绑定暂需明确 UUID/输入名，未开放执行与报告。主 checkout 保持原状。

## 必需验收

领域 hash 正反例；同键并发与响应丢失恢复；新修订使旧确认不可受理；同槽位换键不得重复；跨空间/伪造 actor 拒绝；冻结后 header/成员 mutation 拒绝；真实双连接 late member/finalize 竞争；暂停/停止与受理竞争；事务失败不留下半条记录或孤立证据；空库 down/up 与有历史时 down fail closed；完整前端类型/lint/build；真实浏览器创建、修订、确认、刷新和重复提交。Q1 已冻结的验收记录保持原范围。

## 2026-10-23 修复绑定验收补充

预览 Repository 注入与锁权限已补齐，没有弱化 owner 的 parent lock / provenance 校验。Next 代理适配 Core 的顶层 code 错误信封，缺失输入展示 INPUT_BINDING_INCOMPLETE；浏览器月份输入改为明确 YYYY-MM 格式。所有实际命令继续携带 expectedRevision、当前冻结 plan ID/hash、phase 和幂等键。待核实提交刷新恢复原请求，禁止换键代办。

浏览器委托 ff8e3cbf-90b7-44d6-8d65-0720b46ba02c：V1 DIAGNOSIS 确认后修订 V2 REPAIR，V2 单独确认并保存 ASSIGN_SELF；最终 revision=5，确认 1 条，意图 cd0abb63-4942-417b-a894-aef0afb3812d=REQUESTED，未派发。修复 hash=7710c8c567a00312489bfaf2859c410ad03c090e46ae69d84583786f2ceac8b9。原键重试没有产生 V3。

证据：.data/agent-q2-preview/evidence/repair-plan-final.jpg、repair-api-test.log（4 组全部通过）、.data/agent-q2-repair-build.log、.data/agent-q2-repair-go.log。SQL 直接以预览角色 UPDATE entity_match_job SET id=id 被 preview owner mutation denied 拒绝；真实 Execution 与 assignment 均为 0。合成 fixture 准备使用已有测试 provider/内存存储，不接真实模型或外部引擎；验收 host 本身不具备这些 provider。首次出现的缺失绑定与权限失败均已保留，没有用界面成功提示掩盖失败。

## 后续全站设计收敛

此前“仅 Agent 使用新壳”的范围已被全站统一界面取代，详见 docs/product/governance-modern-ui-acceptance.md。原 Agent Commands、API 与冻结/幂等契约不变。其他页面仍为本地演示，不能作为真实执行、认证或交付证据。
