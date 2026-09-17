# 实体解析引擎边界

## 决策

核心平台拥有权威的 `Entity`、`EntityMapping`、`MatchJob`、人工审核（Human Review）、Evidence 以及匹配策略历史。

Splink 一类的概率化系统**仅作为候选生成器**。它们可以针对已有权威实体提出带排序的关联建议，但不得创建、合并或持久化权威状态。

## 决策顺序

对于 COMPANY 的解析，核心决策流水线为：

```text
强标识符规则
  -> 归一化后的确定性规则
  -> 可选的概率化候选引擎
  -> 策略阈值
  -> AUTO_MATCH / REVIEW / UNRESOLVED
  -> 核心 EntityMapping / 人工审核
```

因此，统一社会信用代码完全一致时仍具有权威性，永远不会被概率得分所覆盖。

## 提供方中立的契约

`internal/entity/resolution` 定义：

- `MatchRecord`
- `ReferenceRecord`
- `CandidateRequest`
- `Candidate`
- `EngineDescriptor`
- `CandidateGenerator`
- `Registry`

提供方配置、训练数据、分块（blocking）策略、SQL 方言、模型文件以及运行时相关的载荷，都必须保留在核心 Entity 领域之外。

## 溯源

候选结果与被接受的映射会持久化记录：

- 策略版本（policy version）
- 匹配方法 / 规则
- 归一化后的置信度得分
- 引擎名称
- 引擎版本
- 模型版本

这使得概率化决策可审计，同时又不让外部引擎成为真相来源。

## 失败 / 停用语义

既有的纯规则路径不依赖任何概率化引擎。当未配置候选生成器时，既有的确定性行为保持不变。运行时相关的降级/错误行为由适配器与应用策略实现，而非由权威 Entity 状态实现。

## 当前 POC 边界

第一个通用实现将活跃的权威实体枚举为参照集（reference set）。对于参照用 POC 而言这是刻意保持简单的做法。生产规模的分块/索引属于适配器/运行时的关注点，可以在不改变 `Entity` 或 `EntityMapping` 模型的前提下替换。
