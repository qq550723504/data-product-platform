# 原生执行恢复与输出幂等设计（C2）

- 状态：**C2-a 已实现（输出幂等）**；C2-b / C2-c 仍为设计。本文定义原生（native）执行的中断
  恢复规则与输出幂等语义，C2-a 的实现偏差记录在 §4.3 / §8。
- 关联：issue #110（持久化执行 + 入队超时/中断的恢复，禁止盲重复创建）、#100（关键 Command
  幂等）、#103（Outbox 派发，见 C1）；AGENTS.md §3（不可变对象）、§6（Evidence 一等）、
  §10（外部引擎 ID 不是业务真相）；ADR-0003（不可变版本）。
- 上游：C1《Outbox 派发与操作幂等设计》`docs/architecture/outbox-delivery-and-idempotency.md`。
  C1 保证「Execution 一定被派发一次以上」；C2 保证「派发一次以上不会产生重复输出」。

## 1. 范围与非目标

### 1.1 本文覆盖（C2）

1. 原生执行的阶段划分与可恢复点（哪里可以安全重放、哪里不能）。
2. 原生执行的**中断恢复**：worker 崩溃、进程被杀、租约丢失后，处于 `RUNNING` 的执行
   如何被检出并收敛到 `SUCCEEDED` / `FAILED`，而不是永久悬挂。
3. 原生执行的**输出幂等**：同一 Core Execution 重复执行时，CURATED `DatasetVersion`
   与派生产物不得重复。
4. 输出版本与 Execution 的绑定规则（哪个执行"拥有"哪个输出版本）。
5. 上述各项的测试与验收计划。

### 1.2 明确不改动（保护边界）

- 不改变「运营性重试 = 新建 Execution」的语义：`ExecutionService.Retry` 仍创建新 Execution，
  新 Execution 产生**新**输出版本（这是不可变版本的正常路径，不是重复）。
- 不改变输入绑定与 `ProcessingRequestFromExecution` 的冻结语义。
- 不改变 managed（Hop/Splink）桥接与 `ManagedReconciler` 的既有路径；C2 只补 native 的缺口。
- C2 不引入分布式事务、不引入外部工作流引擎；继续以平台数据库为 System of Record。

## 2. 现状事实（实现前已核对）

### 2.1 原生执行路径

`workflow/transport/queue/handler.go` `Handle`：

1. 读取 Execution；`SUCCEEDED / FAILED / CANCELLED / SUBMITTING / RUNNING` → 直接 `nil`（幂等短路）。
2. `ValidateExecutionOwnership`；引用失败 → quarantine。
3. `executeNative`（`handler.go`）：
   - `StartWithReferenceCheck(QUEUED → RUNNING)`，`engineExecutionID = "native:<executionID>"`；
   - `native.Execute(ctx, request)`；
   - 成功 → `Succeed(executionID, outputDatasetVersionID, metrics)`；失败 → `Fail(...)`。

`workflow/native/engine.go` `Execute`（约 74–228 行）：

- 读取输入版本 → 实体解析输出（`FindSucceededOutputVersionForInput` 复用）→ 计算指标 →
  `datasetWriter.Handle(...)` 写 CURATED 输出版本 → `AddLineage`。
- 输出 CSV 含 `generatedAt = time.Now().UTC()`（`engine.go:140`），并且 `entityRepo.InsertMapping`
  （WORKFLOW_ALIAS，`engine.go:421`）会写入映射。两者都**不是**按内容确定性的。

`dataset/application/upload_version.go` `Handle`：

- `AllocateVersion`（`SELECT id FROM dataset ... FOR UPDATE`，`MAX(version_no)+1`）；
- 写对象存储 → `SetReady`；`dataset_version` 有 `uq_dataset_version_no(dataset_id, version_no)`，
  `generated_by_execution_id` 可空且**无唯一约束**。

`dataset/infrastructure/lineage.go` `AddLineage`：已用
`uq_dataset_lineage (output_version_id, input_version_id, relation_type)` +
`ON CONFLICT ... DO NOTHING`，**lineage 写入已是幂等的**（无需 C2 改动）。

### 2.2 现状缺口

