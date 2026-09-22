# Outbox 派发与操作幂等设计（C1）

- 状态：**设计 + C1-a/C1-b/T2 已实现，扇出前置（统一 dispatcher）已实现**。C1-a（向前迁移 `000015_outbox_delivery_hardening`：
  `event_version` / `claim_token` / 死信状态 / 可领取索引 / 每消费者确认表）与
  C1-b（publisher：claim token 守卫、指数退避、最大尝试、死信、瞬时错误容错、稳定排序、
  事件版本兼容）已落地并通过回归测试。
  「统一 dispatcher + 版本化路由表 + 每处理器确认」（issue #103）已落地，即 §7.1 选定的模型 A，
  是 C1-d 的**前置**：`worker` 不再直连「publisher + 单闭包 handler」。
  处理义务在写入时就**冻结到事件上**（`000016_outbox_event_routing_obligation`）；
  dispatcher 只消费已冻结义务，不再为未冻结旧事件推导 routing contract。
  T2 已实现 C1-c（Execution 请求幂等）与 C1-d（Execution 入队经 Outbox）：
  `WORKFLOW.CREATE_EXECUTION` / `WORKFLOW.RETRY_EXECUTION` 使用请求指纹，Create/Retry
  只在事务内写入 Outbox，由 `execution-queue` handler 经真实 Redis/asynq 入队；旧的
  当前 Execution 入队事件都冻结为 `execution-queue` 义务；历史 retention-only 对账路径已移除，
  不再维护针对开发阶段旧事件的补派发兼容流程。
- 关联：issue #103（Domain Event / AuditEvent / Transactional Outbox 覆盖缺口）、
  #110（持久化执行 + 入队超时的恢复，禁止盲重复创建）、#100（关键 Command 幂等）；
  AGENTS.md §4（显式 Command）、§5（Domain Event + Audit + Transactional Outbox）、
  §10（外部引擎不作为业务真相）、ADR-0005（领域事件与审计）、ADR-0003（不可变版本）。
- C2（原生执行恢复与输出幂等）另文，本文只在边界处引用。

## 1. 范围与非目标

### 1.1 本文覆盖（C1/T2）

1. Outbox 事件结构、事件版本与状态机。（C1-a）
2. 派发确认规则：claim / 租约 / 退避 / 最大尝试 / 死信 / 租约到期恢复。（C1-b）
3. 操作幂等键的命名与持久化约定（复用 `command_idempotency`）。（C1-c）
4. 把 **Execution 入队** 从「事务提交后直接 `queue.EnqueueExecution`」迁移为
   「事务内写 Outbox，由 worker 派发」的设计。（C1-d）
5. 上述各项的测试与验收计划。

### 1.2 明确不改动（保护边界）

- **不改动共享 Execution 的输入绑定与输出提交语义**：`domain.NewExecution` 冻结
  `Inputs`、native engine 的 `buildCanonical*` / 输出版本写入逻辑、`Succeed` 的
  `outputDatasetVersionID` 语义都不变。C1 只改「派发这一条腿」。
- 不新增通用 `PATCH status`；死信重放只用显式 Command。
- 不新增 broker / 消息中间件；继续使用 PostgreSQL Outbox + Redis(asynq) 队列。
- 不重写已执行迁移 `000001` / `000013` / `000014`；补强一律使用新的向前迁移。
- 原生执行恢复与输出幂等（同一 Execution 重跑、`engine_execution_id` 匹配、输出版本
  去重）属于 **C2**。

## 2. 现状事实（实现前已核对）

### 2.1 表与写入

- `outbox_event`（`migrations/000001_platform_bootstrap.up.sql:3`）：
  `id / aggregate_type / aggregate_id / event_type / payload / status / attempts /
  available_at / created_at / published_at / last_error`；
  `ck_outbox_status` 限定 `PENDING | PROCESSING | PUBLISHED | FAILED`；
  索引 `idx_outbox_pending (status, available_at, created_at) WHERE status IN ('PENDING','FAILED')`。
- `outbox.Append`（`apps/platform/internal/platform/outbox/store.go`）在聚合事务内插入，
  已满足 AGENTS §5「与聚合更新处于同一事务」。
- `outbox.NewEvent` 只有 `ID / AggregateType / AggregateID / EventType / Payload /
  AvailableAt`，**没有事件版本字段**。
