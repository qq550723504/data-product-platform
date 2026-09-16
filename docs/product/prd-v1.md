# Data Product Platform PRD V1.0

## 1. 产品目标

将分散、异构、权利边界不清晰的原始数据，持续生产为可理解、可治理、可交付、可流通、可审计的数据产品。

## 2. 非目标

V1 不定位为：

- 完整元数据平台
- 完整 ETL/湖仓平台
- 完整可信数据空间基础设施
- 数据交易所
- 财务 ERP
- 一键数据入表工具

## 3. 用户角色

- 产品经理：定义 Use Case、Data Product
- 数据管理员：资源盘点、分类、Owner
- 数据工程师：接入、标准化、加工
- 业务专家：指标、实体匹配、人工复核
- 治理人员：质量、标准、元数据
- 合规/法务：Rights、Authorization、Compliance Review
- Product Owner：Release 与生命周期
- 系统管理员：用户、权限、系统配置

## 4. 核心流程

```text
Create Use Case
→ Product Opportunity
→ Rights Review
→ Data Resource
→ Dataset / Dataset Version
→ Standardization
→ Entity Resolution
→ Processing
→ Compliance Gate
→ Quality Gate
→ Data Contract
→ Data Product
→ Product Release
```

## 5. 一级模块

正式产品规划：

- 工作台
- 场景中心
- 数据资源
- 数据集
- 数据生产
- 实体中心
- 权利中心
- 数据质量
- 安全合规
- 指标中心
- Data Contract
- 数据产品
- 成本中心
- 证据中心
- 可信流通
- 资产化准备
- 行业模板
- 系统管理

POC 首屏只保留 8 个入口：

- 工作台
- 场景
- 数据资源
- 数据集
- 数据生产
- 实体中心
- 数据产品
- 证据中心

Rights / Quality / Compliance / Contract / Cost 在 POC 阶段作为详情页能力出现。

## 6. 核心业务对象

- Workspace / Project
- UseCase / ProductOpportunity
- DataResource / ResourceBinding
- Dataset / DatasetVersion
- EntityType / Entity / EntityMapping
- Authorization / Policy / Purpose
- Workflow / WorkflowVersion / Task / Execution
- QualityRule / QualityResult
- CompliancePolicy / ComplianceResult
- DataContract / ContractVersion
- DataProduct / ProductVersion / ProductAsset / ProductRelease
- CostEvent
- Evidence / EvidenceSnapshot / AuditEvent

## 7. 核心状态机

### Data Product

```text
DRAFT
→ DESIGNING
→ DEVELOPING
→ TESTING
→ READY
→ PUBLISHED
→ ACTIVE
↔ SUSPENDED
→ DEPRECATED
→ RETIRED
```

### Product Release

```text
DRAFT
→ VALIDATING
→ READY
→ PUBLISHED
├→ SUSPENDED
└→ WITHDRAWN
```

### Dataset Version

```text
CREATED
→ PROCESSING
├→ FAILED
└→ READY
   ├→ INVALID
   └→ SUPERSEDED
```

## 8. Release Readiness

Release 发布前统一检查：

- Production
- Rights
- Quality
- Compliance
- Contract
- Dataset
- Evidence
- Delivery（如适用）

结果：

- READY
- NOT_READY
- REVIEW_REQUIRED

任何关键状态不得通过通用 PATCH status 绕过。

## 9. 第一 Reference Implementation

### Use Case

企业融资风险辅助。

### Data Product

企业经营活跃度 V1.0。

### 输入

- enterprise
- lease
- energy

### 输出 Product Dataset

- company_id
- period
- tenancy_stability
- rent_performance
- energy_stability
- activity_score
- activity_level
- generated_at

### POC 验收

必须真实完成：

1. 创建 Use Case；
2. 注册 Data Resource；
3. 建立基础 Authorization；
4. 导入 Raw Dataset；
5. Company Entity Resolution；
6. 生成 Standardized / Curated Dataset；
7. Compliance Gate；
8. Quality Gate；
9. Data Contract；
10. Data Product Version；
11. Product Release；
12. 查询 Cost 与 Evidence。

## 10. POC 非目标

暂不实现：

- 完整 Billing / Settlement
- 完整 Asset Accounting
- 区块链平台
- 高敏人脸/门禁/视频场景
- 复杂 AI 黑盒评分
- 过早微服务化

## 11. 技术方向

- Frontend：React / Next.js
- Backend：Go
- Control DB：PostgreSQL
- Queue：Redis
- Object Storage：MinIO / S3 compatible
- Metadata Projection：OpenMetadata
- Processing Adapter：Native/Python → Apache Hop
- Entity Adapter：Rules → Splink
- Quality Adapter：Native → Soda/GX
- Compliance Adapter：Rules → Presidio

Core Platform 必须与具体 Engine 解耦。