| 编号 | 缺口 | 证据 |
| --- | --- | --- |
| N1 | 原生执行没有 reconciler。`StartWithReferenceCheck` 置 `RUNNING` 后进程崩溃 → 执行永久悬挂在 `RUNNING`，无人收敛 | `handler.go` `executeNative`；`managed_execution.go` 只有 managed reconciler |
| N2 | 重复派发在**并发**下靠 status 短路安全，但在「RUNNING 中断后重放」场景下短路返回 `nil`，既不完成也不失败 → 与 N1 叠加成永久悬挂 | `handler.go` `case SUBMITTING, RUNNING: return nil` |
| N3 | 输出幂等只靠"同一 Execution 不二次执行"这一隐含假设。一旦需要重放已中断的 Execution，`datasetWriter.Handle` 会分配**新版本号** → 同一 Execution 产生两个 CURATED 版本 | `upload_version.go` `AllocateVersion`；无 `(output_dataset_id, generated_by_execution_id)` 唯一约束 |
| N4 | 输出内容非确定（`generatedAt`、映射写序），无法用 checksum 相等判定"同一输出" | `engine.go:140` |
| N5 | 中断后可能已写对象存储但未 `SetReady`，或已 `SetReady` 但未 `Succeed`，缺少"孤儿/半成品输出"的检出与归属规则 | `upload_version.go` 两阶段 + `handler.go` `Succeed` |
| N6 | WORKFLOW_ALIAS 映射写入（`InsertMapping`）不幂等，重放会重复写映射 | `entity_repository.go` `InsertMapping`；`engine.go:421` |

## 3. 目标语义

1. **每个已持久化的 Execution 都会收敛**：要么 `SUCCEEDED`，要么 `FAILED`，不允许无限期 `RUNNING`。
2. **同一 Core Execution 至多一个有效输出版本**：重放不产生第二个 CURATED 版本，
   也不产生重复 lineage / 映射。
3. **恢复是确定性的**：给定同一 Execution，恢复动作只有"采用已存在的输出并完成"或
   "终止并给出可审计的失败原因"两种，不做"猜测式重跑"。
4. **运营性重试仍是新 Execution → 新版本**（不变）。

## 4. 目标设计

### 4.1 原生执行的阶段划分与可恢复点

| 阶段 | 事务边界 | 可重放 | 说明 |
| --- | --- | --- | --- |
| P0 QUEUED | — | 是 | 见 C1：由 Outbox 派发 |
| P1 RUNNING 抢占 | 单事务 CAS `QUEUED → RUNNING` | 是（CAS 天然幂等） | `StartWithReferenceCheck` |
| P2 计算（解析/指标） | 无副作用（解析版本已复用） | 是 | 纯读 + 复用已有解析输出 |
| P3 写 CURATED 输出 | 两阶段（分配版本→存储→SetReady） | **需幂等键** | 见 4.3 |
| P4 写 lineage / 映射 | 单事务 | **映射需幂等键**（lineage 已幂等） | 见 4.3 |
| P5 完成 `Succeed` | 单事务 CAS `RUNNING → SUCCEEDED` | 是（CAS 幂等） | `workflow/application/execution.go` `Succeed` 的 compare-and-set |

**关键结论**：P1/P5 已有 CAS 幂等；P2 已复用解析输出；**P3/P4 是输出幂等的唯一缺口**。

### 4.2 中断恢复：native reconciler

新增 `NativeReconciler`（与 `ManagedReconciler` 对称，复用同一调度骨架）：

1. 选取 `engine_type = 'NATIVE'`（或等价标识）且 `status = 'RUNNING'` 且
   `updated_at < now() - nativeLeaseTTL` 的执行。
2. 对每条执行按 4.3 的规则判定：
   - **存在已 `READY` 的输出版本** → 采用该版本，CAS `RUNNING → SUCCEEDED`；
   - **不存在已 `READY` 的输出版本，但在分配中/存储中** → 按 4.3 的"半成品"规则处置
     （丢弃半成品并允许一次安全重跑，或标记失败）；
   - **无任何输出版本且租约已过期** → 允许**一次**受控重跑（`attempt` 有上限），
     重跑同样走 4.3 的幂等输出路径。
3. 超过重跑上限 → `FAIL(EXECUTION_RECOVERY_EXHAUSTED)`，保留 Evidence/Audit。
4. 恢复动作本身产生 Domain Event + AuditEvent（AGENTS §5）。

