# #307 引用需求与业务竞争：有限验收

最新阶段（2026-10-14）：独立只读 source review 完成，当前切片 BLOCKER=0；
新隔离环境 `dpp-lifecycle307-5d2274ee` 从源码
`4650c435f8ba89fdb0b2dc0591152d25ded182d7` 完成非删除 native 重验。
下述 a7116c35 原阶段及首次 closure 的 NOT_RUN 记录保留其原始时点，
最新结果见 [本轮回执](../../examples/lifecycle-d1/results/5d2274ee-a671-4bdc-b406-22b8dad8fd61/reference-offline-verification.json)。
Draft、真实 DELETE=0、完整 D1 未接受的边界不变。

## 2026-10-14 独立评审与当前源码 native 重验

独立 reviewer 使用无继承会话的 fresh fork，只读审查 pinned HEAD4650c435
对 main074a33ba 的实际代码/SQL/Adapter。未发现可验证的当前切片代码
BLOCKER；reviewer 未运行 DB/native tests，未把作者自审/CI/旧 binary 当成
invariant 证明。原报告与只读范围见
[independent review](evidence/issue307-d1/reference-independent-review.json)。

已按 review 修正措辞：`StartWithReferenceCheck` 可能先提交 RUNNING，
再由 PinnedEngine 拒绝实际数据面 admission。证明的边界是 restriction
后没有新 GET/bytes/output、输出不被接纳，不能声称每个 Start 状态迁移
都被拒。reviewer 对两处文档修正复核，确认无新增 authoritative contract
冲突；没有因此扩展到通用 Workflow lifecycle 或改动实现。

新 run `5d2274ee-a671-4bdc-b406-22b8dad8fd61` 复用现有 prepare/plane/
reference-races/final/seal 入口，独立内部网络 `10.234.151.0/24`，无 host
port，Core/objects 各专属 named volume。当前 committed source/SQL67
先构建再执行；reader Linux binary SHA256
`5e04e88bbd65880a5e65da5795dcdc116021512e04d27a200ec0d03f83e972c8`。
685 个源码/script/schema 文件的 working SHA256 与 canonical Git blob
分别绑定，运行中不改；Go build settings 未携带 vcs.revision，故不声称
VCS 内嵌证明。见 [source binding](../../examples/lifecycle-d1/results/5d2274ee-a671-4bdc-b406-22b8dad8fd61/reference-source-binding.json)。

PASS（非删除 native）：真实 Entity STANDARDIZED/NATIVE CURATED 两个输出、
Quality/Certification/current rights、processing 输入 GET 后暂停、实际慢 TCP
DirectData 传输与处置两个顺序。活动时两个 dispatch 同时因 references /
readers 被拒；verified stop 后 23 owner releases 使两个 dispatch admission
通过。8 类强类型源关系仍保留；7 pins/7 stops，active=0。
DirectData 129328 bytes，原 ISSUED 保留，transfer observation 追加。
queued/replay/retry 与自然等待 10 分钟 RUNNING lease 后的 recovery、处置后
新 DirectData 为 0 新 GET/output；没有手改 lease/time。

[Native business receipt](../../examples/lifecycle-d1/results/5d2274ee-a671-4bdc-b406-22b8dad8fd61/reference-business-races.json)：
竞争阶段 32 calls/observations/typed costs；final 独立只读 native inventory/
protection 增加 20，总数 52/52/52。
[独立 READ ONLY SQL](../../examples/lifecycle-d1/results/5d2274ee-a671-4bdc-b406-22b8dad8fd61/independent-final-counts.json)
核对成本逐 call 一一匹配、四次 gate evaluation、3 FAILED/1 SUCCEEDED、
1 ISSUED、send claims=0、completion=0，并测得 deployed admission guard
function body（LF SHA256）与绑定 SQL67 完全相同。
首次登记双向 barrier 与错误 trigger 的负例仍属于先前独立 PG 验证；
本轮 native 证明当前修复在真实输出首次登记、全部引用检查和业务数据面
组合中通过，不把 PG/schema fault 注入冒充新 native fault 注入。

