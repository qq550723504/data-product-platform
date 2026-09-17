# 数据库设计 V1.0

目标数据库：PostgreSQL 16+。

## 1. 通用约定

- 主键：UUID
- 业务编码：`varchar(64)`
- 时间：`timestamptz`
- 扩展字段：JSONB
- 可变聚合的乐观锁：`revision bigint`
- 仅对可变业务主对象使用软删除
- 不可变事实不得软删除或覆盖

## 2. 多租户边界

核心业务对象预留：

- `workspace_id`
- `project_id`（适用时）

Workspace 代表组织 / 租户边界；Project 代表一个具体的项目或产品工作空间。

## 3. 核心表

首批迁移应覆盖：

```text
workspace
project
use_case

data_resource
resource_binding

dataset
dataset_version
dataset_version_lineage

entity_type
entity
entity_mapping

workflow
workflow_version
task
workflow_run
execution
execution_dataset

data_product
product_version
product_asset
product_release
product_release_dataset

evidence
evidence_relation
evidence_snapshot
audit_event
outbox_event
```

第二批：

```text
authorization
authorization_resource
authorization_action
authorization_scope

quality_rule
quality_result

compliance_policy
compliance_result

data_contract
contract_version

cost_event
cost_allocation
```

## 4. DataResource

DataResource 是业务级资源，而不是物理表。

关键字段：

- id
- workspace_id
- project_id
- code / name / description
- domain_code
- resource_type
- owner
- sensitivity_level
- rights_status
- quality_status
- lifecycle_status
- business_metadata JSONB

## 5. ResourceBinding

用于将核心域与元数据引擎、物理系统解耦。

关键字段：

- resource_id
- provider（`OPENMETADATA` 等）
- entity_type
- external_id
- external_fqn
- connection_ref
- binding_metadata JSONB
- is_primary

对外部系统不建立数据库外键。

## 6. Dataset / DatasetVersion

Dataset 是逻辑身份。DatasetVersion 是不可变的生产事实。

Dataset 类型：

- RAW
- STANDARDIZED
- CURATED
- PRODUCT

DatasetVersion 存储：

- version_no
- storage_type / storage_uri
- schema_version
- row_count / byte_size
- checksum
- generated_by_execution_id
- rights_snapshot_id
- quality_status
- compliance_status
- snapshot window（快照时间窗口）
- metadata JSONB

DatasetVersion 进入冻结状态后不得更新。

## 7. 生产血缘

`dataset_version_lineage` 记录输入/输出血缘，独立于 OpenMetadata 的技术血缘。

这就是平台的 Production Graph（生产图谱）。

## 8. Entity

核心模型：

```text
EntityType → Entity → EntityMapping
```

Entity 字段：

- canonical_key
- canonical_name
- attributes JSONB
- status

EntityMapping 存储：

- source_type
- source_ref
- source_key
- source_name
- match_method
- policy_version
- confidence
- status
- reviewer
- evidence

## 9. Execution

Execution 是平台的业务执行记录，独立于引擎作业 ID。

存储：

- workflow / workflow_version / task
- execution_type
- executor_type
- engine_execution_id
- status
- timing（时序信息）
- rows / bytes in/out（输入输出的行数 / 字节数）
- runtime_metrics JSONB
- error code/message

Execution 可产生：

- DatasetVersion
- CostEvent
- Evidence
- AuditEvent

## 10. DataProduct / ProductVersion / ProductRelease

DataProduct：稳定身份。

ProductVersion：不可变的产品规格。

ProductRelease：不可变的已发布快照。

ProductRelease 精确引用：

- product_version
- dataset versions
- contract version
- rights snapshot
- quality result
- compliance result
- evidence snapshot

已发布的 Release 绝不可就地编辑。

## 11. Evidence

Evidence 存储证据元数据以及可选的产物位置/哈希。

EvidenceRelation 通过 `(object_type, object_id)` 将证据关联到任意业务对象。

EvidenceSnapshot 在某一时间点冻结某个 Release / 案件（case）的证据清单（manifest）。

## 12. Cost

CostEvent 同时支持金额型与数量型事件。

示例：

- amount=12.5 CNY, category=COMPUTE
- quantity=2.5 HOUR, category=HUMAN

会计口径归类属于后续的专业复核工作，不得与生产成本归集混为一谈。

## 13. JSONB 使用策略

JSONB 用于：

- 引擎相关元数据
- 运行时指标
- schema 与快照
- 行业扩展属性
- 交付配置
- 证据清单（evidence manifest）

JSONB 不用于：

- ID / 外键
- 状态
- 版本号
- owner
- 时间戳
- 需要频繁关联或约束的字段

## 14. 删除策略

允许软删除：

- UseCase
- DataResource
- Dataset
- DataProduct
- Entity

不得删除的不可变事实：

- DatasetVersion
- Execution
- ProductVersion
- ProductRelease
- EvidenceSnapshot
- AuditEvent
- CostEvent
