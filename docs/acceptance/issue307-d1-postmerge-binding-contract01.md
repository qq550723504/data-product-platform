# #307 D1：发布后执行绑定的有限修复

基线是已合并 `e0e4f77a576ea4572cf16b3a3475461127f44acc`。本轮只做本地源码、非删除回归和独立评审；不推送、不创建候选、MaintenanceID、业务execution、预算或窗口，不启动源环境、写保护或删除。#307 OPEN，旧数据、预算、pins及失败证据保留。

根因是把“开发PR尚为Draft/open”当成长期执行来源证明。旧 `execution_freeze.ready` 仍严格用于原路径；它不接受已合并PR。本轮增加显式 `D1_MERGED_RELEASE_SOURCE_V1` 来源，不伪造Draft快照或自动fallback。

三个对象分开：

1. `release_source`固定同仓库的已合并PR、review head P、实际merge M、base及完整tree；M/P tree必须相同。squash的parent是main base，不能要求P是M的ancestor。核最新真实`push/main`两工作流、所有job/check/app/suite/attempt及完整分页，拒绝旧绿色、PR CI、失败/skip/foreign结果。
2. `protection_anchor`是既有BRANCH的独立操作fence：固定anchor head、rule及完整原生保护，仍要求锁定/admin enforced/no force/no delete/no fork sync。anchor head不冒充新P或M，不要求源PR一直open；任何漂移继续拒绝，不写保护。
3. 操作包/新授权固定来源摘要与原scope/window/binary/config/owner/budget及终态。来源PASS不是操作授权，旧execution及旧授权不恢复。

实际执行代码必须从已批准M运行，`host.source=M`，HOST_PATHS成员与raw/LF字节完全一致，拒绝dirty/untracked/别的commit，先于GitHub/history/build/Docker。当前未发布补丁不能搭配旧e0e4证明执行。

复用`fixture_artifacts`验证P源码、保留R的Git直接父/固定delta/schema69/66/shared contract、Git UTC store ZIP原字节与全部成员。R4不修改、不迁移源Core；二进制/12构建成员及候选source-freeze仍由原门禁验证。新增kind贯通preallocation、build receipt、source-freeze和authorized_gate，Host/H1/attester/完整入口继续走相同授权入口。

维护V3包显式引用release source；V2包不改变。读/写不能混用不同来源：V3复用已有BudgetID为域分离的`SHA256(tag,MaintenanceID,release-source SHA)`，因此现有BindingsSHA及两份签名授权共同约束同一来源；不改变严格R4 Go字段、不产生自引用。read ManifestSHA另绑定整个包。换来源、旧write授权或重新hash不一致输入均拒绝。

非目标：真实延期/17版本源读、六份删除、候选准备、默认运行key、保护变更、源schema升级、重开#332或新的通用工作流/策略引擎。复用Git/GitHub、现有O_EXCL证据、制品校验、签名和预算，不新增依赖。

验收包括完整成功与wrong merge/tree/P/R/ZIP/host bytes、foreign/旧/pending/failed CI、不完整成员、保护漂移、旧kind/授权、terminal、读写来源混用及任何I/O前拒绝。Windows/Linux Python3.13 normal/-O分别报告；纯模型与GitHub只读来源证明均不冒充源环境验收。新代码尚未发布/其精确HEAD CI未运行须作为交付条件保留，不能拿基线绿色替代。
