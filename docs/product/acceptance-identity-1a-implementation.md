# 1A：可信多人身份与责任准入

2026-10-24。用户已授权实施 1A，范围为独立验收环境的逐用户会话、责任预检、事务内身份/责任校验及新版页面。本文拥有 **1A 已实现范围及证据**；[第一切片设计](../architecture/business-acceptance-draft-receipt-admission-v1.md) 继续拥有未来草稿、确认、业务签收与恢复契约。

## 已实现与尚未开启

- 复用冻结的 OIDC/RP 候选 `fde81069d79c1189c9dae08515646eeac5e008d0`，没有合并其其他模块、账号管理 HTTP 写入口或迁移编号。外部 IAM 协议由 ZITADEL Go OIDC / openid-client 负责，平台只拥有 canonical actor、workspace grant 与业务责任语义。
- 私有查询 `POST /internal/identity/business-acceptance-principal` 返回固定 `business-acceptance.v1` 投影；只接受该用户 Bearer + 唯一 workspace_id。要求本模块 READ，不扩大 HUMAN_DECISION / Agent 旧路径能力。新增 `GET /api/v1/workspaces/{workspaceId}/acceptance-targets` 只列出本人有效责任成果；现有合约版本/context 安全投影复用原 Repository 与领域判定。
- Code + PKCE S256、state、nonce、RS256/JWKS 验证；浏览器仅持有 Secure/HttpOnly/SameSite=Lax opaque cookie。加密 PG 会话跨进程可恢复，原子消费回调；每次读取重新 introspect 并检查当前映射/能力，pin authorityRevision，无共享 token/演示身份回退。外部 sub 与平台 canonical Subject 明确分离。
- `bindings.Store.ValidateInTx` 在调用者同一 READ COMMITTED pgx.Tx 内取 IAM fence，重读 actor、subject binding、workspace grant、能力、revision 和数据库时钟。`CheckResponsibilityInTx` 再取 workspace binding fence，刷新资格和时间并复用完整 `EvaluateAcceptanceBindings`；不另开连接、不提交调用者事务、不把 GET 投影当写入 lease。现有/新增 binding/disposition INSERT 与身份 projection 变更使用同一 namespace 和锁顺序。
- `/identity` 和 `/core/acceptance-context` 显示本人、空间、有限能力、明确成果、四项责任预检、标准适用与不可变历史。空间选择只改变登录路由，Core 再检查成员关系；默认 off 的原站点及 Agent 路径保持原运输方式。1A profile 不开放其他模块/动作。

**所有业务写入口继续关闭。** 没有 Case、DraftRevision、BasisConfirmation、BusinessReceipt、CommandReceipt 的持久化或 HTTP Command；没有收到登记、验收决定、模型调用、Agent dispatch 或真实交付执行。技术登录 POST 和临时会话写入不是业务写入。责任匹配、合成标准适用或 EvidenceSnapshot 均不能证明收到或验收通过。

四项预检固定对应 PROVIDER/DRAFT、BASIS_CONFIRMER/BASIS_CONFIRM、RECEIVER/RECEIPT_RECORD；REVIEWER/ARCHIVIST/DISPOSER 本阶段仅 READ。Synthetic、非 ISSUED、标准缺失/不适用及写关闭持续显示阻断。事务 Port 只证明当前本地身份/责任，不替代未来 Case、真实来源、收到材料、幂等、Evidence/Audit/Outbox 或部署准入。

## 隔离与最小权限

新增 migrations 75（当前 IAM 映射及共享 fence）/76（临时 RP 状态）。75 的 down 拒绝丢弃身份历史或 fence；76 的 down 必须排空并持锁。原 schema74 责任事实保持 append-only，不覆盖历史。

本机独立数据库 `acceptance_1a_20261024`，loopback TLS PG 25495；合成 Keycloak realm 19120；查询 API 18498；页面 `https://localhost:4290/identity`。六名合成目录人员、两个空间；提供方在空间 B 有责任事实但没有 IAM grant，用于验证业务事实不能授予读取资格。夹具交付均 BLOCKED / SYNTHETIC_NO_PROVIDER，没有调用交付 Command。

API 角色仅对 12 张指定表 SELECT，默认 transaction_read_only；RP 角色仅对两张临时表 SELECT/INSERT/DELETE。两者均无业务写、schema CREATE、TEMP 或 public function EXECUTE 权限。独立进程信任本次测试 CA，Go/Node API 与数据库均验证证书；没有安装系统 CA、全局关闭 TLS 或给浏览器可读 token。浏览器自动化仅在其独立测试 context 接受测试证书。

原 `dpp-agent-q2-test-postgres-20261023` 本轮检查时已退出、无挂载，旧 PID/端口清单也已过期。没有按旧清单重启、迁移、重建或删除它，没有改原 Agent 预览配置。保留前后容器 identity/status/mounts/startedAt 与旧 manifest hash 的独立核对；不声称已对停止的原库执行内容比对。

