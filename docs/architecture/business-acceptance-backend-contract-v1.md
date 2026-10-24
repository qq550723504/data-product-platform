# 业务签收验收与归档后端契约草案

状态：业务写入为 DESIGN ONLY。设计基线为 2026-10-24 的 `e448fa8fc98079b5d86d0fb9aeb4d71937cd9042`。用户随后授权第 0 切片只读实施，当前标准格式、查询与验收见 [第 0 切片记录](../product/governance-acceptance-s0-readonly-20261024.md)。其余业务 Command、表及写入契约仍为目标设计，不授权 migration、后端业务写入、模型调用、真实交付或执行。

## 业务事实与范围

业务签收回答“接收方确认收到了哪些成果”；业务验收回答“指定责任人是否按约定标准接受这一成果”；归档回答“当时依据哪些材料作出了什么决定”。三者分别保存事实，平台数据库是最终记录来源。

现有 DeliveryOperation、DatasetCertification、ProductRelease、EvidenceSnapshot 可作为明确引用。`ISSUED` 只证明交付授权释放，不能证明传输完成或接收方收到；认证、发布、证据快照均不能代替业务验收决定。业务验收也不授予新的下载、授权或执行能力，不修改上述对象。

V1 每个验收单只绑定一个明确的 DeliveryOperation 及其一个 DatasetVersion。服务端验证同工作空间、版本 membership、接收主体和历史交付事实；允许引用的交付须为 ISSUED。若交付涉及多个版本，只验收本单明确版本，不自动声称整次交付已验收。可选 ProductRelease 必须来自该交付的实际绑定。独立 DatasetVersion 交付无需包装成产品发布。

没有真实交付记录的演示成果继续保留在演示域；不能把 DEMO ID、BLOCKED 交付、名称匹配或最新版本转换成正式验收对象。无交付的线下验收、批量成果验收属于后续独立范围。

## 责任与标准

| 责任 | 服务端依据 | 可以办理的动作 |
|---|---|---|
| 提供方经办人 | 验收单指定的可信主体及工作空间权限 | 编辑草稿、提交、撤回、取消 |
| 接收方代表 | 指定接收组织的有效代表关系，且与交付的 effective consumer 一致 | 登记收到情况、追加签收补正 |
| 标准确认人 | 接收方指定的标准责任人 | 确认本单标准基线；不得由提交人代替 |
| 业务验收人 | 验收单指定的接收方验收主体 | 逐项核验、退回或通过；不得与本次提交人相同 |
| 归档经办人 | 明确归档权限及本单可见范围 | 冻结已通过且未作废的决定及其完整材料 |
| 决定处置人 | 明确的决定作废权限及本单业务责任范围 | 追加决定作废处置 |

这些是业务责任分工，不是新的登录系统。复用 `principal.Resolver`，ActorID、Subject、WorkspaceID 均来自服务端验证结果。页面演示身份、`X-Actor-ID`、body 中的 actor/consumer 或资产 owner 不能授予权限。权限判断同时检查能力和本单责任关系；现有 `HUMAN_DECISION` 不能被解释成所有业务动作的通行证。

当前 StaticResolver 是受控试点的单一主体配置，不能证明多个真实参与方的身份或代表关系。正式多方流程须由 Resolver Adapter 提供可信主体，并补齐服务端业务责任/代表关系映射；沿用能力地图的外部身份方向，不自建账号、密码、SSO 或通用 RBAC 引擎。授权关系的变更与命令提交须使用同一权威源的串行化或 revision 校验；只有请求入口的一次鉴权不足以证明提交时仍有权限。该保证是未来写入的准入条件。

V1 责任人和成果绑定在创建后固定。人员替换通过撤回、取消并新建关联验收单办理；待审单可由具有本单管理权限的责任管理员注明代办原因撤回，不能越过指定验收人直接通过。责任人动态移交属于后续显式 Command，不用通用 PATCH 补出。

