# 业务签收异常验收矩阵

状态：**DESIGN ONLY**，2026-10-24，基线 `73e6f3f1`。这是待实现验收规范，不是执行报告。来源 UNCONFIGURED、业务写入口关闭；本轮没有启动环境、改库、生成材料、请求模型、执行 Command 或浏览器验收。

权威规则归[可信依据清单](../architecture/business-receipt-trusted-basis-v1.md)、[第一切片契约](../architecture/business-acceptance-draft-receipt-admission-v1.md)；页面行为归[流程稿](business-receipt-page-flow-20261024.md)。D/A/P/H/B 分别表示 Domain、Application、真实 PostgreSQL、HTTP、Browser 验证层次。每行列出未来必需层次，不表示已执行。当前下表所有新写入/可信来源用例的执行状态统一为 **NOT_RUN**；真实来源另为 **UNCONFIGURED**。

## 1. 当前关闭与来源可信性

| ID | 前置与触发 | 必须观察到的结果 | 层次 |
|---|---|---|---|
| R01 | 所有依赖看似齐全，但运行配置保持关闭 | BFF/HTTP 不分发新业务 Command，业务表/事件无增量；页面只读，无启用授权推断 | A/H/B |
| R02 | 目录、代表、标准或材料来源未配置 | 分别显示缺口；不能因 Core 有记录/合成测试通过而标为来源可信 | D/A/H/B |
| R03 | 合成身份/责任/材料用于正式来源门禁 | 正式模式拒绝；合成正例仅隔离测试 Adapter 使用，不暴露生产放行开关 | D/A/H/B |
| R04 | 材料 metadata 声称 verified/receiver/contentSha256 | 不成为接纳决定；记录 hash 正确也不证明附件或接收方，RECEIVED 被阻断 | D/A/H/B |
| R05 | 只有质量报告、认证、发布或 EvidenceSnapshot | 不替代业务收到声明；不自动创建签收或验收通过 | D/A/H/B |
| R06 | 平台记录 hash 损坏、记录超限或依据缺失 | INVALID/NOT_CHECKED/不可见分别展示；无绿色可信结论，不用未核验 hash 放行 | A/P/H/B |
| R07 | 原件内容版本/hash 与接纳记录不符 | 拒绝使用；不能用相同文件名/URI/平台 envelope hash 掩盖内容变化 | D/A/P/H/B |
| R08 | 受控类别不支持、出处/原记录者/收到声明不可核对 | 不降级普通 Evidence 作为可信材料；给出安全的材料不可用原因 | D/A/H/B |
| R09 | 来源断网、检查点过期、版本未知或撤销窗口未接受 | 无新的可用结论；未分发请求不写；以前的 UNKNOWN 不因此变成未提交 | A/H/B |
| R10 | 本地接纳处置先取得共享依赖 fence 并提交，签收等待 | 实际 PG 锁等待可见；之后签收最终重读拒绝，零新签收事实 | A/P/H |
| R11 | 当前签收先完成受锁判断并提交，之后其仍依赖的材料接纳被处置 | 原签收及办理依据逐字段不变；该当前准备依赖受阻，不自动翻转旧 outcome；仅历史专属依赖见 R48 | D/A/P/B |

## 2. 身份、责任与明确对象

| ID | 前置与触发 | 必须观察到的结果 | 层次 |
|---|---|---|---|
| R12 | 伪造 body/header actor、party、status、时间或信任字段 | 严格输入拒绝；不得改变 Resolver 解析的主体或绕过动作资格 | A/H |
| R13 | 登录有效，但非本单 RECEIVER/无动作能力/代表 party 不匹配 | 拒绝新签收；READ 不等于写资格，管理员或资产 owner 不能代签 | D/A/H/B |
| R14 | actor/grant 或本地责任撤销与新签收竞争 | 同序 fence 与新数据库时点决定结果；撤销先提交则拒绝，历史不改 | A/P/H/B |
| R15 | workspace A 的人读取/提交 B 的 case、receipt 或 evidence | 身份/空间及父子关系检查拒绝，不暴露 B 内容，不展示旧空间缓存 | A/P/H/B |
| R16 | 材料只有版本关联/只有交付关联/其他交付或版本 | 精确双关联及接收主体条件不满足，引用不可用；不 fallback latest | D/A/P/H/B |
| R17 | 多个责任候选，或原冻结责任被替任 | 复用完整判定得出冲突；不先过滤后挑人、不自动接管旧单或未决请求 | D/A/P/H/B |
| R18 | BLOCKED/DEMO 交付或无 membership，尝试建单 | 拒绝创建；不得通过执行真实交付、改状态或匹配名称造正例 | D/A/P/H/B |
| R19 | 只有 ISSUED，但无收到证明 | 可作为既有目标资格的一项，不能自动得到 RECEIVED、准备齐全或验收通过 | D/A/B |