[最终离线核验](../../examples/lifecycle-d1/results/5d2274ee-a671-4bdc-b406-22b8dad8fd61/reference-offline-verification.json)
对原四份 identity、六份 exact inventory、保护、calls/observations/costs、
pin owner stops、ISSUED/transfer、源码/binary 作独立比对。
[封存](../../examples/lifecycle-d1/results/5d2274ee-a671-4bdc-b406-22b8dad8fd61/plane-seal.json)：
reader/producer DISABLED、window disabled、gateway DENY_ALL，六份版本和
所有环境/volumes 保留；DELETE/cleanup/hold release/retention bypass 均 0。
本轮开始前的 172 containers/2626 volumes 核对缺失=0。

独立只读计数 query 首次误用了不存在的 completion table，失败且
READ ONLY transaction 回滚；核对真实 schema 后改用 lifecycle_plan_completion
成功。原 operator diagnostic 单独保留，不冒充 native 业务失败或隐藏失败。
实际 DELETE/absence/completion、真实 Redis worker、native process crash、
native pre-ISSUED pause、archive/restore rerun、完整 D1/D2 仍 NOT_RUN /
未接受；这次独立评审不授予 Ready、合并或真实删除权限。

## 2026-10-13 原 native 阶段及随后首次 closure

实现 contract 见 [finite contract](issue307-reference-demand-contract.md)；
base main `074a33bace7edbaf481759aac3a84708363ffd02`，migration67。
受验源码 `a7116c355428ee71ff621870ceafeb6ede9e9205`；
最终文档/回执 HEAD 不冒充重新执行过的 binary。
随后 closure 修正将 metadata 豁免限于 exact public table+column；
foreign 同名表及 control 表新 FK 列的真实 PG 负例 PASS。对封存新 Core
做只读 catalog 对比，分类 delta=0；该修正的 native re-execution 为 NOT_RUN，
证据单列于 [qualification check](evidence/issue307-d1/reference-qualification-check.json)。
本轮 Draft，不 Ready/merge/deploy/close #307；完整 D1 未接受。

## 2026-10-14 closure：首次登记与 guard qualification

本轮发现并修复两个共享 admission invariant 的遗漏：

- `f5476e0b4fa4ece062fc89aaf417ef8af5a943b0`：原 source guard 在未发现
  fixture copy 时不获取 fence；未提交引用可以跨越首次登记。现在所有新
  DatasetVersion 引用先取得其 workspace fence，再查询 copy registry。
  首次 copy 登记使用同一 fence，原引用在 commit 后仍阻断 disposition。
  非 fixture 引用写入也取得该短事务锁，这是首次登记串行化的必要部分。
- `0b88e47c92d69d4b783a90f678d128a79bedccdf`：原检查只看 trigger name /
  function / enabled，缺失参数、错列、仅 INSERT、AFTER 或 WHEN filter 会
  被误认为有效 guard。现在验证完整 BEFORE ROW INSERT/UPDATE + exact column /
  argument + no WHEN contract，不符合即 UNKNOWN，即使当前引用表为空。

修复前：首次登记两个顺序均未到达 parent lock barrier；5 个错误 trigger
配置均被误判 SATISFIED。失败日志独立保留。修复后：两个首次登记顺序观察
真实 PostgreSQL lock wait，原引用仍 DENIED；6 个错误 trigger 配置均 UNKNOWN。
owner release / insertion / pending-copy / exact absence 等 targeted PG race
检查 PASS（91.570s，另加 wrong-update-column 10.574s）；全模块 race 的
无外部服务模式 PASS，未配置 provider/PG 的用例为 SKIP；vet PASS。

