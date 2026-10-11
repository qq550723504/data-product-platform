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