标准来源首版仅接纳同工作空间的明确 **PUBLISHED ContractVersion**。保存 contractVersionId、sourceSha256、条款定位，以及接收方确认的逐项标准：稳定 criterionId、内容、required、核验方式和所需证据类别。使用现有合约版本，不另建通用标准库。标准确认事实绑定本次草稿内容哈希、确认人和责任分工；不得自我确认，且至少包含一项必需标准。

ContractVersion.Document 仍是通用文档。第 0 切片已实现有版本的 acceptanceCriteria 来源读取与完整格式校验；逐项强类型落库、冻结接纳和标准确认尚未实现。目标来源包含完整条目 ID 集合、条款定位、required、受支持 method 与 evidenceCategory，草稿必须覆盖该集合，不能只选较容易通过的条目。缺格式返回 UNSUPPORTED_SOURCE，不能靠自由文本、AI 推断或默认 required=false 补齐；需要新合约版本承载明确标准，不能给旧版本回填内容。

确认及提交时在事务内核对来源版本仍为 PUBLISHED；来源版本与本单按固定顺序加锁，避免确认/提交与来源状态变更竞态。提交后判断使用冻结来源，不因合约后来弃用而改写原材料。接收方的标准确认表示对本次精确标准集的业务认可，不宣称平台完成了整份合约的法律解释。

需求、邮件或双方约定目前没有统一的正式版本接纳能力。它们可作为背景证据，但在补齐“不可变内容、明确版本、接收方确认、同空间关联”的独立接纳契约前，不能作为 V1 唯一标准来源。页面现有可编辑来源文本继续只属于演示域。电子签名、附件上传、自动解析条款和新建合约不在本切片内。

草稿修改内容、来源、必需性或核验方式后，旧确认继续保留，新草稿必须重新确认；提供方不能删除合约要求、把 required 改为 false 或修改核验方式来绕过标准。已提交的标准始终取冻结提交，绝不读取当前草稿替换历史。V1 只支持明确人工核验及既有证据引用；需外部检测、法律签名或其他未支持方式的条目显示 UNSUPPORTED 并阻断提交。

## 领域对象

以下名称、关系和约束是目标设计，不表示已有表或 API。核心 ID、状态、责任、版本、作用域与成员关系使用强类型列及同工作空间约束；JSONB 仅存扩展展示信息。

| 对象 | 核心内容 | 可变性 |
|---|---|---|
| BusinessAcceptanceCase | workspace、明确交付/版本/接收组织、责任主体、status、revision、当前草稿/签收/待审提交/通过决定指针、可选 previousCaseId | 受 Command 控制的当前投影；成果和责任绑定固定 |
| AcceptanceDraftRevision | 单号、递增序号、合约版本、标准子项、内容哈希、创建人 | 每次保存生成新修订，旧修订不可改 |
| AcceptanceBasisConfirmation | 单号、draftRevisionId/hash、合约来源哈希、标准确认人和权限依据 | 追加事实；只对完全相同的草稿修订有效 |
| BusinessReceipt | caseId、交付/版本、接收组织、收到情况、说明、证据引用、登记人、可选 replacesReceiptId | 追加补正；明确当前指针，不按时间猜当前 |
| AcceptanceSubmission | caseId、序号、draftRevisionId、confirmationId、receiptId、完整冻结标准及证据成员、submittedBy、内容哈希 | 原子创建并冻结；撤回不修改原材料 |
| AcceptanceReviewDecision | submissionId、RETURNED/ACCEPTED、逐项结果、依据变化原因、说明、reviewedBy、reviewActivityId | 每次有效审核追加一个终结决定，原决定不可改 |
| AcceptanceTransition | caseId、from/to、submission/decision 引用、办理主体、原因和序号 | 追加流程历史，包括撤回和取消 |
| AcceptanceArchive | caseId、submissionId、acceptedDecisionId、冻结签收/标准/核验成员、EvidenceSnapshot 引用与清单哈希 | 一单一份；冻结后不修改或删除 |
| AcceptanceDecisionDisposition | 明确 acceptedDecisionId、INVALIDATED、原因、处置主体与记录时间 | 追加作废；不把原 ACCEPTED 改成 RETURNED |
| AcceptanceCommandReceipt | workspace、可信主体、command/target、幂等键、请求指纹、原结果 ID/revision | 原子保存命令处理回执；与业务收到情况不同 |

