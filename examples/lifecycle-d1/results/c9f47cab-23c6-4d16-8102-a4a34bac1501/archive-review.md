# #307 归档/恢复实跑与新增删除确认清单

2026-10-11 20:45 +08。归档是原生 exact-version COPY + 同字节验证 + ARCHIVED fact，源版本保留；不是冷存储介质验收。恢复先 quarantine，经独立当前台账 + 共享 fence 重核后曾 ONLINE；随后追加 current restriction，所有新读取阻断。不存在通用产品下载授权。

环境 `dpp-lifecycle307-c9f47cab`；run `c9f47cab-23c6-4d16-8102-a4a34bac1501`；workspace `5fe971c0-7682-4b6b-b4c6-741883247365`。internal network `dpp-lifecycle307-c9f47cab-private`，仅 PG/MinIO/gateway 三服务，无 host ports。专属 volumes `dpp-lifecycle307-c9f47cab-postgres` / `dpp-lifecycle307-c9f47cab-objects`。独立 witness 在私有 host 目录，位于 Core/provider volumes 与 checkout 之外。详细 container IDs、image IDs、二进制 SHA256、provider/incarnation 和库存见 [manifest](manifest.json) / [acceptance](archive-acceptance.json)。

| Namespace | Exact key | Exact version | Bytes | 当前状态/范围 |
|---|---|---|---:|---|
| dpp-lifecycle307-online | fixture/a.csv | 3fe1cc44-b0f8-428c-9bb8-9f92dac5d15d | 27 | 无保护源，唯一后续删除候选；未授权 |
| dpp-lifecycle307-online | fixture/b.csv | 8228fe6b-e938-4404-8ad2-4e490e875495 | 26 | LegalHold ON，保留 |
| dpp-lifecycle307-quarantine | archive/a.csv | 613c3231-5077-4326-9a7c-4a073a105e47 | 27 | ARCHIVED，保留 |
| dpp-lifecycle307-quarantine | fixture/a.csv | 87bd6a39-f420-4bfa-9619-4812a01e5835 | 27 | GOVERNANCE，保留 |
| dpp-lifecycle307-quarantine | fixture/b.csv | 6e3b6ae2-2b8e-40ee-9103-419322b78718 | 26 | 无保护对照，保留 |
| dpp-lifecycle307-quarantine | restore/gap-a.csv | 2a316e28-3efb-4e94-ab5b-c1e2637b50df | 27 | 隔离 BLOCKED/UNKNOWN，保留 |
| dpp-lifecycle307-quarantine | restore/missing-a.csv | 76b867ec-a365-47fd-b2ab-11e825c7198e | 27 | 隔离 BLOCKED/UNKNOWN，保留 |
| dpp-lifecycle307-quarantine | restore/old-cut-a.csv | 5d85eea5-7451-4a11-a4be-d7b80b39db1d | 27 | 隔离 BLOCKED/UNKNOWN，保留 |
| dpp-lifecycle307-quarantine | restore/online-a.csv | 81f1f926-ccf9-4df2-8f5b-abce76b2c131 | 27 | 历史 ONLINE，现使用已阻断；保留 |

合成 A 全部副本 SHA256 `818392d0732d499a777b5b5a74f1351ddffce3cf8a3b66f8d91dbfd98dfa1523`，DatasetVersion `aa67944f-df64-4ad3-a9a5-4f205ebe8f97` 的 identity/hash/原 storage URI/READY 历史均保留。以 JSON 中实际 SHA256 为准；下方验证记录与清单相互核对。

验收 PASS：1 ARCHIVE COPY + 4 RESTORE COPY；5 exact bytes 验证 + 1 finite fixture reader，reader 已权威终止；6 immutable admission decisions（ARCHIVED / ONLINE / 旧水位 UNKNOWN / 当前禁止 BLOCKED / 缺失 UNKNOWN / 断链 UNKNOWN）；4 外置连续 checkpoint；全部 9 native versions 仍在，无 delete markers。82 physical starts / observations / CostEvent + typed allocations；其中含一次被网关拒绝的只读 LIST，成本未抹除。195 Domain Event / Audit / Evidence；active readers=0、unverified copy intents=0。copy policy/native copy identity 已关闭，object DELETE/PUT/POST/payload GET 已封闭，管理员 API 仍在私网且须独立认证。

本次 native DELETE=0，对象/volumes cleanup=0。原 `dpp-lifecycle307-a6b30840` 及两份保护对象未参与本次对象请求，也未升级/重启/改配置/解除保护。仅核对原三 container IDs 与 volumes 保留。

失败准备/验收也完整保留：`dpp-lifecycle307-a1647373` 仅 bootstrap 四个合成版本，copy policy HTTP400，不曾 COPY；`dpp-lifecycle307-bb44901c` 有一个已 materialized、未验证的 archive target `archive/a.csv@ae706ed1-c77b-45a1-818c-41929529b7e7`，GET403 后保留 pending/quarantine intent，未自动重发或清理。`c9f47cab` 的所有复制/门禁步骤已完成，LIST 路由错误修复后仅通过显式 readback 完成 metadata/inventory 回核，没有重复 COPY、payload GET 或 admission。私有空目录 `aea8433b` 在 image preflight 即中止，无环境/对象创建。

验证：真实 PostgreSQL lifecycle 全套 `go test -race -count=1 ./internal/lifecycle/... ./internal/platform/storage ./internal/platform/routing` PASS；含并发 COPY single-send、读取与复制的两顺序、restore finalize 与 restriction 的两顺序、伪造 intent、错 native version/hash 回执回滚、缺失/断链/跨 workspace/落后台账、历史 UPDATE/TRUNCATE 阻断。module `go test ./...` PASS（未启用全模块 PG integration；上述 lifecycle PG suite 独立实跑）；`go vet ./...` PASS。

来源边界：本次实跑来自未提交 working tree，初始 COPY executable hash 未单独留存；受控 helper/调用/对象和回执可追溯，但不声称这是 final commit 的 exact-HEAD CI。后续 wrapper 已补每个 helper 启动前 immutable binary/source/topology provenance。PR CI/review/merge/main CI 尚未运行；全 D1、生产 DR、真实权限/认证交付、自动恢复 worker、真正冷 tier 仍 NOT_RUN。

如继续执行源删除，确认范围只能是本环境 online 的 `fixture/a.csv@3fe1cc44-b0f8-428c-9bb8-9f92dac5d15d`。这只是拟授权范围，不是已通过 fresh 删除门禁的承诺；确认后仍需新 scoped plan/current retention/reference/occupancy gate 与 exact absence 回执。其余八份、上述失败运行、原两保护版本和所有 volumes 全部保留。**目前删除未授权、未执行。**

源删除提案已通过既有 PrepareDeletePlan Command 冻结，plan `50b71a6c-f837-42e4-a670-c924aebceb22`，scope SHA256 `7ccf841b84d919b3f9634983fe789e74fb82b0cbbf86017fd1ad95a841aed47e`，仅含清单第一份源版本，见 [source proposal](archive-source-delete-plan.json)。创建提案没有 provider 请求；当前 approvals=0、dispatches=0、physical calls 仍82，追加 plan fact 后 Domain Event/Audit/Evidence 各196。未授权、未删除；新窗口再次禁用，native copy user 未重新启用。
