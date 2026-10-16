# #318 最终收口评审与可合并条件

2026-10-16。**当前有限切片技术 blocker=0；#318 保持 OPEN / Draft，不合并。** 本轮仅核对公开证据、Git/GitHub 和宿主封存 metadata，provider I/O、DB I/O、container start、DELETE、cleanup、merge 均为0。

受检实现/执行提交为 `f1b03843ce26929d10cb71e744082ba77b2ebdda`，其 [PR CI 38837019057](https://github.com/qq550723504/data-product-platform/actions/runs/38837019057) 为 `pull_request`、实际相同 head SHA、13/13 SUCCESS。当前 main `d7c9ccdfe4fba066b512fe4adbcd1df281a2d0d9` 已完整包含在受检分支中，behind=0。此次收口只追加文档/离线核验回执，317项冻结 runtime 无变化；最终收口提交的 CI 另在 PR checkpoint 核对，不能沿用受检提交的绿色状态。

## 冻结的 merge contract

本 PR 交付有限受控 fixture 的 reference-demand / module-owner append-only release、合格 FK admission guard 和共享 fence，以及 LC-13 可信 owner stop、新 Retry Execution、credential containment/redemption。原 Execution/ISSUED、依赖、pins、attempt、成本、Event/Audit/Evidence/Outbox 保留。unknown reference/owner、未经验证的停止、native outcome UNKNOWN 均 fail closed。

candidate05/v15 的两份单独获批源版本删除及原生崩溃恢复，作为该有限切片的执行验收，现已完成；授权额度已消费完毕。保留四 survivor、全部备份与其他环境，cleanup/hold-release/bypass=0。没有公共删除 API、通用 IAM/owner/DR runtime 或第二套基础设施。

权威归属：`docs/architecture/data-lifecycle.md` / ADR-0015 拥有领域、并发和副作用规则；[reference contract](issue307-reference-demand-contract.md) / [LC-13 contract](issue307-lc13-resume-contract.md) 拥有本 PR 有限实现边界；[#307 D1 v15 执行矩阵](issue307-d1-confirmed-v15-execution.md)拥有该次已消费授权及实际范围；本文件只汇总本 PR 的收口判断。`docs/product/data-lifecycle-design-acceptance.md` 的完整 D1/D2–D6 范围不因本 PR 合并而被勾选完成。

Reuse Check 延续已评审结论：现有 Ports/SPI、PostgreSQL parent locks / delivery fence、Asynq/Outbox、native engine、MinIO/madmin、Cost/Audit/Evidence/Journal 复用；MinIO OSS policy evaluator 及 SDK/transitive 升级已在 LC-13 contract 说明，Core 不直接依赖 SDK，无新部署组件或 fork。

## 收口矩阵

| 门禁 | 核验结果 |
| --- | --- |
| reference / immutable ownership | 既有独立评审通过；当前317项 runtime LF hash 与获批 source 及受检 Git 内容一致。Git 原 runtime 含 CRLF，按已声明 LF contract 比较，未把 normalized hash 冒充 raw blob hash |
| LC-13 原生业务 | 继承已提交独立收敛回执：可信签名停止完整 pin set，新 RetryOf + native GET/PUT + SUCCEEDED；credential same-capability replay / expiry / unknown→confirmed containment；原 FAILED/ISSUED 保留。517项末阶段索引 payload 本轮逐项重核 |
| candidate05/v15 授权 / actual send | 新execution `9ea8f3e2-c805-44b4-b133-a36f50df9d15`，两 target 各一个新 attempt/claim/CONFIRMED UNKNOWN observation、各一次物理发送；旧614cd9ad授权未复用，无 UNKNOWN 重发 |
| 精确 absence / scope completion | 2 receipts / 1 completion，A receipt `01:34:40.893014Z` 早于 B attempt `01:34:44.882004Z`；terminal replay0新I/O；四survivor identity/bytes/SHA/hold/retention和完整inventory一致 |
| 成本 / 历史 | 157 storage calls / observations / typed costs，外加3个真实provider invocation成本，共160；本轮新增32 read +2 DELETE，quantity1；旧ledger逐identity保留，旧3份release不变 |
| Core / 停止回执 | 同bound dump恢复审计确认194表/2361→2851行的声明增量、旧行保留及fence90→97；原 execute 末尾审计误阻断、audit01/02预期错误及封存回执保留，audit03只读闭合，没有改写失败为原流程PASS |
| 证据完整性 | 635项唯一公开索引payload与受检Git raw bytes一致；现有strict checker另重核344历史raw及候选准备快照。准备快照的NOT_RECEIVED是原时点，不再用作当前授权状态 |
| 封存 | 本轮仅Docker inspect / 文件hash读回：113个完整ID实际容器全停止/PID0/noports，权限关闭、gateway sealed、4个冻结binary及26份旧seals/backups原字节不变；native IAM disabled引用原权威回执，没有重启服务或再做管理RPC |
| 原独立评审 / 本轮性质 | LC-13、candidate05准备和v15实际执行独立评审均PASS/blocker0；本轮为主线程最终收口复算，未宣称独立重演provider/SQL或全部历史测试 |
| GitHub线程 / review | unresolved threads=0；正式review提交=0。required approving review count0、code-owner approval=false；已有独立离线评审来源另列，没有虚构GitHub APPROVED。现有60项提交均有GitHub关联author/committer |
| main规则与集成 | active `main-protection` ruleset23543992：required线程解决、strict `required` check、linear history、squash/rebase，并开启`require_extra_approval_for_unattributed_changes`条件规则。结束Draft时仍按GitHub实际规则核对额外审批要求。旧branch-protection端点404不能解释为main无保护；受检HEAD MERGEABLE/CLEAN、behind0 |
| 最终收口提交 | 文档/回执提交后重核same-head CI和Git原字节；状态见PR checkpoint。保持Draft，不关闭#307 |

[离线复核](evidence/issue307-d1/pr318-final-closure01/offline-audit.json)、[GitHub/rules/CI快照](evidence/issue307-d1/pr318-final-closure01/github-preflight.json)、[只读宿主回执](evidence/issue307-d1/pr318-final-closure01/host-metadata-readback.json)、[完整索引](evidence/issue307-d1/pr318-final-closure01/evidence-index.json)。原v15的81 payload/index及旧回执未改写。

## 本轮发现与处理

| 分类 | 发现 / 收口 |
| --- | --- |
| 已修正的当前状态冲突 | PR正文仍停留v14 / `2babc930`，写新授权NOT_RECEIVED及实际DELETE/absence/completion NOT_RUN。改为本PR最终有限行为、v15实际成功和当前CI/边界，移除过期的“当前”判断 |
| 已修正的入口时间边界 | reference contract、LC-13及D1阶段入口仍可能被读为当前状态；追加此收口入口并标明原文阶段含义。原观察、raw/index及失败历史保持原字节 |
| 已关闭而不复开的native接受缺口 | candidate05准备评审列出的native成功DELETE、response-loss/absence恢复、two-target completion现由v15实际回执闭合，不再索取新授权或循环刷新pre-delete snapshot |
| 不扩大当前切片 | 完整D1的真实删除竞争、late-hold和三copy混合结果仍未验收；本PR既不声称这些PASS，也不要求为此次收口再执行删除。继续由open #307跟踪 |

## 距离可合并还缺什么

1. 本轮最终文档/证据提交的 **exact-head PR CI** 绿色、base仍包含当前main、closure结果与review线程状态无漂移。该步骤由主线程完成并记入PR；新失败或真正的安全/历史/并发/required-path缺陷才重新打开当前scope。
2. 完成上项后，本有限PR没有已知额外技术开发门禁。**还需要用户另行明确允许结束Draft及合并，并在当时满足GitHub实际审批规则。** 当前指令要求继续Draft，因此本轮不转Ready、不合并，也不请求追加删除授权。无需为了“可合并”再跑一轮真实删除或重建候选。
3. 真正获准合并时，按当时HEAD/base/checks重新核对并使用规则允许的方法；合并后核对实际merge SHA及匹配的`push/main` CI，作为合并后验收。当前PR CI不替代该结果，也不授予部署或关闭#307的权限。

## 保留在 #307 / #296 的后续边界

完整D1仍未接受，至少还需按其原contract独立安排：真实processing / DirectData与实际DELETE的双向竞争（LC-11/12）、in-flight DELETE late hold（LC-06/07相关组合）、三copy删除/timeout/retention mixed outcome（LC-14）。既有PG/非删除native/两版本成功证明各保留自己的范围，不合并成未执行场景的PASS。任何新增真实删除都必须另行精确授权，本次额度不能沿用。

[原生非删除补验](issue307-d1-native-completion.md)的pre-ISSUED、COPY/metadata worker faults、archive/restore业务重跑已完成；[LC-13成功续跑和containment](issue307-lc13-continuation.md)也已完成，不能再把这些列为当前待开发。通用external owner stop、生产多租户IAM/完整delegation chain、D2广泛接入、D3–D6、生产DR、全域介质/外部recipient销毁继续属于后续范围。原人工解释验收保持其暂缓状态，不用技术PASS替代人工验收。#307及父Epic #296均保持OPEN。
