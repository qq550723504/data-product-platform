# #307 COPY / RESTORE UNKNOWN worker 恢复受控验收

本切片通过受控合成对象验证了原生 COPY 响应丢失后的只读恢复、worker 中断恢复、outbox 修复，以及 RESTORE 当前独立账本门禁。三个原始 COPY 各发送一次，原始 UNKNOWN 观察保持原样。最终七个 exact versions 全部保留，Core 记录的 70 次实际存储调用各有独立成本分配。**本次 native DELETE、解除保护、bypass、对象清理均为 0。**

最终本地代码候选：`4868774a97e55651beba2a92d156c69ca655afce`。原生运行时和封存后守卫修正的证据范围分别如下，不把本地候选称为已合并或已发布。

这是本地、未发布的 D1 候选验收。Issue #307 仍 OPEN；生产 reader、Rights/Certification delivery 和完整 D1 业务验收为 NOT_RUN。没有 push、PR、合并或部署。

## 基线、迁移与源码绑定

- C1/#310 exact candidate：`f114e506527b7097405acb42a94a59e8b018a900`。
- 已合并的实际 main：`1892cd76c4ee578896ddfb3c6f4a7252bfacc51b`。匹配该 SHA 的 [main push CI 38147297060](https://github.com/qq550723504/data-product-platform/actions/runs/38147297060) 为 SUCCESS；这只证明基线 main，不能作为本 D1 候选的 CI。
- 本地重编号提交 `269c25f1b42e50b24cd17fe079dfdf0819d72b9c`；候选合入提交 `45d55dd6dff18fac1fc67af5237932906b4808d2`；实际 main 合入提交 `6961fde137046f6eb335e9924ba00bc56a7361ff`。

| 已发布基线 / 未发布 D1 | 当前文件 |
| --- | --- |
| C1 / 59 | `000059_quality_remediation` |
| D1 / 60 | `000060_lifecycle_fixture_plan` |
| D1 / 61 | `000061_lifecycle_scoped_completion` |
| D1 / 62 | `000062_lifecycle_denied_reconciliation` |
| D1 / 63 | `000063_lifecycle_archive_restore` |
| D1 / 64 | `000064_lifecycle_reconciliation_work` |

60–63 仅顺延编号；64 扩展现有 DELETE recovery work 为强类型 COPY subject、独立 discovery 和 copy decision 关联。60 的 CostAllocation 单一主体/workspace guards 保持完整。迁移数据库 `dpp-lifecycle307-migration-d05b650f` 及 named volume 保留，未连接任何 native object environment。

[迁移与测试证明](migration-and-tests.json)记录了六个独立保留数据库：原 main/58 → C1/59 → D1/64、初始 fresh/64、实际 main/59 → 最终 COPY/64、对应 fresh/64，以及封存后补充守卫的 main/59 upgrade 与 fresh install。每库记录数为 61、最高版本 64；历史迁移编号有空缺，不能表述为连续安装 1–64。59–64 的 exact names 与最终 SQL hashes 均已读回。

原生运行时 HEAD 为 `6961fde...` 加当时未提交的工作区源码，**通过逐 helper 的 source/binary/SQL SHA256 绑定**，没有冒充该 HEAD 的纯净二进制。环境及 helper provenance 文件保留初始值；旧数据库与原生程序没有被“更新”为最终候选。

原生 schema64 的 up SQL hash 为 `259ceb5ab945c38d36ccf00f2b362dbd1dcf7f1dad6f64d6f3bd935b74645e0f`。封存后另加了 `discovery_id` 的强关联守卫，防止成功 ACK 掩盖其他 operation 的 discovery；该最终 SQL 在最后两个新数据库和真实 PostgreSQL 回归中通过，未应用到已验收库。路由版本也缩短为 `c1-v26+ext1+exec1+usage1+rem1+life2`，两个 profile（含 `+governance`）均通过 varchar(64) 合约测试；已有事件的冻结路由义务未改写。

## 独立封闭环境

| 范围 | 实际 identity |
| --- | --- |
| Project | `dpp-lifecycle307-f7fa46ff` |
| Run | `f7fa46ff-3a93-4e50-97a5-0d68fac8b235` |
| Workspace | `98a94425-6526-421b-a791-83db9347e83c` |
| Provider instance | `73f3fc19-803b-42fb-8650-c73cda6ee834` |
| Provider incarnation | `44936d04-0559-41ea-b6f2-61ffd02123df` |
| Internal network | `4265eef0bf80c7dca4ee1e978ec09c5fc2ad98178f9a12a1bd76099c64bc40ff` |
| Host ports | 无 |
| Schema | 当时接受的 C1/59 + D1/60–64 |
| Volumes | `dpp-lifecycle307-f7fa46ff-postgres`、`dpp-lifecycle307-f7fa46ff-objects`、`dpp-lifecycle307-f7fa46ff-redis-data`，全部保留 |

固定实际镜像与原生对象来源见 [manifest](manifest.json) 和 [环境清单](copy-recovery-environment.json)。复用仓库已有 MinIO SDK、Asynq、PostgreSQL advisory lock、worker scanner、outbox、Journal/CostAllocation；没有新增队列或 workflow runtime。Worker 的 Port/原生 identity 只允许受控只读盘点、exact HEAD/protection 和内部 checksum；没有 CreateCopy、DELETE、解除 hold 或 bypass 能力。独立 witness 在 Core/provider/Redis volumes 之外，worker 只读挂载；独立显式 export command 写入 witness。

初始 producer binary SHA256：`36d3efc94a4c8b305f86a34a4b0c4f9c827720f8a39b96c688389631c73edcb9`。第二目标修正 fixture idempotency key 后的新 producer binary SHA256：`6ac3fac9688df2bc6909602cedd0b127e59ed60959de067453ee1ae7b65e55b5`。Worker binary SHA256：`6bc11d66bb3f64f3ae07a1c3b508350b3c577ab3437d1436e4d6ae6bee576154`。旧二进制及失败 helper 全部保留。

## 实际验收结果

| 场景 | 结果 | 实际证据 |
| --- | --- | --- |
| 原生 ARCHIVE response loss | PASS：native COPY 一次；client 丢弃响应，producer exit 93，原观察 TRANSPORT/0/no version | [producer](copy-producer-ARCHIVE.json)、[最终原始调用](sealed-copy-recovery-final.json) |
| Worker 在 durable inventory + exact HEAD 后退出 | PASS：exit 92；没有 bind；既有 Asynq orphan recovery 随后恢复为 ARCHIVED，源保留 | [中断屏障](copy-crash-after-lookup-barrier.json)、[退出](dpp-lifecycle307-f7fa46ff-copy-crash-after-lookup-91608d-exit.json)、[worker](copy-worker-after-crash.json) |
| Broker 暂停与 outbox 修复 | PASS：FAILED/0 confirmations → Redis 原 volume 启动 → PUBLISHED/1 confirmation | [不可用](copy-outbox-broker-unavailable.json)、[修复](copy-outbox-repaired.json) |
| RESTORE 无独立账本 | PASS：原始 native COPY 一次；恢复为 UNKNOWN / INDEPENDENT_LEDGER_UNAVAILABLE | [36 调用快照](after-archive-and-restore-unknown.json) |
| RESTORE 新独立 witness + 新恢复请求 | PASS：append 新决定 ONLINE / CURRENT_LEDGER_AND_POLICY_ALLOWED；未重发 COPY | [witness 1](copy-witness-1.json)、[40 调用快照](after-fresh-restore-online.json) |
| 完成 work 的重复 replay | PASS：物理调用及成本 40 → 40 | [重复快照](after-duplicate-terminal-replay.json) |
| 新限制与旧 witness | PASS：第二 RESTORE 先为 UNKNOWN / LEDGER_GAP_STALE_OR_CONTEXT_MISMATCH | [最终 append-only decisions](sealed-copy-recovery-final.json) |
| 新限制与当前 witness | PASS：新请求 append BLOCKED / CURRENT_USE_OR_RESTORE_PROHIBITED | [witness 2](copy-witness-2.json)、[56 调用快照](after-current-restriction-blocked.json) |
| 撤销 worker 窗口 | PASS：新请求 BLOCKED_AUTHORITY，调用及成本 56 → 56 | [撤销快照](after-recovery-window-revocation.json) |
| 最终原生版本/保护/完整盘点 | PASS：两个 namespace 七版本、无 delete marker；四个原始 exact versions 与 protection 无变化 | [原生最终读回](copy-originals-final.json)、[独立核对](independent-evidence-validation.json) |
| 成本与原始 UNKNOWN | PASS：70 calls/observations/known invocations/独立 typed allocations；三个原始 observation 未覆盖 | [最终 Core 读回](sealed-copy-recovery-final.json) |
| 新 native DELETE / cleanup | NOT_RUN / 0 attempts / 0 absence receipts | [最终 Core 读回](sealed-copy-recovery-final.json) |

原始 physical call IDs 分别为 ARCHIVE `d899585b-ae0b-4ed9-bce8-099fc546da2a`、RESTORE online `d87faccb-72da-4bd4-aa86-8702d9c18e19`、RESTORE old-cut `fa2a8bba-0165-477e-9991-b06c8e6ec7f0`。每个原 observation 保持 HTTPStatus=0、failure=TRANSPORT、copy_result_version=NULL、一个 physical invocation CostAllocation。后续 recovered target 是独立不可变 fact，没有伪造原 COPY ACK。

ONLINE 是当时 fixture admission 的历史事实。随后显式 ContentUseRestriction 对该 alpha DatasetVersion 生效，**当前 use 已 BLOCKED**；先前 ONLINE 历史保留，不能据此对用户声称现在可取数。

原生 missing/ambiguous/truncated/wrong-tag 的全部组合没有各自再建 live provider 场景；这些 fail-closed 边界由 Adapter contract 与真实 PostgreSQL tests 覆盖。不能把 SQL-only fault inputs 说成原生验收。

## 最终 exact version 保留清单

以下全部保留。清单用于核对，无删除批准或删除计划。

| Namespace | Key | Exact version | 原生保护 / 结果 |
| --- | --- | --- | --- |
| `dpp-lifecycle307-online` | `fixture/a.csv` | `67c92bd9-3ed2-4a72-aa51-b73b0a518417` | 原始无保护源；保留 |
| `dpp-lifecycle307-online` | `fixture/b.csv` | `1403204c-2dfe-4dc2-8077-17e2f1e06760` | LEGAL HOLD ON，保持原样 |
| `dpp-lifecycle307-quarantine` | `fixture/a.csv` | `a84dceb0-5475-4359-89cc-0881f65483d3` | GOVERNANCE 到 `2026-10-12T14:39:22Z`，保持原样 |
| `dpp-lifecycle307-quarantine` | `fixture/b.csv` | `b6ba9bc5-17fa-4a81-98af-ebefee25f3d2` | 原始无保护对象；保留 |
| `dpp-lifecycle307-quarantine` | `archive/a.csv` | `8c0c9716-fe02-47fa-b1f6-b0dd1db43df5` | verified ARCHIVED，源保留 |
| `dpp-lifecycle307-quarantine` | `restore/online-a.csv` | `92f990a0-f1b5-468e-921d-c5543fbec821` | 历史 ONLINE；当前 use 限制生效 |
| `dpp-lifecycle307-quarantine` | `restore/old-cut-a.csv` | `6a64eeb9-97b4-446d-ae04-365498367e01` | 当前 fresh admission BLOCKED |

所有 A 内容为 27 bytes、SHA256 `818392d0732d499a777b5b5a74f1351ddffce3cf8a3b66f8d91dbfd98dfa1523`；两个 B 为 26 bytes、SHA256 `aafd9ae024f2444eefea042c59ffd87c29fac3f4675c49124785b417da34b784`。Core identities、原始四对象 exact protection、完整 native inventory call IDs/hash 见最终读回。

## 测试、封存与保留边界

真实新 PostgreSQL 上 scoped lifecycle/storage/routing/fixture suite PASS；应用 tests 耗时 297.786 秒。全模块 `go test ./... -timeout 8m` PASS（85 个 ok packages、44 个无测试 packages）；此广域检查没有 TEST_POSTGRES_DSN，真实数据库验证来自单独 scoped suite。最终 discovery scope guard 的相关 COPY/RESTORE 真实 PostgreSQL 回归再次 PASS（32.613 秒），两个 routing profiles 测试 PASS。Race 本次 NOT_RUN。

[封存快照](preservation-and-seal.json)：四份有限窗口均 enabled=false；COPY producer 与只读 recovery 原生 identities 经各自 SetUserStatus 响应成功禁用；gateway 恢复所有 DELETE/COPY/POST/admin/payload DENY。用于原始 metadata 核对的 inspection identity 没有 payload/mutation 权限，应用窗口同样关闭。IAM 创建/绑定/禁用为八次原生 START + 八次 200 observation，私有 bootstrap receipts 保留；它们是 fixture 基础设施操作，不冒充 Core 的 70 次业务存储调用。

本次前置快照里的 **17 个旧 container 均保留，image/mount/state 无变化；11 个旧 owned named volumes 均保留**。新增四个 owned volumes（迁移 PG 一个、native fixture 三个）保留，合计 15 个。旧原生对象本轮没有重新读回，标为 NOT_RUN；没有命令访问旧 native environments，也没有在旧 59–63 库上安装重编号迁移。全局其他线程的 volumes retention 没有断言。此前环境的历史报告和已消费删除确认保持原样。

提交前[再次核对](final-container-preservation.json)：旧17个container、旧11个owned volumes以及本次31个具有provenance的helper均存在，旧container未变化。[公开证据凭证扫描](final-evidence-hygiene.json)与四个脚本parser检查通过。

执行中保留的失败尝试：PowerShell 参数/mount 组装错误在 helper/native I/O 前被拒绝；初次 IAM receipt 文件只读导致两个 helper 在写 START 前退出（0 native calls），随后只给 receipt 文件单独可写挂载，凭证仍只读；Redis 暂停后成员数量预检先拒绝，增加 exact broker-fault 范围后才执行；第二 RESTORE 的 fixture key 曾只包含 action，命中旧命令而在 COPY 前被拒绝，两次已发生 protection 调用仍入账，改为 action+target 后使用新保留 binary 发送一次 COPY。没有 provider 自动重发、删除或失败补偿清理。

新增删除实跑仍需先提交环境与 exact key/version 清单并获得用户确认。本次没有提出或执行新删除。
