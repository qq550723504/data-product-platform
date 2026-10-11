# External publication execution — #86 merge contract

## Goal / authority / ownership

在 merged main `21762737a76033667df4a138368070868a34fbf8` 的 #84 immutable associations / publication intent 上，完成可信 caller 的显式执行、单 owner dispatch、失败/未知恢复、verified REGISTERED/PUBLISHED projection 和可审计 physical invocation 成本。权威为本文、[External Core](external-integration-core.md)、[ADR-0009](../adr/0009-national-data-infrastructure-integration-boundary.md)、AGENTS §1A/3/7/16/17 与 #86。专门 Release/Rights/Delivery contract 继续拥有其边界。

本聊天唯一 writer，复用已有干净隔离 worktree，branch `codex/issue86-execution`；主 checkout 不切换/修改。#307 owns 000054，#299 owns 000056；本次 000057 不继承它们未合并的代码。shared routing/cost guard/API/worker 在未来合并前按 latest main 串行整合。交付 Draft PR，不 Ready/merge/deploy/close Issue。

## Reuse / Build-vs-Buy

已有 PostgreSQL + transaction.Manager 的跨事务 advisory ownership、command_idempotency、Evidence/Audit/CostAllocation、Outbox obligations/dispatcher、Asynq queue/worker 和周期 reconciliation 已足够。现有 ingestion/delivery/annotation attempts 拥有各自业务对象，不能把外部 catalog publication 冒充这些对象；复用其通用运行时与事实模式，不复制另一套 scheduler/账本。新增的是平台 publication attempt/call/observation 语义。