底层支撑（已存在，无需新迁移）：`migrations/000011_managed_execution_hardening.up.sql`
已建 `idx_execution_engine_status_id (engine_type, status, id)`，reconciler 的
"`engine_type='NATIVE' AND status='RUNNING'`" 扫描可直接命中；租约时间用 `execution.updated_at`
（`000004_workflow_execution.up.sql`）或 `started_at`，无需新增列。

`nativeLeaseTTL` 与最大恢复次数按执行类型配置，默认值在实现 PR 中定稿。

### 4.3 输出幂等

**幂等键**：`(output_dataset_id, generated_by_execution_id)`。

- 新向前迁移（编号按**实际合并顺序**分配，不预先锁定；`000015` 已被 C1-a 占用，
  扇出前置占用 `000016`，`000017`/`000018` 被 T2/T3-B2 占用，`000019` 被 HQD-1 占用，
  C2-a 落为 `000020`；不改写已有迁移）：

  ```sql
  CREATE UNIQUE INDEX uq_dataset_version_execution_output
      ON dataset_version(dataset_id, generated_by_execution_id)
      WHERE generated_by_execution_id IS NOT NULL
        AND status NOT IN ('INVALID', 'SUPERSEDED');
  ```

  **相对草案的收紧（实现时补充，理由见下）**：草案的谓词只有
  `generated_by_execution_id IS NOT NULL`，会把终态行（`INVALID` / `SUPERSEDED`）也算进约束。
  但 `dataset_version` 行永不删除（`guard_dataset_version_immutability` 禁止 DELETE），
  且 `READY` 行的 `generated_by_execution_id` 不可改写，所以该版本在任何已经产生过重复
  输出的真实安装上**无法安装且无法补救**。因此索引只覆盖"仍可作为该 Execution 输出"的
  状态（分配窗口 `CREATED`/`PROCESSING`、半成品修复目标 `FAILED`、已发布 `READY`），
  终态行作为历史事实排除在约束之外。同一对 `(dataset, execution)` 可以有多条历史行，
  但有且仅有一条 live 行；把多余副本移出 live 集合需要显式状态变更
  （对 `READY` 副本 `InvalidateDatasetVersion`，对半成品 `FAIL`），而不是改写历史。
  迁移守卫只统计 live 行，遇到重复时 `RAISE EXCEPTION` 并给出该补救路径，而不是静默丢事实。
- `datasetWriter.Handle` 改为**先查后写**，且**在分配版本的事务内**完成（`AllocateVersion`）：
  - 若 `(datasetID, generatedByExecutionID)` 已有 `READY` 版本 → 直接返回该版本（不新建、
    不重写对象、不重复发事实）；
  - 若存在半成品（`CREATED`/`PROCESSING`/`FAILED`）→ 复用它（必要时 `StartProcessing` 修复
    `FAILED`），不新增版本号；
  - 若命中的是 `INVALID`/`SUPERSEDED` → 硬错误，**不**静默复活已撤销的输出；
  - 否则分配新版本。
  - 命中唯一索引冲突（并发重放）时读回既有行，而不是新增版本号。
- **幂等键落库时点**（草案 §8 的落地）：`generated_by_execution_id` 在**分配**时写入
  `INSERT`，而不是等到 `SetReady`。这样唯一索引覆盖完整的两阶段窗口，关闭"并发重放各自
  建半成品行"的缺口。
- **内容防覆盖**：对象存储 `Put` 无条件覆盖，因此每次尝试使用独立暂存键
  `datasets/<dataset>/v<no>/<attemptToken>/<file>`，只有获胜尝试引用的对象成为发布字节；
  失败的尝试只留下未被引用的对象，不会覆盖已发布内容。（用户 C2 第三条：唯一输出行 ≠
  内容不被覆盖。）
- 该改动把"同一 Execution 至多一个输出版本"变成数据库强约束，而非调用方约定（关闭 N3/N7）。
- **lineage 幂等**：已满足（`uq_dataset_lineage` + `ON CONFLICT DO NOTHING`），C2 不加改动，
  已补回归测试锁住（`TestLineageReplayKeepsOneEdgePerInput`）。
