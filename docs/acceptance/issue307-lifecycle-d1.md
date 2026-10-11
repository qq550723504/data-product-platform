# #307 D1 开发与验收记录

> 2026-10-11；D1 OPEN。已完成五个本地增量：plan foundation、restriction/hold/reader occupancy、DB-only approval/dispatch/physical observations、非删除 storage inspection / 逐 HTTP 调用 Journal、独立封闭 native fixture 与实际版本清单；完整 D1 runtime **NOT_RUN**，删除确认 **PENDING**。
> 用户已授权创建 Issue 并开发，仅受控合成对象；实际 provider 删除（含测试/cleanup）仍须先确认环境和 exact object/version 清单。

## 权威与写入归属

[Issue #307](https://github.com/qq550723504/data-product-platform/issues/307) 属于 [Epic #296](https://github.com/qq550723504/data-product-platform/issues/296)，设计前置 [#300](https://github.com/qq550723504/data-product-platform/issues/300) 已关闭。设计 PR #306 merge 为 `9502bd1caea61dd86e80827142bd1d18269a2d79`，matching main push CI [38125986415](https://github.com/qq550723504/data-product-platform/actions/runs/38125986415) SUCCESS；docs-only required 通过、runtime jobs 跳过。

业务规则仍以 [Lifecycle Contract](../architecture/data-lifecycle.md)、[ADR-0015](../adr/0015-data-lifecycle-disposition-boundary.md) 和 [设计验收 §4–6](../product/data-lifecycle-design-acceptance.md) 为准。本文只记录当前增量与证据，不另建生命周期规则。

当前聊天为 #307 唯一 writer。branch `codex/issue307-lifecycle-d1`，worktree `C:\Users\Henry\.codex\worktrees\issue307-lifecycle-d1\data-product-platform`，起始 base `9502bd1`；第三增量结束时 main 已合入 #308，当前独立分支更新至 `origin/main@3a59b591d25e7d07fc38956a7ae729d74967215b`。主工作区保持原 HEAD，不写 #298/#294 的 worktrees、代码或运行环境。首轮 App inventory transport 不可用；后续已恢复并重新核查 active threads/artifacts、Git worktrees、GitHub PR 与 remote main。#307 为 native Git worktree，App 确认它不属于 managed worktree；不修改 App metadata。

Main 已含 migrations 000051–000053，D1 使用 **000054**；该 migration 尚未发布，在当前严格 schema 内增加 facts，不添加双读/兼容 fallback。Draft #295 仍携旧 000050，原 owner 后续须在发布前解决其排序；本片不恢复该任务。共享改动限新增事件 routing/vocabulary、migration runner 重复版本拒绝、现有 CostEvent/CostAllocation 的 typed lifecycle attempt allocation；未引入新依赖、队列或存储 engine。

第四增量期间核验 #299/C1 线程中用户的“先协调 #307 的迁移与共享写域”授权后，已与其原 writer 协调：000054 保留给 #307，000055 由 #299 预留；共享 routing、single-subject / workspace cost guards 按先进入 main 的候选串行 rebase/集成，保留双方强类型 subject 分支。#307 当前 routing 为 c1-v23，不改 C1 模块/页面或既有 API/worker 注册。

## 首个增量的有限 contract

业务结果：可信 server-side authority 可以对**已登记合成 copy IDs**原子创建并冻结一个 exact-version 删除计划，得到可比较 scope hash；同键重放复核当前 authority，scope/actor 变化拒绝。准备计划不等于批准、dispatch 或删除。

- Typed registry 固定 workspace、DatasetVersion、provider instance/incarnation、namespace、key、provider version、content hash/size；Command 不接受 URI、key、endpoint 或任意 target metadata。
- `DRAFT → FINALIZED` 计划与 exact membership 在同一 parent row lock 串行化；数据库重算 `SHA256-LIFECYCLE-PLAN-V1`，冻结后拒绝 header/membership/identity mutation，不能 truncate 历史。UTF-8 byte framing 与 Go 相同。
- Application 复用 deliveryfence、transaction、command_idempotency；Domain Event、Audit、最小 Evidence、retention-only Outbox 与计划同事务。没有 provider invocation，不编造 provider CostEvent。
- 纯 Domain 删除 prerequisite 门禁显式要求当前 disposal authority/approval、retention start/boundary、restrict/drain/shared-reference/hold、inventory、provider retention 与 ledger。零值、UNKNOWN、任意 ANY 不能放行；结果只用于预检，不能授权副作用。
- Migration runner 原来同版本文件会按 applied-version 静默跳过另一份，现改为 up/down 都明确拒绝 duplicate version；允许正常 up/down pair 和已存在的版本 gap。

本增量尚未挂到 API/worker，不配置真实 provider，不登记任何 live object。fixture registry 行只是 SQL synthetic identity，不是 provider verification receipt。当前 authority integration tests 使用受控 test authority，不能代替 live principal/delegation acceptance。

Merge blockers：本增量的 tenant/authority bypass、冻结/幂等/原子历史错误、scope hash 不一致、reader 未提交 pin 即准许读取、未经匹配的停止证明释放 pin、fence/锁序错误、migration 与测试失败。D1 余项仍是本 Issue 待开发内容，不将它们伪装成此增量已完成，也不关闭 #307。

## 第二个增量的有限 contract

业务结果：对已登记合成内容追加立即生效的 use restriction、独立 hold/release 和 reader pin/termination。Command 与 frozen plan occupancy query 复用同一 workspace delivery fence；固定 Dataset → DatasetVersion → Copy → Plan/Pin/Hold 锁序。current state 从完整 typed facts 解析，不用 mutable refcount 或 soft delete。

- `RestrictContentUse` 按 DatasetVersion 阻止其所有合成 copies 的新 reader；时钟回退也不会使立即 restriction 失效。它不等于处置批准，当前增量不提供恢复使用 Command。
- 多条 hold 取联合；释放必须指向明确 hold，并追加独立 release。保全本身不授予使用权，也不阻止另经 current gate 准许的读；解除 hold 不解除其他 hold 或 use restriction。
- Reader admission 在共享 fence 下复核可信 caller、当前 gate、精确 workspace/copy/reader/actor 和明确 use window，提交 durable pin 后才返回 `Admitted=true`；本方法不读取 bytes。UNKNOWN、过期、缺失窗口、错配 scope 均拒绝。
- 同键 admission 重放只返回历史 pin，`Admitted=false`，不会重新开启读取；重放也复核当前 caller authority。重新读取须新 key 和 fresh gate。
- Pin 无 expiry/heartbeat 自动释放。服务端停止证明须匹配 pin、reader、generation、workspace、结果和观察时间；仅 `COMPLETED` / `CANCELLATION_CONFIRMED` 可追加 termination。TIMEOUT、陈旧 generation 和未确认停止保留 pin。
- 每次成功变更推进 revision，并在同一事务写 Domain Event、Audit、最小 Evidence、retention-only Outbox 和幂等事实。新增五类事件使用 routing `c1-v21`；未发生 provider invocation，不编造 CostEvent。
- `InspectPlanOccupancy` 只报告 frozen plan 的 restriction/hold/reader 占用，不能批准 dispatch。实际 material/shared references、inventory、policy、当前 disposal approval 和 provider constraints 仍须在后续 dispatch 逐项 fresh 验证；不能把当前查询全部绿色当成允许删除。

Reuse Check：复用现有 transaction/deliveryfence、command_idempotency、Audit/Evidence/Outbox 与 lifecycle registry；没有新增 scheduler、queue、storage runtime 或 IAM 实现。Authority / current use gate / termination verifier 为 Application Ports；本轮使用受控 test adapters。真实 principal/current-rights、reader owner/controller、HTTP/worker/data plane 尚未接入，不能将 test verifier 视为真实 quiescence 证明。

## 第三个增量的有限 contract

业务结果：可信 server-side authority 可对 frozen plan 追加有限 scope approval/revocation；fresh fenced dispatch 建立 exclusive copy reservation 与 physical start；服务端 verifier 的 observation 单独追加。当前实现不调用 provider、不读取 bytes，也不启用 worker/HTTP dispatch。

- Approval 绑定 workspace、plan hash、action、basis、批准人及有限有效期；dispatch 重新校验 caller 与批准人的当前权限，以及撤销/到期事实。资产 owner 或 use right 不隐含 approval role。
- Domain deletion prerequisites 在 dispatch 下使用当前 registered copy occupancy；保全、reader、未核实 dispatch、UNKNOWN inventory/reference/policy/provider/ledger 或缺失显式依赖窗口均阻断。受控 dependency Port 仅供本轮测试，真实 policy binding/inventory/material references/native provider state 尚未接入，不能据此启用 live deletion。
- Gate evaluation 保存强类型 retention/input/occupancy facts；输入 INSERT 与 `DRAFT → FINALIZED` 获取同一 parent lock，先持锁读取完整成员，再校验 count/hash 冻结。FINALIZED 后拒绝 late INSERT 与历史 mutation，避免只靠终态 trigger 留下竞态。
- Reader 与 dispatch 在当前查询返回后复核 caller/声明窗口与 expiry，查询过程中到期不接纳。当前依赖 Port 必须覆盖其所有时间边界并显式给出 unbounded/deadline；zero/UNKNOWN 不冒充永久有效。
- 仅新提交 dispatch 返回 `Admitted=true`；同键重放只返回历史 IDs、`Admitted=false`。每个 copy 同时只允许一个未核实 reservation；start-only、UNKNOWN、provider ACCEPTED 或 FAILED 均不能盲目再删，也不能当作 exact absence receipt。
- Start 与 observation immutable；UNCERTAIN 无确定 invocation quantity。CONFIRMED（含 failed/timeout）使用 physical attempt ID 追加 quantity=1、amount=NULL 的 CostEvent 与 typed CostAllocation；同 attempt 后续 resolution 去重，独立 attempt 独立计账。测试中的 CONFIRMED 来自合成 verifier，不是实际计费证据。
- NOT_SENT 只接受匹配的服务端权威证明；与已有 CONFIRMED 互斥。释放 reservation 另追加 fact + Event/Audit/Evidence/Outbox，随后新显式 key 仍须 fresh gate；原 observation 不覆盖。
- hold-first 阻断未 dispatch copy；dispatch-first 的 late hold 保存 typed conflict，并返回 `PreservationUnconfirmed=true`。既有调用可能已发出，不能宣称保全成功；后续 containment/reconciliation 尚待真实 worker/provider 接入。
- Approval、blocked/allowed evaluation、start、observation、成本和 reservation release 保持相应同事务/idempotency 边界；新增事件 retention-only，routing `c1-v22`。没有增加 scheduler、第三方审批系统、第二账本或 provider SDK。

Merge blockers 仍包括当前增量的越权/跨 tenant、scope/expiry bypass、丢失成本、未知重发、frozen input 竞态、晚到 hold 无冲突证据、事务部分提交与必需测试失败。Provider absence/finalize、lookup/reconciliation 的独立 physical attempts、fixture HTTP/worker 与完整 D1 仍属 #307 待开发项。

## 第四个增量的有限 contract

业务结果：增加 SDK 无关的 `StorageInspector` / `StorageCallJournal` Ports，并复用 MinIO v7.0.83 实现 exact-version HEAD、retention/hold 与受控 namespace 的完整单页版本盘点。每个实际 HTTP 请求由真实 PostgreSQL Journal 在 I/O 前提交 immutable start，返回/丢响应后追加 observation；CONFIRMED invocation 复用既有 CostEvent/typed CostAllocation，失败和 UNKNOWN 也不吞成本。当前没有 native storage 配置、API/worker/data-plane wiring。

- Server-owned config 固定 workspace、provider instance/incarnation、endpoint 和恰好两个 namespace。每次调用同时匹配 registry 的完整 copy identity；URI/endpoint/namespace 不从 client command 接收。Copy 必须为 non-null exact version；HEAD response 必须 echo 同一 version、非 delete marker 且长度一致。返回的 SHA256 仍是 registry 历史 identity，HEAD **不验证实际 bytes/hash**。
- Inspector 只允许三个 metadata GET 形状及 exact HEAD；普通 object GET、PUT/POST/DELETE、额外 query、错 endpoint、bare-key 均在 transport 前拒绝。原 Put/Get Store 的权限未扩大；本增量没有 DeleteExactCopy、Archive/Restore、absence finalize 或直接提供 payload 的方法。
- 固定 Region/path-style，SDK `MaxRetries=1` 明确禁用默认重试；关闭 net/http keep-alive 重用重试及 HTTP/2，禁止隐式 proxy 和 redirect。一个 operation context 只准一个实际 HTTP 请求；retention + hold 的两个请求有各自 physical ID，不合并成本。
- 每个 namespace 只支持完整、成功、最多 2 MiB 的单页 native version inventory。403/404、错 Name/Prefix、缺失/重复 envelope、unknown root member、missing/null version、delete marker 混淆、截断页或后续分页均不能冒充完整扫描。保留每个版本/delete marker 和 observed time/response SHA；它不是“已销毁”结论，不覆盖内部介质、备份、replication 或未登记系统。
- 不支持/错配/损坏 provider result 使用 typed error；不把异常字符串、credential、headers 或 payload 写入 Journal。Metadata 响应有大小/deadline 上限。取消后使用独立短 proof context 保存已经发生的调用；Journal start/observation 失败均返回 UNKNOWN，start 保留，不自动重发。
- Journal Start 在既有 fence 与 Dataset → Version → Copy 锁序下复核当前 inspection authority 与完整 registry identity，然后提交；外部 I/O 不持这些锁。同 physical ID 的 Start 不能二次 admission。Observation 来源只限 server-owned instrumented Adapter，未提供 client receipt API；原 caller 随后撤权不删除已经发生的 cost，但新 Start 会拒绝。
- `lifecycle_storage_call` / `lifecycle_storage_observation` append-only、强类型 workspace/copy/call scope；observed/invoked 时间不得早于 start，同 physical call 的后续观察不得藏入第二次 invocation。confirmed observation、quantity=1 / amount=NULL CostEvent、`lifecycle_storage_call_id` allocation、Event/Audit/Evidence/retention-only Outbox 同事务；uncertain start 没有确定 quantity。同 receipt replay 去重，独立新 call 独立计账。routing 使用 c1-v23。

Reuse Check：现有 MinIO client、transaction/deliveryfence、registry、LifecycleAuthority、Evidence/Audit/Outbox 与 CostEvent 足以承接；新增的只是 exact-copy scope 和逐物理调用事实，没有第二账本、generic storage runtime、SDK、scheduler 或 IAM。真实 current authority/provider credential scope、固定 incarnation/container 验证、business gate freshness、native API contract 和真正业务 reader 仍需 fixture 入口验证，本增量未把 metadata inspection 作为 CurrentDisposalDependencies ALLOWED。

非删除 artifact preflight：官方 [repo metadata](https://api.github.com/repos/minio/minio) / [latest release](https://api.github.com/repos/minio/minio/releases/latest) 再核对仍 archived=true / tag `RELEASE.2025-10-15T17-29-55Z`。历史 #298 report 的 image `086b0874…` 本机已不存在；当前本机 image `sha256:d11a0cb81cffefa89aa17635273bcd3d224c30185d956486182ab2aa28b6aee0` 内的 `/usr/local/bin/minio` SHA256 为 `fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8`，与原 report 一致。独立 `--network none` / `--version` 显示 commit `9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a`、Go 1.26.5、AGPLv3，但运行时 version label 为 **DEVELOPMENT.GOGET**；不把 tag 名称当 runtime label 或继承旧 image admission。只做 binary/version 读取，未启动 storage server、未挂 data、未接触 #298 retained 环境。后续独立 fixture 必须固定实际 image/binary/source、专属 network/volumes 与唯一 gateway，并保留 unsigned-trailer mitigation；production admission 不在本片。

Merge blockers：未提交 start 就发送请求、SDK/transport 隐式重发、错 exact scope/损坏 inventory 被接纳、跨 workspace 或错 copy 计账、撤权使已发调用消失、mutable call history、同调用吞掉重试成本、proof/cost 部分提交和必需测试失败。本轮 Ports/Journal 的有限完成不关闭 D1，也不替代 provider-native/封闭 fixture/worker/LC 验收。

## 已执行检查

仅使用独立 PostgreSQL：container `dpp-lifecycle307-pg-20261011`，image ID `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`，data 使用 tmpfs、无 volume mounts。各轮 loopback 分别为 `127.0.0.1:54802`、`:58798`、第三轮 `:64285`、第四轮 `:63760`。Lifecycle integration tests 各自创建/移除唯一 scratch DB；migration suite 的旧 T3/B2 case 使用本容器已初始化的基础库。测试完成后停止本容器；不挂对象存储，不读/清既有 retained volumes。

| 检查 | 状态 / 证明范围 |
|---|---|
| Domain：canonical/exact hash，bare-key/unknown/foreign/duplicate 拒绝；current prerequisite/approval/retention 负例 | PASS；pure Go，无 provider |
| PG command：UTF-8 Go/DB hash 一致，fresh-authority replay、scope 冲突、同键并发只一套事实 | PASS；真实 PostgreSQL，synthetic SQL facts |
| PG atomic proof：缺 routing 导致事务整体 rollback，未落半套 plan/Event/Audit/Evidence | PASS |
| PG freeze：member-first / finalize-first 双顺序、观察真实 lock wait、late writer 拒绝/完整成员纳入 | PASS；不是靠 sleep 推断竞态 |
| PG immutable/hash/down：forged/empty finalize 拒绝、identity/frozen mutation 拒绝、destructive down 保留历史 | PASS |
| Domain reader：fresh exact context/use window 与 generation-bound termination 负例 | PASS；pure Go |
| PG occupancy：双 hold 独立解除，restriction 覆盖两个 copies，same-key non-admitting replay、fresh authority、改 key 内容拒绝 | PASS；真实 PostgreSQL，受控 authority/gate/verifier |
| PG pin：deadline/heartbeat 不释放，TIMEOUT/错 generation 拒绝；精确确认停止才追加 termination | PASS；Application contract；真实 reader quiescence NOT_RUN |
| PG races：reader-first / restriction-first；hold-first / occupancy inspection；观察真实 fence lock wait | PASS；fixture contracts，不作为完整 LC-06/11 验收 |
| PG occupancy history：跨 workspace 拒绝、错误 termination binding/time 拒绝、所有五类 facts 不可 UPDATE/DELETE/TRUNCATE、缺 routing 原子 rollback | PASS |
| PG disposition：fresh approver/revocation/expiry、UNKNOWN/wrong-scope、active reader、non-admitting replay、NOT_SENT 与新显式 attempt | PASS；受控 adapters，没有实际 provider |
| PG dispatch/hold 与 gate freeze 双顺序：真实 fence/parent lock wait、late input/forged hash 拒绝、完整 hash 保留、late hold conflict 原子证明 | PASS；DB admission，完整 native LC-06/07/11 NOT_RUN |
| PG observation/cost：start 与 invocation 分开、unknown 保留、failed/timeout 按 physical ID 计账、resolution 去重、routing failure 整体 rollback | PASS；合成观察事实，没有真实 provider invocation/费用 |
| PG elapsed window：reader/query 与 dispatch/query 期间到期，0 reader pin / physical attempt | PASS |
| SDK inspection contract：exact version/长度/delete marker、foreign scope、403/404/503/redirect、丢响应、无 journal/no-op admission、metadata request allowlist、retention/hold 与完整单页 inventory 负例 | PASS；httptest 合成 HTTP，MinIO v7.0.83；没有 native provider/data read/delete |
| PG HTTP-call Journal：start 提交、同 physical ID 不二次 admission、fresh authority、scope identity、unknown 与 confirmed 分开、failed / revoked-caller 成本、same receipt/resolution 去重、新 call 独立成本、immutable scope/time/quantity guards | PASS；真实 PG，合成 transport observations |
| PG Journal proof rollback：start routing failure 0 start；observation routing failure 0 observation/cost、保留原 uncertain start | PASS |
| `go test -race -count=1 ./internal/lifecycle/... ./internal/platform/routing ./internal/cost`（独立 PG） | PASS；16 个 lifecycle PG top-level tests，含双顺序 subcases |
| 第四增量 `go test -race -count=1 ./internal/lifecycle/... ./internal/platform/storage ./internal/platform/routing ./internal/cost`（独立 PG） | PASS；新增两个 Journal PG tests，共 18 个 lifecycle PG top-level tests；首次新测试误查不存在的 domain_event 表，改为既有 outbox_event 后完整范围重跑通过 |
| migration version collision / normal pair | PASS；up/down duplicate 明确拒绝 |
| `go test -count=1 ./internal/platform/migration`（独立已初始化 PG） | PASS；首次在 restart 后空基础库运行的旧 T3/B2 schema precheck 失败，初始化本测试库后补跑通过 |
| `go test ./...` / `go vet ./...` / formatting 与 diff 检查 | PASS；module tests 未配置 live provider/PG，该轮外部 integration SKIP；上述 PG tests 另有独立实跑证据 |
| CI / independent reviewer / matching main CI for #307 code | NOT_RUN；本地增量未发布 PR |
| provider-native hold/retention/delete、absence receipts、archive/restore、worker/HTTP/data plane、完整 LC cases | NOT_RUN |

执行命令（Go module `apps/platform`）：`go test -count=1 ./internal/lifecycle/... ./internal/platform/routing` 及带 `-race` 的同范围检查（独立 `TEST_POSTGRES_DSN`）；migration suite 前用本独立库的 `POSTGRES_DSN` 执行 `go run ./cmd/migrate -direction up -dir ../../migrations`，再运行 migration suite。最终检查与 commit 以 Issue comment 记录为准。

## 后续开发和删除确认

下一步仍在 #307 内：用户确认已建立的环境与 actual exact-version 清单；接入真实 fixture current gate/reader owner 与 retention policy/material reference facts；补 MinIO native delete/archive/restore Adapter 与 existing worker reconciliation（每个真实 lookup/verify 也独立 attempt/cost）；scoped absence receipt/finalize、restore ledger、封闭 fixture HTTP/worker 与完整 D1 LC 验收。非删除 native inspection 已实跑，不能据此放行 deletion 或 business read。既有 production processing/delivery 的全面纳管按 D2 与原 owner 协调。

实际 provider destructive test 前的独立环境与**已实际登记** manifest 现已生成：[待确认清单](../../examples/lifecycle-d1/results/a6b30840-3e76-49ad-91d3-f0c399fcf170/review.md)。包含 project/container/network、实际 build/image/SDK、workspace、两个 namespace、四个 key + native version/hash/bytes、计划 scope hash、retention/hold、动作与 excluded scope。批准状态仍 **PENDING**；任何 DELETE（含预期拒绝负例）与 object/bucket/volume cleanup 均未执行。

外部备份/recipient、敏感内嵌 Evidence/Audit、Annotation sealed payload、#298 demo/retained objects 均不在 D1 destructive allowlist。D2–D6 按原设计后续顺序，不增加第二 scheduler 或自行恢复暂停联调。

## 第五增量：原生 fixture，停在删除确认前

Run `a6b30840-3e76-49ad-91d3-f0c399fcf170` 只使用 27/26-byte synthetic CSV。新 internal network 的三个成员为本 run 的 PostgreSQL/MinIO/gateway；两个专属 named volumes 保留。Docker Desktop internal network 的最初 loopback publish 请求没有实际 mapping；准备 helper 因而在 gateway network namespace 内执行，未开放 provider/DB/console/业务 API。详见 [入口与隔离复用](../../examples/lifecycle-d1/README.md)、[manifest](../../examples/lifecycle-d1/results/a6b30840-3e76-49ad-91d3-f0c399fcf170/manifest.json) 与 [环境独立读回](../../examples/lifecycle-d1/results/a6b30840-3e76-49ad-91d3-f0c399fcf170/environment-readback.json)。

四个 provider versions 实际创建，按 exact version 重读 bytes/hash 后注册 Core；两个 DatasetVersions 都通过现有 command 限制使用。四份 object 是 bootstrap inputs，不能冒充业务 archive/restore；READY 不代表 rights/certification/delivery。Online B legal hold=ON；quarantine A GOVERNANCE 至 2026-10-12T10:41:09Z；没有解除/bypass。检查身份复用 native IAM policy，无 Put/Delete/Bypass；gateway 阻断检查身份 payload GET。成功后禁用 fixture principal，启动 guard 拒绝 finalized manifest replay。

接入修正保留真实失败证据：脚本 command-name 冲突和 image ID 变化发生在 provider 创建前；bucket URL trailing slash 被 gateway 拒绝时，显式复核空 registry/provider 后才修正创建；四个版本创建后，缺 Outbox routing 的 restriction 原子回滚，补用既有 routing 后只读注册版本继续；未设置 retention/hold 的 native 400/NoSuchObjectLockConfiguration 与空 200 contract 不同，按固定官方 source 做 exact error/resource/cardinality 校验，未知/拒绝/缺失仍 fail closed。没有覆盖或重建已有对象/registry/history。

[真实 PostgreSQL 读回](../../examples/lifecycle-d1/results/a6b30840-3e76-49ad-91d3-f0c399fcf170/database-readback.json)：4 copies、2 restrictions、19 storage starts / observations / invocation cost allocations（此前5次检查 + 完整14次检查，均保留）、41 Event/Audit/Evidence，scope hash 与 DB 重算一致；approval/dispatch/delete-attempt/reader-pin 全部0。物理调用成本 quantity=1，amount=NULL，使用既有账本；fixtures infrastructure provisioning 的 JSONL 只记录输入准备，不充当生产活动成本账本。

本轮 `go test -race -count=1 ./cmd/lifecycle-fixture-prepare ./internal/platform/storage ./internal/lifecycle/... ./internal/platform/routing ./internal/cost` PASS，含当前 authority 禁用、无删除/reader capability、foreign/destructive shape、隐式重发拒绝、finalized output 恢复拒绝与 native absence-of-protection 正负例。此次该命令未配置 TEST_POSTGRES_DSN，PG unit suites SKIP；实际 fixture Journal/plan/restriction 另由上述 native + PG 实跑证明，之前18项 PG concurrency/constraint tests 记录仍保留。完整 LC / delete / absence finalize / archive/restore / business reader / PR CI / main CI 仍 NOT_RUN。

本次 fixture 固定 main `3a59b591...` + 当时未发布 D1 migration 000054；private SQL snapshot、binary 与 manifest hashes保留。期间 main 合入 #309/000055，#299/C1 预留000056；D1 开发 migration 需重排000057并重新与 main 集成，不能把重排后的 migration 安装到本次已保留的 fixture 历史库，也不能以兼容/重跑为名擦除它。
