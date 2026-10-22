# #307 D1 candidate10/v20 固定窗口实跑结果

**2026-10-18 当前结果：FAILED_RENEW_PROFILE_SEALED_NO_DELETE。真实 DELETE=0，七份目标均未发送。**

本次 execution `67de3acd-3a9d-4ece-bebb-cd53d0ab3ac5` 在已授权的新加坡18:00–18:40（UTC10:00–10:40）内唤醒。完整原生新鲜核验通过后，首个固定 Renew owner 在10:05:18Z以 exit1拒绝执行；没有进入第一个 Renew command、真实业务或发送。10:05:25Z已记不可重入终态，随后暂停同一 heartbeat。本次不重试、不复用授权、不延长或重开窗口。#323保持Draft、不Ready、不合并；#307保持OPEN；完整D1仍 **NOT_ACCEPTED**。

[窗口结果](evidence/issue307-d1/delete-confirmed-v20-window01/window-result.json)、[实际封存步骤](evidence/issue307-d1/delete-confirmed-v20-window01/failure-seal.json)、[最终物理读回](evidence/issue307-d1/delete-confirmed-v20-window01/physical-final-readback.json)保存实际事实。此前[授权后准备矩阵](issue307-d1-v20-execution01.md)、冻结确认/手册及独立评审保留原时点和原SHA，不将准备PASS改写成运行成功。

## 本次实际验收矩阵

| 项目 | 实际结果 | 证据及边界 |
| --- | --- | --- |
| 新授权、窗口和代码身份 | PASS | 直接人类按引用授权，原文/模板分离，旧授权不复用；执行时HEAD `2ead583ee8d7f84e06272bb34f3b7865cfb0ddb1`，runtime source `71b1711927247d7aafc7d6a8bd707f013f2b310c`。窗口后文档提交的HEAD不冒充本次执行HEAD |
| 实时PR CI / Draft / OPEN | PASS | [start checkpoint](evidence/issue307-d1/delete-confirmed-v20-window01/start-checkpoint.json)：该精确HEAD的PR CI39104508835全部13项成功，review线程0；PR Draft/OPEN、Issue OPEN。不是main CI或业务验收 |
| 离线冻结及评审门禁 | PASS | 824源文件/1119原始证据、全部授权引用、885独立报告/11文件索引、607最终读回均匹配。该结果只证明冻结字节一致；漏掉了下述R1跨字段语义检查 |
| 完整窗口内原生核验 | PASS，18版本、76次GET/HEAD | [complete](evidence/issue307-d1/delete-confirmed-v20-window01/native-preflight/complete.json)：bytes/SHA/full inventory/hold/retention、完整disabled IAM/单policy/no groups/官方语义、3源Core只读指纹前后不变、9备份一致；每组技术helper及源在finally关闭 |
| closed-only检查、owner和预算 | PASS | [closed-after-fresh-native](evidence/issue307-d1/delete-confirmed-v20-window01/closed-after-fresh-native.json)：在重新启动源前运行review04 check；实际39owner配置/命令/mount/netns/隔离、7恢复profile/截止、3预算归属/挂载/空状态通过 |
| 受限业务读取身份 | 实际启用后已禁用 | 仅business-races固定enable-read启用reader/producer/inspector三身份，删除身份未启用；固定disable-all后四身份fresh禁用读回成功。root bootstrap与worker分离 |
| 六expected-head Renew | FAIL，第一条命令前拒绝 | [当前窗口日志](evidence/issue307-d1/delete-confirmed-v20-window01/renew-current-window-log.txt)与[根因](evidence/issue307-d1/delete-confirmed-v20-window01/renew-root-cause.json)。`candidate07-policy-started.json`及六Renew结果文件均不存在；不把owner启动计作Renew成功 |
| P1/P2/T1/T2实际业务与owner闭合 | NOT_RUN | Renew失败后未启动business owner；没有新Processing/DirectData结果，不能沿用旧case或mock结果 |
| H0/H1/H2与M1混合结果 | NOT_RUN | late-hold/mixed源和case均未启动；不记录NOT_SENT/absence或伪造UNKNOWN、witness、late hold、mixed checkpoint |
| 7目标发送、provider attempts及恢复 | NOT_RUN，真实DELETE=0 | 17个sender/wire/recovery owner在本窗口均未启动；3个持久预算仍为空；enable-send未运行；没有创建或重发UNKNOWN。原source versions、11survivor、其他环境和全部备份保留 |
| 实际技术调用与成本边界 | 76次GET/HEAD + 38次admin调用 | [root ledger](evidence/issue307-d1/delete-confirmed-v20-window01/root-admin-window-ledger.json)：admin为31 GET、7 PUT，PUT仅固定identity status。无S3 mutation/DELETE，不把技术调用说成零调用或零成本；price/amount未评估，不伪造Core业务provider attempt/cost |
| 失败封存 | 物理关闭确认；整轮fresh IAM不能标PASS | 110项固定步骤中108项完成；208绑定/技术容器stopped/PID0/noports/no restart、128permission=false、7case和7recovery profile已撤销、三gateway停止且sealed。business四身份fresh disabled确认；late-hold/mixed native已停止且从未启用，seal不重启源，因此这两组封存后的fresh IAM读回 **UNCONFIRMED**。它们此前窗口完整只读核验为disabled，但不冒充封存后的新读回 |
| 自动续跑与再次执行 | PAUSED，终态禁止重入 | 同一 `307-d1-candidate10-v20` 已暂停，原v17仍PAUSED；没有新调度、补发、清预算、release旧pin、hold-release、bypass或cleanup |

## 有效阻断 R1 与有限根因

冻结的case-renew profile SHA `cb3a9e11172c7135d50ce39fd9cd6be043829fa0d75da431154526b2afcafeee`中的 `CandidateUseUntil` 仍为 **2026-10-17T02:40:00Z**，而Base.StartedAt/ExpiresAt已改成v20的2026-10-18T10:00:00Z/10:40:00Z。实际程序在 `remaining_setup.go:170` 要求该字段等于编译期 `pilot.RemainingNotAfter`，因此先返回storage scope mismatch；缺少policy-start marker也与此一致。

生成器 `freeze-remaining-executable.py:131` 从旧source-owner profile复制case_base；132行仅刷新Batch/ExpectedPolicyHeads，133行仅刷新Base时间等字段，没有把CandidateUseUntil重新派生为新END。此前离线校验和独立评审核对了新SHA及Base窗口，却漏掉这一语义交叉检查。哈希一致不能证明跨代配置含义正确。这是当前必需运行路径阻断，**R1仍OPEN**；既有编码问题B1的修复及PASS不消除R1。

本轮没有修改冻结profile、source/binary/candidate/template或门禁，也没有在剩余窗口内修补后重跑。原失败、撤销前profile原字节/SHA、root O_EXCL归档/journal、全部预算和物理资源保留。后续有限修复应在生成新profile时从统一END派生CandidateUseUntil，并在完整生成/离线入口加入“旧值拒绝、新值一致”回归，交叉检查所有case与恢复期限；修复后另准备新候选、新execution、新窗口和精确确认，不复用本授权。
