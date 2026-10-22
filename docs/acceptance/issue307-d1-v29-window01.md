# #307 D1 candidate19/v29 固定窗口实际结果

记录：2026-10-22T06:07:11.385425+00:00。execution a5d1e569-f80e-4b38-8f32-d8139a30a077。状态：FAILED_PR_HEAD_DRIFT_PHYSICALLY_SEALED_NO_DELETE。完整D1未接受，#332保持OPEN/Draft/unmerged，#307保持OPEN。

2026-10-22新加坡14:00窗口唤醒后，第一轮真实GitHub门禁发现PR当前HEAD 17c5248a868376603ae9c3951123c545f2f7e56f，授权/冻结要求caab46da8761e71e1d80d0c0931707a410c13a4d。GitHub原始commit回执证实03:11:35Z（新加坡11:11:35）将main合入PR分支，父提交为授权HEAD与main a25ebac1c930d749daa085ef63ca3db8f0e7d6b8，涉及72个文件，包括运行代码和migrations 70/71。不能用旧HEAD绿色PR CI替代新HEAD门禁，也不能在窗口内修改冻结限制继续执行。

| 验收项 | 实际结果 |
| --- | --- |
| 当前授权原文/924批准引用 | 原字节匹配；不复用旧授权 |
| 精确HEAD前置 | FAIL；本地仍为授权caab46da，远端PR为17c5248a |
| PR/Issue状态 | #332 OPEN/Draft/unmerged；#307 OPEN |
| 旧批准HEAD CI | pull_request成功，仅为旧HEAD事实；未冒充新HEAD或main CI |
| 完整窗口native/Core/IAM/17版本核验 | NOT_RUN；在启动源环境前停止 |
| P2/T1/T2/H2A/M1A/M1B | 全部NOT_RUN；实际DELETE=0/6，无新增dispatch/provider invocation |
| 新权限启用/签发送proof/source启动/Core Commands | 全部0 |
| 固定seal | 已对全部组执行；103步骤中100确认、3个disable-all因native原已停止而无fresh IAM读回 |
| IAM关闭新鲜证明 | UNCONFIRMED_NATIVE_STOPPED_NO_SOURCE_START；不启动原生服务补查，不声称整轮PASS |
| 实际物理封存 | 820固定已知CID全部stopped/PID0/noports/norestart；30permissionfalse；6case/6recovery撤销；3gateway sealed |
| 新旧预算 | 21旧预算tar SHA与完整挂载CID集合不变；3新预算SHA不变且仍为空，未消费或退款 |
| 备份/原证据 | 57备份SHA不变；924批准引用及原授权/调度索引不变；seal原profile字节在私有seal-history追加保存 |
| pins | 本次无Core启动或Command，不释放/改写旧pins；fresh Core pin查询NOT_RUN |
| 原生库存 | 本轮未重新读取；不把历史17版本结果冒充窗口内fresh |
| 调度 | 307-d1-candidate19-v29已删除，实际数据库与TOML均无记录；不补发、不新建调度 |
| 窗口/保留限制 | 不延长、平移、重开、UNKNOWN重发；cleanup=0、hold-release=0、bypass=0 |

已保存原始失败tool回执、6项GitHub fresh响应、actual merge commit、全部固定seal结果、物理关闭/24预算/57备份读回及哈希索引。原失败和旧预算/pins保留，原授权/准备矩阵及其历史NOT_CREATED/NOT_RECEIVED不回写。

当前设计边界是：冻结执行依赖的PR HEAD仍可被更新，门禁只能发现漂移后停止。下一步应先明确冻结期间PR分支的更新归属与执行绑定，再评审替代方案；本次不自动生成新候选或窗口，不复用此授权。

[实际失败与封存](evidence/issue307-d1/delete-confirmed-v29-execution01/window01/post-seal-readback.json) · [根因证据](evidence/issue307-d1/delete-confirmed-v29-execution01/window01/head-drift-root-cause01.json)
