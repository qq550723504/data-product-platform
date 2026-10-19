# #307 D1 candidate10/v20 execution01 授权后验收矩阵

本页记录新授权后的当前执行准备。冻结的 [v20 精确方案](issue307-d1-candidate10-confirmation.md)、[窗口手册](issue307-d1-v20-window-runbook.md)、candidate/status 及原始失败证据保持原字节；其中 NOT_RECEIVED / NOT_CREATED 是授权到达前的历史快照。本页及独立评审回执追加后来事实，不覆盖该快照。

执行状态：**AUTHORIZED_WAITING_FIXED_WINDOW**；完整 D1：**NOT_ACCEPTED**。execution 为 `67de3acd-3a9d-4ece-bebb-cd53d0ab3ac5`，profile 为 `candidate10/v20`。本轮没有启动源环境、业务 case、Renew、enable、发送/恢复 worker 或签署 topology proof；provider I/O / DELETE / cleanup / hold-release / bypass 均为 0。#323 保持 Draft、不合并，#307 保持 OPEN。

唯一发送窗口是 **2026-10-18 新加坡 18:00≤时间<18:40（UTC 10:00≤时间<10:40）**。窗口内全部新鲜前置通过后，才可对冻结方案列出的 7 份 exact 目标各发最多一次、全局最多 7 次；UNKNOWN 不重发。11 份保留版本、其他环境、全部备份、预算及失败事实保留。失败或过窗立即停止新增发送并封存，不平移、延长、重开或补发。独立只读恢复最晚 UTC 12:40 / 新加坡 20:40，不能成为重新 enable 身份或继续发送的依据。

## 当前证据

