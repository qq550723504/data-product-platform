# #307 D1：当前 PR CI、17版本只读验收及六目标执行缺口

本轮只收口 PR CI、刷新三源全量只读证据、列清下一次执行条件。没有新删除候选、删除 execution、窗口、调度或授权；不重开 v27。#332 保持 OPEN/Draft/unmerged，#307 保持 OPEN，完整 D1 仍 NOT_ACCEPTED。

## 已完成的有限验收

| 项目 | 最新事实 |
| --- | --- |
| 入场 HEAD 的 PR CI | `15afaac1306f1f4190b7813bf93c762e77c73d25`，run `39563318843`，13/13 SUCCESS，event=pull_request；不是 main CI。原始读回见 `evidence/issue307-d1/current-head-ci-15afaac1/result.json` |
| 新只读审计 | `post-v27-seventeen01`：`PASS_CURRENT_SEVENTEEN_NATIVE_READONLY_SEALED`；原生实际读取时间 2026-10-21T07:52:53Z–07:53:09Z（新加坡15:52:53–15:53:09） |
| 版本与库存 | 三组分别7/6/4，共17份：6目标+11保留。完整 bytes/SHA、精确版本、namespace 全库存、hold/retention 均通过；P1 真正 NoSuchVersion/404，永久排除 |
| 身份与策略 | 29个已知身份全部 disabled；28 sole-policy/no groups、唯一 retained partial 身份严格 NONE/no groups；未分配 policy 独立 Equals；9个未创建用户名 fresh NoSuchUser。采用官方 ParseConfig/Equals，未把顺序不同的 raw SHA 当策略漂移 |
| Core | 三源所有关系、roles/ACL 的 REPEATABLE READ READ ONLY 指纹前后相同。business 使用可信闭合后的 `ffdf75013a393da6a8444b28284fdfcc47b5a01a346f7fdf25a8c03138cd8a7f` 检查点；另两源使用原冻结检查点，不覆盖旧基线 |
| 历史保留 | 18个旧预算卷的 tar SHA、完整挂载CID集合、531份旧配置/备份文件、91份失败维护文件、已结束维护03全部原字节不变；不 release pins、不退款预算、不重发 |
| 原失败证据 | 旧两个索引的270个原HEAD成员与 Git 原字节匹配；252份历史证据 payload 的当前字节仍相同。维护入口/CI 后续修改与历史快照分开，不要求新源码伪装成旧源码 |
| 技术请求与成本边界 | 实际51+40+34=125个 GET/HEAD 请求 start/observation 完整保留，未观察 attempt=0，amount=NOT_ASSESSED。不是零技术请求；本轮无业务 provider DELETE，无 Core 业务写入 |
| 关闭 | 原620个CID+3个新受限读取owner，共623个 stopped/PID0/noports/no-restart；新3只读配置均 false；旧发送权限与身份从未开启。cleanup=hold-release=bypass=0 |
| 新入口回归 | 共7方法 normal/-O 各通过：新只读完整三组合、第二组失败停止后续、保留未观察请求、旧终态/授权扩张/预算扩张/基线或源码漂移拒绝；公共 collector 仍验证 bound Core 前后不变 |

权威原始回执为 `evidence/issue307-d1/post-v27-seventeen01/terminal.json`、三组 `files/six-native-verification.json`、`Core-before.json`/`Core-after.json`、`final-closed-and-history-readback.json`。全部17精确版本与六目标/11保留的完整身份分别列于 `current-inventory17.json`、`six-target-readiness.json`。

## 六目标当前状态

| 目标 | 精确源版本 | 本轮原生结果 |
| --- | --- | --- |
| P2 | dpp-lifecycle307-online/fixture/p2.csv@d8040608-c5cd-4a6f-8a2d-b3f856cf5302 | PASS / hold=false / retention=NONE |
| T1 | dpp-lifecycle307-online/fixture/t1.csv@39d4ef5d-e983-413d-a6c2-9759d456b750 | PASS / hold=false / retention=NONE |
| T2 | dpp-lifecycle307-quarantine/fixture/t2.csv@90bd8ded-337a-4ca1-aec3-66c4ee6a1314 | PASS / hold=false / retention=NONE |
| H2A | dpp-lifecycle307-online/fixture/h2-a.csv@453c6f42-fbb2-4fdf-9f86-00a20a54b7bc | PASS / hold=false / retention=NONE |
| M1A | dpp-lifecycle307-online/fixture/m1-a.csv@e8a8418e-c515-464e-99e5-10bc4c64fa8d | PASS / hold=false / retention=NONE |
| M1B | dpp-lifecycle307-quarantine/fixture/m1-b.csv@54900849-d59d-459e-90bb-275a5975d0a6 | PASS / hold=false / retention=NONE |

