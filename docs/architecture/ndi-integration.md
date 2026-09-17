# NDI 集成架构

## 1. 定位

Data Product Platform 的定位是**数据产品生产与治理能力平台**，它作为业务节点能力参与外部数据基础设施生态，而不是自身实现一个区域级/全域级的基础设施节点。

平台应当先产出稳定的 `ProductRelease`，再通过外部适配器将其发布出去。

```text
原始 / 源数据
      ↓
Data Product Platform
      ↓
ProductRelease
      ↓
外部集成层
      ├── NDI
      ├── 可信数据空间
      ├── 数据交易所
      └── 其他渠道
```

## 2. 分层架构

```text
┌──────────────────────────────────────────────┐
│ 外部数据基础设施                             │
│                                              │
│ NDI / 可信数据空间 / 数据交易所              │
│ 身份 · 标识 · 目录 · 连接器                  │
└──────────────────────▲───────────────────────┘
                       │ 适配器
┌──────────────────────┴───────────────────────┐
│ Data Product Platform                        │
│                                              │
│ UseCase · Rights · Dataset · Entity          │
│ Workflow · Quality · Compliance · Contract   │
│ Product · Release · Cost · Evidence          │
└──────────────────────┬───────────────────────┘
                       │ 治理投影
┌──────────────────────▼───────────────────────┐
│ 元数据 / 治理引擎                            │
│ OpenMetadata 及未来的替代实现                │
└──────────────────────────────────────────────┘
```

各层回答的是不同的问题：

- 元数据引擎：**存在哪些数据，以及这些数据在技术上如何被治理？**
- 核心平台：**数据如何被生产为一个受治理的数据产品？**
- 外部基础设施：**一个已发布的产品如何被跨组织地标识、发现、交付和使用？**

## 3. 核心对象到外部的映射

| 核心对象 | 外部关注点 | 映射规则 |
| --- | --- | --- |
| Subject / Organization | 外部身份 | 绑定，绝不替换内部 ID |
| DataResource | 外部资源标识 / 目录条目 | 通过适配器发布 |
| DataProduct | 产品目录描述 | 仅投影元数据 |
| ProductRelease | 可发布的具体版本 | 发布单元 |
| Authorization / Rights | 使用资格 | 外部使用策略的来源 |
| DataContract | Schema / SLA / 交付契约 | 映射契约中的选定字段 |
| Evidence | 运行时 / 发布证据 | 将外部日志导入为证据 |

## 4. 发布流程

```text
ProductRelease = PUBLISHED
        ↓
ExternalPublicationRequested
        ↓
Provider Adapter
        ↓
身份 / 标识校验
        ↓
目录注册
        ↓
连接器 / 交付配置
        ↓
ExternalPublication = PUBLISHED
        ↓
使用事件 / 连接器日志
        ↓
Evidence Graph
```

外部发布过程不得修改原始 ProductRelease 快照。

## 5. 身份与标识模型

内部 ID 在平台内部始终保持权威地位。

示例：

```text
内部产品 ID
DP-ACTIVITY-001

内部 Release ID
REL-2026-001
```

外部标识符单独存储：

```text
provider = NDI
object_type = DATA_PRODUCT
object_id = <内部 UUID>
external_id = <提供方标识符>
```

这使得同一个对象可以同时拥有多个外部标识符。

## 6. 策略与执行

核心平台拥有策略定义权：

```text
Subject
+ ProductRelease
+ Purpose
+ Action
+ Scope
+ Constraints
= Decision
```

外部连接器可在交付过程中执行该决策结果。

因此，连接器是执行/运行时组件，而不是业务授权的真相来源。

## 7. 证据集成

外部基础设施产生的证据被追加到既有 Evidence Graph 中。

```text
Rights Evidence
      ↓
Dataset / Processing Evidence
      ↓
Quality / Compliance Evidence
      ↓
ProductRelease Evidence
      ↓
External Registration Evidence
      ↓
Connector Usage Evidence
```

外部运行时日志不会替代上游的生产证据。

## 8. 实施阶段

### 核心 POC —— 不依赖 NDI

继续 Sprint 1-3，不以外部集成为阻塞条件。

### 集成探针（Integration Spike）

在 ProductRelease 稳定之后，实现提供方中立的模型：

- `external_identity_binding`
- `external_identifier`
- `external_publication`
- 适配器端口（adapter ports）

随后，依据实施时点可获得的正式接口规范/测试环境，实现 `adapters/ndi` 的概念验证。

### 生产集成

只有在具备正式接口文档并通过测试床验证之后，才最终确定 NDI 专属的生产行为。

## 9. 非目标

本项目目前不打算实现：

- 一个 NDI 全域节点；
- 一个区域级/行业级功能节点；
- 一个标准接入连接器的替代品；
- 一个 NDI 专属的核心领域模型。
