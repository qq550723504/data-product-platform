# #307 D1 candidate15/v25 execution01

本代新的人类删除授权和单独固定窗口调度授权均已记录；execution `9f9c67d7-8a09-45a8-897f-9e3077b60980`。原准备快照、独立评审和冻结手册的 NOT_RECEIVED/FROZEN_NOT_AUTHORIZED 是当时事实，原字节保留。本文件只追加后来的授权和调度事实，不改写历史，不放宽执行门禁。

| 项目 | 当前证据与状态 |
| --- | --- |
| 精确删除授权 | [实际人类按引用授权](evidence/issue307-d1/delete-confirmed-v25-execution01/human-authorization.json)，原文含末尾 LF，共432 UTF8字节；user_retyped_template=false；56项批准引用实际字节匹配 |
| 独立调度授权 | [单独人类回答](evidence/issue307-d1/delete-confirmed-v25-execution01/schedule-human-authority.json)，只绑定同一 execution、日期和窗口 |
| 实际调度 | [创建与读回](evidence/issue307-d1/delete-confirmed-v25-execution01/scheduled-resume.json)，同一聊天 heartbeat `307-d1-candidate15-v25`，ACTIVE；SQLite实际 next_run_at=1792476000000，UTC2026-10-20 06:00 / 新加坡2026-10-20 14:00；仅一次，scheduler runs=0 |
| 窗口和预算 | UTC06:00≤now<06:40；六目标各最多一次、全局最多六次、UNKNOWN不重发；失败/过窗封存，不延长/平移/重开 |
| 固定候选 | candidate15/v25；运行 source f6acb072db9881e7b1cfd72003d4ef616955947b；原 candidate/template/bindings/source/binary/全部配置不变 |
| 最终 HEAD / PR CI | `a63fd5275f5fd26c832cb9664eb8b64b0f5e15bf`；PR CI39320908916，13/13 SUCCESS，event=pull_request；[授权后实时读回](evidence/issue307-d1/delete-confirmed-v25-execution01/github-checkpoint.json)；窗口前须再次实时复核，不能当 main CI |
| 窗口前关闭核验 | [固定Host.check](evidence/issue307-d1/delete-confirmed-v25-execution01/composition/authorized-prewindow-check.json) PASS_PREWINDOW_CLOSED；86本代owner停止、30 permission false、六case/六recovery及九新/九旧备份、三空预算正确；896次只读进程，无启动/exec/启用/签名/Source/provider/DELETE |
| 最终独立评审 | [独立07](evidence/issue307-d1/post-v22-policy-fix01/independent-frozen-confirmation-review07.json) PASS_BOUNDED_INDEPENDENT_FROZEN_CONFIRMATION_REVIEW，remaining_blockers=[]；不替代人类授权或窗口内 fresh 前置 |
| 新鲜完整入口 | NOT_RUN；开窗后固定 verify-six-native.py 新O_EXCL输出；检查17现存版本、完整IAM/保护/拓扑/预算/三源Core及备份，自动关闭后先 Host.check 再启动 |
| 历史 P1 | 原v22已记录一次DELETE/204/absence；永久排除本代六目标和预算，不重跑 |
| H0/H1 / H2 / M1 | 本代live结果 NOT_RUN；H0/H1零发送；H2真实204之后才晚hold/revoke/响应丢失；M1 checkpoint复制并核SHA后才B只读lookup |
| 只读恢复 | NOT_RUN；逐份send→禁用send→选定Outbox→既有Asynq→提交Core immutable absence→下一份；独立只读期限UTC08:40（新加坡16:40），不得重开身份或延长发送 |
| 本代实际DELETE | 0；六个slot全部NOT_RUN，不预先接受完整D1 |
| 保留边界 | 十一survivor、其他环境、全部新旧备份、旧pins、新旧预算、所有失败/UNKNOWN/attempt/成本保留；cleanup=0、hold-release=0、bypass=0 |
| PR / Issue | #332 OPEN/Draft，不Ready、不合并；#307 OPEN；#323已按独立历史授权合并，不回写 |

六个一次性slot的唯一顺序为 P2→T1→T2→H2A→M1A→M1B；当前均为NOT_RUN。exact环境、key/version、copy/DV、plan/hash和十一保留版本以[完整精确确认文本](issue307-d1-candidate15-confirmation.md)及[固定手册](issue307-d1-v25-window-runbook.md)为准。

调度工具只省略了提交prompt末尾一个LF。提交原字节与实际保存prompt分别留档并记录不同SHA；实际人类原文和批准机器模板未变。创建工具、TOML和SQLite三方读回一致，旧07/10/11/12调度均PAUSED，没有重复本execution调度。本地电脑、桌面app和Docker Linux引擎须可用；调度成功不保证未来新鲜前置通过。

任何非预期失败立即停止新增发送，使用固定window_six_support.py seal逐组关闭；一项失败继续其他关闭。无fresh IAM读回、enabler未闭合或归档不完整时保留UNCONFIRMED，不报告整轮PASS。完成、失败或过期后暂停同一heartbeat，已有terminal只读报告，不重复发起。
