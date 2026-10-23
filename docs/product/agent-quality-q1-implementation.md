# Agent Q1 只读接入实施记录

日期：2026-10-23。分支：`codex/agent-quality-readonly`。基线：`f7a90022aad5da4d67ea594ccb86f8b9925a3840`。

代码已实现，本地接口、真实 PostgreSQL、类型、lint、前端构建验证通过。**Q1 已完成合成 HTTP 夹具的 14 项浏览器验收，以及真实 Core 的 10 项 API / 11 项浏览器只读联调；208 张 public 表的行数和内容摘要前后完全一致。** 本记录不代表 Q7 全流程执行验收通过。未提交远端 PR，未合并、部署或接触保留运行环境。详见 [夹具浏览器验收](agent-quality-q1-browser-acceptance.md) 和 [真实 Core 只读联调](agent-quality-q1-live-readonly-acceptance.md)。

## 实现范围

- `/agent`：Agent 工作台入口，明确当前只读范围及后续尚未开放的阶段。
- `/agent/quality`：授权数据集 → 明确版本 → 评测历史 → 精确报告；提供版本与模板上下文侧栏、分页和已有评测尝试的核实入口。
- `GET /api/v1/workspaces/{workspaceId}/agent-quality-context`：复用现有可信 principal resolver，先验证工作空间与 `HUMAN_DECISION` capability，再查询现有仓库。
- 精确报告、历史和尝试分别复用既有授权 Quality GET。任一读取失败只把该区域标为 UNKNOWN；报告不会替换成 latest，也不会触发评测、修复或重试。
- 不新增 AgentTask、迁移、命令、队列、worker、模型、成本或审批事实。模板已配置仅表示服务端成功加载模板，不表示输入齐备或执行已获授权。

## Q1 查询契约

接口只接受 `datasetId`、`versionId`、`limit`、`offset`，拒绝重复及未知参数。

| 选择 | 响应 |
|---|---|
| 无选择 | 当前主体授权空间中的数据集分页 |
| datasetId | 验证数据集归属后返回版本分页 |
| versionId | 返回该明确版本及所属数据集；不回退当前版本 |
| 两个 ID | 两者必须匹配并属于授权空间，否则 404 |

默认 limit=25，范围 1–100；offset 为 0–2147483647。未配置/缺失/错误凭据为 401，空间或 capability 不允许为 403，对象不属于空间/不匹配为 404，不能核实查询为 503。响应 `Cache-Control: no-store`，错误不回显数据库错误、凭据或存储地址。

上下文返回白名单字段：数据集身份、名称、类型；版本身份、版本号、状态、行数、创建时间；服务端已验证模板的名称、版本、hash、workflow hash 和输入要求。无 storage URI 或任意 metadata。

`executionReadiness=NOT_EVALUATED` 固定表示未进行执行门禁。待核验项包括同批三 RAW、对应实体解析输出、固定输出绑定和当前执行授权。这里不另建一套 readiness 判断。

## Reuse Check

1. 复用 readmodel 与 Dataset PostgreSQL 仓库，Quality 报告/尝试接口、现有只读传输层及控制台 UI；新增的仅是授权查询组合与白名单 DTO。
2. 通用目录 HTTP 读取并不能建立这次 Agent 试点所需的可信主体边界，不能把浏览器传入 workspace/actor 当身份。因此增加受保护的查询组合，不向全部目录请求广播服务 token。
3. 身份继续采用项目现有受控试点 resolver，不自建 IAM。标准 OIDC 的多用户身份映射已列入后续 Q9；本次不引入平行身份组件或额外部署成本。
4. 不新增运行时依赖，不引入 LangGraph、Temporal 等通用执行设施。Q1 不执行工作，无需引入新 runtime；后续仍先复用已有 outbox/queue/worker。

发现一处现有启动耦合：`cmd/api/main.go` 在注册任何查询前就要求对象存储初始化及 Redis 探活。后续真实 Core 联调已通过独立依赖完成完整 API 启动，但启动会创建空测试 bucket、入队健康探针，不能称为全部基础设施零写入。Q1 没有为规避依赖另建服务；如果产品要求只读控制台在存储或队列故障时继续可用，应单独评估启动依赖和 readiness 的分离。

