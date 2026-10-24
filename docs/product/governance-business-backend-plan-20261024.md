# 业务签收验收与归档接入计划

状态更新：1B前置[业务依据只读核对](acceptance-business-basis-read-implementation.md)已实施；用户确认企业来源暂未确定，只有安全引用和来源缺口，不启用Case/签收写入。

1A状态更新：独立 1A 身份/责任准入已实施，当前范围归 [1A 实施记录](acceptance-identity-1a-implementation.md)；业务写入仍关闭。以下设计轮及前置只读记录保留其原时间边界。

原状态：第 0 切片及责任/标准适用的持久化只读查询已实施，当前行为归 [责任与适用关系契约](../architecture/acceptance-binding-read-contract-v1.md) 和 [绑定只读验收](governance-acceptance-bindings-readonly-20261024.md)。2026-10-24 对 `1141b72c` 细化 [第一切片写入准入设计](../architecture/business-acceptance-draft-receipt-admission-v1.md)，本轮仅文档。下表按当前代码更新，不再将已补齐的读取身份/标准格式列为未实现。其余业务写入仍为目标设计；保留原库、预览运行状态和 Agent API，业务写入口继续关闭，不接模型或真实执行，PR #363 保持 Draft、不合并。

## 当前能力与缺口

新版业务签收、验收、归档页面的正常与异常办理交互仍运行在 DemoProvider/sessionStorage 中；独立 `/core/acceptance-context` 已读取持久化责任与标准适用事实。`internal/acceptance` 是现有端到端测试包，不是这三类业务事实的服务实现。1A已实现独立逐用户身份及本地事务责任校验；正式写入仍缺企业身份/业务依据的可信接纳、Case/Receipt和冻结提交/验收决定，不能只把演示按钮改成POST。

优先级含义：P0 是正式业务写入前必须解决的基础边界；P1 是首个真实闭环；P2 是首版以后的便利和扩展能力。现有 API 的注册能力与当前预览是否启用应分别核对，下表不将仓库中的所有路由视为已部署。

| 页面或交互 | 已有能力 | 缺口或接入问题 | 优先级 |
|---|---|---|---|
| 新建验收单的成果选择 | delivery-history 与 acceptance-context 已按可信身份/明确成果展示交付、版本和接收主体 | 缺 Case 的强类型固定绑定与事务内重新准入；当前合成交付均 BLOCKED，不能正式建单；ISSUED 不证明已收到 | P0 → 第一切片 |
| 接收方和验收责任人 | 已有六类责任、代表关系及追加处置、有效期/职责分离/多绑定判定和历史；独立 OIDC/RP 候选可复用 | 当前 StaticResolver/合成绑定不等于企业多人身份；候选未集成，缺本模块 profile 与身份/责任撤销同事务 fence | P0 |
| 标准来源选择 | 合约版本列表/详情已具备 principal/workspace 限定，完整 acceptanceCriteria 格式/hash 校验及标准适用查询 | 缺草稿逐项冻结、接纳后写入验证；标准适用不等于接收方对本次草稿确认 | P0 → 第一切片 |
| 编辑标准和接收方确认 | 标准原文和完整集合可读；演示拒绝削弱 required/method | 缺绑定精确 DraftRevision/hash 的 Confirmation；正式版仅编辑注记/证据，来源由服务端重建 | P1 第一切片 |
| 业务签收 `/#business-receipts` | 完整、部分、未收到、争议及补正演示；已有代表关系只读模型 | 无 BusinessReceipt、可信收件材料接纳、强类型证据关联及服务器历史；不可用 Command 自己的审计 Evidence 充当收到依据 | P1 第一切片 |
| 业务验收草稿 `/#business-acceptances` | 新建、筛选、详情、编辑演示 | 无 Case/DraftRevision、服务器 revision、受权持久化和明确目标 | P1 第一切片 |
| 冻结提交和撤回 | 演示冻结标准、签收并保留旧材料；支持撤回 | 无 AcceptanceSubmission、标准确认、DB 冻结/成员 hash 与跨客户端竞争控制 | P1 第二切片 |
| 退回和重提 | 演示有退回理由、签收变化阻断、重新提交 | 无指定验收人的服务器判断、互斥终结决定及命令回执 | P1 第三切片 |
| 逐项验收通过 | 既有质量、认证、发布历史可提供证据引用 | 无业务验收决定；技术认证/发布不替代逐项业务判断，必需标准须精确覆盖 | P1 第三切片 |
| 取消验收 | 演示仅草稿/退回态允许取消 | 无显式取消 transition、权限与并发保证 | P1 第二切片 |
| 业务归档 `/#business-archives` | 演示独立归档、冻结追溯；`/core/archives` 已读 EvidenceSnapshot | 无 AcceptanceArchive 及对 receipt/submission/decision 的强类型完整链；证据档案须继续独立 | P1 第四切片 |
| 决定作废和归档后追溯 | 演示追加 disposition，原档案不改 | 无服务端 disposition、当前有效性投影与作废/归档串行化 | P1 第四切片 |
| 刷新恢复、消息回跳和失败处理 | 演示按空间保存选择；现有 Agent 有回执校验与 UNKNOWN 恢复模式 | 缺本领域精确 ID 深链、同主体命令回执恢复；不能回退 latest 或自动重发 | P1 贯穿各切片 |
| 需求/邮件标准、附件、签章、批量验收 | 当前仅文本演示或未支持 | 无正式版本接纳/上传/签章契约；首版不假装已支持 | P2 |

