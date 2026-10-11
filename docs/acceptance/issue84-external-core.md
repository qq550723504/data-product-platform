# #84 — External Integration Core local acceptance

2026-10-11，Asia/Singapore。本次是有限 Core 纵向实现与本地验证，不是 NDI/provider、产品发布或生产验收。

权威与冻结 merge contract：[External Integration Core](../architecture/external-integration-core.md)、[ADR-0009](../adr/0009-national-data-infrastructure-integration-boundary.md)、[Issue #84](https://github.com/qq550723504/data-product-platform/issues/84) / parent #47。#86 执行、#87 usage 继续独立。

## Candidate / ownership / reuse

基线 remote main `3a59b591d25e7d07fc38956a7ae729d74967215b`；开工与最终检查均相同。基线 [push/main CI 38129641146](https://github.com/qq550723504/data-product-platform/actions/runs/38129641146) 为 SUCCESS，只证明基线。当前候选未推送，不能继承该 CI。#84 OPEN，无关联 open PR；#47/#86/#87 OPEN。唯一 writer：本聊天，独立 `codex/issue84-external-core` / `issue84-external-core` worktree。#307 在独立 worktree 使用 000054；本次 000055 不继承其未合并代码。#294/Draft #295 及其他运行环境未修改。

复用 pgx/transaction、command_idempotency、可信 principal.Resolver、Audit/Evidence/Outbox/routing。绑定表示关联事实，没有新 IAM、密码、Token、OIDC、queue 或 provider runtime。官方 OIDC/Keycloak 复用评估及不新增部署的理由见权威 contract。共享代码只有 API bootstrap 与六个 retention-only routes；routing version `c1-v19+external-core1` 在未来整合时必须按共同 main 重新确认。

## Results

| Check | Result | Evidence / limit |
|---|---|---|
| Domain contract | PASS | 3 domain tests：只有 DRAFT 可 request；未来 provider 状态不可达；未知 object/provider、冲突内部引用拒绝；fingerprint 全字段变化 |
| Identity/identifier history | PASS | 真实 PG：4 类 typed identifiers、immutable UPDATE/DELETE 拒绝、INVALIDATED append-only、独立 replacement；历史外部 ID 禁止重新绑定 |
| PUBLISHED eligibility / multi-provider | PASS | 非 PUBLISHED release 拒绝；同 release 两 provider 分别创建；同 provider/release 不重复创建；foreign release/binding/evidence 拒绝 |
| Replay / concurrent claims | PASS | same key 8 个并发返回同回执；changed fingerprint 409；8 个不同 key 自然键竞争仅 1 成功/7 conflict，losers 没有遗留 Evidence；replay fresh principal/capability 拒绝变更身份或失权 caller |
| Identity invalidation vs submit | PASS | 两连接真实锁屏障 + pg_stat_activity 等待证据；invalidation-first 保持 DRAFT 并拒绝晚到 submit；submit-first 先保存 SUBMITTING，再追加失效，不丢历史 |
| Projection/history / transaction | PASS | DRAFT 创建与 DRAFT→SUBMITTING 各有确切 transition/Evidence/Event/Audit；same key 不重复；Outbox 故障全部回滚并可同 key 重试；没有 transition 的 projection commit、fake PUBLISHED、release reference mutation、transition deletion 拒绝 |
| Core historical reference | PASS | 非空 synthetic release 的整个 header、dataset membership、ProductVersion、asset membership 组合摘要在成功/重复/失败路径前后一致 |
| HTTP / read model | PASS | httptest 经真实 Repository：匿名 401、foreign workspace/缺少 capability 403、任意 subject body 400、X-Actor-ID 不改变可信 actor、bounded query、missing 404、generic PATCH 405、未配置 resolver 503、publication ordered history 可读 |
| Migration / concurrent down | PASS | 独立空库完整 through 000055 → down 55 → up 55；ACCESS EXCLUSIVE down 实际等待未提交 history writer，writer commit 后 down 拒绝；记录仍存在。测试固定到 55，未来 migration 不会替代此 guard 验证 |
| Full Go test / vet / build | PASS | `go test ./... -count=1`（真实 PG）、`go vet ./...`；API/worker/migrate/outbox-replay 构建，routing 与 diff checks |
| Race | PASS | `CGO_ENABLED=1 go test -race ./internal/externalintegration/... -count=1`，同真实 PG |
| Independent review / candidate CI / merge-main CI | NOT_RUN | 未推送、未建 PR、未合并；作者 closure audit 不是独立 reviewer 批准 |
| Real provider / browser / production | NOT_RUN | 没有 remote registration/publication、provider credential、真实数据或生产部署；HTTP tests 不等于浏览器/业务验收 |

可达 publication 仅 `DRAFT → SUBMITTING`。SUBMITTING 是持久 Core 请求，dispatch=NOT_STARTED；REGISTERED/PUBLISHED/SUSPENDED/WITHDRAWN 等待 #86 的可信 execution facts。身份只绑定当前可信 principal 的内部 UUID，组织/connector 注册表未在 main 落地，本次拒绝未知 owner 类型。外部 ID/basis 为 caller asserted，不能当成 provider verification 或授予 delivery 权利。

## Reproduction and evidence

专用 Docker container `dpp-issue84-pg-20261011`，PostgreSQL 16.15，image ID 与 Go/源码 SHA256 记录在 [verification manifest](issue84-local-verification.json)。独立数据库 `issue84_final`；每个 testcase 随机 synthetic workspace。迁移 scratch database 由测试创建/删除，与保留业务环境分离。最初 fixture 的字段/路径及 Audit actor enum 失败已修正为现有 schema / USER contract；最终当前源码重跑 PASS。未以改弱 DB guards 消除失败。

在 `apps/platform` 设置指向专用测试数据库的 `TEST_POSTGRES_DSN`；新库先以 `POSTGRES_DSN` 运行 `go run ./cmd/migrate -dir ../../migrations`，然后执行 manifest 中的 checks。只有配置该 DSN 才实际运行 PostgreSQL tests；缺失时是 SKIP，不能声称持久化/并发 PASS。全量 package PASS 不代表所有需外部 Redis/对象存储/engine 配置的 runtime test 都已实跑。

原始本地输出保留在 worktree 的 `apps/platform/tmp/issue84/go-test.log`、`race.log`、`vet.log`（Git ignored），log SHA256 位于 manifest。源码摘要绑定实际验证过的 Go/SQL 文件，报告/文档不加入业务 fingerprint。命令回执只返回 Core object/fact/Evidence UUID，没有 credential secret。

## Closure checkpoint

作者依照 AGENTS §17 审阅 source-of-truth、可达状态、历史冻结、事务、并发/幂等、可信 caller/workspace 和 Issue ownership；本次声明的必需测试全部 PASS，作者未发现尚未修复的 blocker。没有独立评审批准或远端 gate 结论。

后续明确保持 #86 provider execution/attempt/cost/reconciliation、#87 usage/runtime ingestion、组织/connector owner 接入与生产 IAM。#86 接手时要重新判断 current binding、可信 principal、release/rights 和发布渠道策略；不能把旧 SUBMITTING/Evidence 当作外部成功或 delivery authorization。pending intent 要显式 durable claim，不依赖已经完成的 retention-only outbox route。推送、PR、合并、部署、关闭 Issue 需要各自明确授权。
