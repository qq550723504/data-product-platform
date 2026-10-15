# #307 封闭 fixture 准备入口

本入口仅创建并登记四个受控合成版本，读取 exact version 校验 bytes/SHA256，记录 native retention/hold、完整单页版本盘点和逐调用事实。没有删除、清理、approval、dispatch、reader 或 delivery 入口。实际待确认清单见 [当前 run](results/a6b30840-3e76-49ad-91d3-f0c399fcf170/review.md)。

## 复用与隔离

复用已引入的 MinIO SDK / madmin 原生 API、PostgreSQL、StaticResolver、Lifecycle commands、delivery fence、既有 Evidence/Audit/Outbox routing 和 CostEvent/typed allocation。没有新存储 engine、队列、scheduler、IAM 或商业账本。Bootstrap JSONL 是本地测试输入的基础设施创建记录，不是生产 activity 成本账本；平台原生 inspection 的每次 HTTP 调用写真实 Core Journal/CostEvent。

- 新 run 创建唯一 internal Docker network、两个专属 named volumes、PostgreSQL/MinIO/gateway；provider 和数据库均没有可用宿主端口。准备程序复用 gateway network namespace，不能旁路 gateway 访问 MinIO。宿主机不暴露 API、worker、console 或业务 reader。
- Gateway 拒绝所有 DELETE、copy、overwrite、bare-key payload read 和范围外路径，并保留官方 unsigned-trailer mitigation。检查身份的 provider policy 只授予版本盘点及四个固定 key 的 metadata 权限；gateway 仅允许 bootstrap 身份读取已知合成 bytes，inspection 身份不能取 payload。Bootstrap 身份只用于隔离 fixture 创建/复核，不是处置 executor。
- 固定实际 image ID / binary hash / source commit；runtime label 是 `DEVELOPMENT.GOGET`，不把 release tag 当成运行时 label。这个本地准入不等于生产准入。
- 创建时使用 create-only conditional PUT、禁用 multipart。普通 SDK `MaxRetries=1`；bootstrap transport 对每一步只允许一个物理请求，并在发送前持久化 receipt。拒绝隐式重发，取消/失败不触发 cleanup。Receipt 不含 credentials、headers、query 或 bytes。
- MinIO 原生 retention/hold 未设置时返回特定 400；Adapter 只在完整 Error envelope、唯一 Code、正确 Resource/可选 Bucket/Key、固定 status 匹配时映射 NONE/OFF。其他 400、403/404、错误 scope/重复字段仍 fail closed。语义依据固定源码 [GetObjectRetention / GetObjectLegalHold handlers](https://github.com/minio/minio/blob/9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a/cmd/object-handlers.go#L2635) 与 [error contract](https://github.com/minio/minio/blob/9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a/cmd/api-errors.go#L1048)。

## 创建新环境

从仓库根目录执行 `examples/lifecycle-d1/prepare.ps1 -PrivateRoot <checkout 外的私有绝对目录>`。Windows ACL 仅允许当前用户和 SYSTEM，credentials 通过只读文件挂载，不放 Docker argv/env metadata。输出 public manifest/receipts/policy；private 配置、binary、固定 migration snapshot 留在私有目录。成功后禁用 fixture principal，manifest guard 阻止 helper 重启时再次运行。

失败时保留全部对象和历史。`LIFECYCLE_FIXTURE_RESUME_EMPTY=1` 仅能在 registry/database 与 native namespace **均为空**、输出只有 bootstrap receipts 的情况下显式恢复；不是自动重试。`LIFECYCLE_FIXTURE_INSPECT_REGISTERED=1` 只读取已有四个 registry exact versions，不能创建 namespace/policy/identity/version；它仍需当前 enabled authority，已 finalized manifest 拒绝 replay。不要复制、覆盖或移除已登记历史来“重跑成功”。

`LIFECYCLE_FIXTURE_PREPARE_UNPROTECTED_PLAN=1` 是另一个非破坏 command：读取已验证 manifest，先与 immutable registry 比对，只为 online/A + quarantine/B 创建两对象冻结候选。它不访问 storage、不授予 approval/dispatch、不能解除保护，输出 deletion-candidate.json；已有候选拒绝 replay。仍需临时启用当前可信 fixture authority，完成后立即禁用。此次通过独立 `-scope` helper 执行，退出成功、无 named volumes、新 network member 或 provider requests。

准备程序 seeded READY 只表示已验证的合成 content identity。两个 namespace 的同 hash 对象都是输入 fixture，不能作为 archive/restore 成功事实；没有 Rights、Quality、Certification、正常业务 reader、真实交付或完整 D1 验收。

## 本次保留环境

Run `a6b30840-3e76-49ad-91d3-f0c399fcf170` 保留三台运行中的服务、已退出成功的 prepare helper，以及 PostgreSQL/object 两个 named volumes。初次脚本尝试中的空 credentials 目录也未清理；没有清理任何对象、bucket 或 volume。

本次 native 准备冻结在 main `3a59b591d25e7d07fc38956a7ae729d74967215b` + 本地 D1 schema000054；完整 SQL snapshot 私下保留。确认删除时只安装当时开发编号additive58/59，`schema_migration` 仍为54。收口开发分支已 rebase #311/main `253ce3b4...`：main55/57，协调后 C1 未发布58、D1当前59/60/61；**不得把重编号后的 foundation 再安装到这套历史库，或因此重建/擦除本 fixture**。清单保存各次 native 执行时 source/binary/SQL hashes，不改写成新 main 实跑。

准备阶段测试：`go test -race -count=1 ./cmd/lifecycle-fixture-prepare ./internal/platform/storage ./internal/lifecycle/... ./internal/platform/routing ./internal/cost`；native provider +真实 PostgreSQL 成功记录在 run artifacts。准备阶段没有调用 DELETE，包括预期返回拒绝的负例。

## 已确认的两版本删除

用户随后确认本环境及 online/A、quarantine/B 两份无保护 exact versions，已完成 DELETE、完整 native inventory、两份 absence receipts 与精确范围 completion：[受控删除报告](results/a6b30840-3e76-49ad-91d3-f0c399fcf170/delete-review.md)。另外两份保护对象保持原保护，只做 metadata/HEAD/inventory 与领域 prerequisite 阻断核验，native DELETE=0。执行身份已禁用，gateway 恢复 DELETE 全拒绝，原对象/volumes 不清理。

`run-confirmed-delete.ps1` 是这一固定 run 的有限执行记录入口，要求 `-HumanConfirmedExactTwoVersions`，独立 permission/identity、有限窗口及 registry/native topology preflight。已存在 `delete-acceptance.json` 会拒绝再次执行；本次完成后不得重新启用。新对象或环境需要新的清单和用户确认，不能复用本次批准。

## 归档/恢复与单源删除切片

后续独立环境 `dpp-lifecycle307-c9f47cab` 按main55/57+D159–62安装，未升级原a6b30840环境。完成1 archive + 4 restore native COPY/same-byte verification、独立当前台账与隔离恢复 admission，见 [归档阶段报告](results/c9f47cab-23c6-4d16-8102-a4a34bac1501/archive-review.md)。

用户随后只确认本环境 online `fixture/a.csv@3fe1cc44-b0f8-428c-9bb8-9f92dac5d15d`。fresh 删除门禁与发送前重核通过，唯一 DELETE + 完整库存 absence receipt/scoped completion已完成；其余八版本与所有环境/volumes保留，见 [当前单源删除验收](results/c9f47cab-23c6-4d16-8102-a4a34bac1501/delete-review.md)。`run-confirmed-archive-source-delete.ps1 -HumanConfirmedExactSource` 是固定范围的一次性执行记录入口，已配置窗口即拒绝重跑；native executor已禁用，gateway全部DELETE拒绝。不要重新启用任何旧窗口。

归档后的源删除复用既有执行器，通过显式 `archive-source-c9f47cab` 固定 profile 限定环境/计划/hash/version；完整九副本库存仅登记、不获删除批准。测试依据校验完整冻结 registry及批准子集，不能把 fixture测试零最小保留期用于业务数据。真实业务 reader/worker recovery、完整D1与生产验收仍待完成；新删除仍需新的清单确认。

## 2026-10-11：C1/59 基线与 COPY UNKNOWN worker 恢复

实际 main/C1 已落在 migration59；当前未发布 D1 文件顺延为60–64，旧59–63验收环境的 schema、对象、保护、binary与volumes保持原样。新隔离数据库验证了 main59 upgrade 与 fresh64，最终 discovery关联守卫又在两个新保留库上验证。[迁移与测试证明](results/f7fa46ff-3a93-4e50-97a5-0d68fac8b235/migration-and-tests.json)记录 SQL hashes、准确基线与测试边界。

新封闭 `dpp-lifecycle307-f7fa46ff` 完成 COPY响应丢失/worker中断/outbox broker恢复、RESTORE独立账本 UNKNOWN→fresh ONLINE、当前restriction→BLOCKED，以及重复/撤销零额外I/O。使用原有 Asynq/outbox/worker；worker只读，不能重发COPY。七版本、全部旧环境和owned volumes保留，DELETE/解除保护/bypass/清理为0。当前alpha内容use已限制；历史ONLINE不代表现在可取数。详见 [受控验收与exact version清单](results/f7fa46ff-3a93-4e50-97a5-0d68fac8b235/copy-recovery-review.md)。

`prepare-copy-recovery.ps1`、`run-copy-recovery-step.ps1` 和 `readback-copy-recovery.ps1` 仅用于新schema64的有限合成环境。已封存的f7fa46ff窗口和原生producer/recovery身份均禁用，不得按旧脚本重新启用或迁移旧库。新增删除仍须新的环境/exact key/version清单与用户确认。Issue #307、完整D1和生产业务reader/delivery仍未收口。

## 2026-10-12：封闭 exact-version reader

新 `dpp-lifecycle307-d6dfc1cd` 只做非删除读取验收：三次真实GET分别验证进程崩溃、完整HTTP读取和输出取消；先提交durable pin，发送前重核当前caller/window/restriction，只有实际native read与输出owner均已停止才追加termination。取消不能提前排空，崩溃pin保留；same-key replay和阻断路径0新I/O/0内容bytes。四份原始版本、两份保护及全部旧环境/volumes保持原样；验收后native reader身份、窗口与gateway payload/admin已封闭。详见 [本轮验收、边界和exact保留清单](results/d6dfc1cd-8eff-4d7d-bed2-515a4b5243f2/reader-review.md)。

后续核对发现HTTP Write返回可能只完成缓冲，初次候选没有覆盖实际flush。已补owned write deadline和同步Flush，另建 `dpp-lifecycle307-34696798` 完成最终native重验、真实PG/race和全Go模块检查；以 [修正后的最终报告](results/34696798-06e3-42de-8b31-6b237c6e9c22/reader-review.md) 为当前HTTP输出闭合证据。两个环境都封存、四份对象各自保留；无删除/清理，原报告追加更正而不覆盖原始事实。

仅用于新环境的入口：先`prepare.ps1 -ArchiveAcceptance`，可显式传不重叠的`-InternalSubnet 10.234.x.0/24`。若需重建本地fixture image，`-MinioImage sha256:<实际digest>`仍强制检查固定MinIO binary SHA256；不得用于修改旧环境。`prepare-reader.ps1`建立有限window及独立native readonly身份；`run-reader-step.ps1`依次bind/crash/accept/final；`seal-reader.ps1`禁用身份并封闭窗口/payload/admin，`verify-reader.ps1`独立离线核对证据与敏感信息。已封存环境不能重跑，所有成功/失败helpers与volumes保留。

仍只修改未发布64，不占用C2/65及共享路由。现有production processing/delivery通用读路径尚未接pin，不在本封闭runtime内；LC-11/12、全平台closure及完整D1仍NOT_RUN，#307不关闭。没有新增删除或清理授权。

## 2026-10-12：实际 NATIVE processing、DirectData pin 与自然到期

封闭 `dpp-lifecycle307-326fedd2` 使用新的合成输入、原有 NATIVE engine / queue handler 和真实 Quality / Certification / current entitlement Command/Gate。全部 execution input typed pins 先于 I/O，输出 intent / physical call 先于固定槽位 create-only PUT；DirectData pin 与 ISSUED 同事务。真实 TCP 慢客户端不读响应，有限 UseUntil 驱动实际 WriteDeadline / Flush 超时，确认停止后追加 interrupted transfer observation，原 ISSUED 保留。same-key replay、queued reverse restriction 和自然到期后的新 delivery 均0新 native I/O。详见 [验收及六个保留 exact versions](results/326fedd2-8c22-4eb8-acca-05be57e15d77/plane-review.md)。

新环境入口为 `prepare.ps1 -ArchiveAcceptance -ProcessingDeliveryAcceptance` → `prepare-plane.ps1` → `run-plane-step.ps1 -Mode setup/accept/final` → `seal-plane.ps1`。只使用两个固定合成输入和两个固定输出槽位；helper HTTP 仅自身 loopback，无 host port。reader / producer 与 root-admin 分离；未提供删除实现或批准。所有旧窗口及本轮成功/失败环境保留，不重放已经封闭的步骤。失败的3f1e4cdb、e1ddedb7、121e25f9事实独立保留，不算最终验收通过。

D1/60 up/down 同时修复迁移组合发现的 CostAllocation guard 漏掉 main external provider/usage subjects 的问题；完整新库 PostgreSQL 回归及空60 down恢复完整main59 guard通过。D1仍≤64，C2使用65并须对最终 committed snapshot 重验，原C2组合证明不自动升级。完整D1、真实业务对象、production composition、外部provider与新增删除仍不在本轮通过范围，#307保持OPEN。

## D1 → C2 组合重验与保留服务持久化检查

组合重验发现旧D1 down测试没有固定60的边界，加入空65后错误要求合法65 down拒绝。`b853466f`将该真实PG测试固定到独立max60 scratch，核对具体拒绝及五表完整count/hash；生产Go与60–64不改。共享routing/worker注册冲突由C2 writer在scratch source中处理，SQL组合与完整源码组合分别记录，不能互相代替。具体阶段与最终回执见 [协调报告](results/d1-c2-0196b49d-00bd4d4e/coordination-review.md)。

C2曾误用其保留Core的256MiB tmpfs进行组合验证，空间不足导致进程退出及数据库内容丢失；旧试用不可用、原历史未恢复，新的验证库不叫历史恢复。D1自己的named-volume服务和封闭事实仍保留。未来D1 prepare/reader/plane入口新增只读`Assert-FixtureDurableStorage`检查，复用通用`Assert-RetainedServiceStorage`供C2四角色采用；真实owner、独立可写registered named volumes、无data/WAL tmpfs，以及明确pre-start/started状态须通过才能继续。它不修改旧挂载，不提供cleanup或解保护能力。

## Checkpoint 发布重试契约（PR #314）

Core checkpoint 与独立文件 witness 不构成跨存储原子事务。发布调用必须持有稳定 Command key，直到该 cut 的文件 append/fsync 成功；数据库提交后失败，使用同一 key 取得原 ID、sequence、hash 和 expiry，不能先分配新的 key/sequence。`copy-witness` 使用必填 `LIFECYCLE_FIXTURE_CHECKPOINT_KEY`；脚本用固定 `-Scenario` 传入。新的 cut 使用新的 scenario，失败重试保留原 scenario。每次重试仍校验当前 publisher 权限；重试不延长 checkpoint 有效期。

独立 ledger 通过现有 `golang.org/x/sys` OS 文件锁串行化多进程读写；同一 cut 的完整 append 是幂等的，部分 append 只允许补齐该 cut 的精确字节前缀。不同内容、缺失前序、损坏或过期 witness 仍拒绝恢复 admission，不重建独立历史、不修改旧环境。后一个 cut 先到时拒绝跳号，前序发布成功后以原 key 重试后一个 cut。复用现有 Command 幂等表、workspace fence、Audit/Evidence/Outbox，无新增 migration、队列或通用 workflow runtime；不把 fixture witness 扩大为生产灾备系统。


新run的四个同步helper入口复用 `examples/temporary-containers.ps1`，完成业务/provenance/exit落盘后默认私有归档并退役helper，保留provider、数据和卷。
依赖原始Docker metadata的有限D1验收可显式传 `-RetainHelper`；reader/copy的预期非零故障、plane扩展D1模式自动retain，copy `StartOnly`仍在completion前返回并保留屏障helper。
既有封存5d入口和私有配置不热更新、不重跑；需要持续metadata的新验收必须显式选择retain，不能把exit0等同证据生命周期结束。