## 页面接入方案

保留侧栏“业务签收、业务验收、业务归档”三个入口；证据档案继续位于现有 Core 只读入口。页面职责和办理顺序沿用新版演示，不增加一套顶部重复菜单。

| 页面 | 正式版应展示的内容 | 核心交互 |
|---|---|---|
| 验收列表与新建 | 真实明确成果、提供方、接收组织、指定验收人；业务状态/决定有效性/归档状态分列 | 创建前检查目标资格和责任；无合法标准或主体时展示具体阻断原因 |
| 签收详情 | 明确成果、登记人代表关系、收到情况、证据、历次补正和当前依据 | 追加补正前显示被替换的 receiptId；签收不能触发验收通过 |
| 验收草稿 | 标准来源版本、条款、必需性、核验方式、接收方确认状态 | 修改后重新确认；不允许提供方降低必需条件；提交前展示冻结材料清单 |
| 待审详情 | 明确 submissionId、冻结标准/签收/来源、当前依据变化提示 | 指定验收人逐项判断；退回/通过分别确认；撤回与审核并发由服务端裁决 |
| 已通过详情 | 原通过决定、判断依据、当前有效性、归档状态 | 验收通过后独立归档；作废有独立权限和必填原因，不能编辑旧判断 |
| 业务档案详情 | 当时签收、冻结提交、标准、决定、清单 hash；之后处置另列 | 精确回跳至原对象；当前作废不覆盖原决定；无权读取时阻断而非展示缓存 |

接入时正式数据使用独立 `/core/business-acceptances`、`/core/business-receipts`、`/core/business-archives` 路由，沿用既有 Core 壳和服务端身份；`/#business-*` 保留为明确标识的演示模式。两者复用可展示组件，不共享数据源、操作 handler 或存储 key。正式接口失败不得降级为演示记录。

正式深链携带 workspaceId 及明确 caseId、receiptId、submissionId、decisionId 或 archiveId。由当前身份重新鉴权并核对从属关系，消息跳转不携带命令权限。跨空间先完成明确上下文切换和服务器校验，清空未保存表单；浏览器刷新从 GET 恢复事实，不以 sessionStorage 判定成功或恢复未保存的 PASS。

写入超时后保留原幂等键和完整 body，显示“结果未知”；只读查询原命令回执。收到与原请求绑定的确认后再读详情；详情 GET 失败保留“命令已确认、当前详情未知”，禁用会造成重复业务动作的入口。不自动重发，不给用户换键再点一次的错误引导。

第一切片拟用受保护服务端意图记录保存原请求，浏览器仅保留不透明恢复引用。回执 404 仅表示 NOT_OBSERVED；未来用户明确选择恢复原请求时，也只能同一人/空间、原键及完全相同 body，由 Core 幂等锁处理在途竞争。恢复契约、身份撤销和记录丢失的边界归详细设计，当前不实现该入口。

## 分阶段实施

完整生命周期归 [后端契约草案](../architecture/business-acceptance-backend-contract-v1.md)，第一切片的更窄动作范围、字段和准入归 [详细设计](../architecture/business-acceptance-draft-receipt-admission-v1.md)。本表只规定拆分、页面覆盖及每个切片的完成条件，不另维护状态机。

