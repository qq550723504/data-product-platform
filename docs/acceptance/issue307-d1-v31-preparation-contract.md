# #307 D1 candidate21/v31 有限准备 contract

本代只准备剩余 P2/T1/T2/H2A/M1A/M1B 六份目标，P1 永久排除，11 份 survivor、所有历史预算、pins、失败和备份保留。execution 为 `9ba214bd-a481-499b-bef6-ca7a789ef501`；人类已重新选择 2026-10-22 新加坡22:00–22:40（UTC14:00–14:40），只读恢复最晚UTC16:40。原 10月22日18:00 窗口不复用。没有删除授权或定时续跑授权。#332 保持 OPEN/Draft/unmerged，#307 保持 OPEN。

权威范围继续使用 issue307-d1-execution-freeze-boundary.md、现有 remaining_scope.json、remaining_policy_heads.json、remaining_window.py / remaining_six_window.py 与固定 six preparation / complete native / host support 入口。复用现有 Git/Go 构建、GitHub CI/分支保护、PostgreSQL、MinIO 官方 SDK、Asynq/Outbox 与原生校验器，不引入第二套状态机或持久工作流。

本次新增边界只有 candidate20 的失败保留接线与新代 execution/window/独立预算。candidate20 的 public/private terminal、allocation、原12构建成员、原归档、撤销前 profile 与当前撤销态均按原字节固定。其86个owner、network/volume、3个空预算及完整挂载集必须保留；原 schema rehearsal PG 副本继续停止、保留，不升级或重建三源。既有820个保留CID、24个旧预算和57份备份仍走原门禁，历史 IAM UNCONFIRMED 不改写。

candidate20 没有运行 native preparation、没有 provision 六个 IAM 用户。新代须按官方 NoSuchUser 对这六个旧用户名重新证明不存在，不能伪造为已创建的 disabled 用户；既有实际用户与 sole policy/no groups、c13 dormant NONE、旧冻结策略继续原样核对。

R/P 仍是两个显式不可变制品：R从既有 schema69 fixture runtime 派生，仅改变本代 identity/window/实际 absence 要求；P 保留当前产品 schema 和 protected identity。R archive 使用已修复 UTC/LF/store-only recipe，在独立目录构建；三源不迁移，无 Processing/DirectData/P1 重跑、无 Renew/新Rights。新source须独评且同HEAD全部必要PR CI通过后，才分配新资源。

有限顺序为：保留门禁与新代源码/跨字段回归 → 源码独评/精确CI → 新 allocation/materialize/独立预算初始化 → 三源/分离 restore/业务副本非删除验收 → 固定完整入口17版本/完整库存与保护/IAM/Core前后指纹核验 → 冻结全部绑定/独评 → 最终静态HEAD CI → 原生锁定分支及锁后execution binding → 完整精确确认文本。锁后回执追加保存，不能再推进锁定HEAD。删除和调度仍需后续新的人类直接授权。

本代验收只认实际证据：文件/回归/CI/副本证明不能代替源核验，NOT_RUN 不写 PASS，失败不绕过门禁续跑。新增对旧失败 facts、owner拓扑/启动状态、预算归属/UID/字节/挂载扩张、配置扩张/启用、缺失或篡改证据的反例均需正常/-O通过。独立评审只关闭当前有限必需路径 blocker；新产品生命周期、源schema升级及其他环境不在本次范围。
