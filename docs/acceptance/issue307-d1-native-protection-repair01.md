# #307 D1 — 保留期延期方向：本地修复与验收矩阵

结论：本轮保护前置检查、失败诊断、非删除入口排序和两版本延期提案已完成本地回归及独立评审，当前切片源码阻断为 0。两份源版本仍未延期；实际维护写入口尚未实现，不能申请或执行保护写入。完整 D1 未验收，新候选/execution/删除预算未创建。

人类本轮选择“按保留期延期方向准备修复（推荐）”，问题明确仅准备、未授权实际保护写入或删除。原文语义与问题 ID 保存于 [preparation-direction01.json](evidence/issue307-d1/native-protection-repair01/preparation-direction01.json)。旧删除授权和旧受限分支维护授权均不复用。#332 保持 Draft、不合并，#307 保持 OPEN；本轮未访问或修改 GitHub，远端实时状态未重查。

## 根因与修复边界

原冻结清单中 retention 时间相等，只证明与历史快照一致，不能证明当前仍受保护。C23 原生读回的 control-retention 保留期为 2026-10-23T06:25:44Z，mixed-C 为 06:33:33Z，均早于当次实际 source 核验。领域中“已过期 retention 不再阻止处置”的语义正确；修复限定在 fixture 验收前置条件，不改变 Core 处置资格语义。

完整只读入口与非删除准备共享 `native_protection.py`：要求完整 Copy/version/provider/bytes/SHA 一致、GOVERNANCE、LegalHold=false、实际观察新鲜、当前未过期且覆盖固定恢复截止。三组 SOURCE 全部通过后才能开始任何备份或业务 rehearsal；RESTORE 同样先检查保护，再 enable inspector。批量 SDK receipt 最大 5 分钟，Go 实时 dispatch 观察最大 1 分钟，两者适用对象不同。

Go protected-C 路径按固定 C CopyID 进入门禁，再核完整冻结 Copy 与实际 StorageInspector 观察；同 ID 错 version 无法绕过。固定期限使用编译窗口 END+2h，短 rehearsal 不缩短要求。dispatch_error、admitted、attempt_present 分别留证。旧 generic “native retained C admitted”文本不证明实际 admission 或物理发送，原失败原字节保留。

权威有限契约见 [contract.md](evidence/issue307-d1/native-protection-repair01/contract.md)。沿用已有 StorageInspector、官方 MinIO SDK、日志/Cost、owner containment、Git 归档；未新增通用基础设施。

## 验收矩阵

| 项目 | 状态 | 实际证据与边界 |
| --- | --- | --- |
| SOURCE 三组保护检查先于任一 backup/rehearsal | PASS_LOCAL | 成功组合与 expired mixed 组合；源失败时没有业务副作用 |
| RESTORE 保护检查先于 inspector enable | PASS_LOCAL | 完整依赖组合拒绝过期/错误版本，finally-close 保留 |
| 完整原生核验 CLI 接入保护门禁 | PASS_LOCAL | 文件/Docker/PG/checker doubles 成功路径及反例；不是实际17版本核验 |
| C 实时完整身份、当前保护、固定 horizon | PASS_LOCAL | 官方 SDK+已有 inspector 的 localhost GET 协议测试；4 顶层 Go 测试，保护组合含15子案例 |
| 安全拒绝原因与独立 admission/attempt 诊断 | PASS_LOCAL | 不序列化任意原始错误；不把错误当作 admission |
| 延期 exact-two typed proposal/check | PASS_LOCAL | 完整身份、旧保护、期限/max1/max2/UNKNOWN、不扩权反例；无 apply 模式 |
| Python 当前修复与关联回归 | PASS_LOCAL | 8 套件正常及 `-O` 各45方法；包括35当前切片、10归档/窗口/冻结关联方法 |
| P 本地 Go 三包 | PASS_LOCAL | final02 exit0：reader/storage/pilot |
| 实际 R Git ZIP 重建及离线契约五包 | PASS_LOCAL | final03 exit0，核原始 ZIP SHA/字节、直接父/增量和8 shared members |
| 独立终审 | PASS_BOUNDED | 只读代理独立 normal/-O各35方法、P/R各4个Go顶层测试及归档/771成员；源码阻断=[] |
| 原 C23 失败与冻结证据保留 | PASS_FILE_AUDIT | 5954成员0漂移，public/private terminal一致，原 C23 tracked_diff=[] |
| 本轮17版本实际原生只读重验 | NOT_RUN | 本轮未启动源环境或 Docker；local doubles不替代它 |
| 两份源版本的实际保护延期 | NOT_AUTHORIZED / NOT_RUN | 人类只选择准备方向；未调用 PutObjectRetention |
| 一次性维护写入口及精确 owner/binary/profile/预算绑定 | NOT_IMPLEMENTED | 后续必须先实现、非删除审查、冻结，再呈现可授权方案 |
| 新候选/execution/独立删除预算/窗口绑定 | NOT_CREATED | 不恢复 C23，不复用旧授权 |
| 新修复 PR CI / main CI | NOT_RUN | 未发布、未解锁、未合并；本地回归不冒充 CI |

