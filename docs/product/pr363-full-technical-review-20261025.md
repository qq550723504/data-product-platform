# PR #363 冻结候选与独立全量技术审查

2026-10-25。结论：冻结候选全量静态审查覆盖完成，**确认的阻塞缺陷0；两项P3已最小修正并由独立审查者复核**。这不是企业来源、正式签收、生产运行或合并批准。PR保持Draft，不合并、不部署。

## 冻结对象与覆盖

- 冻结候选：`8495de4e1bb3100e9cf4e73760caaddc02264153`；PR比较基线：`e0e4f77a576ea4572cf16b3a3475461127f44acc`。
- GitHub分页文件清单与本地三点diff一致：**378个变更文件**。[原冻结清单](../acceptance/evidence/pr363-full-review-20261025/candidate-original.json)的LF规范化SHA256为 `b2f4422eb8b8bbe03d1a7c08072cb1df1b5b7e8b2dfadc5b883490640bacbc66`；冻结时Windows CRLF原始字节SHA256为 `e2b5c9774703ed1d0d8ffb44a7454e9b19ef2bc7fc9df45e33a6abe22c61bf80`。跨系统核对前者时只把CRLF转换为LF，不重排JSON。
- [逐文件覆盖清单](../acceptance/evidence/pr363-full-review-20261025/candidate-coverage.json)保存每项路径、冻结Git blob、工作树字节SHA256、审查分组及审查方式。工作树摘要不是运行构建身份认证。
- 三个独立子审查者从冻结源码开始阅读，未继承作者的旧审查结论；作者负责整合与复验。分组不是四位审查者，Agent与集成由同一位独立审查者完成。

| 分组 | 文件数 | 独立审查者 | 原始记录 |
|---|---:|---|---|
| 身份、责任、业务依据、Trace/历史及SQL74–76 | 113 | identity_review | [身份审查](../acceptance/evidence/pr363-full-review-20261025/identity-review.md) |
| Agent Q1/Q2、计划/意图/恢复、迁移与worker路由 | 54 | agent_review | [Agent审查](../acceptance/evidence/pr363-full-review-20261025/agent-review.md) |
| API组合、只读配置、预览/准备脚本、依赖/CI及证据查询 | 59 | agent_review | [集成审查](../acceptance/evidence/pr363-full-review-20261025/integration-review.md) |
| 新版Web、空间/演示历史、BFF客户端、浏览器夹具及交付材料 | 152 | web_review | [Web审查](../acceptance/evidence/pr363-full-review-20261025/web-review.md) |

所有分组均核对其冻结摘要，无未分配文件。328份文本/源码/SQL/测试/文档按变更及相关上下文阅读；50张PNG按资产身份/摘要归类，**不把此项称为50张独立视觉审查**。Web审查者另目视检查了修复前后的05截图。下列修复和交付索引增量另行复核，不把旧候选的结论自动套给变更文件。

## 四条重点边界

| 边界 | 审查结果与依据 | 实际限制 |
|---|---|---|
| 可信身份 | OIDC/RP复用现有SDK；按请求校验issuer/client、当前映射/修订/有效期；Web逐用户会话不回退共享token。API在读取前校验principal，写入不使用部署读凭据代办 | 只验证现有技术契约与合成身份；真实企业目录/代表授权未配置 |
| 工作空间隔离 | principal允许空间、根对象真实归属、子对象和Evidence关系分别核对；Trace和依据查询复用同一只读事务；前端精确ID失败显示UNKNOWN | 前端演示分桶不等于真实多租户授权；跨请求历史分页不冒称同一冻结事务 |
| 历史不可变 | 计划成员与Finalize共享parent锁、DRAFT不可提交；命令/回执/审计/证据/outbox同事务；签收补正、提交、决定处置与归档分别保留；绑定/快照历史不按latest覆盖 | 业务签收/验收/归档当前只有前端合成闭环，服务端Case写入仍是设计；静态审查不替代所有并发调度的执行证明 |
| 写入与执行关闭 | Agent API与BFF默认关闭写开关；只读预览在Service前拒绝命令；正式Case/Receipt写入口未实现/注册；worker不声明未实现的Agent执行消费者，冻结obligation不会被静默确认 | 不关闭或替换原有已授权Agent API；不把计划/意图或历史成功当作模型、owner执行或当前交付授权 |

