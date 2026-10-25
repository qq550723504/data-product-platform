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

### 两个源码制品 R / P

保留环境的 fixture runtime R 与 PR/宿主 P 是两个明确制品。R 从已验证的 fixture commit 派生，只包含新 execution/window、已保留 IAM 事实及固定 schema 门禁；`fixture-runtime-artifact.json` 固定 parent、完整 delta、Git archive 原字节/成员和共享 scope/policy。构建只能在该归档的独立目录中运行现有 Go，GOWORK=off，不使用 P checkout 构建后宣称属于 R。R 仅支持三组既有 provider-version fixture 与 schema max=69 / 66 migration members / 无 identity_kind；不能运行当前 protected Core，不提供 schema fallback。

P 保留当前产品模型及 identity_kind 保护。宿主构造、完整入口、拓扑签名、分支锁工具和共享 scope/policy 在另一个清单中冻结。P 的宿主源码 commit、原字节/LF SHA 与 R 的源码、归档、四份 binary、12 个原构建成员分别留证。source-freeze v2 把两者整体绑定进候选；完整入口先验证 R/P 共享契约、归档、宿主源码及冻结交叉字段，再读取 Docker/source。最终 review HEAD 仍需精确 CI 与原生分支锁，不以 R 或早期 P 的 CI 代替。

归档统一使用原生 Git `TZ=UTC`、`core.autocrlf=false`、ZIP store-only `-0`，并逐字节固定 ZIP SHA 与完整成员 SHA。ZIP 本地时间及压缩器实现不能成为操作系统间的隐含输入；不以“源码成员相同”豁免执行归档的原字节核验。原 v30 本地时区/压缩归档与失败回执保留，不更新成新格式冒充原始构建。

任何下一代分配之前，必须先有构建所用 P 精确 HEAD 的全部必要 pull_request CI 成功，复用相同的仓库/PR/workflow/app/suite/job 成员来源核验；API 未知、pending、失败或外来 HEAD 在 history/build/任何资源分配前停止。历史核验与 Go 构建之后、第一项 network/volume/owner 创建之前，实际 build 的 P source 必须仍等于该 CI HEAD，且完整宿主 raw/LF 字节仍匹配。新 allocation 与 source-freeze 保存原 CI 回执 SHA/快照；最终文档 HEAD 的 CI 与原生锁仍另行验证。未冻结准备也可以终止：追加 preparation-terminal，保留原 allocation/build/预算和撤销前 profile，固定入口不得恢复或重新使用该 execution。

main 更新带来的新产品 schema 不能隐式改变 R 的执行语义，也不能据此迁移保留源 Core。三源升级若另有业务必要，应使用单独的人类维护授权；本次已保留其隔离克隆证明和未授权备选提案，没有执行源升级。R/P 任一必需契约变化必须在冻结前完成回归、独评与新制品；冻结后不得更新分支来补交回执。

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