原失败也保留：初次组合测试 harness 在模拟拒绝后仍读取成功 complete.json，修正的是测试断言；实现当时已拒绝并关闭。初次 R archive spec 用旧 base，直接父检查实际拒绝；现已绑定真实直接父，门禁未放宽。最终成功回执另存，不覆盖这些失败。打包时一次 Windows 默认 GBK 读取 UTF-8 评审 JSON 的错误另存，使用显式 UTF-8 复核原索引和评审字节成功，源码无改动。

## 两份版本的延期提案

仅提议将以下现存版本的 GOVERNANCE retain-until 延长到 **2026-10-24T12:40:00Z（新加坡10月24日20:40）**，保持 LegalHold=false、内容、version、Copy/DatasetVersion/provider/incarnation 不变：

| 组 / namespace | key | exact version | 原 retain-until（UTC） |
| --- | --- | --- | --- |
| business-races / dpp-lifecycle307-quarantine | fixture/control-retention.csv | 2606d71f-207b-42a7-b671-4b7f4ca4d194 | 2026-10-23T06:25:44Z |
| mixed / dpp-lifecycle307-quarantine | fixture/m1-c.csv | 81a8402e-390f-4247-9d34-552df5eadeb3 | 2026-10-23T06:33:33Z |

全部 source CID、Copy/DV/provider/incarnation/bytes/SHA、SDK options 见 [exact-two-proposal01.json](evidence/issue307-d1/native-protection-repair01/exact-two-proposal01.json)。每版本最多1次、全局最多2次 metadata mutation，UNKNOWN不重发；DELETE、content PUT、hold-change、bypass、cleanup、budget-reset、pin-release 均为0。该提案不是执行授权，也未生成新候选或可执行维护绑定。

下一必需实现与验收顺序已明确于 [renewal-runbook01.md](evidence/issue307-d1/native-protection-repair01/renewal-runbook01.md)：复用官方 PutObjectRetention，完成独立持久维护预算、physical start/observation/cost、exact transport allowlist、可信 owner 与失败封存；离线/非删除评审后冻结 source/binary/profile/topology，才提交具体维护方案供人类授权。授权后的实际写入须另行 fresh bytes/inventory/hold/retention 前后验收，再追加新期望 profile；不得覆盖旧冻结清单。此后才允许准备新的删除候选。

## 本地来源与证据索引

- P 工作树 `d1-protect` 基于 `1841d39d9ab5a08a823f83ab8e9a625b619727e2`，修复未远端发布，当前按实际文件 SHA 审查；不是新的 execution-binding。
- R 本地 commit `8e53ea564f9b81bdebf7c31d6a99060d1ec3b7db`，直接父 `92b9801dfb7e46295e17165e7762907cb6e87be2`。Git ZIP 6440579 bytes / 771选定成员，SHA256 `a57c946610b968a8eaaa1414e85367af43938784c973d723a28a1f7355a5c0f5`。8 shared contract 成员逐项 LF 相同；未声称所有 raw=Git LF。
- [independent-final-review01.json](evidence/issue307-d1/native-protection-repair01/independent-final-review01.json)：独立代理结论，由父线程序列化，逐项核实际审查 SHA。
- [evidence-index01.json](evidence/issue307-d1/native-protection-repair01/evidence-index01.json)：78项显式 files-map-v1，SHA256 `739f8a8d9deac67fc86f3961f1358ee230d7d0b430b93fb51291077b728de892`。使用既有 evidence_indexes.audit_index 完整通过；索引不包含自身及后续矩阵/checkpoint，无循环哈希。
- [index-audit01.json](evidence/issue307-d1/native-protection-repair01/index-audit01.json)：78项索引与11项独立审查字节匹配。后续打包回执另存，原索引不改写。

原 C23 terminal 仍为 `FAILED_NONDELETE_NATIVE_PROTECTION_EXPIRED_SEALED_NOT_AUTHORIZED`，SHA256 `6bf40cca760fdf81532ba52dd2677d77180f49c80efac97db5fb03f85103c8b2`。本轮源环境启动=0、真实 provider 调用=0、保护写入=0、DELETE=0；旧预算、pins、失败、UNKNOWN、attempt/成本和备份未改写。