## 3. 签收结果、标准与历史

| ID | 前置与触发 | 必须观察到的结果 | 层次 |
|---|---|---|---|
| R20 | RECEIVED 无原因/无可用材料，或夹带一个无效引用 | 拒绝整次新请求，无部分保存、不静默丢弃坏引用 | D/A/P/H/B |
| R21 | PARTIAL/MISSING/DISPUTED，有原因且自身身份/责任有效，无材料 | 未来已启用环境允许登记但准备仍不齐全；当前关闭环境仍不得分发 | D/A/P/H/B |
| R22 | 标准失效，但接收方合规登记 MISSING/DISPUTED | 不把标准确认为收到登记前置；准备仍阻断，旧确认及签收不删除 | D/A/P/B |
| R23 | 无 COMMITTED 原回执的新请求，首次 replacesReceiptId 非 null，或补正不是当前 ID | 拒绝或返回明确冲突；不追加到错误链，不覆盖原记录；旧成功重放按 R46 | D/A/P/H/B |
| R24 | RECEIVED 补正为 DISPUTED | 新事实引用旧 receiptId；准备变为阻断，原 RECEIVED 和其材料成员不变 | D/A/P/H/B |
| R25 | 两个窗口同 expectedRevision/原 receiptId 同时补正 | PG 竞争最多一成功，另一明确冲突；不静默合并、不刷新 revision 自动重发 | A/P/H/B |
| R26 | UPDATE/DELETE 原签收，或 late member INSERT 改材料集合 | header 与成员均拒绝历史改写；若采用 finalize，验证共同 parent lock 下的竞争 | P |
| R27 | 草稿追加修订，即使新旧内容 hash 相同 | 旧确认不再匹配新 DraftRevision；签收事实保留，需重新确认草稿 | D/A/P/B |
| R28 | 签收补正但草稿未变 | 不改草稿 hash 或伪造新标准确认；重新计算准备条件 | D/A/P/B |
| R29 | 草稿确认与签收先后顺序互换 | 两条路径均可独立办理；二者齐全仍为 DRAFT，无 READY/提交/通过 | D/A/P/B |
| R30 | 无 COMMITTED 原回执的新写，对已有非 DRAFT case 发送 Revise/Confirm/Record | 状态冲突；原成功回执恢复见 R46。Create 产生新 DRAFT，可选 previousCaseId 仅追溯，不继承旧单状态或取消旧单 | D/A/H/B |

## 4. 幂等、回执与恢复

| ID | 前置与触发 | 必须观察到的结果 | 层次 |
|---|---|---|---|
| R31 | 同主体/空间/命令/target/key 同内容重放或并发 | 相同不可变回执及事实 ID；事件、Audit/Evidence/Outbox 不重复，无第二份签收 | A/P/H/B |
| R32 | 同命名域/key 异 body；或跨人/跨空间猜回执 | 异内容冲突，越界不可读；不把已有成功套到新意图 | A/P/H/B |
| R33 | 事务中任意事实/事件/回执持久化失败 | 整体回滚，无孤立指针或部分事实；不把应用日志当审计成功 | A/P/H |
| R34 | commit 成功后响应丢失/服务重启 | UNKNOWN 锁定原意图；只读原回执恢复同事实，不自动 POST 或换键补建 | A/P/H/B |
| R35 | 回执 GET 404，原命令仍在途 | NOT_OBSERVED，不认定未执行；仅查询，不因 404 自动 POST 或换键；受控恢复的运行/权限条件见 R47 | A/P/H/B |
| R36 | 回执 COMMITTED 但主体、指纹、factIds 或 case 不匹配 | 不标记成功、不清除原意图；保持 UNKNOWN 并安全核对 | A/H/B |
| R37 | 匹配回执已确认，详情 GET 失败 | 成功回执保留，详情 UNKNOWN；只读刷新，不重复保存 | A/H/B |
| R38 | 旧请求 UNKNOWN 后收到 401/403/422 或不确定 5xx | 不倒推旧请求未提交；保留原键/body 与待恢复状态 | A/H/B |
| R39 | 写资格撤销但 READ 尚在；随后 READ 也撤销 | 前者可恢复本人旧成功回执但不能新写；后者停止读取，无共享凭据/演示兜底 | A/P/H/B |
| R40 | BFF 意图保存失败、恢复记录丢失或换人登录 | 保存失败则零分发；丢失不伪造键/body；新主体不得接管，未能核对继续 UNKNOWN | A/P/H/B |

