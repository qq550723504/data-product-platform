# #307 引用需求与业务竞争：有限验收

2026-10-13。实现 contract 见 [finite contract](issue307-reference-demand-contract.md)；
base main `074a33bace7edbaf481759aac3a84708363ffd02`，migration67。
受验源码 `a7116c355428ee71ff621870ceafeb6ede9e9205`；
最终文档/回执 HEAD 不冒充重新执行过的 binary。
本轮 Draft，不 Ready/merge/deploy/close #307；完整 D1 未接受。

| 层次 | 结果及范围 |
| --- | --- |
| Domain/App | PASS；八类强类型引用，owner capability/source/workspace 校验，显式 quiescence/loss，fingerprint/replay，append-only Event/Audit/Evidence/Outbox |
| 独立 named-volume PostgreSQL | PASS；引用与检查、pending copy 与 dispatch 两个顺序的 barrier；活动/未知 pin、tenant/owner/replay/immutability；冻结源码完整 lifecycle race 套件 190.833s |
| 全模块 Go / vet | PASS；go test -race ./... 的无外部服务模式；未配置的集成用例属于 SKIP，不计真实 PG/provider |
| NATIVE Processing | PASS 非删除 admission；真实 Entity STANDARDIZED、NATIVE CURATED baseline；在实际 input GET 后暂停；活动 pin 阻断，批准/restriction 后输出不被接纳 |
| Queue / recovery | 实际 Asynq task Handler 与 Application Create/Start/Retry/NativeReconciler；queued/start/replay/retry 被拒，自然等待 10 分钟 RUNNING lease 后 recovery 为 0 新 GET/output |
| DirectData | PASS 非删除；真实 Quality/Certification/current rights，ISSUED commit 后真实慢 TCP 传输；transfer pin 阻断直到 writer 明确结束；same-key non-payload，新请求在 admission 拒绝 |
| actual provider DELETE / exact absence / completion | NOT_RUN；无 DELETE adapter/credential/send claim，completion=0 |
| 原生进程崩溃、真实 Redis worker 投递、native ISSUED commit 前暂停 | 本轮 NOT_RUN；已有 SQL/fake owner/commit barrier 单列，不替代上述 native fault 验收 |
| 全 D1、native archive/restore rerun、D2、人工解释 | 未接受 / NOT_RUN / 暂缓 |

## 新环境、阶段及计数

唯一成功新环境 `dpp-lifecycle307-e1487ce7`，run
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