- `command_idempotency`（`migrations/000008_release_publish.up.sql:22`，指纹由
  `000017_execution_command_fingerprint` 增加）：
  `workspace_id / command_type / idempotency_key / object_id / result_ref / request_fingerprint`，
  唯一键 `(workspace_id, command_type, idempotency_key)`。

### 2.2 派发

**实现前**（`outbox.Publisher.publishOne`，见 `apps/platform/internal/platform/outbox/publisher.go`）：

1. claim：`SELECT ... WHERE status IN ('PENDING','FAILED','PROCESSING')
   AND available_at <= now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`；
2. 标记 `PROCESSING`，`attempts = attempts + 1`，`available_at = now() + claimTTL(30s)`
   —— `available_at` 同时充当**租约**；
3. 提交 claim 事务；
4. `handler(ctx, event)`；
5. 成功 → `PUBLISHED`；失败 → `FAILED`，`available_at = now() + 5s`。

**已满足**：`PUBLISHED` 的标记严格晚于 handler 成功（#103 中"先派发后标记已发布"
这一历史项在当前实现里已成立）。C1 只补回归契约把该性质锁住。

**C1-b 改动后**：claim 生成新 `claim_token`；终止性回写按 token 条件更新；
失败走指数退避，达到 `MaxAttempts` 进入 `DEAD_LETTER`；
`Run` 对 handler 失败与瞬时 DB 错误退避继续，仅对致命 DB 条件或 `ctx` 取消返回。

### 2.3 现状缺口

| 编号 | 缺口 | 证据 |
| --- | --- | --- |
| G1 | 事件没有 schema/版本字段，payload 结构演进无约束 | `store.go` `Event` / `NewEvent` |
| G2 | 没有最大尝试次数，也没有死信状态，毒事件无限重试 | `publisher.go` 失败分支只写 `FAILED` |
| G3 | 租约到期的回扫路径依赖 `available_at`，但 `idx_outbox_pending` 不覆盖 `PROCESSING` | `000001` 索引定义 |
| G4 | 任何 DB/claim 错误都会让 `Run` 返回，worker 把它送 `errCh` 并触发整体停机，而不是退避重试 | `publisher.go` `Run`；`cmd/worker/main.go` publisher goroutine |
| G5 | 排序只有 `created_at`，无稳定 tiebreaker，也没有 per-aggregate 顺序保证 | `publisher.go` `ORDER BY created_at` |
| G6 | 请求幂等只有 Release 发布使用 `command_idempotency`（`product/transport/http/publish.go:23`），另有映射决策的专用 `entity_mapping_decision.idempotency_key`；Execution 创建没有请求幂等键 | 已由 T2 关闭，见 4.4 |
| G7 | Execution 入队在**事务提交之后**直接调用 `queue.EnqueueExecution`（`workflow/application/execution.go` `Create` 末尾），失败时返回"persisted but enqueue failed"，没有任何自动恢复，也没有经 Outbox | 已由 T2 关闭，见 4.5 |
| G8 | handler 幂等靠各 handler 自己实现（`metadata/application/service.go` `ProjectProductRelease` 用 `governance_projection` 唯一键 + `SUCCEEDED` 短路），没有统一约定与契约测试 | `migrations/000010_metadata_projection.up.sql:35` |

**C1-a/C1-b 已关闭**：G1（`event_version` 列）、G2（`MaxAttempts` + `DEAD_LETTER` + 死信停止自动重试）、
G3（`idx_outbox_claimable` 覆盖 `PROCESSING`）、G4（瞬时错误退避继续，仅致命错误停止）、
G5（`ORDER BY created_at, id` 稳定 tiebreaker）。
**T2 已关闭**：G6（Execution 请求幂等键）、G7（Execution 入队经 Outbox）。
G8 的统一 handler 契约仍由消费方状态保护与本轮 `execution-queue` handler 测试覆盖；
死信重放 Command 仍不在 T2 范围内。

## 3. 目标语义

1. **至少一次投递 + 幂等 handler = 有效一次**。系统不承诺"只派发一次"；重复派发必须安全。
2. **派发成功早于标记 `PUBLISHED`**（保持现状，并加契约测试）。
3. **瞬时失败自动恢复**：DB 抖动、handler 失败、worker 崩溃都在租约/退避后自动重试，
   不导致 worker 停机、不产生悬挂的已持久化未派发事实。
