# Outbox 派发与操作幂等设计（C1）

- 状态：**草案（design only）**。本文只定义结构、状态机、确认/恢复规则与幂等键约定，
  不包含实现。实现作为 C1 的后续增量，另开聚焦 PR。
- 关联：issue #103（Domain Event / AuditEvent / Transactional Outbox 覆盖缺口）、
  #110（持久化执行 + 入队超时的恢复，禁止盲重复创建）、#100（关键 Command 幂等）；
  AGENTS.md §4（显式 Command）、§5（Domain Event + Audit + Transactional Outbox）、
  §10（外部引擎不作为业务真相）；ADR-0005（领域事件与审计）、ADR-0003（不可变版本）。
- C2（原生执行恢复与输出幂等）另文，本文只在边界处引用。

## 1. 范围与非目标

### 1.1 本文覆盖（C1）

1. Outbox 事件结构、事件版本与状态机。
2. 派发确认规则：claim / 租约 / 退避 / 最大尝试 / 死信 / 租约到期恢复。
3. 操作幂等键的命名与持久化约定（复用 `command_idempotency`）。
4. 把 **Execution 入队** 从「事务提交后直接 `queue.EnqueueExecution`」迁移为
   「事务内写 Outbox，由 worker 派发」的设计。
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
- `command_idempotency`（`migrations/000008_release_publish.up.sql:22`）：
  `workspace_id / command_type / idempotency_key / object_id / result_ref`，
  唯一键 `(workspace_id, command_type, idempotency_key)`。

### 2.2 派发

`outbox.Publisher.publishOne`（`apps/platform/internal/platform/outbox/publisher.go`）：

1. claim：`SELECT ... WHERE status IN ('PENDING','FAILED','PROCESSING')
   AND available_at <= now() ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`；
2. 标记 `PROCESSING`，`attempts = attempts + 1`，`available_at = now() + claimTTL(30s)`
   —— `available_at` 同时充当**租约**；
3. 提交 claim 事务；
4. `handler(ctx, event)`；
5. 成功 → `PUBLISHED`；失败 → `FAILED`，`available_at = now() + 5s`。

**已满足**：`PUBLISHED` 的标记严格晚于 handler 成功（#103 中"先派发后标记已发布"
这一历史项在当前实现里已成立）。C1 只补回归契约把该性质锁住。

### 2.3 现状缺口

| 编号 | 缺口 | 证据 |
| --- | --- | --- |
| G1 | 事件没有 schema/版本字段，payload 结构演进无约束 | `store.go` `Event` / `NewEvent` |
| G2 | 没有最大尝试次数，也没有死信状态，毒事件无限重试 | `publisher.go` 失败分支只写 `FAILED` |
| G3 | 租约到期的回扫路径依赖 `available_at`，但 `idx_outbox_pending` 不覆盖 `PROCESSING` | `000001` 索引定义 |
| G4 | 任何 DB/claim 错误都会让 `Run` 返回，worker 把它送 `errCh` 并触发整体停机，而不是退避重试 | `publisher.go` `Run`；`cmd/worker/main.go` publisher goroutine |
| G5 | 排序只有 `created_at`，无稳定 tiebreaker，也没有 per-aggregate 顺序保证 | `publisher.go` `ORDER BY created_at` |
| G6 | 请求幂等只有 Release 发布使用 `command_idempotency`（`product/transport/http/publish.go:23`），另有映射决策的专用 `entity_mapping_decision.idempotency_key`；Execution 创建没有请求幂等键 | 见 4.4 |
| G7 | Execution 入队在**事务提交之后**直接调用 `queue.EnqueueExecution`（`workflow/application/execution.go` `Create` 末尾），失败时返回"persisted but enqueue failed"，没有任何自动恢复，也没有经 Outbox | #103 未完成项、#110"盲重复创建"风险 |
| G8 | handler 幂等靠各 handler 自己实现（`metadata/application/service.go` `ProjectProductRelease` 用 `governance_projection` 唯一键 + `SUCCEEDED` 短路），没有统一约定与契约测试 | `migrations/000010_metadata_projection.up.sql:35` |

## 3. 目标语义

