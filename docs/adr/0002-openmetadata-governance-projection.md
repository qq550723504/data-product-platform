# ADR-0002：OpenMetadata 是治理投影，而非真相来源

- 状态：已接受

## 背景

OpenMetadata 擅长技术元数据、血缘、术语表（Glossary）、分类、Owner 归属与治理视图，但平台同时还需要 DatasetVersion、Rights、生产 Workflow、Cost、Evidence 与 ProductRelease 等语义。

## 决策

核心平台数据库是数据产品生产与生命周期状态的真相来源（System of Record）。

OpenMetadata 通过 `MetadataEngine` 与 `ResourceBinding` 以治理投影（Governance Projection）的方式接入。

## 后果

OpenMetadata 可以拥有或投影：

- 物理资产元数据
- 技术血缘
- 术语表 / 分类
- 数据产品的治理视图

OpenMetadata 不得拥有：

- DatasetVersion
- Authorization / Rights 状态
- 生产 Workflow 的真相
- Cost Ledger（成本账本）
- Evidence
- ProductRelease 状态

OpenMetadata 故障不得回滚一个有效的核心 Product Release。
