# #307 D1 本代受限发布方案：be38来源锚点

状态：**BLOCKED_REQUIRES_INTEGRATED_REVIEWED_P**。本文件供确认发布路线和边界；当前不能作为解锁或推送授权。计划JSON SHA256：`cce2ace00cc671b93e6fb7f6f1a0e00156cba24cd8123a031c35504613d4aa45`。

## 1. 当前可确认的绑定

| 字段 | 精确绑定 |
| --- | --- |
| 仓库 / PR / Issue | qq550723504/data-product-platform / #332 / #307 |
| 唯一远端分支 | `codex/issue307-post-v22-dispatch-closure` |
| 已审修复来源锚点 | `be38a68ded0bd4267a2c7711b5c4bc8bd1012e60` |
| 来源锚点的Git树 | `e1523ec55778ecd9db370992a3174e7753419ab4` |
| 当前远端P / 唯一发布基线 | `e54967e822847e97316e703ee413ffc1298cb627` |
| 两条分支共同祖先 | `8de3bc4c9f737c3c3e81fddb60491422ba6039f2` |
| 拟发布HEAD | **未生成；不能填be38，也不能以占位符请求发布授权** |
| GitHub原生精确规则 | `BPR_kwDOUdQLBs4FE_7b`，pattern为上述完整分支 |
| 当前保护 | lockBranch=true；isAdminEnforced=true；lockAllowsFetchAndMerge=false；allowsForcePushes=false；allowsDeletions=false |
| 完整非lock REST保护的规范化SHA256 | `77b0679be0f70f7858a864fd66c90aec78e63dbd387029919ad7a1693c6fc6ab` |
| 完整原始保护快照SHA256 | `8cd5ee417ac04d082edd44f3bd621a214934aaf4ae63b66ce5358a9e45d34760`；完整字段及原始API回执另存 |
| 当前远端精确CI | run39983308178 / attempt1 / event=pull_request / e54967e8 / 13项全SUCCESS；它不覆盖be38或未来P |
| 当前评审/状态 | 未解决线程0；#332 OPEN/Draft/unmerged；#307 OPEN |
| 本代发布授权 | NOT_RECEIVED；旧授权不复用 |

上述GitHub状态实测于 `2026-10-24T05:35:03.630055+00:00`。未来任何写入前必须重新读取，不能用本次快照当新鲜授权前置。

## 2. 唯一根因阻断：发布基线分叉

`git merge-base --is-ancestor e54967e8 be38a68d` 返回1。be38的三个本地提交从旧8de3bc4c派生，没有包含当前已发布P。

直接将远端分支替换成be38，按不启用rename推断的树比较会移除642个现有路径，其中620个是已发布的#307证据，另有5个保留期相关Go源码/测试/夹具；还会回退已发布的retention宿主接线与fixture R规范。这个差异不是发布清单，也未获准执行。

| R属性 | 当前已发布、必须保留 | be38历史比较值，不可用于回退 |
| --- | --- | --- |
| runtime source | `ecf40e6c7a9b3aa11166e6a56a5163a29a15691d` | `81a083751c01a281d42df594f6e91012255081f1` |
| profile / execution | `candidate23/v33` / `4fad1007-6595-4037-a7a5-1d8d38cb8bcc` | candidate22/v32 / 0e5cb8f3-46a9-4339-88e4-7154d6ebe13f |
| 原始runtime ZIP | `5e86e36808f00f62b9e9c7eb9b6b76d0c306958f4c5118c6d9d4eb4ff00de092`，6479934 bytes | 旧candidate22归档，保持原件 |

本方案不修改或启用上述任一旧candidate/execution/window，也不构建runtime。发布源必须先整合当前P，不能强推、rebase或通过修改保护位允许回退。

Git原生merge-tree已仅计算本地对象预览：`745b294391b959edf52570e08e89219fcdbc356f`。它保留当前P的34,090个#307证据成员、5个retention Go成员以及当前R规范的Git blob；CI预览同时保留旧retention测试和新增successor测试。唯一冲突为 `examples/lifecycle-d1/prepare-six-candidate.py`。此树包含未解决冲突，**不是有效发布树或提交**。本轮未创建整合提交、未移动任何分支，也未改变tracked工作区。

## 3. 推荐确认的整合路线

1. 保留be38作为不可变的已审修复来源，原74项回归/独评/现场非删除结论仅解释该隔离来源。
2. 以当前e549为发布基线，本地生成同时继承e549与be38的新P；明确解决上述单文件冲突。保留当前candidate23/R ecf规范、retention能力和全部已发布证据，接入已审的v2后继依赖门禁。
3. 对整合后的实际字节运行组合回归、当前全部宿主CI配方及独立评审。原be38评审不能冒充整合评审。
4. 固定新P的完整40位SHA、树、源码/CI清单和评审证据后，再呈现新的精确受限发布确认文本。现在不生成能被视为发布授权的文本。

