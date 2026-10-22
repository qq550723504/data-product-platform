# #307 D1 execution 与 PR 分支冻结边界

## 有限 merge contract

Goal：修复 v29 在授权后被合入 main、直到窗口才发现 PR HEAD 漂移的问题。保留 exact PR HEAD / runtime source / candidate / binary / Core / IAM / gateway / budget 授权约束，增加原生分支写保护与实际入口核验。

权威：本文负责执行绑定与分支更新边界；既有有限窗口手册负责发送顺序与物理封存；GitHub 原生 BranchProtectionRule 负责阻止分支写入。v29 的冻结文档、授权、terminal 与失败证据不可回写。Core、Outbox/Asynq、sender budget 和 pins 不新增平行机制。

In scope：最终提交→精确 HEAD CI/评审→原生锁定→锁后重新核对→新的人类授权/单独调度→执行前及签名时 fresh 核验→终态封存→显式受控解锁。任何失败保持停止与锁定，不能自动回退或复用旧授权。

Non-goals：修改历史 candidate19/v29、恢复其授权、执行对象删除、自动选择下一窗口、通用仓库 policy engine、绕过管理员信任边界、自动解锁或改写既有分支保护。解锁是可信保护 owner 的显式原生操作，须先取得本 execution terminal 与物理封存证明、停止调度，并确认 HEAD/ruleID/完整保护未漂移；不能放进 finally，也不能删除原有保护。

## 执行绑定

同一 execution 必须区分三类事实：

1. runtime source、source-freeze、原构建 binary、candidate/template/bindings 为不可变执行制品；
2. review HEAD 为最终 PR commit，必须有该精确 HEAD 的必要 pull_request CI；不等同 main CI，也不由旧构建 source CI 替代；
3. GitHub 原生分支锁及锁后收据为外部维护边界，不授予对象删除权限。授权仍来自新的人类精确确认。

新候选所有源码、非删除准备、候选、文档、评审索引及最终静态计划必须先提交并取得 CI，再锁该最终 PR HEAD。锁后收据、授权、调度、窗口与 terminal 回执只能在追加目录保存，不能为了提交这些回执而推进锁定 HEAD，不能在包含自身 commit 的 tracked manifest 里构造哈希环。

## 分支更新边界

| 阶段 | 允许的分支操作 |
| --- | --- |
| 准备/开发，尚未冻结 | 可以正常提交、同步 main；更新后重跑对应评审/CI |
| 原生锁定后至终态闭合 | 不允许 push、Update branch、merge main、rebase、force push、删除分支或 fork sync |
| 窗口失败/UNKNOWN | 停止新增发送并物理封存；旧预算/pins/成本/证据保留；锁不自动解除 |
| 终态与封存后 | 可信保护 owner 显式确认并解锁本工具实际拥有的冻结位后，才允许更新；原授权不能因此恢复 |

复用 GitHub 原生精确 BranchProtectionRule：lockBranch=true、isAdminEnforced=true、lockAllowsFetchAndMerge=false、allowsForcePushes=false、allowsDeletions=false。现有精确 rule 用 GraphQL 按 ruleID 只修改这五个字段，其余字段不发送并完整读回比较；不使用可能清空原保护的 REST 全量 PUT。不修改 wildcard/shared rule，不以新的 exact rule 覆盖它，不采用未知或不可读取的保护状态。

GitHub API 没有 expectedHead/expectedProtection 的原子 CAS。实现只能在预读后锁定，再重新读取 HEAD、规则、完整保护及 CI；不得将 clientMutationId 或预读比较描述为服务端 CAS。锁后有任何漂移即停，保留 intent/原始响应，保持锁定。管理员仍有修改保护的能力，可信保护 owner 单写是明确前提；每次发送 proof 之前继续 fresh 核对 HEAD/规则/保护。

## Build-vs-Buy / Reuse

已有候选 SHA、宿主拓扑签名、固定完整入口、O_EXCL 回执和 GitHub CLI 足以承载编排与留证；缺口是 GitHub 分支本身没有写保护。本次采用 GitHub 原生 lock，不自建 Git lease 服务、队列、policy engine 或工作流运行时。Git native refs 提供内容寻址但单凭 tag/本地 detached HEAD 不能阻止 PR 分支推进，因此不能单独解决本故障。GitHub rule 的管理归属固定在现有仓库/#332；worker 不获得 GitHub token。

## 验收边界

必须覆盖完整成功路径及反例：foreign execution/head/repository/PR、缺失或已终态绑定、解锁/管理员绕过/fork sync/force/delete 位、完整旧保护变化、wildcard/shared rule、锁前后 HEAD 漂移、CI 未完成或失败、API 403/404/超时/不确定结果、重复冻结、写回执失败，以及固定完整入口/Host.check/attester 在任何 Docker/Source/签名动作之前拒绝。正常和 -O 均验证，不能用 assert 实现安全门禁。

原生非删除验收使用独立 GitHub rehearsal 分支，保留锁定分支和回执，证明实际锁会拒绝更新；不操作 #307 的对象、凭据、预算或 pins。最终独立评审只关闭当前有限 contract 的有效阻断，后续新候选仍须重新冻结、核对、授权。

官方 API：<https://docs.github.com/en/graphql/reference/branches#updatebranchprotectionruleinput>；<https://docs.github.com/en/rest/branches/branch-protection#update-branch-protection>。
