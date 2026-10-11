# #307 D1 开发与验收记录

> 2026-10-11；D1 OPEN。首个开发增量为 non-destructive plan foundation；完整 D1 runtime **NOT_RUN**。
> 用户已授权创建 Issue 并开发，仅受控合成对象；实际 provider 删除（含测试/cleanup）仍须先确认环境和 exact object/version 清单。

## 权威与写入归属

[Issue #307](https://github.com/qq550723504/data-product-platform/issues/307) 属于 [Epic #296](https://github.com/qq550723504/data-product-platform/issues/296)，设计前置 [#300](https://github.com/qq550723504/data-product-platform/issues/300) 已关闭。设计 PR #306 merge 为 `9502bd1caea61dd86e80827142bd1d18269a2d79`，matching main push CI [38125986415](https://github.com/qq550723504/data-product-platform/actions/runs/38125986415) SUCCESS；docs-only required 通过、runtime jobs 跳过。

业务规则仍以 [Lifecycle Contract](../architecture/data-lifecycle.md)、[ADR-0015](../adr/0015-data-lifecycle-disposition-boundary.md) 和 [设计验收 §4–6](../product/data-lifecycle-design-acceptance.md) 为准。本文只记录当前增量与证据，不另建生命周期规则。

当前聊天为 #307 唯一 writer。branch `codex/issue307-lifecycle-d1`，worktree `C:\Users\Henry\.codex\worktrees\issue307-lifecycle-d1\data-product-platform`，基于 refreshed `origin/main@9502bd1`。主工作区保持原 HEAD，不写 #298/#294 的 worktrees、代码或运行环境。App inventory transport 当前不可用；通过 Git worktree、GitHub open PR 和当前 main 重新核查。

Main 已含 migrations 000051–000053，D1 使用 **000054**。Draft #295 仍携旧 000050，原 owner 后续须在发布前解决其排序；本片不恢复该任务。唯一共享改动为新增事件 routing/vocabulary 和 migration runner 重复版本拒绝；未引入新依赖、队列或存储 engine。

## 首个增量的有限 contract

业务结果：可信 server-side authority 可以对**已登记合成 copy IDs**原子创建并冻结一个 exact-version 删除计划，得到可比较 scope hash；同键重放复核当前 authority，scope/actor 变化拒绝。准备计划不等于批准、dispatch 或删除。

- Typed registry 固定 workspace、DatasetVersion、provider instance/incarnation、namespace、key、provider version、content hash/size；Command 不接受 URI、key、endpoint 或任意 target metadata。
- `DRAFT → FINALIZED` 计划与 exact membership 在同一 parent row lock 串行化；数据库重算 `SHA256-LIFECYCLE-PLAN-V1`，冻结后拒绝 header/membership/identity mutation，不能 truncate 历史。UTF-8 byte framing 与 Go 相同。
- Application 复用 deliveryfence、transaction、command_idempotency；Domain Event、Audit、最小 Evidence、retention-only Outbox 与计划同事务。没有 provider invocation，不编造 provider CostEvent。
- 纯 Domain 删除 prerequisite 门禁显式要求当前 disposal authority/approval、retention start/boundary、restrict/drain/shared-reference/hold、inventory、provider retention 与 ledger。零值、UNKNOWN、任意 ANY 不能放行；结果只用于预检，不能授权副作用。
- Migration runner 原来同版本文件会按 applied-version 静默跳过另一份，现改为 up/down 都明确拒绝 duplicate version；允许正常 up/down pair 和已存在的版本 gap。

本增量尚未挂到 API/worker，不配置真实 provider，不登记任何 live object。fixture registry 行只是 SQL synthetic identity，不是 provider verification receipt。当前 authority integration tests 使用受控 test authority，不能代替 live principal/delegation acceptance。

Merge blockers：本增量的 tenant/authority bypass、冻结/幂等/原子历史错误、scope hash 不一致、migration 与测试失败。D1 余项仍是本 Issue 待开发内容，不将它们伪装成此增量已完成，也不关闭 #307。

## 已执行检查

仅使用独立 PostgreSQL：container `dpp-lifecycle307-pg-20261011`，loopback `127.0.0.1:54802`，image ID `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`，data 使用 tmpfs。每个 integration test 创建/移除自己唯一命名的 scratch DB；不挂对象存储，不读/清既有 retained volumes。

| 检查 | 状态 / 证明范围 |
|---|---|
| Domain：canonical/exact hash，bare-key/unknown/foreign/duplicate 拒绝；current prerequisite/approval/retention 负例 | PASS；pure Go，无 provider |
| PG command：UTF-8 Go/DB hash 一致，fresh-authority replay、scope 冲突、同键并发只一套事实 | PASS；真实 PostgreSQL，synthetic SQL facts |
| PG atomic proof：缺 routing 导致事务整体 rollback，未落半套 plan/Event/Audit/Evidence | PASS |
| PG freeze：member-first / finalize-first 双顺序、观察真实 lock wait、late writer 拒绝/完整成员纳入 | PASS；不是靠 sleep 推断竞态 |
| PG immutable/hash/down：forged/empty finalize 拒绝、identity/frozen mutation 拒绝、destructive down 保留历史 | PASS |
| migration version collision / normal pair | PASS；up/down duplicate 明确拒绝 |
| `go test ./...` / `go vet ./...` / formatting 与 diff 检查 | PASS；module tests 未配置 live provider/PG，该轮外部 integration SKIP；上述 PG tests 另有独立实跑证据 |
| CI / independent reviewer / matching main CI for #307 code | NOT_RUN；本地增量未发布 PR |
| provider-native hold/retention/delete、absence receipts、archive/restore、worker/HTTP/data plane、完整 LC cases | NOT_RUN |

执行命令（Go module `apps/platform`）：`go test -count=1 ./internal/lifecycle/... ./internal/platform/routing`（独立 `TEST_POSTGRES_DSN`）以及 migration runner 的 duplicate/pair tests。最终检查与 commit 以 Issue comment 记录为准。

## 后续开发和删除确认

下一步仍在 #307 内：current restrictions/holds/pins 与 reader admission；当前 disposal approval + fresh fenced dispatch；typed physical attempts/observations/CostAllocation；MinIO native Adapter 与 existing worker reconciliation；受控 archive/restore ledger；封闭 HTTP/processing/delivery 路径与完整 LC 验收。

实际 provider destructive test 前先建立独立 D1 storage 环境，生成**已实际登记**的 manifest：project/container/network、provider build/image/SDK、workspace、两个 namespace、每个 key + version/generation/hash/bytes、计划 scope hash、retention/hold、允许动作、cleanup 与 excluded scope。提交用户并收到明确确认后才调用 deletion；当前尚无 live object/version 清单，批准状态 **PENDING**，不得提前用 bare-key 列表或 SQL fixture ID 冒充它。

外部备份/recipient、敏感内嵌 Evidence/Audit、Annotation sealed payload、#298 demo/retained objects 均不在 D1 destructive allowlist。D2–D6 按原设计后续顺序，不增加第二 scheduler 或自行恢复暂停联调。
