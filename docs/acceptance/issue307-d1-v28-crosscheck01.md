# #307 D1 candidate18/v28 精确确认逐项核对

2026-10-21 UTC10:03–10:09（新加坡18:03–18:09）完成本轮核对。结论：**PASS_BOUNDED_C18_V28_EXACT_CONFIRMATION_CROSSCHECK**，候选范围内剩余阻断为0。此文件是新增核对矩阵，不覆盖原准备快照、原失败或冻结方案。

execution：`4fd5a7e7-779e-41d7-a35a-3bab7dd615b5`。唯一发送窗口是2026-10-21新加坡22:00–22:40，即UTC14:00≤now<14:40；只读恢复最晚UTC16:40（新加坡10月22日00:40），不能据此延长发送或重新启用身份。

| 项目 | 本轮结果与证据边界 |
| --- | --- |
| 实际聊天完整文本 | 从本聊天实际assistant消息提取；121行机器代码块30466字节与冻结模板逐字节相同，SHA256 `5cd148132b380b4a3361ae7210c8cc02fe6932fee3824ae77b4fec0cc2523a90`。12项原构建成员、35项宿主表及13项补充文件SHA均一致。该assistant文本不是人类授权。 |
| 冻结绑定 | candidate、bindings、source-freeze、profile-windows、cross-field窗口、Core/IAM/gateway/预算/CID/manifest与确认全文一致。审批manifest1017项及manifest自身、1018个发布HEAD Git blob均核对；源码raw/LF和ZIP身份按原冻结契约分别核对，未修改候选或门禁。 |
| 完整入口静态审计 | 使用冻结verify-six-native.py实际CLI的audit-only模式；7个显式索引、3348项引用全部PASS。此模式不做原生预检、不启动Source或Docker owner。 |
| 六份目标 | P2→T1→T2→H2A→M1A→M1B的全部project/provider/incarnation、namespace/key/version、copy/DV、SHA/bytes、plan/hash逐项匹配。六目标均为native hold=false、retention=NONE、retain-until=None；每份最多1次、全局最多6次，UNKNOWN不重发。 |
| 十一份保留版本 | business4、late5、mixed2，全部完整tuple及保护状态与实际原生核验回执一致。三份control-hold仍为true；business control-retention和mixed M1C的GOVERNANCE期限分别为2026-10-23T06:25:44Z、06:33:33Z。其余保留版本即使没有native hold也不成为目标。P1另作已有真实NoSuchVersion404的历史事实，永久排除。 |
| 实际窗口字段 | 6份case.Batch等于本execution，StartedAt=14:00Z，ExpiresAt=CandidateUseUntil=14:40Z；6份recovery.ValidUntil=16:40Z。通过冻结窗口校验，未延长、平移或重开。 |
| 新鲜关闭核验 | 直接调用冻结完整入口sealed_gate，未调用需要授权的Host.check；86本代owner的实际拓扑/完整mount/角色安全约束、30份false权限、6case/6recovery、3新预算归属/挂载集/空预算及旧代保留门禁全部PASS。再实际inspect712个已知CID，全部停止/PID0/noports/no-restart。 |
| 保留事实 | 18份旧预算、旧pins/失败/UNKNOWN/attempt/成本事实沿固定历史门禁核对；48份实际备份逐路径、字节数、SHA重查通过，其中9新、36旧完整checkpoint、3旧失败partial。完整入口旧字段old_backups_retained=27仍是原子集统计，不覆盖原回执，也不当作备份总数。 |
| 评审与发布 | 两份原独立报告状态分别为PASS_BOUNDED_C18_SOURCE_PREPARATION_REVIEW、PASS_BOUNDED_C18_FROZEN_NONDELETE_PREPARATION_REVIEW，remaining_blockers均为空。实时GitHub读回HEAD=`a8930b31aa84fe0c003e2744253358e36c2f12f7`，PR CI run39577806574/event=pull_request/attempt1，13/13成功、未解决线程0；#332 OPEN/Draft/unmerged，#307 OPEN。未将PR CI称为main CI。 |
| 当前授权/调度 | NOT_AUTHORIZED / NOT_SCHEDULED。本代authorized-execution01不存在、heartbeat18不存在；七份旧heartbeat保持PAUSED。没有复用旧授权，也没有创建调度。 |

本轮只比较已保存原生证据并做宿主关闭检查，Source start=0、provider I/O=0、DELETE=0。原完整原生核验的三组实际完成时间是UTC09:19:31、09:19:39、09:19:46；它们保留真实请求/成本事实，不能冒充未来窗口内的新鲜核验。

执行前仍必须完成：

1. 本代新的直接人类精确授权，并分别留存实际原文、机器模板、全部引用、manifest和envelope；user_retyped_template=false。定时续跑另需人类指令。当前核对请求不授予任何发送。
2. 固定窗口内实时重查精确HEAD、必要PR CI、线程、Draft/Open、批准字节，以及固定完整入口的新鲜三源Core/17版本库存与bytes/保护状态/35个disabled IAM及9个absent用户/备份/真实拓扑/预算。任一漂移停止，不能改候选规避。
3. 完整入口finally-close后，先通过实际授权Host.check，再有限启用。禁止Renew、新Rights、重跑Processing/DirectData或P1。按冻结手册逐slot send→关send→选定Outbox→Asynq只读恢复→Core immutable absence→下一slot。
4. H0/H1零发送；H1可信终止与fresh disabled/sealed证明、H2真实204后的late-hold/UNKNOWN、M1 mixed checkpoint复制后才B lookup等有限顺序保持原样。每个sender/wire/recovery需各自≤1分钟fresh真实拓扑；wire context≤3分钟；预算先于Core claim，SDK无重试。
5. 失败或到14:40Z停止新发送，固定seal继续所有组关闭；关闭未确认须UNCONFIRMED。保留旧/新预算、pins、备份、原失败、UNKNOWN、真实attempt及成本，cleanup=0、hold-release=0、bypass=0。完整D1仍NOT_ACCEPTED，未运行业务项仍NOT_RUN。

辅助观察脚本两次失败已追加保留：第一次误把2129个后缀选定源码成员等同于整个ZIP，第二次误把ZIP原始CRLF字节等同于LF。按原冻结生成器和原独立评审的既有契约纠正观察口径后通过：完整归档2184文件/230目录，选定源码2129，实际ZIP SHA/3584518字节均不变。两个原观察脚本、错误及纠正结果均保留；没有修改运行源码、冻结文件或门禁，也不因此再造候选。

详细逐tuple、逐文件和实际GitHub/宿主回执见[本轮核对目录](evidence/issue307-d1/candidate18-crosscheck01/)。原[完整确认](issue307-d1-candidate18-confirmation.md)、[窗口手册](issue307-d1-v28-window-runbook.md)与[准备矩阵](issue307-d1-v28-preparation.md)均保持原字节。