这些是同一有限批次的六个目标。当前库存/保护没有发现阻止准备下一方案的漂移；六份真实 DELETE 仍全部 NOT_RUN。

## 执行前还缺什么

| 条件 | 当前状态及完成标准 |
| --- | --- |
| 新运行候选及全部冻结绑定 | NOT_PREPARED。将已验证修复纳入新 source/binary/reader/checker/host 接线；以本次 post-closure Core 与17版本为新准备输入。新 candidate/profile/execution、CID、config、IAM、active/sealed gateway、备份、target digest、审计索引须一次性闭合；旧候选只作历史证据 |
| 独立新预算与最小权限 | NOT_PREPARED。六目标每份最多一次、总计最多六次的新持久 target/wire budget，精确归属新 execution、完整挂载集合、首次发送前为空；18旧预算与其已消费/未消费事实全部保留。新身份默认 disabled，权限默认 false，禁止沿用旧 sender/wire 权限 |
| 新 execution 的 Core 准备 | NOT_RUN。P2/T1/T2 三个旧 dispatch 的 NOT_SENT+exact release 已完成，unresolved=0；新执行只追加自己的 fixture basis/批准/dispatch，不复活旧撤销批准或 attempt。复用已完成 Processing/DirectData、可信owner/handles/pin/ref闭合及 P1 absence，不重跑旧生产/传输，不新增 outputs，不 Renew 旧 policy |
| 新完整非删除演练与独立评审 | NOT_RUN。对新候选的实际完整入口、签名、逐slot前序absence、sender/wire/recovery组合、失败封存与授权绑定复评；本次只读 PASS 与先前有限修复评审不替代新候选验收 |
| 新固定窗口与人类精确授权 | NOT_SELECTED / NOT_RECEIVED。新方案必须先展示新 execution、所有冻结绑定、六目标/11保留、窗口、次数及 UNKNOWN 不重发限制，再获得新授权。旧v27窗口已失败结束，不能延长、平移、重开或复用授权。本轮未预约窗口或建立调度 |
| 窗口内新鲜完整前置 | NOT_RUN。新候选精确HEAD的必要PR CI、Draft/Open/评审线程、三Core、17版本/完整库存/保护、disabled IAM/官方策略语义、备份、真实CID/挂载/网络/权限与预算须再次核对。当前只读快照不是未来发送授权，也不承诺未来无漂移 |

实际执行顺序仍须 P2→T1→T2→H2A→M1A→M1B，每slot `send → disable-send → 选定Outbox→既有Asynq只读recover → Core immutable absence → 下一slot`；每个sender/wire/recovery前各自≤1分钟fresh实际宿主证明，wire context≤3分钟且逐target刷新。SDK无重试，预算先于Core claim；UNKNOWN不重发。

| 特殊案例 | 仍待实际运行的验收条件 |
| --- | --- |
| H0/H1 | 全程零send。实际owner成功终止、case仅撤销Enabled、late权限false、enabler停止、固定disable-all、≤45秒fresh disabled读回与sealed gateway之后，才一次可信签名并在≤1分钟内关闭H1为NOT_SENT；不能填布尔值或自动重签 |
| H2A | 仅A一次，真实上游204之后才witness、A+B late Core hold、撤销及响应丢失，形成预期CONFIRMED UNKNOWN；B永不dispatch；随后仅既定只读恢复链 |
| M1A/M1B | A确认absence后才B一次；形成B CONFIRMED UNKNOWN与C-only保护阻断mixed checkpoint；原checkpoint复制并核SHA之前禁止B lookup/全namespace query；C永不删除 |

## 复用与证据边界

复用已有 `verify-six-native.py::collect`、冻结 GET/HEAD-only reader、官方SDK策略语义与 Windows file-only checker，未增加第二套队列/工作流或存储实现。只为新零删除审计向公共 collector 显式传入精确SHA约束的 post-closure Core 基线；原删除执行入口仍使用自己的冻结基线、终态门禁和原授权规则。

新读取profile的 AuditID/有效期独立生成，MaxProviderDeletes=0；reader内部的历史map/execution字段仅绑定原三源身份，不授予运行原删除execution的权力。原v27终态、授权、模板、预算及pins均原样保留。

两次发布辅助检查错误分别为错误导入相对路径及误把旧字符串SHA索引当metadata索引；都发生于原生验收完成之后，未启动source/identity/provider，单独保留 `publication-helper-failure01/02.json`。随后按两份旧索引的实际字符串SHA contract核对原Git快照及当前历史payload，未修改或重跑已成功的原生审计。

入场HEAD CI通过后，本轮新维护接线及证据的提交HEAD须单独核对对应PR CI，不能用15afaac1绿色冒充新HEAD绿色；具体最终CI读回另存追加回执，不改本次原生terminal或旧索引。全程真实删除0，#332 Draft、不合并，#307 OPEN。