签收收到情况为 RECEIVED/PARTIAL/MISSING/DISPUTED。RECEIVED 是接收方的明确声明，必须有非空说明及至少一条同空间、匹配成果的持久化 Evidence 引用；不能由 ISSUED 自动生成。替换签收须指向该单当前 receipt，旧事实不被覆盖。待审期间可追加补正，但当前签收改变后阻断通过。ACCEPTED/CANCELLED 后关闭本单签收变更；后续发现错误通过决定作废及新单处理，不在历史单上补写签收。

## 状态和办理条件

| 当前状态 | 显式 Command | 必需条件 | 结果 |
|---|---|---|---|
| 无 | CreateBusinessAcceptance | 明确成果、交付及接收方匹配，责任齐全、提交人与验收人不同 | 新 DRAFT，revision=1 |
| DRAFT/RETURNED | ReviseAcceptanceDraft | 提供方经办人；expectedRevision 匹配；标准不削弱来源要求 | 追加草稿修订，旧标准确认失效，状态不变 |
| DRAFT/RETURNED | ConfirmAcceptanceBasis | 接收方标准责任人；明确草稿修订/hash 和条款映射全部可核验 | 追加确认，状态不变 |
| DRAFT/RETURNED/PENDING | RecordBusinessReceipt | 指定接收方代表；明确当前签收和本单 revision | 追加签收并推进 revision；PENDING 的旧提交保持原 receiptId |
| DRAFT/RETURNED | SubmitBusinessAcceptance | 当前签收为 RECEIVED；标准确认与当前草稿一致；材料完整可冻结 | 新 submission，转 PENDING |
| PENDING | WithdrawBusinessAcceptance | 提交人或本单责任管理员，明确当前 submissionId 和原因 | 追加撤回 transition，回 DRAFT；原提交保留 |
| PENDING | ReturnBusinessAcceptance | 指定验收人；明确当前 submission；至少一个必需项 FAIL/UNKNOWN 或签收依据变化，附原因 | 新 RETURNED 决定，转 RETURNED |
| PENDING | AcceptBusinessAcceptance | 指定验收人，非提交人；当前 receiptId 等于冻结 receiptId；所有必需项 PASS，每项有核验依据及说明 | 新 ACCEPTED 决定，转 ACCEPTED |
| DRAFT/RETURNED | CancelBusinessAcceptance | 提交人或本单责任管理员，非空原因 | 追加取消 transition，转 CANCELLED |
| ACCEPTED | ArchiveBusinessAcceptance | 决定有效、未归档，冻结链完整，归档权限有效 | 新 archive；业务状态仍为 ACCEPTED |
| ACCEPTED | InvalidateBusinessAcceptanceDecision | 明确决定且尚未作废，处置权限有效，原因完整 | 追加 disposition；原通过与档案保持原内容 |

不支持对任意状态执行通用 PATCH。ACCEPTED/CANCELLED 不可重新编辑、提交、退回或撤回。PENDING 必须先撤回/退回才能编辑标准或取消。一次 submission 的撤回、退回和通过互斥；重新提交生成新 ID 和序号。

判断结果只接纳冻结标准中的精确 criterionId 集合，每项恰好一次；未知、重复、遗漏或伪造 PASS 均拒绝。非必需项可以 FAIL/UNKNOWN，但必须记录结果与说明，不能默认丢弃。退回基于签收变化时可全部标准为 PASS，服务端须验证引用的旧/新 receiptId 确实不同。

