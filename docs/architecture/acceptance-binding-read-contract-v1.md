# 验收责任与标准适用只读契约

日期：2026-10-24。范围为已有第 0 切片的后续只读补齐。新增绑定事实、查询与页面，业务写入口、模型、worker/provider 和真实交付保持关闭。原库与既有 Agent API 保留，DDL/夹具和最小 SELECT 权限仅在独立验收库验证。PR #363 保持 Draft、不合并。

## 事实归属与复用

现有 Principal Resolver 继续提供可信调用身份；Evidence 提供依据引用；ContractVersion 及完整 criteria hash 提供不可变标准来源；DeliveryOperation 提供明确交付和成果。Core 保存业务责任与适用关系，不实现登录、OIDC、通用角色管理或第二套策略引擎。

external_identity_binding 仅代表外部系统身份关联，其 provider/external_subject 不证明接收方代表资格、有效期或成果责任，因此不能直接复用为验收授权。四类新增事实是业务语义，不新增基础设施；遵循能力地图和 ADR-0010。当前验收库的人员、组织和绑定均为显式 SYNTHETIC_FIXTURE，用于验证真实存储/查询，不代表已经接入企业身份目录或取得真实业务签收。

| 事实 | 强类型内容 | 不变量 |
|---|---|---|
| acceptance_party_representation | workspace、actorId、principalSubject、partyRef、依据 Evidence、validFrom/validTo、origin、recordedBy | 人代表哪个主体；不因有读取身份而自动建立关系 |
| acceptance_responsibility_binding | workspace、deliveryId、datasetVersionId、role、representationId、依据、有效期、origin | 代表谁办理哪笔明确成果；接收/确认/验收角色的 party 必须匹配交付 effective consumer |
| acceptance_standard_applicability | 同一成果范围、contractVersionId、sourceSha256、完整 criteriaSetHash、confirmerBindingId、依据、有效期、origin | 标准适用由明确 BASIS_CONFIRMER 责任绑定支持；不能因同工作空间或内容相似而自动适用 |
| acceptance_binding_disposition | 三类事实之一的强类型 ID、workspace、INVALIDATED/REVOKED/SUPERSEDED、effectiveAt、原因、Evidence、recordedBy | 追加失效，不更新/删除原事实；未来生效的处置只在到期时退出当前集合 |

所有事实插入后 header 不可 UPDATE/DELETE；没有可变 child membership 或 DRAFT/finalize 阶段。新版本或更换人员追加新事实并处置旧事实。DB 校验同空间/同成果引用、依据 Evidence、角色与确认人关系、来源 hash，所有 DatasetVersion FK 加入既有 lifecycle reference admission guard，保持未释放历史引用。migration down 在锁内确认所有新表为空，否则拒绝。

## 当前判定

查询使用同一个 READ ONLY/REPEATABLE READ transaction 的数据库时间 evaluatedAt。有效区间为 validFrom <= evaluatedAt < validTo，空 validTo 表示无有限结束时间；已生效 disposition 优先。前端时间、任意 actor/consumer 参数不参与判定。

责任分别为 PROVIDER、RECEIVER、BASIS_CONFIRMER、REVIEWER、ARCHIVIST、DISPOSER。无记录 UNASSIGNED；记录存在但未到期/过期/失效或代表关系失效为 INACTIVE；同一角色存在多个当前有效责任事实为 AMBIGUOUS，不选 latest；唯一且全部依赖满足为 ASSIGNED。提供方与验收人、提供方与标准确认人须 actor 和 subject 均分离，否则相关角色 CONFLICT 并阻断。当前调用者匹配只来自 Resolver，展示 isCurrentCaller 不授予办理能力。

标准适用必须绑定查询中的明确交付、DatasetVersion 和 ContractVersion，来源须 PUBLISHED、格式 SUPPORTED、source hash 与完整标准集 hash 均相等；确认责任必须是当前唯一有效的 BASIS_CONFIRMER 且精确等于记录中的 confirmerBindingId，代表接收主体。没有选择标准为 UNSELECTED，无记录 MISSING，过期/处置为 INACTIVE，多个当前记录 AMBIGUOUS，hash/确认人不匹配为 MISMATCH，仅全部满足 MATCHED。新的责任人不会自动接管旧适用确认。

绑定有效、标准 MATCHED、交付 ISSUED 仍不等同已签收/验收通过或当前下载许可。本阶段 writeEnabled 始终 false，并始终返回 ACCEPTANCE_WRITES_DISABLED。业务 Case/Receipt/Submission/Decision/Archive 仍归后续写入切片；未来写 Command 需重新验证当前责任、标准及其依赖，并与处置共享业务 fence/revision，不能信任本次 GET 的旧结果。

## 查询与页面

复用 GET `/api/v1/workspaces/{workspaceId}/acceptance-context`，明确 deliveryOperationId/datasetVersionId，contractVersionId 可选。增加 bindingOffset（固定每页 25）和可选 bindingId 精确历史详情。历史可浏览代表、责任、适用及处置原事实，详情保留原有效期、依据和后续处置；跨空间/跨成果或缺失精确 ID 返回 404，不回退最新。分页不影响当前判定，当前判定不截取历史分页再推测。

上下文新增 evaluatedAt、真实责任详情及 isCurrentCaller、标准适用详情、bindingHistory.page/items/selected/related。related 使用完整范围事实追踪原代表关系、确认责任及其追加处置，不受当前历史页影响。不同事实类型的 ID 分开识别；bindingId 在同一成果范围命中多个不同类型时拒绝，不猜类型或 latest。不返回 provider key、credential、下载 secret、任意 metadata 或 Evidence 内容。鉴权、参数校验和无缓存边界沿用第 0 切片；只注册 GET。调用者由 Principal Resolver 验证，不能以页面演示角色、X-Actor-ID 或 body 冒充。

新版沿用 `/core/acceptance-context`，展示责任主体/代表依据/有效期、当前判定及阻断原因；标准区展示明确适用绑定、确认责任和 hash 对照。增加独立历史表和精确详情，分页/详情/标准选择/原交付回跳保留上下文，失败区域 UNKNOWN，已读标准与列表不丢失。历史有效性与当前有效性分开，所有合成来源显式标识。演示页面和其 session 数据不迁移为正式事实。

## 有界验收

Domain 覆盖时间边界、未来处置、过期/撤销代表、角色冲突/自审、标准错配、旧确认人、完整 hash 与多绑定。真实 PostgreSQL 覆盖同空间/同成果强约束、不可变性、处置追加、lifecycle admission、拒绝破坏性 down 和独立库读取权限。HTTP 与浏览器覆盖 401/403/404/非法参数、刷新、历史分页/详情、失效后追溯、区域 UNKNOWN、0 业务 POST 与 Agent 保留。

本轮不新增业务写 Command；fixture 是独立库显式准备工具，不是公开配置入口。没有验证过的企业目录、多人登录、真实组织权限或动态 IAM 同步不得声称已完成。
