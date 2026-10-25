# #307 D1 candidate16/v26 — fixed-window schedule checkpoint

本记录追加后来的人类调度授权；原冻结准备文档及authorization-evidence-index01的19项成员保持原字节，早期PENDING/NOT_CREATED是历史快照。本代execution为 aa2605a5-1fcf-4e9d-b6f2-8f5b13321b8a。

| 项目 | 实际事实 |
| --- | --- |
| 删除授权 | 2026-10-20T06:02:11.933Z按引用授权，76项附件及实际原文独立保留；user_retyped_template=false |
| 单独调度授权 | 人类2026-10-20T06:29:37.450Z回复“安排固定窗口续跑（推荐）”，问题明确同一execution及18:00–18:40 |
| 实际调度 | 同一聊天heartbeat 307-d1-candidate16-v26，ACTIVE，实际next_run_at=1792490400000，即2026-10-20新加坡18:00；COUNT=1，0次运行，无重复 |
| 唯一执行窗口 | UTC10:00≤now<10:40，新加坡18:00–18:40；失败或过窗封存，不延长/平移/重开，随后暂停同一任务 |
| 范围 | P2→T1→T2→H2A→M1A→M1B六个exact版本，每份最多一次、全局最多六次，UNKNOWN不重发；P1永久排除 |
| 保留 | 11survivor、其他环境、全部备份、旧pins、新旧预算、失败/UNKNOWN/physical attempts/成本事实保留；cleanup/hold-release/bypass均0 |
| 关闭核验 | 授权后只读Host.check恢复原字节后PASS，994受限只读进程、30false、3新空预算；首次c13缺文件失败及恢复证据保留，缺失原因UNDETERMINED |
| 冻结及HEAD | 76批准引用及前19项证据均实际SHA相等，tracked无变化，HEAD仍74aa674db18dd68966d070e9f04173bb37ad1a7c |
| 旧调度 | v17/v20/v21/v22/v25均PAUSED |
| 窗口内fresh Core/native/IAM/17版本 | NOT_RUN；既有准备、关闭检查和PR CI均不能代替 |
| 本轮Source/enable/case/wire/sender/proof/provider/DELETE | 全部0；批准机器文本尚未给worker |
| 六份发送/恢复及完整D1 | NOT_RUN / NOT_ACCEPTED |
| #332 / #307 | Draft、不合并 / OPEN |

[实际调度读回](evidence/issue307-d1/delete-confirmed-v26-execution01/scheduled-resume.json)与[单独人类调度授权](evidence/issue307-d1/delete-confirmed-v26-execution01/schedule-human-authority.json)分别保留。固定完整入口、76项冻结引用、精确HEAD CI、最新人类指令及所有窗口内新鲜前置仍须逐项通过；任一漂移停止，不能修改冻结字节继续。只读恢复最晚新加坡20:40，不能重新启用身份或延长发送窗口。

这是本机续跑，届时电脑、Codex桌面应用、Docker Linux引擎和原保留资源须可用。[官方调度说明](https://developers.openai.com/training/walkthroughs/scheduled-tasks)要求本地文件任务保持电脑及应用运行。实际future执行尚未发生。
