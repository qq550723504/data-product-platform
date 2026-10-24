# 责任绑定与标准适用只读验收

日期：2026-10-24；前置提交 cdef26695598209f97dc69cc6272e4d54476fa16。主要契约：[责任与适用关系](../architecture/acceptance-binding-read-contract-v1.md)。本轮只读实施补齐四类持久化业务事实，不开放业务写 Command，也未实现企业目录同步或正式签收。

## 实现与使用

- migration 74：主体代表关系、成果责任绑定、标准适用绑定和追加式处置；强类型范围/依据/有效期，禁止 UPDATE/DELETE；历史存在时 down 拒绝。两项 DatasetVersion 引用复用 lifecycle admission，不另建 IAM/RBAC/工作流基础设施。
- acceptance-context GET 复用可信 Principal Resolver；身份、工作空间和明确交付/版本均在查询前校验。同一 READ ONLY/REPEATABLE READ 时点读取完整范围，独立计算当前状态，再对历史分页。多绑定、自审、代表到期/撤销、source/完整标准集变化、原确认责任替换均 fail closed。
- 新版 `/core/acceptance-context` 展示六类责任、代表主体/身份/有效期/依据、标准适用和阻断；明确详情关联原代表关系、确认责任及处置，可展开继续追溯。刷新、分页、返回和标准选择保留明确上下文。跨成果或缺失的 bindingId 返回 UNKNOWN，标准与列表仍可独立读取。
- 所有演示与企业角色仍分离；验收夹具标为 SYNTHETIC_FIXTURE。绑定有效不代表签收、验收通过、下载资格或执行授权，writeEnabled 始终 false。

## 独立环境与证据边界

原库 `agent_q2_test` 保留 schema 73 / 216 表；仅独立库 `agent_q2_acceptance_20261024` 进入 schema 74 / 220 表。准备工具要求明确 ACK 和精确数据库名，已有绑定时拒绝重放；复用 migration runner 和 Evidence.Append。

准备阶段新增七组持久化合成场景（当前、代表到期、资格撤销、多绑定、职责冲突、标准集错配、确认责任替换），交付均为明确 SYNTHETIC_NO_PROVIDER / BLOCKED，没有真实签发或 provider 调用。默认场景另有 16 条过期责任用于翻页。reader 仅新增四表 SELECT，无业务 DML/DDL/TEMP/schema CREATE。

新二进制与 Next standalone 在隔离目录构建后，按 owner checkout、PID、启动时间、端口、路径/命令验证，仅替换 18497 API 和 4188 UI；保留旧可回滚构建、4189 认证网关和 PostgreSQL 容器。正式 Agent API 源码与语义保留，预览写入和 dispatch 继续关闭。

读取窗口前后，原库 216 表、独立库 220 表的逐表行数/内容指纹相等；原库另与准备前指纹相等。Execution / AgentTask / ActionRequest / DeliveryProviderAttempt / Outbox 均为 0。测试用例需要插入的独立数据与只读窗口分开；通用新 PostgreSQL guard 用例在单独事务内全部回滚。

## 验收覆盖

- Domain：有效期左右边界、未来处置到点生效、撤销/到期依赖、actor 或 subject 自审、多绑定、完整标准集与来源、原确认责任、不同事实类型的同 UUID 不混淆。
- Query/HTTP：可信调用身份不接受 X-Actor-ID/query 冒充；401/403/404、重复/非法参数、精确历史不回退、关联依赖不使用替代责任、分页不影响当前判定。
- 真实 PostgreSQL：通用 guard 用例不依赖预览固定 ID，可由 PostgreSQL CI 直接执行；成功事实、范围拒绝、不可变性、未来处置和 lifecycle admission 均在回滚事务验证。另有明确预览夹具专用查询/历史/down 测试，仅在该合成环境执行，不冒充通用 CI 覆盖。
- 前端历史校验 20 项、既有 HTTP fixture 19 项、完整 Next 构建及 lint/typecheck 通过。新增标准/责任浏览器 8 项覆盖正常/异常/历史/UNKNOWN/0 业务 POST；已有全站回归继续保留。
- 现场 Edge 使用已有独立验收标签页，保留原业务归档两个标签页和会话。实走责任与标准绑定、翻页/刷新/精确回跳、六类异常、跨成果历史拒绝、旧确认责任与替换处置展开。所有业务操作入口保持关闭。

现场补齐了关联处置追溯，修正了详情 DOM 与现有定义列表布局不匹配导致长 hash 挤入窄列的问题。失败证据保留：一次 gateway 用例因调用脚本遗漏 Basic 前缀返回 401，修正测试调用后 8/8 通过，未放宽认证；一次全站浏览器运行 121/122，失败为页面导航前 ERR_NO_BUFFER_SPACE，保留日志/trace，未添加自动重试或改变业务断言。最终构建的验证结果和精确 HEAD CI 在 PR #363 当前增量中单独记录，不继承前置提交绿灯。

本轮没有业务 Case/Receipt/Decision/Archive 写入，没有模型、worker、provider 或真实执行；PR 保持 Draft，未合并、未部署。后续业务写入仍需独立 Command、幂等、事件/审计/证据及与责任/标准处置共享的 fence；本次 GET 不构成办理授权。
