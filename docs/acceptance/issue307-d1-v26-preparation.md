# #307 D1 candidate16/v26 preparation

本代仅准备方案、备份恢复、非删除核验及冻结，不构成删除授权或定时续跑指令。新execution为`aa2605a5-1fcf-4e9d-b6f2-8f5b13321b8a`；人类选择的新拟窗口为2026-10-20新加坡18:00–18:40，即UTC10:00≤now<10:40；独立只读恢复最晚UTC12:40。冻结完成后仍需重新核对并明确授权。#332保持Draft、不合并，#307保持OPEN。

原candidate15/v25在窗口前独立评审发现CLI参数类型错误，旧执行已记BLOCKED_PREWINDOW_REVIEW并暂停原heartbeat。修复通过`Host.group`解析后调用既有方法，已在8a172cade7b5eab2d871fc75ac9d0963f9d27199独立复评、normal/-O各7项及精确HEAD PR CI13/13通过。新候选保留该修复，使用新的execution、窗口、源码/二进制/宿主绑定和独立停止资源；不改原冻结checkout，不复用原授权或预算。

## 有限范围与复用

范围仍是P2→T1→T2→H2A→M1A→M1B六个exact版本，每份最多一次、全局最多六次，UNKNOWN不重发；P1已在旧v22确认缺失，永久排除。十一survivor、全部其他环境、新旧备份、旧pins、新旧预算及所有失败/UNKNOWN/attempt/成本事实保留；cleanup=0、hold-release=0、bypass=0。

复用已验收的prepare-six-candidate、six_candidate_runtime、freeze-six-candidate、verify-six-native和window_six_support；源码中的单一窗口END继续生成CandidateUseUntil与恢复截止。复用现有MinIO官方SetUser disabled/单成员ImportIAMV2、policy ParseConfig/Equals、PostgreSQL、Port与Outbox→Asynq，不新增平行基础设施或业务生命周期。

原三源Core/native/network和17份实际版本继续作为原源；本代分配独立的停止owner、internal restore网络、15个新卷、三新空预算及clean/rehearsal恢复库。原c13/c14/c15全部保留；实际359个旧CID、c15的56项批准引用/原授权与BLOCKED证据、原预算完整挂载/空状态、九旧加九c15备份须先只读通过。c15的六个实际disabled IAM用户加入保留集合；本代新增六个不同身份后，完整核对23个已知身份（22 sole-policy、1严格NONE）及九个fresh NoSuchUser，不把缺失用户伪造成disabled。

## 验收矩阵

| 项目 | 状态 |
| --- | --- |
| 新窗口与新execution | 已选定；仅方案偏好，删除授权NOT_RECEIVED，调度NOT_CREATED |
| 旧资源与失败保留 | 实际只读359 CID关闭、c15九备份/三旧预算及原56引用通过；693个只读进程，Source/provider/DELETE=0 |
| CLI及六版本组合Python | normal/-O各34方法PASS，含实际main归档/单字段撤销及失败反例 |
| Go生成器/reader/checker/operator | 复验274 PASS/2明确SKIP；原9/4/4旧断言失败保留，当前23身份总数为11/6/6；新增真实c14/c15保留身份集合回归 |
| 新停止分配/预算初始化 | NOT_RUN |
| 三源只读/原生备份恢复/副本业务重跑 | NOT_RUN |
| 冻结与Go离线校验 | NOT_RUN |
| 冻结后固定完整只读入口 | NOT_RUN |
| 当前授权Host.check、窗口内fresh、发送/恢复/H1/H2/M1 | NOT_RUN；不能用准备PASS替代 |
| 实际DELETE / 完整D1 | 0 / NOT_ACCEPTED |

任何一次性分配、初始化、非删除准备或冻结失败均保留原字节并关闭已知owner，不在同一阶段覆盖失败或补丁续跑。root bootstrap与worker物理分离；所有发送及root permission保持false，新用户Source身份始终disabled，仅恢复副本的受限inspector允许非删除业务验收。未授权不materialize人类批准模板，不启动sender/wire，不签发送proof。
