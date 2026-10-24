# 验收草稿与业务签收：可信身份及写入准入设计

> 2026-10-24 范围更新：用户已授权实施独立验收环境 1A。逐用户身份、责任预检和事务内本地权威校验的已实现范围及证据归 [1A 实施记录](../product/acceptance-identity-1a-implementation.md)。下文 Case/Draft/Confirmation/Receipt、业务写入及恢复仍是目标设计，不因 1A 实施而启用。

状态：**DESIGN ONLY，2026-10-24**。核对基线为 PR #363 的 `1141b72ce1af68194809f03997be3d94d7d4a287`。本次仅细化方案，不实施 IAM、DDL、API、页面或配置，不更新预览运行进程。保留原库和 Agent API，业务写入口继续关闭，不接模型、不派发真实执行，保持 Draft、不合并。

2026-10-24 后续设计补充基线 `73e6f3f1`：1A 与 1B 前置只读已经实施；企业来源仍未配置。本轮将可信依据、签收准备页面、异常验收和写入启用条件整理成文档，未开发或启用新能力。文档入口见[接入计划](../product/governance-business-backend-plan-20261024.md)；以下历史设计基线保留，不将旧缺口描述为当前已实现范围。

本文是 [业务验收总契约](business-acceptance-backend-contract-v1.md) 中第一切片的详细约束，拥有本切片的身份、准入、请求字段与恢复规则；总契约继续拥有后续提交、撤回、审核、归档及作废流程。已实现的责任与标准适用查询仍以 [只读契约](acceptance-binding-read-contract-v1.md) 为准。其中 1A 私有身份查询、有限 READ profile 和事务 Port 已在独立 host 实施；其余新业务接口与对象仍是目标设计，不能据此声称已接入或启用。

## 1. 交付范围与现状

第一切片只允许未来实现四个 Command：创建验收草稿、追加草稿修订、确认本次草稿标准、登记或补正业务签收。验收单停留在 DRAFT；“草稿准备齐全”是计算投影，不表示已提交、通过或归档。后续流程按钮不注册、不借演示 handler 代办。

| 已检查的能力 | 可以复用 | 仍缺的边界 |
|---|---|---|
| 当前 acceptance-context、ContractVersion 和 migration 74 | 精确成果、六类责任、代表关系、标准适用、完整标准来源与追加处置的只读事实 | SYNTHETIC_FIXTURE 不证明企业身份或真实签收；1A 读取 fence 已有，业务 Command 及其事务尚未实现 |
| 当前 principal.Resolver、quality-server.ts | 服务端身份入口、已有只读预览 | StaticResolver 与共享 HUMAN_DECISION_API_TOKEN 不能区分实际办理的多个人 |
| 独立 OIDC/RP 候选 | 外部 IAM Adapter、逐用户私有会话、当前身份映射、IAM 变更 fence 的路线 | 候选的身份/会话基础已由 1A 有界复用；新增独立 acceptance profile，其他候选模块未合并 |
| transaction、Evidence/Audit/Outbox、Agent 回执/CAS 模式 | 同事务记录、明确请求恢复、历史保留的机制 | 新增本领域对象和约束；不复用 Agent receipt 表、API 或状态机 |
| 新版业务演示页面 | 列表、草稿、签收及历史的页面职责和导航 | 演示角色与 sessionStorage 不是正式身份或业务事实，不迁移为 Core 数据 |

复用评估：账号、密码、MFA、SSO 继续交给企业 OIDC；[能力地图](capability-map.md) 的 ZITADEL/Keycloak 方向适用，不新造登录系统。已有有限能力映射与精确业务责任校验足够时，不再引入 Casbin/OPA；需要通用策略引擎时再独立评估。短事务办理复用现有事务与 Outbox，无需 Temporal、第二套队列或通用审批平台。Case、签收和责任准入属于平台业务语义。

候选代码的冻结检查依据是本机独立 checkout `C:/Users/Henry/.codex/worktrees/dpp-oidc-contract/data-product-platform`，提交 `fde81069d79c1189c9dae08515646eeac5e008d0`，主要文件为 `docs/architecture/identity-authentication-contract.md`、`browser-identity-candidate.md`、`entity-review-iam-contract.md` 及 `principal/oidc`、`principal/bindings`。原候选本身不是企业现场验证证明；本分支 1A 的有界复用以实施记录为准。候选使用成熟 ZITADEL OIDC Go 库（Apache-2.0）与 openid-client/pg（MIT）的 Adapter 方案；版本、漏洞、维护状态及运维配置须在将来选定集成提交时重新核验，不将旧版本号当成当前推荐。

**历史集成冲突及当前边界**：候选曾占用 migration 51，而本分支 51 已用于 ingestion_batch；1A 已通过增量 migrations 75/76 和统一模块基线完成有界集成，见 [1A 实施记录](../product/acceptance-identity-1a-implementation.md)。不再将候选集成列为当前未完成项；企业身份映射、真实依据接纳和业务写入仍未接入。后续不得直接 cherry-pick 旧迁移、改写已应用编号或给原库补跑。本轮不分配迁移号、不操作其他 checkout。

## 2. 身份信任链与办理权限

未来信任链为：企业 OIDC 人员登录 → 已有 RP/BFF 候选的逐用户私有会话 → 该用户的 API access token → Core Resolver → 平台 canonical actor/subject、当前 workspace grant → 明确代表关系与本单责任 → Command。浏览器只有 opaque 会话 cookie；token 不进入 React、localStorage、可读 cookie 或业务 JSON。

