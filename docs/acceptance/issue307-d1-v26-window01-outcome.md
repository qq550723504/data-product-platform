# #307 D1 candidate16/v26 — window01 actual outcome

本execution aa2605a5-1fcf-4e9d-b6f2-8f5b13321b8a 已终结：FAILED_OFFLINE_AUDIT_SEALED_NO_DELETE。不能在剩余窗口内重新开始、修补续跑或复用此授权。原授权、候选、调度快照和失败事实保持原字节。

本次失败来自临时添加的宿主附件审计脚本：把独立证据索引当作members列表读取，实际冻结schema是files映射（53项），产生KeyError。不是已证明的库存、权限或产品源码漂移。此前76项批准引用、私有授权/模板原字节、HEAD均匹配；实时该HEAD PR CI13/13成功、评审线程为空、#332 Draft/OPEN及#307 OPEN。固定完整只读入口尚未启动。

| 验收项 | 实际结果 |
| --- | --- |
| 精确HEAD | 74aa674db18dd68966d070e9f04173bb37ad1a7c，tracked无变化 |
| 失败阶段 | 临时离线索引审计，KeyError: members；原错误保留 |
| 固定完整窗口内只读核验 | NOT_RUN |
| P2 / T1 / T2 / H2A / M1A / M1B | 六份全部NOT_RUN，physical DELETE attempts=0 |
| Source / identity enable / case / wire / sender / recovery / proof | 全部0，批准文本未给worker |
| 固定seal | 103步骤；100步骤完成，3组disable-all因native始终停止而无fresh identity读回；整体UNCONFIRMED |
| 物理及本地关闭读回 | 445 CID stopped/PID0/noports/norestart，含86本代owner及359旧保留CID；30permission false、6case及6recovery撤销，3gateway stopped/sealed |
| 新预算及备份 | 三新budget仍空、九新backup SHA匹配；旧预算/pins/备份及失败证据保留 |
| 调度 | 同一candidate16/v26 heartbeat已PAUSED，next_run_at为空；不重开、不平移、不重复执行 |
| 完整D1 | NOT_ACCEPTED |
| #332 / #307 | Draft不合并 / OPEN |

[原失败](evidence/issue307-d1/delete-confirmed-v26-execution01/window01/offline-audit-failure.json)、[固定seal](evidence/issue307-d1/delete-confirmed-v26-execution01/window01/seal.stdout.json)、[物理关闭读回](evidence/issue307-d1/delete-confirmed-v26-execution01/window01/physical-closure-readback.json)和[终态](evidence/issue307-d1/delete-confirmed-v26-execution01/window01/terminal.json)独立保存。seal进程exit0不能替代all_steps_confirmed=false的事实；没有为取得fresh IAM证明重新启动源环境。

根因修复应把索引审计收进固定受测入口，分别覆盖独立索引files映射、授权/调度索引members列表及跨格式拒绝组合，完成完整成功路径回归和独立评审后再考虑后续方案。本轮不改源码、候选、二进制或门禁，也不创建下一代或请求重复授权。cleanup=0、hold-release=0、bypass=0。
