# AGENTS.md

本文件约束所有 AI Coding Agent、Codex 线程及人工开发者在本仓库中的实现边界。

## 1. Core Domain Independence

Core Domain 不得直接依赖以下具体产品或 SDK：

- OpenMetadata
- Apache Hop
- Splink
- Soda / Great Expectations
- Presidio
- MinIO / S3 SDK

外部能力必须通过 Port / SPI + Adapter 接入。

## 2. System of Record

Data Product Platform 自身数据库是以下对象的 System of Record：

- UseCase
- DataResource
- Dataset / DatasetVersion
- Entity / EntityMapping
- Authorization / Rights
- Workflow / Execution
- DataContract
- DataProduct / ProductVersion / ProductRelease
- CostEvent
- Evidence / AuditEvent

OpenMetadata 仅作为 Governance Projection。

## 3. Immutable Objects

以下对象创建并进入冻结状态后不可原地修改业务内容：

- DatasetVersion
- ProductVersion
- ProductRelease
- ContractVersion
- WorkflowVersion
- EvidenceSnapshot

发现错误时创建新版本或新 Release，不覆盖历史事实。

## 4. State Transitions

禁止通过通用 `PATCH status` 修改关键业务状态。

关键状态变化必须使用显式 Command，例如：

- `ApproveAuthorization`
- `InvalidateDatasetVersion`
- `ValidateProductRelease`
- `PublishProductRelease`
- `SuspendDataProduct`
- `WithdrawProductRelease`

状态迁移规则必须由 Domain 层控制。

## 5. Domain Events and Audit

关键状态变化必须：

1. 产生 Domain Event；
2. 产生或触发 AuditEvent；
3. 需要异步副作用时写入 Transactional Outbox。

不得把普通 application log 当作 AuditEvent。

## 6. Cost and Evidence First

Cost 与 Evidence 是一等业务对象，不允许设计成项目结束后补录的附属字段。

生产、人工审核、质量整改、外部服务、Release 等关键活动应尽可能产生：

- CostEvent
- Evidence
- AuditEvent

## 7. Industry Pack Boundary

园区、制造、医疗、政务等行业逻辑不得硬编码进 Core Domain。

行业能力应通过 `industry-packs/<industry>/` 提供：

- Entity Types
- Glossary
- Standardization Rules
- Matching Policies
- Indicators
- Quality Rules
- Compliance Rules
- Product Templates

禁止在核心代码中散落 `if industry == "PARK"` 逻辑。

## 8. Vertical Slice First

优先完成端到端业务切片，不优先“做完整某一技术层”。

第一条 Vertical Slice：

```text
CSV
→ DataResource
→ RAW Dataset Version
→ Entity Mapping
→ STANDARDIZED Dataset Version
→ Evidence
```

第二条：

```text
Standardized Datasets
→ Workflow
→ CURATED Dataset
→ Data Product
→ Product Release
```

## 9. Release Readiness

任何 Product Release 发布必须经过统一 `ReleaseReadiness` 判断。

至少检查：

- Production
- Rights
- Quality
- Compliance
- Contract
- Dataset
- Evidence
- Delivery（如适用）

不得绕过 Readiness 直接设置 `PUBLISHED`。

## 10. External Engines

外部 Engine 的 ID、状态和错误信息不能成为 Core Domain 的业务真相。

例如：

- `execution.id` 是平台业务 ID；
- `engine_execution_id` 只是 Hop / Python / Spark 的外部引用。

Engine Adapter 错误需要映射为平台统一错误模型。

## 11. Database Rules

- 核心可查询业务字段使用强类型列；
- Engine 扩展信息和行业扩展属性可使用 JSONB；
- 不要把核心主外键、状态、版本号藏入 JSONB；
- 历史事实对象不得软删除或覆盖；
- 对外部系统 ID 不建立数据库 FK。

## 12. Definition of Done

核心业务 Issue 至少考虑：

- Domain Rule
- Migration（如涉及持久化）
- Repository
- Application Command / Query
- HTTP API（如适用）
- Domain Event
- AuditEvent
- Evidence（关键动作）
- Idempotency（关键 Command）
- Tests

关键业务动作不得仅实现为 CRUD。

## 13. Testing

优先级：

1. Domain tests
2. Application tests
3. Adapter contract tests
4. Vertical slice integration tests

外部 Engine Adapter 应尽量复用统一 Contract Test。

## 14. Scope Discipline

POC 阶段不要主动加入：

- 完整 Billing / Settlement
- 完整 ERP / Accounting
- 自建区块链平台
- 复杂 Service Mesh
- 过早微服务拆分
- AI 黑盒风控模型
- 高敏门禁 / 人脸 / 视频数据

除非对应 Issue 明确要求。