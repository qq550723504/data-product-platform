# Q2 计划持久化隔离验收

入口：<http://127.0.0.1:4189/agent/tasks>。合成登录 `operator` / `agent-q2-preview`，登录后使用不含凭据的普通 URL。直连 <http://127.0.0.1:4188/agent/tasks> 只能查询，匿名写入返回 401。

这是本机、合成数据、仅计划记录的验收环境。专用 Go host 复用产品 Application、HTTP、principal、transaction、Audit/Evidence/Outbox；只注册 Agent 路由，无模型、业务引擎、worker、Redis 或对象存储。主 API 的 `AGENT_TASKS_WRITE_ENABLED` 和 Next 的 `AGENT_TASKS_UI_WRITE_ENABLED` 默认均关闭。单独启用页面开关不授予身份权限。

## 首次准备

在隔离 checkout 根目录使用 PowerShell。先核实以下容器名称、端口未被其他会话占用；不停止或替换其他实例。

```powershell
docker run -d --name dpp-agent-q2-test-postgres-20261023 --label codex.scope=agent-q2-test --network bridge -p 127.0.0.1:25494:5432 --tmpfs /var/lib/postgresql/data -e POSTGRES_PASSWORD=agent-q2-test-only -e POSTGRES_DB=agent_q2_test postgres:16-alpine
New-Item -ItemType Directory -Force .data/agent-q2-preview
Push-Location apps/platform
go build -o ../../.data/agent-q2-preview/agent-preview.exe ./tests/agentpreview
Pop-Location
$env:AGENT_Q2_PREVIEW_ACK='ISOLATED_PLAN_ONLY'
& ./.data/agent-q2-preview/agent-preview.exe -bootstrap
& ./tests/agent-task-preview/build-web.ps1
& ./tests/agent-task-preview/start.ps1
```

bootstrap 只针对固定的 `agent_q2_test`，使用 migration Runner 应用当前迁移并插入合成 RAW READY 版本及受限角色。seed 不是可重复执行的业务命令；已存在时直接使用，不重置数据。数据库为 tmpfs，容器停止后数据不保证保留。普通 Core/Next 重启不会停止数据库。

Caddy 使用已有 `caddy:2.11.7-alpine` 的标准 basic_auth/reverse_proxy。认证后替换 Core Bearer；Core 凭据不会返回给浏览器。所有端口仅绑定 loopback，属于本地试点身份，不是多人 SSO。不要复用这些合成凭据到真实环境。

## 验收

诊断来源版本：`bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb`。依次新建、确认、修订、再次确认、保存意图、暂停/恢复、停止。历史计划只读；原确认不能用于新计划；保存意图不会调用 Detect。修复绑定入口为详情的“配置修复计划”，复用 revisions / approvals / actions API；填写 case、工作流版本、输出数据集、YYYY-MM 与全部输入绑定。修复确认独立于诊断；当前仅能保存 ASSIGN_SELF 意图。

```powershell
node --test tests/agent-task-preview/api.test.mjs
Push-Location apps/platform
$env:TEST_POSTGRES_DSN='postgres://postgres:agent-q2-test-only@127.0.0.1:25494/agent_q2_test?sslmode=disable'
go test ./internal/platform/migration -run TestAgent -v
go test ./internal/agenttask/... ./internal/platform/config ./internal/platform/routing
Pop-Location
```

迁移测试创建并清理随机 scratch databases，不操作 Q1 保留库。Remediation 的真实 PostgreSQL 回归也可用该测试 DSN 执行，内部使用随机工作空间、内存对象存储和受控 fake engine；其合成 Execution/Quality 数据不代表 Agent 派发。

浏览器验收只能使用 browser/CUA 工具。断网模拟后必须恢复网络。待核实请求刷新后保留原键，不自动新建或派发。API 验收会追加合成 Agent 记录并以 STOPPED/CANCELLED 结束，保留历史。

## 停止与恢复

`stop.ps1` 先核对记录中的 checkout、进程名称、创建时间、监听端口，以及 Caddy 容器 label 和配置挂载，再停止本轮 Core/Next/Caddy。数据库与记录保留。随后 `start.ps1` 可恢复同一预览；不会自动重建数据库或重置历史。

```powershell
& ./tests/agent-task-preview/stop.ps1
& ./tests/agent-task-preview/start.ps1
```

前端使用独立 `.next-agent-q2`，构建恢复原 `next-env.d.ts`，不覆盖 Q1 `.next` / `.next-agent-q1-live`。

## 修复夹具与验收

新版连接预览入口为 http://127.0.0.1:4189/agent，任务详情 /agent/tasks?taskId=明确委托ID，修复绑定 /agent/tasks/repair。工作台、详情与绑定采用已确认的暖白/翡翠绿设计；4178 是独立设计原型，不能作为 Core 保存结果的验收入口。其他业务中心尚未整体迁移。

新库 bootstrap 同时安装 repair-locks.sql：仅供预览的列级锁权限与 owner mutation deny trigger。已存在旧库时，只对本任务拥有的独立验收库执行一次此文件；不得重跑 seed 或重置库。

修复夹具使用既有 Remediation 测试工厂，内存对象存储、受控 fake ingestion/quality 与本地实体测试数据，不接真实外部引擎；不调用任何实际修复 Execution。它会创建合成 Quality/成本记录作为浏览器前置事实，不能将其说成 Agent 执行产物。仅首次准备且确认目标库后运行：

```powershell
Push-Location apps/platform
$env:TEST_POSTGRES_DSN='postgres://postgres:agent-q2-test-only@127.0.0.1:25494/agent_q2_test?sslmode=disable'
$env:AGENT_REPAIR_FIXTURE_ACK='ISOLATED_SYNTHETIC_ONLY'
go test ./internal/quality/remediation/application -run '^TestAgentRepairBrowserFixture$' -count=1 -v
Remove-Item Env:AGENT_REPAIR_FIXTURE_ACK
Pop-Location
node --test tests/agent-task-preview/api.test.mjs tests/agent-task-preview/repair-api.test.mjs
```

.data/agent-q2-preview/repair-fixture.json 保存明确依赖 ID，存在时保留并跳过生成。库若丢失，不得使用旧 manifest 当有效事实，应先验证库与事实，单独准备新夹具。验收前后固定空间 Quality=1、Cost=7、Execution=0、修复 case assignment=0；成本与 Quality 均为夹具准备产生，Agent 办理未增加。

## 全站新版入口（2026-10-23）

当前浏览器入口统一为 http://127.0.0.1:4189/。ConsoleShell 使用同一新版 App，Agent 保留既有 Core API，其他模块只使用本地合成状态，不运行旧业务页面 loader。演示数据在当前标签页 sessionStorage 恢复，可通过“重置演示”清空；Agent 待核实请求与服务端记录不在重置范围内。

导航、迁移边界及验收证据以 docs/product/governance-modern-ui-acceptance.md 为准。非 Agent 已覆盖业务中心、系统设置、空间、来源、批次与实体审核演示。不要把演示空间 ID/版本 ID 提交给 Core；Agent 页面明确显示独立试点范围。
