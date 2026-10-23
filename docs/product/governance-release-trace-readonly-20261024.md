# 发布证据链只读接入（2026-10-24）

状态：源码与自动化回归完成；本轮浏览器与运行实例接口验收 NOT_RUN，不宣称已完成整体验收。

## 本轮实现

- 两个现有 GET Trace 入口都要求单一、明确的 `workspaceId`，复用服务端 principal resolver 与 HUMAN_DECISION 试点 capability。无凭证 401，身份不具备工作空间权限 403；客户端 actor/workspace header 不产生权限。
- 发布根对象通过 ProductRelease → DataProduct 的真实归属确认。外空间与不存在根对象均 404，不通过证据是否存在来猜归属。通用入口只支持已核验的 EXECUTION；其他 objectType 400，没有无边界兜底。
- 发布与执行 Trace 都在 PostgreSQL REPEATABLE READ / READ ONLY 事务内完成根查询、成员读取与归属检查。复用现有 Evidence/Cost query repository，增加可接受 pool 或 transaction 的最小 Queryer contract，没有并行查询基础设施。
- 发布的数据版本/血缘、执行及工作流、冻结依赖、匹配任务、实际不可变映射决策/实体、快照、证据、成本、审计逐项验证工作空间。任何成员边界不能确认时整条查询失败，不静默过滤成完整证据链。
- 对外使用字段白名单：历史标识、状态、哈希/完整性、数量与精确金额等；排除 metadata、manifest、存储地址、引擎运行标识、来源键、审核自由文本等。错误不返回 SQL/provider 原文。响应与错误均 private/no-store。查询 10 秒超时，超过 1000 个返回成员整体拒绝，不截断冒充完整。
- 新版 `/core/release-trace` 展示冻结快照、数据版本与血缘、生产执行、实际映射决策、匹配历史、证据、成本与审计。发布历史直接进入，返回保留筛选/分页/明确发布。刷新继续查同一发布，不回退最新；无记录/越界/失败均 UNKNOWN。未选发布显示选择提示。未知金额不当零。
- 数据版本跳转复用现有 Core 资产/质量页面。旧 Release 页面收敛为新版入口并带 expected product ID，不再按已移除的敏感响应字段渲染。保留 Agent 路由、API 与执行关闭边界。

## 验证证据

- `.data/governance-trace-postgres-final.log`：隔离 PostgreSQL Trace、Evidence、Cost 回归 PASS。涵盖真实发布证据/哈希、不可变决策与当前映射区别、外空间根拒绝、外空间审计成员使整条失败、单连接事务读取、HTTP 身份先于 repository、字段脱敏。
- `.data/governance-trace-build-complete.log`：完整 Next 构建及所有现有前端回归 PASS，含新增明确 Trace root/成员/重复标识/缺失字段/回跳校验。
- `.data/governance-trace-go-compile.log`：全 Go 包编译检查（`go test ./... -run '^$'`）。这不是全 Go 行为测试。
- `tests/agent-task-preview/trace-readonly-api.test.mjs` 已准备，当前未对重启后的预览运行，NOT_RUN。
- PostgreSQL 行为测试使用独立 `trace_scope_acceptance_20261024_v2`，没有在保留源工作空间补造发布。第一次手工迁移未包事务，遇 LOCK TABLE 要求事务而失败，记录保留在 `.data/governance-trace-migration.log`；第二个独立库逐 migration 包事务成功。没有业务 schema migration 文件变更。
- 保留源工作空间 `11111111-1111-4111-8111-111111111111` 当前读取：CostEvent=7、Execution=0、QualityAssessment=1、认证=0、发布=0。全库还有其他既有测试工作空间，不能把全库数量当本工作空间回归结果。

## 尚未完成的验收

今晚协调消息新增了“不更新本地运行实例、不启动新 GUI”的边界；本轮最后构建后未重启预览，也未刷新浏览器冒充新构建验收。浏览器 NOT_RUN，现场 HTTP API 回归 NOT_RUN。预览此前通过所属 stop 脚本停止，数据库保留；不能报告当前预览可访问。

保留源工作空间无发布批次，因此正向完整证据链浏览器路径也需要单独的隔离合成发布夹具演示，并清楚标识数据来源。不能把 PostgreSQL 单测或旧截图代替浏览器验收。没有提交/推送/合并本轮：当前分支包含之前多轮未提交的 Q1/Q2、新版全站与只读接入前置变更，尚无锁定 HEAD 的完整可审 PR 或独立评审证据。

本轮未接模型、外部 provider 或真实执行，没有发布/认证/交付写入；历史发布、认证和快照完整性都不构成当前交付授权。


## 提交与 CI 收口补充

已建立 [Draft PR #363](https://github.com/qq550723504/data-product-platform/pull/363)，包括前序尚未提交的已批准 Agent/Q1/Q2、新版全站和 Core 只读前置工作，完整范围见 `governance-preview-merge-contract.md`。初始提交为 `4fde49fe8523636da898ada5aff1ba66d0f97426`，不是只提交 Trace。

首轮 CI 暴露 Vet 和新增 Agent FK 未接生命周期准入 guard；源码修复保留强类型 FK 发现与不可释放内容 pin，未变更现场数据、预算、#332 资源或 owner。路由修复只重写根演示页，保留真实 Core 深链并增加 `/core/workbench`；原浏览器测试保留原业务断言。新增现代根页/明确 Trace/越界与缺失 Trace 的 CI 浏览器用例，并复用既有真实 Core 发布历史验收覆盖新投影。最新 CI 结果需按实际修复 HEAD 查询，未完成前不得记录 PASS。独立全范围评审与本地最新预览验收仍未完成，不合并。
