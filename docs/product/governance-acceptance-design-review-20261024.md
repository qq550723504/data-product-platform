# 草稿与签收设计：有界复审及 CI 修复

日期：2026-10-24。审查输入为 PR #363 的设计提交 `33180e110ad815c70cc05e236271d3be1ead1bf6`。本轮仅修正文档与一个 CI job 的时间预算，不开发业务功能。原库、独立验收库、Agent API、只读预览与业务开关均不操作；不接模型/provider、不派发真实执行，保持 Draft、不合并。

## 权威与有限范围

- [第一切片设计](../architecture/business-acceptance-draft-receipt-admission-v1.md)：身份、责任、仅 DRAFT 的四个 Command、接口/回执、事务与恢复。
- [总契约](../architecture/business-acceptance-backend-contract-v1.md)：后续提交、撤回/取消、审核、归档及决定作废。
- [已实现只读契约](../architecture/acceptance-binding-read-contract-v1.md)：代表/责任/标准适用/处置及当前判定；不是新写入授权。
- [接入计划](governance-business-backend-plan-20261024.md) 和 [评审契约](governance-preview-merge-contract.md)：分阶段交付、已实现与设计的区分、运行及合并边界。

对照实现为 contract/domain 的 acceptance.go、acceptance_bindings.go，当前 Principal Resolver、transaction/Evidence/Audit/Outbox；身份候选只按第一切片文档注明的独立冻结提交核对，不修改或合并其 checkout。

## 发现与处理

| 项目 | 原问题与影响 | 本轮处理 |
|---|---|---|
| D01 标准格式 | 详细设计将“ID 为 UUID”泛化到 criterionId，并提到来源 description；现有来源使用业务编码 id 与 clause/text，按旧文档实现会拒绝合法标准或产生第二来源格式 | 平台对象才使用非 nil UUID；criterionId 精确引用来源 id，保留既有编码、大小写、完整集合、clause/text 字节限额与 method/evidenceCategory 校验；来源字段不改写 |
| D02 回执鉴权顺序 | 设计同时要求新写动作能力和历史成功回执只需当前读取资格；未明确入口分支，若先按写能力 Resolve，会错误阻断已提交请求的恢复 | 入口及 fence 后先校验 READ，再核实本人/空间/目标/原键/指纹的 COMMITTED 回执；无回执的新请求才要求动作能力、责任、状态与 revision；没有写权限不能靠旧键创建新事实 |
| D03 判定复用 | “当前唯一有效绑定”可能被误解为先滤掉无效代表后挑人，与现有多候选 fail closed 的顺序不同 | 明确复用 EvaluateAcceptanceBindings，完整范围先数 ACTIVE 责任候选，再检查代表依赖；不从历史页/当前调用者反推集合 |
| C01 trial CI | 同一 Windows job 顺序运行普通与 -O 完整套件，5 分钟包含 checkout/setup 和两轮；正常轮已用约 146 秒，第二轮被 job 预算截断 | 仅将 timeout-minutes 从 5 调整为 10，保留两条完整命令、135 项当前套件及现有 job 名称/权限；新提交重新验证两轮均完成 |

原失败证据保留：[trial run 40081135224](https://github.com/qq550723504/data-product-platform/actions/runs/40081135224)、job 120834761585。普通模式日志为 `Ran 135 tests in 145.514s`；check annotation 明确为 `The job has exceeded the maximum execution time of 5m0s`，-O 被取消，不能算 PASS。未发现该日志包含业务断言失败；未通过重跑旧失败或跳过 -O 掩盖。Node action runtime 的一般弃用提示不是此次取消原因，不在本增量升级 action。

## 依赖闭合核对

| 主题 | 结论 |
|---|---|
| 来源与状态 | 第一切片只办理 DRAFT；总契约的 RETURNED/PENDING/ACCEPTED、撤回/取消/归档仍属后续。标准适用、草稿确认、收到声明、验收通过分别记录，无 EvidenceSnapshot 代验收 |
| 历史 | 草稿、确认、签收、办理依据及命令回执追加；当前指针/revision 是受控投影；后续冻结 parent-first、处置不覆盖历史 |
| 身份与空间 | 逐用户 OIDC 候选未集成；新增固定 profile/有限能力均是未来设计。canonicalSubject 与外部 sub 区分，责任/成果/回执精确 scope，无共享 token 或演示身份 fallback |
| 并发 | IAM→业务绑定→幂等→case→来源/成员顺序；持同事务 fence 后重读当前事实。既有只读快照和入口 introspection 不声称已满足写入保证；远端 IdP 检查点边界明确 |
| 幂等与恢复 | READ 重放与动作准入分支一致；CAS 不静默更新；NOT_OBSERVED 不表示原事务未提交；原 UNKNOWN 不因后续拒绝被清除；无自动 POST/换键补建 |
| 跨 owner 依赖 | IAM 的 Tx-scoped Port、迁移/模块协调、可信责任/收件材料来源、业务 Command/不可变约束仍由详细设计门禁跟踪。未实现/未启用被明确记录，不扩展成此次开发任务 |
| 复用 | 复用 Principal、当前绑定 evaluator、ContractVersion 来源、事务、Evidence/Audit/Outbox 与既有 JCS；不新增 IAM/RBAC/队列/通用工作流，不改 Agent 事实或 API |

本轮是作者的有限契约复核，不等于新增独立全 PR 批准、企业 IAM 或业务写入验收。原 CI 的取消保留为失败证据；新 HEAD 的全部 CI 结果在 PR 的精确提交检查与收口记录中确认，不能凭本文件宣告通过。

静态验证覆盖 Markdown 链接/编码、来源字段和编码规则对应、diff 范围、workflow YAML 及仅 timeout 变化。未运行本地业务 API、DDL、浏览器或试点脚本；新增写入能力全部仍为 DESIGN/NOT_RUN。业务源码、数据库、运行进程与开关不得因 CI 成功自动改变。
