# #307 D1 candidate19/v29 固定窗口续跑回执

记录：2026-10-22T03:09:26.296455+00:00。execution a5d1e569-f80e-4b38-8f32-d8139a30a077。此为授权后新增调度事实；原准备/授权矩阵的NOT_CREATED保留当时事实，不覆盖。

| 项目 | 当前核对结果 |
| --- | --- |
| 删除授权 | 人类直接按引用授权已记录；924项批准引用保持原字节；不复用旧授权 |
| 单独调度授权 | 人类回答“安排固定窗口续跑（推荐）”；真实wrapper751 UTF-8 bytes另存 |
| 实际automation | 307-d1-candidate19-v29；heartbeat；ACTIVE；目标为本聊天 |
| 实际下次运行 | 2026-10-22T06:00:00Z = 新加坡2026-10-22 14:00；next_run与nominal一致 |
| 数量及历史 | 本execution仅1调度；已运行0次；旧07/10/11/12/15/16/17保持PAUSED，旧18不恢复 |
| 固定窗口 | 仅2026-10-22新加坡14:00≤now<14:40，过窗不延长、平移、重开或补发 |
| 追加phase索引 | 固定完整入口audit-only通过；10显式indexes/5473references |
| actual prompt | 与批准创建参数、TOML及实际SQLite prompt逐字一致 |
| 当前关闭状态 | 820个固定已知CID stopped/PID0/noports/norestart，30permission=false，worker无提前物化批准文本 |
| 新预算与保留证据 | 03:01Z固定Host.check证明3新预算为空、9新+48旧backupSHA匹配；本轮调度不消费预算或修改pins |
| HEAD/PR CI | caab46da8761e71e1d80d0c0931707a410c13a4d；已核13/13 pull_request成功，不是main CI；窗口内必须再核实时精确HEAD/线程/CI |
| 状态约束 | #332 OPEN/Draft/unmerged；#307 OPEN |
| 尚待执行 | 窗口内fresh Core/native/IAM/17版本、拓扑及预算完整核验NOT_RUN；本轮source_start/provider/DELETE均0 |
| 停止规则 | 完成、失败或过期暂停本同一heartbeat；已有terminal只读报告，不重复执行 |
| 完整D1 | NOT_ACCEPTED |

六目标仅P2→T1→T2→H2A→M1A→M1B，max1each/max6total，UNKNOWN不重发；P1永久排除。保留11survivor、其他环境、全部备份、新旧预算、旧pins及失败/UNKNOWN/attempt/成本事实。cleanup=0、hold-release=0、bypass=0。每slot发送关闭、选定Outbox→Asynq只读恢复并提交实际Core absence后才能下一slot；H0/H1零send，H2/M1严格冻结顺序。

原授权索引7成员、窗口前索引34成员、冻结候选/source/binary/config不变。调度prompt和实际TOML快照后存，不加入此前授权审计plan构造自引用。TOML快照是本次创建事实，未来按指令暂停后不再要求实际TOML等于创建时SHA。准备独立评审PASS不冒充新增授权/调度已独立复评。

[实际调度读回](evidence/issue307-d1/delete-confirmed-v29-execution01/scheduled-local-readback01.json) · [调度授权原文与绑定](evidence/issue307-d1/delete-confirmed-v29-execution01/schedule-human-authority.json) · [固定窗口手册](issue307-d1-v29-window-runbook.md)