外部用户按 `(issuer, sub)` 显式映射到平台 ActorID 与 canonical Subject，邮箱、显示名和 IdP 角色 claim 均不能自动建立业务代表关系。OIDC 的稳定身份范围以及不同 client 的 pairwise subject 边界见 [OIDC Core §5.7](https://openid.net/specs/openid-connect-core-1_0.html#ClaimStability)。业务表中的 principalSubject 是平台 canonical Subject，不能直接拿外部 sub 比较。

沿用候选的成熟 Code + PKCE、state/nonce、固定回调白名单和服务端加密会话，按 [OAuth Security BCP](https://www.rfc-editor.org/rfc/rfc9700.html) 复核；API 使用 access token，不接收 ID token 冒充 API 凭据。候选未提供 refresh/offline_access，本切片按到期重新登录设计，不假定已有静默刷新。

| 层次 | 权威来源 | 拒绝的替代输入 |
|---|---|---|
| 人是谁 | Resolver 验证 token，并查询当前 actor、issuer/sub binding、workspace grant 及 revision/有效期 | DEMO actor、X-Actor-ID、body actor、共享读取账号 |
| 代表谁 | 明确 party_representation，canonical actor/subject、workspace、有效期和依据全部匹配 | 资产 owner、邮件域名、名称相似、IdP 组织 claim |
| 办理哪笔成果 | deliveryId + datasetVersionId + 精确 responsibility_binding | URL 选中空间、同组织、只具有 HUMAN_DECISION |
| 本单责任 | 创建时冻结的六个责任 binding ID；动作所需绑定仍为当前唯一有效绑定 | 新任责任人自动接管旧单、按最新时间选人 |

身份接入拟复用候选 current-principal 的固定 profile 模式，新增私有 `POST /internal/identity/business-acceptance-principal`（仅独立 1A host 已注册，只是当前资格查询，不保存业务事实）。只允许固定 TLS transport、该用户 Bearer 和 `{"workspace_id":"UUID"}`，body ≤4 KiB、拒绝未知/重复字段；不接收能力、actor、party 或 caller 指定的 profile。它要求本模块 READ，不复用 reviewer 的 HUMAN_DECISION 或 annotation-admin 能力。企业 access-token audience/client/scope 白名单仍由 IAM Adapter 契约固定。

拟议成功投影固定为 `profileVersion=business-acceptance.v1, principalId, canonicalSubject, workspaceId, issuer, subject, authorityRevision, capabilities, expiresAt`。issuer/subject 指已验证的外部 OIDC 身份，canonicalSubject 才是平台业务 subject；不得把候选七字段审核投影中的 subject 原样写进业务代表表。BFF 比对登录时 issuer/sub、固定空间和 canonical principal，pin 当前 authorityRevision；不同 profile/schema 或 revision 变化拒绝旧会话。投影仅供私有 BFF，浏览器只展示本人最小信息。expiresAt 沿用候选的 token/本地资格期限上限，不能把该投影当可缓存的业务写入 lease。身份缺失 401、无本模块 READ 403、依赖不可用 503，无静态凭据回退；该 profile 的实现和版本须由 IAM owner 集成审核。

| 内部 Port（目标接口） | 输入与输出边界 |
|---|---|
| HumanPrincipal Resolver Adapter | 服务器配置的 issuer/client/audience/scope + 人员 access token + workspace → canonical actor/subject、已验证外部身份、当前 capability/revision/期限；Domain 不依赖 OIDC SDK |
| Tx-scoped Authority Port | 同一个业务 tx + 可信 principal → 持 fence 后的当前 actor/binding/grant 强类型引用或复合键、revision、能力和期限；不接受浏览器填写的历史 authorityRevision 代替重读 |
| Acceptance responsibility Query | 同一 tx + 固定成果/case、所需角色和数据库判定时间 → 明确 binding/representation/适用依赖及 blockers；只拥有业务责任语义，不决定登录或 IdP 角色 |

外部 introspection 在进入持锁业务事务前完成，事务中不持锁等待外部 IdP 网络调用；本地权威的最后重读及外部检查点限制见第 4 节。

建议新增的有限能力为 `BUSINESS_ACCEPTANCE_READ`、`BUSINESS_ACCEPTANCE_DRAFT`、`BUSINESS_ACCEPTANCE_BASIS_CONFIRM`、`BUSINESS_RECEIPT_RECORD`。这些名字已进入独立1A验收profile的有限能力契约；企业IAM映射尚未接入，正式启用仍须由 IAM owner 核对受控 profile/映射及契约测试，不提供任意字符串权限编辑器。能力只决定模块准入，还必须满足下表的精确责任。

所有新写同时要求 READ 与对应动作能力，保证办理后的读取/恢复通道有明确准入；只有 READ 不能写。下表 DRAFT/BASIS_CONFIRM/RECEIPT_RECORD 为上述能力的简称，不是 Case 状态或 IdP 任意角色名。

默认 Core profile 保留已有 HUMAN_DECISION 读取准入；独立1A验收profile已使用 BUSINESS_ACCEPTANCE_READ 允许 acceptance-context、合约版本安全投影。1B前置再增加业务依据与材料安全引用，未建单目标必须匹配本人当前有效成果责任。后续 Case 仍须核验冻结参与人；可信收件材料接纳与业务写入尚未实现。所有读取复用 scoped repository，不用共享 token 代理，不将 READ 扩散到 Agent/其他 Core 读写路径。登录成功仍不构成业务写入准入。

| 动作 | 当前能力及本单角色 | 当前依赖校验 |
|---|---|---|
| 创建 | DRAFT + 目标唯一 PROVIDER，actor 与 canonical subject 均匹配 | 六类责任、代表关系和人员目录/workspace 资格齐全；标准适用 MATCHED；调用者不能代替接收方 |
| 修订 | DRAFT + 本单冻结 PROVIDER 仍为当前唯一有效绑定 | 新草稿来源及其适用确认仍匹配本单固定 BASIS_CONFIRMER；不削弱来源标准 |
| 确认依据 | BASIS_CONFIRM + 本单冻结 BASIS_CONFIRMER 仍有效 | 当前精确草稿与来源/hash；与提供方 actor、subject 均分离 |
| 登记/补正签收 | RECEIPT_RECORD + 本单冻结 RECEIVER 仍有效 | 代表的 party 与交付 effective consumer 相等；成果、原 receipt 和证据匹配 |
| 读取本单/历史/原回执 | 当前 READ + 本单冻结参与人身份 | 仍有当前 workspace 读取授权；跨单、跨空间、其他主体回执不可读取 |

创建时校验六名责任对应的目录身份和 workspace 资格，不收集其他人的 token。角色不要求六个不同的人，但 PROVIDER 与 BASIS_CONFIRMER、PROVIDER 与 REVIEWER 必须同时满足 actor 不同且 canonical subject 不同。接收、标准确认、验收角色的 party 均须等于交付接收主体；其余角色以有依据的目标责任事实为准，不推导法律所有权。

后续动作只重新校验该动作的依赖闭包，不能因为标准待修复就抹掉已经发生的收件声明。例如标准失效时，有效 RECEIVER 仍可登记 MISSING/DISPUTED；准备齐全投影则继续阻断。六类责任出现变更、缺失、多绑定或冲突时，不自动改本单的冻结责任。新建关联单可以引用 previousCaseId，但不会取消旧单；取消/代办须待第二切片的显式流程，第一切片没有“超级管理员改历史”恢复入口。

当前绑定判定复用 `contract/domain.EvaluateAcceptanceBindings`，输入是完整成果范围事实与本次数据库判定时间，不重建另一套“选有效人员”的算法。按现有语义，先计入责任事实自身处于 ACTIVE 的候选，再检查代表依赖；多个候选即 AMBIGUOUS，不能先过滤无效代表后挑剩余一人。写入仅再叠加本单固定 binding ID、可信身份来源及动作准入，分页或 isCurrentCaller 不能缩小候选集合。

业务代表关系撤销会阻断该角色的新业务动作；只要该冻结参与人仍有当前 READ/workspace 授权，可继续查看旧事实和自己的已提交命令回执。actor、身份映射或 workspace 读取权限撤销，则停止读取并清理页面缓存。替任人不自动获得旧单参与人读取资格；额外历史审计授权属于后续明确范围。

本切片仅允许真实人员自己办理，不接受机器 client、管理员 token 或任意 on-behalf-of consumer 代替人员。后续若支持代办，须独立定义服务端 delegation，不复用页面角色切换。

## 3. 业务事实、状态与历史

创建目标必须是同空间的明确 ISSUED DeliveryOperation 及其实际 DatasetVersion membership，接收主体由服务端绑定推导。ISSUED 只证明授权释放，不证明收到；BLOCKED、DEMO 或按名称/最新版本拼出的目标拒绝接纳。本切片只引用已有事实，不下载数据或签发凭据；今后再次取数仍走独立 CurrentDeliveryGate。

previousCaseId 若提供，必须是调用者可读取、同 workspace/接收主体且同一明确交付和版本的旧单；只建立追溯关联，不继承标准确认、签收、权限或验收结论。相同幂等键防止重复创建；不同键新建另一张单是独立显式意图，不宣称一笔交付只能有一个历史验收单。

| 事实 | 绑定内容 | 当前有效含义 |
|---|---|---|
| StandardApplicability（已有） | 明确交付/版本、ContractVersion、sourceSha256、完整 criteriaSetHash、原确认责任 | 该标准可适用于该成果；不能替代对某张草稿的确认 |
| DraftRevision（待实现） | 固定成果/责任上下文、title、来源/适用 ID/hash、完整标准集及提供方注记/证据、版本化 draftHash | 当前指针明确一版；每次保存追加，旧版不可改 |
| BasisConfirmation（待实现） | caseId、精确 draftRevisionId/hash、来源/完整标准集 hash、适用及确认责任/代表 ID、办理身份与原因 | 只对这版草稿有效；新修订即需重新确认，即使内容 hash 相同 |
| BusinessReceipt（待实现） | 明确 case/交付/版本/接收主体、outcome、原因、证据成员、接收责任/代表 ID、replacesReceiptId | 原事实追加保留；当前指针不能按 createdAt 猜测 |
| CommandReceipt（待实现） | 命令、可信主体、幂等键、请求指纹、原结果 ID/revision、办理依据 | 技术处理回执；与“收到成果”的 BusinessReceipt 分开 |

Create 原子产生 Case、DraftRevision #1、当前指针和 CommandReceipt，revision=1。首次草稿由服务器完整复制选定来源标准。Revise 可替换来源版本及适用关系，但成果与六类责任固定；新的适用确认必须仍绑定本单原 BASIS_CONFIRMER。提供方只能修改标题、逐项 providerNote 和证据引用；来源 `id,clause,text,required,method,evidenceCategory` 从现有 AcceptanceSource 完整重建，不新增 description 来源字段或改写原格式。请求的 criteria[].criterionId 精确引用来源 criteria[].id；集合必须相等，包括非必需项，且至少一项 required。

draftHash 使用有版本的本领域规范化方案，覆盖 case/固定成果/接收主体/六类责任引用、title、来源版本/适用 ID/sourceSha256/criteriaSetHash、全部标准/注记/证据成员；标准和成员按稳定 ID 排序，UUID 规范化，不把前端传入 hash 当权威。固定格式为 SHA256（UTF-8 前缀 `business-acceptance-draft.v1` + 一个 LF + JCS 完整 payload）的 64 位小写十六进制值。JCS 使用 [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785.html) 的既有实现：Go 已引入 github.com/cyberphone/json-canonicalization；Web 采用同协议成熟实现并验证跨端固定向量，不自造 JSON 排序/转义。该新 schema 不改变现有 sourceSha256/criteriaSetHash。每次保存生成新 Revision；旧确认只从当前投影退出，不 UPDATE 原确认事实。

签收结果为 RECEIVED/PARTIAL/MISSING/DISPUTED，全部要求非空原因。RECEIVED 另须至少一条已持久化、同 workspace、通过安全读取权限和明确成果关系校验的业务收到依据 Evidence；仅有质量报告、认证、ISSUED 或 EvidenceSnapshot 不足以自动证明收到。复用 Evidence/evidence_relation，Receipt 与 Evidence 之间新增强类型同空间成员约束；不靠 metadata 文本、StorageURI 或自填“材料编号”充当依据。PARTIAL/MISSING/DISPUTED 可无附件依据，但须明确说明，均阻断后续提交。附件上传、电子签章及自动验真不在本切片。

当前没有已验证的业务收件材料接纳入口；[1B前置只读核对](acceptance-business-basis-read-v1.md)仅提供安全引用、记录完整性和未配置缺口，不改变本条准入。Evidence owner 还须明确可选择的材料类别及受控来源，并提供能核对接收主体、交付/版本、原记录人和材料内容 hash 的安全投影/关系。缺少该路径时返回 422 REQUIRED_CONDITIONS_NOT_MET，blocker 为 `RECEIPT_EVIDENCE_UNAVAILABLE`，不开放 RECEIVED 正式登记；Command 自己新建的审计 Evidence 不能反过来充当其收到依据。独立测试可用明确合成材料验证，企业材料接纳仍是准入缺口，不以放宽 Evidence 条件填平。

### 3.1 签收契约补充：可信依据与准备闭环

可信依据的必需内容和来源职责统一归 [T01–T12 清单](business-receipt-trusted-basis-v1.md)，不在各页面或 Command 中各自实现一套“可信”算法。当前目录、代表责任、标准权威源及材料来源均 UNCONFIGURED；新写入口一律关闭，包括无附件的 MISSING/DISPUTED，不因其较易满足而提前开放。

未来 `RecordBusinessReceipt` 仍只有第 5 节现有请求字段；客户端不能提交接收主体、可信来源、核验时点或验证结论。第 4/6 节原回执分支优先；仅无已提交回执的新写依次核对当前本人/冻结接收责任、明确成果及当前原签收，再按 outcome 和引用用途，由 Evidence owner 的来源接纳 Port 核对精确 evidenceIds。收到证明须满足 T07–T10；背景或争议材料不自动升级为收到证明。提交的每个引用均须有权读取、目标匹配且满足其用途，不能丢弃无效引用后把剩余材料当整次成功。

| outcome | 额外准入与准备投影 | 写入后含义 |
|---|---|---|
| RECEIVED | 非空原因、至少一条受控收到依据；材料出处、内容身份、接收主体和接纳当前有效性均明确可用 | 有权接收代表作出的收到声明；不自动验收通过或产生新的交付权利 |
| PARTIAL | 非空原因，可无附件；有引用时仍逐一核验权限/范围/用途 | 部分收到，准备继续阻断 |
| MISSING | 非空原因，可无附件；自身身份、代表/责任及目标仍有效 | 未收到声明，准备继续阻断 |
| DISPUTED | 非空原因，可无附件；有引用时仍逐一核验权限/范围/用途 | 争议声明，准备继续阻断 |

标准当前失效或本单确认待补时，不增加“先确认标准才能登记未收到”的硬依赖；未来已启用环境仍允许满足其自身依赖的 MISSING/DISPUTED。来源条件必须按本次动作和提交材料检查，不把所有上游来源全部可用变成所有动作的固定门禁；全局写入关闭与动作 eligibility 始终分开。

历史记录、当前准备条件和来源配置分别处理：

- 本单签收冻结明确材料/内容/接纳引用及办理依据；Receipt header 与全部材料成员同事务追加，创建后不可原地变更。底层具体强类型接纳事实由实际来源确定后定稿，不把 source URL/metadata 当关系模型。
- 可信内容变化须新内容身份及新接纳依据；新收到声明通过补正追加。原签收与将来已有 submission/archive 的旧引用不变，不自动转换 outcome 或移动 current pointer。
- 当前签收冻结的依据被撤销、过期或当前不可核对时，准备情况阻断并显示具体缺口；保留当时签收依据。已补正退出当前指针的旧签收，其专属材料失效不自动阻断无此依赖的新签收。历史回执恢复仍按第 6 节的当前 READ 和同主体规则，不因来源变化重做已提交业务动作。
- 当前 Evidence envelope hash 只校验平台记录；不能据此重建附件 hash、证明企业授权或自动填 RECEIVED。`CORE_CONFIRMED` 来源标记也不替代企业接纳。

未来可变本地接纳有效性必须参与第 4 节最终重读及共享依赖 fence；来源检查只在别的连接做一次预检不足以授权写入。外部检查点与本地串行化的边界归可信依据清单，具体来源/处置路径或撤销窗口未定稿则不启用。本轮不新增锁服务、来源注册中心或接纳写接口。

新写首次签收显式传 `replacesReceiptId=null`；新补正必须等于当前 receiptId，并提供 expectedRevision。并发补正最多一个成功。RECEIVED 改为 DISPUTED 合法，旧收到声明仍保留，准备齐全投影随之阻断。仅签收变化不改草稿 hash 或原标准确认；未来冻结提交仍精确引用当时 receiptId。已提交原键重放不再要求其旧 replacesReceiptId 或 resultingRevision 等于当前指针/revision，也不再次移动当前指针。

| 当前状态 | 本切片动作 | 状态结果 | 追加事实/版本 |
|---|---|---|---|
| 无 | CreateBusinessAcceptance | DRAFT | Case + DraftRevision #1，revision=1 |
| DRAFT | ReviseAcceptanceDraft | DRAFT | 新草稿修订，revision+1，旧确认不再匹配 |
| DRAFT | ConfirmAcceptanceBasis | DRAFT | 精确确认，revision+1；同一草稿/同一确认责任已有确认时，不同键重复请求返回冲突并引导 GET |
| DRAFT | RecordBusinessReceipt | DRAFT | 新签收及当前指针，revision+1 |
| 其他状态 | 上述新增写请求 | 拒绝 | 本切片不借用后续状态或演示实现 |

查询返回 `preparation={complete, blockers, evaluatedAt}`，不新增 Case READY 状态。complete 需要所有冻结责任当前有效、标准/来源有效、当前草稿有有效确认、当前签收为 RECEIVED 且证据满足。它不提供下一切片的提交授权；当前实际只读页面仍为 writeEnabled=false。责任或标准失效只改变当前 blockers，不能改写原草稿、确认、签收与办理依据。

这里的确认和材料只取 Case 显式当前草稿/签收的依赖闭包；历史查询独立展示旧事实及来源变化，不把历史成员并入准备判定。创建只核验其自身依赖，不要求尚未产生的 BasisConfirmation/BusinessReceipt；确认与签收由后续各自 Command 产生，不能作为来源只读见证或建单的前置产物。

标准确认和收到登记可在建单后分别办理，没有“先签收才能确认标准”的硬依赖；只有修改草稿才要求重新确认。两者都齐全也不会在第一切片创建 Submission 或 AcceptanceDecision。

## 4. 写入的事务准入与并发保证

**候选历史缺口（1A 本地 Port 已有界补齐，见实施记录）**：原 OIDC 候选在请求入口查询当前身份；其 Entity Review 契约没有将 grant 撤销和已进入的业务事务串行化。原绑定 GET 的 READ ONLY/REPEATABLE READ 时点也只是读取快照。独立 1A/1B 读取已复用本地 fence 与 READ COMMITTED 重读；已有读取能力不直接成为业务写入授权。

未来业务 Command 复用 1A 已有、由 IAM owner 维护的 **Tx-scoped Authority Port**：在验收事务的同一个 pgx.Tx/连接中取得既有 IAM mutation fence 的兼容锁，并重读 actor、issuer/sub binding、workspace grant、capability、revision/期限；不得复制私有 advisory key，或在另一条连接持锁就声称同事务。业务责任 owner 维护已有 workspace 级 binding fence，所有新增代表/责任/适用事实和 disposition 的受控写路径均须参与。存在可绕过 fence 的运行期写路径时不开放本切片；本轮不重新实现这些 Port。

固定顺序为：**IAM authority fence → workspace acceptance-binding fence → scoped idempotency lock → case row → 本次来源 ContractVersion → 本领域从属成员**。验收命令对前两个 fence 持共享锁，IAM/绑定事实变更持各自排他锁；若一个变更同时需要二者，严格同序。复用现有事务/advisory lock 基础设施，不新增锁服务。后续 submission/decision/archive 仍在 case 之后 parent-first。ContractVersion 锁与其状态 UPDATE 互斥；证据接入不得反向取得另一 case lock。

事务按 READ COMMITTED 读写；取得 fence 后才读取当前本地权限、责任和依赖，不能复用入口快照。第一次查同主体幂等回执在新业务 revision/状态校验之前：已提交请求走历史重放读取；新请求才取得 case/current revision 并完整准入。创建时先用幂等锁串行化，没有 case 行也不能双建。

HTTP 入口先以本模块 READ 解析可信人员，不在查回执前要求 DRAFT/BASIS_CONFIRM/RECEIPT_RECORD。持 IAM fence 后重读当前 READ；仅本人、同空间、命令/target/key/指纹匹配且结果可读的 COMMITTED 请求可直接返回原回执。没有匹配回执的新请求，再在相同事务锁内要求动作能力、当前责任和所有业务依赖。读权限丧失时所有分支拒绝；写权限丧失时，已提交请求仍可恢复，未提交请求不能借原键获得新写权限。同键异内容仍为冲突，不把旧成功结果套给新 body。

该优先级也覆盖当前 case 非 DRAFT、原 expectedRevision/replacesReceiptId 已陈旧、标准/材料失效或来源服务不可用的情况：同主体当前仍可读的已提交结果保持原回执，不再按新写条件拒绝。外部材料候选核验仍在持业务锁前完成，但失败结果只由无原回执的新写分支消费，不得在幂等恢复之前提前终结；业务事务内仍先在原键锁下重查是否已有 COMMITTED，再消费候选并执行最终准入。这里不要求持锁发外部请求，也不新增平行恢复服务。全局写入口关闭时不分发任何 POST，已经可用的受权回执 GET 可继续恢复；本轮尚无该业务 GET，实现前不伪造回执。

所有新写在持锁后、追加事实前执行最后一次权威决策，使用**新的数据库 clock_timestamp** 重查有效期及已生效 disposition，并核对固定依赖和 revision；禁止用事务开始时间、浏览器 asOf 或预检 evaluatedAt。最后受锁决策是本地时间资格的判定点；不声称真实时钟到期与物理 commit 无时间间隔。本地撤销/事实变更则由同 fence 排序：撤销先提交就拒绝，业务先提交则保留其当时依据。锁等待超时、死锁或连接中断没有授权兜底或自动 Command 重试。

业务事实、当前指针、revision、不可变办理依据、Domain Event、AuditEvent、Evidence、CommandReceipt 同事务提交；当前无外部副作用。办理依据以强类型关系保存 actor/issuer binding、workspace grant revision、capability、责任/代表/适用 IDs、标准/hash、evaluatedAt 及最早有限期限，禁止只写普通日志或 token 原文。历史依据证明当时判断，不能作为下一次授权 lease。

第一切片事件固定为 CaseCreated、DraftRevised、BasisConfirmed、ReceiptRecorded，均带本领域 event schema 版本和精确事实 ID；补正使用 ReceiptRecorded 的 replacesReceiptId 解释关系，不覆盖旧事件。复用现有 Outbox 的 retention-only 路由，不注册通知或执行消费者，不因保存草稿虚构人工审核成本；后续实际审核的 CostEvent/CostAllocation 仍归总契约。

**外部撤销的保证边界**：成熟 Adapter 在每次请求实时 introspection，失败或不可用时拒绝新写，不缓存成功鉴权；但 HTTP introspection 与 PostgreSQL 无分布式原子提交，不能保证 IdP 在返回后发生的撤销立刻中断已进入事务的命令。[RFC 7662](https://www.rfc-editor.org/rfc/rfc7662) 说明 introspection/cache 的时效边界。拟议模式明确为 `EXTERNAL_CHECK_POINT`：外部身份资格以此次有效 introspection 为检查点，本地权威以受锁决策为准；写入开放前必须在身份接入契约中明确接受该撤销窗口和有效期策略。若要求“IdP 撤销先于 DB commit 则必拒”，本方案尚不满足，保持关闭，不能用“无缓存”冒充该保证。

未来实现还须证明：actor/grant 撤销、责任处置/替换、适用处置/未来生效、合约状态变化、并发补正、相同键创建与草稿修订竞争均遵守上述顺序。不可变 Revision/Confirmation/Receipt 的 header 与成员同事务生成，创建后拒绝 INSERT/UPDATE/DELETE 改变成员；若实现引入 BUILDING/finalize，必须共享 parent lock 并锁内校验完整 hash。Case 当前投影允许受控更新，不能加全行 immutable guard。

## 5. 拟议接口规格

`B=/api/v1/workspaces/{workspaceId}`。除现有 GET B/acceptance-context 外，本节均 **NOT_REGISTERED**。不改 Agent API，也不扩展现有读取 token 的业务写权限。

| 接口 | 请求/返回的关键字段 | 约束 |
|---|---|---|
| GET B/acceptance-context | 现有精确 deliveryOperationId/datasetVersionId，可选 contractVersionId | 将来复用为选择器，增加当前人员资格投影；GET 不是办理授权，不另建平行 preflight 服务 |
| GET B/business-acceptances | 分页 caseId/status 筛选及本人可见列表 | 仅授权参与单；固定分页/排序，不能按 guessed ID 跨空间 |
| GET B/business-acceptances/{caseId} | 当前 revision、精确草稿/签收/确认 ID、preparation、动作 blockers | writeDeploymentEnabled 与业务 eligibility 分开；不把旧 GET 动作当授权 |
| GET B/business-acceptances/{caseId}/draft-revisions/{draftRevisionId} | 精确完整修订及 hash | 从属关系校验，无 latest 回退 |
| GET B/business-acceptances/{caseId}/basis-confirmations/{confirmationId} | 原草稿及办理依据、当前是否仍适用 | 不改写历史确认状态 |
| GET B/business-acceptances/{caseId}/receipts | 分页历史，可选明确 receiptId | 历史与当前指针分列；补正链可精确追踪 |
| GET B/business-acceptances/{caseId}/history | 追加办理历史及精确关联 ID | 第一切片只返回实际存在的四类动作 |
| POST B/business-acceptances | title、deliveryOperationId、datasetVersionId、contractVersionId、applicabilityBindingId、responsibilityBindings[{role,bindingId}]、可选 previousCaseId | 六类责任各一次；服务端推导 party/人员，生成首版草稿 |
| POST B/business-acceptances/{caseId}/draft-revisions | expectedRevision、baseDraftRevisionId、title、contractVersionId、applicabilityBindingId、criteria[{criterionId,providerNote,evidenceIds}] | 全量标准 ID 集合；注记可为空，证据去重；来源字段从服务器重建 |
| POST B/business-acceptances/{caseId}/basis-confirmations | expectedRevision、draftRevisionId、draftHash、applicabilityBindingId、confirmationReason | 必须当前草稿、当前适用及本单原确认责任，非空原因 |
| POST B/business-acceptances/{caseId}/receipts | expectedRevision、replacesReceiptId、outcome、reason、evidenceIds | replacesReceiptId 必传，首次为 null；补正为精确当前 ID |
| GET B/business-acceptance-command-receipts | commandKind、targetId、requestKey、requestFingerprint | 仅当前主体，CREATE 无 targetId；精确查找，不接收 subject 参数 |

所有业务 Command POST 带 UUID `Idempotency-Key`；除 CREATE 外 expectedRevision 为 1 至 2^53−1 的整数。workspace、actor、case、delivery、DatasetVersion、ContractVersion、责任/代表/适用 binding、草稿、确认、签收及 Evidence 等平台对象 ID 使用非 nil UUID；**criterionId 不是 UUID**，沿用来源 id 的业务编码规则 `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`，大小写敏感且必须精确匹配完整来源集合，不创建编码别名。role/outcome 为受支持枚举，canonicalSubject 与 partyRef 保持权威源中的主体字符串语义。

title 1–200 Unicode code point，原因 1–2000 code point；criteria 沿用来源 1–100 项限额，每条 providerNote ≤2000 code point，单次 Evidence 引用总数 ≤100、去重；JSON body 最大 64 KiB。来源 clause/text 的现有 UTF-8 字节限额、required、method 和 evidenceCategory 验证保持不变，不能套用新增注记的字符限额弱化来源规则。不接收 actor、consumer、status、时间或自定结果 ID；未知/重复字段、重复标准 ID、重复角色、无效 Unicode 拒绝。JSON 超限为 413，过期/错误会话不能降级共享凭据。上述限额为待实现的严格当前 contract，无双读兼容分支。

commandKind 仅接受本切片四个 Command 的全名：CreateBusinessAcceptance、ReviseAcceptanceDraft、ConfirmAcceptanceBasis、RecordBusinessReceipt；文中 CREATE 只是创建的简称。requestFingerprint 为 SHA256（UTF-8 前缀 `business-acceptance-request.v1` + 一个 LF + JCS `{workspaceId,actorId,canonicalSubject,commandKind,targetId,body}`）的 64 位小写十六进制值，CREATE 的 targetId=null。外层身份/路径 UUID 来自可信上下文且规范化；body 保留已验证原 JSON 值、显式 null 和数组顺序，不静默 trim、填默认值或 Unicode normalization。服务端独立计算，不能以客户端值替代；Web/Go 使用同协议和固定向量。

成功返回：`state=COMMITTED, commandReceiptId, commandKind, requestKey, requestFingerprint, actorId, canonicalSubject, workspaceId, caseId, resultingRevision, factIds, authorityDecisionId, evaluatedAt`，只投影本人身份，不返回外部 token/sub 或其他人的目录信息。CREATE 首次 201，其余成功及历史重放 200；重放返回原 revision，不伪装成当前 revision。前端须核验主体上下文、命令、原键/指纹、case/事实关联与字段形状后，才能标记“已保存”；详情另走 GET。

factIds 逐命令固定：创建/修订为 `{draftRevisionId}`，确认是 `{confirmationId,draftRevisionId}`，签收是 `{receiptId,replacesReceiptId}`（首次后者为 null）。不接受其他形状或关联到不同 case 的事实。回执 GET 返回同一 COMMITTED 结构；未提交事务不产生 PENDING 回执行，BFF 的未决发送状态与 Core 业务/回执状态分开。

| 结果 | 页面和恢复含义 |
|---|---|
| 400/413 | 输入/重复字段/大小不符，已确认未分发的新请求可修正；不能清除更早 UNKNOWN 请求 |
| 401/403 | 未认证/无当前能力；身份或模块准入失败在对象存在性查询前拒绝 |
| 404 | 已授权空间内精确对象/从属关系未命中；不暴露外空间对象，不回退 latest |
| 409 REVISION_CONFLICT/STATE_CONFLICT/BASIS_CHANGED/IDEMPOTENCY_CONFLICT | GET 当前事实并展示差异；不覆盖 CAS、不自动套入新 revision 重发 |
| 422 REQUIRED_CONDITIONS_NOT_MET/UNSUPPORTED_CRITERION | 返回安全的逐项 blockers，无部分业务保存 |
| 503 AUTHORITY_UNAVAILABLE | 无新的授权依据，不落业务事实；如以前已有未决请求，仍须恢复其原结果 |
| 503 SOURCE_UNAVAILABLE（未来来源 Adapter） | 本动作必需的权威来源不可用，无法产生新的可用结论；当前请求不得写入；不能据此断言更早 UNKNOWN 请求未提交 |
| 网络中断、无匹配回执、畸形响应或不确定 5xx | UNKNOWN；即使响应写“失败”也不能凭 HTTP 状态推断 DB 未提交 |

## 6. 幂等、断线与异常恢复

幂等命名域固定为 `(workspaceId, canonicalActorId, canonicalSubject, commandKind, targetId/null, requestKey)`，绑定完整规范化请求指纹，包括 expectedRevision、显式 null、原 receipt 和草稿版本。相同键不同内容 409；同键同内容在重新认证及校验当前读取资格后返回原不可变回执，不再次检查新业务 revision，也不重复事件、证据或事实。角色写权限丧失但仍有读取资格的人可以恢复自己的原成功记录，不能借此执行新写。

BFF 将待发意图的原键、完整 body、指纹、canonical actor/workspace 和发送状态存入受保护的服务端恢复记录，复用候选的私有加密存储模式。scope、命令/target、原键和指纹为可查询字段，原 body 加密保存；不将核心业务关系只藏进 JSONB。记录与短期 RP 会话生命周期分开：登出销毁 token/会话，不抹掉未决请求；重新登录同人/同空间后经当前授权查回其待恢复记录，不复用旧 token。

原意图必须在向 Core 分发前持久化，失败则零分发；开始网络发送前先记录“可能已发送”，进程中断也不能当作未发送。Core 成功与 BFF 标记完成不在同一事务，后者失败时始终以 Core 原回执恢复。

浏览器只持不透明 recoveryHandle；不在 sessionStorage 写入 token、责任个人信息或完整业务材料，handle 本身也不授予读取权限。未决记录不因页面刷新或会话到期自动清除；确认结果后关联 Core 不可变回执，原材料清理依照后续明确的保留策略，不能用清理动作把 UNKNOWN 伪装成失败。不增加通用任务队列。恢复记录不构成 Core 的业务真相；其持久化失败、登出重登及丢失恢复均须测试，当前尚未实现。

| 异常/阶段 | 唯一允许的恢复路径 |
|---|---|
| 发送前被本地校验/关闭开关拒绝，且从未分发 | 保留表单，可重新检查；不声称发生 Core 写入 |
| 已分发或是否分发未知 | 锁定该意图的新写；保留原键/body/指纹，只读查询原 CommandReceipt，不自动 POST |
| GET 找到匹配的 COMMITTED 回执 | 固定原 case/事实 ID 与原 revision；再独立读详情，成功后结束未决状态 |
| 回执已确认，详情 GET 失败 | 显示“命令已确认，当前详情未知”；保留成功事实，只读刷新，不重新创建或改键 |
| GET 回执 404 | 显示 NOT_OBSERVED；可能原事务仍在途，不能称作未执行，不允许换键补建 |
| 持续 NOT_OBSERVED，需要继续原请求 | 未来仅提供明确的“恢复原请求”动作：先展示可信恢复记录中的原意图，只有写运行边界已另行授权且开放、本人/空间与原意图一致并重新获当前资格时，才发送**相同键、逐字段相同 body**。Core 原键锁下先重查 COMMITTED，否则按本次新写完整准入；仍陈旧的 revision/责任/来源不自动替换。它是原四个 Command 的原意图恢复，不是第五个 Command、后台重试或只读操作。当前关闭，禁止分发 |
| 会话到期或权限撤销 | 重新登录同一 canonical actor/workspace 后先只读恢复；新身份、替任人或新空间不得接管未决请求 |
| 保护记录丢失或原 body/指纹不可核实 | 不伪造新键/新 body 重试；只读查本人历史并核对明确回执，未能关联则保留 UNKNOWN，人工核对后才允许新的独立业务意图 |
| revision、标准或责任变化 | 展示原事实与当前 blockers；已确定失败的新请求可在用户重新核对后生成新意图；原 UNKNOWN 先结清，不悄悄更新 body |
| 服务重启/commit 后响应丢失 | Core 回执与业务事实同事务；重新 GET 恢复原提交，不能依赖 BFF 内存确认成功 |

已经存在 UNKNOWN 的意图，后来获得 401/403/422 等拒绝也不能把先前结果改成“未提交”。服务端只有能证明该次事务未提交的明确拒绝才可终结该次尝试；网络层无法证明时持续 UNKNOWN。重复点击、双窗口、同键并发均必须由服务器幂等约束兜底。

## 7. 新版页面与人员办理顺序

未来 Case 业务页面复用新版侧栏的“业务验收”“业务签收”“业务归档”和既有 Core 壳，不增加重复菜单。正式路由与 `/#business-*` 演示域分离；这些业务表单尚未实现。1A 实现身份/工作空间和责任预检，1B 前置补齐业务依据/材料只读核对；新的[页面流程稿](../product/business-receipt-page-flow-20261024.md)复用这些完整流程，不改变现有 GET 或演示存储。

| 页面/顺序 | 主体与唯一主要动作 | 返回、刷新和阻断 |
|---|---|---|
| 验收列表 → 新建独立页 | 提供方选择明确交付/版本，再核对六类真实责任、标准/来源、依据；主要动作“创建草稿” | 列表只选择和浏览；预检不通过显示具体缺口，不在列表里混入完整表单 |
| `/core/business-acceptances/{caseId}` 草稿详情/编辑 | 提供方核对当前修订，修改允许字段并“保存新修订” | 保存后展示新 revision/hash 和“待接收方确认”；取消编辑回原详情，不改事实 |
| 同单标准确认页 | 指定标准确认人查看完整来源/条款、草稿差异及本人代表依据，“确认本次草稿标准” | 显示精确 draftRevisionId；旧窗口确认冲突后返回当前单，不静默换版本确认 |
| `/core/business-receipts` → 明确 case 的签收详情 | 接收代表先看当前收到情况及历史，再进入独立登记/补正表单，“保存签收记录” | 补正页明确显示替换哪个 receiptId；历史详情可回原验收单，保留列表筛选/页码 |
| 草稿准备情况 | 展示当前签收、标准确认、责任和阻断项，主要动作按本人角色指向缺失材料 | complete 也只显示“草稿材料齐全”；提交/审核/归档仍未接入，无“已通过”提示 |
| 历史/消息回跳 | 明确 workspaceId + caseId + receipt/draft/confirmation ID | 每次重新鉴权及核实从属关系；缺失/越界明确拒绝，不回退最新，不隐式办理 |

人随企业登录确定，页面不提供“切换为验收人”代办。切换 workspace、账号、case 时，清理未保存字段、证据选择和旧动作资格；按上下文 epoch 丢弃迟到响应，旧表单不得写到新空间。UNKNOWN 原请求留在其服务端主体/空间恢复记录，不转移到新表单。Core 查询失败只将该区域标记 UNKNOWN，已知兄弟事实保留；失去身份读取资格则不继续展示旧缓存。

未来 acceptance 专用 BFF profile 只能将该用户 token 转给允许的业务路径，严格同源与 CSRF 校验；cookie 使用 Secure/HttpOnly/适当 SameSite，鉴权/业务响应 no-store。不能在 OIDC 失败、超时或配置缺失时回落到 HUMAN_DECISION_API_TOKEN、POC actor 或 demo handler。现有 Agent API/只读预览的身份与路由保持独立，不因接入新 profile 自动改动。

## 8. 实施前门禁、责任归属与验证清单

下表是未来准入工作，不是本次开发任务或启用授权。

后续详细核对使用[异常验收矩阵 R01–R48](../product/business-receipt-exception-acceptance-matrix-20261024.md)和[写入启用清单 G0–G8](../product/business-receipt-write-enablement-checklist-20261024.md)。它们是本文的验证/启用视图，不另定义 Command 状态机；全部新写入与真实来源现场用例仍 NOT_RUN，业务写入仍 NOT_AUTHORIZED。

| 门禁/责任 owner | 必须提供的证据 | 当前判定 |
|---|---|---|
| IAM / RP owner | 选定集成提交、迁移/模块冲突解决；本模块有限能力、逐用户 token、有效主体映射；外部撤销窗口契约 | 1A 独立实现及合成多人验收见实施记录；真实企业多人登录 NOT_RUN |
| IAM + 责任事实 owner | Tx-scoped Port、共享 fence 顺序；所有身份/绑定/处置写路径受控；到期和未来处置时点测试 | 1A 本地身份/责任 Port 及真实 PG fence 已实现；Case 写入及远端原子撤销不在其中 |
| 业务/目录责任方 | 六类责任的可信来源、代表依据、人员可联系性和职责分离；真实标准适用确认 | 当前只有 SYNTHETIC_FIXTURE；不准正式写入 |
| Acceptance owner | Case/Draft/Confirmation/Receipt 及强类型关联、完整 hash、显式 Command、独立回执与不可变历史 | DESIGN ONLY |
| Evidence / Audit / Outbox owner | 可验证的收件材料接纳来源/类别、强类型证据关系、同事务事实/审计/事件；当前 retention-only 无通知消费者 | 复用点已确认；业务收件材料接纳及新关联/事件未实现 |
| Web owner | 单用户 BFF、CSRF、恢复记录、明确页面职责和跨空间隔离；无共享凭据 fallback | DESIGN ONLY |
| 验收 owner | 独立验收库的 PG/HTTP/browser 证据，标清合成正例与企业现场差别 | 新写入切片全部 NOT_RUN |
| 运行边界 owner | 对哪套独立库、哪些有限 Command 开放的另行明确授权及默认关闭 profile | 1A 独立身份/预检已获授权；业务写入仍未授权，保持只读，不改原库/Agent |

未来最小测试矩阵：

| 层次 | 必须覆盖 |
|---|---|
| Domain | 当前/过期/未来处置，唯一与多绑定，actor/subject 职责分离；完整标准集、草稿换版确认失配；四类签收与证据条件；只有 DRAFT 可写 |
| Adapter/应用 | 企业人、机器、管理 client 的 audience/scope/profile 分离；issuer/sub 到 canonical actor；无缓存成功授权、外部不可用拒绝；原读取 token 不可写；新写要求对应有限 capability+精确角色；仅失去写权限时原 COMMITTED 回执可恢复、无原回执则拒绝新写，失去 READ 时两者均拒绝 |
| 真实 PostgreSQL | IAM 撤销与 Command 双事务顺序；责任新增/处置竞争、未来失效、锁等待后重新取时；双创建、双补正、修订/确认竞争；历史 header/成员及 hash 不变；幂等回执同事务及失败全回滚 |
| HTTP | 401/403/404/409/413/422/503、严格字段、非 UUID 标准编码及大小写/完整集合、现有 id/clause/text 来源格式、同键异 body、丢响应后原回执；不同主体/空间/单据不可读写或恢复；旧确认、新责任和 Synthetic 来源拒绝 |
| Browser | 至少提供方、接收代表、标准确认人三个独立真实会话；创建→修订→确认→签收→只读刷新及历史回跳；MISSING/DISPUTED 阻断、撤权/换空间/旧窗口冲突、UNKNOWN 只读恢复与明确同键恢复，无自动 POST |
| 边界保留 | Agent API 回归；原库无迁移/业务变化；默认写关闭；无模型、provider、执行/发布/交付副作用；后续通过与归档接口仍不存在 |

独立库可为正例准备明确标识的合成 ISSUED/责任/证据，但仅作为测试夹具，不能伪称实际交付、实际接收或企业目录已验证；不得为造验收数据去调用真实交付。测试与现场接入证据分别记录，CI 不自动打开写入口。

合成正例只能由隔离测试 harness 的测试身份/来源 Port 接纳，绑定明确测试库与空间，不在正式运行构建/profile 提供放行 Synthetic 的开关；正式来源门禁的 Synthetic 拒绝须独立负测。浏览器中的独立会话可验证应用隔离，但企业 IdP 的真实多人登录仍需另列现场 NOT_RUN/PASS，不能用测试 Adapter 结果代替。

原设计轮（1A 实施前）的核对覆盖权威来源、状态、历史不可变、身份/工作空间、并发与恢复。实际变更仅 Markdown；没有进行本切片的接口、DDL、企业 IAM 或浏览器验收，也不把前一只读切片的 PASS 延伸到本设计。