## 5. 页面与后续生命周期边界

| ID | 前置与触发 | 必须观察到的结果 | 层次 |
|---|---|---|---|
| R41 | 精确历史/消息回跳、刷新、标准和材料分页返回 | 保留相同 workspace/目标/记录 ID/页码；失效引用不替换最新；消息不携带办理权限 | H/B |
| R42 | 切换空间/case 后旧请求迟到 | 不显示旧上下文或恢复旧表单；原未决意图仍留原主体范围 | A/H/B |
| R43 | 390px、键盘、长 hash/ID、字段错误与禁用状态 | 无横向溢出；返回/说明入口可达、焦点可见、错误与控件关联 | B |
| R44 | 未来冻结提交后签收补正、撤回/审核竞争、归档后回跳 | 原 submission/decision/archive 精确引用不变；按总契约处理，第一切片不提前实现这些动作 | D/A/P/H/B；后续切片 |

## 6. 本轮契约一致性回归

以下补充仍为 NOT_RUN，不增加 Command、状态或运行授权。

| ID | 前置与触发 | 必须观察到的结果 | 层次 |
|---|---|---|---|
| R45 | 来源只读见证/首次建单前，没有本单 DraftRevision、BasisConfirmation 或 Receipt | G2 只核对来源及目标，不要求后置本单确认/签收或写能力已发放；未来 Create 只按自身依赖产生 DRAFT，当前仍不分发 | D/A/H/B |
| R46 | 已有匹配 COMMITTED 回执；case 非 DRAFT、旧 revision/原签收已陈旧，材料来源不可用或写资格撤销，当前本人仍有 READ | GET 恢复原回执/原 resultingRevision/原事实 ID，无新业务写；已注册且允许分发的原键重放也先查成功回执再判断新写条件。READ 失效则拒绝；全局关闭只用 GET | A/P/H/B |
| R47 | 持续 NOT_OBSERVED，从可信恢复记录显式恢复原请求；原命令可能仍在途 | 关闭/未获运行授权/无当前新写资格时零 POST；未来启用且获资格后仅原人/空间/命令/目标/key/body，原键锁重查，最多一份事实。陈旧 CAS/来源不自动改写或换键，旧 UNKNOWN 不能仅凭后来拒绝清除 | A/P/H/B |
| R48 | r1(e1) 已补正为当前 r2(e2)，之后仅 e1 的接纳失效 | 不向无 e1 依赖的 r2 增加 e1 blocker；其他当前条件仍核验。若 r2 仍依赖 e1 则阻断，不静默丢成员。r1 和旧提交/档案不变，旧 submission 的 receipt 匹配规则仍按 R44/总契约 | D/A/P/H/B |

## 7. 将来执行与留证要求

- 每行记录具体提交、profile、数据库/空间、主体映射、预期与实际结果、证据位置；PASS/FAIL/UNKNOWN/NOT_RUN 分列。设计静态核对不能填成 PG 或 Browser PASS。
- 先 Domain/Application/Adapter，再真实 PG（含锁等待、反向竞争及不可变 header/member 指纹），最后浏览器。浏览器单窗口提示不代替 DB 并发证明；一次绿色状态不证明来源接纳。
- 合成正例仅在新独立验收库和测试来源 Adapter 中运行；正式来源门禁的 Synthetic 拒绝独立负测。企业来源只读试点、正式业务写入、模型/真实执行分别记录授权与证据。
- 当前已有只读回归见[1B 前置实施记录](acceptance-business-basis-read-implementation.md)，不将其 Edge/PG PASS 延伸为本表新写入 PASS。CI 绿也不自动改变[启用结论](business-receipt-write-enablement-checklist-20261024.md)。
