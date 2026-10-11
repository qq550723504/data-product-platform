# External Integration Core — #84 merge contract

本文件拥有 #84 的模型、Command、并发和 HTTP contract。上层定位以 [NDI 集成架构](ndi-integration.md)、[ADR-0009](../adr/0009-national-data-infrastructure-integration-boundary.md)、Issue #84 / parent #47 为准；ProductRelease、Rights、Evidence 的专门契约继续拥有各自规则。

以下状态表与验证结论记录 #84 的历史基线。当前 #86 的 adapter、execution/attempt/cost/reconciliation、verified REGISTERED/PUBLISHED 增量由 [External publication execution](external-publication-execution.md) 唯一拥有；不要把 #84 当时的不可达状态或无远端调用结论应用到 #86。

## Goal / Reuse Check

平台记录已发布 Core ProductRelease 向多个外部 provider 的独立发布意图及身份/标识关联，内部 UUID 保持权威。当前 main `3a59b591` 没有这三种业务事实；metadata/resource_binding 是可变治理投影，annotation engine binding 是专属运行时协议，均不能充当通用外部发布历史。

复用现有 pgx、transaction.Manager、command_idempotency、principal.Resolver、Evidence/Audit/Outbox 和 routing；不增加 SDK、IAM、queue、worker 或 durable workflow runtime。现有 Asynq/Outbox 在 #86 执行期复用。OIDC / [Keycloak 官方文档](https://www.keycloak.org/documentation) 已提供通用身份认证能力，[OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html) 拥有 issuer/subject/token 验证语义；本模块仅记录可信内部 actor 与外部身份的业务关联，不实现这些能力，也不新增 Keycloak 部署。Core 发布引用/历史纠错不能由 IdP 或治理目录替代，属于平台业务语义。无新增 OSS、Fork 或运维组件。

## Frozen scope and facts

- ExternalIdentityBinding：immutable `workspace_id/provider/subject_type=PRINCIPAL/subject_id/principal_ref/external_subject_id/basis_evidence_id`。subject_id 必须是 Resolver 返回的 actor UUID，principal_ref 取可信 Subject；不是请求提供的另一个用户。当前 main 没有 Subject/Organization 注册表，因此组织和任意第三方主体不在本次可达范围。关联状态 BOUND 只代表平台记录了映射，不证明外部登录、身份真实性或注册成功。
- ExternalIdentifier：immutable provider + object_type/object_id + external_id + basis_evidence_id。SUBJECT 引用同 provider/workspace 的 binding，并保留 canonical actor UUID；DATA_RESOURCE / DATA_PRODUCT / PRODUCT_RELEASE 使用专门 FK 列。CONNECTOR 缺少内部 owner 模型，当前拒绝；不创建虚构 connector。provider key 是部署/实例命名空间的受控标识（`[a-z][a-z0-9._-]{0,63}`），不是自由 URL。外部 ID 是 opaque 非凭据标识，不建立数据库 FK，不获取 URI，不保存 credential、任意 metadata 或 raw provider payload。
- Identity/Identifier 纠错：追加 INVALIDATED disposition，原行不 UPDATE/DELETE；新映射用新 identity/外部 ID。历史 external ID 不允许重新指向另一内部对象；本次没有自动重分配或 supersede 推断。Query 根据显式 disposition 返回 current 状态。
- ExternalPublication：immutable workspace/provider/binding/ProductRelease reference；每个 `(workspace,provider,release)` 一条意图，一 release 可多 provider。仅 PUBLISHED release 可创建，保留既有 release 的 immutable identity，不复制或修改其内容。publication status/revision 是可变 projection；创建与每次迁移追加 immutable transition，包含 principal、actor、Evidence、event identity 和时间。

| State | 事实来源 | 本次 Command / 可达性 |
|---|---|---|
| DRAFT | Core 接纳发布意图 | CreateExternalPublication，revision=0 |
| SUBMITTING | Core 明确请求后续外部提交 | RequestExternalPublication，DRAFT→SUBMITTING、revision=1；尚无 remote dispatch |
| REGISTERED | 经过验证的 provider 注册结果 | #86；本次不可达 |
| PUBLISHED | 经过验证的 provider 发布结果 | #86；本次不可达 |
| SUSPENDED / WITHDRAWN | Core 请求及已确认外部执行结果 | 保留状态；未来独立 Command/containment contract，不把本地状态更新当 remote containment |

SUBMITTING 是未执行的 durable intent，#84 不宣称已排队调用 provider。ExternalPublicationRequested 在本版本为显式 retention-only；#86 必须以新的 routing obligation/claim/reconciliation 接纳这些待执行意图，不依靠已经完成的 retention event 自动获得执行保证。没有 generic PATCH、人工 mark-registered/published、失败占位成功或外部重试接口。provider failure/attempt/cost 属 #86；此增量不发生可能计费的远端操作，不凭空记录 CostEvent。任何失败都不能修改 Core release。