| 验收项 | 当前结论 | 权威证据及边界 |
| --- | --- | --- |
| 新人类授权 | PASS，按引用批准 | [原文及绑定](evidence/issue307-d1/delete-confirmed-v20-execution01/human-authorization.json)，SHA256 `261378648576c27a106893a2b06af425e490966495e0dfc8fccda83d64bbaab8`；消息 ID `01a14de4-c6bb-7aa2-8e13-20d27a533710`，实际原文含末尾 LF；`user_retyped_template=false`、`old_authorization_reused=false` |
| 完整冻结范围 | PASS，7 目标 / 11 保留，14 份额外引用文件一致 | [授权记录](evidence/issue307-d1/delete-confirmed-v20-execution01/human-authorization.json) 同时绑定机器模板、candidate/bindings、source/binary、Core/IAM/gateway/owner/预算、完整入口、review04 工具及原最终评审。私有原文、批准模板、附加宿主绑定、manifest、envelope 分开保存；worker 未 materialize 人类批准文本 |
| 原始模板与候选 | PASS，字节不变 | candidate `c6ba9f9d7cfad7909be20c9df2858e30dea613181ff3c659f0e4c4bb444940ab`；bindings `f074576acafe56eef67385c36f8057b052c6e4fb456279e0e45b5c61add4f23b`；template `f1345f4909da80b6c25b2658f3d083dfdd2e737dd49dc50e644dd3aec9dc400c` |
| 运行源码及离线校验 | PASS，824 源文件 / 1119 原始证据 | [本次 checker 原输出](evidence/issue307-d1/delete-confirmed-v20-execution01/offline-check.json)；source `71b1711927247d7aafc7d6a8bd707f013f2b310c`。原输出中的 NOT_RECEIVED 指冻结准备快照，新授权用独立记录证明，不改 checker 或旧状态 |
| 当前 HEAD / PR CI / 评审线程 | PASS，精确 HEAD 的 PR CI 13/13；未解决线程 0 | [本轮 GitHub checkpoint](evidence/issue307-d1/delete-confirmed-v20-review01/github-checkpoint.json)：HEAD `2ead583ee8d7f84e06272bb34f3b7865cfb0ddb1`、run `39104508835`；独立 reviewer 另核对实际 run 的 head_sha/event。#323 Draft/OPEN、#307 OPEN。执行前仍须实时重查，不能冒充 main CI |
| 授权后的本代封闭检查 | PASS，58 容器 stopped/PID0，32 permission=false | [原检查](evidence/issue307-d1/delete-confirmed-v20-execution01/prewindow-check.json) 与 [本轮新鲜只读检查](evidence/issue307-d1/delete-confirmed-v20-review01/parent-readonly-prewindow-check.json)。仅运行 review04 `check`；没有运行可变维护或 provider 调用 |
| 全代物理状态 | PASS，205 stopped/PID0/no ports/no restart；128 permission=false | [本轮宿主读回](evidence/issue307-d1/delete-confirmed-v20-review01/parent-readonly-prewindow-check.json)。原生源停止，因此本项不声称提供新的 IAM 服务端读回 |
| 恢复配置、预算及备份 | PASS，7 实际恢复 profile、3 新预算为空；9 备份 SHA 一致 | [授权后宿主核验](evidence/issue307-d1/delete-confirmed-v20-execution01/prewindow-host-readback.json)；恢复截止 UTC12:40，配置无发送 secret/wire key；预算归属及挂载精确。窗口内仍须重核；不清空或重建预算 |
| 18:00 定时续跑 | ACTIVE，固定日期、同聊天、一次唤醒 | [原调度快照](evidence/issue307-d1/delete-confirmed-v20-execution01/scheduled-resume.json) 与 [本轮评审门禁后的最终读回](evidence/issue307-d1/delete-confirmed-v20-review01/review-gated-schedule02.json)；ID `307-d1-candidate10-v20`，Singapore Standard Time，UTC10:00。原 prompt SHA256 `ea553adace261d0fe0cef395fe3aa17145b84f861d6454b7d2ff7fd991bf5ebd` 原字节保留；仅追加本轮报告/独立索引的收窄门禁及后续读回路径，新 prompt SHA 见最终读回。早醒不启用，晚醒不执行；完成/失败/过期暂停同一任务；旧 v17 调度保持 PAUSED |
| 本轮授权/调度/封存独立评审 | PASS，remaining_blockers=[] | [独立报告](evidence/issue307-d1/delete-confirmed-v20-review01/independent-authorization-window-review.json)，SHA256 `88589338c265179182b561f108b4e7e7859bd8a9fecaa13db1da0afd719e3c6c`；核对实际原文/模板/host/envelope、14项冻结引用、7项原准备索引、实际调度及失败路径，无有效阻断；本轮不改变已授权冻结绑定 |
| 失败封存及授权非删除回归 | 独立 support normal/-O 各14 PASS；新增授权4方法、15拒绝子案例+1正例 PASS | [封存 normal](evidence/issue307-d1/delete-confirmed-v20-review01/support-normal.txt)、[封存 -O](evidence/issue307-d1/delete-confirmed-v20-review01/support-optimized.txt)、[授权反例](evidence/issue307-d1/delete-confirmed-v20-review01/authorization-counterexamples.txt)。覆盖外来execution/source/binary/digest、改窗、预算扩张、UNKNOWN重发、旧授权/伪称重键、原文LF/envelope/模板篡改和提前启用。只用临时 mock；不冒充真实 IAM 关闭或窗口内业务结果，真实封存尚未执行 |
| 本轮有效阻断 B1：调度文本传输编码 | 已修复；独立编码4方法 PASS | [失败及根因修复回执](evidence/issue307-d1/delete-confirmed-v20-review01/schedule-encoding-repair.json) 与 [独立最终读回](evidence/issue307-d1/delete-confirmed-v20-review01/independent-final-readback.json)。一次非ASCII Python console JSON传输使待更新前缀/name乱码，最终字节门禁在启动/启用/发送前拒绝。实际坏配置、失败input/verifier及原prompt保留；改为ASCII JSON escapes读取、从原UTF-8文件取前缀并经结构化tool传Unicode，实际中文name和前缀逐字节恢复。原01读回/checkpoint保留，02仅追加最终路径及结果，不覆写历史 |
| 窗口内完整新鲜核验 | NOT_RUN，发送前必需 | 同一完整 native CLI 必须重做18版本 bytes/SHA/inventory/hold/retention、完整 disabled IAM/policy 语义、3源 Core 只读指纹前后不变、9备份、真实 owner/netns/mounts 及预算；CLI 关闭资源后先执行 closed-only support check，再启动授权业务 |
| P/H/M 业务与7目标执行 | NOT_RUN | P1/P2/T1/T2 业务 owner/handles/pin/ref 闭合；H0/H1 零发送及单次可信闭锁证明；H2 只 A、上游204后 late hold/revoke/UNKNOWN；M1 A absence → B UNKNOWN+C-only mixed checkpoint → 精确复制 checkpoint → B lookup，C 永不删除。只沿冻结 sender/wire、每 target budget 先于 Core claim、proof≤1分钟推进 |
| 完成/失败后的真实封存与对账 | NOT_RUN | 使用冻结 review04 `seal`，不启动 source/enable/sender/wire；关闭一项失败仍继续其他组。缺 fresh identity 读回或 enabler 未闭合标 unconfirmed，保留 UNKNOWN/attempts/cost/pins/预算。实际结果另追加，不提前写 NOT_SENT 或 absence |

## 本轮评审收敛范围

只将授权范围/时间/冻结字节绕过、历史或失败证据改写、当前必需执行路径不可实现、次数/UNKNOWN/身份隔离/封存缺陷认定为本轮有效阻断。没有以 CI、离线测试或宿主 stopped 事实替代窗口内 native/Core/IAM 核验，也没有新增通用 IAM、workflow 或第二套策略基础设施。

本轮独立评审剩余阻断为 **0**。已保存原授权、原调度及准备索引；收窄门禁仅要求本轮独立报告 PASS、remaining_blockers=[]、报告与独立索引 SHA/索引成员字节一致，不替代原有任何前置。索引不包含后来 prompt/本页/最终 checkpoint，避免哈希自引用。最终读回及本页 SHA 见 [最终 checkpoint02](evidence/issue307-d1/delete-confirmed-v20-review01/final-checkpoint02.json)，另由 [独立最终读回](evidence/issue307-d1/delete-confirmed-v20-review01/independent-final-readback.json) 检查新增门禁、实际调度配置、B1闭合及矩阵字节。原01 checkpoint仅代表此前步骤，不冒充后续矩阵字节。

运行源码、binary、candidate、template、原确认文档和原窗口手册均不变；HEAD 保持原已核对的 `2ead583ee8d7f84e06272bb34f3b7865cfb0ddb1`，本轮新增的是评审/执行准备回执及本矩阵，尚未为这些新回执创建新提交。若将来需要改变任一已授权精确冻结绑定，不能静默复用本授权执行。
