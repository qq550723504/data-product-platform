# ADR-0015：内容生命周期处置与不可变历史分离

- 状态：提议；设计合并后成为实施约束，不代表 runtime 已实现。
- 日期：2026-10-11。
- 关联：[Issue #300](https://github.com/qq550723504/data-product-platform/issues/300)、Epic #296；产品范围归 [V2 蓝图](../product/full-lifecycle-governance-v2.md)。
- 上游：[ADR-0014](0014-full-lifecycle-governance-boundary.md)、[ADR-0010](0010-reuse-commodity-capabilities.md)、ADR-0003/0005/0006；不撤销 AGENTS 与既有专门领域契约。

## 背景

不可变版本和认证保存的是历史业务事实，不承诺全部内容永久在线；敏感内容可能需要保留、停止使用、归档或受控销毁。直接新增 DatasetVersion.DELETED、软删除关系、重写 hash、关闭 guards 或配置 bucket expiration 会混淆历史、当前授权和物理现实，并可能删除活动读取、共享对象或 Evidence 的必需正文。

当前 Storage Port 尚无精确版本处置契约；delivery authorization fence 尚无物理 reader pin；Evidence V2 的 metadata/manifest 可能内嵌敏感内容。生命周期实现需要这些边界的最小演进，不能把已有 immutable trigger、对象锁或 worker 当完整证明。

## 决策

1. **三类事实分开。** 历史版本/认证/发布/交付 identity/hash 不变；当前使用资格由原 current gates 加生命周期拒绝条件控制；内容位置/可用性/处置覆盖通过新事实和合法 current projection 表达。精确规则由 [Lifecycle Contract](../architecture/data-lifecycle.md) 唯一维护。
2. **Core owns disposition semantics，provider owns physical primitives。** Core 持有版本化 Policy、Hold/release、exact plan/approval、当前 evaluation、活动引用与 completion coverage。复用既有 Storage Port/Adapter、Worker/Outbox/reconciliation 与 Audit/Evidence/Cost；不建新对象存储、通用法律/合规/工单引擎或第二套调度。版本/edition/license、维护和能力证据归 [Reuse Check](../architecture/data-lifecycle-storage-reuse.md)。
3. **当前复核、共用 fence 和 durable 占用。** 被纳管内容的生命周期 mutation 与 identity/rights/certification、processing/delivery/restore admission 串行化；沿 workspace fence、业务 parent、physical copy parent、operation/child 的固定锁序。长 I/O 不持 DB 行锁，读取占用/在途副作用不能靠 lease 到期认定消失。dispatch 只线性化副作用 admission；与晚到保全的不可逆竞争要诚实报告，不能承诺 provider 未提供的撤回能力。
4. **DB-first、append-only、范围化证明。** 外部调用前持久化 request/physical attempt，unknown 不盲重发；每次真实调用独立留成本，后续 resolution 不覆盖首次观察。仅在 exact targets/coverage 验证后声明范围内完成，备份/外部未确认单列。delete marker、404、请求成功均不是全域销毁。
5. **敏感正文与最小证明分离须专门演进。** 新 envelope/payload、hash/integrity、typed membership、migration 和 parent-first 约束先由 Evidence/Audit owner 冻结；现有内嵌敏感事实不因追加处置记录而已被擦除。本设计不修改 guards。pre-production 合成数据可按获准方案重建，不制造旧行兼容层；若发现真实需保留对象则另立专门方案。
6. **恢复不能复活禁用内容。** 隔离恢复依独立最新处置台账/水位重放当前禁止项，重验 bytes 与当前 gate 后才可接纳；台账缺失拒绝开放。历史认证与已销毁正文的可重算能力分开呈现。

## 替代方案及不采用原因

| 方案 | 不采用原因 |
|---|---|
| 修改 DatasetVersion status/hash/storage_uri 或删除历史 rows | 把物理位置与历史含义混在一起，破坏认证/发布/证据引用 |
| bucket lifecycle 自动到期即删除 | provider 不知道当前 rights、shared refs、reader、批准与 DB hold；现有边界无法将自动删除纳入 fence |
| 只用 object lock 或 DB transaction | WORM 只保护 provider version，不提供 Core 处置判断；DB 无法原子提交外部删除、阻止已交付副本传播 |
| 把所有 Audit/Evidence 永久保留 | 内嵌正文和 hash 也可能敏感；没有逐类依据/期限或最小化方案 |
| 新建通用治理状态机、Temporal/OPA/合规平台 | 现有单体、queue/outbox 已有运行责任方；缺口是业务语义与 Adapter admission，新增基础设施不解决它 |
| Fork 存储或标注系统以实现本契约 | 优先原生 API + Adapter；#300 不发现必须 fork 的理由，也不扩大 #294 已有 fork 边界 |

## 后果与实施门槛

历史解释可以保留，但原始内容删除后某些证明不可重算，必须明确呈现；provider/备份/外部接收方的保证范围会影响可支持模式。当前 MinIO Community 已归档，固定旧基线只准用于受控合成验证，生产需单独维护/安全/替换判断。

有界首片、负例、真实操作审批、后续顺序和 review blockers 归 [实施与验收](../product/data-lifecycle-design-acceptance.md)。本 ADR 不增加 schema、migration、runtime、UI、策略开关，不批准真实删除、部署或外部通知；设计完成不能计作归档/销毁功能验收。
