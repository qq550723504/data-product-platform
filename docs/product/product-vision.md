# 产品愿景

## 产品定位

Data Product Platform 是一套通用的数据产品生产与治理平台。

它不是单纯的数据目录、ETL、可信数据空间或数据入表工具，而是围绕 Data Product 生命周期统一管理：

- Use Case
- Rights / Authorization
- Data Resource
- Dataset / Dataset Version
- Entity Resolution
- Workflow / Execution
- Quality / Compliance
- Data Contract
- Data Product / Product Version / Product Release
- Cost / Evidence / Audit

## 核心问题

平台必须能回答：

1. 为什么生产这份数据？
2. 是否有权以当前 Purpose 执行当前 Action？
3. 使用了哪些数据、规则、算法与版本？
4. 数据是否通过质量与合规门槛？
5. 产品如何发布、暂停、撤回和复现？
6. 生产成本来自哪里？
7. 支撑每个关键结论的证据在哪里？

## 核心链路

```text
Use Case
→ Rights Gate
→ Data Resource
→ Dataset
→ Standardization
→ Entity Resolution
→ Transformation
→ Compliance Gate
→ Quality Gate
→ Data Contract
→ Data Product
→ Product Release
```

Product Release 后形成两个独立出口：

```text
Product Release
├── Trusted Data Offering → 可信数据空间 / 数据流通
└── Assetization Case → 数据资源资产化准备
```

两个出口共享前面的数据产品生产能力，但解决不同问题：

- Trusted Data Offering：解决谁可以在什么规则下使用数据产品；
- Assetization Case：组织权利、成本、质量、证据，支撑专业会计与审计判断。

## 产品结构

```text
Industry Solution Layer
    ↑
Industry Packs
    ↑
Data Product Core
    ↓
Engine Adapter Layer
    ↓
Metadata / Processing / Entity / Quality / Privacy Engines
```

## 通用 Core 与行业包

Core Platform 不包含园区专属业务词汇。

第一行业包为 `Park Industry Pack`，包含：

- Company / Park / Building / Meter 等 Entity Type
- 园区领域 Glossary
- 企业实体匹配策略
- 能耗、租赁等标准化规则
- 园区指标模板
- 企业经营活跃度等 Data Product Template

未来可增加制造、政务、金融、医疗等 Industry Pack，而不修改核心 Domain。

## 第一 Reference Implementation

- Use Case：企业融资风险辅助
- Product：企业经营活跃度
- Source：企业基础信息、租赁、能耗
- 目标：跑通 `Raw Data → Product Release → Cost/Evidence` 全链路

POC 的目标是验证业务模型，不是验证某个开源组件能否部署。