- **映射幂等**：已满足——原生引擎的 alias 写入走 `RecordMappingDecision`，
  带 `stableAliasIdempotencyKey(executionID, inputName, inputVersionID, sourceRef, sourceKey)`，
  重放返回既有 decision 而非追加；`engine_integration_test.go` 已断言重放不改变
  `entity_mapping_decision` 计数。C2 不新增映射侧改动。
  - 遗留入口 `InsertMapping`（仅测试调用）不带幂等键，作为 legacy 路径保留并在注释中标注；
    新命令路径统一用 `RecordMappingDecision`。

### 4.4 半成品与孤儿输出版本

- 对象存储已写但 DB 未 `SetReady`：由幂等键复用同一 `version` 行（P3 的第一阶段已建行），
  重跑只补 `SetReady`，不新增行。
- DB 已 `SetReady` 但未 `Succeed`：reconciler 采用该版本完成 Execution（4.2 第一分支）。
- 二者都不需要"删除历史事实"：半成品在复用后成为正式版本，或被显式 `InvalidateDatasetVersion`
  标记（AGENTS §3/§4 的不可变语义）。

### 4.5 与 C1 的衔接

- C1 保证 `ExecutionQueued` 事件至少派发一次 → 队列任务至少投递一次。
- 队列 handler 的 status 短路 + C2 的输出幂等键共同保证：**重复投递不产生重复输出**。
- 恢复由 reconciler 承担，而不是"靠队列重投保证"（队列重投在 RUNNING 短路下不解决问题）。

### 4.6 观测

- 指标：`native_executions_recovered`、`native_recovery_exhausted`、`output_version_reused`、
  `half_written_outputs_repaired`。
- 每条恢复记录 `executionId`、`engineExecutionId`、`reason`、`action`、`attempt`。

### 4.7 C2-a 实现落点（供审阅对照）

| 机制 | 位置 |
| --- | --- |
| live 输出唯一索引 | `migrations/000020_dataset_version_execution_output_key.up.sql` |
| 分配时写幂等键 + 先查后写 + 冲突读回 | `dataset/infrastructure/postgres_repository.go`（`AllocateVersion` / `findVersionByExecutionOutputTx` / `isExecutionOutputConflict`） |
| 两阶段写与暂存对象隔离 | `dataset/application/upload_version.go` |
| 行锁重读（不重复发事实） | `LockVersion` + `SetReady` |
| 回归测试 | `dataset/application/execution_output_idempotency_integration_test.go` |
| 迁移守卫测试（含历史重复可升级性） | `platform/migration/dataset_output_key_up_integration_test.go` |
| 迁移测试脚手架（独立 scratch DB） | `platform/migration/scratch_database_test.go` |

## 5. 测试与验收计划

### 5.1 输出幂等（真实 PostgreSQL + 对象存储 stub）

1. 同一 `(dataset_id, generated_by_execution_id)` 调用 `Handle` 两次 → 只有一个 `READY` 版本，
   第二次返回同一 `version.ID`。（`TestExecutionOutputReplayPublishesExactlyOneVersion`）
2. 第一阶段后中断（已建 PENDING 行）→ 重跑复用该行，`version_no` 不变。
   （`TestInterruptedOutputIsRepairedWithoutConsumingANewVersionNumber`）
3. `AddLineage` 重放不产生重复边。
   （`TestLineageReplayKeepsOneEdgePerInput`）
4. 映射重放不产生重复决策；已发布 Release trace 不变
   （`InsertMapping` → `RecordMappingDecision` 稳定键，`engine_integration_test.go` 已断言；
   与 B 的回归测试对齐）。
5. 并发投递不覆盖已发布字节，也不新增版本号。
   （`TestConcurrentDeliveriesNeverOverwritePublishedContent`）
6. 已撤销（`INVALID`）输出不被静默复用。（`TestInvalidatedOutputIsNotSilentlyReused`）
7. 数据库拒绝同一 Execution 的第二个 live 输出，且错误可识别。
   （`TestDatabaseRefusesASecondOutputForOneExecution`）
8. 迁移：已有重复 live 输出 → 拒绝且回滚（不留下索引、不删行）；只有历史重复 → 允许升级。
   （`TestC2AOutputKeyMigrationRefusesExistingDuplicateOutputs`、
   `TestC2AOutputKeyMigrationUpgradesInstallationsWithHistoricalDuplicates`）
