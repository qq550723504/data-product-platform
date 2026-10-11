# External usage/runtime evidence — #87 merge contract

## Goal, authority and ownership

在已合并 #84/#86 的 main `253ce3b4ef044a233de1179534422eed04ded660` 上，将提供方中立的 usage/access/delivery/connector-log 观察追加到 Evidence Graph。本文、#87、ADR-0009、External Core 和 External publication execution 是权威；生产 Release、Delivery、Rights 与 EvidenceSnapshot 仍由原 contract 拥有。本聊天唯一 writer；复用隔离 worktree，branch `codex/issue87-usage`。000058 由本次拥有，000054/#307、000056/#299/#310 不继承未合并实现。只推送和提 Draft PR，不 Ready/merge/deploy/close Issue。

## Reuse / Build-vs-Buy

复用现有 registry、trusted principal/Authority、PostgreSQL transaction/session ownership、command_idempotency、Evidence/Audit/Cost/Outbox 与 worker 的 bounded reconciliation。没有现成的 external usage fact/correlation/cursor，不能拿 Execution mapping usage、DeliveryOperation 或生产 EvidenceSnapshot 冒充外部观察。

事件身份沿用 [CloudEvents 1.0 的 source + id 去重语义](https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md)；Adapter 负责解析原始协议，Core 仅接收严格 normalized metadata。不是另写 CloudEvents parser、通用日志采集/CDC runtime。OpenTelemetry logs 是成熟的通用日志方案，但 [其 data model](https://opentelemetry.io/docs/specs/otel/logs/data-model/) 不拥有平台 publication/release 关联、隔离与 immutable Evidence 语义；不部署第二套采集运行时。无新 SDK/OSS/fork。正式 NDI 等 Adapter 等待 #47 的规范/testbed。

## Source and normalized contract

- 为 #86 registry 增加可选 `UsageEventSource.Read` capability；不支持必须 fail closed，不影响已有 publication ports。
- CreateUsageSource 冻结 workspace、binding/provider/instance/adapter revision、URN event-source、可信 actor/subject。URN 只是事件命名空间，不能作为可访问 URL。创建与每次 pull 都要求当前绑定和当前可信 caller；不接受外部传入的 Core release/publication UUID。
- Read 是只读、可按同一 cursor 重放的 bounded metadata page，不 ACK、不删除 provider events、不签发数据/凭据；最多 50 events、128 KiB、5 秒，一次调用不隐式 retry/redirect。page 精确绑定 request hash/instance/from-cursor。cursor 不透明，Core 不比较其大小、不用 occurred_at 作游标；非空 page 必须推进 cursor，empty idle page 可以保持 cursor。
- normalized occurrence 保存 source/id/category/occurred_at/external subject/identifier/remote publication identity。四类 category 为 USAGE、ACCESS、DELIVERY、CONNECTOR_LOG；未知 category 进入隔离。无原始日志正文、URL、token、任意 attributes、价格或结算字段。时间统一 UTC/microseconds；事件是 provider-reported observation，DELIVERY category 不证明 CurrentDeliveryGate、ISSUED 或收到 bytes。

## Immutable facts, deduplication and correlation

事件身份为 workspace/provider/instance/event-source/provider-event-id；规范化 payload hash 不包含 cursor、pull 或 ingestion 时间。完全相同重放不再创建 usage/Evidence/Audit；同 identity 不同 hash 追加 conflict quarantine，绝不覆盖原 occurrence 或关联。重复 conflict hash 同样去重。

每个 occurrence 与其初始 ACCEPTED/QUARANTINED decision 都是 immutable fact。唯一的 accepted correlation 必须从同 workspace/provider/instance/revision/binding 的 #86 immutable PUBLISHED observation + published transition、PRODUCT_RELEASE identifier 和 ProductRelease 求得，且三项外部身份逐项匹配。未知/缺失/歧义/subject mismatch 不按 latest timestamp 或客户端 Core ID 猜测。accepted decision 强类型引用 binding/identifier/publication/release/provider observation；其 Evidence 追加 ExternalPublication/ProductRelease 的 DOWNSTREAM_RUNTIME relation，不写生产快照。

隔离 occurrence 可查询；显式 RecheckUsageEvent 仅重新运行同一 server-side matcher，映射仍不可靠返回 conflict，映射已具备则追加 ACCEPTED decision/Evidence/Audit/Outbox。旧 quarantine 不修改，已 accepted 不可重新映射。conflicting payload 可查询但本次不提供人工覆盖历史的 API。

## Pull, cursor and recovery

显式 RequestUsagePull（严格 empty body + Idempotency-Key）冻结 source 的 checkpoint revision/from-cursor；默认不自动开启无限轮询。现有 worker reconciliation 扫描持久 pending pulls；Outbox pull-request event 明确 retention-only，PG work row 是恢复依据，不引入新 queue handler/runtime。same command replay 返回 receipt，changed source/caller fingerprint 拒绝。

复用现有独立 lock pool/session ownership，按 source 串行 remote I/O。固定 command key → binding → source → pull → occurrence 顺序；关联只读取 immutable published facts。每次实际 Read 前先持久 physical call start，dispatch marker + quantity=1 invocation CostEvent + typed CostAllocation/Evidence/Audit/Outbox 同事务提交；调用失败、timeout、commit failure 也保留已发生成本。start-only 不计已调用成本；crash 后只读同 cursor 可用新的 physical call identity 重读，真实重读独立计费。

严格 normalized page 先存 append-only outcome（受控恢复 buffer，不是原始 provider payload），再 fresh-authorize，在 source parent row lock 内验证 checkpoint revision，原子写入 dedup/quarantine/correlation + checkpoint + pull resolution + Evidence/Audit/Outbox。提交故障保留 page，下一次恢复不重新调用 provider。无有效 page 时不得推进 cursor。DB deferred constraint 要求 occurrence 与初始 decision 一同提交；checkpoint guard 验证 page 每个 identity/hash 都已有 canonical decision 或 conflict quarantine，不能跳过 page 内的事件。stale concurrent pull 不接纳旧 page、不回退 cursor；失去 caller/binding/adapter authority 不导入关系。parent-first DB guards 同样串行 checkpoint/acceptance 与 concurrent writes。source/checkpoint/pull/call/dispatch/outcome/occurrence/decision/conflict/resolution 全 append-only；destructive down 先锁 parents/children，存在历史即拒绝。

## HTTP, validation and blockers

workspace-scoped source/pull/call/usage-event/conflict queries 与 event detail（typed decisions/evidence references）；所有路径经既有 principal boundary。创建 source、请求 pull、重新核对隔离事件为显式 Commands，不提供 raw-event webhook/generic PATCH。loopback reference `/usage/read` 仅 synthetic normalized 协议；fake/reference 共用 adapter contracts。

路径均在 `/api/v1/workspaces/{workspaceId}/external-integration` 下；写请求要求 Idempotency-Key，Query 复用 bounded limit/offset（最多 100）。创建 source body 为 `{bindingId,stream}`；pull/recheck body 严格为 `{}`。

| Method | Path | Result |
|---|---|---|
| POST / GET | usage-sources | 创建 source / workspace 列表 |
| GET | usage-sources/{id} | immutable identity + latest committed checkpoint projection |
| POST | usage-sources/{id}/pull | 202 + durable pull receipt，worker 随后处理 |
| GET | usage-sources/{id}/pulls | 当前 source 的 pull history |
| GET | usage-pulls/{id} / usage-pulls/{id}/calls | pull result / physical calls；不返回 page buffer |
| GET | usage-events?status=ACCEPTED\|QUARANTINED | canonical events + current decision projection；provider identity 位于 occurrence |
| GET | usage-events/{id} | canonical occurrence、最多两项 immutable decisions 和 Evidence references |
| POST | usage-events/{id}/recheck | 重新关联已隔离 occurrence；不会重写旧判断 |
| GET | usage-conflicts | conflicting normalized occurrence + immutable quarantine identity |

同一 workspace/provider/instance/stream 只创建一次 source，不支持把历史 source 重新绑定到另一身份。Binding/actor 失效后停止导入；source ownership transfer 是后续显式 lifecycle contract。

必需验证：真实 PG migration up/down/历史 fail closed/concurrency；双 cursor 重放与 id/hash conflict；四种 category、unknown/unmapped/recheck、cross-workspace/provider/instance/subject/remote-id isolation；lost response/start-only/revoked authority/stale concurrent pulls；page saved / acceptance fault recovery；provider physical cost；Release/version/membership 与 EvidenceSnapshot header/root/membership digest 不变；HTTP auth/strict body/bounded queries；full Go/vet/build/race/diff。

AGENTS §17 blockers 为权限/数据泄漏、历史/成本丢失、错误关联、cursor/幂等并发缺陷、权威冲突或必需路径/test failure。Billing/settlement、原始日志搜索、通用 collector、自动持续采集调度、人工重写冲突事件、production IAM、正式 NDI/provider/browser 产品验收和部署是 follow-ups，不把 synthetic fixtures/CI 称为产品验收。
