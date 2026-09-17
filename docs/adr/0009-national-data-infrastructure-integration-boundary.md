# ADR-0009：国家数据基础设施（NDI）集成边界

- 状态：已接受
- 日期：2026-09-16

## 背景

Data Product Platform 最终会将已发布的数据产品发布到外部数据基础设施生态中，包括中国的国家数据基础设施（NDI）、可信数据空间、数据交易所及其他分发渠道。

NDI 提供主体身份、标识符、目录注册、基于连接器的交付、跨节点互操作等外部互操作能力。这些能力是重要的外部集成目标，但绝不能成为核心平台的真相来源。

核心平台始终负责数据产品生产与治理的业务真相：

- DataResource 与 DatasetVersion
- Entity 与 EntityMapping
- Authorization / Rights
- Workflow / Execution
- 质量与合规决策
- DataContract
- DataProduct / ProductVersion / ProductRelease
- Cost / Evidence / Audit

## 决策

NDI 被视为**外部互操作层**。

核心平台仍然是数据产品生产与生命周期状态的**真相来源（System of Record）**。

NDI 专属的 schema、标识符、凭证、连接器状态与 API 契约不得泄漏到核心领域实体中。它们通过适配器与通用绑定（generic binding）访问。

首选集成模型为：

```text
Data Product Platform
        │
        │ ProductRelease / Domain Events
        ▼
外部集成层
        │
        ├── NDI Adapter
        ├── 可信数据空间 Adapter
        ├── 数据交易所 Adapter
        └── 其他未来适配器
```

## 通用外部模型

核心应使用提供方中立的概念，而不是 NDI 专属字段。

### 外部身份绑定（External identity binding）

将内部 Subject / Organization 映射到外部基础设施身份。

建议结构：

```text
provider
subject_id
external_subject_id
credential_ref
registration_node
status
metadata
```

### 外部标识符（External identifier）

将内部业务对象映射到外部标识符。

建议结构：

```text
provider
object_type
object_id
external_id
external_uri
status
registered_at
metadata
```

支持的对象类型可能包括：

- SUBJECT
- CONNECTOR
- DATA_RESOURCE
- DATA_PRODUCT
- PRODUCT_RELEASE

### 外部发布（External publication）

跟踪某个 ProductRelease 向外部基础设施的发布过程。

建议生命周期：

```text
DRAFT
  ↓
SUBMITTING
  ↓
REGISTERED
  ↓
PUBLISHED
  ├── SUSPENDED
  └── WITHDRAWN
```

## 策略与执行边界

平台拥有业务策略：

- 谁可以使用某个产品；
- 出于何种目的；
- 暴露哪些字段 / 资产；
- 哪个 ProductRelease 是有效的；
- 使用约束与有效期。

外部连接器/基础设施可以执行传输与访问控制：

- 身份认证；
- 网络访问；
- 交付；
- 连接器侧访问控制；
- 运行时 / 使用日志。

连接器日志作为下游 Evidence 导入，但不替代平台的生产 Evidence Graph。

## 适配器接口

集成层可以演进为提供方中立的端口，例如：

```text
ExternalIdentityProvider
ExternalIdentifierProvider
CatalogPublisher
DeliveryConnector
UsageEventSource
```

NDI 专属实现放在适配器目录下，例如：

```text
adapters/ndi/
```

## POC 影响

NDI 集成**不**阻塞核心 POC。

Sprint 1-3 继续聚焦于：

```text
DataResource
→ DatasetVersion
→ Entity Resolution
→ Workflow
→ Quality / Compliance / Rights
→ DataContract
→ ProductRelease
→ Evidence / Cost
```

NDI 集成探针（integration spike）应在 ProductRelease 稳定之后才开始。

## 后果

- 核心业务模型独立于 NDI 协议的演进。
- 同一个 ProductRelease 可以发布到多个外部生态。
- 外部 ID 永不替代内部稳定 ID。
- NDI 集成可以独立测试或替换。
- 后续可在不重新设计核心领域的前提下采纳正式的 NDI 规范与测试床要求。
