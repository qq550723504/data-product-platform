# T3/B2：Execution 固定生产依赖与映射使用关系

Issue：#128

## 目标

生产计算必须消费本次 Execution 已经固定的解析输出、映射决定和策略内容。Release trace 只能沿同一组持久化事实回溯，不能在发布时重新读取当前 `entity_mapping`。

## 绑定模型

`execution_dependency_preparation` 是一组绑定的提交边界。只有同一事务中已经写入依赖、映射使用关系、证据、审计和 Outbox 事实，并最终写入 `PREPARED` 行后，这组绑定才有效。

- `execution_dependency_binding` 保存具名依赖：`enterprise_resolution`、匹配策略快照、指标策略快照，以及 DatasetVersion、版本、原始内容和 SHA-256。
- `execution_mapping_usage` 保存一个 Execution 对某个输入来源实际使用的 `decision_id` 和 `entity_id`。它与决定产生者的 `source_job_id` 分开。
- `entity_resolution_output_decision` 把 STANDARDIZED 解析输出中的源键绑定到当时产生该输出的不可变决定。
- `entity_match_job` 保存解析任务使用的匹配策略内容快照。旧任务没有快照时，不能作为新生产的可验证解析依赖。

`enterprise_resolution` 必须是明确传入的 STANDARDIZED DatasetVersion，并由持久化的成功解析任务证明它来自同一 workspace、`enterprise_raw` 和来源身份。没有唯一证明时拒绝，不按最新记录猜测。

## 执行流程

1. 第一次准备加载明确的解析输出决定和策略文件内容，并检查 workspace、输入版本、来源范围及成功任务。
2. 租赁和能耗来源优先复用现有合法决定；缺失时通过 `RecordMappingDecision` 以稳定操作键创建 `WORKFLOW_ALIAS` 决定。
3. 在 Execution 级 advisory transaction lock 下再次检查准备行，写入全部依赖和 usage，最后写 `PREPARED`。并发请求复用已提交的一组绑定。
4. native 计算只读取准备阶段恢复出的映射和策略快照，不查询当前映射，也不在计算阶段创建决定。
5. CURATED DatasetVersion 的 lineage 同时包含三个 RAW 输入和确切的 `enterprise_resolution`；Release trace 通过 generated Execution 的 usage 读取决定。

## 历史与门禁

新生产若声明了解析依赖但没有完整 `PREPARED` 绑定，Production readiness 拒绝。旧输出没有可证明的 usage 时，trace 显示历史缺口，不回填当前映射，也不改写旧快照或哈希。

## 非目标

本切片不实现 NativeReconciler、执行租约接管、崩溃自动重跑、完整输出幂等、对象提交协议或死信重放。