[Asynq](https://github.com/hibiken/asynq) 已在 go.mod 固定 v0.25.1、MIT，提供至少一次任务与恢复；业务幂等仍由 PostgreSQL claim/facts 保证。成熟替代 [Temporal](https://docs.temporal.io/workflow-execution) 能提供 durable execution，但当前有限 submit/poll 不需要第二套部署/运维、workflow history 或 runtime。无新 OSS、SDK、fork、IAM 或 queue。

## Ports / adapter / current authority

- `ExternalIdentityProvider.CheckIdentity` / `ExternalIdentifierProvider.CheckIdentifier` 验证 #84 已记录的 external subject / PRODUCT_RELEASE identifier；不重新实现身份认证或任意注册服务。
- `CatalogPublisher.Submit` / `Poll` 以 frozen request key/context hash 执行同一个 catalog operation；共享 contract 要求单次调用不隐式重试、持久 key identity、权威 poll、ABSENT/FAILED 证明无活跃 operation。未知/超时不能冒充 ABSENT。
- `DeliveryConnector.PrepareDelivery` 是独立 Port；本次 reference capability 明确不支持 data delivery，必须无 I/O 地返回 unsupported。不输出数据、URL、token 或 credential，不把 catalog PUBLISHED 当 DeliveryOperation/交付授权。
- immutable registry entry 包含 provider/instance/adapter revision 和真实 capabilities；不支持/未配置必须可查询并 fail closed。没有正式 NDI specs/testbed，因此 reference HTTP adapter 仅为明确启用的 loopback 合成协议；正式 NDI implementation 不在本 PR。
- 所有 Command/Query 经既有 principal.Resolver。后台保存 actor/subject/workspace，不保存 bearer；每次 I/O 和 projection finalize 经 Authority Port 重新验证当前权限。受控 pilot Authority 复用现有 static resolver 的 server config，每次读取当前配置；生产 IAM 仍独立，不把配置 principal 当生产 IAM。

## Frozen request / attempts / lifecycle

执行前必须有当前 BOUND identity 和同 workspace/provider/release 的 RECORDED PRODUCT_RELEASE identifier，Core release 必须 PUBLISHED。request 仅包含 typed Core UUID、外部 opaque IDs、provider/instance/revision/key/hash；不含数据正文、storage URI、凭据、raw provider payload、任意业务 metadata。

显式 Prepare/Retry 创建 immutable logical attempt，冻结调用人、request context/bytes/hash 和新的 provider request key；same-key replay 原 receipt，changed fingerprint 409。Retry 只在前一次 definite FAILED、且证明没有活跃远端 publication 时创建新 attempt；UNKNOWN、pending、已确认发布、未确认 containment 不能新 submit。Attempt start、physical call start/dispatch、observations、terminal resolutions 全部 append-only，不覆盖首次失败/未知。一个 attempt 的 SUBMIT durable claim 最多一次。

`DRAFT → SUBMITTING` 继续由 #84 Request 拥有；#86 仅在验证 receipt 的 provider instance/key/context hash、external identifier 和 remote identity 后，经显式 Apply observation 将 `SUBMITTING → REGISTERED → PUBLISHED` 写入 projection + immutable transition。PUBLISHED terminal，不允许 generic PATCH。失败留在原 publication stage，追加 attempt resolution；更晚 attempt 不改旧历史。SUSPENDED/WITHDRAWN 未在 #86 当前 Goal 要求启用，仍 reserved，需要未来独立 Command / provider containment contract。

远端返回/timeout 后先保存 sanitized observation，之后再 fresh-authorize / 检查 current bindings 并应用 projection；projection/event 故障不丢 provider result。若远端已发布但当前权限/绑定失效，保留观察并 BLOCKED，不释放任何数据/credential，不盲重发。remote catalog visibility 与 Core workflow projection 分开解释。

## Dispatch / recovery / locking

#84 的 Requested event 仍保持历史 retention-only，不回写旧 obligation。Prepare 发新的 `ExternalPublicationExecutionRequested` → existing Asynq handler。worker 的 bounded reconciliation 同时认领 SUBMITTING/no-attempt intent 并扫描未终结 attempts；queue 与 reconciliation 都走同一个 PostgreSQL session ownership key，不形成第二套 runtime。Redis outage/丢任务、worker crash、丢 callback 由 DB scan 恢复。

fixed locks：command key → binding → Core release → identifier → publication → attempt/call。复用 transaction.Manager.WithAdvisoryLock 与 worker 已有独立 lock pool，避免持有全部 business connections 后等待自身 Repository。跨 remote I/O 不持有 row transaction。SUBMIT claim 已提交但没有可用 observation 时只能 Poll；即使可能 crash-before-dispatch，也不盲重发。每次 Poll 是新 physical call / 独立成本。

callback 在本次只是经可信 workspace caller 的 reconcile wakeup，不接纳 callback body 声称的 provider state；provider state 必须由 registered adapter 的 authoritative Poll 验证。错误/mismatched/unknown receipt 记录平台固定 error code；raw diagnostics 不进 Core fact、Audit/HTTP。

Reference adapter 的 frozen revision 为协议版本 + loopback origin 的 SHA256；端点改变即视为新 adapter identity，即使配置复用相同 instance 名，也不能替旧 attempt 查询另一个服务。每次 wakeup 最多一次 catalog Submit/Poll；REGISTERED 后等待下一次唤醒。扫描仅覆盖已配置 providers，并用旋转游标处理 bounded 25 条，避免长期 UNKNOWN/已失效 caller 挤掉后续意图。游标是遍历优化，PG facts/ownership 是恢复与幂等依据。

显式 reference 配置：API 与 worker 使用相同 `EXTERNAL_INTEGRATION_API_ENABLED/TOKEN/SUBJECT/ACTOR_ID/WORKSPACE_IDS` 可信 pilot 设置；另设 `EXTERNAL_INTEGRATION_REFERENCE_URL=http://127.0.0.1:<port>`（不含 path/query/userinfo）和非空 `EXTERNAL_INTEGRATION_REFERENCE_INSTANCE`。provider key 固定 `reference`。默认 registry 为空。loopback provider 需在该进程所在网络命名空间；合成协议只接受 metadata POST `/identity/check`、`/identifier/check`、`/catalog/submit`、`/catalog/poll`，body 为 typed request + requestHash，response 为严格有界的 ProviderResult。

## Costs / atomic facts / HTTP

每次实际 adapter invocation 先提交 immutable call start，adapter 在外部 I/O 前调用 durable dispatch marker；marker + quantity=1/unit=invocation/amount=NULL/ACTUAL CostEvent + typed `CostAllocation.external_provider_call_id` + Evidence/Audit/Outbox 同事务。marker 失败必须 0 provider I/O；start-only 不伪装已发生调用成本，派发后失败/超时/lookup 也不丢成本。marker replay 不二次 I/O/计账；adapter 内部不自动 retry。成本证明实际调用尝试，不证明 wire bytes 或 provider invoice 金额。

新增 `/publications/{id}/execute`、`retry`、`reconcile` 显式 POST（严格 `{}`、Idempotency-Key），以及 workspace-scoped bounded attempts/calls 与 registry capabilities Query。Default no adapter = unavailable/unsupported，不伪造成功。Command/fact/transition 与 Evidence/Audit/Outbox/receipt 原子；成本按 physical identity，不按顶层 publication 合并。DB guards 验证 typed workspace/references、历史 immutability、projection/observation lineage；down 锁全部相关历史表后 fail closed。

## Required validation / blockers / follow-ups

Domain/Port shared contracts；真实 PG full migrations + 57 empty down/up/history down；same-key/concurrent Prepare、single submit、多 provider、失败→新 attempt、timeout/unknown→poll、不重复 submit、provider-success/Core-fault recovery、missed wakeup、changed authority/bindings/provider instance、wrong receipt/tenant、physical start/dispatch/failed/poll cost identities、原 Release/content/evidence snapshot digest 不变；strict HTTP and Asynq/Redis replay/fault contract；full Go/vet/build/race/diff。

AGENTS §17 blocker：authorization/data/credential leak、历史/成本丢失或覆盖、authoritative contradiction、必需路径/build/test failure、破坏声明并发/幂等 invariant。formal NDI/TDS/exchange、production IAM/real credentials、billing、usage #87、data delivery、suspend/withdraw 和生产 rollout 为 follow-up。fake/reference/CI 与 real-provider/browser/product acceptance 分开报告。
