# 新版治理原型与 Core 试点：评审 / 合并契约

日期：2026-10-24。分支 `codex/agent-quality-readonly`，独占 checkout `C:/Users/Henry/.codex/worktrees/aq1`。基础提交 `f7a90022aad5da4d67ea594ccb86f8b9925a3840`（#358）；准备时与 origin/main 一致。

## 实际范围与依赖

这是之前多个已批准迭代尚未提交的合并收口，不是只有 Trace 的小改动：

1. 重新设计的全站演示壳、菜单与页面、需求→方案→任务→报告、标准/模型引用、整改/消息回跳、认证/发布/交付/验收归档、管理与空间演示。状态只保存在独立浏览器演示域，不表示 Core 已有同名业务成果。
2. Agent Q1 质量上下文与报告读取；Q2 委托、冻结计划、人工确认、意图受理与暂停/停止的真实服务端持久化，含 migration 73、默认关闭的生产写开关、既有 outbox 和幂等/CAS 边界。意图不是执行，未注册派发消费者。
3. 新版 Core 资产、认证历史、发布历史、明确质量报告的证据/审计/成本，以及发布证据链。复用既有领域查询，补 principal/workspace 校验，未知独立展示。
4. 独立本地验收 host、合成数据库夹具和浏览器/API验收脚本；这些文件用于复现，PR 创建/CI 不启动本机预览、不执行本机迁移或部署。

Trace 页面依赖尚未入主线的新版壳、Core 历史页面和质量查询。Q2 依赖本分支的 Q1 查询与既有 Remediation 只读准备。此 PR 保留上述前置工作，不能将其描述成“仅 Trace 修复”，也不能以 Trace 单测代替整套 Agent 与演示流程验收。

## 权威来源

- AGENTS.md：Core/Adapter、历史事实、workspace、安全边界及合并规则。
- `docs/architecture/agent-quality-backend-contract-v1.md`：Agent 应用事实与现有业务 owner 的边界；Q3/Q4 提案不构成本 PR 已实现能力。
- `docs/product/agent-quality-q2-implementation.md`：Q2 实际实现、非目标和既有验收；migration 73 与 Agent domain/application 定义实际持久化规则。
- `docs/product/governance-backend-integration-map.md`：演示与 Core 真实查询的边界。
- `docs/product/governance-release-trace-readonly-20261024.md`：最新 Trace 实现与尚未完成的运行/浏览器验收。

## 必须保证

- 模型、实际 Agent dispatch、生产/评测执行、发布、认证与交付写命令不因 UI 迁移自动开启；Q2 写入仅限明确授权的计划事实。
- 服务端凭据不进入客户端包；浏览器 actor/workspace 字段不能产生身份。匿名写入不使用部署读凭据代办。
- 计划修订/确认/意图与收据、证据、审计、outbox 同事务；不可变计划成员冻结与并发写使用同一 parent lock。
- 明确版本、评测、发布和历史引用不能回退最新；演示状态不能替代后端未知或失败。
- Trace 根对象、返回成员在同一 PostgreSQL READ ONLY / REPEATABLE READ 快照核实归属，越界 fail closed；只投影允许字段。
- 不替换现有领域事实 owner；不引入第二套队列/工作流/IAM，不把历史认证/发布当当前交付权。

## 非目标

真实模型、Q3/Q4 owner receipt/执行闭环、标准 SSO/IAM、真实业务写命令、部署、本地运行更新、数据清理及 #332 工作均不包含。

## 合并门禁与当前状态

**当前是 Draft / 不可合并检查点。** 本地源码、离线回归与既有浏览器证据不能替代以下门禁：

- 锁定精确 PR HEAD，必需 CI 全部完成且通过。
- 独立复核完成；安全、历史一致性、必需路径和并发/幂等 blocker 修复且再次核验。
- 最新 Trace 的现场 API 和浏览器验收完成。目前均 NOT_RUN；今晚运行边界禁止为了补验收而重启预览或启动新 GUI。此阻塞不能因 CI 通过被忽略。
- 保留工作空间没有发布记录；正向 Trace 浏览器验收需明确隔离合成夹具，不能偷偷补造历史。真实 PostgreSQL 正向测试不是浏览器通过。
- 运行与浏览器门禁解除并完成前，保持 Draft，不开 auto-merge。未来如合并，另核实际 merge SHA 对应 push/main CI 终态；PR CI 不等于 main CI，不构成部署许可。