| 顺序 | 交付范围 | 完成条件 |
|---|---|---|
| 0 标准和责任只读 | 已有安全 acceptance-context、合约版本/完整格式/标准适用、六类责任及处置历史 | 只读范围已验收；合成来源显式展示，writeEnabled=false，不代表企业身份或真实收到 |
| 1A 身份与责任准入依赖 | 复用独立 OIDC/RP 候选，解决迁移/模块冲突、有限业务能力和 Tx-scoped Authority Port；补齐收件材料来源 | 独立 1A 已补齐多人会话、当前身份/责任与本地撤销 fence，见实施记录；企业目录、收到材料和正式业务写入仍未接入 |
| 1B 草稿与签收 | Case、DraftRevision、BasisConfirmation、BusinessReceipt 及本领域 CommandReceipt；四类显式 Command 和精确查询 | 只处理 DRAFT；角色/来源当前有效；签收有可核对材料；补正追加不覆盖；原键恢复、PG 并发与页面见证 |
| 2 冻结与流程控制 | Submission、withdraw/cancel transition、冻结成员；扩展已有本领域命令回执与可办理状态 | 不完整材料无法提交；补正与提交竞争安全；撤回与取消不删除旧提交；刷新能恢复精确材料 |
| 3 业务决定 | RETURNED/ACCEPTED、完整逐项判断、ReviewActivity 和强类型成本关联 | 非指定验收人/自审被拒；签收变更阻断通过；撤回与通过只能一个成功；退回重提保留所有历史 |
| 4 归档与处置 | Archive、EvidenceSnapshot 关联、INVALIDATED disposition、当前有效性投影 | 一单一档幂等；作废不改原事实；作废与归档竞争顺序明确；精确追溯及消息回跳完成 |

每个切片独立包含 Domain、Repository、Application Command/Query、HTTP、事件/审计/证据、幂等、workspace/责任校验及测试。按纵向可验收路径推进，不先做一整层通用 CRUD。进入第三切片前不能把草稿或冻结提交显示为“验收通过”；进入第四切片前不能用 EvidenceSnapshot 显示“业务已归档”。

未来数据库 DDL、测试夹具和最小权限只在独立验收库验证；原库不动。预览默认保持只读，写接口是否注册、具备哪些动作、连接哪一独立库须作为后续单独运行边界。CI 或离线测试通过不自动启用写开关。PR #363 不承载这些后续正式业务写入。

## 首版验收矩阵

| 场景 | 后端应证实 | 浏览器应证实 |
|---|---|---|
| 正常闭环 | 同一明确成果的签收 → 确认标准 → 冻结提交 → 指定验收人通过 → 独立归档 | 每步展示服务端 ID、角色和 revision，刷新及回跳恢复同一记录 |
| 不完整或争议签收 | PARTIAL/MISSING/DISPUTED 阻断提交 | 明确阻断项，已有材料仍显示 |
| 标准来源和修改 | 无合法来源或确认拒绝；修改后旧确认不可用于新提交 | 展示版本及确认差异；历史提交保持旧标准 |
| 签收补正 | 旧提交 receiptId 不改；新签收阻断旧提交通过 | 退回/撤回后重提；旧材料可精确展开 |
| 撤回与取消 | 审核/撤回互斥；仅合法状态取消；终态不复活 | 旧窗口操作显示冲突并读取当前事实，不自动重试 |
| 作废和历史 | 追加 disposition；原 receipt/submission/decision/archive 逐字段不变 | 原决定与当前作废分别展示，另建关联单重新验收 |
| 工作空间隔离 | principal、所有父子引用、读写及回执均按空间/主体约束 | 同编号/异空间不串数据，未保存输入清理，消息越界明确拒绝 |
| 幂等与 UNKNOWN | 同键同内容返回原回执，不同内容冲突；失败事务无部分写入 | 丢响应后只读恢复，无重复决定/档案/成本，无演示回退 |
| 冻结竞争 | 真实 PostgreSQL 验证成员 late commit、补正/通过、归档/作废竞争 | 浏览器观察确定结果，不能用单客户端校验代替 DB 保证 |

当前设计轮仅检查源码对应关系、权威文档一致性、链接和变更范围；新业务写入的接口、DDL 与上述后端/浏览器用例均 NOT_RUN。已实现只读切片的验收单独归属其精确提交，不能计入写入草案的实施完成率。

## 已有实现依据

- 页面和本地模型：`apps/web/components/governance-demo/BusinessAcceptancePages.jsx`、`business-acceptance-model.mjs`；现有行为归 [演示说明](governance-business-acceptance-demo-20261024.md) 和 [增量审查](governance-business-review-20261024.md)。
- 交付与证据查询：`apps/platform/internal/readmodel/transport/http/delivery_history.go`、`internal/readmodel/delivery_history.go`；范围归 [交付与证据只读验收](governance-delivery-history-readonly-20261024.md)。
- 合约/标准与当前边界：`apps/platform/internal/contract/domain/acceptance.go`、同目录 `acceptance_bindings.go`、`apps/platform/internal/contract/transport/http/readonly.go`、`apps/platform/internal/contract/infrastructure/readonly.go`、`migrations/000074_acceptance_bindings.up.sql`；已有读取安全不等于 Command 准入。
- 复用基础设施：`apps/platform/internal/platform/principal/principal.go`、`platform/transaction/manager.go`、`platform/audit/store.go`、`platform/outbox/store.go`、`internal/evidence/snapshot.go`、`internal/agenttask/application/service.go`。
