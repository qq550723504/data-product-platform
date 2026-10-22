# #86 — External publication execution local verification

2026-10-11，Asia/Singapore。基线为 main `21762737a76033667df4a138368070868a34fbf8`（#309 squash）；#84 代码依赖已合并，Issue 仍 OPEN。#87 usage 是后续；#307 / 000054、#299 / Draft #310 / 000056 是并行 owner，本次 000057 不继承它们未合并内容。

权威：[External publication execution](../architecture/external-publication-execution.md)、[External Core](../architecture/external-integration-core.md)、[Issue #86](https://github.com/qq550723504/data-product-platform/issues/86) / parent #47、ADR-0009 与 AGENTS §17。目标是可信 workspace caller 的 publication execution、durable submit/poll recovery、immutable attempts/observations、verified external REGISTERED/PUBLISHED 与 physical invocation Cost/Evidence/Audit；Core Release 与 Delivery contract 保持其 owner。

| Check | Status | Evidence |
|---|---|---|
| Preflight / design / reuse | PASS | latest main + dependency/owner/routing/cost/migration audit，先落 authoritative contract；复用 PostgreSQL/Asynq/Outbox/principal，无新增 runtime/SDK |
| Shared fake + reference HTTP contract | PASS | identity/identifier verification、stable same-key publication、definite failure、registered/poll、lost response、dispatch rejection、unsupported delivery |
| Real PostgreSQL execution/concurrency | PASS | same-key concurrent Prepare、queue/scanner single submit、旧 retention intent 扫描补捞、failed→new attempt、UNKNOWN→poll、claim-before-I/O crash |
| Current authority / context / history | PASS | fresh command replay、binding/identifier invalidation、adapter instance/origin identity、wrong receipt、workspace HTTP isolation、DB bypass/immutable guards、冻结 Core release/version/membership digest 不变 |
| Incurrence / commit fault recovery | PASS | 每次 dispatch 独立 CostEvent + typed allocation；FAILED/timeout/poll 不丢成本；start-only 0 cost；observation commit failure 查询恢复，projection commit failure 重放已保存 receipt、不重复 I/O |
| Migration 57 | PASS | fresh full install、固定 57 empty down/up、down 等待并发 attempt writer，历史提交后拒绝破坏性 down |
| Redis / routing / HTTP | PASS | real Redis same-event TaskID dedup + handler；队列错误不伪造确认；#84 retention obligation 不回写；typed bounded reads / strict empty command body |
| Full Go / vet / build / race / diff | PASS | 75 个有测试的 package 全绿；source/log hashes 已记录，最终 down 锁顺序另做 targeted race 回归 |
| Candidate PR CI / independent review | NOT_RUN | 本地结果不代替 exact-HEAD GitHub CI 或 reviewer 批准 |
| Ready / merge / matching main CI | NOT_RUN | 本次仅授权 push + Draft PR，未授权 Ready/merge |
| Real NDI / IAM / browser / production | NOT_RUN | 仅 synthetic fake/reference；没有正式 specs/testbed、真实 provider credential/data、paid provider call 或部署 |

本次端到端测试使用专用 PostgreSQL 16.15 container `dpp-issue84-pg-20261011` 中新建测试库 `issue86_final_verified`，保留原库；Redis 7 container `dpp-issue86-redis-20261011` 仅绑定 loopback。测试创建随机 synthetic workspace 与 scratch DB，故障 trigger 仅命中特定 synthetic workspace 并移除。fixture 的 Core PUBLISHED prerequisite 不代表 ReleaseReadiness 产品验收。

复现：在 `apps/platform` 设置专用测试库的 `POSTGRES_DSN/TEST_POSTGRES_DSN`、专用 `REDIS_ADDR`（queue contract 使用 DB 15，不 flush 其他数据），先 `go run ./cmd/migrate -dir ../../migrations`，再 `go test ./... -count=1`、`go vet ./...`、`go build ./cmd/api ./cmd/worker ./cmd/migrate ./cmd/outbox-replay` 和 `CGO_ENABLED=1 go test -race ./internal/externalintegration/... -count=1`。缺少测试 DSN/Redis 地址时对应验证是 SKIP，不能当作 PASS。

代码/log 摘要与实测状态写入 [verification manifest](issue86-local-verification.json)；原始输出位于本 worktree 的 ignored `apps/platform/tmp/issue86/`。全量 Go suite 不表示所有未配置 external-engine/runtime tests 都实跑。formal NDI/TDS、生产 IAM、真实交付/credential、SUSPENDED/WITHDRAWN lifecycle、billing 与 #87 usage 保持后续范围。
