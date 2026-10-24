# 1B 前置：业务依据只读接入与验收

2026-10-24。用户授权本次只读接入，并明确企业来源暂未确定；本轮完成接口、页面及隔离验收，真实来源显示未配置。当前契约归 [业务依据只读契约](../architecture/acceptance-business-basis-read-v1.md)，后续 Case/Receipt 写入仍归原第一切片设计。

## 交付范围

新增新版 `/core/acceptance-basis`：从本人有效责任成果进入，核对人员/责任、交付/标准和收件材料。复用完整的原标准选择/责任历史流程，返回保留明确交付、版本、标准、材料ID与页码；材料详情和返回列表分别定位到相应区域。相同成果的不同交付在选择列表显示明确交付ID。

两个 scoped GET：`/workspaces/{workspaceId}/acceptance-basis` 与 `/workspaces/{workspaceId}/acceptance-materials/{evidenceId}`（均以 `/api/v1` 为前缀）。独立 acceptance host 才注册，BFF固定GET白名单、逐用户当前权限和IAM→责任fence复用1A，没有共享读取凭据回退。默认profile、原库及Agent API保留。

Core持久化事实、企业来源和收到证明明确分开：

| 已能核对 | 本次不能据此认定 |
|---|---|
| 真实OIDC协议会话、canonical actor/workspace/revision | 六个合成账号不代表企业员工目录已接入 |
| 平台责任/代表/适用事实及其 Evidence 记录完整性 | CORE_CONFIRMED、合法身份或资产owner不证明真实企业授权 |
| 明确交付、发布的标准版本、完整标准集hash和适用关系 | BLOCKED不能建单；ISSUED不证明收到；来源hash的存在不证明未读取的原件内容 |
| 同空间、同时关联明确交付和版本的材料引用 | Evidence类型/metadata中的verified、contentSha256或receiver不证明材料真伪、接收主体或收到情况 |

材料始终 `businessUse=BLOCKED`、内容/收件主体 `UNATTESTED`；业务目录、责任、标准权威来源及收件来源仍未配置。无业务写按钮、handler、Case/Receipt事实或自动“通过”。不下载附件、不访问任意URI、不接模型或派发真实执行。

## 复用与实现边界

- 复用已有 OIDC/RP、`ValidateInTx`、workspace binding fence、完整 `EvaluateAcceptanceBindings`、scoped repository、Evidence/evidence_relation及既有SHA256-EVIDENCE-V2。未新增业务表、DDL、来源注册中心、登录系统、文件上传/连接器、队列或策略引擎。现有能力满足本次只读投影，不引入第二套成熟基础设施。
- 将原Context投影拆出可复用纯函数；原GET及本次核对使用同一判定，没有从历史分页截取角色后推断责任。
- Evidence owner提供有界完整性引用，≤64KiB才私有读取原字段并调用原hash校验。title、metadata、storageUri、任意source字段、下载凭据均不进入新API。材料计数、ID/内容安全投影使用同一SQL快照；精确材料失败404，不查最新。缺失、损坏、超限分别显示。
- 身份与责任在同一 READ ONLY/READ COMMITTED事务持锁重读，随后读取明确成果依赖；撤销先提交必须拒绝。当前事实与历史type+ID一起识别；不通过业务事实授予READ能力。

## 独立准备与验证

保留原停止容器及其台账；只使用已授权的 `acceptance_1a_20261024`。一次性附加准备器拒绝已有fixture ID，追加60条显式合成Evidence及关联，包括声明为收件、治理结果、坏hash、超限和单边关联。没有执行交付、签收、Case或其他业务Command；原有Evidence不可更新/删除。

API reader增加且仅增加 Evidence/evidence_relation 的SELECT（合计14表），RP仍仅两张临时会话表。全表权限检查确保无业务DML、DDL、TEMP或public function EXECUTE。新构建使用独立dist，切换时只核验并重载本任务拥有的API/UI进程，保留先前构建与原Agent环境。

已实施的验证覆盖：

- 真实PG：当前责任/标准及13个当前依据、28个精确双关联引用/25+3分页/空末页；坏hash、超限、非收件声明和伪造metadata不被信任；跨空间/单边关联404；伪造query actor不改变身份；纯读取前后Evidence指纹一致。实际pg_locks证明读事务等待撤权后拒绝。
- HTTP：身份先于对象读取，有限能力/显式host opt-in、重复/未知/越界参数、401/403/404/503、private/no-store；未配置的默认profile无新路由。
- Web：严格DTO/当前身份/精确目标校验，拒绝来源升级、原始私密字段、替换页码和材料；原标准/责任页完整保存返回上下文。损坏详情不抹掉已知概览/材料列表，身份读取失效则停止展示。
- Edge：原21组及新增10组，共31组，覆盖提供方/接收方/确认人、分页/详情/刷新/标准历史返回、坏hash/超限/治理结果、缺失/单边/跨空间材料、390px页面及撤权；纯读取窗口包含Evidence关系的业务指纹相等，执行/任务/请求/provider/outbox均空。
- 本机全量Go在新的独立空库运行；RP18项在独立unit库实际执行，无PG跳过；完整隔离构建/lint/typecheck。新增PG和浏览器路径加入原required身份工作流，CI只依据最终精确HEAD完成结果，不继承前一提交。

浏览器首轮未完成的导航断言保留失败记录；修正测试对SPA返回导航的就绪判断，未放宽页码/材料身份断言、未加业务重试。长列表的详情和返回另补锚点及视口断言，避免仅DOM存在却不可见的虚假通过。

当前有界作者复核不冒充新独立全PR批准。企业IdP/目录、业务权威来源、原件内容、真实收到、正式建单/签收/验收/归档均未接入；PR #363仍Draft，不合并、不部署。后续真实来源接纳和正式业务写入不因本次只读通过自动启用。
