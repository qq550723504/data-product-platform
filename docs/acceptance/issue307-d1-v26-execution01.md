# #307 D1 candidate16/v26 execution01 — authorization checkpoint

新execution `aa2605a5-1fcf-4e9d-b6f2-8f5b13321b8a`。本文件只追加后来真实授权和关闭核验，原candidate16/status、准备矩阵、独立报告及窗口手册的NOT_RECEIVED/NOT_CREATED为历史快照，保持原字节。

## 新的人类按引用授权

2026-10-20T06:02:11.933Z，人类在本聊天紧接唯一完整方案链接及execution/固定窗口说明回复“授权”加末尾LF。实际UTF-8共7 bytes，SHA256 `fce3771b1b29f5b8c466a7b4e76e01f1ab8436b481c706af4563f5819cc1e017`；原session消息ID `msg_01a157e7-325d-7f90-bf33-09006c69bc95`，app消息ID `01a157e7-325d-7f90-bf33-091cda20357e`，turn `01a157e7-3161-7201-bdd9-6e45b858289c`。两命名空间ID及原文已交叉核对，不把机器模板伪称人类重新键入，user_retyped_template=false。

[公开授权](evidence/issue307-d1/delete-confirmed-v26-execution01/human-authorization.json) SHA256 `b09d9a5024ccb46a1c8dd8fd19cefa96085aa4ea89f5a4666fe33f1afb933d14`，76项批准引用全部实际原字节匹配。机器108行模板、实际人类原文、完整方案、reference manifest、host bindings和envelope分别保存在私有authorized-execution01；envelope SHA256 `2dfa61a98fb9b703b6a17bb211256100ca5f6f55523730724fca19155f431335`，manifest `1888154332d5904b60982ac3f888e7f007c097d2bf6cd3e3a2e5c54de5f4c3b3`，host bindings `0d462605318c61b6171b64ad2507dd73c71618edf919f22745c9c9eb3483e6d6`。

范围仅P2→T1→T2→H2A→M1A→M1B六个exact版本，每份最多一次、全局最多六次，UNKNOWN不重发。11 survivor、其他环境、全部备份、新旧预算、旧pins及失败/UNKNOWN/attempt/成本事实保留；cleanup=0、hold-release=0、bypass=0。旧授权不复用。

固定窗口为2026-10-20新加坡18:00–18:40（UTC10:00≤now<10:40）；独立只读恢复截止UTC12:40（新加坡20:40），不能用于重新启用身份或延长发送。失败或过窗封存，不延长、平移或重开。

## 实际只读关闭核验与保留文件恢复

授权记录脚本第一次误用session日志channel字段，实际为phase=final_answer，写入前AssertionError停止；原脚本及诊断保留，修正只限日志schema读取，不修改运行源码或门禁。实际授权原文随后成功按新context留证。

首次[关闭检查失败](evidence/issue307-d1/delete-confirmed-v26-execution01/composition/check-authorization.stderr.txt)：旧candidate13的Windows file-only checker原路径缺失。失败保留，没有启动Source或开启权限。发现预先存在的source-rebuild02副本，SHA256与原allocation、checkpoint02冻结值均相同（`538a8ec129aa57482f75928cabd79dc350ddc06f83b96778fdc0ed7e8c60db4d`，9012736 bytes）。仅用O_EXCL恢复该原字节，原副本保留；没有重编译、修改候选/source/binary绑定或改门禁。缺失原因仍UNDETERMINED，过滤的Defender检测查询没有匹配记录。

[恢复回执](evidence/issue307-d1/delete-confirmed-v26-execution01/retained-artifact-recovery-result.json)及后续独立输出目录的[完整只读Host.check](evidence/issue307-d1/delete-confirmed-v26-execution01/composition02/authorized-prewindow-check.json)均保存。后者PASS_PREWINDOW_CLOSED，994条受限只读进程、无未批准进程；30 permission=false、6 case/6 recovery字段匹配、86本代owner关闭、359旧保留、9新+18旧backup及3新空预算通过。检查后恢复文件仍在且SHA相同。原失败不改写为PASS。

## 验收矩阵

| 项目 | 当前事实 |
| --- | --- |
| 本代精确删除授权 | RECEIVED_BY_REFERENCE，仅本execution/固定窗口及全部绑定 |
| 批准附件/原文/envelope | 76项及真实7字节原文PASS；user_retyped_template=false |
| 当前HEAD与PR CI | `74aa674db18dd68966d070e9f04173bb37ad1a7c`，39359998615，event=pull_request，13/13 SUCCESS；不是main CI |
| 准备阶段独立评审 | 原395f48e16acac92f9ce3b82dbaa6efc776211841107a989997e631ced90813c1 PASS、0有效阻断；新授权/恢复回执没有冒充已独立复评 |
| 授权后关闭核验 | 首次FAIL保留；原字节恢复后新目录PASS_PREWINDOW_CLOSED，994只读进程 |
| 新调度 | NOT_CREATED；冻结手册要求另获人类调度指令，当前待明确 |
| 窗口内fresh Core/native/IAM/17版本核验 | NOT_RUN；本次关闭检查不替代 |
| materialize批准文本/enable/case/wire/sender/proof/真实DELETE | 全部0/NOT_RUN |
| H1/H2/M1及六份只读恢复/完整D1 | NOT_RUN / NOT_ACCEPTED |
| #332 / #307 | Draft不合并 / OPEN |

冻结确认文本和手册、运行源码/source-freeze、所有候选/二进制/权限描述字节保持不变；HEAD不因这些未跟踪授权/核验回执移动。将来仍须严格窗口内新鲜核验，任一漂移停下并保留原失败。
