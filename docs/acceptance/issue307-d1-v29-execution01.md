# #307 D1 candidate19/v29 授权后窗口前矩阵

记录时间：2026-10-22T03:01:54.669871+00:00。execution a5d1e569-f80e-4b38-8f32-d8139a30a077。本次新的人类按引用授权已收到，原文432 UTF-8 bytes（含末尾LF），SHA256 b0c94da7bcacb7c9c0a3863fadfd5faba0418322cb603ed6e26e3ffb0c8993b3。批准模板另存，user_retyped_template=false；不复用旧授权。

唯一发送窗口：2026-10-22 新加坡14:00≤now<14:40（UTC06:00–06:40）；只读恢复最晚16:40（08:40Z），不授予重新启用或延长发送。顺序P2→T1→T2→H2A→M1A→M1B，每份max1、全局max6，UNKNOWN不重发。6目标/11保留版本及全部绑定以冻结模板为准，P1永久排除。

| 项目 | 本次事实 |
| --- | --- |
| 直接人类授权 | RECEIVED_EXPLICIT_USER_AUTHORIZATION_BY_REFERENCE；实际人类原文与机器模板分开留证 |
| 924项批准引用及私有envelope/host bindings | 原字节核对通过 |
| 固定完整入口离线审计 | PASS；9 indexes / 5470 references；files映射与members列表显式区分 |
| 实际冻结Host.check | PASS_PREWINDOW_CLOSED；30权限false、86本代owners关闭、3新预算为空、6case/6recovery精确 |
| 保留备份 | 本次固定门禁9新+48旧SHA匹配；旧预算与pins未修改，旧失败保持历史事实 |
| 当前HEAD与PR CI | caab46da8761e71e1d80d0c0931707a410c13a4d；run39710200629 / pull_request / 13成功；不是main CI |
| PR/Issue/评审线程 | #332 OPEN/Draft/unmerged；#307 OPEN；未解决线程0 |
| 窗口内fresh Core/native/IAM/17版本全量核验 | NOT_RUN；必须在固定窗口内通过，不能用本次关闭检查替代 |
| 启用、签发送proof、source/provider I/O、DELETE | 本次均0 |
| 独立准备评审 | 原冻结报告PASS；不声称本次新增授权/调度另经独立复评 |
| 新定时续跑 | NOT_CREATED_REQUIRES_SEPARATE_HUMAN_REQUEST；删除授权无需重复 |
| 完整D1 | NOT_ACCEPTED；六目标本次尚未执行 |

不修改candidate/source/binary/config，不重跑prepare/freeze、重建资源或预算；保留新旧预算、旧pins、57备份、其他环境及全部失败/UNKNOWN/attempt/成本事实。cleanup=0、hold-release=0、bypass=0。失败或过窗立即固定seal，不延长、平移、重开或补发。原准备NOT_RECEIVED/NOT_CREATED、旧失败与旧授权不覆盖。

后续如另获调度指令，只建立本execution固定日期的一次续跑；开窗先实时核UTC、终态、当前HEAD/CI/Draft/Open/线程，运行冻结完整CLI的新phase审计计划与fresh Core/native/IAM/17版本/拓扑/预算检查，再按冻结有限顺序执行。已有terminal不再执行。任何非预期失败先封存，实际未跑NOT_RUN，完成/失败/过期停同一调度。

[授权原始回执](evidence/issue307-d1/delete-confirmed-v29-execution01/human-authorization.json) · [实际窗口前核验](evidence/issue307-d1/delete-confirmed-v29-execution01/authorized-prewindow-check01.json) · [窗口手册](issue307-d1-v29-window-runbook.md)