4. **毒事件可诊断、可人工处置**：超过尝试上限进入死信并保留 `last_error`。
5. **每个关键 Command 有稳定幂等语义**：同键同语义返回原结果，同键不同语义显式冲突。

## 4. 目标设计

### 4.1 事件结构与版本

C1-a/C1-b 的已实现形态是**平铺 payload + 独立 `event_version` 列**，
如下 envelope **仅作为未来（C1-c/C1-d 或更晚）可选的演进形态**，尚未实施：

```json
{
  "eventVersion": 1,
  "occurredAt": "2026-09-17T00:00:00Z",
  "workspaceId": "…",
  "data": { "…": "领域相关载荷" }
}
```

当前 payload contract 是平铺结构，`event_version = 1` 声明这一结构版本。
引入 envelope 时必须显式提升版本或新增事件类型，不能猜测解析。

- `outbox_event` 新增列（**已实现：`000015_outbox_delivery_hardening`**，不改写 000001）：
  - `event_version smallint NOT NULL DEFAULT 1` —— **仅表示 payload 结构版本，不是发生次数**；
  - `claim_token uuid`、`claimed_by varchar(128)`、`claimed_at timestamptz`；
  - `dead_lettered_at timestamptz`；
  - `ck_outbox_status` 扩展为包含 `DEAD_LETTER`；
  - 新索引 `idx_outbox_claimable (available_at, created_at) WHERE status IN ('PENDING','FAILED','PROCESSING')`
    以覆盖租约到期回扫（关闭 G3）。
- **事件版本规则**：
  - `event_version <= MaxSupportedEventVersion`（当前 `1`）按已知的平铺结构解析；
  - 更高（未知）版本**不猜测、不静默跳过**，直接以可诊断错误失败并进入退避/死信路径
    （`unsupported outbox event version N`），handler 不被调用；
  - 如需 envelope，必须显式提升事件版本或新增 `event_type`。
- 事件词汇表（event vocabulary）在 `docs/architecture/` 维护：`aggregate_type`、
  `event_type`、当前 `eventVersion` 与消费者。新增事件类型必须登记。

### 4.2 状态机

```
                 claim (lease=available_at)
  PENDING ───────────────────────────────▶ PROCESSING
     ▲                                        │  handler 成功
     │                                        ▼
     │                                     PUBLISHED
     │                                       
     │  handler 失败 (退避)                     handler 失败 / 租约到期
     └──────────── FAILED ◀───────────────────┘
                    │ attempts >= maxAttempts
                    ▼
               DEAD_LETTER ──(显式 Requeue 命令)──▶ PENDING
```

- `PROCESSING` 超过租约（`available_at <= now()`）可被重新 claim（现状已如此，C1 加索引与测试）。
- **每次 claim 生成新的 `claim_token`**。任何终止性回写（`PUBLISHED` / `FAILED` / `DEAD_LETTER`）
  都必须满足 `WHERE id = :event_id AND status = 'PROCESSING' AND claim_token = :my_token`；
  影响行数为 0 即表示**已失去处理权**，旧处理者不得再改该事件状态（仅校验
  `status = 'PROCESSING'` 不够：重启后的新处理者会留下同样的 `PROCESSING` 状态）。
  领取令牌只保护 DB 回写，**无法撤销已发出的队列/外部请求**，因此消费侧仍需幂等。
- `DEAD_LETTER` 只能由显式 Command（如 `RequeueOutboxEvent`）回到 `PENDING`，
  记录操作者与理由（AGENTS §4）。
- `DEAD_LETTER` 的语义是**自动派发已停止**，**不是**「Execution 执行失败」或
  「业务动作失败」。重放必须记录操作者、理由与独立的**重放记录**，保留原 `event_id`
  与历史尝试（`attempts` / `last_error` / `dead_lettered_at`），不抹掉旧尝试。

### 4.3 派发确认与恢复规则

