# #307 D1 candidate17/v27 非删除验收矩阵

新execution `6f40bca9-cfe2-4362-ad2b-4e38bbbd8453`，固定窗口2026-10-21新加坡14:00–14:40（UTC06:00≤now<06:40），只读恢复最晚08:40Z。本轮只准备与核验，不构成删除授权或调度。#332 Draft、不合并，#307 OPEN。

| 验收项 | 实际结果与边界 |
| --- | --- |
| 新代接线 | source `8cca345525af828a8c9461e75f2dc5bf5f1fb428`，统一START/END、CandidateUseUntil=END、recovery=END+2h；六新身份与旧身份分离 |
| 源码独立评审 | PASS_BOUNDED_C17_SOURCE_PREPARATION_REVIEW，0有效阻断；原normal/-O各57方法：56PASS/1明确symlink权限SKIP，实际junction覆盖；Go274PASS/2明确opt-in SKIP |
| source PR CI | run39414710061，event=pull_request，精确source HEAD13/13SUCCESS；不是main CI或最终发布HEAD CI |
| v26保留 | 原76批准引用、失败terminal、25私有seal-history、6case/6recovery单字段撤销、三旧预算归属/挂载/空状态、九旧backup及原构建字节通过；旧IAM UNCONFIRMED历史不改写 |
| 旧CID完整附录 | 固定gate445 + 原v26技术reader3 =448；原445报告保留。额外3的fresh PID0/norestart、Disabled/maxDELETE0、反向Enabled内存SHA等于原allocated-profile |
| 新分配 | 86 owner、15新卷、3internal网络；原源/旧环境不重建；三新预算UID/GID10001、完整挂载全集、实际为空 |
| 非删除native准备 | 三组PASS；17版本、29known（28 sole/no-groups+1固定NONE）、9 fresh NoSuchUser；三源Core全关系/roles/ACL前后不变；九新backup、独立clean/rehearsal/native恢复、四业务重跑；20旧事实multiset保留 |
| 隔离业务边界 | native_object_put=0/native_delete=0/source_business_mutations=0；真实in-flight late hold与mixed provider outcomes仍NOT_RUN，不能用rehearsal替代 |
| 冻结 | candidate `20d08f0a2d334e779b64f0c082515131c285d9fd005c6fbaa94294b147f38d42`；bindings `77ce0be4d5dccd95a276a5b4c3f060689bea870f4f3caef1a827ef81154efa24`；template `7319519cb8d18cd024ac8b80bfe7552b27d858d5407c9e59ac8521afe1ab11c3`；2117 source成员、12原build成员、source ZIP `ed0c07e7f706b44049010b5a1770835d24da9a4d30b51872346a838b840ad04d`；Go离线validator PASS，DB/provider I/O=0 |
| 索引接线 | 新writer直接输出metadata files-map，freeze与完整入口同一typed auditor；准备plan核1240成员引用；原digest-map历史按其固定旧契约核对，无猜测/回退 |
| freeze长路径失败 | 原stderr/零输出/空partial目录保留；仅子进程longpaths支持后同源码成功，source/binary/gate不变、native未重跑；独立有限恢复附录PASS |
| 冻结后完整入口 | PASS_COMPLETE_SIX_NATIVE_ENTRY；三组7+6+4=17版本，125技术GET/HEAD/admin只读请求对、amount=NOT_ASSESSED；29 known/9 absent；官方policy Equals，raw顺序变化不误判 |
| 完整入口关闭 | 三组closure无failure且all_closed；6case/6recovery跨字段、30false、9新+27旧backup、三空预算、实际topology均通过；三个新技术reader关闭 |
| 宿主封存anchor | packet Git `462fae3a6cddde5417cbdf2cae4be1a384a5f1a0` 与同SHA candidate/bindings/template读回一致；只作containment定位，不是发送授权 |
| 冻结包独立复评 | 以 `candidate17-review01/independent-final-review01.json` 最终状态及0阻断为准，当前正在复评；原source PASS不冒充packet PASS |
| 精确授权/调度 | NOT_RECEIVED / NOT_CREATED；旧授权不复用，旧heartbeat保持PAUSED |
| 本轮真实删除/完整D1 | DELETE=0；六源目标NOT_RUN；完整D1 NOT_ACCEPTED |

初期 `issue307-d1-candidate17-preparation-plan.md` 是不可变旧准备快照，最终绑定以本矩阵、完整确认文本、窗口手册及冻结packet/独立最终回执为准。已保存失败和跳过不覆盖为PASS；全部survivor、旧pins、新旧预算、备份、失败/UNKNOWN/真实attempt/成本保留。cleanup=0、hold-release=0、bypass=0。
