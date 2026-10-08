# ADR-0013：受控 Label Studio fork 的有限 Submission 准入

- 状态：拟接受，随 [DPP #293](https://github.com/qq550723504/data-product-platform/pull/293) 文档合并生效；补齐既定 C1 方案的 fork 准入依据
- 日期：2026-10-08
- 补充：[ADR-0010](0010-reuse-commodity-capabilities.md)、[ADR-0012](0012-gold-dataset-annotation-boundary.md)；不替代 #205/#208 官方 CE reference Pilot 的历史验收
- 范围：专用、有限合成 Pilot 的 `controlled-fork-submission-v1` 模式；不授权 D 实施、镜像分发、环境变更或部署

## 背景与所需执行点

现有 Core 已有 Annotation Port、Result/ReviewDecision/Snapshot、worker/reconciliation、Outbox、Audit/Evidence/Cost 以及 Gold/rights/certification/delivery 能力。通用标注 UI 和交互继续复用 Label Studio，不能另建画布、审核引擎或队列。

#205/#208 的官方 CE reference adapter 已在其原范围完成验收。新的有限 Pilot 要求在 Label Studio 服务端拒绝错误 task/actor、撤销或重新分配后的旧页面写入，并在正式提交时产生可回收的可信作者、assignment/revision、配置与结果快照。外部结果回收的“接纳/拒绝”不能替代引擎写入时的授权；Core 事后复制 mutable annotation 也不能证明未捕获期间的正式提交历史。

需要区分两个 enforcement owner：

- Fork 承担自身资源边界的当前身份/权限与 assignment 写入控制，以及正式 Submission 的服务端生成和授权读取。
- Core 承担来源验证/接纳、Result 与 exact SourceResultBinding 的原子关系、全部 replay 分支、CORRECT 来源闭包、Snapshot seal/read-integrity/build 和 Gold/rights/certification/CurrentDeliveryGate。这些 Core 规则不搬入 fork。

## 固定基线证据与复用顺序

本决策的证据限定于官方 CE `1.23.0` 的基线提交 `2a9bfbcbf0a844b999de97e601d16050a893f5fb`，及 B 交接的 fork 镜像源码 `90153bb6450a160ed6a1a9129adce65b7c4b42f8`。不由当前网站文档推断其他版本、Enterprise edition 或未来产品的能力。

在固定 CE 基线的 [tasks/models.py](https://github.com/qq550723504/annotation-engine-label-studio/blob/2a9bfbcbf0a844b999de97e601d16050a893f5fb/label_studio/tasks/models.py) 与 [tasks/api.py](https://github.com/qq550723504/annotation-engine-label-studio/blob/2a9bfbcbf0a844b999de97e601d16050a893f5fb/label_studio/tasks/api.py) 中，没有本协议的 TaskAssignment、正式 Submission、assignment 令牌校验及其共享锁执行点。普通 Annotation 的创建、更新和导出不是本协议的不可变 Submission。

已有 fork 的 [assignment 写入检查](https://github.com/qq550723504/annotation-engine-label-studio/blob/90153bb6450a160ed6a1a9129adce65b7c4b42f8/label_studio/tasks/api.py#L65-L104) 锁定当前 assignment 并校验身份/令牌；[正式提交创建](https://github.com/qq550723504/annotation-engine-label-studio/blob/90153bb6450a160ed6a1a9129adce65b7c4b42f8/label_studio/tasks/submissions.py#L44-L92) 在引擎事务内锁定 project/assignment、校验 annotation 关系、保存 snapshot/hash 与服务端 actor，并提升写入令牌。授权普通 [Submission list/detail](https://github.com/qq550723504/annotation-engine-label-studio/blob/90153bb6450a160ed6a1a9129adce65b7c4b42f8/label_studio/tasks/api.py#L1091-L1174) 可被本模式复用。引用代码只证明已有执行点；不是本次新增跨仓协议的运行验收。

| 依次评估的方案 | 本次取舍及不能独自满足的条件 |
| --- | --- |
| 仓库已有 Port、worker、reconciliation、Audit/Evidence/Cost | 复用；这些位于 Core 边界，不控制 Label Studio 浏览器/直接 API 的所有引擎写入事务 |
| 官方 CE 原生 API / SDK | 保留为 #205/#208 reference 路径；本固定基线的普通 Annotation/export 不能提供所需 assignment 写入 fence 和正式不可变 Submission 身份 |
| Plugin / Extension / webhook | 前端隐藏/回调和创建、更新后的通知不能成为服务端 pre-commit 授权 fence；本固定基线未验证到覆盖注释、草稿及相关写入且与 assignment 撤销共享事务的受支持扩展契约，因此不据此准入。若通过改写 views/models/事务实现，须按服务端 patch 同样承担升级审查，不能仅重命名为 plugin 来免除准入 |
| 外部 Adapter | 用于可信映射、回收和 Core 接纳；拒绝回收错误结果不能撤回已在引擎提交的越权写入，不能原子绑定引擎当时的 actor/assignment/snapshot |
| Sidecar / gateway | 能限制网络入口，不能单独与引擎数据库的 assignment 撤销、直接写入及正式快照共享事务；本批不另建覆盖全部路径的平行授权/历史系统 |
| 复用既有受控 Label Studio fork | 仅复用上述进程内执行点，以 provider-neutral Port/Adapter 隔离；不新造标注产品，不把 Core 规则复制到 fork |
| X-AnyLabeling 等另一标注工具 | 能力地图保留未来候选；本有限文本 Pilot 没有更换标注 UI/增加第二引擎的必要，也没有已验证的等价服务端协议，故不引入 |

官方 [导出说明](https://labelstud.io/guide/export) 介绍 Annotation 的导出，且明确取消任务也可在导出中出现；[webhook 事件说明](https://labelstud.io/guide/webhook_reference) 描述 Annotation 创建/更新通知。将这些接口当作正式不可变提交或写入前授权锁没有证据。本表的不足结论是对固定版本和所需事务边界的评估，不是宣称所有扩展或其他 edition 永远无法实现。

## 决策与有限例外

允许为本专用合成 Pilot 接入已经存在的受控 fork，作为 ADR-0010/能力地图中“API/Extension/Adapter/Sidecar → Fork”最后一级的有限例外。只有冻结的 `controlled-fork-submission-v1` binding 选择该路径；不把它变成所有 Annotation 项目的默认 engine，也不回退到 mutable annotation 协议来绕过来源校验。

C1 保持：Core 是唯一业务 reviewer。Adapter 以已授权的项目管理身份使用普通 Submission list/detail 做内部回收，不要求 fork approval，不调用 fork review/release 代替读取，不授予自己 Reviewer，也不把 Core 决定写成 fork approval。意外的 fork 人工 review 按混用流程记录并隔离；自动 superseded 是来源历史。Fork 对外 release 继续 Manager + approved exact Submission，mutable export/storage delivery 的拒绝边界不开放。内部回收不能变成公开下载/交付路径。

本 ADR 拥有选择既有受控 fork 的理由与适用例外；[Engine integration §5.1](../architecture/annotation-engine-integration.md) 拥有字段映射、固定产物和有限回收协议；[Annotation Domain §3.1、§5–6](../architecture/annotation-domain.md) 拥有来源身份/fingerprint、原子绑定、replay/correction/freeze 契约；[LS #73](https://github.com/qq550723504/annotation-engine-label-studio/pull/73) 拥有 fork 的服务端授权边界。这里不维护第二套不同的字段或状态机规则。

## 维护、许可与替换边界

继续保留 Label Studio 的 Apache-2.0 许可和上游 notices。采用既有 controlled fork 的代价是维护小范围 server-side patches、升级差异和负例回归；固定基线不代表未来 release 自动兼容。升级必须重核所有受影响的 API/草稿/批量/导出/文件/存储写入边界、旧页面/令牌撤销和正式快照行为，重新固定来源、版本、镜像 digest 和 contract evidence。未验证的新构建不能继承旧 candidate 验收。

B [#72 固定交接](https://github.com/qq550723504/annotation-engine-label-studio/blob/ab7b76a4a36b19060c527659e2a994ead05cc5e8/docs/authorization/issue48-release-candidate.md) 的文档合并 SHA 与上述镜像源码 SHA 分开；digest、构建输入与迁移/恢复边界以 integration 的固定 manifest 引用为准。镜像仅构建机本地可用，未发布 registry；112 条服务端合成断言和已有 fork API 不证明跨仓接入或受信任浏览器验收完成。

退出时只替换 Adapter/连接及外部绑定；Core 已接纳的不可变 bytes、来源关系、Result/Decision/Snapshot 与 Gold 历史不回读旧引擎 current state。若上游 API 或受支持扩展以后能证明相同写入 fence 与正式提交契约，应优先复用并经新 contract/升级 review 替换，不扩大本 fork 的长期产品职责。

## 实施差额与生效门禁

当前 DPP 尚未实现新协议所需的完整 SourceResultBinding、所有 Result replay 的来源复核、Snapshot 来源闭包完整性以及 REVIEWED/SEALED 后到留证；差额和必要负例以 Domain 文档为权威。本 ADR 不能把已有 #205/#208 CE PASS、B 的本地服务端 PASS 或文档 CI 当作这些功能已实现。

- 本次仅新增 ADR 并同步旧 ADR、能力地图和 integration 的准入索引，保留已确认的三文档业务契约；文档准入随本 PR 合并生效，合并仍需用户单独授权。
- D 只有在文档评审/精确 HEAD CI、获授权文档合并及对应 main CI、B 的获授权可消费产物/环境边界，以及独立 D 实施授权后才能开工；不得因本 ADR 落盘或 CI 通过自行启动。
- 跨仓实施后须验证错误 scope/actor、撤销/重分配旧令牌、同源不同 fingerprint、每个 replay 分支、binding/payload 丢失或篡改、CORRECT 闭包、parent-first 竞争及有限批次完整回收。真实浏览器标注到 Core Review/Snapshot/Gold 的验收另行完成；用户暂缓的证书信任/浏览器门禁保持原状态。
- 镜像发布、生产/共享环境、真实数据、IAM/凭据和证书信任均需各自明确授权；不在此 ADR 中开启。

## 与旧决定的关系

ADR-0012 拒绝的是 fork 标注系统并复制 Core 规则，以及把可变外部状态当 Gold 真相；该拒绝继续成立。#205/#208 官方 CE reference Pilot 的历史选择和验收不被回写。本 ADR 只追加“复用既有 fork 的必要进程内执行点、Core 规则仍留在 Core”的有限例外，不构成对其他产品、edition、模式或生产场景的通用 fork 许可。
