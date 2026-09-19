# AGENTS.md

本文件约束所有 AI Coding Agent、Codex 线程及人工开发者在本仓库中的实现边界。

## 1. Core Domain Independence

Core Domain 不得直接依赖以下具体产品或 SDK：

- OpenMetadata
- Apache Hop
- Splink
- Soda / Great Expectations
- Presidio
- Label Studio / X-AnyLabeling
- MinIO / S3 SDK

外部能力必须通过 Port / SPI + Adapter 接入。

## 2. System of Record

Data Product Platform 自身数据库是以下核心业务事实的 System of Record：

- UseCase
- DataResource
- Dataset / DatasetVersion
- Entity / EntityMapping / immutable mapping decisions
- Authorization / RightsSnapshot
- RightsDeclaration / rights verification facts（#137 起）
- Workflow / Execution / frozen execution dependencies
- QualityAssessment / Quality findings（#131 起）
- DataContract
- CertificationProfile snapshot / DatasetCertification（#134 起）
- DataProduct / ProductVersion / ProductRelease
- CostEvent
- Evidence / EvidenceSnapshot / AuditEvent

OpenMetadata 仅作为 Governance Projection。

外部质量、处理、实体解析或标注系统均不得成为上述核心业务状态的最终真相。

## 3. Immutable Objects and Historical Facts

以下对象创建并进入冻结状态后，不得原地修改业务含义：

- DatasetVersion
- EntityMappingDecision
- Execution dependency preparation / binding / mapping usage
- ProductVersion
- ProductRelease
- ContractVersion
- WorkflowVersion
- EvidenceSnapshot
- RightsSnapshot
- QualityAssessment（#131 起）
- verified RightsDeclaration / verification fact（#137 起）
- CertificationProfile snapshot（#134 起）
- DatasetCertification（#134 起）

修正错误时创建新版本、新声明、新评测、新认证或新 Release，不覆盖历史事实。

历史事实对象不得依赖软删除来模拟修正。

## 4. Ownership and Rights Semantics

data_resource.owner_id、dataset.owner_id 只表示平台内资产责任/归属，不自动等同现实世界法律所有权。

权利模型必须区分：

~~~text
RightsDeclaration
→ Authorization
→ RightsSnapshot
→ Effective Rights
→ DatasetCertification
~~~

平台记录权利声明、依据、主体角色、允许项、限制项和 Evidence；不得声称平台自动裁定现实世界法律所有权。

衍生数据的 Effective Rights 默认 fail closed：任何必要输入不允许某个动作时，输出不得自动获得该动作。

## 5. State Transitions

禁止通过通用 PATCH status 修改关键业务状态。

关键状态变化必须使用显式 Command，例如：

- ApproveAuthorization
- VerifyRightsDeclaration
- InvalidateDatasetVersion
- RunQualityAssessment
- CertifyDatasetVersion
- ValidateProductRelease
- PublishProductRelease
- WithdrawProductRelease

状态迁移和认证判定规则必须由 Domain / Application 边界控制。

## 6. Domain Events and Audit

关键状态变化必须：

1. 产生 Domain Event；
2. 产生或触发 AuditEvent；
3. 需要异步副作用时写入 Transactional Outbox。

不得把普通 application log 当作 AuditEvent。

## 7. Cost and Evidence First

Cost 与 Evidence 是一等业务对象，不允许项目结束后再补录。

生产、人工审核、质量评测、Rights verification、Certification、外部服务、Release 等关键活动应按业务需要产生：

- CostEvent
- Evidence
- AuditEvent

## 8. Industry Pack Boundary

园区、制造、医疗、政务等行业逻辑不得硬编码进 Core Domain。

行业能力应通过 industry-packs/<industry>/ 提供：

- Entity Types
- Glossary
- Standardization Rules
- Matching Policies
- Indicators
- Quality Rules
- Compliance Rules
- Certification Profiles
- Product Templates