| 规则 | 设计 |
| --- | --- |
| claim | `FOR UPDATE SKIP LOCKED`，单条；事务内标记 `PROCESSING`、生成本次 `claim_token` 并提交后再调用 handler |
| 租约 | `available_at = now() + claimTTL`；`claimTTL` 可按事件类型配置，默认 30s |
| 终止性回写 | `PUBLISHED` / `FAILED` / `DEAD_LETTER` 全部按 `(id, status='PROCESSING', claim_token)` 条件更新；0 行 = 丢失处理权，旧处理者只记日志、不改状态 |
| 退避 | `available_at = now() + min(base * 2^(attempts-1), cap) + jitter`，默认 base 5s、cap 5m |
| 最大尝试 | 默认 `maxAttempts = 12`，达到后 → `DEAD_LETTER`（关闭 G2）；死信后不再被 claim |
| 派发顺序 | `ORDER BY created_at, id`（稳定 tiebreaker，关闭 G5 的一半） |
| 严格 per-aggregate 顺序 | 可选增强：claim 时排除「同 `aggregate_type + aggregate_id` 存在更早未完成事件」的行。**默认关闭**，需要时按事件类型开启 |
| worker 容错 | handler 失败 → 记 `FAILED`/`DEAD_LETTER`，`Run` 继续；**瞬时 DB 错误退避后继续**；只有 `ctx` 取消或**致命 DB 条件**（SQLSTATE 类 `42`/`28`/`3D`/`3F`：缺表/缺列/无权限/库模式不存在）才终止 `Run`（关闭 G4）。缺表/权限/schema 不兼容**不能**无限循环伪装正常 |
| 未知事件版本 | `event_version > MaxSupportedEventVersion` → 可诊断失败 → 退避 → 死信；handler 不被调用（见 4.1） |
| 重复派发 | **允许重复投递，业务结果不得重复**。Outbox/handler 无法消除「队列已接收、确认丢失」的重复；"最终仅入队一次"改为「至少一次投递 + 幂等消费」 |

**幂等 handler 契约（统一约定，含去重键修正）**：

- 消费侧去重键是 **`(consumer_name, event_id)`**，由 `outbox_event_consumption` 唯一键承载。
  **不得**用 `(event_type, aggregate_id, eventVersion)` 作为去重键：`eventVersion` 是**载荷结构
  版本**，不是同一业务事件的区分维度，也不代表发生次数。
- 同一事件的**派发重试与死信重放保留原 `event_id`**（新业务动作才产生新 ID），因此
  `(consumer_name, event_id)` 是稳定的消费幂等键。
- 多个消费者各自记录处理结果（每消费者一行）；一个消费者成功不代表其它消费者成功。
- 业务结果幂等**不依赖**消费确认表：由各业务对象自身的唯一约束兜底（例如 Execution 状态
  短路、输出版本唯一约束），消费表只是「本消费者已处理」的确认记录。
- 实现状态：`metadata.ProjectProductRelease` 已满足；C1 后续为该契约补可复用的契约测试。

### 4.4 操作幂等键

现存两种幂等载体，C1 不合并它们，只统一**命名与语义约定**：

| 载体 | 位置 | 用途 |
| --- | --- | --- |
| `command_idempotency` | migration 000008，`product/infrastructure/release_publish.go` | 通用 Command 请求去重（当前只有 Release 发布在用） |
| `entity_mapping_decision.idempotency_key` | migration 000014，A1 | 映射决策的专用幂等键（已实现 `MappingDecisionMatchesRequest` 语义比较） |

- 命名：`command_type = <DOMAIN>.<COMMAND>`，例如 `WORKFLOW.CREATE_EXECUTION`、
  `PRODUCT.PUBLISH_RELEASE`。
- HTTP：需要幂等的命令要求 `Idempotency-Key` 头（沿用现有 `publish` 的约定）；
  缺失时返回 `IDEMPOTENCY_KEY_REQUIRED`（400），不静默生成。
- 语义：`command_idempotency (workspace_id, command_type, idempotency_key)` 唯一。
  - 同键**同语义** → 返回原 `object_id` / `result_ref`，不重复产生副作用；
  - 同键**不同语义** → 显式 `*_KEY_CONFLICT`（与 A1 的
    `MappingDecisionMatchesRequest` 同一思路：比较请求语义，排除时间戳与随机 ID）。
- **严格模式（硬性）**：新接入本幂等约定的 Execution 命令**必须**提供稳定键
  （由调用方/上游生成，不能每次请求重新随机）；缺失 → `IDEMPOTENCY_KEY_REQUIRED`，
  不静默生成、不降级为「每次都新建」。同键不同语义 → 冲突，**不返回旧结果也不新建事实**。
