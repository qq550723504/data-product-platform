# #307 D1 R1 / candidate11-v21 验收矩阵

本轮为 **PREPARED_NOT_AUTHORIZED**。#323保持Draft、不合并，#307保持OPEN。旧v20失败与授权原字节保留，不恢复旧execution，不复用旧授权。真实删除=0，全量D1仍NOT_ACCEPTED。

新execution：`602acafe-065a-4385-b739-5776408f5448`。人类选择用于准备的窗口：**2026-10-18新加坡22:00–22:40（UTC14:00–14:40）**；只读恢复上限UTC16:40，即新加坡10月19日00:40。尚需人类核对并另行精确授权，未建立新调度。

| 验收项 | 证据与结论 |
| --- | --- |
| 根因 | 旧生成器复制旧CandidateUseUntil，仅更新Base窗口；v20 runtime严格门禁正确拒绝。旧预检/离线只检查哈希，遗漏字段关联 |
| 统一窗口来源 | `remaining_scope.go` → `remaining_window.py`；generator覆盖Batch、Base起止、CandidateUseUntil；恢复截止从同一END+2h派生 |
| 完整预检顺序 | 新完整CLI先验证实际七case/七recovery的语义和SHA，再读permission/inspect/启动source；无一致语义不得进入I/O |
| 离线语义 | public profile-windows SHA由candidate绑定；即使重新计算配置、receipt、candidate SHA，错期限/执行/Base/成员等仍拒绝 |
| 回归 | R1 normal/-O各4方法（离线11类反例、完整CLI15类反例）；独立另加6类实际配置错误但投影伪好的反例；完整native录制回放各5；关闭故障各1；Go五包及未缓存nil-pool门禁通过 |
| 源码独立评审 | [R1 source review](evidence/issue307-d1/remaining-r1-review01/source-independent-review.json)：PASS_BOUNDED_INDEPENDENT_R1_SOURCE_REVIEW，remaining_blockers=[] |
| 首轮非删除准备失败 | [PG01 failure](evidence/issue307-d1/remaining-renew-r1-01/failure.json)：默认Docker地址池耗尽，未启动任何资源；证据保留，未覆盖重跑 |
| 实际六项Renew | [PG02 acceptance](evidence/issue307-d1/remaining-renew-r1-02/acceptance.json)：独立pg_restore副本，六条追加；12份旧policy原样保留，变为18；无native/网关/IAM/发送者，provider_io=0；两CID停止/PID0 |
| 窗口配置 | [cross-field receipt](evidence/issue307-d1/remaining-candidate11/cross-field-windows.json)：七case CandidateUseUntil=2026-10-18T14:40:00Z，七recovery截止=16:40Z；实际私有字节复核通过 |
| 冻结运行源码 | `3d62dacb36b94096a364f32525853fbe95aaa2ed`，829文件；packet anchor `c00d4a8f85aaaaa71b9c19c76d042dee6aad1802`；源码不再修改 |
| 新候选 | [candidate](evidence/issue307-d1/remaining-candidate11/candidate.json)，7目标/11保留/18版本；fresh source/native/binary/Core/IAM/gateway/owner/预算/恢复配置全绑定 |
| 冻结离线校验 | check-remaining-candidate11.py：PASS_CANDIDATE11_OFFLINE；829源码/1122raw，Go实际离线候选检查PASS；未授权 |
| 新完整原生CLI | [complete](evidence/issue307-d1/remaining-preflight-native04/complete.json)：PASS_COMPLETE_FRESH_NATIVE_PREFLIGHT，18版本/76 GET-HEAD；完整库存/保护/禁用IAM/policy官方语义，三Core/roles/ACL前后一致 |
| 技术实际调用 | [ledger](evidence/issue307-d1/remaining-preflight-native04/technical-read-ledger.json)：准备76 + 完整CLI76 + bootstrap admin GET16 = 168读取；无PUT/DELETE，amount NOT_ASSESSED；不声称零provider调用或零成本 |
| 原始失败身份状态 | v20两组最终fresh IAM未确认事实保持；本轮新只读证明三组当前disabled，不回写旧封存结论 |
| 封闭否定回归 | 17实际operator/wire/recovery及7case在源Core/native停止、权限关闭/窗口未到时全部拒绝；不冒充实际业务成功 |
| 物理与预算复核 | [readback](evidence/issue307-d1/remaining-window-review05/physical-close-recheck.json)：核对的248 retained CID均stopped/PID0/noports/norestart，07–11合计160permission false；七新case、七受限recovery、三空预算完整挂载集合通过 |
| 固定封存工具 | review05/window_support.py只更新本代固定绑定，复用已评审关闭逻辑；normal/-O各14；缺新授权check拒绝，不生成授权；mock不冒充真实封存 |
| 最终独立绑定复评 | [final review](evidence/issue307-d1/remaining-window-review05/independent-final-review.json)为最终记录；只有PASS且remaining_blockers=[]才进入之后的人类精确确认 |
| 精确方案 | [完整确认文本](issue307-d1-candidate11-confirmation.md)含7份目标、11份保留及全部冻结绑定；[窗口手册](issue307-d1-v21-window-runbook.md)固定前置、顺序、失败封存及UNKNOWN不重发 |
| 历史 | v17/v19/v20在各自冻结源码上严格审计通过；此为历史哈希/回归审计，不使v20原失败消失，也不授予新执行权限 |
| GitHub/CI | 最终发布HEAD、评审线程和该精确HEAD的PR CI在发布后实时读回；PR CI不是main CI；无合并、无main CI声明 |
| 尚未验收 | 本代源Core六Renew/实际业务/P-H-M有限发送与只读恢复均NOT_RUN；不能用PG副本、拒绝回归或GET/HEAD证明D1真实删除完成 |

授权后仍必须在固定窗口内重做全部新鲜前置；当前准备结果不代替窗口内核验。失败或过窗立即按固定seal逐组关闭，不能修改门禁续跑、平移窗口或复用旧授权。