禁止在核心代码中散落 if industry == "PARK" 逻辑。

## 9. Vertical Slice First

优先完成端到端业务切片，不优先“做完整某一技术层”。

已完成 POC 切片：

~~~text
CSV
→ DataResource
→ RAW DatasetVersion
→ Entity Resolution
→ STANDARDIZED DatasetVersion
→ Workflow
→ CURATED DatasetVersion
→ ProductRelease
→ Evidence
~~~

当前 Certified Dataset Pilot 切片：

~~~text
DataResource
→ Rights Provenance
→ DatasetVersion
→ Processing / Entity Resolution
→ QualityAssessment
→ Effective Rights / Compliance / Contract
→ DatasetCertification
→ Certified DatasetVersion
~~~

## 10. Quality and Certification

QualityAssessment 回答“数据质量如何”；DatasetCertification 回答“该 DatasetVersion 是否满足某个 CertificationProfile”。

不得把“评测完成”自动等同于“认证通过”。

V1 不强制全行业统一总分。Critical rule、Rights、Compliance、Contract 或 Traceability 任何 required 条件缺失时，Certification 必须 fail closed。

Certified Dataset 是可独立交付成果，不要求必须包装成 DataProduct。

## 11. Release Readiness

任何 Product Release 发布必须经过统一 ReleaseReadiness 判断。

至少检查：

- Production
- Rights
- Quality
- Compliance
- Contract
- Dataset
- Evidence
- Delivery（如适用）

不得绕过 Readiness 直接设置 PUBLISHED。

DatasetCertification 与 ProductRelease 是不同业务事实；不要用一个对象替代另一个。

## 12. External Engines

外部 Engine 的 ID、状态和错误信息不能成为 Core Domain 的业务真相。

例如：

- execution.id 是平台业务 ID；
- engine_execution_id 只是 Hop / Python / Spark 的外部引用。

Engine Adapter 错误需要映射为平台统一错误模型。

## 13. Database Rules

- 核心可查询业务字段使用强类型列；
- Engine 扩展信息、行业扩展属性和受控参数可使用 JSONB；
- 不要把核心主外键、状态、版本号、权利关系藏入 JSONB；
- 历史事实对象不得软删除或覆盖；
- 对外部系统 ID 不建立数据库 FK；
- 破坏不可变历史的 migration down 必须 fail closed；
- 先检查再 DDL 的 destructive down 要考虑并发业务写入的锁语义。

## 14. Definition of Done

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
- Workspace / ownership boundary
- Historical immutability
- Tests

关键业务动作不得仅实现为 CRUD。

## 15. Testing

优先级：

1. Domain tests
2. Application tests
3. Adapter contract tests
4. Real PostgreSQL integration tests（持久化/并发/约束）
5. Vertical slice integration tests
6. Browser / live-core acceptance（用户路径）

外部 Engine Adapter 应尽量复用统一 Contract Test。

## 16. Current Stage and Scope Discipline

核心 POC 已完成。当前阶段是 #129 Certified Dataset 受控试点。

第一阶段主任务：

- #131 QualityAssessment
- #132 Quality Engine
- #133 Quality Report
- #137 Data Rights Provenance
- #134 DatasetCertification
- #135 API / UI
- #136 E2E Pilot

除非对应 Issue 明确要求，第一阶段不要主动加入：

- T4/T5/T6 全套生产可靠性路线作为前置
- 完整 Billing / Settlement / ERP
- 自建区块链平台
- 完整 IAM / 灾备 / 大规模性能平台
- 复杂 Service Mesh / 过早微服务拆分
- AI 黑盒风控模型
- 高敏门禁 / 人脸 / 视频数据
- Label Studio / X-AnyLabeling / Gold Dataset 第二阶段实现
- 完整法律合同管理或自动法律推理

文档基线见 docs/product/certified-dataset-pilot.md、docs/architecture/certified-dataset.md、docs/architecture/data-rights-provenance.md。
