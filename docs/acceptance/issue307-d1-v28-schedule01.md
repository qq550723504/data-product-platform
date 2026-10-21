# #307 D1 candidate18/v28 一次固定窗口续跑

本快照追加单独人类调度授权及实际调度读回，不覆盖原准备/删除授权/窗口前检查矩阵。execution仍为`4fd5a7e7-779e-41d7-a35a-3bab7dd615b5`，删除授权、六目标、十一保留版本、全部冻结字节和次数限制均不变。

| 项目 | 实际结果 |
| --- | --- |
| 单独调度授权 | 人类消息`msg_01a15e0b-4f97-7ca2-b69e-c95b2af21a0b`，2026-10-21T10:39:22.008Z，回答“安排固定窗口续跑（推荐）”；问题明确同一execution及今天22:00的一次续跑。原wrapper634 UTF-8字节，SHA256 `82224029f93f050a9a281e2bad7bace46452416d37407879c11c7c1e751c2274`，原session event和实际文本分别留证。 |
| 删除授权独立保留 | human-authorization.json SHA256 `0c9abefaade12fd59095f0a269ef052c8474e4692ae76b7a4982630ed81c9f21`；432字节实际删除授权原文、机器模板、envelope及1062项批准引用保持原字节。调度不扩大删除范围。 |
| 调度 | `307-d1-candidate18-v28`，kind=heartbeat，ACTIVE，回到本聊天`01a13579-573d-7a32-b1e1-3297fe78380a`；通过automation create创建，再用view及本地TOML/只读SQLite交叉核对。 |
| 实际下一次触发 | next_run_at与next_run_nominal_at均为`1792591200000`，即2026-10-21T14:00:00Z／新加坡2026-10-21T22:00:00+08:00。仅一次；尚无运行记录。 |
| 唯一性 | 此execution的实际automation数量=1；七个旧heartbeat均PAUSED、next_run_at/nominal为空；没有重复任务。 |
| 固定执行窗口 | 2026-10-21新加坡22:00–22:40，UTC14:00≤now<14:40；不得延长、平移、重开或补发。完成、失败、过期或已有terminal时暂停本同一heartbeat。 |
| 提示词实际字节 | 16282 UTF-8 bytes，SHA256 `4aff23065dece624a93803763e906c736b54533ac646e21a78a0a43e46209d38`；工具参数、实际TOML与SQLite prompt逐字节一致。绑定唯一d1-c18工作目录、实际新授权/调度、冻结证据、精确HEAD CI、新鲜前置、六slot有限顺序与失败逐组seal。 |
| 完整入口静态审计 | 原9索引4415引用追加真实调度3成员，实际冻结verify-six-native.py audit-only通过10个显式索引/4418项引用。scheduled-evidence-audit-plan01.json SHA256 `d371bc30d6d2c35b83b1802e314ef6c3953c22a96b985fe78f3ab8a3eb8ad0de`。files-map-v1与members-list-v1明确区分；后来的prompt、TOML及调度回执另存，避免自引用。 |
| 窗口前实际关闭读回 | 30份operator/root permission全部冻结字段false、ConfirmedAt零；712个已知CID停止/PID0/noports/no-restart；3新预算实际tar读回为空且UID/GID10001，旧预算/pins/失败保持。没有启动Source/case/enable/wire/sender或签发送proof。 |
| 当前发布状态 | HEAD仍`a8930b31aa84fe0c003e2744253358e36c2f12f7`，tracked修改0；精确HEAD PR CI13/13成功的原实时回执保留，执行前仍须再次实时核。#332保持OPEN/Draft/unmerged，#307保持OPEN。 |
| 当前业务验收边界 | 本轮Source start/provider I/O/DELETE均0；窗口内完整Core/native/IAM/17版本/保护状态/拓扑/预算/备份预检NOT_RUN，完整D1 NOT_ACCEPTED。调度创建及准备PASS都不能替代它们。 |

实际执行仍遵守[完整冻结确认](issue307-d1-candidate18-confirmation.md)和[v28手册](issue307-d1-v28-window-runbook.md)。窗口前保持发送关闭，早醒只读等待；过窗不启用。只有固定窗口内全部新鲜前置通过后才按P2→T1→T2→H2A→M1A→M1B、每份max1/全局max6执行，UNKNOWN不重发，P1永久排除。十一份保留版本、其他环境、全部备份、旧pins、新旧预算、原失败/UNKNOWN/真实attempt/成本事实保留；cleanup=0、hold-release=0、bypass=0。

完整入口finally-close后、重启Source前先通过实际授权Host.check；逐slot关send并经选定Outbox→既有Asynq只读恢复及实际Core immutable absence后才能下一slot。H1可信终止/禁用/封网证明、H2 post-204 late hold/UNKNOWN和M1 mixed checkpoint原字节复制后B lookup等有限顺序保持冻结约束。任何非预期失败或过窗立即停止新增发送并继续所有组固定seal；关闭未确认明确UNCONFIRMED，未跑NOT_RUN。只读恢复最晚16:40Z（新加坡10月22日00:40），不能用于重启身份或延长发送。

本轮调度授权、工具create/view、实际SQLite next_run与唯一性、TOML prompt及窗口前关闭读回见[evidence目录](evidence/issue307-d1/delete-confirmed-v28-execution01/)。未来执行/暂停另存追加证据；不能要求未来已暂停的TOML仍等于本轮创建时SHA，也不能覆盖本快照伪造一致。

本地续跑依赖电脑开机与桌面应用运行，符合[OpenAI官方调度说明](https://learn.chatgpt.com/docs/automations)。