`status=ACCEPTED` 保留当时通过的历史含义；查询同时返回 `decisionValidity=VALID/INVALIDATED` 和独立 `archiveState=NOT_ARCHIVED/ARCHIVED`。决定作废后不得新归档或复活旧单；重新验收新建单并明确关联 previousCaseId。已有档案继续展示当时的原决定及之后的处置时间线，不改写冻结字段。

V1 作废在事务提交时生效，不接受追溯或未来 effectiveAt。并发归档和作废按同一 case lock 排序：归档先提交则档案保留并显示后续作废；作废先提交则新归档被拒绝。交付、认证或权利的后续变化按标准规定展示风险或形成新的处置，不能静默翻转历史业务决定；重新取数仍由现有 CurrentDeliveryGate 单独判断。

## 拟议接口

下表为业务接入的目标接口；除第 0 切片已实现的 GET B/acceptance-context 外，**当前未注册**。合约版本列表及现有详情的身份/workspace 改造归第 0 切片记录。`B=/api/v1/workspaces/{workspaceId}`。实际请求拒绝未知/重复查询、非法 UUID 和未支持枚举。

| 方法与路径 | 用途与主要请求 |
|---|---|
| GET B/business-acceptances | 分页列表；支持明确 caseId、status、decisionValidity、archiveState |
| GET B/business-acceptances/{caseId} | 当前投影、revision、服务端可执行动作及逐项 blockers |
| GET B/business-acceptances/{caseId}/history | 分页 transition，精确 receipt/submission/decision/archive/disposition 引用 |
| GET B/business-acceptances/{caseId}/receipts | 该单签收历史；支持明确 receiptId |
| GET B/business-acceptances/{caseId}/submissions/{submissionId} | 精确冻结材料，拒绝跨单/跨空间关系 |
| GET B/business-archives 和 GET B/business-archives/{archiveId} | 档案列表与明确详情，原快照与当前处置分别返回 |
| GET B/acceptance-context | 明确 deliveryOperationId/datasetVersionId；返回可选标准、接收方/责任人资格和证据引用的安全投影 |
| POST B/business-acceptances | CreateBusinessAcceptance：交付/版本、接收组织和责任主体 |
| POST B/business-acceptances/{caseId}/draft-revisions | ReviseAcceptanceDraft：明确基于哪版草稿、条款映射及核验标准 |
| POST B/business-acceptances/{caseId}/basis-confirmations | ConfirmAcceptanceBasis：draftRevisionId、draftHash、confirmationReason |
| POST B/business-acceptances/{caseId}/receipts | RecordBusinessReceipt：outcome、replacesReceiptId、evidenceIds、reason |
| POST B/business-acceptances/{caseId}/submissions | SubmitBusinessAcceptance：draftRevisionId、confirmationId、receiptId |
| POST B/business-acceptances/{caseId}/submissions/{submissionId}/withdraw | WithdrawBusinessAcceptance：reason；管理员代办另记录授权依据 |
| POST B/business-acceptances/{caseId}/submissions/{submissionId}/decisions | 只接受 ReturnBusinessAcceptance 或 AcceptBusinessAcceptance 的独立验证分支：outcome、完整 results、reason |
| POST B/business-acceptances/{caseId}/cancel | CancelBusinessAcceptance：reason |
| POST B/business-acceptances/{caseId}/archives | ArchiveBusinessAcceptance：明确 submissionId、acceptedDecisionId |
| POST B/business-acceptances/{caseId}/decisions/{decisionId}/invalidate | InvalidateBusinessAcceptanceDecision：reason |
| GET B/business-acceptance-command-receipts | 用原 commandKind、targetId、requestKey 精确恢复当前调用主体的处理回执；CREATE 的 targetId 为空 |

所有 POST 使用 `Idempotency-Key`，除创建外必传 `expectedRevision`；服务端生成事实 ID、序号和时间。写返回 commandReceiptId、commandKind、caseId、相关事实 ID、resultingRevision，前端先确认回执与原请求绑定，再分别读取当前状态。查询可以返回当前 revision，不能把它覆盖到历史命令回执中。