## 验证与证据边界

| 验证 | 结果与范围 |
|---|---|
| Go Agent HTTP | PASS：身份检查早于仓库访问；缺凭据/错误凭据/缺 capability/其他空间；ID 不匹配、无效分页、错误脱敏、POST=405 |
| 真实 PostgreSQL | PASS：应用查询及真实 HTTP handler + 现有仓库组合；工作空间隔离、分页、旧版本身份、响应白名单 |
| 数据库只读 | PASS：查询连接启用 `default_transaction_read_only=on`；相关 workspace Audit/outbox 未新增 |
| 既有能力回归 | PASS：principal、Quality HTTP、Remediation application/domain/HTTP |
| Go 静态检查与构建 | PASS：`go vet ./internal/agentquality/... ./cmd/api`；API 二进制构建 |
| 前端新增回归 | PASS：10 个用例，含精确旧报告在 history/attempt 失败时仍可读、错误身份不替代、凭据不匿名降级、分页保留选择 |
| 前端完整构建 | PASS：包含现有各模块回归、Next 编译、类型与 lint；Agent 两个路由均为动态服务端读取 |
| 浏览器测试夹具 | PASS：18 个 Node fixture 用例；覆盖授权上下文、26 条历史/发现项分页与精确尝试恢复。这只证明测试夹具，不证明生产 Core 运行 |
| 浏览器实际交互 | PASS：14 项隔离验收。工作台/来源/旧报告、前后分页/刷新、尝试核实、局部 UNKNOWN、归属拒绝、无效引用、键盘与三个响应式断点、浏览器凭据隔离和 standalone 启动均已核验 |
| 真实 Core 只读联调 | PASS：真实 API + PostgreSQL，10 项 API / 11 项浏览器；52 次 Next→Core GET，208 表计数和内容 hash 无变化；Core 实际使用 SELECT-only role |
| 全栈保留环境/模型/真实工作执行 | NOT_RUN：不属于 Q1 范围 |

最初的 Repository/HTTP 数据库测试使用独立临时 PostgreSQL 16 容器和合成数据，无现有卷挂载，结束后该临时容器已移除。后续真实 Core 联调另建独立 Compose 依赖，当前保持运行以供预览；没有连接保留业务数据库、对象存储或 worker。

## 复验入口

在 `apps/platform`，配置指向**独立、已执行当前 migrations 的测试 PostgreSQL** 的 `TEST_POSTGRES_DSN` 后运行：

```powershell
go test -count=1 -v ./internal/agentquality/...
go test ./internal/platform/principal ./internal/quality/transport/http ./internal/quality/remediation/... ./cmd/api
go vet ./internal/agentquality/... ./cmd/api
```

在 `apps/web`：

```powershell
npm run test:agent
npm run typecheck
npm run lint
npm run build
```

仓库当前未跟踪 npm lockfile；本次使用现有 package.json，通过 `npm install --no-package-lock` 安装，未调整既有依赖版本。新增测试已纳入 build。

实际预览需由部署端设置 `PLATFORM_API_BASE_URL`、`POC_WORKSPACE_ID` 和既有 `HUMAN_DECISION_API_TOKEN`。只读页面不需要开启任何 `POC_ENABLE_*_ACTIONS`。凭据仅在 Next 服务端传输，浏览器只提交所选对象 ID。当前主体是部署端受控试点服务主体，不代表多用户登录。

原先的直接启动命令被自动审批拦截，随后明确的只读启动器成功运行。原合成 HTTP 夹具预览仍为 `http://127.0.0.1:4186/agent`；真实 Core 只读预览为 `http://127.0.0.1:4187/agent`，用独立 Next distDir 保留原预览。各自的启动/停止命令见对应验收记录。

验收中补齐了折叠菜单的无障碍名称及提示，让尝试核实保留历史和发现项分页位置，并修复真实 Core `FAIL` 状态使用中性 badge 的显示问题。Q2 委托与计划持久化仍待后续实施。
