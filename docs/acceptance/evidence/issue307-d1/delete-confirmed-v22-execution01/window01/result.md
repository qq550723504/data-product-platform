# candidate12/v22 固定窗口执行结果

execution：`8e41cfba-a41e-44dd-9ce8-d6365b3bb90e`。状态：**FAILED / SEALED；D1 NOT_ACCEPTED**。

- 窗口内完整新鲜核验、六项Renew及真实Processing/DirectData业务通过。
- 实际DELETE共1次：P1，上游204、CONFIRMED/ACCEPTED；尚未完成exact absence对账。
- P2预发送失败，预算槽已消耗、没有wire转发；其余目标未发送，UNKNOWN不重发。
- 固定seal的110项步骤全部通过；61个相关CID停止，32权限关闭，9备份SHA匹配。
- 同一heartbeat已暂停；本execution及授权终止，不继续剩余窗口。

有效阻断为“四次发送后再恢复”的顺序与完整库存门禁冲突，以及sender掩盖原始claim失败。详见[runtime-blockers.json](runtime-blockers.json)、[execution-terminal.json](execution-terminal.json)及[验收矩阵](../../../../issue307-d1-v22-execution01.md)。

#323保持Draft、不合并；#307保持OPEN。失败、预算、旧pins、11份survivor、其他环境和全部备份保留；cleanup=hold-release=bypass=0。发送后survivor原生重读未执行，不能冒充新鲜证明。
