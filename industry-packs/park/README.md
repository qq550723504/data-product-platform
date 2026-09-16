# Park Industry Pack

园区行业包是 Data Product Platform 的第一个 Reference Industry Pack。

它不是 Core Platform 的边界，而是验证 Industry Pack 机制的第一套行业实现。

## Planned Contents

```text
industry-packs/park/
├── entities/
├── glossary/
├── matching/
├── indicators/
├── quality/
├── compliance/
└── products/
```

## Initial Entity Types

- COMPANY
- PARK
- BUILDING
- METER
- EQUIPMENT

POC 第一阶段主要使用 `COMPANY`。

## Company Strong Identifier

第一优先级标识：统一社会信用代码。

首版匹配策略：

1. 统一社会信用代码完全一致 → AUTO_MATCH
2. 标准化企业名称 + 地址一致 → AUTO_MATCH / high-confidence candidate
3. 企业名称相似 + 法定代表人一致 → REVIEW
4. 其他情况 → UNRESOLVED

具体阈值应通过 POC 数据校准，不在 Core 中硬编码。

## Initial Domains

- ENTERPRISE
- LEASING
- ENERGY
- PROPERTY
- EQUIPMENT
- SPACE
- SECURITY
- PARKING

## Initial Product Templates

第一批候选模板：

- enterprise-activity（企业经营活跃度）
- enterprise-performance（企业履约能力）
- carbon-account（企业碳账户）
- industry-map（园区产业图谱）
- space-efficiency（空间设施效率）
- predictive-maintenance（设备预测维护）

POC 只实现 `enterprise-activity`。

## Pack Contract

Industry Pack 可以提供规则与模板，但不得改变：

- DatasetVersion 不可变原则
- Product Release 生命周期
- Rights / Cost / Evidence 核心语义
- Engine SPI
- Core API 状态机
