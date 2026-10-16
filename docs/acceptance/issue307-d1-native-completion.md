# #307 D1：pre-ISSUED、worker 故障与 archive/restore 原生补验

当前PR收口见[#318最终评审及剩余边界](issue318-final-closure.md)。本文记录2026-10-14本轮非删除补验；后续LC-13和单独v15删除/恢复已完成，旧“待确认/未运行”保留原时点。完整D1仍未接受。

2026-10-14。#318 保持 Draft；merge、真实 DELETE、hold release、retention bypass、cleanup 均为0。
本轮按[有限 contract](issue307-d1-completion-contract.md)完成三项非删除补验。
完整 D1、D2 和生产 DR 未验收；#307 保持 OPEN。

源码入口 `9b42c195a686f29286da0dfa1d4c03c64f559fc8`，后续 driver 修订分别补显式重复入队、
保留 Docker outage 的新 generation 和最终全网关封存；Go helper 源码保持相同。
最终 driver `c18ffe64`。全部706个当前 Go/SQL/依赖/脚本文件的 working SHA256、Git blob SHA256、
CRLF差异，以及四类 Linux binary SHA256 见[源码绑定](evidence/issue307-d1/native-completion01/source-binding.json)。
二进制没有 VCS build metadata；各 helper 的实际 source-head/provenance 和源码 hash 为绑定依据，
不声称其 build metadata 带有最终提交号。

## 新环境与隔离

全部为本轮新建 schema68/base074a33ba 环境；内部网络、0宿主端口、专属 named volumes，
复用既有 PostgreSQL/MinIO/Caddy/Redis pins、Commands/fence、transfer SPI、call journal、
Cost/Audit/Evidence/Outbox、Asynq 与 FileRestoreLedger；没有新依赖、迁移、通用API或第二套 runtime。

| 角色 | Run / project suffix | subnet | Native versions / physical calls |
| --- | --- | --- | --- |
| pre-ISSUED | `4176888e-2c11-476e-80cf-b9800db1f8e9` | `10.234.154.0/24` | 5 / 33 |
| archive/restore business | `df27ccf8-00e5-46f0-8ac2-58568fe5b574` | `10.234.155.0/24` | 9 / 69 |
| COPY worker | `74b0dd66-0194-46ec-aa84-26af6662e8b3` | `10.234.156.0/24` | 7 / 75 |
| metadata worker | `30cc0c1e-9e57-4507-88f5-2bee66d89ec5` | `10.234.157.0/24` | 4 / 32 |

project 均为 `dpp-lifecycle307-<suffix>`。各 `*-fixture.json` 保存完整 workspace/provider/incarnation、
key/version/bytes/hash、容器/image IDs、网络和卷。所有原对象及新 COPY/输出版本保留。
IAM admin helper 单独挂载 root 和唯一可写 admin receipt；业务 producer/worker/reader 挂载不含 root。
COPY worker 只有 metadata 与有界 target GET，无 COPY/DELETE；metadata worker 无 payload GET。

Docker Desktop 在前两个环境已封存后停止，第一次 COPY preparation 在 image inspect 被拒绝，
未创建 network/volume/helper、未发送 COPY；`d26ace12` 私有失败目录和 attempt 保留。
显式新 `prepare-copy-02` 在运行时恢复后分配74b0dd66，未重试旧 COPY。
旧5d2274ee删除候选服务保持 stopped，未重开其业务/维护窗口。
本轮前两个新环境的 Core 后续仅为 `BEGIN READ ONLY` 账务核对启动；其 MinIO/gateway 保持 stopped。
见[运行时记录](evidence/issue307-d1/native-completion01/runtime-incident.json)与
[最终只读核对](evidence/issue307-d1/native-completion01/final-accounting.json)。

## 验收矩阵

| 用例 | 本轮结果与精确边界 | 证据 |
| --- | --- | --- |
| native pre-ISSUED pause | 真实认证 loopback HTTP→Certification/current rights→实际 transfer-pin admission，停在未提交事务。独立连接看不到 candidate operation/pin/outbox/cost；TCP response0 bytes，native GET0。提交后正例129328 bytes/1 GET。 | [closure](../../examples/lifecycle-d1/results/4176888e-2c11-476e-80cf-b9800db1f8e9/pre-issued-native-closure.json) |
| pre-ISSUED process loss | 实际 helper exit94，operation/pin/events/cost/idempotency 等12表计数完全不变，candidate不可见，GET/bytes0。 | [rollback](../../examples/lifecycle-d1/results/4176888e-2c11-476e-80cf-b9800db1f8e9/pre-issued-crash-closure.json) |
| restriction 双方顺序 / replay | ISSUED-first 时观察到实际 PostgreSQL fence wait；restriction 提交后 fresh read403/0 GET。restriction-first403/0 GET，同 key409/non-payload/0 GET；两份原 ISSUED历史保留，2 pins/2 verified stops。 | 同一 pre-ISSUED closure |
| archive/restore business rerun | 实际 ARCHIVE1/RESTORE4，原 source保留；新 target exact bytes/hash/protection一致。current独立ledger→ONLINE+fresh pin read；old cut UNKNOWN→fresh current restriction BLOCKED；missing/gap均UNKNOWN。5 copy operations/5 verification/6 append-only decisions，1 pin/1 verified stop。 | [acceptance](../../examples/lifecycle-d1/results/df27ccf8-00e5-46f0-8ac2-58568fe5b574/archive-acceptance.json) |
| broker outage / duplicate enqueue | 实际同卷Redis停止→Outbox FAILED/0 confirmations；恢复同Redis→PUBLISHED/1 confirmation；现成入口双次EnqueueRecovery，0新Core work/0原生I/O。 | [faults](evidence/issue307-d1/native-completion01/copy-archive-faults.json) |
| COPY producer/worker faults | 原生COPY response被丢弃→producer exit93；run start后lookup前exit91；真实Asynq lookup后exit92；durable target GET后verify fact前exit96。原调用UNKNOWN事实保留，真实orphan任务恢复COPY_ARCHIVED，COPY仍只发送1次。 | [faults](evidence/issue307-d1/native-completion01/copy-archive-faults.json)、[resume](evidence/issue307-d1/native-completion01/copy-archive-resume.json) |
| COPY 双owner / missing→current ledger | 实际lookup后持有PG advisory lock；竞争owner返回OWNER_BUSY，0 run/0 I/O。原work缺ledger→UNKNOWN_COPY_LEDGER；新witness+显式新request→真实Outbox/Asynq→RESTORE_ONLINE；terminal replay0 I/O。 | [owner](evidence/issue307-d1/native-completion01/copy-restore-owner.json) |
| COPY lookup后撤权 / stale/current restriction | lookup已产生2个durable物理调用后关闭当前worker authority，后续I/O0，追加BLOCKED_AUTHORITY，原成本保留。后续显式新request：stale ledger→UNKNOWN_COPY_LEDGER；新restricted witness→BLOCKED_COPY；无COPY重发。该环境累计ARCHIVE1/RESTORE2。 | [revocation](evidence/issue307-d1/native-completion01/copy-revocation.json) |
| metadata真实worker crash/recovery | 单独SQL故障输入明确为NO_NATIVE_DELETE_APPROVAL，创建未发送attempt；lookup前exit91、实际Asynq lookup后exit92，真实orphan恢复仅得到UNKNOWN_PRESENT/non-terminal。 | [faults](evidence/issue307-d1/native-completion01/metadata-faults.json)、[resume](evidence/issue307-d1/native-completion01/metadata-resume.json) |
| metadata 双owner / lookup后撤权 | 竞争者OWNER_BUSY/0 I/O；lookup后关闭当前authority，下一HEAD0 I/O并追加BLOCKED_AUTHORITY；terminal replay0 I/O。reservation仍保留，absence/completion/release均0。 | [owner](evidence/issue307-d1/native-completion01/metadata-owner.json) |
| invocation accounting / seal | 四环境209 calls/209 observations/209 typed invocation costs，逐call allocation恰1；active pins0，provider DELETE invocations/send claims/absence/completion/dispatch releases均0。窗口全部false、四网关全请求DENY_ALL，资源保留。 | [只读SQL](evidence/issue307-d1/native-completion01/final-accounting-readonly.sql)、[readback](evidence/issue307-d1/native-completion01/final-accounting.json)、[seal](evidence/issue307-d1/native-completion01/sealed.json) |

这轮补齐 LC-03/20 当前原生业务重跑和 LC-12 pre-ISSUED 非删除边界。
真实 DELETE 竞争/response loss、in-flight DELETE late hold、三copy删除混合结果、
当前环境post-delete absence/completion仍NOT_RUN。
该轮未扩入 LC-13 live processing owner crash/recovery；后续已完成[独立原生补验及5d只读核验](issue307-lc13-readonly.md)，此前证据保持原边界。
metadata/COPY worker crash不能冒充processing或真实DELETE crash。
六版本备份/原生恢复和server no-send仍引用[前轮独立证明](issue307-d1-reference-preparation.md)，
不把本轮的5/9/7/4版本环境当该六版本checkpoint。

## 评审、回归与下一步

独立只读 reviewer发现并关闭三项根因：archive strict MinIO pin、admin receipt挂载、失败分支窗口/路由封存。
随后核对冻结源码和本轮保存证据；不声称reviewer独立运行了测试/SQL/provider。
全量真实PG `go test -race -count=1 -p 1 -json ./...`：89包/1117测试与子测试PASS，
0失败/0race；14个环境门控skip逐项保存在[回归回执](evidence/issue307-d1/native-completion01/verification.json)。
`go vet ./...`、22个Python正常/优化模式PASS。当前HEAD PR CI单独核对，CI不是main push验收。
最终索引与私有值扫描见同目录回执；所有失败诊断、helpers、对象、卷、witness保留。

真实删除仅待[5d2274ee精确方案](issue307-d1-delete-proposal.md)的单独确认；本轮新环境全不在删除清单。
旧5d服务当前stopped，执行前仍须新的只读恢复检查/current gate与短期authority。
本轮非删除PASS既不消耗也不产生人工删除批准。