- 幂等记录与业务事实在**同一事务**内写入。
- 幂等记录**不是** Outbox 事件：Outbox 负责“异步副作用”，幂等键负责“请求去重”，两者不可互相替代。
- `ENTITY.RECORD_MAPPING_DECISION` 继续使用 `entity_mapping_decision.idempotency_key`
  （已有专用表与语义比较）；C1 只要求它的错误码与命名与本约定对齐，不迁移其存储。

### 4.5 Execution 入队经 Outbox（C1 的核心行为变更）

T2 实现：

1. 事务内：`InsertExecution` + `appendExecutionEvent("ExecutionQueued")` + Audit
   + 写 `command_idempotency`（含 canonical SHA-256 指纹）。**不在事务内直接入队。**
2. Worker 侧的 `execution-queue` Outbox handler 消费 `ExecutionQueued` / `ExecutionRetried`
   → `queue.EnqueueExecution(aggregate_id)`。
3. handler 幂等由队列消费者兜底：`workflow/transport/queue/handler.go` 已对
   `SUCCEEDED / FAILED / CANCELLED / SUBMITTING / RUNNING` 直接返回 `nil`，重复入队不会
   产生重复输出或重复远程 job。
4. 恢复：入队瞬时失败 → Outbox `FAILED` → 退避重试；队列接收但确认丢失允许重投，
   消费者已有领取保护，不创建第二个 Execution。
5. 不再提供针对 pre-production 旧 `QUEUED` / retention-only 事件的 reconciliation CLI。
   当前恢复依赖 Outbox 的 claim/lease/退避重试，以及 worker dispatch 前的 workspace/reference
   revalidation；引用失效或越权时 fail closed 并 quarantine。

**对共享路径的影响声明**：以上只替换"提交后直接入队"这一步。`CreateExecutionCommand`、
输入绑定（`Inputs`）、`domain.NewExecution`、`Succeed` 的输出版本语义均不变；
队列端口仅由 Outbox handler 调用，而不是 `ExecutionService.Create/Retry`。

**两个派发入口都要切换（硬性）**：`Create`（首次入队）与**显式 Retry**（重试入队）
是两个独立的 `EnqueueExecution` 调用点，C1-d 必须**同时**改为经 Outbox，不能只移除
`Create` 里的直接 enqueue 而留下 Retry 的直连路径。当前系统不再为假想的 pre-production
遗留 `QUEUED` 行保留额外对账协议。

### 4.6 观测

- 日志/指标：`claimed`、`published`、`failed`、`dead_lettered`、`lease_reclaimed`，
  按 `event_type` 分组；`last_error` 保留最近一次错误。
- `attempts`、`published_at`、`claimed_by/claimed_at` 支持"事件卡在哪一步"的定位。

## 5. 测试与验收计划

### 5.1 Outbox 派发（真实 PostgreSQL）—— C1-a/C1-b 已实现

实现于 `apps/platform/internal/platform/outbox/publisher_integration_test.go`
（`TEST_POSTGRES_DSN` 守卫）与 `publisher_test.go`：

1. **领取令牌守卫**：旧租约持有者在被接管后既不能 `PUBLISHED` 也不能写失败状态，
   续租/完成回写的行数为 0，消费确认整事务回滚（`TestLostLeaseHolderCannotOverwriteCurrentClaim`）。
2. **毒事件死信**：连续失败达到 `MaxAttempts` 后 `DEAD_LETTER`，其后不再被 claim、
   `attempts` 不再增长（`TestPoisonEventDeadLettersAfterMaxAttempts`）。
3. **当前 V1 payload 可消费**：`event_version = 1` 的平铺载荷可正常派发
   （`TestVersionOnePayloadEventStaysConsumable`）。
4. **未知版本可诊断**：`event_version = 99` 不调用 handler，进入可诊断失败并死信
   （`TestUnknownEventVersionFailsDiagnosably`）。
5. **重复投递**：确认丢失后重投为至少一次，但每消费者只有一条消费确认
   （`TestRedeliveryAfterLostAcknowledgementRecordsSingleConsumption`）。