## Build-vs-Buy / Reuse Check

| 能力 | 决定与替换边界 |
|---|---|
| 身份协议、账号、PKCE/JWKS | 复用候选：ZITADEL Go OIDC v3.45.0（Apache-2.0）、openid-client 6.8.8 / pg 8.23.1（MIT）；没有自建密码、MFA 或 SSO。企业 IdP 可替换测试身份源，静态 issuer/endpoints/client/audience/scope 属 Adapter 配置 |
| 独立身份源 | 复用官方 Keycloak 26.8.0（Apache-2.0），镜像固定 digest，使用其原生 realm import/mapper，不 fork。仅本地/CI 合成 IdP，单独进程/证书和维护成本；不是企业生产 IAM 部署。[官方容器](https://www.keycloak.org/server/containers)、[realm 导入](https://www.keycloak.org/server/importExport) |
| 授权规则 | 既有有限能力映射 + 精确责任领域判定足够；未引入第二套 Casbin/OPA。未来通用策略需求另行评估 |
| 会话、并发与存储 | 复用候选 PG session store、现有 pgx.Tx/锁基础设施，不新增 Redis/Temporal/队列。新增的本地身份/责任串行化只拥有平台准入 invariant |

依赖检查发现原 Next/PostCSS 及 pgx/Go 工具链漏洞：Web 更新 Next 15.5.28 / PostCSS 8.5.29；pgx 更新 5.9.2（要求 Go ≥1.25），仓库工具链及 CI/演示构建固定 Go 1.26.10。没有更新或重启原有二进制。npm audit 与身份包 govulncheck 的当前结果单独核验，不将扫描等同企业安全验收。

## 验收与可复现性

`tests/acceptance-identity/bootstrap.py` 只允许全新独立资源，发现同名资源/已准备数据库即拒绝覆盖；私密 profile、证书私钥、密码和日志在 ignored `.data/acceptance-identity-1a`，不进 Git/CI artifact。运行脚本验证已有进程的路径和创建时间后才允许重载自己拥有的 API。HTTPS 主机使用 Next 原生 TLS 请求的实际 hostname/port；曾发现代理内部端口改变 NextRequest.url，已修正，保留来源/CSRF 检查。

| 证据 | 范围 |
|---|---|
| Edge 独立浏览器 context | 六个 canonical 人员、提供/接收/标准确认的不同能力和责任；明确标准选择、适用关系、责任历史详情与刷新；跨空间目标拒绝；B 撤权不影响 A；actor 撤权/恢复不复活旧会话；数据库会话到期；手机宽度；读操作不改冻结业务事实 |
| RP SDK / PG | 真实 TLS Code/PKCE/JWKS、双人 canonical mapping、跨浏览器盗用 state/code、nonce/签名/过期/撤权、单次并发回调、加密状态、同源/CSRF、原子排空迁移、固定 GET allowlist；独立 unit DB 下所有 PG 用例实际执行 |
| 真实 PG 双事务 | 撤权先提交后拒绝；已准入事务阻塞撤权；锁等待后过期拒绝；拒绝 Repeatable Read 旧快照；调用者事务阻塞责任 disposition，之后责任失效。用 pg_locks 验证真实等待，没有用 sleep 猜串行化 |
| 权限与边界 | 精确检查所有表/操作权限；旧业务历史 UPDATE/DELETE 拒绝；读取前后业务指纹相等；Execution / AgentTask / ActionRequest / provider attempt / Outbox 仍为空；原容器未改动 |

新增 required CI 子工作流在全新 TLS PG + 官方 Keycloak 中重跑身份/责任 PG 和 Chromium 用例；本机 Edge 与 CI Chromium 分别记账。CI 最终结果以精确 PR HEAD 的检查为准，不继承前一提交。已有全站/Agent 回归仍在原 required 组。

全量 Go 回归首次还暴露两处旧测试时序假设：续期“未来”夹具只有五秒、schema marker 的 500ms 同时覆盖子进程启动。分别将未来/过期夹具分开，并在已观察启动成本后取消 hang 场景，保留真实失败/挂起行为，不放宽生产准入逻辑。

NOT_RUN：企业 IdP/真实员工目录、企业责任依据/标准确认、真实收件材料、业务写事务、模型/外部 provider/执行、生产安全与部署。外部 IdP 的撤销在每次 introspection 的检查点生效，本地 PG fence 不会把远端撤销变成分布式原子事务；业务写入未来仍需独立定义检查点和授权窗口。

## 后续边界

进入 1B 前仍须补齐可信业务代表/责任与标准适用的接纳来源、真实收到材料的安全选择投影，以及 Case/回执/事件/审计的独立契约实现。六个测试账号和本地 Port PASS 不替代这些条件。PR #363 保持 Draft，不合并、不部署。
