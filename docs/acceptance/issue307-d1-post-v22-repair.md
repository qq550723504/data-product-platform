# #307 D1 v22失败后的有限修复边界

本轮仅修复执行组合与错误留证，并完成非删除验证。v22 execution `8e41cfba-a41e-44dd-9ce8-d6365b3bb90e` 已终止，原授权不复用；#323 Draft、不合并，#307 OPEN。

唯一业务事实来源仍为Core；完整库存、精确absence、实际调用及成本沿用现有Lifecycle Port、MinIO Adapter、Transactional Outbox与Asynq。没有新workflow runtime、并行队列、policy engine或新的处置状态。原v22手册、候选、二进制和所有失败/预算证据保持历史字节，不修订成仿佛原来正确。

## 当前阻断与修复

- R1：下一份发送前，必须先有前序精确版本的Core absence回执。操作顺序为单份send → 关闭发送 → 既有只读recovery → 验证回执 → 下一份。前序未确认时在native inspection与持久预算之前拒绝；Core的fence与完整库存门禁照常执行，不能用204、UNKNOWN、取消或耗尽预算代替absence。
- R2：保存原始authority/budget/Core claim拒绝错误。claim拒绝时不得查询不存在的provider observation来覆盖原错误，不补造provider invocation、NOT_SENT或Cost事实；预算已消耗也不回收。真实调用后观察持久化失败必须保留transport原观察和原错误，继续禁止重发UNKNOWN。

## 验证与复用

复用现有Go `errors`、pgx、MinIO SDK、现有exact-send/absence/recovery服务与测试夹具。不引入通用编排器。验证包含多份顺序成功、已删除但未对账的反例、旧/异租户absence拒绝、claim拒绝及预算不重用、响应丢失和观察存储失败；模拟provider明确标识，不冒充原生删除验收。真实PG仅使用本轮独立scratch数据库，不启动已封存源来做回归。

新授权前可以准备P1的受限只读核验/对账接线和剩余六份方案；不能重新启用旧case/sender、恢复旧execution或再执行DELETE。P1只有真实204与ACCEPTED，仍需精确库存与Core absence对账；未来方案必须按新库存固定六份待发送目标，不能复用旧七份整批。新删除execution、候选与窗口在修复/非删除验证/独立复评闭合后另行冻结、逐项核对并请求人类新授权。

本文件为当前修复切片边界；原窗口执行矩阵与终态见 `issue307-d1-v22-execution01.md`。独立评审仅对这些安全、历史、必需成功路径及回归中的有效阻断收敛。
