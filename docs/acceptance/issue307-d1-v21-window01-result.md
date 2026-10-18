# #307 D1 candidate11/v21 固定窗口实际结果

**FAILED_STOPPED_SEALED_WITH_POST_SEAL_IAM_UNCONFIRMED。真实DELETE=0，完整D1未验收。**

execution `602acafe-065a-4385-b739-5776408f5448`，授权窗口2026-10-18新加坡22:00–22:40。窗口内先完成新鲜核验，随后我的补充宿主预算检查脚本出现 `AttributeError: 'str' object has no attribute 'decode'`，在22:03:39中止；22:04:15记停止事实并运行固定封存。未使用剩余窗口修补、重跑、续发或重开执行。本次同一heartbeat已PAUSED。

根因是临时宿主包装器用 `subprocess.check_output(..., encoding='utf-8')` 返回str，却复制了现有bytes包装器的 `.decode().split()` 调用。错误发生于读取business budget的挂载CID列表后、预算复制与后续源启动之前。这是我编写的宿主核验代码错误，不是原生存储策略漂移，也不是R1统一END门禁失败。实际失败工具原字节、原始异常及全部旧失败保留。

| 验收项 | 本次固定窗口实际结果 |
|---|---|
| 新授权/全部26项冻结引用 | PASS；原文、模板、envelope分别核对；user_retyped_template=false，旧授权未复用 |
| HEAD / PR状态 / CI | `8eef47a2edfbd9df2dc2a83467d9361a3fa4ec96`；精确HEAD PR CI39121716342、13/13 SUCCESS；#323 Draft/Open、未合并，#307 OPEN；不是main CI |
| 当前离线源码/证据 | PASS，829 source / 1122 raw；不可变准备快照NOT_RECEIVED保留 |
| 完整窗口内fresh只读入口 | PASS，2026-10-18T14:02:05.941220+00:00；18版本bytes/SHA/inventory/hold/retention、完整disabled IAM/官方策略语义、三Core/roles/ACL前后不变；自有source/helper已关闭 |
| 完整入口后的关闭检查 | PASS，58本代资源stopped/PID0，32权限false；未重新启动源 |
| 补充业务owner/网络/持久预算前置 | **FAIL，宿主脚本类型错误**；不得用后续终止观察补成执行前PASS |
| 六项源Core Renew | **NOT_RUN**；隔离PG副本准备结果不是源结果 |
| P1/P2/T1/T2、H0/H1/H2、M1业务 | **NOT_RUN** |
| root enable-read / enable-send、wire/sender/recovery | **NOT_RUN**；无worker机器授权文本materialize |
| 7目标发送 / 真实DELETE / 持久预算消耗 | **0 / 0 / 0**；UNKNOWN未重发 |
| 真实技术provider调用 | 76 GET，含16 admin GET及60对象/库存/保护GET；45次HTTP200、31次HTTP400，无transport error；PUT/DELETE=0、金额NOT_ASSESSED |
| HTTP400解释 | 固定verifier校验NONE/false时接受的NoSuchObjectLockConfiguration；不是对象版本缺失，18份版本内容另行验证PASS |
| 固定seal | 110项执行，107成功，3项disable-all无法fresh读回：native在只读入口结束时已停止；seal不重启源 |
| 关闭后的身份新鲜读回 | 三组均 **UNCONFIRMED**；保留14:02的disabled观察，不改称封存后新鲜证明，不报告整轮PASS |
| 最终物理状态 | 251 retained CID stopped/PID0/noports/norestart；32权限false，7 case及7 recovery撤销，3gateway停止且sealed |
| 保留与最终只读观察 | 9备份SHA精确、3budget归属/全部挂载CID精确且空；11保留版本、其他环境、全部备份、旧pins和失败证据保留。终止后预算观察不用于恢复执行 |
| 授权与调度终态 | no_resume=true，同一307-d1-candidate11-v21 heartbeat PAUSED；本execution不再续跑，不平移/延长/重开窗口 |
| 完整D1 | **NOT_ACCEPTED**；所有未运行项保留NOT_RUN |

后续必须先把必需的case/topology/budget组合收进维护中的固定完整核验入口，复用已有Docker wrapper并明确bytes/str单一返回契约，补齐完整入口成功组合及类型/归属/挂载扩张/非空预算反例、非删除原生验收与独立评审，再准备新候选、execution和窗口，另行取得新精确授权。不能仅修这一行后复用本轮授权。

原授权及窗口前矩阵保留为历史快照，不覆盖；本文件是追加的实际执行矩阵。原始完整回执见 [window-result.json](evidence/issue307-d1/delete-confirmed-v21-window01/window-result.json)、[失败](evidence/issue307-d1/delete-confirmed-v21-window01/failed-gate.json)、[逐项封存](evidence/issue307-d1/delete-confirmed-v21-window01/failure-seal.json)、[实际物理关闭](evidence/issue307-d1/delete-confirmed-v21-window01/physical-final.json)、[技术调用账本](evidence/issue307-d1/delete-confirmed-v21-window01/technical-read-ledger.json)。cleanup=hold-release=bypass=0。#323保持Draft、不Ready、不合并；#307保持OPEN。