本轮只读检查封存 native Core：27 个关系 / 27 个 qualified guards，
classification delta=0、provider I/O=0、mutation=0。native source 仍是
a7116c35；没有将该 Core 升级为新 SQL67，也没有重跑 native binary。
全模块真实 PG 使用冻结 f5476e0b 源码、新隔离数据库与既有专属 named-volume
测试服务，`go test -race -count=1 -p 1 -json ./...` exit0，89 个测试包
PASS（lifecycle application 584.130s）；未配置其他外部服务的用例为 SKIP。
最终 metadata 检查在单独新 scratch DB 验证。具体结果及压缩完整日志
绑定见 [closure evidence](evidence/issue307-d1/reference-closure-check.json)。

上一 HEAD269fe15e 的 PR CI run38425913437 为 11/11 SUCCESS；后续 HEAD 的
CI 必须单独核对。这是 PR CI，不是 main push CI。本轮真实 DELETE=0，
完整 D1 仍未接受，原 108 回执 manifest 和原失败/封存环境不变。

| 层次 | 结果及范围 |
| --- | --- |
| Domain/App | PASS；八类强类型引用，owner capability/source/workspace 校验，显式 quiescence/loss，fingerprint/replay，append-only Event/Audit/Evidence/Outbox |
| 独立 named-volume PostgreSQL | PASS；引用与检查、pending copy 与 dispatch 两个顺序的 barrier；活动/未知 pin、tenant/owner/replay/immutability；冻结源码完整 lifecycle race 套件 190.833s |
| 全模块 Go / vet | PASS；go test -race ./... 的无外部服务模式；未配置的集成用例属于 SKIP，不计真实 PG/provider |
| NATIVE Processing | PASS 非删除 admission；真实 Entity STANDARDIZED、NATIVE CURATED baseline；在实际 input GET 后暂停；活动 pin 阻断，批准/restriction 后输出不被接纳 |
| Queue / recovery | 实际 Asynq task Handler 与 Application Create/Start/Retry/NativeReconciler；queued/start/replay/retry 的数据面 admission 被拒，自然等待 10 分钟 RUNNING lease 后 recovery 为 0 新 GET/output。Start 仍可能先提交 RUNNING，再由 PinnedEngine 拒绝，不把这项结果表述为所有 Start 状态迁移都被拒 |
| DirectData | PASS 非删除；真实 Quality/Certification/current rights，ISSUED commit 后真实慢 TCP 传输；transfer pin 阻断直到 writer 明确结束；same-key non-payload，新请求在 admission 拒绝 |
| actual provider DELETE / exact absence / completion | NOT_RUN；无 DELETE adapter/credential/send claim，completion=0 |
| 原生进程崩溃、真实 Redis worker 投递、native ISSUED commit 前暂停 | 本轮 NOT_RUN；已有 SQL/fake owner/commit barrier 单列，不替代上述 native fault 验收 |
| 全 D1、native archive/restore rerun、D2、人工解释 | 未接受 / NOT_RUN / 暂缓 |

## 新环境、阶段及计数

此前阶段的成功新环境 `dpp-lifecycle307-e1487ce7`，run
`e1487ce7-172d-4641-80a3-725aa12cc483`。内部网络，无 host port，
Core/object 均为专属 named volume；原环境未重开。
[环境](../../examples/lifecycle-d1/results/e1487ce7-172d-4641-80a3-725aa12cc483/reader-environment.json)；
[source/binary binding](../../examples/lifecycle-d1/results/e1487ce7-172d-4641-80a3-725aa12cc483/reference-source-binding.json)。
Linux binary SHA256 `bdd90a3c02fe645e1d722ecbef036b537e77a7f2ff720dd8fa4130c3a4617875`。
Go build settings 没有 vcs.revision；Git tree/blob 与 checkout hash 分别绑定，不作虚假的 VCS 内嵌证明。

