# #307 D1：发布后执行绑定验收矩阵

本轮范围仅为发布后来源门禁修复、非删除回归及独立评审。#307 保持 OPEN。不推送，不创建候选、业务 execution、预算或窗口，不启动源环境、写保护或删除。数据、旧预算、pins、失败/UNKNOWN及成本事实不修改；旧授权不复用。

权威设计：[发布后执行绑定契约](issue307-d1-postmerge-binding-contract01.md)。本矩阵记录验证事实，不授予操作权。

| 项目 | 结果及证据 |
| --- | --- |
| 已发布 P / 实际 squash M | P `e5a547b4601f39740256fa018d3877a6276ad1cf`；M `e0e4f77a576ea4572cf16b3a3475461127f44acc`；GitHub GET 与本地完整 Git tree 交叉核对通过 |
| tree / sole parent | `9a0ae2bb488dcf716a40f168b908983388f84f2b` / `f7a90022aad5da4d67ea594ccb86f8b9925a3840`；P/M tree 相同，正确接受 squash，不假称 P 是 M 的 ancestor |
| 精确 M 的 main CI | `40148063796` 13/13、`40148063765` 1/1；均 attempt 1、`push/main`、同仓库/精确 M；全部 job/check/app/suite 与分页通过，未使用 PR CI 替代 |
| 独立保护锚 | 原分支 head P、同一 `BPR_kwDOUdQLBs4FE_7b` 及完整原生保护相同；锁定/admin/no-force/no-delete/no-fork 继续有效；没有解锁或写保护 |
| 保留 R4 | R `0446fde5985b51de7da5863b45df3f1b47b6167c`；parent `53f6f8a6e69c17fa6896a40293d323f6ee9d0958`；ZIP `a60ddfeb8a81d4414bc56e1ddd05fa01ca7f2e0e9579a7d403783bdd318d115a`，6,515,425 bytes / 781 文件；直接父/delta/shared/schema69/66/完整成员均通过。未修改 R4 或运行源 migration |
| 新显式来源路径 | `D1_MERGED_RELEASE_SOURCE_V1` 贯通 preallocation、原 CI 收据/snapshot、12 构建成员、source-freeze、完整授权入口；旧 Draft/open 门禁未放宽，旧 kind 不会自动升级 |
| 未发布源码隔离 | 当前本地补丁借用旧 M 的来源证明被拒绝；HEAD、tracked/untracked、raw/LF、source-ref API 前后重核。不可使用旧 main 绿色接受本补丁 |
| 维护读写同源 | V3 的独立来源 SHA 通过域分离 BudgetID → BindingsSHA 同时约束 read/write；完整签名成功组合通过。旧 write、新 read 混用及来源/预算漂移均拒绝，不修改 R4 Go 格式或重置预算 |
| 非删除 Windows / Linux | 两种独立 CPython 3.13.16 各 27 套、54 条 normal/-O 命令，有效结果全部 PASS；每种系统 207 个方法，414 次方法执行；保留路径读取审计拒绝数均为 0 |
| 完整成功与反例 | 临时真实 Git squash、P/R/ZIP、冻结链、完整维护模型与确认文本成功；wrong merge/tree/parent/P/R/ZIP、dirty/raw/untracked、旧/foreign/failed/pending/skip CI、不全分页/job/check、保护漂移、未知 kind、terminal、旧授权、预算及读写混用反例通过 |
| 首次失败保留 | 新增冻结链测试曾重复创建模型中的同名 O_EXCL 文件，两系统各 2 命令失败；改用另一个临时输出目录，仅复测受影响命令通过，未放宽生产门禁。旧 52 条命令每系统均首次通过。原完整失败日志及输入快照保留 |
| 测试 helper | 仅两个独立、无网络、无 retained/source 挂载的测试 helper；失败 exit 1 与成功 exit 0 分别按真实状态归档后移除；volume/image/network 删除均 0 |
| 独立源码终审 | `PASS_BOUNDED_SOURCE_NONDELETE_REVIEW`，`remaining_blockers=[]`；核全部 10 个源码/测试/契约 SHA 及两份结果摘要。评审代理未改文件、未重跑测试或接触运行环境；未声称独立逐项审计完整原件索引 |

证据位于 [postmerge-binding-repair01](evidence/issue307-d1/postmerge-binding-repair01/evidence-index.json)。GitHub 原始读回、独立测试输入/输出、失败和 helper 归档保存在私有 `release-binding-repair01`，公开摘要记录其 SHA。源码终审结论见同目录独立报告。

当前剩余条件严格分开：

1. **发布条件，NOT_RUN：** 本修复尚未推送或合入；其新精确 HEAD PR CI、未来实际 merge/tree 及对应 main CI 尚未运行。基线 M 的成功只证明原发布，不证明本补丁已发布。
2. **真实延期，NOT_AUTHORIZED / NOT_RUN：** 两份保留版本的旧期限过期事实继续保留；本轮未启动源、未刷新对象状态、未写延期。未来仍须新的精确对象/期限/owner/预算、原生前后对账与独立操作授权。
3. **剩余六份删除，NOT_AUTHORIZED / NOT_RUN：** 无新候选、execution、独立预算、窗口、删除或调度授权。P1 永久排除，11 survivors 保留；旧 candidate23 及更早终态/已用预算不可恢复或重发。未来全部真实前置、保护、发送次数、UNKNOWN、封存门禁继续适用。

因此本轮只将发布后绑定的代码缺口收敛为“本地实现/非删除验收已完成，待受限发布”，不将完整 D1 提前标记通过。
