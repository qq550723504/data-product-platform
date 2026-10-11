# #87 — External usage/runtime evidence local verification

2026-10-11，Asia/Singapore。开发前核对并复查 main `253ce3b4ef044a233de1179534422eed04ded660`：#84/#86 代码已由 #309/#311 合并，matching push/main CI `38136308837` 全绿，Issue #86 仍 OPEN。现有 Draft #295/000050、#310/000056 与 #307/000054 的未合并实现未继承；本次 sole writer，branch `codex/issue87-usage`，migration 000058。

下表是创建 Draft 时的本地验证快照。后续 Ready/review/CI/merge 状态以 PR #312 的 exact-HEAD 记录为准；新增合并授权见设计文档，不回写初始验证结果。

权威为 [External usage evidence](../architecture/external-usage-evidence.md)、[External publication execution](../architecture/external-publication-execution.md)、[External Core](../architecture/external-integration-core.md)、[Issue #87](https://github.com/qq550723504/data-product-platform/issues/87) / parent #47、ADR-0009 与 AGENTS §17。设计先于开发落地；复用 registry、principal/Authority、PostgreSQL transaction/session lock、worker reconciliation、command idempotency、Evidence/Audit/Cost/Outbox，没有新增 collector、queue runtime、SDK 或 OSS fork。

| Check | Status | Evidence |
|---|---|---|
| Latest main / dependency / architecture / reuse | PASS | #84/#86 已合并；冻结 source、normalized metadata、exact publication correlation、finite scope 后开发 |
| Shared fake + loopback HTTP adapter contracts | PASS | replayable same cursor、stable source/id/hash、每次 dispatch 一次只读调用、取消/拒绝 dispatch 0 I/O、严格 JSON/128 KiB/page context |
| Categories / dedup / quarantine / recheck | PASS | USAGE/ACCESS/DELIVERY/CONNECTOR_LOG 接纳；same event/hash 不增加 usage/Evidence；changed hash 冲突隔离并去重；unknown/unmapped 不猜关联，mapping 可用后追加 accepted decision |
| Current authority / isolation / concurrency | PASS | 同 pull 8 并发 owner 一次 I/O；同 revision 的另一 pull STALE；caller/binding/instance/revision/capability 改变 fail closed；workspace/subject/remote ID 隔离 |
| Recovery / physical cost / deadline | PASS | lost response/start-only；saved PAGE + acceptance fault 恢复 0 新 I/O；outcome commit fault 同 cursor 重读并记第二次成本；5 秒 deadline；失败/撤销仍保留实际 invocation 成本 |
| Historical and production evidence integrity | PASS | canonical event/decision/outcome/checkpoint UPDATE/DELETE 拒绝；Core Release/version/membership 与独立 FINALIZED EvidenceSnapshot header/root/items digest 不变；accepted Evidence 追加 DOWNSTREAM_RUNTIME relations |
| Migration 58 / page completeness | PASS | fresh full install、pinned empty down/up；down 等待 parent-first writer 且历史提交后拒绝；checkpoint 不能跳过事件，orphan occurrence commit 拒绝；恢复完整 page 成功 |
| HTTP / routing / worker | PASS | 认证、当前 Authority、strict empty/typed body、foreign 404、status filter；bounded queries；retention-only events + persisted pending pull scanner，未新增 queue handler |
| Full Go / vet / build / race / diff | PASS | 75 个有测试的 Go package；api/worker/migrate/outbox-replay build；externalintegration 全包 race；完整输出和工作区 source/log hashes 见 manifest |
| Exact candidate PR CI / independent review | NOT_RUN | 本地验证不代替候选 HEAD 的 GitHub CI / review |
| Ready / merge / matching main CI for this change | NOT_RUN | 仅授权 push + Draft PR |
| Formal NDI / real provider / production IAM / browser / deployment | NOT_RUN | synthetic fake/reference；没有真实 provider data/credential、付费调用或部署 |

最终安装/验证使用保留的 PostgreSQL 16.15 container `dpp-issue84-pg-20261011` 内新测试库 `issue87_final_verified`；现有库保留。Redis 7 container `dpp-issue86-redis-20261011` 仅 loopback，Go full suite 的队列 contract 使用专用 DB 15。Migration 测试只删除它自己创建的随机 scratch DB；故障 trigger 仅命中 synthetic workspace 并清理。fixture 的 Core PUBLISHED prerequisite 不构成 ReleaseReadiness 或真实 provider 产品验收。

复现目录为 `apps/platform`。设置专用测试库 `POSTGRES_DSN/TEST_POSTGRES_DSN` 与专用 `REDIS_ADDR`，执行 `go run ./cmd/migrate -dir ../../migrations`、`go test ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker ./cmd/migrate ./cmd/outbox-replay`、`CGO_ENABLED=1 go test -race ./internal/externalintegration/... -count=1`。缺少测试 DSN/Redis 时对应验证是 SKIP。Race run 未设置 Redis，仅 PG/usage concurrency 实跑；Redis contracts 已在 full suite 实跑。其他未配置外部 engine 的 live tests 仍为 SKIP，75 packages PASS 不表示所有外部产品路径已验收。

[Verification manifest](issue87-local-verification.json) 保存 source/log SHA-256；原始输出位于本 worktree ignored `apps/platform/tmp/issue87/`。故意制造的 checkpoint RED 在新增 DB guard 后转 GREEN；失败安装/中间数据库保留作诊断，最终结果来自 fresh final verified schema。Billing/settlement、正式 NDI adapter、raw log search、持续自动采集、source ownership transfer、真实交付和生产部署为后续范围。

## Review round 1 — removed adapter recovery

[P1 原线程](https://github.com/qq550723504/data-product-platform/pull/312#discussion_r4241655962) 指出 registry/provider 移除后，durable pull 被 scanner 过滤，永远 PENDING。`TestUsageRemovedAdapterIsResolvedAfterWorkerRestart` 在 empty registry / unrelated provider 两例均先 RED（PENDING），修复 RunOnce 的 empty-registry 提前返回与 PendingUsage 的 provider filter 后均 GREEN。每次重新创建 importer 模拟 worker restart；要求追加一次 BLOCKED、0 provider reads / dispatch / cost、0 imported events、checkpoint revision=0，再重启不重复 resolution。25 条 bound、旋转扫描、source session ownership 与 fresh-authority RunPull 继续复用。

PASS：新测试与 adapter/caller/concurrent-pull 定向回归、75 个 package 全量 Go、vet、api/worker/migrate/outbox-replay build、externalintegration 全包 race（本轮 Redis 也已配置实跑）、gofmt/diff。真实 PostgreSQL 使用新增 `issue87_review1`，保留已有测试库。容器重启后重新发现 loopback ports；不可沿用上次随机分配的端口。Migration 未改变，fresh up / down 与并发历史保护随全量测试实跑。

本轮 source/log 指纹见 [review verification manifest](issue87-review1-verification.json)。Exact-HEAD GitHub CI 与 reviewer 👍 必须针对修复后的候选重新核对，原 `5ef3a132` 的 CI 成功不替代新 HEAD 门禁。本地验证未执行真实 NDI/provider、部署或数据交付。
