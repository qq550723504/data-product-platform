# #307 D1 开发与验收记录

> 2026-10-11；D1 OPEN。已完成本地 non-destructive plan foundation 与 restriction/hold/reader occupancy 两个增量；完整 D1 runtime **NOT_RUN**。
> 用户已授权创建 Issue 并开发，仅受控合成对象；实际 provider 删除（含测试/cleanup）仍须先确认环境和 exact object/version 清单。

## 权威与写入归属

[Issue #307](https://github.com/qq550723504/data-product-platform/issues/307) 属于 [Epic #296](https://github.com/qq550723504/data-product-platform/issues/296)，设计前置 [#300](https://github.com/qq550723504/data-product-platform/issues/300) 已关闭。设计 PR #306 merge 为 `9502bd1caea61dd86e80827142bd1d18269a2d79`，matching main push CI [38125986415](https://github.com/qq550723504/data-product-platform/actions/runs/38125986415) SUCCESS；docs-only required 通过、runtime jobs 跳过。

业务规则仍以 [Lifecycle Contract](../architecture/data-lifecycle.md)、[ADR-0015](../adr/0015-data-lifecycle-disposition-boundary.md) 和 [设计验收 §4–6](../product/data-lifecycle-design-acceptance.md) 为准。本文只记录当前增量与证据，不另建生命周期规则。

当前聊天为 #307 唯一 writer。branch `codex/issue307-lifecycle-d1`，worktree `C:\Users\Henry\.codex\worktrees\issue307-lifecycle-d1\data-product-platform`，基于 refreshed `origin/main@9502bd1`。主工作区保持原 HEAD，不写 #298/#294 的 worktrees、代码或运行环境。首轮 App inventory transport 不可用；第二增量已恢复并重新核查 active threads/artifacts、Git worktrees、GitHub open PR 与 remote main。#307 为 native Git worktree，App 确认它不属于 managed worktree；不修改 App metadata。

Main 已含 migrations 000051–000053，D1 使用 **000054**；该 migration 尚未发布，在当前严格 schema 内增加 occupancy facts，不添加双读/兼容 fallback。Draft #295 仍携旧 000050，原 owner 后续须在发布前解决其排序；本片不恢复该任务。唯一共享改动为新增事件 routing/vocabulary 和 migration runner 重复版本拒绝；未引入新依赖、队列或存储 engine。

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

## 已执行检查

仅使用独立 PostgreSQL：container `dpp-lifecycle307-pg-20261011`，image ID `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`，data 使用 tmpfs、无 volume mounts。首轮 loopback `127.0.0.1:54802`；第二轮 restart 后为 `127.0.0.1:58798`。Lifecycle integration tests 各自创建/移除唯一 scratch DB；migration suite 的旧 T3/B2 case 使用本容器已初始化的基础库。测试完成后停止本容器；不挂对象存储，不读/清既有 retained volumes。

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
| PG races：reader-first / restriction-first；hold-first / occupancy inspection；观察真实 fence lock wait | PASS；未实现 delete dispatch，不作为完整 LC-06/11 验收 |
| PG occupancy history：跨 workspace 拒绝、错误 termination binding/time 拒绝、所有五类 facts 不可 UPDATE/DELETE/TRUNCATE、缺 routing 原子 rollback | PASS |
| `go test -race -count=1 ./internal/lifecycle/... ./internal/platform/routing`（独立 PG） | PASS；9 个 lifecycle PG top-level tests，含双顺序 subcases |
| migration version collision / normal pair | PASS；up/down duplicate 明确拒绝 |
| `go test -count=1 ./internal/platform/migration`（独立已初始化 PG） | PASS；首次在 restart 后空基础库运行的旧 T3/B2 schema precheck 失败，初始化本测试库后补跑通过 |
| `go test ./...` / `go vet ./...` / formatting 与 diff 检查 | PASS；module tests 未配置 live provider/PG，该轮外部 integration SKIP；上述 PG tests 另有独立实跑证据 |
| CI / independent reviewer / matching main CI for #307 code | NOT_RUN；本地增量未发布 PR |
| provider-native hold/retention/delete、absence receipts、archive/restore、worker/HTTP/data plane、完整 LC cases | NOT_RUN |

执行命令（Go module `apps/platform`）：`go test -count=1 ./internal/lifecycle/... ./internal/platform/routing` 及带 `-race` 的同范围检查（独立 `TEST_POSTGRES_DSN`）；migration suite 前用本独立库的 `POSTGRES_DSN` 执行 `go run ./cmd/migrate -direction up -dir ../../migrations`，再运行 migration suite。最终检查与 commit 以 Issue comment 记录为准。

## 后续开发和删除确认

下一步仍在 #307 内：接入真实 fixture authority/current gate/reader owner；当前 disposal approval + fresh fenced dispatch；typed physical attempts/observations/CostAllocation；MinIO native Adapter 与 existing worker reconciliation；受控 archive/restore ledger；封闭 fixture HTTP/worker 路径与完整 D1 LC 验收。既有 production processing/delivery 的全面纳管按 D2 与原 owner 协调。

实际 provider destructive test 前先建立独立 D1 storage 环境，生成**已实际登记**的 manifest：project/container/network、provider build/image/SDK、workspace、两个 namespace、每个 key + version/generation/hash/bytes、计划 scope hash、retention/hold、允许动作、cleanup 与 excluded scope。提交用户并收到明确确认后才调用 deletion；当前尚无 live object/version 清单，批准状态 **PENDING**，不得提前用 bare-key 列表或 SQL fixture ID 冒充它。

外部备份/recipient、敏感内嵌 Evidence/Audit、Annotation sealed payload、#298 demo/retained objects 均不在 D1 destructive allowlist。D2–D6 按原设计后续顺序，不增加第二 scheduler 或自行恢复暂停联调。
