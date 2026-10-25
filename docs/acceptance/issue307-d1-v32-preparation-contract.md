# #307 D1 candidate22/v32 准备边界

人类在本聊天选择的固定窗口为2026年10月23日新加坡18:00–18:40（UTC10:00–10:40），只读恢复最晚UTC12:40。拟用新 execution `0e5cb8f3-46a9-4339-88e4-7154d6ebe13f`。这里只记录准备意图，不构成精确冻结方案、删除授权或调度授权。

剩余范围仍为P2→T1→T2→H2A→M1A→M1B，共六份exact版本；P1永久排除，十一份survivor、所有环境、备份、旧预算、pins及失败/UNKNOWN/成本事实保留。每份最多一次，全局最多六次，UNKNOWN不重发；cleanup=0、hold-release=0、bypass=0。#332保持OPEN/Draft/unmerged，#307保持OPEN。

## 本次增量

继续使用[execution/分支冻结边界](issue307-d1-execution-freeze-boundary.md)和既有固定完整入口，不增加新工作流或策略基础设施。candidate21的修复、既有独立评审及证据保持原字节。新代只更新必须随execution/window变化的编译契约和IAM命名、显式审计实际已创建的candidate21旧身份、保留已闭合candidate21的资源/预算/备份，并分配独立新预算。R/P仍是两个单独冻结制品，R仅针对实际schema69 fixture，P保留当前产品语义；不迁移三源，不Renew、不新增Rights、不重跑Processing/DirectData或P1。

旧candidate21的固定窗口和恢复期限均已过期，未收到该代删除或调度授权。已通过原固定sealed gate及996个已知CID的宿主只读闭合，30份permission完整冻结字段false、三个该代预算为空、27个更早预算和66份备份均保留。新终态为`EXPIRED_UNAUTHORIZED_FROZEN_PREPARATION_CLOSED`，public/private终态SHA相同：`5568708e62c4ad37f8765aba667697de547d83fe0e598d59d7d4341a84b4b188`。现有state()会在读allocation前拒绝恢复已终态准备。未声称本轮新鲜观察了native IAM或17版本，源服务没有启动。

## 发布边界

旧最终HEAD仍为`f4f27c789f53b6c45910f55f83bee6cabfdee4f6`，旧execution-binding仍为`f27a617f278fcbaf64db29ddf536d5816d88acaba8a0d760280eb23bbe55d0de`。本次只在独立本地分支`codex/issue307-candidate22`记录准备，不推进旧锁定HEAD。

现有精确原生rule为`BPR_kwDOUdQLBs4FE_7b`。任何实际分支维护前仍须重新读取并逐字段比较HEAD/ruleID/完整保护，确认旧终态、物理闭合和无调度，由可信保护owner显式批准后，只解除`lockBranch`这一冻结位，其余保护字段保留；不删除rule、不force push、不关闭管理员约束、不允许fork-sync或删除。该维护许可与后续删除授权独立。

当前GitHub连接已实时确认旧HEAD、Draft/Open/unmerged及其13个PR CI jobs仍成功；这是旧HEAD的PR CI，不是新代source CI或main CI。管理保护读取在connector返回403（integration缺少administration），本地gh管理路径TLS超时。因此原生保护当前状态标记UNCONFIRMED；不能用历史保护回执冒充本次fresh通过，或在未知保护下解锁。原始失败分别保留。

受控分支维护完成后，才推进以下步骤：新代有限R/P接线及回归/独立评审→精确新source PR CI→独立资源和预算→三组原生非删除/隔离恢复验收→完整17版本/保护/IAM/Core只读核验→全部绑定冻结及独立评审→最终HEAD精确CI→原生重新锁定并写新execution-binding→完整精确确认文本。新source CI前不构建或分配资源，不以旧CI替代。完整方案完成后仍需人类新删除授权，未来调度另按人类指令建立。

本代allocation、原生验收、冻结、完整精确文本、删除与调度均尚未完成；不得把本准备contract称为冻结候选或PASS完整D1。