[竞争回执](../../examples/lifecycle-d1/results/e1487ce7-172d-4641-80a3-725aa12cc483/reference-business-races.json)：
8 类 / 23 条 exact-copy 需求、23 条显式释放；7 pins / 7 verified stops。
两次活动 dispatch 拒绝同时记录 REFERENCES_RETAINED_OR_UNKNOWN 与 READERS_ACTIVE_OR_UNKNOWN；
两个停止后的 dispatch admission 通过。Admission 不表示 provider 调用。

竞争阶段 32 storage calls / observations / typed costs；4 READ_PINNED、2 PROCESSING_PUT。
处置先提交后的 queued/replay/retry/recovery/新 DirectData 为 0 新 GET/output。
直接传输 129328 bytes；原 DeliveryOperation 保持 ISSUED，transfer observation 追加。
之后 final 的独立只读 exact inventory/protection 再产生 20 调用，总数
52 calls / 52 observations / 52 typed storage costs；7 stops、active=0、send claims=0、completion=0。
[最终独立 SQL 计数](../../examples/lifecycle-d1/results/e1487ce7-172d-4641-80a3-725aa12cc483/independent-final-counts.json)。
IAM/bootstrap journal 与这些业务数据面计数分开。

[封存](../../examples/lifecycle-d1/results/e1487ce7-172d-4641-80a3-725aa12cc483/plane-seal.json)：
reader/producer DISABLED，window disabled，gateway DENY_ALL。
六个 exact versions、两个保护、Core、容器、卷全部保留；cleanup=0。
历史 source FK、lineage、Quality、Certification、ISSUED 未被删除或降级；
23 条需求仅释放所列 physical copy，其他 copy 的引用仍阻断。
丢失能力通过 typed loss 逐项记录：source/Execution replay、mapping input 重验、
Entity replay、Quality 复算/attempt replay、Certification 内容重验、Delivery payload replay。
当前 source rights 继续依赖 immutable lineage facts。

## 必须保留的失败

- `46606e34` / source69ad696f：driver 漏 beta bounded-use policy；baseline FAILED，
  0 READ_PINNED / 0 pin；已封存，未重跑覆盖失败。
- `9c612a78` / source6e6608af：baseline SUCCEEDED，但 driver 提前批准处置，
  current-use gate 拒绝活动加工；等待 helper 被停止，active=0，已封存。
- 一轮长跑 PG 测试的旧 binary6e6608af 读取到运行期间新增 quiescence 列的 SQL，
  得到23502；属于 mixed revision FAIL，不计 candidate PASS。最终从 Git archive
  a7116c35 冻结源码重跑，完整 PG race PASS；原结果不改写。

## 未获批准的实际删除提案

[精确提案](../../examples/lifecycle-d1/results/e1487ce7-172d-4641-80a3-725aa12cc483/provider-delete-proposal.json)
仅为 PENDING_USER_CONFIRMATION。Plan
`6cb1b6b7-d855-4da1-8909-cc9184855036`，hash
`10d024ed7554739ed52409bd939259181738af0550f83129abfee8d724f7bcb9`。

| namespace / key | exact provider version |
| --- | --- |
| dpp-lifecycle307-online / fixture/a.csv | ac15b44b-9944-4326-9e19-aecf7c62b57f |
| dpp-lifecycle307-quarantine / fixture/b.csv | 92af5ef2-e26f-46bf-8189-acbf67e2e5bc |

排除原环境、#298/#294/Pilot/demo、两份受保护版本、两份新输出、
所有 DB/history/容器/named volumes、provider 内部/备份/外部 copies。
cleanup 清单为空，0 hold release / retention bypass。Business window 已封存。
任何真实调用必须另获当前 exact-list 人工确认及 fresh authority/approval、
inventory/protection/reference/pin 检查；旧9812f0fc批准已消费，不能沿用。
SQL approval 的 CONTROLLED_NON_DELETE_ADMISSION_EXERCISE 不授权真实 DELETE。

旧 C2 Core 保持 LOST/UNAVAILABLE/NOT_RESTORED，本轮新库 PASS 不表示恢复。
