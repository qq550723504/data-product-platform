# #307 D1 candidate18/v28 非删除验收矩阵

新execution `4fd5a7e7-779e-41d7-a35a-3bab7dd615b5`；拟定2026-10-21新加坡22:00–22:40（UTC14:00≤now<14:40），只读恢复最晚16:40Z（新加坡10月22日00:40）。本轮仅准备与核验，删除授权NOT_RECEIVED，调度NOT_CREATED；#332 Draft、不合并，#307 OPEN。

| 验收项 | 当前事实与边界 |
| --- | --- |
| 新代接线 | 运行source `618f484061c3624ff2195c0b591e7d8e3d743ce1`；统一START/END、CandidateUseUntil=END、recovery=END+2h，六新IAM身份及新execution/预算与旧代分离 |
| 当前Core与旧policy | business完整Core精确用闭合03后的ffdf7501…；另两组保持原基线。aab0 policy checkpoint原字节、五head及已过期期限保留，不Renew、不用旧policy CoreSHA替代当前Core |
| 源码独立复评 | PASS_BOUNDED_C18_SOURCE_PREPARATION_REVIEW，remaining_blockers=[]；独立25命令通过，Python normal/-O各66方法及3定向Go包。作者5Go包-count=1通过 |
| 回归结果 | 原CI driver61命令先59PASS、2因未冻结packet而失败；原失败保留。冻结后attester normal/-O实际PASS，未覆盖原报告。完整入口/索引、bytes/text、窗口跨字段、预算归属/完整挂载、非空及旧集合隔离反例保持覆盖 |
| 旧环境与失败保留 | 固定旧623 CID；18旧预算的原SHA及完整挂载集、531旧控制/备份文件、91失败维护文件、闭合03及原826批准成员/771原失败成员均保留。新目录不扩张旧集合，旧目录扩张仍拒绝 |
| 新资源 | 86新owner、15新卷、3独立internal网络；新预算3份归新execution/nonce，UID/GID10001、完整挂载集、首次发送前实际空；原源及其他环境不重建 |
| 原生非删除准备 | 三组PASS、92 closure、19profile撤销；17现存版本、35 known disabled（34sole/no-groups+1固定NONE）、9 fresh NoSuchUser。官方ParseConfig/Equals，raw顺序差异不误判 |
| 真实请求与成本 | 准备source/restore GET/HEAD合计274，另66仅MinIO admin，全部340 start/observation成对；冻结后完整入口137技术请求对。amount=NOT_ASSESSED，file-only provider_io=0不冒称原生零请求 |
| 原源Core与备份恢复 | 三源全关系/roles/ACL before=after=固定基线；9新备份恢复至独立clean Core/native，另独立rehearsal Core，9新及39旧备份（36完整checkpoint文件及3份c14失败partial）保留 |
| 业务重跑边界 | 仅隔离rehearsal Command；20表旧rows multiset保留，Cost/Audit/Evidence/Outbox对应追加。P2/T1/T2不重跑旧Processing/DirectData、无新输出；源业务Command=0、源对象PUT/DELETE=0 |
| 尚未真实验收 | H1仍需可信closed-owner真实闭合；H2真实204之后late hold/revoke/UNKNOWN、三副本mixed outcomes/checkpoint-before-lookup及六源DELETE均NOT_RUN，rehearsal不替代它们 |
| 冻结与离线校验 | candidate `bb0924c1814b0bcdecd33d3b57a21a24beaafce2a3d64e10e5dac9aba1947f63`；bindings `8d021af81d905cc6de255bf84e460fc0b416d5af5bacc8388cfbb2b423a60624`；template `5cd148132b380b4a3361ae7210c8cc02fe6932fee3824ae77b4fec0cc2523a90`；2129 source raw/LF成员、12原build成员及source ZIP `4f1a7f8fd0c8a6df4a35136be98667033e2f69aa6b3aaa7d9a96ce2c60a2d9ad`；冻结Go离线validator PASS，无provider/DB I/O |
| 冻结后固定完整入口 | PASS_COMPLETE_SIX_NATIVE_ENTRY；17版本7/6/4、35disabled、9absent、P1 exact NoSuchVersion404、full namespace inventory/hold/retention、Core不变及全部backup/topology/budget通过，137真实技术请求对 |
| 最终关闭与预算 | 712已知CID=623旧+86新+3新技术reader，全stopped/PID0/noports/no-restart；30permission=false，6case/6recovery哈希/窗口精确；18旧预算保留、3新预算仍空 |
| 宿主封存绑定 | packet anchor `baa1555dab9b4fcaf058109b53a81e49ac13ccda`与实际candidate/bindings/template原字节相同，仅作containment，不授予发送；35成员宿主绑定索引独立核对 |
| 冻结包独立评审 | PASS_BOUNDED_C18_FROZEN_NONDELETE_PREPARATION_REVIEW，remaining_blockers=[]；34成员评审索引及808外部输入索引实际全量通过，9次独立Windows聚合及attester normal/-O各3方法通过 |
| 最终发布HEAD PR CI | 精确HEAD/event=pull_request及全部必要job结果在本代publication-checkpoint01.json发布后追加，避免把未来run号写进自身HEAD；该回执未存在/完成前不声称通过，不拿旧HEAD绿灯代替，不冒称main CI |
| 本代授权/调度 | NOT_RECEIVED / NOT_CREATED；旧07/10/11/12/15/16/17 heartbeat全部PAUSED，旧授权不复用 |
| 本轮真实删除/完整D1 | DELETE=0，剩余六源目标NOT_RUN，完整D1 NOT_ACCEPTED |

[完整精确确认](issue307-d1-candidate18-confirmation.md)、[有限窗口手册](issue307-d1-v28-window-runbook.md)。source/packet/审批引用清单、全部实际原始回执与独立报告分开留证，后续真实授权/调度另存，不回写准备快照。初次bindings类型检查失败、模型旧断言及缺packet测试失败原字节保留；当时未启用源发送或执行DELETE。所有十一survivor、其他环境、全部备份、旧pins、新旧预算、失败/UNKNOWN/physical attempt/成本事实保留；cleanup=0、hold-release=0、bypass=0。

备份统计澄清：完整入口冻结摘要字段old_backups_retained=27保持原事实；它不枚举新增retained_v27集合。实际固定531文件中含旧39备份（完整36+失败partial3），加本代9共48，逐路径/SHA见candidate18-planning01/retained-backups48-readback01.json。额外9+3已在完整入口调用的state/historical_gate以及最终cold固定集合核验中实际通过。

最终静态审计已通过：7个显式files-map-v1索引，3348成员引用；1017项审批manifest SHA256 `3b8e4c6351d7147fb604f64cee57b9d34482c90dc8a7e439570a963d6f03620b`，final-static plan SHA256 `59f7d102aa425ba7684aaace9136701fb8f3bbea6c3952325287e169d3e36ac7`。同一冻结完整入口--audit-only实际PASS，不是native重跑或授权。第一次静态调用误填checker SHA，在审计/Docker/Source I/O前被拒，错误回执保留；从原allocation读取实际frozen checker `b05d9f331cb2126176021db402b189678a7b0a0c1aaa8776612e5751de5b7081` 后通过，未修改候选、二进制、配置或门禁。最终发布字段另按 [发布检查点](evidence/issue307-d1/candidate18-planning01/publication-checkpoint01.json) 核对；本矩阵不进入准备审批索引，后续新增回执不能覆盖既有静态快照。
