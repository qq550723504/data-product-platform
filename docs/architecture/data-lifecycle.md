# 数据保留、归档与销毁契约

> 状态：#300 的设计候选，**未实现 runtime / schema / migration / 存储策略**。
> 日期：2026-10-11；核查基线：`main@b69433df4aa1b88e03fdfd6166f077c11960248d`。
> 产品权威：[V2 蓝图](../product/full-lifecycle-governance-v2.md)、[Issue #300](https://github.com/qq550723504/data-product-platform/issues/300)。架构决定：[ADR-0015](../adr/0015-data-lifecycle-disposition-boundary.md)。

## 1. 权威、owner 与边界

本文是保留、保全、物理内容处置、可用性与活动引用的专门设计契约。状态与 Command 名称是后续实施的语义约束，不能据此声称 API 可用。存储选型证据归 [Storage Reuse Check](data-lifecycle-storage-reuse.md)；正负验收、首个代码切片、审批准入与有限 merge contract 归 [实施与验收](../product/data-lifecycle-design-acceptance.md)。总览只引用本文，不再维护生命周期规则副本。

| 业务事实 / 执行责任 | 唯一语义 owner / 既有权威 | 本契约与其关系 |
|---|---|---|
| DatasetVersion 内容身份、schema、实际 lineage、版本状态 | Dataset；ADR-0003、[Domain Model](domain-model.md)、AGENTS §3 | 不修改历史身份、hash、storage_uri 或借软删除修正；追加内容位置/可用性事实 |
| 当前 entitlement、source/delegation/disposition、认证、交付 | Rights / Certification / Delivery；[Rights](data-rights-provenance.md)、[Certified Dataset](certified-dataset.md) | 增加独立 lifecycle/content availability 拒绝条件，不替代已有 gate、不扩大授权 |
| Execution frozen dependencies、恢复/输出幂等 | Workflow；[依赖绑定](t3-b2-execution-dependency-bindings.md)、[执行恢复](native-execution-recovery-and-output-idempotency.md) | 后续在读取/恢复/产物接纳处接入当前可用性及活动占用，保留既有重试语义 |
| Annotation Result / SourceResultBinding / Snapshot、Gold | Annotation / Gold；[Domain](annotation-domain.md)、[Engine](annotation-engine-integration.md)、[Gold](gold-dataset-production.md) | 当前 Pilot 的 payload 完整性保留要求继续有效；本设计不批准清除其必需内容 |
| EvidenceSnapshot / Audit / Cost 历史 | Evidence / Audit / Cost；ADR-0005/0006、AGENTS §6–7 | 历史最小事实与可删除 payload 分离须有专门演进；不关闭现有 guards |
| Retention / Hold / disposition decision、覆盖报告 | Core 生命周期应用边界（待实现，模块化单体内） | 只拥有本次处置的业务语义和事实，不建设通用审批/法律引擎 |
| 字节、对象版本、WORM、tiering、副本观察 | Storage Port + provider Adapter；[Capability Map](capability-map.md)、ADR-0010 | provider 执行原语；Core 决定能否执行及可证明范围 |
| 源数据库、外部 engine、备份、外部接收方副本 | 对应系统/备份责任人/接收方 | 没有管理 authority 的副本只能跟踪请求与确认，不能代删或宣称不存在 |

数据负责人申请并确认资产范围；有服务端授权的处置批准人决定业务依据与例外；保全责任人决定保全/释放；运行身份只执行已批准 exact targets。owner_id 不等于现实法律所有权，用户点击、目录标签、root 存储账号均不能替代这些 authority。真实场景的责任人及职责分离由组织明确；平台不提供法律裁判，也不规定通用法定年限。

## 2. 三类独立事实与结果含义

| 维度 | 保存什么 | 不能推导什么 |
|---|---|---|
| 历史身份 / 决策 | 原 DatasetVersion、内容 digest/长度/schema、lineage、认证/发布/交付时点事实 | 历史 CERTIFIED/PUBLISHED/ISSUED 不代表当前能使用，也不证明字节仍在 |
| 当前使用资格 | 可信 principal→workspace/consumer；既有 current rights/certification；追加停止使用及到期约束 | 权利已撤销不等于允许销毁；保全不授予业务读取；在线不授予访问 |
| 物理内容可用性 | exact copy 的观察、在线/受控归档/未知/已验证删除；本次覆盖和未覆盖集合 | GET 404、delete marker、请求成功、密钥句柄失效均不是全域销毁证明 |

`RestrictContentUse` 的提交线性化停止**新**业务访问/处理/交付/恢复接纳；已有按既有规则提交的 direct-data ISSUED 是历史授权，其传输完成情况另记，不能改写。`ArchiveContent` 只证明在批准范围内有验证的受控归档副本和位置转换；归档内容即使 provider 能透明 GET，也必须经过 Core 当前门禁。`RequestContentDisposition` / `ApproveContentDisposition` 只记录申请/批准，批准不证明副作用发生。`FinalizeContentDisposition` 仅在 exact coverage 验证后证明“列明范围内副本已删除”；明确列出 retained、blocked、unknown、external-unconfirmed 和 out-of-scope。

不提供无范围的 `DELETED` / “数据已彻底消失”产品结论。在线、归档、删除不能成为 DatasetVersion 新的泛用 status，也不把归档等同 INVALID 或新版本。数据内容改变才创建新 DatasetVersion；同字节的批准迁移/归档恢复可以追加 locator/availability，保持原身份；已批准销毁或禁止重新入库的对象不可用 restore 将同一身份重新开放。确需重新采集时必须单独验证当前准入与处置依据、创建新的接纳/版本事实，不能悄悄撤销旧处置历史。

## 3. 最小必要模型（全部待实现）

以下是关系和字段约束，实施时可复用现有对象而不必逐名建立表。gate-critical identity、时间、关系、decision、revision 使用强类型列/关系；JSONB 只承载受控 provider 扩展，不能把 required target/reference/approval/hold 藏在 JSON。

| 概念 | 必需内容 / 冻结边界 |
|---|---|
| RetentionPolicyVersion + subject binding | workspace、适用 subject/category/purpose、policy version/hash、批准依据与责任人；明确起算事件种类及 exact event ID/time；最小保留 `retain_until`、允许使用截止 `use_until`、处置到期 `dispose_due_at` 分开；UNKNOWN/missing 不等于无约束。已生效 policy/binding 历史不覆盖，修订追加新版本/生效事实 |
| Content identity / copy binding | 绑定 exact DatasetVersion/Artifact/EvidencePayload 等已有主体；冻结 digest/算法/长度及 manifest；copy 保存 provider instance/incarnation、bucket/namespace、key + provider version/generation（或经证明唯一不可重用 identity）、tier/region/replica kind 与 controller。复制到新 key 是新 copy，不换写旧 binding |
| Content availability / restriction facts | append-only exact copy observation + subject use restriction；当前 projection 可由 Command 更新，绑定来源事实、observed_at、revision 和范围。建议展示 ONLINE / ARCHIVED / UNAVAILABLE / UNKNOWN / VERIFIED_REMOVED；这些不是现有枚举。subject 的业务可用性来自 resolver 当前批准的 copy 集合及 restriction，不因某个残留副本仍在线而自动 ONLINE。观察有时效，stale 不当授权依据 |
| Hold + release facts | workspace、强类型 target/member、保全范围（内容/正文/必要证明）、basis_ref、责任人、effective_at；release 单独追加。一个 hold 释放不释放其他 hold、retention 或 provider lock。禁止根据 cron 或权益到期自动解除；暂停在途自动 replication/copy/cleanup 也是保全覆盖的一部分，无法控制者显式列未确认 |
| DispositionOperation / plan / approval / evaluation | operation identity、action、request fingerprint、exact target membership/hash、scope exclusions、依据、expected revisions、批准人/有效期；plan 冻结后只通过新 operation / superseding plan 更改。每次 prepare/dispatch/reconcile/finalize 追加 decision/blockers、当前 authority、依赖 revision vector；transition 引用 evaluation/attempt，不覆盖历史 |
| Active content reference | 复用 Execution/Delivery/Annotation 等 immutable dependencies，另增加强类型受控占用：subject/copy、actual operation/reader/capability、generation/fence、开始、停止/containment 事实。历史关系与仍需要字节的 material pin 分开；refcount 只是 projection，完整成员才是依据 |
| Provider attempt / observation 与 completion coverage | stable provider_request_key（业务操作）、physical attempt identity/start（调用前提交）、append-only outcome/unknown/resolution；每份 copy 独立完成 receipt。coverage 冻结 expected/confirmed/retained/unknown 集合、证明方法/时间及验证者；完成时锁 parent 验证精确成员/hash |

所有需要 `DRAFT→FINALIZED` 的 plan、coverage 或 target aggregate 都遵守 parent-first：member mutation 与 finalize 持同 parent lock/revision，锁内校验 exact set/hash；finalized 后 INSERT/UPDATE/DELETE 拒绝。不能只在 trigger 无锁读取 status。current projection 可以合法更新，不安装阻止 lifecycle transition 的整行 immutable guard。

### 3.1 起算、冲突与批准

Policy 由真实责任人选择业务起算事件，如 Core 接纳完成、合同结束或业务关系终止；不默认以 object mtime、latest created_at、归档时间、恢复时间重置时钟。起算尚未发生时状态为未确定，定时扫描只能产生候选/提醒。生效时间以可信服务端事实和 UTC instant 计算；展示时可换时区，规则必须明示 duration/calendar 和端点语义（`as_of >= deadline` 到期）。合成试验的短期限只是 fixture，不是产品保留规则。

多项最小保留及 hold 取保守联合，禁止删除到最晚允许边界；最早 use_until 到达时停止新业务使用。需要在最小保留期前删除、未知起算/依据、司法要求冲突、共享主体策略冲突时，记录 `POLICY_CONFLICT` 并 fail closed，交给授权责任人明确依据；平台不得自选“优先法律”或通过绕过 WORM 执行。

申请应包含准确主体/副本、动作、剩余引用、不可重算影响、可验证与无法覆盖范围。批准绑定 plan hash、workspace、action、依据和有限有效期，不是可用于未来任意 key 的通行证。申请人不能用 dataset owner 身份隐式成为处置批准人；生产职责分离由部署组织配置并验证。运行前 fresh 验证批准 authority、principal/delegation 及有效期。取消批准、policy/hold 变化推进同一 fence。重试不复用已失效批准；须追加新 approval/evaluation。

## 4. 范围、引用与责任

| 对象范围 | 默认 owner / 处置条件 | 能证明的边界 |
|---|---|---|
| 主产物、RAW、STANDARDIZED、CURATED、Gold 物理内容 | Core 内容责任人；精确版本/manifest，生命周期门禁 + 所有合法活动/material pins | 历史身份仍存在；删除字节后原内容重算 unavailable |
| 派生产物及 required source inputs | Dataset/Workflow 真实 lineage；逐版本 policy，不自动级联 | source 字节不再保留不自动抹除独立输出；其 current source rights 仍按全部 required inputs 检查，依赖事实不得随字节删掉 |
| 共享/去重内容 | 所有强类型 copy bindings/reference owners | 只要任一合法 material reference、未到期保留或 hold 仍需内容，就不能删公共字节；解除一个 binding 不能声称公共对象已销毁 |
| 多副本、非当前版本、tier、replication target、未完成 multipart | Storage controller，exact version/generation 与已知复制清单 | 原对象删除不证明 replica/旧版本/未完成分片消失；独立确认，不能忽略正在创建的副本 |
| 缓存、搜索索引、预览、导出临时文件 | 创建它们的模块/数据面 owner | 首先关闭受控新访问，再按 subject generation 失效并验证；索引删除不等于 origin 删除，缓存无法失效则限制保证范围 |
| Annotation engine / Core canonical payload / 隔离样本 | Annotation/Workflow owner；源/result/snapshot/Gold 必需闭包 | 已 seal/read-integrity/build 必需 payload 是 material pin，当前契约不允许本任务清除；外部 project delete 也不等于 Core/source 副本删除 |
| Evidence/Audit、诊断日志、Outbox/dead letter、CI artifact | 相应 owner；按 §8 的正文/证明分类 | provider 回执、错误、trace、before/after 和 hash 也可能含敏感信息；每个副本单独纳入计划 |
| 数据库与对象备份、WAL、snapshot、离线/异地介质 | 备份责任人 | 无法逐对象擦除时保留受控、不可业务恢复的副本，列明到期/销毁办法与未确认范围；不能把在线删完描述为全局完成 |
| 已交付外部副本 / 外部 live source | 源系统或接收方；Core Delivery/通知事实关联 | 能验证的是请求/接收方确认或约定 provider 范围。无法控制的下载、截屏、二次传播始终不作消失承诺 |

source 字节删除对依赖它的 replay、质量复算、Annotation source integrity、证据重验的影响必须逐项列出。只要当前合同/保全/已批准工作要求可重算，该关系就是 material pin。纯历史引用不自动成为永久物理保留理由，但要先由专门 owner 判断保留要求，并在批准中明确失去什么验证能力；生命周期模块不能自行把所有历史关系降级。

归档不通过改写历史 reference 把源 pin“转走”。允许切换 operational material pin 到新 copy 时，由该 reference owner 在共享 fence/parents 下验证新 copy 等价、持有同样 policy/hold、满足当前读取/重算要求，追加新 binding 与旧占用释放事实；immutable dependency identity 仍不变。尚未完成迁移的 pin 继续保护源 copy。

首个切片不启用跨 workspace 共享去重。未来确有共享 provider copy 时，物理 copy 使用平台可信共享 identity 和 parent revision，绑定/解除/保全均锁该 parent；全局受控 resolver 在授权范围内只返回“存在其他有效引用”阻断，不泄露其他租户详情。查不全引用时拒绝删除，不能从当前 workspace 的 `count=0` 推导全局无人使用。

## 5. Policy × current rights × availability × active references × hold × external capability

这是 guard 组合，不是只按一个因素放行的评分。P 包含已解析起算/保留/到期、动作依据与本次有效批准；R 是请求具体动作的当前 authority/rights：USE/PROCESS/DELIVER/RESTORE 继续原有语义，DESTROY/保留迁移要求独立处置/保管 authority，不从 use rights 推导。A 是当前 exact copy availability；F 包含活动 reader、material pins、共享引用和可用 credential；H 是所有有效 hold/retention lock。C 是 Adapter 已验证的该动作与范围能力。任何 gate-critical UNKNOWN 都不是 ALLOWED。

| 行 | 动作 / P | R（含可信主体） | A | F | H | C | 决定 / 证明限制 |
|---|---|---|---|---|---|---|---|
| D01 | 新访问/处理/交付；use_until 未到，无 restriction | 当前具体动作全 ALLOWED，交付另完整 CurrentDeliveryGate | ONLINE、identity verified | 可在锁内建立占用 | 任意，hold 不授予 use | 受控读路径 | 放行一次受控读取；保全不抵消 rights 禁止 |
| D02 | 同 D01 | BLOCKED / UNKNOWN 或 lifecycle 到期/停止使用 | 任意 | 任意 | 任意 | 任意 | 0 新 payload / job / credential；保留历史/保全内容 |
| D03 | 新业务读取 | ALLOWED | ARCHIVED / UNAVAILABLE / UNKNOWN | 任意 | 任意 | 即使支持透明 GET | 阻断该读取；申请受控 restore，不自动回源或从缓存绕过 |
| D04 | RestrictContentUse；依据和管理批准有效 | 当前管理 authority ALLOWED | 任意 | 可有活动/credential | 可有 hold | Core gate 可执行 | 提交新-use restriction；已有访问能力的 containment/未确认范围独立记录，不能声称已收回 bytes |
| D05 | Archive；保管依据/有效批准，非销毁 | 保管迁移 authority ALLOWED；业务 use 可 BLOCKED | 在线且身份明确 | 没有阻止迁移的活动；pins 仍由归档满足 | hold 范围明确允许等价保全迁移 | verified target + retention/hold 传递 | 验证目标后追加归档事实；源删除仍需独立 destroy gate，不能用 copy 成功代替 |
| D06 | 同 D05 | 同 D05 | 任意 | 活动/共享迁移影响未知 | hold 不允许/未知，或 lock 不能传递 | 任意/不支持 | 不动源；BLOCKED。可以只保留现有内容 |
| D07 | DESTROY；到期/依据明确且 exact plan 有效批准 | disposal authority ALLOWED；业务 use 可已 REVOKED | exact copies 已盘点 | 0 未释放活动/required material/shared pins/活跃能力 | 无 hold，最小 retention 全到期 | exact-version delete + authoritative scoped verification | 准许 dispatch；**仍不是 VERIFIED_REMOVED**，finalize 需 receipt 与 coverage |
| D08 | DESTROY；未到最小保留期/未知起算/冲突/批准失效 | 任意 | 任意 | 任意 | 任意 | 任意 | BLOCKED，无调用；定时器不能裁决 |
| D09 | DESTROY；同 D07 | 同 D07 | 任意 | 非零或 UNKNOWN；另一合法版本共享 | 任意 | 任意 | BLOCKED；不能以引用软删除、lease 到时或当前 rights revoke 推导 quiescence |
| D10 | DESTROY；同 D07 | 同 D07 | 任意 | 0 | 有 hold / UNKNOWN / provider lock | 任意 | BLOCKED；禁止 governance-bypass，hold 释放后仍重查 retention |
| D11 | DESTROY；同 D07 | 同 D07 | 只有 key 404/marker，version/replica 未查清 | 0 | 无 | 不支持 exact identity/范围核验 | UNSUPPORTED / UNKNOWN；不伪造全删结论 |
| D12 | Reconcile 已 dispatch 的操作 | 当前对账 authority ALLOWED，fresh gate 禁止新处置时只读核验/containment | 部分 removed 或 UNKNOWN | 排他 reservation 保留 | 可出现 late hold | 能查原 exact request/copy | 追加原操作 outcome；新真实调用独立 attempt/cost；不自动重发 delete |
| D13 | Finalize disposition | 当前 finalize authority/coverage 可验证 | required targets 全已验证 removed | 无遗漏或新副本 | late hold/冲突按 §6.3 记录 | exact receipts 有效 | 只冻结 confirmed coverage；late incident、保留备份/外部未确认均如实显示，不回写旧证据 |
| D14 | Restore / backup re-entry；未禁止恢复，policy 仍允许 | 当前 restore/保管 authority ALLOWED；后续 use 各自 fresh gate | ARCHIVED、hash 可重验 | 可建立新 copy pin/reservation | 不破坏 hold/retention | 隔离 restore + 校验 | 仅先进入 quarantine，fenced 接纳后可成为 ONLINE；不自动新授认证或 use |
| D15 | Restore；已销毁/禁止重入库/过期或台账无法追到最新水位 | 即使旧 backup rights ALLOWED | 任意 | 任意 | 任意 | 可恢复 bytes | 拒绝业务开放，0 新 delivery/processing；不能复活旧 projection |
| D16 | 请求外部接收方处置 | 可验证通知 authority | 外部 UNKNOWN | 外部引用未知 | 由外部责任人决定 | 只有通知/自报 | 仅 REQUESTED/ACKNOWLEDGED；无验证不得列入 verified destruction |

合取优先级：先检查 workspace/可信 authority，再拒绝 policy/hold/reference/能力冲突，最后允许具体动作；每个 blocker 都保留，不通过首个成功项短路。对 derived 新交付，当前 source rights 不依赖 source bytes 在线；但所需 source 声明、授权、delegation、disposition 等 current facts 必须存在且可重验。字节可用性的门禁检查**本次实际会读**的 inputs/copies；historical EffectiveRightsSnapshot 仍不能代替 current rights。

## 6. 命令、状态与并发边界

### 6.1 显式 Command 与 operation projection

最小 Command 为 `BindRetentionPolicy`、`PlaceContentHold` / `ReleaseContentHold`、`RestrictContentUse`、`RequestContentDisposition`、`ApproveContentDisposition`、`DispatchContentDisposition`、`ReconcileContentDisposition`、`FinalizeContentDisposition`；归档/恢复是 exact action 的受控 operation（`ArchiveContent` / `RestoreContent`）。恢复 use restriction 必须单独管理 Command + 当前复核，不能 PATCH 到 ONLINE 来放行。

建议 projection：`REQUESTED → APPROVED → DRAINING → PREPARED → DISPATCHED → VERIFYING → COMPLETED`；批准前拒绝/取消可进入 `BLOCKED / CANCELLED`。外部可能已执行时进入 `OUTCOME_UNKNOWN`；部分已确认进入 `PARTIAL`，后续只能对未确认集合对账/执行新获准尝试；已不可逆的已确认成员不退回 REQUESTED。`FAILED` 只表示没有未决副作用且该 operation 无法完成；存在不确定删除不能包装为可安全重试的失败。具体枚举在实施 PR 固定，本文未新增可达 runtime 状态。

关键 transition 同业务事务产生固定 Domain Event（如 ContentUseRestricted、ContentHoldPlaced/Released、ContentDispositionApproved/Dispatched/OutcomeUnknown/PartiallyCompleted/Completed/Blocked/Failed）+ Audit + Evidence + Outbox routing obligation。UNKNOWN/PARTIAL 是安全关键非终态，也留完整事实。幂等键为 workspace + command + canonical fingerprint；同键不同 targets/action/policy/approval 冲突。same command replay 不重复 event/成本；新真实动作、新批准/plan 或 attempt 都有独立 identity。

### 6.2 共享 fence 与固定锁顺序

复用 `platform/deliveryfence` 的 workspace authorization fence/revision；生命周期变更、当前 identity/rights/certification 变更、实际读取 admission、pin 建立/释放、copy 注册、processing retry/recovery/output finalize、delivery initial/finalize/replay、restore 接纳必须参与同一机制。这里只约束被纳入生命周期管理的路径，**当前尚未证明这些路径全部接入**。

实施目标的短事务固定顺序：

1. 按 workspace ID 排序锁授权 fences；首片仅单 workspace。
2. 锁业务内容 aggregate parents：按 `(kind, id)` 固定顺序，Dataset parent 在它的 Version 前。沿用各领域已有 parent-first 顺序。
3. 按稳定 physical content/copy ID 锁 copy/reference parent；跨租户共享需可信全局 parent，首片不启用。
4. 锁本次 operation/plan/其他相关父 aggregate，再读写 child refs、attempts、observations、coverage；每个父内固定 child 排序。

所有所需集合先用只读查询发现，拿锁后重新验证 closure/revision；有新 binding/copy/lineage 时回滚重取，不能持后层锁再倒着扩锁集合。idempotency claim 可先独立短事务分配，但不能边持 operation/ref child lock 边回头拿 fence。现有 Execution advisory lock 用于运行接管，不能替代该 gate fence；implementation preflight 必须检查既有调用序列是否与上述顺序相容，必要时在同一有界 PR 调整相关路径，禁止只新增删除 worker 后宣称安全。

外部 HTTP/对象 I/O 不持 DB row lock。短事务提交 durable reservation/reader pin 后再 I/O，过程中每次新 admission/renewal/retry 都 fresh 验证当前状态；删除 dispatch 必须确认先前占用已经真实结束。reservation、关闭新 admission 和 revision 串行化引用变化，而非靠缓存 refcount、租约超时或事后 trigger。

### 6.3 处理、交付、保全与删除竞争

| 竞争 | 线性化与约束 |
|---|---|
| reader/processing 先进入 | 在 fence 下验证 current rights/availability 并提交强类型占用；处置先进入 DRAINING、停止新 admission，等待已准许工作完成，或按既有 Command 取消并确认 engine/reader quiescent；不能删除仍被读取的 bytes |
| 处置先进入 | restriction/reservation 提交后所有新 processing/delivery/restore/build/retry admission 拒绝；queued job 到真正读之前重验，不把排队时检查当授权 |
| direct-data ISSUED 与删除 | 沿用 ISSUED commit 后才能首字节；同事务建立内容传输 pin，commit 前 0 bytes。响应结束/中断追加独立 observation 后才能释放；崩溃/lease 到期不能证明停读，须验证 reader 已终止或受 token generation fencing 约束后解除。不得长时间持 fence，也不改写 ISSUED |
| 现有 credential 与删除/归档 | 尚未到 fresh-cap expiry、未确认 revoke/contain 的可用 capability 是活动引用；等待到期或确认 containment 后才能移除依赖内容。replay 继续原 fresh authorize；已下载外部 bytes 归外部范围，不假设 revoke 会收回 |
| restore/copy/output 接纳与删除 | 外部写入前注册 pending copy/生产占用；重新接纳 ONLINE/READY/派生输出前锁 fence 重验 restriction、copy identity、policy 与依赖；失败保留 quarantine/orphan receipt，不能登记为可用而避开 reservation |
| hold 在 dispatch 前提交 | Hold 推进 fence，阻止 PREPARED→DISPATCHED；即使已有批准也失效。释放后重新验证全部条件 |
| dispatch 在 hold 前提交 | dispatch 是不可逆副作用 admission 的线性化点，**不是删除完成证明**。Hold 请求记录保全冲突/紧急处置，阻止所有尚未 dispatch 的 members；可能已发出的调用/丢失响应不能承诺撤回或保全成功，须尝试 provider stop/hold（若支持）并对账、升级责任人，记录无法保全的 exact 范围 |

没有跨 PostgreSQL/provider 原子提交。late hold 不能被静默当成“已保护”，也不能删除其申请记录；若组织要求“收到任何保全请求后连在途删除都必然能撤销”，而 provider 不支持可验证暂停/撤销协议，则该模式 UNSUPPORTED，不能用一个 DB fence 虚构保证。目标选择/dispatch 以当前 DB fence 为准；要启用 provider 自动生命周期删除，须另证明它也参与受控 admission/hold/reference 协议，否则在平台受管内容上禁止配置自动 expiration。

新 credential issuance / replay 的 fresh expiry cap 在既有完整 identity/rights/certification 边界之外，还须取已知 `use_until` 和未来生效 use restriction 的最早截止；不将最小保留 `retain_until` 当业务可用期限。redemption/gateway 每次使用重查 lifecycle/current gates。已有不可回调 bearer capability 在新增 restriction 后无法立刻收回时，保持 active pin 并记录 containment/未确认范围，不能承诺即时收回；未完成 containment 不准删其仍可读 copy。

## 7. Provider Port、DB-first 与验证

### 7.1 扩展既有 Storage Port 的最小动作

在 Application 接口定义 SDK 无关的 `InspectExactCopy`、`InspectRetentionAndHold`、`CreateArchiveCopy/RestoreCopy`、`DeleteExactCopy`、`LookupDisposition`、`VerifyAbsentInScope`；只对实施片必需动作增加 contract，原有 Put/Get 不赋予删除 authority。不新增通用 storage 系统。返回 typed copy identity、observed scope、provider receipt/reference、actual retention/hold、outcome 与 error category，不将 error 文本当业务真相。

Adapter capability 必须固定 provider release/build/edition、SDK、versioning/lock/tier/replication/backup 配置及验证日期；不支持一项就显式 UNSUPPORTED。S3 不保证业务 request_key 原生去重：只要 exact version identity 永不重用，对该 identity 的受控重复删除可采用“查原对象状态后 fresh gate 再 dispatch”的幂等后置条件；不能对可能被重新上传的 bare key 盲删。S3-compatible 不等于能力等价。

ARCHIVE 先 DB 登记 target copy intent，调用复制/tiering，校验目标 digest/长度/retention/hold、可访问边界、已知副本及 source identity；不重写原 DatasetVersion locator。resolver 使用追加 copy binding/availability 选择当前批准位置。源副本删除独立受 gate 控制，未知复制期间保留源。RESTORE 先放隔离 namespace，尚未 finalize 不可业务读；re-gate + identity verify 后才追加可用 copy。provider tiering 自动读回不允许绕过 resolver。

### 7.2 一次物理调用的恢复协议

1. DB 事务分配 operation/request_key，冻结 exact plan，追加 approval/evaluation，关闭不允许的新 admission，Outbox 唤醒既有 worker。定时扫描只登记候选。
2. 持 fence/parent 锁 fresh 验证 principal、批准、policy、references、hold、availability 与 capability；提交 exclusive reservation、`DISPATCHED` admission 和 physical attempt/start。只有调用前 start 已提交才可调用 provider。
3. 每一次真实 delete、copy、restore、lookup、hold/stop、verify 请求使用自己的 physical attempt ID。真实调用成本不取决于业务终态；至少记录 quantity/unit，金额未知不编造。调用前 start 对“可能已发出”建立待核实 activity；若进程在实际调用前死亡，只能把 invocation 标为 unknown，不声称已知计费或删除。能证明未发送时追加 NOT_SENT，不覆盖原 start；能确认实际调用时对应 CostEvent quantity 计真实调用，unknown 单列可能成本，不把 start 行数当确定 invocation 总数。
4. 返回/超时/crash 追加 observation；超时为 UNKNOWN，lease 失效也不能回到未发出。原 attempt 永不 UPDATE 首次观察；重复入队先恢复原 operation，Core reconciliation 不独立启动同一工作。
5. 对账先 fresh 验证当前 authority。gate 已不允许新删除时可以经单独只读核验/安全 containment authority 查询原副作用，不把这类查询伪装为普通用户读取；不能继续删除未执行 members。每次实际查询独立 attempt/cost。需新真实副作用时必须重新 fresh gate/批准/expiry，并使用新 physical attempt，不吞入 operation 顶层幂等。
6. authoritative 验证 exact version/generation、已声明覆盖的复制/缓存/索引以及 pending copy 不再产生新内容；锁 parent 完成 coverage exact membership 校验，追加 completion event/Audit/Evidence/CostAllocation（若有该活动成本）/Outbox。保留未确认 members 和 original unknown observation。

`VerifyAbsentInScope` 的成功须证明有权限完成已冻结范围的版本/副本盘点、明确 observed_at/watermark 和 identity，区分 NOT_FOUND、DENIED、provider unavailable、eventually-consistent unknown。DELETE 200、bare-key HEAD 404、ETag、delete marker 或清空缓存均不足。可报告“provider 可访问命名空间的 exact version 已验证不存在”；底层介质、provider 内部灾备和物理擦除需另有可审计 provider 证明，否则不扩大措辞。

历史 DatasetVersion/认证/发布仍引用旧 hash 和原位置事实；当前查询呈现 receipt/new locator/不可重验原因，禁止声称原 hash 仍已重新校验。部分成功是 per-copy receipts + PARTIAL/UNKNOWN，不能回滚物理现实，也不对已经确认删除的 copy 创建第二个成功事件。外部通知在没有直接用户消息授权时只形成任务/计划，不自动向接收方发消息。

## 8. 敏感 Evidence / Audit 与历史证明

### 8.1 内容分类及最小保留

| 部分 | 本契约要求 |
|---|---|
| 最小历史证明 | 仅为批准依据所必需的 pseudonymous 主体/对象/operation IDs、action、decision/reason codes、rule/policy version、时间、exact coverage/receipt references、可受控 digest/commitment、活动成本单位及关系；本身也受权限、独立保留依据/期限、hold 与泄露风险管理，不默认永久或匿名 |
| 原始正文 | 原数据/样本/annotation text、SQL/错误回显、附件、provider response、before/after 全量 JSON、个人身份信息等，默认外置成受控 Payload identity/copy；依其 policy 删除/归档正文，不通过重复写入新 audit/excerpt 保留同样敏感内容 |
| credential / secret | 禁止进入 Evidence/Audit/Outbox/log；只能受控 secret 系统持有，处置报告不返回 secret。发现历史 secret 是泄露事件，先 containment/rotation，再专门处置，不能用改 hash 掩盖 |
| hash / URI / actor reference | 可被字典猜测、关联或携带身份；不自动匿名，不公开全部 URI、digest 或外部作者映射。对未能保留的证明字段也明确未知，不能编造匿名证明 |

证明保留与正文保留依据分别记录；正文有 hold/material pin 则不删除，policy 冲突由授权责任人裁决。最小 proof 也无法合法保留时，必须专门设计其处置与引用语义，当前首片不支持，不能“把所有历史删掉”。

### 8.2 与现有冻结格式的真实冲突和 migration 准入

当前 `evidence/hash.go` 的 `SHA256-EVIDENCE-V2` 覆盖 title、storage_uri、metadata、actor/time；`evidence/snapshot.go` 保存 manifest/hash payload，migration 000030 冻结 exact membership。`platform/audit/store.go` 允许 before_state/after_state/metadata。它们不是已实现的“无敏感最小 envelope”；若正文内嵌其中，直接删字段会破坏原 hash/immutable facts。这是 **open enforcement/model gap**，本文不把追加一条 disposition 当作内嵌正文已消失。

后续敏感正文片须先由 Evidence/Audit owner 接受以下单一目标 contract 并提交专门 migration：

1. 新写入仅保留经过 field allowlist 的最小 immutable envelope；业务主体/payload membership 为 typed relations，正文存受控独立 copy。新的 hash format 明确冻结 envelope、exact payload IDs/digests/算法和 membership，不把可删除正文复制到 manifest_hash_payload、Outbox、Audit 或成本 metadata。每个正文类有独立 retention/basis。
2. envelope/snapshot/hash/membership 仍 immutable；删除 payload 后追加 PayloadDisposition/availability receipt，不改旧 root、relation、digest。验签/重算 API 必须分别报告 ENVELOPE_VERIFIED、CONTENT_VERIFIED、CONTENT_UNAVAILABLE 或 MISMATCH；正文删后不能把“envelope hash 对得上”当原数据可重算。
3. migration 加强新 envelope 的字段边界、typed references、parent-first finalization/concurrency 和不可变 guards；明确 up、destructive down fail-closed、payload erase 授权及访问路径。无法细粒度删除的共享文件/共享密钥不准纳入正文 erase 支持范围。
4. 当前 pre-production、无真实稳定 consumer：开发/合成数据库可经另行环境授权重建为一套严格当前格式，保留 migration 安装历史；不为假想旧行增加双读、UNKNOWN 回填或永久 shim。#298 保留环境在本任务内不得重建。若实施时发现真实需保留的旧敏感事实，暂停该类删除，先冻结单独迁移/隔离/证明影响方案；不能静默修改旧 V2 hash 或迁移成“仍完整可验”。

密钥销毁不属于第一片。未来 crypto-erasure 仅能在已证明每个 deletion domain 的唯一密钥、全部包裹/复制/备份密钥清除、无残留明文/缓存且无其他合法共享引用时记录其验证范围；不得以删除 KMS alias 或撤销 caller 权限冒充销毁字节。复用成熟 KMS/envelope encryption，不自研密码系统。

## 9. 备份恢复与重新开放

恢复路径必须先关闭业务入口，在隔离 namespace/数据库执行。处置、restriction、hold、policy、rights/certification dispositions 与 copy inventory 的最小台账按独立受控备份/日志保留，不能只从正在恢复的旧业务备份读“当前”状态。

1. 备份责任人固定 backup identity/cut、水位、所含 copy/DB/WAL/缓存与恢复范围；冻结 RestoreOperation/current authority。latest watermark 来自独立当前台账；台账失联或缺 gap 时业务门禁全闭。
2. 恢复后先重放该 backup cut 之后的 restriction/disposition/hold/current rights/identity facts，验证无 gap；旧 projection/旧签发 credential/旧 queue reservation 不得直接生效，重建 generation 与过期/撤销状态。
3. 被销毁、过期或禁止重入库的 copy 继续 quarantine；按新受控 operation 清除恢复出来的 bytes（可能仍有 hold 冲突），不开放处理/交付。执行仍遵循 §6–7，不能把 restore sweep 当获准批量删除。
4. 可恢复的 exact bytes 重验 identity/长度/schema/hash；缓存/索引/预览从当前许可集合重建并带 current generation，排除禁止 subject。旧索引 backup 不直接服务；重入库 append new copy/availability facts。
5. fence 下 fresh 验证 restore 接纳；之后每次新 processing/delivery 仍按原 current gates，不用旧 backup 的 CERTIFIED 或 ALLOWED。内容/规则/认证依据变化需要原 owner 的新评测/认证/版本；纯同字节迁移不机械创建新 DatasetVersion 或 Certification。

备份中存在无法逐对象删除的内容时报告 retained backup scope、责任人、限制访问、最早可处置边界和未确认状态；介质/快照期满后经验证才补 completion。全域完成声明在没有验证这些范围前禁止。本文不实现完整灾备系统。

## 10. 当前缺口与有限后续

基线的 storage Store 是 Put/Get/PutImmutable，不是 lifecycle API；`dataset/application/ObjectStore` 只有 Put。deliveryfence 有 workspace row/revision；direct-data 返回 PayloadReady 后才可读取 storage，但没有本设计的持续传输 pin。已有队列/Outbox/reconcilers、Cost/Audit/Evidence 可复用；没有证据证明完整 copy inventory、保全、生命周期 reader admission、restore watermark 或敏感 envelope 已实现。

后续次序与检查归 [实施与验收](../product/data-lifecycle-design-acceptance.md)，所有 runtime scenarios 本轮为 NOT_RUN。#294/#295 的 Annotation admission 与暂停跨仓联调保持原 owner；#298 的接入代码/运行环境不改；外部 delivery/usage 仍归 #47/#84/#86/#87。真实对象、备份、正文/密钥处置、bucket 自动删除、部署、付费 provider 均需独立明确授权，不能从设计合并推导。

### 10.1 D1 封闭 processing / delivery fixture 的实施边界（2026-10-12）

未发布 D1/64 增加强类型 Execution / DeliveryOperation pin，以及独立 append-only transfer observation。封闭 composition 包装既有同步 NATIVE engine；在 engine 开始前，为全部 required execution input 提交 exact-copy pins，typed input Port 不接受 URI，实际 read handle 与 engine/output owner 停止后才追加 termination。未关闭 handle、原生停止未知、输出调用 UNKNOWN 或进程退出保留 pins；同 Execution 不重新打开。source restriction 与有限 use_until 在 admission、每次物理读取、输出发送和 READY publication 的共享 fence 下重新检查，派生输出沿冻结 producer inputs 重新检查当前限制与期限。

派生对象复用既有 Dataset Upload Command、MinIO OSS signer 与单次调用 journal；先持久化 typed output intent / physical call，再向两个配置固定槽位执行 create-only PUT，当前 input pins 与门禁仍成立且原生 exact version 返回后才在同一 publication transaction 登记 copy / READY。Unknown 不盲目重发。此最小业务事实模型不新增通用 storage、workflow 或 queue runtime；Reuse Check 的既有 OSS / Port 结论继续适用。

DirectDataService 的显式 transfer SPI 在原 ISSUED transaction 内提交 pin；封闭 HTTP owner 只在该 commit 后通过 ReadPinnedTo 读取，并在物理 GET 前重新执行真实 Certification / current entitlement gate。UseUntil 安装到实际 HTTP WriteDeadline，并同步 Flush。写入停止后追加 transfer observation / termination；ISSUED 不改写，accepted bytes 不证明客户端收到。same-key replay 0 新 payload / pin / native call。

仅 fixture executable 注入上述 SPI；公共 API / 常规 worker composition 未启用，生产 processing/delivery、全部 LC-11/12 disposal 竞争和全平台 closure 仍不声明通过。实际运行证据与环境清单见 [D1 processing/delivery 报告](../../examples/lifecycle-d1/results/326fedd2-8c22-4eb8-acca-05be57e15d77/plane-review.md)。旧数据库/原生环境不升级，不引入兼容双读。D1 仍占60–64，C2 仅在该候选之后应用65。
