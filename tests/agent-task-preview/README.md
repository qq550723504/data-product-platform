# 默认只读的独立验收预览

当前入口：<http://127.0.0.1:4188/core/releases>（本机只读页面）及网关 <http://127.0.0.1:4189/>。网关继续使用原有本机合成认证，凭据不在文档或验收日志中展示。生产 Agent API 不变；专用预览保留 Agent 路由，但默认在 Service/事务之前拒绝命令。

## 隔离与只读边界

- 保留原 `agent_q2_test`，不迁移、不授予权限、不重跑 seed，不停止承载 tmpfs 的 PostgreSQL。
- 显式使用新库 `agent_q2_acceptance_20261024` 和新角色 `agent_q2_reader_20261024`。准备程序拒绝覆盖已存在的库/角色。
- 复用当前 Migration Runner 和 Evidence Snapshot 实现；准备阶段仅插入标识为 syntheticAcceptance 的历史夹具，不调用发布/认证/质量执行命令、模型、引擎、worker 或交付。
- 运行角色只对读取端点所需的显式表清单授 SELECT，无表/列 DML、DDL、sequence、TEMP 或 public 函数执行权限，角色及连接默认 READ ONLY。不授 Q2 planner 的协调锁写权限。
- 启动脚本强制关闭 UI/BFF 与 API 写开关；host 本身默认关闭 `AGENT_TASKS_WRITE_ENABLED`，无隐式 DSN、无启动迁移/seed，且拒绝原库。Caddy 拒绝非 GET/HEAD。仅开启 UI 不会授予身份或后端权限。
- 每次命令 HTTP 探测都只验证拒绝，不调用业务 Service；GET/API/浏览器窗口前后对独立库及原库做内容计数/摘要对比。

## 一次性准备（仅明确授权后）

现有容器必须存活，归属标签和 loopback25494 必须吻合。先构建两个独立工具，再运行准备脚本：

```powershell
Push-Location apps/platform
go build -o ../../.data/agent-q2-preview/agent-preview.exe ./tests/agentpreview
go build -o ../../.data/agent-q2-preview/readonly-prepare.exe ./tests/readonlypreview
Pop-Location
python tests/agent-task-preview/prepare-readonly.py
& ./tests/agent-task-preview/build-web.ps1
& ./tests/agent-task-preview/start.ps1
```

准备时使用既有容器的管理员身份，仅针对新库安装当前 DDL、合成夹具和最小角色。生成的随机角色口令仅存于被 gitignore 的 `.data/agent-q2-preview/readonly-profile.json`，Windows ACL 限当前用户和 SYSTEM；不输出 DSN/口令。失败状态保留，不自动删除/重置/重试准备。库已准备后只重用 profile，不再次运行 prepare。

## 验收与停止

- `go test ./tests/agentpreview ./internal/traceability`：默认关闭、拒绝原库、HTTP 命令在 nil Service 前拒绝、空映射历史 JSON 为数组及 immutable decision 去重。
- `readonly-acceptance.test.mjs`：使用调用者显式提供的本机验收凭据环境变量；检查 Agent/context/quality/history GET、精确发布快照、安全投影、401/403/404/400，以及 API/BFF/gateway 拒绝 POST。
- Edge 通过发布历史筛选→精确 Trace→展开快照/数据版本→质量与认证→历史回跳/刷新，另核 Agent 保存按钮禁用和缺失引用 UNKNOWN。浏览器仅操作 GET 表单与导航。
- 准备阶段与读取阶段分开记录。合成 PUBLISHED 行不是平台完成真实业务发布；Execution、provider、dispatch 始终未运行。

停止只操作本轮记录的 API/Next/gateway，保留两个数据库及全部历史：

```powershell
& ./tests/agent-task-preview/stop.ps1
```

旧 Q2 可写验收记录见 `docs/product/agent-quality-q2-implementation.md`，不代表当前预览仍开放写入。当前事实以本 README、代码开关和精确验收记录为准。
