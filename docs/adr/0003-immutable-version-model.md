# ADR-0003：带版本的生产事实不可变

- 状态：已接受

## 背景

平台必须能够复现历史产品发布，并证明发布当时使用了哪些数据、规则与契约。

## 决策

以下对象一旦冻结/发布，即视为不可变历史：

- DatasetVersion
- WorkflowVersion
- ContractVersion
- ProductVersion
- ProductRelease
- EvidenceSnapshot

修正错误的方式是创建新的版本/发布，而不是编辑历史事实。

## 后果

- 历史发布保持可复现。
- 审计与 Evidence 保持可信。
- 存储会随时间增长，需要保留/归档策略。
- API 必须暴露显式的"创建版本"语义，而不是通用更新语义。