本地回归证据记录在各批次验收文档和被 gitignore 的 `.data/` 日志中。未运行的项目保持 NOT_RUN，不上传凭据或原始运行数据冒充可复现证据。


## 首轮 CI 收口

- Vet 指出跨包 Action 的无字段名初始化，已改为显式字段名。
- 新增 Agent DatasetVersion FK 必须接入现有生命周期准入 fence。migration 73 为 source/input 关系安装现有 admission guard，保留全量 FK 发现与 unreleased demand；Q2 没有释放命令，不将 Agent 引用排除，不放宽 UNKNOWN。新增真实 PostgreSQL 测试检查两个关系各自的内容 pin。
- 全路径 rewrite 会吞掉实际 Core 深链，属于实现缺陷。收敛为只重写根演示页，现有服务端业务路由在同一新版壳内保留；Core 工作台明确位于 `/core/workbench`。原浏览器契约只调整工作台入口，保留原断言。发布 Trace 的断言随新安全投影改为明确决策 ID、快照/哈希及禁止原始自由文本，历史事实要求不削弱。
- 上述修复仅改源码及离线/CI测试；本机不执行 migration，不重启预览，不启动 GUI。新增现场 Trace 验收仍 NOT_RUN。


作者复核另修正两个历史展示问题：映射列表按 immutable decision ID 识别，允许同一 mutable mapping 的不同真实决策同时出现；冻结快照成员与后来追加的关联证据分别展示，不能把整个关联证据列表冒充发布时冻结 membership。这不是独立评审 PASS；仍需完整范围独立评审。


后续 CI 的 action trace 显示服务端已返回成功/明确拒绝及完整 Flight，而嵌在 client-only dynamic 壳内的已连接页面仍保留 pending/旧事实。已连接壳改为稳定、可 SSR 的客户端边界，直接保留 Next router/action children；只有纯演示页使用浏览器存储 provider 与 `ssr:false`。Core 不监听演示 hash、不按演示页面 key 重挂载服务端 children。没有增加自动重发或执行重试，也不放宽既有动作断言。另一个 Trace CI 失败是测试未展开数据版本 details 就用可访问性 locator 查找内部链接，测试改为先执行实际展开交互。


整套 Go CI 另暴露两个前置集成缺口：未来 Agent dispatch obligation 不能直接成为当前 worker 的启动必需 handler；现有 dispatcher 的 frozen-obligation 机制已能把未注册 handler 的事件保持失败/未确认。生产者保留完整 obligation，当前 worker 使用明确版本化的已实现消费 profile，仅不声明尚未实现的 AgentActionRequested，其他 route 与 startup 校验不变；不注册空实现，不把意图当 retention-only，不启动新 worker。既有 Redis 故障回归使用同一消费 profile。新编译的 trial-business candidate 严格 schema gate 跟随新增 migration 73，仍拒绝任意旧/新 schema；保留 schema71/72 二进制、运行实例与冻结收据不变，不执行升级或该 CLI。


独立复核入口为协调线程 `01a1699c-721d-7125-8b0c-6b3f734f90d8` 对 `fe0bf8f49bbcd87de63a8c817b07631d80580935` 的局部只读源码评审（不是 GitHub 全量批准）。其新增 P2：Agent 2xx 收据未绑定校验，且确认与后续 GET 共用 catch，可能丢失原键并把已保存误报未知。修复在收到匹配收据前保留原请求；确认后先保存事实和明确 task/plan URL，再独立读取，GET 失败只提供只读恢复并禁用新写。离线回归验证畸形/错绑收据、同键同内容重试、有效确认后两个独立 GET 失败及只读恢复不增加 POST。完整 UI/其余文件仍无独立全范围 PASS，且上述修复后的新 HEAD 仍需复核。