## 两项修正

1. **W-N1 / P3，修复编辑页重复缩进和嵌套landmark。** 外壳已有main，编辑页又返回main，命中通用侧栏布局选择器。仅将内层包装改为section，保留class、字段、事件、权限和保存门禁；没有改共享CSS或业务状态。重演新增“页面只有一个main”的断言。对比[冻结截图](leader-demo-20261025-reviewed-final/05-repair-readonly.png)与[修复后截图](leader-demo-20261025-technical-review/05-repair-readonly.png)。
2. **W-N2 / P3，历史文档未标明被替代。** 在2026-10-23验收记录顶部补历史声明，指向当前只重写根页面、保留Core深链的既有权威契约；原批次内容和证据保留，没有恢复全路径重写。

本轮未发现需要改动后端安全/状态契约的确认缺陷。其余观察按AGENTS第17节收敛为验证限制或既有后续范围，不增加IAM、队列、派发器、来源接纳或业务写实现。

[独立增量复核](../acceptance/evidence/pr363-full-review-20261025/web-delta-review.md)确认两项修正解决，并核对当前交付索引、覆盖清单、原始清单换行摘要和本地验证摘要；没有发现授权、历史或写入边界回归。源码复核、截图摘要与作者实际执行证据分别记录，不声称审查者启动过运行时。

## 本轮验证

| 验证 | 结果 | 证据边界 |
|---|---|---|
| Go安全单测 | 5包通过，89个测试及子测试PASS，0失败/跳过 | Agent领域/HTTP、质量HTTP、OIDC及默认只读preview；只用内存/httptest，无真实PG/IdP |
| Web IAM | 17 PASS，2真实PG用例SKIP | 绝对到期拒绝、映射撤销、CSRF/固定跳转、只读GET；跳过不能算PASS |
| Web Agent | 20/20 PASS | 包括实际控制器的UNKNOWN、原键恢复、确认成功后GET失败分离 |
| 独立Next构建 | PASS，含类型/lint | 新目录 `.next-acceptance-identity-leader-tech-review-20261025`，不覆盖原构建 |
| 独立Edge重演 | 21/21 PASS，16张PNG | [原始JSON](leader-demo-20261025-technical-review/acceptance.json)；浏览器业务API写请求0、外部请求0、未捕获错误0；包含单main、来源回跳、签收异常/历史/空间隔离 |
| 冻结基线CI | ci2385的14/14、source95和trial85全部completed/success | 精确8495de4e的既有隔离CI，本轮读取状态确认；浏览器128/128、真实PG/Core由该CI负责，不能当本轮新建库执行 |

[本地验证摘要](../acceptance/evidence/pr363-full-review-20261025/local-validation.json)与四份独立报告分别记录谁执行了什么。独立审查者只做源码阅读，未启动服务、数据库或浏览器；作者的单测、合成Edge和CI核对单独留证。修复后提交的CI须按其精确HEAD重新完成，最终PR检查点承接该结果，不继承旧SHA绿色或宣称main/push成功。

新截图包独立生成；原演示包、中间包及失败证据不覆盖。4193预览先保留旧运行直到新构建通过，再由原脚本核对PID/精确创建时间/命令/监听端口，只更新清单中的4193/4493两个演示进程。没有读取或写入原库、启动真实Agent服务或修改其身份环境；不要把这句话解释为新一轮双库fingerprint验收。

## 后续与收口条件

企业来源、真实场景、接收组织及责任团队仍待定；[业务确认与启用清单](business-receipt-write-enablement-checklist-20261024.md)的G1–G8没有因本审查完成而通过。远端撤销窗口、真实原件/材料接纳、Case/Receipt写入和Agent Q3/Q4仍属后续范围，当前不实现、不运行。

追加的特定PG并发调度、Agent未实现handler的独立dispatcher集成用例和更强CREATE回执相关性仅作为原始报告中的后续验证建议：审查未复现其导致当前授权/历史不变量被突破，不能把“还可加测试”变成无限扩张当前范围。若后续出现具体反例，再按现有merge contract分类为BLOCKER或FOLLOW-UP。

本轮技术闭包：确认阻塞0、两项P3修正完成；证据范围、未来门禁与未运行项明确。保持Draft、不合并、不部署，来源UNCONFIGURED、业务写DISABLED，模型和真实执行NOT_RUN。