6. **稳定排序**：`created_at` 相同时按 `id`（`TestDispatchOrderIsStableByCreatedAtThenID`）。
7. **worker 容错**：handler 失败不终止 `Run`，后续事件正常派发；缺表等致命 DB 条件
   快速失败（`TestRunSurvivesHandlerFailureAndDispatchesFollowingEvents`、
   `TestRunStopsOnFatalSchemaError`）；退避与致命错误分类有纯单测。

**未实现（明确未完成项）**：死信重放 Command（`RequeueOutboxEvent`）及其 Audit 尚未实现，
留待 C1-e；因此「死信人工处置」目前只有诊断信息，没有重放入口。

### 5.2 统一 dispatcher 与每处理器确认（真实 PostgreSQL）

实现于 `apps/platform/internal/platform/outbox/dispatcher_integration_test.go` 与 `router_test.go`：

1. **部分失败不提前完成**：一个事件两个必需处理器，一个成功一个失败时，事件为 `FAILED`、
   成功方确认保留、失败方无确认（`TestDispatcherPartialFailureRetriesOnlyUnconfirmedHandlers`）。
2. **只重跑未确认处理器**：重试后成功方不再调用，只有失败方重跑；全部确认后才是 `PUBLISHED`。
3. **同类型多事件不丢失**：同一 `aggregate_id` 下两次不同类型/结构的 `event_id` 均被处理且各确认一次
   （`TestDispatcherProcessesDistinctEventsOfSameTypeAndAggregate`）。
4. **确认丢失重投**：处理器已执行但确认未提交时重投（至少一次），业务事实因处理器幂等而不重复，
   确认仅一条（`TestDispatcherRedeliveryAfterLostConfirmationKeepsSingleFact`）。
5. **租约接管**：旧持有者既不能写处理器确认也不能置 `PUBLISHED`，状态与新 token 不被破坏
   （`TestDispatcherStaleHolderCannotConfirmAfterTakeover`）。
6. **显式仅保留**：`RequiredHandlers` 为空的事件无处理器也能 `PUBLISHED`，且不写确认行。
8. **缺失处理器拒绝启动**：`NewDispatcher` 对未注册的必需处理器返回显式错误。
9. **路由表词汇完整**：`internal/platform/routing/routing_test.go` 断言事件词汇表与路由表一一对应，
   且 OpenMetadata 开关只改变 `ProductReleased` 是否要求 `metadata-projection`，不隐式完成。
9. **义务在事件上冻结，跨部署剖面不变**：governance 剖面写入 `ProductReleased` 时即冻结
    `routing_version` 与 `required_handlers`；以 governance 关闭的剖面重启后处理同一事件时，
    不会重新解释义务，而会在缺少必需 handler 时显式失败
    （`TestDispatcherHonoursFrozenObligationAcrossRoutingProfiles`）。
10. **写入期冻结，先于任何派发**：通过 `outbox.Append` 记录事件时即按部署剖面冻结义务；
    并发的“较小处理器集合”实例即使先领取，也无法按较少处理器提前完成事件，只能显式失败
    （`TestAppendFreezesObligationBeforeAnyDispatch`）。
11. **仅保留是显式的空集合**：retention-only 义务冻结为 `routing_version<>''` + 非 nil 空数组
    （`TestAppendFreezesRetentionOnlyObligation`）。
12. **未声明事件类型拒绝写入**：`outbox.Append` 对未声明类型或缺少 routing source 返回显式错误
    （`TestAppendRejectsUndeclaredEventType`）。

### 5.3 Handler 幂等契约

同一事件重复派发两次 → 可观察副作用计数为 1（以 `ProductReleased` →
`governance_projection` 为契约样例）。

### 5.4 请求幂等

- Create 同键同语义重放 → 返回同一 Execution，Execution/输入绑定/Outbox/Audit 计数为 1；
  并发提交也只允许一个事务赢得幂等槽位。
- Retry 同键同语义重放 → 返回同一重试 Execution，不能连续创建多个重试任务。
- 同键不同语义 → `IDEMPOTENCY_KEY_CONFLICT`，无新事实。
- 缺失幂等键 → `IDEMPOTENCY_KEY_REQUIRED`，无新事实。

### 5.5 入队恢复

使用真实 Redis/asynq 让 `execution-queue` handler 入队失败：Execution 已提交、Outbox
事件为 `FAILED` 并按退避重试；Redis 恢复后可继续派发。确认丢失允许消息重投，消费者
领取保护不创建第二个 Execution。

