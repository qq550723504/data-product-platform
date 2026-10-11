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

准备程序 seeded READY 只表示已验证的合成 content identity。两个 namespace 的同 hash 对象都是输入 fixture，不能作为 archive/restore 成功事实；没有 Rights、Quality、Certification、正常业务 reader、真实交付或完整 D1 验收。

## 本次保留环境

Run `a6b30840-3e76-49ad-91d3-f0c399fcf170` 保留三台运行中的服务、已退出成功的 prepare helper，以及 PostgreSQL/object 两个 named volumes。初次脚本尝试中的空 credentials 目录也未清理；没有清理任何对象、bucket 或 volume。

本次运行冻结在 main `3a59b591d25e7d07fc38956a7ae729d74967215b` + 本地 D1 schema 000054；完整 SQL snapshot 私下保留。期间 #309 把 000055 合入 main，开发候选随后须重排迁移；**不得把新编号 migration 再安装到已保留的这套历史库，或因此重建/擦除本 fixture**。清单保存执行时 source/binary hashes，后续入口修正不覆盖原生版本/运行证据。

测试：`go test -race -count=1 ./cmd/lifecycle-fixture-prepare ./internal/platform/storage ./internal/lifecycle/... ./internal/platform/routing ./internal/cost`；native provider +真实 PostgreSQL 成功记录在 run artifacts。没有调用 DELETE，包括预期返回拒绝的负例。