错误区分：400 输入错误；401 未认证；403 无工作空间/动作权限；404 授权空间内对象不存在或关系不匹配；409 REVISION_CONFLICT、STATE_CONFLICT、IDEMPOTENCY_CONFLICT、BASIS_CHANGED；422 REQUIRED_CONDITIONS_NOT_MET/UNSUPPORTED_CRITERION。错误只提供安全字段和明确 blockers，不泄漏另一空间对象是否存在。服务不可用、超时或不完整响应在页面显示 UNKNOWN，不当成空列表或成功。

分页默认 limit=25、最大 100，offset 为非负整数；固定按序号或 createdAt,id 排序。列表与总数、详情与从属历史在同一 READ ONLY/REPEATABLE READ 快照中读取。查询参数中的对象 ID 不授予访问权限；历史深链不回退到 latest。

## 事务与不可变性

所有改变本单状态、草稿、签收、提交、判断、归档或处置的命令先取得同一 workspace-scoped case row lock，再校验 expectedRevision、当前责任权限和明确依赖。固定锁顺序为 case → 本次需要验证的来源 ContractVersion → submission → decision → archive → 从属成员；确认/提交对来源版本持有能与状态 UPDATE 互斥的行锁。跨入已有 EvidenceSnapshot 创建须遵守其锁契约，不能引入反向 case lock。锁内更新 case projection 并推进 revision；失败事务不留下部分成功。

冻结提交一次事务内复制完整标准、签收与证据身份，校验成员唯一性、workspace、来源/hash 和 required coverage，计算有版本的规范化内容哈希后 finalization。成员写入与 finalize 使用同一 parent lock；私有 BUILDING 阶段不得作为可恢复草稿对外暴露。判断结果、归档成员同样 parent-first 串行化，FINALIZED 后 header、成员的 INSERT/UPDATE/DELETE 全部拒绝。可变的 case projection 不安装全行 immutable guard。

唯一约束至少覆盖 `(workspace,case,submissionOrdinal)`、同 submission 一个有效终结决定、一单一个 archive、明确决定的一次 INVALIDATED 处置。撤回与决定的互斥由 case lock 和追加 transition 的唯一业务键共同保证；仅使用时间排序或 FINALIZED 后无锁检查 trigger 不足以证明并发安全。跨空间引用使用复合关联约束或等价 DB guard，不能只靠 UI UUID。

重复归档必须绑定同一明确 submission/decision。相同幂等键返回原回执；不同键但完全相同的既有归档请求，在校验当前权限及引用后返回原 archiveId，不新增档案、归档事件或成本。引用不同提交/决定则冲突。决定已作废时，已有档案通过 GET 查看，不用再次归档来读取历史。

幂等范围为 `(workspace, authenticated subject, commandKind, targetId, requestKey)`，绑定规范化完整请求指纹。同键同内容在重新验证调用者及结果读取权限后返回原回执，同键不同内容返回 409。恢复原回执先于新 revision 校验，不重复写入；当前事实由独立 GET 展示。CREATE 没有 case row 时，复用已有事务级 advisory lock 与唯一键模式串行化请求。复用 Agent 的处理模式，**不复用 Agent receipt 表或 Agent 状态机**。

签收补正与通过并发时，先提交补正则通过因 BASIS_CHANGED/revision 冲突失败；先提交通过则补正因终态被拒绝。标准修订与提交、撤回与通过、取消与提交也须用真实 PostgreSQL 双事务证明仅一个合法顺序生效。

## 事件证据与成本

关键事实与 transition 在同一业务事务追加 Domain Event、AuditEvent、Evidence 和命令回执；需要异步副作用时通过现有 Outbox。事件包括 CaseCreated、BasisConfirmed、ReceiptRecorded、Submitted、Withdrawn、Returned、Accepted、Cancelled、Archived、DecisionInvalidated，各事件带明确版本及对应业务事实 ID。生产者登记真实 routing obligation；当前没有通知消费者时明确 retention-only，不注册空 worker，不让未实现 handler 成为启动前置。

