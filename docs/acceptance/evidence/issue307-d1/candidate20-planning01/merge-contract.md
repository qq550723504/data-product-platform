# candidate20/v30 有限准备 contract

本轮人类仅选择 2026-10-22 新加坡 18:00–18:40（UTC10:00≤now<10:40），用于准备新的精确确认方案。新 execution 为 5a54e01a-657e-4b8a-ae67-a2d6fd5be8f6；恢复只读截止 UTC12:40。当前 PROPOSED_NOT_FROZEN_NOT_AUTHORIZED，schedule NOT_CREATED。选窗口不等于授权或调度。

交付：在已独立复评通过的分支冻结修复上，保留 v29 及全部历史失败、预算、pins、57 份备份与 17 份现存版本；使用独立新预算和 owner，为六份目标生成新的不可执行准备、原生非删除验收与完整只读核验，经过独立评审、最终 HEAD PR CI 和原生分支锁后才提供新精确授权文本。

唯一目标仍按 P2→T1→T2→H2A→M1A→M1B，P1 永久排除。11 survivor、其他环境和全部备份保留；cleanup=0、hold-release=0、bypass=0、每目标最多一次/全局最多六次、UNKNOWN 不重发。六目标未获本代删除授权，当前 source PUT/DELETE/业务 Commands 均禁止；准备业务重跑仅在独立 rehearsal PG，不冒充源结果。

权威：execution-freeze-boundary.md 负责分支更新/新授权绑定；现有 fixed six CLI、Core/Outbox→Asynq、native SDK ParseConfig/Equals、host topology 与持久预算负责既有语义。本代只接线新 execution/window/独立预算，并追加 v29 实际保留集；不引入第二套队列、策略引擎或 durable runtime。

源/IAM 只读核验必须包含真实 v29 六个已创建身份，全部 disabled/sole-policy/no-groups；v29 新 delete policy 是相同 exact-version 条件的 DeleteObject+DeleteObjectVersion 成对动作，不得用更早代 Version-only 策略错误比较或覆盖。原 v29 IAM UNCONFIRMED 保持历史事实，后来 fresh 读回须另列。

v29 保留核验只允许读原字节、Docker inspect/预算副本；不得调用其旧 Host 或重启旧执行。实际 v29 terminal、124 回执、原授权、sealed profiles、24 个历史预算 tar SHA/归属/完整挂载集合、820 个已知关闭 owner、57 备份须精确；新 generation 不得纳入旧集合而让旧事实漂移。不得为了缩短核验而放宽历史门禁。

新冻结顺序固定为：源码 review→新 source commit→停止态分配/独立预算初始化→source/restore/rehearsal 非删除验收→冻结制品→完整 17 版本只读→冻结包 review→全部静态文档/计划最终提交→精确 HEAD CI/评审→GitHub 原生锁→锁后 HEAD/完整保护/CI 核对→追加 execution-binding 收据→完整新确认。锁后的收据/人类授权/调度/终态不推进已锁 HEAD，不能产生 self-reference。

当前切片阻断限于：原始保留事实或权限扩大、source/core mutation 或 provider DELETE、原生核验失败、绑定不一致、必要路径/CI 失败和分支锁缺失。自动解锁、真实发送、调度、merge/Ready、pin release、预算退款/回收均不在本轮权限内。#332 保持 OPEN/Draft/unmerged，#307 保持 OPEN。
