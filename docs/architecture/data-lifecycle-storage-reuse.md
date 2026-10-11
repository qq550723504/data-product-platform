# 生命周期 Storage Build-vs-Buy / Reuse Check

> #300 设计核查，2026-10-11；代码基线 `main@b69433df4aa1b88e03fdfd6166f077c11960248d`。
> 本文拥有选型证据与能力准入；业务规则归 [Lifecycle Contract](data-lifecycle.md)。不修改部署，不引入依赖，也不配置自动删除。

## 1. 先复用仓库能力

| 现有能力 / 固定证据 | 可复用部分 | 本任务的真实缺口与处理 |
|---|---|---|
| [Store](../../apps/platform/internal/platform/storage/minio.go)、[Upload ObjectStore](../../apps/platform/internal/dataset/application/upload_version.go) | SDK 隔离的 Put/Get/PutImmutable；已有 conditional create、受控 bucket 校验 | 缺 exact provider version/generation、copy inventory、retention/hold、delete/verify contract。仅在该 Adapter/Application Port 扩展实施必需动作，Core 不导入 SDK |
| [go.mod](../../apps/platform/go.mod) | `minio-go/v7 v7.0.83`、`madmin-go/v3 v3.0.94` 已引入 | SDK 的存在不是部署有全部能力；优先原生 S3 数据面 API，madmin 不作为 Core 必需 contract |
| [queue](../../apps/platform/internal/platform/queue/asynq.go)、[Outbox](outbox-delivery-and-idempotency.md)、[执行恢复](native-execution-recovery-and-output-idempotency.md) | 已有 Asynq `v0.25.1`、worker、事务 Outbox、各领域 reconciliation/幂等 | 生命周期增加受控 handler/对账，不新增第二个 scheduler 或 durable runtime；“retention-only outbox route”是事件留存语义，不是数据保留策略 |
| [deliveryfence](../../apps/platform/internal/platform/deliveryfence/fence.go)、[Direct-data](../../apps/platform/internal/delivery/application/direct_data.go) | workspace fence/revision 与先 ISSUED 后读数据 | 缺持续 reader/physical copy reservation 与 restore 当前台账；需要业务 admission 演进，不能靠 provider object lock 代替 |
| [Evidence](../../apps/platform/internal/evidence/store.go)、[hash](../../apps/platform/internal/evidence/hash.go)、[Audit](../../apps/platform/internal/platform/audit/store.go)、[Cost](../../apps/platform/internal/cost/store.go) | append-only helpers、业务事务、activity/attempt CostAllocation 模式 | 新生命周期活动须增加 typed allocation / attempt 关系；敏感 envelope 的专门 migration 未实施，不把 JSON subject IDs 当完成 |
| [#298 Storage report](../../examples/enterprise-activity/postgres-ingestion/results/2026-10-11/storage-report.json)、[gateway](../../examples/enterprise-activity/postgres-ingestion/storage.Caddyfile) | 固定原生 MinIO CE source/build、受限 STS、conditional write 与 Core/engine 隔离的历史证据 | 报告明确测试 Core/engine delete 被拒绝，不证明 lifecycle delete/object lock/replica erase 已测。不得扩大 #298 gateway 权限或复用其保留环境 |
| [Demo Compose](../../deploy/demo/compose.yml)、[开发 Compose](../../deploy/docker-compose/docker-compose.yml) | 可查看现有环境定义 | demo 使用 coollabsio digest 的历史基线，另一个使用 `minio/minio:latest`；不能据此认定其与 #298 build 相同或生产可准入。本任务均不改变 |

Core 必须最小自研的是 policy/hold/批准与 exact version/shared references 的业务合取、线性化和处置事实。MinIO/Ceph/S3 均不知道平台认证/当前 delegation/历史事实与活动业务输入；把这些语义交给 bucket expiration 不满足 invariant。WORM、tiering、version delete、队列与重试基础设施则直接复用成熟能力。

## 2. 固定版本 / edition / license 与方案比较

这里的“能力”是官方资料或源代码证据，不是本轮 Adapter contract test PASS。2026-10-11 实际核对官方仓库、固定 tag 和 license；漂移的维护状态在实施开工时重查。

| 方案 / edition / 固定版本 | License / 功能与限制 | 维护、运行成本、替换边界 / 决定 |
|---|---|---|
| 既有 Go client `github.com/minio/minio-go/v7@v7.0.83` | Apache-2.0 [S1]；源代码提供 retention、lifecycle configuration、version-ID removal [S2]。不是存储 server license，也不证明 hold/delete 的业务安全 | 复用既有 client，避免平行 SDK。配置/bucket 管理权限与受控 exact-copy 执行身份分开；替换只影响 Adapter，不改变 Core plan/receipts |
| **MinIO Community** `RELEASE.2025-10-15T17-29-55Z`；commit `9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a` | Server AGPL-3.0 [S3]；固定源码记录 lifecycle current/noncurrent expiration、tiering，以及 version retention/hold [S4–S5]。object lock/versioning 有创建/配置前提；不能用 AIStor 当前文档默认为 CE 同版行为 | 官方 repo 已在 2026-04-25 归档，API 核对 `archived=true`，latest release 是此 tag [S6]。复用为首片**合成、隔离验证候选**；已有 build 并不成为安全维护承诺。自行构建、WORM/副本配置与安全更新运维成本须由部署方承担；生产前独立决定支持/替换，不为恢复维护能力 fork 上游 |
| **Ceph RGW，Squid v19.2.6**（OSS 对比候选，不部署） | 主体 LGPL-2.1 或 LGPL-3，组件例外依 COPYING，不能称整包单一 license [S7]。固定版本 docs 明确 retention/hold 的 versionId API，源码有 lifecycle worker [S8]；不据“S3 compatible”声称所有扩展、tiering/restore 或全副本核验等价 | 官方列 v19.2.6 于 2026-08-19 发布；2026-10-11 repo `archived=false`、有当前提交 [S9]。RADOS + RGW/集群运行责任与容量/升级成本高于复用现有试验存储，首片无已证明需求，不引入。若组织已运行 Ceph，后续只换 Adapter/部署，须重跑 exact identity/lock/verify/unknown contract；本轮 runtime NOT_RUN |
| **Amazon S3 general purpose bucket，versioned + Object Lock**（托管标准对比，不是 OSS） | 商业服务条款，server build/开源 license 不适用；固定 S3 REST API `2006-03-01` 及 2026-10-11 官方行为资料 [S10–S11]。lock 保护对象 version，simple DELETE 可只加 marker；Governance bypass 与 Compliance 不同，不能把 API 返回成功等同永久清除 | AWS 承担服务维护，平台仍承担 account/region/IAM/费用、完整 copy inventory、业务审批与证明范围。生命周期异步执行不能参与当前 Core fence；不调用付费服务。后续替换可复用 Port，但 account/region/features、请求费用和 capability tests 须另批准 |

不采用新存储/合规平台：首片现有依赖能提供待验证的原生原语，主要缺口在业务 admission 与证据模型。Ceph 是成熟 OSS 替代，不因 MinIO 归档就立即迁移所有内容。AIStor 属商业产品，本轮未固定可用 build/许可/预算，未选用；CE 的已有能力不扩大为其商业 edition 的承诺。

AGPL/LGPL 的产品分发和网络服务义务由使用组织按 exact artifacts 确认；本记录提供许可证事实，不自动给出组织合规结论。候选镜像/SBOM、来源、漏洞处理与部署维护方案是实际实施准入，不将 `latest` 作为批准制品。

## 3. 第一个 Adapter 准入矩阵

| 原生能力 | 最小验收 / 失败语义 | 首片取舍 |
|---|---|---|
| versioning + exact delete/list/stat | 删除 exact version 后查证；bare-key delete marker、403、无权限 LIST 和 namespace 错配全部不能 VERIFIED | 用固定合成 bucket/版本，不允许 key 重用/任意 prefix glob/整桶清空 |
| retention / hold | provider retention/hold 与 Core policy 同时校验；hold 不可用/未同步 BLOCKED；不授予 bypass；释放 hold 后重查 retain_until | fixture 短周期 retention 与显式 hold/release；不配置用户 bucket |
| archive / restore | exact copy intent→hash/长度校验→受控 locator；丢响应保留源或 quarantine；不直接返回 provider URL | 两个受控合成 namespace 的明确复制，不把本地第二份复制称真正冷存储或 WORM disaster recovery；真实 tiering deferred |
| unknown / duplicate dispatch | drop response、kill worker；恢复原 exact copy/operation；fresh gate 后才准新副作用，new physical calls 各自计数 | 既有 worker/outbox/reconciliation，一条业务链一个调度责任方 |
| replicas / backups / engine copies | 获取/验证所承诺范围的清单；异步 replica/pending restore 不可遗漏 | 首片只有登记的合成 copies；不支持范围必须列明，不能把本地验证外推到云内部介质 |
| authenticated scope | provider identity/STS 仅准指定 bucket/version/action；无 root/bypass/public endpoint；Core tenant/reader fence 双边验证 | 新获授权独立 fixture 执行身份，不改变 #298 的 delete-deny 或现有 engine 凭据 |

任何 required 项未验证：相应 mode UNSUPPORTED / NOT_RUN，不能以架构文档、SDK 方法存在或旧 smoke PASS 替代。provider storage 原语也不能裁定保全冲突；业务决策仍归 [Lifecycle Contract](data-lifecycle.md)。

## 4. 官方证据索引

- S1：[minio-go v7.0.83 LICENSE](https://raw.githubusercontent.com/minio/minio-go/v7.0.83/LICENSE)。
- S2：固定 SDK [retention](https://raw.githubusercontent.com/minio/minio-go/v7.0.83/api-object-retention.go)、[lifecycle](https://raw.githubusercontent.com/minio/minio-go/v7.0.83/api-bucket-lifecycle.go)、[remove](https://raw.githubusercontent.com/minio/minio-go/v7.0.83/api-remove.go)。
- S3：[MinIO CE exact release](https://github.com/minio/minio/releases/tag/RELEASE.2025-10-15T17-29-55Z)、[LICENSE](https://raw.githubusercontent.com/minio/minio/RELEASE.2025-10-15T17-29-55Z/LICENSE)。官方 tag API 指向上述 commit，与 #298 报告相符。
- S4：[MinIO exact-tag Lifecycle Guide](https://raw.githubusercontent.com/minio/minio/RELEASE.2025-10-15T17-29-55Z/docs/bucket/lifecycle/README.md)。tiering 与 all-version expiration 含 MinIO 扩展，不能视为跨 provider 标准。
- S5：[MinIO exact-tag Retention Guide](https://raw.githubusercontent.com/minio/minio/RELEASE.2025-10-15T17-29-55Z/docs/bucket/retention/README.md)。只证明源码文档描述，配置前提和 actual deployment 要实测。
- S6：[官方 MinIO repo](https://github.com/minio/minio)、[repo metadata API](https://api.github.com/repos/minio/minio)、[latest release API](https://api.github.com/repos/minio/minio/releases/latest)。归档/发布信息是当前核查，非持续维护保证。
- S7：[Ceph v19.2.6 COPYING](https://raw.githubusercontent.com/ceph/ceph/v19.2.6/COPYING)。组件许可证按分发清单确认。
- S8：Ceph v19.2.6 [Object operations](https://raw.githubusercontent.com/ceph/ceph/v19.2.6/doc/radosgw/s3/objectops.rst)、[lifecycle implementation](https://raw.githubusercontent.com/ceph/ceph/v19.2.6/src/rgw/rgw_lc.cc)。不是本项目 integration evidence。
- S9：[Ceph Squid releases](https://docs.ceph.com/en/latest/releases/squid/)、[repo metadata API](https://api.github.com/repos/ceph/ceph)。具体环境准入另固定 image digest/安全状态。
- S10：[S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)。版本保护、hold、retention、delete marker/bypass 边界。
- S11：[S3 expiration](https://docs.aws.amazon.com/AmazonS3/latest/userguide/lifecycle-expire-general-considerations.html)。异步删除、noncurrent version、replication/lock 的行为限制；不能当 Core completion receipt。
