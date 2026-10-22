# #307 D1 剩余原生用例：有限设计、非删除准备与验收矩阵

新一轮准备范围：基于已通过评审的有限设计，使用三个全新 project 创建固定 16 版本/8 plan；新增工具与有限布局见 [原生准备边界](../../examples/lifecycle-d1/remaining-native.md)。本轮实际回执、绑定和更新矩阵将在 §8 追加；以下 §1–7 保留原设计/PG-only/独立评审时点。当前授权仅允许非删除准备；新方案未获删除授权。旧 v15 operator 不能执行新 case，不能把其既有 binary 当作七目标新 sender。

2026-10-16。唯一 writer 为当前 #307 聊天。本轮只设计、补组合式 SQL-only 回归、运行独立 scratch PostgreSQL 验证；**真实 provider DELETE=0，hold-release=0，bypass=0，不启用旧环境或旧授权，#307 保持 OPEN。**

当前更新：GitHub 同步已恢复，`bca29f4d63c423962ed5a43b71fd4f71ec2665f9` 已发布并完成独立只读评审，有限切片 **BLOCKER=0**；[新 PR #323](https://github.com/qq550723504/data-product-platform/pull/323) 保持 Draft。最新同步/复评矩阵见 §7；§6 的“未执行新独立评审”和 EOF 回执保留 bca29f4d 原时点。以下 PG/native 证明边界不变。

基线为已合并 #318 的 `b823eb33f8f3f923b619feda06e0ecc53514dd23`，对应 [push/main CI 38842607982](https://github.com/qq550723504/data-product-platform/actions/runs/38842607982) 13/13 SUCCESS。原 PR HEAD `22ee8e3e` 与 merge tree 相同。[旧收口报告](issue318-final-closure.md)及 v15 回执保留各自合并前时点，不能把其中 Draft 文字解释为当前 PR 状态。

## 1. 有限 contract 与权威归属

- Goal：准备 LC-11/12 的真实 processing/DirectData 双向竞争、LC-06/07 的 late hold、LC-14 三份同内容 copy 的 mixed outcomes；本轮不把它们改为 native PASS。
- 领域/并发/副作用权威仍是 [Lifecycle §6–7](../architecture/data-lifecycle.md) / ADR-0015；[产品验收 §4–5](../product/data-lifecycle-design-acceptance.md)拥有 LC 编号与完整 D1 范围。
- 本文件只拥有上述剩余用例的有限运行顺序、证据要求和当前矩阵；[reference contract](issue307-reference-demand-contract.md)、[LC-13 contract](issue307-lc13-resume-contract.md)及 [v15 实际执行](issue307-d1-confirmed-v15-execution.md)继续拥有已完成范围。
- Non-goals：不新建状态机/公共删除 API/批量 hold API、通用 owner/IAM/DR、D2–D6、provider cancel/回滚承诺、真实删除、部署或旧环境清理。原 pre-ISSUED、worker faults、archive/restore、LC-13 不重新列为开发缺口。
- Blockers：越权/泄漏、活跃 pin 被删除、未确认停止即解 pin、UNKNOWN 重发、hold 范围夸大、历史或成本丢失、伪全覆盖、缺 fence/事务/必需路径；其他扩展继续归 #307 / #296。

Reuse Check：复用现有 `FixtureSubjectPins`、NATIVE/PinnedEngine、DirectData/HTTP、`DispositionService` / `ExactExecution`、workspace delivery fence、PostgreSQL、Outbox→Redis/Asynq recovery、MinIO Adapter、call journal 与 Cost/Audit/Evidence。已有原生 API 和测试 barrier 足够；不引入 Temporal/第二队列、storage engine、policy DSL、审批系统或 fork。新增 Go 文件仅为测试，未改 Core/runtime/migration/依赖。

## 2. 四个阶段与 hold 的真实范围

1. **Dispatch/start commit**：冻结 exact target，追加 evaluation/dispatch/attempt 与 recovery obligation，建立排他 reservation；此时不证明发送，更不证明删除。
2. **ClaimExactSend commit**：在相同 fence 下再验 caller、approval、policy/retention/hold/reference/pins；持久化一次发送 claim。dispatch 后、claim 前到达的 hold 或 approval revocation 必须使 claim 拒绝。
3. **Wire invocation**：provider 实际接收 exact DELETE；不能与 PostgreSQL 原子提交。claim 后即使尚无响应，也不能假装能够撤回。只有可信 wire witness 才将调用记 CONFIRMED；没有证据的 claim 只表示可能发送，不能伪造确定成本。
4. **Absence/completion**：完整版本盘点 + exact identity + journal 权限/时序证明，才追加 per-copy absence。只有冻结全部 required targets 都有合法 receipt 才能 completion；200、HEAD404 或 delete marker 均不足。

**现有 `PlaceContentHold` 只接收 CopyID，不是 plan-wide hold。** 对 A 的 hold 不会自动保护 B/C；不能用测试调度“B 暂未启动”伪装该业务保证。有限 late-hold operator 先调用已有 `RevokeDispositionApproval`，在 fence 下停止该 plan 所有未 dispatch / 未 claim 成员；再逐 copy 调用 `PlaceContentHold` 并保留 conflict facts。中途崩溃仍有已提交 revocation，后续只恢复 hold/只读对账，不能重启 sender。另一个 approval 不属于本用例，必须由单 writer 确认不存在，不能假设 revocation 是通用全局停机。

原生 provider 在途 DELETE 无可验证 cancel/rollback contract。本片不会调用解除保护、bypass 或额外 provider hold/stop RPC；平台 hold 是业务事实，不冒充 native lock。若要求“保全请求到达后一定挽回已在途字节”，当前模式 **UNSUPPORTED**，不是本片可以补写的 PASS。

## 3. 原生运行的有限安排（本轮全部 NOT_RUN）

后续使用新合成 project / workspace / provider incarnation 和独立 named volumes；建议按 business-races、late-hold、mixed 三组隔离，每组一个 workspace、两个既有固定 namespace，case 专属 key/version，不能复用已删对象或为方便解除 hold。先核对所有权、named-volume 持久化、无 tmpfs/WAL 漂移、internal network/no host ports，再安装严格当前 schema。旧 5d/9812f0fc/LC-13 环境及备份不启动、不迁移、不重用。

本轮**没有创建这些 native 环境**；project/provider/incarnation、key/version、plan/hash、execution、binary、IAM、gateway、Core checkpoint 均尚未冻结。下面的 case ID 不是可执行 profile，也没有发送凭证或授权。未来先非删除 provision/GET/LIST/protection/Core 备份恢复，获得真实 IDs 后生成独立精确清单和有限窗口供用户确认；不得用占位符、现有 v15 execution 或旧签名 envelope 开始发送。

| Case / LC | 暂停点与顺序 | 未来必须收集的原生结果 |
| --- | --- | --- |
| P1 / LC-01,11 | 真实 Asynq→NATIVE Execution 已提交全部 input typed pins，实际 GET 后暂停 owner；restriction 后尝试 dispatch | 活跃 owner 时 DRAINING（占用查询/拒绝 gate 事实，不虚构持久状态枚举），0 attempt/claim/DELETE；完成或可信停止、完整 handles/output 已闭合、module-owner reference release 后，新 command fresh dispatch 才能获准一次 exact DELETE；输入不得在活跃期间消失 |
| P2 / LC-11 | queued Execution 先建立；restriction 先提交，worker 真读前拒绝；按既有 Command 结束/可信停止并由 module owner release 其 byte demand，才可建立 reservation。之后触发旧队列 replay/start/retry/recovery | 各入口 0 新 GET/PUT/输出 READY；不得以排队时 gate 或旧 snapshot 放行。另行获批一次 DELETE 后只读确认 absence；UNKNOWN owner 不为此强行解 pin。不能拿仍有未释放 Execution input material demand 的 plan 强行制造“先删除” |
| T0 / LC-12 | 复用已验 pre-ISSUED pause contract；新增组合 PG 检查作为依赖 | pin 与 ISSUED 同事务、commit 前 0 provider read/response bytes，竞争 restriction 实际观察 PG lock wait。原生既有 PASS 保留；不凭它认定实际 DELETE 竞争通过 |
| T1 / LC-12 | 真实 authenticated DirectData 已 ISSUED，实际慢 TCP stream 正在输出，独立 owner 撤不掉的 write/flush 暂停期间申请处置 | active transfer pin 阻断 dispatch；cancel/deadline 本身不解 pin。确认 native read + writer/flush 终止后追加 transfer observation；旧 ISSUED 不改写、same-key replay0 payload/I/O；新 gate/批准后才允许一次 exact DELETE |
| T2 / LC-12 | restriction/reservation 先于新 DirectData admission | trusted caller/current rights 重新验证；0 payload、0 native read、0 active transfer pin。后续实际删除与 absence 另按本 case 精确授权；不复用 T1 数据或凭证 |
| H0 / LC-06 | hold 持 fence，dispatch 为已观察到的 SQL lock waiter | hold 先提交后 0 attempt/claim/DELETE；原生只读 protection 佐证保留；hold 不释放 |
| H1 / LC-07 前边界 | dispatch/start 已提交但**未 claim**；撤销 approval，再追加 copy holds | 保存 PreservationUnconfirmed/conflict（dispatch 已产生可能风险），fresh claim 拒绝、remaining target dispatch 拒绝、0 DELETE；不把 durable start 当确定调用成本。只在能证明未发送时追加 NOT_SENT，不伪造 absence |
| H2 / LC-07 在途 | A 已 claim，gateway/provider wire witness 确认实际 DELETE 在途；暂停响应，先撤销 plan approval，再追加 A/B holds，最后制造 response loss | A 的 hold/conflict 与 UNKNOWN/原 attempt/cost 保留；B 未 dispatch，0 DELETE；不得宣称 A 保全成功。只读 Outbox/Asynq recovery 追加 A exact absence 或 UNKNOWN_PRESENT；不重发，不补删 B，不允许全 plan completion |
| M1 / LC-14 | 同一 immutable DatasetVersion/hash 的三个独立 exact copies A/B/C；A 成功，B response timeout/loss，C 原生 retention 未到期 | A 经完整 inventory 追加唯一 absence；B 首个 UNKNOWN 后、lookup 前先记录 mixed checkpoint；C native GetRetention 及 gate blocker、0 claim/DELETE。plan 仅 PARTIAL/UNKNOWN，completion0；A replay0 I/O，B 无 resend，C 保护与 bytes 不变 |

M1 固定复用 v15 已验的 **native forward + response-loss** 路径：先完成 A 的 absence，再发送 B 一次；保存 B 的 CONFIRMED/UNKNOWN 与成本后暂停其既有 recovery，检查 C 阻断，采集“一份 absence + 一份 UNKNOWN + 一份 retention blocked”的 checkpoint。不能在 B timeout 后先全量盘点 B、已观察到其 absence 却仍声称物理结果完全未知；不能伪造 provider 超时/对象仍存在。之后仅只读恢复：若 B 仍 present，保留 UNKNOWN_PRESENT 与 reservation；若 B 已 absent，追加第二份 receipt，但 C 仍保留、completion 仍0。两个合法后续分支都必须可表达，不能把 timeout 推导为 present，也不能覆盖首次 UNKNOWN。A/B/C 的完整真实版本盘点与保护回读不得省略。C 本片证明的是 native retention 读回支撑的 **gate 阻断、0 DELETE**，不冒称已经获得实际 DELETE403 回执。

有限 native sender 必须固定单次物理请求、SDK retry=1、redirect 禁止、exact version condition、gateway deny-all 基线和受限角色；最多一次/获批 target，UNKNOWN 不重发。具体发送上限/固定窗口、fault endpoint 和分支在新候选冻结时填写，本轮发送预算严格为0。

## 4. 证据与验收顺序

每组先固定 source commit + 原字节/LF hash contract、实际 provider build/SDK/image、allowlist/target digest、禁止项、所有 survivor、Core dump/globals/roles/ACL/完整表 count+hash 和 native bytes/protection 备份；独立恢复必须先证明，不把合成新库叫旧事实恢复。

每个 case 记录单调序号与数据库提交点、fence revision、backend PID/实际 lock wait、pin kind/subject/generation/termination、attempt/claim/invocation/observation、native witness 与前后 inventory。外部 I/O 不持 DB row locks；barrier 在受限 fixture wrapper 内，不能靠 sleep 推断竞争。跨 DB/provider 时间顺序须靠可关联 witness，不能只比较不同机器 wall clock。

每次真实 GET/LIST/protection/DELETE/lookup 都按原 journal/physical attempt 记 invocation certainty、quantity/unit 和 typed CostAllocation；同一 observation replay 不重复成本，新 lookup 不被顶层幂等吞掉。未知 invocation 与确定 invocation 分栏。Event/Audit/Evidence/Outbox、original FAILED/ISSUED、旧 snapshots/releases/lineage 保留；输出 intent 未闭合时不得 release processing byte demand。

结束时先关闭 sender/recovery/reader/producer 权限和窗口、gateway deny-all，再核对逐 copy inventory/保护/成本/历史、原生身份 disabled、容器完整ID停止/PID0/noports、备份 hash 和 retained volumes。失败保留原回执与 helpers，不 cleanup、不静默重跑；对账超窗只在另行确认只读 authority 后进行。receipt 明确排除所有备份、provider 内部介质/replicas、其他环境和外部 recipient。

## 5. 当前验收矩阵

| 范围 | 本轮非删除准备 | 原生缺口 / 判定 |
| --- | --- | --- |
| P1/P2 | PASS_PG：typed processing pin 阻断 dispatch，确认 owner 结束后新 dispatch/claim；下一 RUNNING subject 的读取 admission 拒绝；已有双方 fence 回归通过 | native Asynq/NATIVE × actual DELETE 两种顺序 NOT_RUN；SQL RUNNING subject 不是实际 queued delivery/retry/recovery |
| T0/T1/T2 | PASS_PG：ISSUED + transfer pin/stream 阻断 dispatch，终止后才 dispatch；same-key replay 无 payload，新 operation 在 FK admission guard 已拒绝、未到 pin；既有 pre-ISSUED 事务可见性回归通过 | 实际 TCP × DELETE 两种顺序 NOT_RUN；不复开已完成 pre-ISSUED native 证明 |
| H0/H1/H2 | PASS_PG：hold/dispatch 双向实际 lock-wait、claim 前后 revocation/hold、未 hold 的 B 先被 plan approval revocation 阻断；UNKNOWN/成本 replay 保留 | H2 wire pause/late hold/response loss 原生 NOT_RUN；不能承诺 cancel |
| M1 | PASS_PG：同内容三 copy，A absence、B timeout、C retention blocker；B 后续 present/absent 两分支均 completion0，原 UNKNOWN/每次成本保留，A replay0 I/O | 三 copy 原生 NOT_RUN；B 复用既有 response-loss 路径；C 不支持未经授权 destructive probe |
| 旧 v15 / LC-13 / archive-restore | 继承已合并证据，旧失败历史不重标 | 不重新索取旧目标删除授权，不计入本轮 provider I/O |
| 新 native 候选 / 精确确认 | NOT_PROVISIONED / NOT_FROZEN / NOT_AUTHORIZED；本轮预算0 | 真实 environment/key/version/plan/hash/execution/checkpoint 全部尚缺，不是可授权发送清单 |
| 独立评审 / GitHub 同步 | PASS_FINITE_REVIEW：bca29f4d 的独立只读评审 BLOCKER=0；原26 payload Git raw bytes 重核一致；远端分支同步成功，#323 Draft | 未重跑 PG/native；最终更新提交 CI 按其 exact HEAD 在 PR checkpoint 核对，不沿用 bca 或 #318 CI |
| 整体 D1 / #307 | NOT_ACCEPTED / OPEN | 本轮完成设计与 PG 准备不等于完整 D1 已验收；不部署、不合并新的 PR |

证据见 [离线核验](evidence/issue307-d1/remaining-preparation01/preparation-audit.json)、[原始成功 Go JSON](evidence/issue307-d1/remaining-preparation01/pg-race-attempt05.jsonl)、[原始进程回执](evidence/issue307-d1/remaining-preparation01/pg-race-attempt05-process.json)、[最终宿主封存](evidence/issue307-d1/remaining-preparation01/scratch-seal-final.json)及 [索引](evidence/issue307-d1/remaining-preparation01/evidence-index.json)。9 个顶层用例、13 个叶用例，Go JSON 含父子共17个 PASS、0 FAIL、0 SKIP；`go vet ./internal/lifecycle/application` 通过。实际使用 Go1.26.5/windows-amd64 + 已固定 image SHA 的 PostgreSQL17；不是冻结 native Linux binary 的复验或新提交 CI。

这些组合使用既有 **synthetic disposal dependencies / deterministic DirectData gate / SQL-only reader port / observation verifier**，只证明 PostgreSQL 事务协议与 typed pin 组合；不证明真实 current certification/rights、module-owner reference closure、native GET/DELETE、TCP、Asynq、provider hold/retention 或真实成本。原生场景必须运行实际服务，不得把这些 fake dependencies 接进 sender。

本轮新建 PG-only scratch `dpp307-remaining-pg-c20631a616a9`，专属 writable named volume、私有凭据文件；Windows Go 测试通过仅 `127.0.0.1` 的临时端口连接。Docker internal-only 网络不分配该端口，因此另附仅该新 PG 的独立 host-test bridge；这不是 native acceptance topology/noports 的证明。测试结束 PG stopped/PID0，无活跃端口，卷/两个新网络/凭据保留；旧 5d 三个 Core/provider/gateway 服务仅 inspect，均 stopped/PID0。既有 Go scratch harness 只创建/丢弃它自己随机命名的测试数据库，不接 retained Core/provider。

## 6. 失败修正与本轮 closure

- Operator 01：numeric SID 传给 `icacls` 缺 `*`，在凭据/容器创建前拒绝；修正 SID 格式，确认失败目录为空再继续。
- Operator 02：Docker 默认地址池耗尽；核对现有网络/host routes 后使用不重叠显式网段，不清理旧网络。
- Operator 03：新 internal-only scratch PG 的 port metadata 不符 host test 预期；为它增加专属 host-test bridge，保留原拒绝，未弱化 native/noports 规则。
- PG attempt01：新夹具冻结时钟早于 SQL-only reader 的 wall-clock journal，Start/Observe 被正确拒绝；另漏授 synthetic PreparePlan capability。改为 reader 用实际时钟、显式授测试 capability，未改 runtime gate。
- PG attempt02：三 copy fixture 使用纳秒时间，超过 Core 微秒时钟，recovery 正确拒绝 inventory；按既有 Core precision 对齐测试输入。
- PG attempt03：上述根因修正后9组全部通过。
- Closure 后收紧拒绝断言，PG attempt04 显示新 DirectData 被 FK admission guard 更早拒绝，测试错误预期为后置 `ErrUseBlocked`；修正为当前 DB guard 的精确 code/message。attempt05 对最终测试源码重验9组通过；旧 FAIL 和三份 operator 失败回执全部保留原字节，不重标 PASS。

当前 writer 做有限 closure audit：权威分工、四阶段、copy hold/plan stop 范围、reference release 前置、timeout 两种后续、历史/成本/幂等边界已明确；本轮非删除准备无已知 blocker。**未执行新独立评审、未创建 native 候选或删除批准、未验收完整 D1。** 新代码仅测试；保留环境/运行时/migration/依赖不改。下一轮先准备新的 native fixture 和只读清单，再独立核对 source/binary/Core/protection/窗口；获得新的 exact-list 用户确认之前持续 DELETE=0。

远端边界：本轮起始 GitHub 读回 #307=OPEN；结束刷新时 gh GraphQL/REST 返回 EOF、GitHub connector transport 失败，见 [回执](evidence/issue307-d1/remaining-preparation01/remote-readback.json)。本轮没有关闭 Issue 或其他远端写入；本地文档/回归不冒称新 PR CI 或已同步 main。

## 7. GitHub 恢复与独立复评（2026-10-16 追加）

本次用户授权恢复同步和独立评审；原始 `remaining-preparation01` 的 26 payload/index、失败日志、原 PASS 和 EOF 回执均保持原字节。仅追加当前报告与入口，不将旧网络失败重标为成功。

主线程刷新 `origin/main=b823eb33`、本地 clean/behind0，推送并 `ls-remote` 读回 `codex/issue307-d1-remaining-preparation=bca29f4d`；GitHub REST 读回 #307=OPEN。建立独立的 Draft #323，原 #318 已合并，不修改其代码或重复使用旧删除授权。见 [同步回执](evidence/issue307-d1/remaining-review01/sync-readback.json)。

独立 reviewer `/root/d1_remaining_independent_review` 对 **bca29f4d**（base **b823eb33**）完成源码/底层实现/原始 Go JSON/真实 Git blob 的只读交叉复核，结论见 [独立评审回执](evidence/issue307-d1/remaining-review01/independent-review.json)。正式 GitHub APPROVED 与该离线 agent 评审不是同一事实；此次未声称存在新的 GitHub 审批。

| 分类 | 复评结论 / 当前处理 |
| --- | --- |
| BLOCKER | **0**。没有发现须改 Core、runtime、migration、依赖或既有 PG 测试的有效阻断；本次仅更新文档与证据，不为制造修复而改代码 |
| 已核对设计 | dispatch/claim/wire/absence 四阶段；copy hold 与绑定 approval revocation 的有限 plan stop；已 claim 请求不能假装撤回；P2 material reference release 前置；timeout 不推导 present，M1 所有 receipt 未齐不能 completion |
| 已核对 PG 证明 | processing/transfer typed pin 与 dispatch 组合、DirectData FK admission 精确拒绝、same-key 无 payload/旧 ISSUED 保留；既有双方竞争确实观察 PG Lock waiter；M1 原 UNKNOWN/两种恢复结果/独立成本/A replay 有对应断言 |
| 已复算证据 | 独立运行原 offline audit；从 bca29f4d Git blobs 逐份核对26 payload的 SHA/长度、新测试 blob与 attempt05 绑定；9顶层/13叶/17父子 PASS、0 FAIL/SKIP。主线程再次复算见 [回执](evidence/issue307-d1/remaining-review01/offline-recheck.json) |
| FOLLOW-UP，owner=#307 | 新 native fixture、P1/P2/T1/T2/H2/M1 的实际组合验收；source/binary/Core/IAM/gateway/exact-list 冻结与新授权。维持 NOT_RUN / NOT_PROVISIONED / NOT_FROZEN / NOT_AUTHORIZED，不扩大成本或处置覆盖 |
| INVALID / OUT-OF-SCOPE | 用 SQL-only PASS 宣称 native PASS、要求在途 DELETE 必然可撤销、把通用批量 hold/IAM/DR/D2–D6/旧环境升级加入本轮，不作为本切片 blocker |
| 最终提交 CI | bca29f4d 的初始 PR run 为38857638865；本次追加提交须按新的 exact HEAD 核验 CI。最终 run/13项结果、unresolved threads 和 Draft 状态记在 #323 最新 checkpoint；不把受检 bca、合并前 #318 PR CI 或 merge/main CI 冒充新 HEAD CI |

此次 reviewer 未独立重跑 PG，主线程也未重启任何 DB/container/provider；这与已有 9 组本地 PG race 的历史证明分开。无 native 新候选、真实删除、hold release/bypass、旧环境清理、Ready、merge 或部署。[新复评证据索引](evidence/issue307-d1/remaining-review01/evidence-index.json)与原索引分别保留。


## 8. 新原生准备候选与验收矩阵（2026-10-16 追加）

已创建新的 candidate06-preparation/v16，operator execution `fc9d632a-45fc-445a-9652-211db29694b3`，冻结 source `de5f7dd2683cc5b5ed3a68528364d6fe59836f10` 及768文件双hash、实际 prepare/verifier/reference binary、三套 Core checkpoint/roles/ACL/native archive、IAM与active/sealed gateway。精确环境、key/version、8 plan/hash、16 copy/DatasetVersion/bytes/protection、七 prospective target与九survivor全部见 [精确绑定与确认前方案](issue307-d1-remaining-native-confirmation.md)。旧 §3/§5–7 的未创建文字保留其历史时点；本段拥有当前准备状态。

| 范围 | 当前证据 | 原生业务验收 / 权限边界 |
| --- | --- | --- |
| 新环境 | business `808a6960`、late-hold `754ea8f0`、mixed `dfda43b4`，独立named volumes/internal networks/noports；16版本/8 PREPARED plan | 旧5d/LC13与旧授权未复用，cleanup=0 |
| bytes/inventory/protection/IAM | 16 exact GET +完整版本清单+native hold/retention；delete/inspection均disabled；IAM native回读+OSS set语义一致 | 初始54个typed inspection calls/observations/CostAllocation保留；restore bootstrap GET/HEAD journal不是业务CostEvent |
| Core / native restore | 三组每库194表，表row/hash、角色、table/schema/function/default ACL一致；native tar归档/文件SHA256/restored GET/保护读回PASS | dump/globals/tar/restore volumes/helpers全部保留，未用新业务库冒充旧事实恢复 |
| P1/P2/T1/T2 | 各有独立目标版本/plan，P1/P2复用既有合成CSV，T1有背压payload；非删除技术准备PASS | 实际Asynq/NATIVE/HTTP CurrentDeliveryGate/owner停止与reference release尚未接线，native race NOT_RUN |
| H0/H1/H2 | 固定layout、plan成员、受限IAM与gateways已准备；H1/H2-B不在prospective allowlist | wire pause/可信witness/claim边界新operator尚缺，不能宣称late hold挽回在途字节 |
| M1 | 同一DV/hash三个exact copies，C真实GOVERNANCE retention且预算0 | A absence/B UNKNOWN/C blocked mixed checkpoint与只读恢复仍NOT_RUN；不冒称实际DELETE403 |
| 新工具验证 | unit race 6顶层/含子项10 PASS，0FAIL/0SKIP；vet PASS；原PG证据离线重核PASS | 未重跑PG并发/native业务竞态，没有新独立评审结论 |
| 候选/精确确认 | 全部真实准备绑定已冻结，current budget=0，active_window=null，旧sender新profile unsupported | BLOCKED_NATIVE_CASE_RUNNER_AND_NEW_EXECUTABLE_FREEZE；不是可授权删除文本 |
| #323 / #307 / D1 | #323 Draft，#307 OPEN；所有新容器stopped/PID0，无真实删除/hold-release/bypass | NOT_ACCEPTED，不合并，不部署 |

五份原始失败均留存并完成根因修正；两个excluded失败环境的12份版本不删除，808a的continuation只对已恢复副本进行只读比对/核验，不重放provision/restore。失败5f6a的inspector未及disable；该环境全停/PID0/noports、平台authority关闭，严格排除新候选，不夸称全部原生身份disabled。精确方案列明后续五项有限前置：真实业务非删除准备、受限reader/producer与pause/witness、七目标sender非删除回归、独立评审、新可执行候选及固定UTC窗口精确确认。


### 8.1 当前 CI 历史核验入口修正

`10d7861d` 的 PR CI `38862728693` / demo job `116848534219` 在旧 candidate05 的 current-tree hash 检查失败；原因是本轮真实 fixture 工具改变了当前 `main.go`，而旧 v15 冻结源码必须保持 dbe3fbd6。参见 [原失败及根因回执](evidence/issue307-d1/remaining-native-ci01/regression.json)。

修复复用已存在的 historical snapshot 模式：新增固定 candidate05 public-only wrapper，对 b823eb33 中的35个旧payload/index逐字重核，再装入 dbe3fbd6 的317份原Git源码，运行**未修改的原checker**，继续严格核对344份更早raw历史。当前工作区源码不覆盖，不能执行任何native operator或授权。新增篡改payload/index、损坏runtime、越界archive/CLI反例，正常/`-O` 各4项PASS；完整旧checker PASS。CI同时加入当前v16离线audit，Go gate保持原样。冻结候选de5f/source768及全部16版本、Core/IAM/gateway绑定未变；此CI-only新增文件不是新发送候选。最终CI按修复后HEAD在PR checkpoint核对。


## 9. candidate07/v17 可执行冻结与非删除收口（2026-10-16）

当前有限切片已由准备阶段推进至 `PREPARED_EXECUTABLE_NOT_AUTHORIZED`；唯一execution `effa0385-9308-42eb-bd66-d0c379b6a989`，candidate SHA256 `b9660f17cbb745e781a06d50e8ec0e81c87e654318420533b75b4ada29728949`，源码 `f3b5de45857add3f852bbac0348e8c30ea105ade`（812文件），reader `8a9800bd3a77c75d3364cfe01712e7de1dcb060111a4b43c4bb90820b7b2a4b1`、sender/wire/recovery `ef6dbfd8561ac6a070a7e1d19c45caec0042924cbc268a8a58ff0f85a5bf2f9a`、root bootstrap `68ad4a761a105e7d5ad2a678546f80347222b6fab11c0b4b64b49f15989f9d42`。新的 [精确确认方案与验收矩阵](issue307-d1-candidate07-confirmation.md) 拥有当前状态；§8及v16保持历史。

交付目标是有限原生case runner、受限reader/producer、single-send/wire/readonly recovery接线与可审核冻结；authoritative semantics仍为本contract、architecture lifecycle及新精确文本。必需invariants为：exact scope/版本与新授权、共享fence、owner真实停止才解pin/ref、immutable attempts/账务、persistent单次budget、UNKNOWN不重发、source/backup保留。D2–D6、通用IAM/DR、用户未授权的真实删除、Ready/merge不在本轮。

| 切片 | 当前结论 |
| --- | --- |
| P1/P2/T1/T2 | 最新冻结reader真实Asynq/NATIVE/DirectData非删除PASS；P2 queue/Retry/Start/NativeReconciler均0新增IO，T1取消背压期间pin保留至owner闭合；same-key409与T2当前门禁403均0payload |
| H0/H1/H2 | schema69 clone用冻结binary重跑H0真实PG waiter与H1签名NOT_SENT PASS；H2仅ready/unclaimed，真实wire后的late hold/UNKNOWN尚NOT_RUN |
| M1/C | 同DV三份版本保留，native GOV retention实际阻断C PASS；真实A absence/B UNKNOWN/C blocked及随后B lookup尚NOT_RUN |
| Core/native/库存 | 三源18版本（7拟删除+11survivor），含两份真实业务输出；三份新完整dump/globals/native tar及独立恢复、最新只读指纹/保护/IAM均PASS，所有identity disabled |
| 新必要设计修复 | CopyPolicy显式immutable续期，expected-head单链+固定锁序；真实PG双Renew一赢家、restriction/hold/pin六顺序waiter与旧pin/operation/checkpoint保留PASS；≤1h不放宽，未来窗口开始后才Renew |
| 当前权限与工具 | 7case/17operator/15root-bootstrap实际CID冻结；17+7封存拒绝、Go offline+正常/-O篡改反例PASS；独立root IAM与host attester已源码化，当前无human/无send permit/无topology proof |
| 源码冻结与续跑 | 原脚本stdout-only漏采Docker stderr导致最后断言失败；frozen candidate/source/profile未改，有限completion只补封存回执、offline和索引，原失败seal保留；940 raw证据逐字重核PASS |
| 完整D1与仓库状态 | NOT_ACCEPTED；#323保持Draft，#307保持OPEN；真实DELETE/cleanup/hold-release/bypass均0；旧授权不复用 |

复用既有ports、PG Commands/fence、queue/reconciliation、call journal/Cost/Audit/Evidence/Outbox；MinIO、madmin、Asynq和cryptography承担通用能力。新增migration69只解决保留same DV所必需的fixture policy续期，保留全部旧事实，未引入新基础设施。独立复评及最终exact HEAD PR CI记入本轮最终checkpoint，不冒充main CI。

本轮[最终独立只读复评](evidence/issue307-d1/remaining-final-review01/independent-review.json)结论为当前非删除切片BLOCKER=0；最终精确HEAD Linux PR CI须单独核对并在GitHub checkpoint记录。Windows额外全模块测试存在一项未改动metadata子进程500ms启动预算失败，原失败保留；独立诊断确认首次启动1404ms、隔离五次通过且UNKNOWN fail closed，不把它改报全模块PASS。67个精确保留owner CID只读复核均停止/PID0/noports/no restart。

9a41首次CI的gofmt与原Caddy回执Git字节问题已按根因修复：只格式化唯一IAM测试，原CRLF重新入库；新增固定f3/9a41锚点的严格冻结快照校验，940份raw同时核工作区与Git blob，原812源码及原checker保留。当前运行源码必须逐字一致，唯一测试只容许固定gofmt结果，任何其他源码漂移或证据改写均拒绝；原候选与实际binary来源仍为f3，最终CI结果随新精确HEAD在GitHub checkpoint记录。