这次可确认的是上述来源/基线/保留边界与整合路线。真正的解锁/推送需要新P确定后的新的人类直接授权。

## 4. 新P完成并另获授权后，才允许的发布步骤

1. 新O_EXCL追加目录保存真实授权原文和绑定。实时核对远端仍为e549、同一ruleID/精确pattern/完整保护、PR Draft/open/unmerged、Issue OPEN，以及原执行终态/封存/调度关闭和必要依赖。复用已审的scoped successor门禁，不重新绑定无关会话历史PID；不启动源、不分配候选资源。
2. 原生GraphQL `updateBranchProtectionRule` 的input只发送 `branchProtectionRuleId=BPR_kwDOUdQLBs4FE_7b` 和 `lockBranch=false`。读回同一HEAD、ruleID和完整保护，非lock字段必须逐项保持。已通过当前GitHub输入schema只读核验；不发送REST全量PUT，不创建或删除保护规则。
3. 仅一次普通快进推送：`git -c core.longpaths=true push https://github.com/qq550723504/data-product-platform.git <EXACT_NEW_P>:refs/heads/codex/issue307-post-v22-dispatch-closure`。新P必须同时包含e549和be38历史；不使用force/force-with-lease，不Update branch、merge main、rebase、换ref或擅自改P。
4. 确认远端恰为已授权新P后，立即只将同一rule的 `lockBranch=true` 恢复；在锁定状态下等待该P的CI。相较等待CI期间一直解锁，这个建议步骤把可写范围限制到单次推送。
5. 精确新P全部必要PR CI成功后，再次读取同一HEAD、同一规则、完整保护、评审线程及Issue/PR状态。锁后收据仅追加保存，不为提交收据而推进HEAD。本次只产生发布锁收据，不生成候选execution-binding、删除授权或调度。

## 5. 精确HEAD CI验收条件

- 复用 `execution_freeze.ready(snapshot, NEW_P, require_frozen=True)` 的来源检查：固定仓库、head_repository、PR332、workflow `.github/workflows/ci.yml`、event=pull_request，以及完整新P SHA。
- 只接受该HEAD最新匹配run及其实际attempt；状态completed、结论success。GitHub Actions app15368/slug、suite、job/check-run IDs和name必须一一完整对应，所有分页完整。当前作业图13项；整合不得删减必需作业来绕过门禁。
- 不能拿e549的绿色、旧attempt、push/main CI、取消/跳过结果或单独名为required的外来check代替。已有be38本地74项、CI配方43项也不是整合P的远端CI。
- 任何有效安全/历史一致性阻断必须闭合；不因线程outdated而忽略。当前未解决线程0只是基线事实。
- CI通过只用于接纳本代发布，不启动源、不准备新候选、不授予保护状态写入或对象删除。

## 6. 失败收口与明确限制

解锁前任何漂移即停止，保持锁定。解锁后发生API、push、CI或磁盘失败，立即停止新增推送，保留真实原始intent/response/UNKNOWN，并在能新鲜证明同一规则身份时只恢复lockBranch=true。推送结果UNKNOWN先只读确认远端；不自动重发、不force/reset/回滚分支、不以另一个P补发。规则未知或上锁未确认须报告UNCONFIRMED，交保护owner处理，不报告整轮PASS。CI失败后保持锁定；修复P需另立精确方案及新授权。

GitHub保护更新没有expected-HEAD/expected-policy原子CAS；单写保护owner、预读和读后复核是前提，不能把预读当服务端CAS。任何未列出的保护字段均不发送且完整读回比较。

本轮实际GitHub变更0、推送0、PR合并0；源启动、provider调用、业务对象DELETE、retention/hold/IAM写入、资源清理、新候选/execution/预算/窗口/调度均0。必要数据、全部新旧预算、pins、失败/UNKNOWN/成本证据和旧R保持。#332保持Draft/unmerged，#307 OPEN。

## 7. 证据与当前结论

- 完整机器方案：`evidence/issue307-d1/publication-be38-plan01/publication-proposal01.json`。
- 当前保护、CI、线程及原始只读API：`github-read01/`；原生input schema：`schema-read01/`。
- 祖先/树/R差异：`publication-ancestry01.json`；单冲突预览及保全比较：`integration-tree-preview01.*`、`integration-preservation-preview01.json`。
- 方案一致性和7项离线门禁成功/反例：`plan-audit01.json`。它只表示方案核对通过，**实际发布仍BLOCKED**，不冒称新独立整合评审或新HEAD CI已通过。
- 本轮证据另由 `evidence-index01.json` 钉住；原be38提交不变，本方案和回执只追加为未跟踪材料，不提交进be38来改变所选来源锚点。
