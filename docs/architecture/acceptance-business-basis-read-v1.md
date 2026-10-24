# 1B 前置：业务依据只读核对

2026-10-24。用户确认企业人员目录、责任/标准依据和收件材料来源暂未确定，先完成只读接入及独立环境验收。本文拥有本增量的安全投影与来源判定；1A 身份契约、现有责任判定及未来 Case/Receipt 写入契约保持各自归属。

复用现有 OIDC/RP、事务内 authority/binding fence、ContractVersion、EvaluateAcceptanceBindings、Evidence/evidence_relation 与 SHA256-EVIDENCE-V2。没有新增 IAM、文件上传、连接器平台、可信来源注册中心、业务表或写 Command。现有查询足够承载本次核对，不引入第二套目录、对象存储或策略引擎。企业来源 Adapter 只能在实际系统及协议明确后接入，当前不伪造一个已验证来源。

## 范围和信任

- 身份：当前 OIDC→canonical actor/workspace/grant 已核验；企业目录来源 UNCONFIGURED。登录成功不证明代表资格。
- 责任：复用完整事实集合的六类责任、代表/处置/有效期及标准适用判定；分别读取其依据 Evidence。CORE_CONFIRMED 是 Core 记录来源标记，不等于外部企业授权已验证；SYNTHETIC_FIXTURE 显式展示。
- 交付/标准：只引用明确同空间 DeliveryOperation/DatasetVersion 和指定 ContractVersion。ISSUED 只表示已有授权释放事实，不证明收到或授予再次下载；BLOCKED 保持阻断。sourceSha256 是来源材料的记录摘要，criteriaSetHash 是结构化标准集摘要，两者不冒充相互重算的结果；未保留原始来源字节时，不能根据 document JSON 重算后声称原件校验成功。
- 收件材料：只列出同空间且同时具有明确交付、版本关联的 Evidence 安全引用；仅版本关联、跨空间/跨交付/缺失引用不可见。关联、Evidence 类型字符串、metadata 的 verified/receiver/contentHash 均不是可信业务收到证明。记录完整性与附件内容完整性分开。
- Evidence 核验复用既有 hash 实现，私有读取字段总量上限64KiB；超限显示 NOT_CHECKED，不下载、不解析任意 URI。响应不返回 title、metadata、storageUri、sourceId/任意 sourceType、签名 URL 或 token。hash 只返回有界标准记录摘要，不作为附件内容摘要。

全部材料 `businessUse=BLOCKED`、`contentStatus=UNATTESTED`、`recipientStatus=UNATTESTED`；不存在“已签收”“验收通过”或可办理写入的判定。来源缺失、损坏记录、非收件治理结果、无材料均显示具体缺口。独立测试夹具可验证引用与完整性算法，不能把合成正例变成企业信任。

## API 与一致性

仅在显式 acceptance 只读 host 注册：

1. GET `/api/v1/workspaces/{workspaceId}/acceptance-basis?deliveryOperationId=UUID&datasetVersionId=UUID&contractVersionId=UUID&materialOffset=0`。标准可不选，材料固定每页25条。返回原 acceptance context、事务内身份依据、责任/适用 Evidence 核验、材料引用页与来源缺口。
2. GET `/api/v1/workspaces/{workspaceId}/acceptance-materials/{evidenceId}?deliveryOperationId=UUID&datasetVersionId=UUID`。精确材料详情；不存在或越界404，不回退最新。详情失败不抹掉页面已知的概览/列表。

每次都重新验证该用户 BUSINESS_ACCEPTANCE_READ、当前 canonical actor/space/revision，并在同一 READ ONLY/READ COMMITTED tx 中依序取得 IAM→workspace binding fence，刷新数据库时点，核验本人有效成果责任后才读取 Evidence。复用原领域判定，分页不缩小责任候选。计数和材料ID页使用同一SQL快照；每次响应是核对时点投影，不是授权 lease。权限撤销/源不可用无静态或演示回退。

只接受列出的参数及非nil UUID、0..2147483647材料偏移；拒绝重复、actor/party/source URL 等参数。响应 private/no-store。401/403表示当前身份/读取资格失败，404表示有权限范围内未找到明确引用，400参数错误，503读取依赖不可用。Agent 路由和默认 off profile 保持不变。

## 页面及运行

新版 `/core/acceptance-basis` 从本人负责成果进入，按人员与责任、交付与标准、收件材料三个区域核对。复用既有标准/责任页完成明确标准选择与历史追溯，不复制另一套编辑/状态流程；详情/分页/刷新/返回保留明确成果与标准。材料失败区域 UNKNOWN；失去身份读取资格不继续显示旧身份数据。业务写入口无按钮、无 handler。

只在已授权的独立1A库新增合成 Evidence/relations夹具并授予最小SELECT；无DDL、原库不动。继续保留 Agent API、模型/派发关闭、Draft不合并。需要后续另行实现的可信来源接纳/材料内容证明及业务写事务不在本次通过范围内。