1. **至少一次投递 + 幂等 handler = 有效一次**。系统不承诺"只派发一次"；重复派发必须安全。
2. **派发成功早于标记 `PUBLISHED`**（保持现状，并加契约测试）。
3. **瞬时失败自动恢复**：DB 抖动、handler 失败、worker 崩溃都在租约/退避后自动重试，
   不导致 worker 停机、不产生悬挂的已持久化未派发事实。
4. **毒事件可诊断、可人工处置**：超过尝试上限进入死信并保留 `last_error`。
5. **每个关键 Command 有稳定幂等语义**：同键同语义返回原结果，同键不同语义显式冲突。

## 4. 目标设计

### 4.1 事件结构与版本

payload 采用带版本的 envelope，旧事件保持可读：

```json
{
  "eventVersion": 1,
  "occurredAt": "2026-09-17T00:00:00Z",
  "workspaceId": "…",
  "data": { "…": "领域相关载荷" }
}
```

- `outbox_event` 新增列（**新向前迁移，例如 `000015_outbox_delivery`**，不改写 000001）：
  - `event_version smallint NOT NULL DEFAULT 1`
  - `claimed_by varchar(128)`、`claimed_at timestamptz`（仅观测；租约仍以 `available_at` 表达）
  - `ck_outbox_status` 扩展为包含 `DEAD_LETTER`
  - 新索引
    `idx_outbox_claimable (available_at, created_at) WHERE status IN ('PENDING','FAILED','PROCESSING')`
    以覆盖租约到期回扫（关闭 G3）。
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
- `DEAD_LETTER` 只能由显式 Command（如 `RequeueOutboxEvent`）回到 `PENDING`，
  记录操作者与理由（AGENTS §4）。

### 4.3 派发确认与恢复规则

| 规则 | 设计 |
| --- | --- |
| claim | `FOR UPDATE SKIP LOCKED`，单条；事务内标记 `PROCESSING` 并提交后再调用 handler |
| 租约 | `available_at = now() + claimTTL`；`claimTTL` 可按事件类型配置，默认 30s |
| 退避 | `available_at = now() + min(base * 2^(attempts-1), cap) + jitter`，默认 base 5s、cap 5m |
| 最大尝试 | 默认 `maxAttempts = 12`（数值在实现 PR 中定稿），达到后 → `DEAD_LETTER`（关闭 G2） |
| 派发顺序 | `ORDER BY created_at, id`（稳定 tiebreaker，关闭 G5 的一半） |
| 严格 per-aggregate 顺序 | 可选增强：claim 时排除「同 `aggregate_type + aggregate_id` 存在更早未完成事件」的行。**默认关闭**，需要时按事件类型开启 |
| worker 容错 | `publishOne` 的 DB 类瞬时错误改为**退避后继续**，不再终止 `Run`；只有 `ctx` 取消才退出（关闭 G4） |
| 重复派发 | 允许；由 handler 幂等吸收。标记 `PUBLISHED` 失败导致的重复派发是设计内情形 |

**幂等 handler 契约（统一约定）**：每个 handler 必须满足
「同一 `(event_type, aggregate_id, eventVersion)` 重复投递不新增可观察副作用」。
`metadata` 的 `ProjectProductRelease` 已满足；C1 为该契约补一个可复用的契约测试。

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
- 幂等记录与业务事实在**同一事务**内写入。
- 幂等记录**不是** Outbox 事件：Outbox 负责“异步副作用”，幂等键负责“请求去重”，两者不可互相替代。
- `ENTITY.RECORD_MAPPING_DECISION` 继续使用 `entity_mapping_decision.idempotency_key`
  （已有专用表与语义比较）；C1 只要求它的错误码与命名与本约定对齐，不迁移其存储。

### 4.5 Execution 入队经 Outbox（C1 的核心行为变更）

现状（`workflow/application/execution.go` `Create`）：事务内写 Execution + `ExecutionQueued`
事件 + Audit；**提交后**调用 `s.queue.EnqueueExecution`，失败即返回悬挂错误。

目标：

1. 事务内：`InsertExecution` + `appendExecutionEvent("ExecutionQueued")` + Audit
   + （若带请求幂等键）写 `command_idempotency`。**不在事务内直接入队。**
