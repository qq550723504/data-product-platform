# #307 归档/恢复有限切片

2026-10-11；有限合成归档/恢复已实跑 PASS，全 D1 与生产灾备 NOT_RUN。沿用 Lifecycle §2/6/7/9 与 D1 scoped LC-03/18/19/20；本聊天唯一 writer，复用既有 branch/worktree、MinIO SDK、workspace fence、物理调用 Journal、Cost/Audit/Evidence/Outbox。main253ce3b4/57，C1保留58，D1已有59–61，本片新增62；发布前按实际 main 重查。

本片使用新的独立 internal network/三服务/专属 volumes。原 `dpp-lifecycle307-a6b30840`、两份保护版本、原执行证据与 volumes 保持原样。仅新环境的合成无保护 online/A 进入复制归档；native CopyObject 产生新的 exact version，源副本保留。恢复先进入 quarantine，验证 exact bytes/hash/长度及无弱化保护，当前 finite custody authority 与独立台账复核后才能追加 ONLINE availability。DatasetVersion identity/hash/原URI/status不改写，纯复制不创建新DatasetVersion/认证。

Pending target intent 在 I/O 前提交且固定 source、target namespace/key、action、有限期限；一个 intent 只准一次 COPY send，响应未知保留 intent，不自动重发或 cleanup。每个 copy/verify/metadata 请求有独立 committed start/append observation/实际调用成本；复用现有 storage-call Journal及typed allocation，失败/未知不覆盖历史。

独立台账使用本 fixture 的私有外置、fsync追加 checkpoint链（不在 Core/provider backup volume中），checkpoint冻结 workspace/subject/epoch/连续sequence/current fence revision/state hash/期限。Core另存对应 immutable checkpoint。台账只由 server publisher导出当前 typed policy/restriction/disposition/hold facts；Restore gate在入事务前读取独立latest chain，持共享fence重算Core当前state并核对latest checkpoint/revision/hash。缺失/断链/错tenant/旧Core projection/落后checkpoint/过期均拒绝ONLINE；checkpoint提交后未成功导出时也拒绝。这是D1单合成subject的受控witness，不声称实现生产灾备/全仓Rights replay。

必需验收：真实 native archive+restore同字节、源/历史identity保留、待复制/UNKNOWN不可开放或重复发送；真实PG原子facts/immutable receipts/并发claim/late restriction与finalize锁顺序；恢复旧cut后current restriction阻断；缺失/断链/旧watermark/hash错配拒绝；普通入口不能直读quarantine，fixture use另走fresh gate+pin。原两保护对象只读环境核对，不解除/bypass；本片native DELETE=0、cleanup=0。

Reuse/Build-vs-Buy：现有 MinIO原生CopyObject/GET、标准SHA256、已有事务/Outbox/Journal足够；不引入新存储、通用ledger、scheduler、备份平台或加密实现。平台自有的是exact intent/availability与恢复admission语义。AWS S3版本复制/GET、Ceph RGW是标准替换边界，首片不新增第二provider；固定MinIO CE构建/AGPL/维护准入沿既有合成基线，生产维护/替换另议。

Blockers：任意key/version/tenant或credential泄漏、未登记intent先复制、隐式重发、旧台账开放、target hash/保护无法确认、pending copy遗漏、历史/成本覆盖、fence并发穿越、必需测试失败。源删除/复制后cleanup独立授权；实际新增DELETE前必须提交环境与exact key/version清单并获用户明确确认。全仓processing/delivery、生产冷tier/备份介质、自动worker完整恢复继续后续；本片不关闭#307。

实际运行 `dpp-lifecycle307-c9f47cab`：9 个 exact versions 全部保留，1 ARCHIVE + 4 RESTORE native COPY，5 byte verification + 1 finite reader；旧水位 UNKNOWN、当前禁止 BLOCKED、缺失/断链 UNKNOWN，0 active reader。82 starts/observations/CostEvent/typed allocation，195 Domain Event/Audit/Evidence。真实 PG lifecycle/routing/storage race suite、module unit suite 与 vet PASS。执行身份已关闭，native DELETE=0、对象/volumes cleanup=0。实际清单、失败运行留存及验收/来源限制见 [archive review](../../examples/lifecycle-d1/results/c9f47cab-23c6-4d16-8102-a4a34bac1501/archive-review.md)。原环境及两份保护版本未接入本次对象操作。没有 push/PR CI/review/merge/main CI，本片不关闭 D1。