### 5.6 Execution 恢复

当前恢复只验证现行协议：Outbox 派发失败后按 lease/退避自动重试；worker dispatch 前再次
校验 workspace/reference；无效引用进入 quarantine。已移除针对开发阶段旧 retention-only
事件的 `execution-reconcile` / `ExecutionReconciliationQueued` 兼容路径。

### 5.7 既有门禁

在 `required` 七组门禁下回归 native / reference / CSV / live MinIO / demo lifecycle，
并补充 #110 的"外来引用被拒 + 零入队调用 + 无新事实"断言。

## 6. 分阶段

| 阶段 | 内容 | 状态 |
| --- | --- | --- |
| C1-a | 事件版本列 + `claim_token` + `DEAD_LETTER` + 每消费者确认表 + 新索引（`000015_outbox_delivery_hardening`） | **已实现** |
| C1-b | publisher：claim token 守卫、退避/最大尝试/死信、瞬时错误容错、稳定排序、事件版本兼容 + 派发契约测试 | **已实现** |
| 扇出前置 | 统一 dispatcher + 版本化路由表 + 每处理器确认（模型 A，issue #103）；不改 Execution 输入/输出语义；`000016_outbox_event_routing_obligation` 把处理义务冻结在事件上 | **已实现** |
| C1-c / T2-a | 请求幂等：`WORKFLOW.CREATE_EXECUTION` / `WORKFLOW.RETRY_EXECUTION`、canonical 指纹、严格 `Idempotency-Key`、并发收敛 | **已实现**（`000017`） |
| C1-d / T2-b | Execution 入队改经 Outbox（`Create` **与** Retry 两个入口）+ `execution-queue` Redis/asynq handler + 路由版本升级 | **已实现** |
| T2-c | pre-production 遗留 `QUEUED` 对账兼容路径 | **已移除**（#159 cleanup） |
| C1-e | 死信重放 Command（操作者、理由、独立重放记录，保留原 `event_id`） | 待实现（未完成项） |
| C2 | 原生执行恢复与输出幂等（另文） | 设计已合入 |

迁移号约定：编号按**实际合并顺序**分配，不预先锁定；C1-a 占用 `000015`，扇出前置占用 `000016`。
任何新迁移均为**新增向前迁移**，不得改写 `000001`~`000016`（AGENTS §11 历史事实不可覆盖）。
扇出前置复用 `outbox_event_consumption` 记录每处理器确认，并新增 `000016` 冻结事件处理义务。
T2 新增 `000017_execution_command_fingerprint`，不回填或改写历史幂等记录。

C1-a ~ C1-d 允许在同一聚焦 PR 内完成（共享同一迁移与 publisher 改动），
但每一步都要有独立回归测试；C2 不混入。

## 7. 未决问题

1. `maxAttempts` 与 `claimTTL` 是否按 `event_type` 配置，还是全局默认值（当前全局默认）。
2. 是否引入严格的 per-aggregate 顺序（成本 vs 必要性；当前默认关闭）。
3. 死信重放的用户入口与审批/双人复核程度（AGENTS §4），以及重放记录的落库形态。
4. 事件版本升级策略：同 `event_type` 多版本并存 vs 新增 `event_type`；以及
   envelope 迁移是否需要一个显式的转换步骤（C1-c/C1-d）。
5. `outbox_event_consumption` 的保留策略（长期不清理 vs 按窗口归档）。

### 7.1 已知限制：单消费者 claim 语义

C1-a/C1-b 的 claim 把事件状态置为**全局** `PUBLISHED`，一个事件只会被**一个**消费者处理一次。
`outbox_event_consumption` 虽然按 `(consumer_name, event_id)` 记录，但当前 claim 不按消费者区分，
因此**同一个事件无法扇出给第二个消费者**。

现状之所以没有在 T2 暴露：`metadata-projection` 只关心 `ProductReleased`，而
`execution-queue` 只关心 Execution 入队事件，当前事件类型不相交。
一旦某个事件类型需要两个消费者，当前模型会静默丢失第二个消费者。

**这一扇出前置已在「统一 dispatcher」增量中实现（模型 A）**，不再依赖「类型不相交」的巧合：

### 7.2 扇出前置已实现形态（模型 A，issue #103）

