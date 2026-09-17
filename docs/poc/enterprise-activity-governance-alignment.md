# 企业经营活跃度治理对齐（Enterprise Activity Governance Alignment）

本说明记录原生企业经营活跃度处理输出与已冻结的 V1 Data Contract / Quality / Compliance 策略之间必须完成的对齐。

## 必需的产品 schema

原生 worker 产出的 CURATED Product Dataset 必须使用 Data Contract 中的字段名：

- `company_id`
- `company_name`
- `period`
- `tenancy_stability`
- `rent_performance`
- `energy_stability`
- `activity_score`
- `activity_level`
- `indicator_coverage`
- `generated_at`

历史遗留的处理别名（`canonical_company_id`、`target_period`）可以被读取方临时接受，但不得作为权威的 Product Dataset schema。

## 负值能耗隔离（Negative energy quarantine）

对于 `energy_kwh < 0` 的 RAW energy 记录，必须：

1. 以原因 `NEGATIVE_ENERGY_KWH` 持久化到 `execution_quarantine_record`；
2. 从指标所用的已接受能耗读数中排除；
3. 计入 `quarantineCount`；
4. 不增加"已接受负值能耗率"（accepted-negative-energy rate）。

输出的 DatasetVersion 处理元数据必须包含：

- `unresolvedEntityRate`
- `acceptedNegativeEnergyRate`
- `quarantineCount`
- 确切的 WorkflowVersion
- 确切的 Indicator Set 版本
- 确切的 Entity Matching Policy 版本

对于一次成功完成的原生参考工作流，若所有源企业记录均已解析、且负值能耗在指标计算前已被隔离，则：

```text
unresolvedEntityRate = 0
acceptedNegativeEnergyRate = 0
```

## 生成时间戳

`generated_at` 是发布候选（release candidate）的产品元数据。它可以在不同执行之间变化；冻结的 DatasetVersion 校验和（checksum）使得具体的生成产物可复现、可审计。

## 门禁要求

Sprint 3.2 引入的真实 Quality 与 Compliance 门禁必须能够直接针对 worker 产出的 CURATED DatasetVersion 运行，而不需要兼容性 mock。必须在完成本对齐之后，ReleaseReadiness 才能将某个 release 迁移到 `READY`。