2. Worker 侧新增 Outbox handler：消费 `ExecutionQueued` → `queue.EnqueueExecution(aggregate_id)`。
3. handler 幂等由队列消费者兜底：`workflow/transport/queue/handler.go` 已对
   `SUCCEEDED / FAILED / CANCELLED / SUBMITTING / RUNNING` 直接返回 `nil`，重复入队不会
   产生重复输出或重复远程 job。
4. 恢复：入队瞬时失败 → Outbox `FAILED` → 退避重试 → 最终入队一次；
   不再出现"已持久化但入队失败"的悬挂执行（响应 #110）。

**对共享路径的影响声明**：以上只替换"提交后直接入队"这一步。`CreateExecutionCommand`、
输入绑定（`Inputs`）、`domain.NewExecution`、`Succeed` 的输出版本语义均不变；
`ExecutionQueue` 端口保留，仅由 Outbox handler 调用，而不是 `ExecutionService.Create`。

### 4.6 观测

- 日志/指标：`claimed`、`published`、`failed`、`dead_lettered`、`lease_reclaimed`，
  按 `event_type` 分组；`last_error` 保留最近一次错误。
- `attempts`、`published_at`、`claimed_by/claimed_at` 支持"事件卡在哪一步"的定位。

## 5. 测试与验收计划

### 5.1 Outbox 派发（真实 PostgreSQL）

1. **claim 与租约**：并发两个 publisher 不重复 claim 同一条；租约到期后可被重新 claim。
2. **退避与最大尝试**：连续失败的 `available_at` 递增；达到 `maxAttempts` 进入 `DEAD_LETTER`。
3. **死信重放**：显式 Command 把 `DEAD_LETTER` 置回 `PENDING` 并留下 Audit。
4. **worker 容错**：注入一次 DB 错误，`Run` 不退出，下一轮恢复。
5. **顺序**：`created_at` 相同时按 `id` 稳定排序。
6. **派发早于标记**：handler 失败事件绝不出现 `PUBLISHED`。

### 5.2 Handler 幂等契约

同一事件重复派发两次 → 可观察副作用计数为 1（以 `ProductReleased` →
`governance_projection` 为契约样例）。

### 5.3 请求幂等

- 同键同语义重放 → 返回同一 Execution，Execution/Outbox/Audit 计数为 1。
- 同键不同语义 → `WORKFLOW_EXECUTION_KEY_CONFLICT`，无新事实。
- 缺失幂等键 → `IDEMPOTENCY_KEY_REQUIRED`，无新事实。

### 5.4 入队恢复

模拟 `queue.EnqueueExecution` 失败：Execution 已持久化、Outbox 事件 `FAILED`、
重试后仅入队一次、Execution 状态不被改写。

### 5.5 既有门禁

在 `required` 七组门禁下回归 native / reference / CSV / live MinIO / demo lifecycle，
并补充 #110 的"外来引用被拒 + 零入队调用 + 无新事实"断言。

## 6. 分阶段

| 阶段 | 内容 |
| --- | --- |
| C1-a | 事件版本列 + `DEAD_LETTER` + 新索引（新向前迁移 `000015_outbox_delivery`） |
| C1-b | publisher：退避/最大尝试/死信/容错/稳定排序 + 派发契约测试 |
| C1-c | 请求幂等：`command_type` 约定 + Execution create 幂等键 |
| C1-d | Execution 入队改经 Outbox + 入队恢复测试 |
| C2 | 原生执行恢复与输出幂等（另文，使用后续迁移号 `000016`） |

迁移号约定：C1 占用 `000015`，C2 占用 `000016`；两者均为**新增向前迁移**，
不得改写 `000001`~`000014`（AGENTS §11 历史事实不可覆盖）。

C1-a ~ C1-d 允许在同一聚焦 PR 内完成（共享同一迁移与 publisher 改动），
但每一步都要有独立回归测试；C2 不混入。

## 7. 未决问题

1. `maxAttempts` 与 `claimTTL` 是否按 `event_type` 配置，还是全局默认值。
2. 是否引入严格的 per-aggregate 顺序（成本 vs 必要性）。
3. 缺失 `Idempotency-Key` 时拒绝还是允许（当前设计为拒绝）。
4. 死信重放是否需要审批/双人复核（AGENTS §4 的严格程度）。
5. 事件版本升级策略：同 `event_type` 多版本并存 vs 新增 `event_type`。