9. 迁移部分性/可逆性：NULL 执行不受限，down 只删索引、保留行。
   （`TestC2AOutputKeyMigrationIsPartialAndReversible`）

### 5.2 原生恢复

1. 构造 `RUNNING` 且 `updated_at` 过期的执行：
   - 已有 `READY` 输出 → reconciler 收敛为 `SUCCEEDED`，产出恰好一个版本；
   - 无输出 → 受控重跑一次 → 恰好一个版本；
   - 持续失败 → 达到上限后 `FAIL(EXECUTION_RECOVERY_EXHAUSTED)`。
2. 恢复动作幂等：reconciler `RunOnce` 两次不产生第二个版本、不重复 Audit。
3. 并发两个 reconciler 只允许一个赢得 `RUNNING → SUCCEEDED`（CAS）。

### 5.3 #110 端到端

- 入队超时/中断 → Execution 被恢复或显式失败，**不出现盲重复创建**；
- 恢复全程产生 Evidence + AuditEvent；
- `required` 门禁 + live core 回归（MinIO 真实对象存储路径）保持绿。

## 6. 分阶段

| 阶段 | 内容 |
| --- | --- |
| C2-a | 输出幂等键迁移 + `datasetWriter.Handle` 先查后写 + lineage/映射幂等 | **已实现**（迁移 `000020`） |
| C2-b | `NativeReconciler`（租约 + CAS 收敛 + 重跑上限）+ 恢复 Audit/Evidence |
| C2-c | 观测、上限/租约配置、端到端 #110 回归 |

C2-a 必须先于 C2-b（reconciler 依赖幂等输出）；两者可同 PR，但测试独立。

## 7. 未决问题

1. `nativeLeaseTTL` 与最大恢复次数取值；是否按 workflow 类别区分。
2. 半成品输出版本在"永久无法修复"时：C2-a 的选择是**保留为 `FAILED` 证据**（它同时是该
   Execution 的 live 复用槽位）。若运维上必须让它离开 live 集合（例如历史重复导致迁移守卫
   拒绝），文档路径是显式状态变更到 `INVALID`，而不是删除行或改写 `generated_by_execution_id`。
   注意：领域命令 `InvalidateDatasetVersion` 只接受 `READY`，对 `FAILED` 半成品需要
   半产品专用的 FAIL/废弃动——是否要为它加显式 Command 待 C2-b 决定。
3. 当一个 Execution 的输出已被人工上传 `SUPERSEDED`、而该 Execution 被重新投递时，C2-a 选择
   **硬错误**（不静默复活已撤销输出）。C2-b 的 reconciler 需要把这个错误翻译成一个明确的
   终态（例如 `FAIL(OUTPUT_WITHDRAWN)`）或人工决策点。
4. WORKFLOW_ALIAS 映射是否要回填 `source_job_id`（涉及已发布 Release 的 trace 稳定性）。
5. 恢复重跑是否复用 `Retry`（新 Execution）语义的一部分，还是保持"同一 Execution 原地恢复"的独立路径。
6. 是否需要把 reconciler 与 C1 的 Outbox 派发统一到同一个调度器。
7. 失败的暂存对象（`datasets/<ds>/v<no>/<attemptToken>/<file>`）目前只靠"未被引用"隔离，
   没有 TTL 清理；是否加生命周期规则由 C2-c 的运维收口。

## 8. 补充：幂等键必须在"分配版本"时落库

C2-a 之前，`AllocateVersion` / `insertVersion` 插入行时**不写** `generated_by_execution_id`，
该列只在 `SetReady` 阶段被赋值。若只在 `SetReady` 才带键，唯一索引无法在分配阶段拦住并发的
第二个半成品行。

C2-a 已落地：

1. `UploadVersionCommand.GeneratedByExecutionID` 透传到
   `AllocateVersion` / `insertVersion`，行创建即带幂等键；
2. 唯一索引冲突（`uq_dataset_version_execution_output`，SQLSTATE `23505`）后**读回**既有行，
   而不是新增版本号（并发重放安全）；
3. `generated_by_execution_id` 在同一行上不可改写（`guard_dataset_version_immutability`
   已覆盖 `READY` 之后的改写；分配阶段写入后也不得再改）；
4. 复用查询 `ORDER BY version_no ASC LIMIT 1`，使历史重复对总是修复同一条版本。
