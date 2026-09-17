# ADR-0004：外部能力通过 Engine SPI 接入

- 状态：已接受

## 背景

平台可能使用 OpenMetadata、Apache Hop、Python、Spark、Splink、Soda、Great Expectations 或 Presidio，但这些组件都不应定义核心业务语义。

## 决策

核心只依赖以下端口（port）：

- MetadataEngine
- ProcessingEngine
- EntityResolutionEngine
- QualityEngine
- ComplianceEngine
- EvidenceStore

具体产品以适配器方式实现。

## 后果

- 各引擎可独立替换。
- 必须提供适配器契约测试（adapter contract tests）。
- 核心表只把外部执行/引用 ID 作为外部引用存储。
- 引擎专属配置应尽可能保留在核心领域对象之外。
