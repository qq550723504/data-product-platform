# 实体复核 UI —— Issue #75，增量交付

本次变更仅实现 #75 中的实体复核（entity-review）部分。产品详情、独立的发布就绪门禁、发布、证据导航以及完整的参考 POC 浏览器验收测试仍未完成。不要关闭 #75 或 #13。

## 操作员流程

打开 `/reviews`，对照候选 ID 与溯源信息（provenance）比较原始与归一化后的源记录，填写原因（1–2000 字符），然后确认或拒绝。浏览器调用 Server Action。在提交核心命令之前，服务端会：

1. 要求显式启用的 POC 开关、workspace，以及仅存在于服务端的 Human Decision API credential。
2. 拉取该 job，并在 Web 侧做 workspace 预检。
3. 按 `candidateId` 定向拉取该候选（不加载整个 job 的候选列表），并校验候选归属关系与 PENDING 状态。
4. 确认操作必须提供候选实体；拒绝操作绝不创建实体。
5. Web Server Action 使用 Bearer credential 调用既有 `/confirm` 或 `/reject` 命令。Core 根据候选所属 job 的 authoritative workspace 解析 `RequestPrincipal`，校验 workspace + `HUMAN_DECISION` capability，并从 principal 得到 reviewer actor；客户端提交的 `X-Actor-ID` 不参与权威身份决定。状态迁移、映射与证据由 Core 拥有。

返回的核心 job 状态会被展示；队列、工作台与数据集列表会被重新校验。POST 超时被视为结果未知，而不是确证的失败。不会自动重试任何写操作。应刷新并检查后再重试。

队列已分页，且不会把"零待处理候选"等同于"就绪"。为防止大型 match job 在每次决策时重复传输整个候选队列，预检使用 Core 只读端点 `GET /api/v1/entity-match-reviews/{candidateId}` 定向读取单个候选；该端点只返回一个候选，不返回 job 的全部候选负载。

## 本地 / 可信 POC 配置

在 `apps/web/.env.local` 中新增（既有的 API/workspace 配置仍然适用）：

```dotenv
# 默认为只读。
POC_ENABLE_REVIEW_ACTIONS=true
# 仅服务端可见；必须与 Core 的 HUMAN_DECISION_API_TOKEN 一致。
HUMAN_DECISION_API_TOKEN=
```

Core 现在通过可替换的 `RequestPrincipal` resolver 建立最小可信身份边界：credential、actor、workspace allowlist 与 capability 都由服务端配置决定，浏览器不能通过 actor 字段伪造 reviewer。当前 reference resolver 仍只是 controlled-pilot 的静态 bearer 适配器，不等于生产 IAM；生产环境应替换为 OIDC/session 等受验证的身份提供者，而 application/domain 层保持不感知具体 provider。

Next.js 与 eslint-config-next 从 15.2.4 升级到 15.5.24，即 2026-08-25 记录的受维护 15.x 安全版本：
https://nextjs.org/blog/august-2026-security-release
这是一次针对性的依赖更新，并不代表已完成完整的安全审计。基础 web 脚手架中不存在依赖 lockfile。

## 验证

`npm run test:reviews` 会编译与框架无关的边界层，并注入伪造的核心传输层（fake Core transport）运行 Node 契约测试（无需外部服务）。`npm run build` 会在 Next 生产构建之前运行这些测试，因此既有的 web CI 构建也会对它们设置门禁。同时请运行 `npm run typecheck` 与 `npm run lint`。

Web 契约测试覆盖确认/拒绝、客户端 actor 字段不参与身份、workspace/候选作用域、缺失 credential、缺失原因、非 PENDING 候选、缺失实体、畸形响应、409 与超时，以及"预检必须使用定向候选查询而非整个 job 队列"。Core 的 principal 包另有 credential / workspace / capability / forged-header 回归测试。真实 PostgreSQL HTTP acceptance 仍应作为 #214 收口条件。

仍需要针对真实已播种（seeded）POC 做浏览器验收：

- 默认配置显示只读队列；缺少 Human Decision credential 时阻止写入。
- 空/仅空白的原因无法提交；聚焦的控件可键盘操作。
- 确认/拒绝能够到达 Core，并在刷新后移除候选。
- 返回的 job 状态以及（存在时）生成的版本可见。
- 并发复核与超时会给出可操作的提示，而非盲目重试。
- 分页能访问到前 25 条之后的待处理候选。
- 检查 Core 的审计/证据，以核实 reviewer、原因、决策与来源。

独立 Next 构建、lint/typecheck 与浏览器验收必须基于真实运行记录，仅凭 24 个测试无法确立端到端完成。
