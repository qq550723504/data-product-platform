# #307 D1 v19 当前有限准备合同

本切片继承 [v18 完整只读核验设计](issue307-d1-v18-preflight-contract.md) 的策略语义、精确库存、400回执、actual mount/owner/权限、Core前后、逐项封存与离线回放 invariant；以本文件和 candidate09/v19 为当前提案。v17失败、v18未授权冻结快照及完整CLI失败均保留。

v18 生成后，真实CLI验收发现相对输出目录未归一化，Docker在创建GET-only helper时拒绝挂载；该次没有原生请求、helper发送或删除，已启动的Core/native均关闭，原失败与physical-close见 `remaining-preflight-native01`。这是宿主入口缺陷。现CLI和库入口都在任何source启动前归一化全部路径，并增加完整CLI相对路径反例；不靠操作员记忆使用绝对路径规避。

新唯一编译期contract为 **candidate09/v19**，execution **9780707d-55eb-4da8-873f-3a901d7aac3f**。拟窗口仍为 **2026-10-18 02:00–02:40Z（新加坡10:00–10:40）**，尚未获得人类确认、尚未调度。新key、binary、owner、预算和precise payload重新冻结；旧v17授权以及未授权的v18快照均不得使用。18版本、7目标、11保留和9份备份的范围不扩大。

独立评审通过当前修复后才准备candidate09。准备入口先检查07与08两组旧owner实际mount/关闭权限和PID0，再新建09；完整CLI必须在09准备后实际成功一次，不能仅凭collect_group或离线测试声称完整入口已通过。失败输出一次性保留；只读技术验收可用新目录重新核验，但不改变任何发送预算/业务事实，不重新启用权限。

当前仍为非删除准备：不Renew、不运行真实业务case、不enable、不materialize human-confirmation、不签发送topology、不DELETE、不hold-release、不bypass、不合并。#323 Draft、#307 OPEN，完整D1仍NOT_ACCEPTED。旧heartbeat保持PAUSED，不创建新调度。源码/离线、原生准备、完整CLI与最终绑定评审分别留证，不能互相冒充。
