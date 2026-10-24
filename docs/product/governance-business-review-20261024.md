# 业务签收、验收与归档增量审查

## 审查边界

起始 HEAD：`e6c4f3f6d205661ae842db371d56bef30308a26b`，PR #363，Draft。范围为交付只读增量 `5687fb6e` 之后的业务演示、菜单、历史追溯及其测试，重点核对状态边界、历史不可变性和工作空间隔离。

当前演示契约由 `governance-business-acceptance-demo-20261024.md` 拥有，异常路径详见 `governance-business-exceptions-20261024.md`。沿用 `governance-preview-merge-contract.md` 的 Core/Agent 边界。复用现有 DemoProvider、workspace/sessionStorage、显式 businessCommand 和浏览器夹具，不新增基础设施、后端 Command/API 或 migration。

## 发现及修复

1. **P2：草稿保存可削弱必需标准。** SAVE 原先验证条目 ID、文本及来源后，直接采纳整个 criteria 对象；在模型边界提供 `required:false` 或缺少 required，可使后续逐项通过条件变空。页面没有提供此开关，但模型契约不应扩大表单能力。现在拒绝改变必需性或核验方式，只复制明确可编辑的内容及来源，并规范化首尾空白。负例在修复前实际失败（未抛出预期异常），修复后通过；不是 Core/IAM 授权漏洞修复。
2. **P2：签收表单切换验收单时残留上一单输入。** 原选择处理器清空说明，却保留未保存的签收依据及收到情况，可能把另一单的草稿材料带入本单。现在切换对象同时清空依据并恢复收到情况默认值；已登记历史不变。新增浏览器用例覆盖两单分别登记、单间草稿失效及精确历史展示。
3. 同步原页面契约中的退回条件：必需标准未满足，或明确签收依据在提交后变化，均可说明原因退回；不要求为签收变化伪造 FAIL。

## 状态与历史核对

- 草稿/退回后可以编辑、追加签收并冻结新提交；待验收可以撤回，指定验收人可以退回或通过。签收变化阻断通过，重新提交才能冻结新依据。
- ACCEPTED、已归档、决定已作废及 CANCELLED 均拒绝 SAVE/RECEIPT/SUBMIT/WITHDRAW/RETURN/APPROVE/CANCEL，不通过编辑或撤回复活终态。通过后归档仍是独立命令；作废是追加 disposition，重复归档不增加档案。
- 单元回归对四类终态输入递归冻结，逐项执行禁止命令并比较完整原状态；后续作废前后的 receipt/submission/decision/archive 数组必须逐项相等。
- 提交冻结当时的标准和签收；退回/重提生成新提交及新决定。归档冻结明确提交和决定，查询后续处置不重写档案。历史追溯按明确 ID 定位，没有使用 latest 替换。
- Core 的 `/core/archives` 继续表示证据档案；本地业务验收档案不成为 EvidenceSnapshot，也不把证据快照或交付 ISSUED 当成业务验收通过。

## 工作空间核对与回归

DemoProvider 按 `dataKey(workspace.id, key)` 保存业务及详情/追溯上下文，AppContent 按 workspace.id 重新挂载；工作空间切换清理未保存表单和核验输入，原空间已保存事实留在原键下。

新增浏览器用例刻意让两个空间使用相同 `DEMO-ACC-1`、签收和提交编号：企业空间已归档，园区空间仍有独立待验收提交；企业空间作废决定后，原档案不变，园区状态不受影响。跨空间来回切换及刷新核对各自记录、办理身份，未保存说明/PASS 不随切换恢复。每个演示浏览器用例断言 fixture 业务 API 请求数为0。

这是标签页合成状态的隔离和不可变性验证，不是服务端身份认证、租户授权、数据库冻结或跨客户端 CAS 的证明。后端写接口、真实标准/成果选择器及服务端业务验收事实保持后续范围，本轮不引入并行实现。

## 验证记录

- 修复前新增模型负例：8 PASS / 1 FAIL（SAVE 未拒绝改变 required/method）；修复后业务模型9/9 PASS，全部演示测试37/37 PASS。
- `npm run lint` PASS。完整 Next 构建 PASS，包含既有前端回归、编译与类型检查，使用独立 `.next-business-review-20261024`，日志保存在忽略的 `.data/business-review-build-20261024.log`；新增提交的 CI 结果按本轮实际完成记录在 PR，不继承 e6c4f3f6 的112项通过。
- 浏览器增加两项：同空间换单清理未保存签收材料；跨空间相同编号的归档/作废/待审及未保存输入隔离。纳入既有 browser-contracts，不增加 sleep、自动重试或放宽旧断言。
- 本机浏览器预检发现 `tests/browser/node_modules/@playwright/test/cli.js` 不存在，在启动任何 fixture/Next/browser 进程前停止；本地新增浏览器为 NOT_RUN，实际运行交由既有 PR CI，不能用静态审查或语法检查代替浏览器通过。

本轮没有操作保留的数据库、API、网关或原 UI 进程，没有接后端业务写接口、模型、provider 或派发真实执行。保留原库和 Agent API；PR 保持 Draft，不 merge/deploy。此为作者的有界增量审查与修复，不声称整个 PR 新增独立审查批准。
