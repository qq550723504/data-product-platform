# 企业活跃度参考实现 V1.0

## 1. 目的

参考 Use Case：**企业融资风险辅助**。

参考 Data Product：**企业经营活跃度 V1.0**。

目标不是构建银行授信模型，而是用一个可解释、可版本化的场景验证完整 Data Product 生产链：

```text
Raw Data
→ Data Resource
→ DatasetVersion
→ Entity Resolution
→ Processing
→ Compliance Gate
→ Quality Gate
→ Data Contract
→ Data Product Version
→ Product Release
→ Evidence + Cost
```

> V1 的指标、权重和等级阈值仅用于 POC 语义验证，不是经过真实金融数据验证的信用评分、授信决策或违约概率模型。

## 2. 权威规范

实现时以下文件是 V1 的规范来源：

- 企业匹配策略：`industry-packs/park/matching/company-match-policy-v1.yaml`
- 指标集：`industry-packs/park/indicators/enterprise-activity-v1.yaml`
- 人类可读的指标公式：`examples/enterprise-activity/indicators-v1.md`
- 生产工作流：`examples/enterprise-activity/workflow/workflow-v1.yaml`
- Data Contract：`examples/enterprise-activity/contract/data-contract-v1.yaml`
- Park 质量规则：`industry-packs/park/quality/`
- Park 合规规则：`industry-packs/park/compliance/`

若代码中的常量、缺失值处理或公式与上述版本化规范冲突，应修改代码而不是静默修改产品语义。

## 3. 输入数据源

### `enterprise.csv`

```text
source_company_id
company_name
unified_social_credit_code
legal_representative
registered_address
entry_date
company_status
contact_name
mobile
email
```

### `lease.csv`

```text
lease_id
source_company_id
company_name
contract_start
contract_end
rent_amount
payment_due_date
payment_date
lease_status
```

### `energy.csv`

```text
meter_id
source_company_id
company_name
reading_time
energy_kwh
```

POC fixtures 刻意包含脏数据情形，例如：

- 企业名称不一致
- 缺失统一社会信用代码
- 跨文件不一致的源系统 ID
- 日期不一致
- 无效 / 负值能耗记录
- 逾期或未付租金事件
- 别名 / 企业简称

这些记录的存在是为了验证标准化、实体匹配、隔离（quarantine）、Quality 与 Evidence 行为。

## 4. 企业实体解析 V1

权威输出键：

```text
canonical_company_id = COMPANY-xxxxxx
```

初始策略：

1. 统一社会信用代码完全一致 → `AUTO_MATCH`，置信度 1.0。
2. 归一化企业名称完全一致 + 归一化地址完全一致 → 高置信度匹配。
3. 企业名称相似 + 法定代表人一致 → `REVIEW` 候选。
4. 其他情况 → `UNRESOLVED`，直到后续的引擎/人工决策将其解析。

每一条非平凡映射都会记录源身份、权威实体、匹配方法、策略版本、置信度、复核状态与证据。RAW 数据绝不会被重写以强制完成匹配。

## 5. 指标语义

V1 产品包含四个业务指标：

- `tenancy_stability`
- `rent_performance`
- `energy_stability`
- `activity_score`

确切的公式与参数在 V1 指标规范中冻结。重要语义：

- 当不满足最小观测数时，各分项得分可以为 `null`；
- 缺失的分项不会被静默填充为零或中性值；
- 在 V1 中，`activity_score` 要求三个分项指标全部可用；
- 当必需分项不可用时，`activity_score=null` 且 `activity_level=INSUFFICIENT_DATA`；
- `indicator_coverage` 报告"可用必需分项数 / 3 × 100"；
- 负值能耗观测会被隔离，而不是被改写为零。

## 6. 产品数据集

V1 输出 schema：

```text
company_id
period
tenancy_stability          nullable
rent_performance            nullable
energy_stability            nullable
activity_score              nullable
activity_level              non-null
indicator_coverage          non-null
generated_at                non-null
```

Product Dataset 不得暴露不必要的原始个人/联系信息或源级字段，例如 `contact_name`、`mobile`、`email`、`legal_representative`、`registered_address`、原始 `rent_amount` 或 `meter_id`。

## 7. 质量与合规

质量与企业活跃度得分是两个不同的概念。一个 Dataset 可以包含活跃度低的企业，同时仍具有极好的数据质量。

Quality Gate 评估数据适用性（身份完整性、映射率、得分范围、新鲜度等）。Compliance Gate 评估允许的输出与最小必要数据处理。

阻塞性的 Quality/Compliance 决策会阻止 Product Release 就绪；它不会就地修改历史 DatasetVersion。

## 8. Data Contract

参考契约当前定义：

- 消费者：持牌银行与担保机构；
- 目的：企业经营状况 / 信用风险支持；
- 新鲜度目标：每日，最大延迟 24 小时；
- 交付：允许 API，禁止 dataset/raw-source 导出；
- 禁止转售与营销用途；
- V1 输出 schema 与缺失数据语义；
- 产品不得作为唯一的授信决策依据。

## 9. 生产流程

```text
enterprise / lease / energy RAW DatasetVersions
→ 标准化
→ 企业实体解析 + 人工复核
→ STANDARDIZED DatasetVersions
→ 能耗月度聚合
→ V1 指标计算
→ CURATED activity DatasetVersion
→ Compliance Gate
→ Quality Gate
→ 已发布的 Data Contract V1
→ Data Product Version 1.0.0
→ Product Release
→ EvidenceSnapshot + Cost Events
```

核心 POC 刻意不依赖 OpenMetadata、Apache Hop 与 Splink。它们是后续用于验证 Engine SPI 架构的适配器。

## 10. 验收问题

完成的参考实现必须能够回答：

1. 某个企业的结果由哪些 RAW 记录与 DatasetVersion 产出？
2. 为什么这些源企业记录被匹配到同一个权威 Entity？
3. 使用了哪些实体策略、指标集与工作流版本？
4. 哪些技术记录被隔离，原因是什么？
5. Quality 与 Compliance 为什么通过/失败？
6. ProductRelease 中冻结了哪些确切的 DatasetVersion 与 Data Contract？
7. 有哪些 Evidence 支撑该 Release？
8. 记录了哪些 Cost Events？

## 11. POC 待办

核心 POC 由 GitHub Epic `#1` 跟踪。参考语义通过 `#12` 中的 spec-first 工作及相关实现 issue 完成冻结。
