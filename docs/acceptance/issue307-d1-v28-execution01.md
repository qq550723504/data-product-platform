# #307 D1 candidate18/v28 新授权与窗口前检查

本快照只追加真实新授权与关闭检查；原准备矩阵、冻结确认/手册中的NOT_RECEIVED/NOT_CREATED及全部旧失败保持原字节，不能用旧准备状态否认后来另存的新授权，也不能用本轮PASS覆盖旧失败。

execution `4fd5a7e7-779e-41d7-a35a-3bab7dd615b5`，授权窗口2026-10-21新加坡22:00–22:40，即UTC14:00≤now<14:40。实际人类授权消息`msg_01a15df3-b085-7a82-a891-abbb3b82c17c`，时间UTC10:13:33.957；原文含末尾LF共432个UTF-8字节，SHA256 `3cd9468fcb8cfec8b8777020bb71c02f8e506ffb2033ac35fc8ddcd966e33fb9`。这是按当前完整方案/冻结引用明确授权，user_retyped_template=false；没有伪称用户重新键入机器模板。

| 验收项 | 本轮状态 |
| --- | --- |
| 实际人类原文、会话event、完整展示及前一PASS核对文本 | 已分开按实际字节保存；机器批准模板与人类原文分开。 |
| 完整授权范围 | 仅本execution、原冻结六目标P2→T1→T2→H2A→M1A→M1B、单份max1/全局max6、UNKNOWN不重发；11survivor、所有其他环境、全部备份、新旧预算、旧pins和失败/attempt/成本事实保留。P1永久排除；cleanup=0/hold-release=0/bypass=0；失败或过窗立即封存，不延长/平移/重开。 |
| 全部引用 | 原1017项冻结manifest、manifest自身、已展示发布检查点与逐项核对证据共同绑定为1062项实际批准引用，逐项当前SHA/字节匹配。 |
| 新授权记录 | [human-authorization.json](evidence/issue307-d1/delete-confirmed-v28-execution01/human-authorization.json)，SHA256 `0c9abefaade12fd59095f0a269ef052c8474e4692ae76b7a4982630ed81c9f21`。私有envelope SHA256 `30461d1b6a31960845b61f672af8b8dc33cffeef59e60706b924b674d1e82d42`；不复用旧授权。 |
| 完整入口新增授权审计 | 使用冻结verify-six-native.py实际audit-only CLI，9个显式索引、4415项引用PASS；原7索引plan不覆盖，新增members-list-v1与files-map-v1明确区分，不猜格式或递归解释旧观察索引。新plan SHA256 `0941e25fdddeea85dcd2af8bcdd68f4da898fe25f8f1557e6f0c9372d4326f24`。 |
| 固定Host.check | 实际新授权闭合后调用冻结window_six_support.py check，PASS_PREWINDOW_CLOSED；30permission完整冻结字段false、6case/6recovery匹配、86本代owners关闭、3新预算为空，历史保留门禁及封存anchor通过。此check未启动Source/技术reader/enable/wire/sender，也未签发送proof。 |
| 实时GitHub | 实际HEAD/PR head=`a8930b31aa84fe0c003e2744253358e36c2f12f7`；run39577806574/event=pull_request/attempt1，13/13 SUCCESS，未解决线程0；#332 OPEN/Draft/unmerged，#307 OPEN。PR CI不是main CI，未来执行前仍须实时重查。 |
| 本轮执行与调度 | Source start=0、provider I/O=0、DELETE=0、发送权限仍关闭。本快照时schedule=NOT_CREATED；旧heartbeat保持PAUSED。本轮不创建调度。 |
| 窗口内新鲜验收 | NOT_RUN。须在固定窗口重新核对原完整入口的Core、17版本/inventory/bytes/保护状态、35 disabled/9 absent IAM、拓扑/预算/备份及所有冻结字段；本轮关闭PASS不替代它们。 |
| 完整D1验收 | NOT_ACCEPTED；真实H1可信NOT_SENT、H2 post-204 UNKNOWN、M1 mixed outcomes与六slot删除/恢复未运行，仍NOT_RUN。 |

未来实际发送仅按[冻结v28手册](issue307-d1-v28-window-runbook.md)与[完整确认](issue307-d1-candidate18-confirmation.md)。完整只读入口finally-close后，必须先通过真实授权Host.check，再在固定窗口内有限启用；权限仅变Enabled/真实ConfirmedAt，模板/binary/source/配置/窗口/max不变。逐slot关发送并经选定Outbox→Asynq只读恢复、提交实际Core immutable absence后才能下一slot；H0/H1零发送，H2/M1有限顺序保持原样。

只读恢复最晚UTC16:40（新加坡10月22日00:40），不能用于重启身份或延长发送。任何非预期失败或过窗停止新增发送并继续所有组固定seal，未确认关闭明确UNCONFIRMED，原失败/UNKNOWN/预算/pins/备份/成本保留。#332保持Draft、不合并，#307保持OPEN。

手册第8行的原文是“未来定时续跑仅按人类另行指令建立”；删除授权已独立收到，未来调度仍需单独人类指令。后来若出现调度授权或真实执行，另存追加回执，不覆盖本快照。