## Boundaries / concurrency / idempotency

所有 Command 和 Query 要求 Resolver 已认证 principal，workspace 一致且有 EXTERNAL_INTEGRATION capability；HTTP 忽略不了未知字段，而是拒绝 actor/consumer/status 等伪造字段。身份关联仅本人，workspace 操作者可管理本 workspace 资产/发布意图；这不是产品 owner 的法律权利或 delivery authorization。此模块不返回数据或 credential、不绕过 CurrentDeliveryGate。

Evidence basis 必须已经存在于同 workspace，关联记录是 caller asserted，basis 不自动等于可信 provider proof。每个 accepted command 事务内写 business fact/transition、独立 Evidence + relation、Audit、Outbox、request fingerprint receipt；任一步失败全部回滚。请求 fingerprint 包括全部业务字段及可信 principal/actor，排除 trace/time/random ID。same key 同 fingerprint 返回原 result；changed request 409。重复自然身份/对象关联用另一个 key 不静默成功；同 publication 的新 key 也不新建第二份意图。

固定锁顺序：command-key advisory lock → identity binding parent（如有）→ immutable Core object reference → publication parent。invalidating binding 与 create/submit 在同 binding row lock 串行化；submit 再次要求 BOUND，不能使用旧 HTTP precheck。transition 与 projection 同 parent 锁/事务提交，deferred constraint 确保 revision/status 引用确切 transition；历史 facts DB-level UPDATE/DELETE fail closed。跨 workspace references、非 PUBLISHED release 和 mismatched provider 在 Repository/Application 与 DB guards 双重拒绝。down 先 ACCESS EXCLUSIVE 锁全体新表，再检查任何业务历史；非空时拒绝 DDL，空库允许 down/up。无假想 legacy shim。

## HTTP / read model

`/api/v1/workspaces/{workspaceId}/external-integration` 下：

- POST/GET `identity-bindings`，POST `identity-bindings/{id}/invalidate`。
- POST/GET `identifiers`，POST `identifiers/{id}/invalidate`。
- POST/GET `publications`，GET `publications/{id}`（含 ordered transitions），POST `publications/{id}/request`。

写请求必须有 Idempotency-Key；identity/identifier 创建提供 basisEvidenceId。publication 创建提供 provider、identityBindingId、productReleaseId。读取按 workspace，collection 使用 bounded limit/offset；无匿名查询或凭据输出。缺少 Resolver 配置返回不可用，未认证 401、权限 403、foreign/absent 404、冲突 409、非法输入 400、依赖故障通用 503。复用现有 controlled-pilot static resolver，单独 EXTERNAL_INTEGRATION_API 配置，不扩展 HUMAN_DECISION 的权限。生产 IAM 是独立 owner 工作。

配置 `EXTERNAL_INTEGRATION_API_ENABLED`（默认 false）、`EXTERNAL_INTEGRATION_API_TOKEN`、`EXTERNAL_INTEGRATION_API_SUBJECT`、`EXTERNAL_INTEGRATION_API_ACTOR_ID` 与逗号分隔 `EXTERNAL_INTEGRATION_API_WORKSPACE_IDS`。enabled 时缺失/非法配置启动失败；subject 缺失复用 Resolver 的 actor UUID 默认，不能用调用方 header/body 替换。secret 只驻留 server 配置，不进入 Core facts、日志或 Query。`request` POST body 为 `{}`，非法额外字段拒绝。

## Acceptance / blockers / follow-ups

必需：Domain transition/input tests；真实 PostgreSQL 000055 空库 up/down/up、历史 down 拒绝；binding/identifier immutable 与跨 workspace references；请求 concurrent/replay/conflict；identity invalidation vs submit 两种锁先后；only PUBLISHED；一 release 多 provider；projection/history 一致；Evidence/Audit/Outbox 原子回滚；HTTP trusted actor、查询隔离和不可达状态；Core release/content/membership 摘要不变。完整 Go tests/vet/build 与必要 routing checks。

阻塞仅限 AGENTS §17：授权泄漏、历史丢失、权威冲突、必需路径/build/test 失败、破坏上述 invariant 的并发/幂等缺陷。#86 transport/provider dispatch/callback/poll/reconciliation/physical attempt cost；#87 usage/runtime ingestion；组织/connector owner、外部 identity verification、真实 NDI/TDS API/credentials、billing、delivery、生产部署/真实数据均为后续。本地验证与 exact candidate CI/review/main CI 分开，未推送不能报告后者 PASS。

## Ownership checkpoint

本聊天为 #84 唯一 writer，worktree `issue84-external-core` / branch `codex/issue84-external-core`。#307 原 writer 已占用 000054；本次 000055 只依赖 merged main，不继承其未合并 contract。#294 / Draft #295 的 000050 与暂停联调不动。bootstrap/config/routing 最小接入在 rebase/merge 前按最新 main 串行整合；迁移编号必须再次核对。