实现于 `apps/platform/internal/platform/outbox/router.go`、`dispatcher.go`，
路由表在 `apps/platform/internal/platform/routing/routing.go`，
worker 装配在 `apps/platform/cmd/worker/{main.go,handlers.go}`：

- **单一逻辑消费者**：`Dispatcher` 沿用 C1-b 的 `claim_token` 租约领取事件（多实例仍由 token 协调），
  然后在应用层按**事件上冻结的义务**把事件扇出给各处理器。
- **版本化路由表**（`routing.VersionFor(governanceProjection)`，基础版本 `c1-v3`；
  启用治理提供方时为 `c1-v3+governance`，保证同一版本串总对应同一必需处理器集合）：
  为每个 `event_type` 显式声明
  必须确认的处理器集合；`Route.RequiredHandlers` 为空是**显式的仅保留（retention-only）声明**，
  而不是「默认 `return nil`」推断出来的。未在路由表中声明的事件类型是**错误**，
  绝不被隐式完成（`TestDispatcherRefusesUndeclaredEventType`）。
- **处理义务冻结在事件上（`000016_outbox_event_routing_obligation`）**：`outbox_event` 的
  `routing_version` 与 `required_handlers` 均为 NOT NULL。义务必须在**写入事件时**由
  `outbox.Append` 按当前部署剖面冻结；缺少 obligation source 或未声明事件类型时写入直接失败。
  `Dispatcher` 只读事件上的冻结义务，**不**按本进程路由表重新解释，因此跨部署 profile
  仍保持原义务；若冻结义务要求本进程未注册的处理器，则**显式失败**而非发布。
  仅保留事件使用 `routing_version<>''` + 空数组。所有会产生 Outbox 事件的组合根
  （`cmd/api`、`cmd/poc-demo`、`cmd/worker`）都必须显式配置 obligation source。
- **每处理器确认**：处理器名即 `outbox_event_consumption.consumer_name`（如 `metadata-projection`、
  `execution-queue`），键为 `(handler_name, event_id)`；确认写入按 `status='PROCESSING' AND claim_token=token`
  守卫。已确认的处理器不重跑；只有**全部**必需处理器确认后才置 `PUBLISHED`。
  部分失败时已确认的确认行**保留**，重试只跑未确认的处理器
  （`TestDispatcherPartialFailureRetriesOnlyUnconfirmedHandlers`）。
- **缺失处理器 = 显式错误**：`NewDispatcher` 在启动时校验路由表要求的处理器均已注册，
  否则拒绝启动；运行期若仍缺失则显式失败，绝不写假成功确认。
- **部署剖面固定**：是否要求 `metadata-projection` 由部署是否配置 OpenMetadata 决定
  （`routing.Routes(governanceProjection bool)`）；未配置治理提供方时写入的 `ProductReleased`
  是**显式仅保留**义务。剖面在进程生命周期内固定并写入事件，之后换剖面**不可**追溯改变旧事件的义务。
- **不承诺业务幂等**：确认 ≠ 业务副作用幂等。处理器在「已执行但确认未提交」时崩溃会重跑，
  「至少一次」因此保留，业务事实去重仍由各业务对象自身的唯一约束兜底
  （`TestDispatcherRedeliveryAfterLostConfirmationKeepsSingleFact`）。
- **能力边界**：claim 仍是全局 `PUBLISHED`，因此「一个事件 + 两个处理器」只在**同一派发轮次内**支持；
  若未来需要按消费者独立进度/独立重放，仍需升级为模型 B（按消费者 claim）。
- **T2 边界**：新产生的 `ExecutionQueued`/`ExecutionRetried` 必须由 `execution-queue`
  确认。针对 pre-production 旧 retention-only 事件的补派发对账已移除；当前恢复只依赖
  Outbox retry/lease 与 dispatch-time reference validation。

T2 使用上述模型（不得新开第二套 claim）：

- **A. 单派发消费者 + 应用层扇出**：Outbox 只有一个 dispatcher 消费者，它把事件分发给已注册的
  projection/queue 处理器；`outbox_event_consumption` 改为记录每个子消费者的处理结果。（**已实现**）
- **B. 按消费者 claim**：claim 排除「本消费者已确认」的事件，`PUBLISHED` 语义改为
  「所有已注册消费者均已确认」。（未实现，仅在需求出现时引入）