Archive 可复用现有 `evidence.CreateSnapshot` 保存冻结清单，同时保留独立 AcceptanceArchive 对明确业务决定的强类型关系。EvidenceSnapshot 不产生或替代 ACCEPTED。证据、审计与事件禁止 credential、下载 secret 和 provider_request_key。历史阅读仍受当前授权约束，冻结事实不等于永久可见。

人工核验活动沿用现有 CostEvent/CostAllocation。审核命令先分配稳定 reviewActivityId，V1 对实际登记的每次 RETURNED/ACCEPTED 人工核验记录 quantity=1、unit=review，金额未定可留空，不编造工时或费率。same-attempt 重放复用该 activity；退回后的再次真实审核使用新 activity。未来 migration 须增加指向实际业务审核主体的强类型 CostAllocation 关系，不能只塞 JSONB 或沿用整个 case 的幂等键吞掉多次成本。本方案没有外部 provider 调用。

## 复用评估

| 能力 | 仓库复用点 | 选择及边界 |
|---|---|---|
| 身份和权限入口 | platform/principal 的 Resolver；能力地图外部身份方向 | 扩展可信业务责任映射，保留 Adapter；不自建登录或第二套策略产品 |
| 事务、并发与命令恢复 | platform/transaction；agenttask/application 的 revision/receipt 模式 | 复用机制，新增本领域事实及约束；不改 Agent API |
| 合约来源 | contract/domain、contract/infrastructure；migration 000006 的历史内容 guard | 第 0 切片已补可信 workspace-scoped GET；后续接纳复用精确版本，不回退无范围读取 |
| 历史只读 | readmodel 的 delivery-history/evidence-archives 和 Trace | 复用身份、分页、同快照和脱敏模式，不以 EvidenceSnapshot 替代业务档案 |
| 审计与证据 | platform/audit、evidence、platform/outbox | 沿用现有 store、事务和冻结机制；不新增日志/证据库 |
| 人工成本 | cost 与既有 CostAllocation | 增加必要的强类型业务分配，不另建账本 |
| 流程运行 | 现有 Command/transaction/Outbox | 本切片为短事务人工办理；Temporal、通用审批引擎没有必要缺口，不引入平行 runtime |

本切片新增的是接收、验收和历史处置的产品语义，属于 Core。遵循 [能力地图](capability-map.md) 与 [ADR 0010](../adr/0010-reuse-commodity-capabilities.md)。本草案不引入新 OSS、SDK 或基础设施，不对候选 OSS 的部署状态作声明。

## 实施准入与验证

开发前必须先完成：合约标准读取的 principal/workspace 补齐；可信责任/接收方代表关系；标准条款到必需项的完整映射；业务 Case/Receipt 与既有交付、证据、成本的强类型边界。缺任一项时保持演示或只读，不开放正式写入口。

实施测试按 Domain → Application → Repository/真实 PostgreSQL → HTTP → Browser 的顺序。除正常闭环，还必须覆盖同键恢复、错误回执、身份撤销、跨空间及跨单引用、必需标准删改、receipt 补正竞争、撤回/取消竞争、成员 late commit、决定作废/归档竞争、作废后历史逐字段保持一致。浏览器须分别见证“真实 API 成功”“明确拒绝”“UNKNOWN 恢复”，不能用演示状态证明后端成功。

数据库验证仅使用另行授权的独立验收库。当前默认只读预览、原库、Agent API 与 dispatch/model/provider 开关均不因本设计改变。实施拆分和页面映射归 [业务后端接入计划](../product/governance-business-backend-plan-20261024.md)；当前页面演示契约仍归 [业务演示说明](../product/governance-business-acceptance-demo-20261024.md)。
