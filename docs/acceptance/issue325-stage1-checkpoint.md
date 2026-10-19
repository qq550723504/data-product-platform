# #325 PR 1 — 装配、复用与写域 checkpoint

2026-10-19；owner：本 chat `01a151e4-2929-7683-a6a5-9e665ad06a4f`；分支 `codex/issue325-trial-stage1`；独立 worktree `issue325-trial-stage1/data-product-platform`。开工 fetch 后 main = `d4015cb8962ca84b91622434ff9a512d64542857`。当前独立 writer #307 / PR #323 在 `9400`；#294 / PR #295 保持原 owner。没有向其他 chat 重复派工。

## 有限 merge contract

Goal：Windows 宿主 + 本机 Docker Linux，单操作人、loopback、合成 PostgreSQL 固定 6/10/14 行；`preflight → start → /batches 登记/dispatch/accept → status → stop`，同一运行身份和真实持久数据保留。首次 start 仅准备来源、三个空 RAW Dataset 容器和可信 operator profile；不创建 batch、DatasetVersion、审核、加工、认证或交付。

权威来源：[#325](https://github.com/qq550723504/data-product-platform/issues/325) 定义本 PR；`docs/architecture/postgres-ingestion-v1.md` 定义 ingestion；`examples/enterprise-activity/postgres-ingestion/CLOSURE-RUN.md` 与 `docs/acceptance/issue299-c{1,2,3}.md` 仅提供历史范围。现有 Core Commands、immutable facts、真实 queue/worker、native IAM/conditional writes 和 storage gateway 继续拥有其语义。历史 PASS 不转成本候选 PASS。

## Build-vs-Buy / Reuse Check

| 现有能力 / 标准方案 | 决定与原因 |
|---|---|
| B3 Windows host + PostgreSQL 16 / Hop 2.19.0 / JDBC 42.7.14、固定 source.sql / CSV / extract.hpl / metadata | 复用制品与 contract；正常装配不 import `smoke.py`、FaultProxy、Trial、browser fixture 或 failure controls |
| B3 `b3_runtime.py` / `ingestion-browser-fixture` | 不能直接作为产品启动器：耦合 fault proxy、测试命令、rights/release 操作；新增只做 prerequisites 的窄 command，调用现有 Resource/Dataset Commands，不复制业务模型 |
| Docker Compose v2（成熟 OSS / Compose Specification） | 用原生 Compose 组装容器、network、named volumes；CLI 只保存本次 identity、校验实际 inspect、管理 Windows host binaries，避免自建容器调度器 |
| 既有 MinIO native SDK / IAM、官方 pinned source build、Caddy mitigation、D1 read-only durable mount checker | 复用；不新增 storage SDK、credentials service、queue 或 recovery runtime。MinIO 为现有 AGPL-3.0 reference，Hop 为现有 Apache-2.0；不引入新 OSS 或 fork，不声称 historical pin 是生产安全认证 |
| 现有 CSV demo Compose / `demo.mjs` | 保留 CSV 的原声明；它的公开 synthetic secrets 与 CSV seed 不适合 PG 试用，新增独立 `deploy/trial/`，不扩写旧 demo 的完成范围 |
| Temporal / Asynq / 新 ETL、IAM、通用 supervisor | 不采用：现有 queue/worker、Hop、static trusted profile、Compose 足够；新增组件没有本阶段必要 invariant |

## 写域与保护

允许：`deploy/trial/**`、新增 `apps/platform/cmd/trial-prepare/**`、本 checkpoint、试用操作/验收文档；必要的 `/batches` 阶段展示及其定向检查。既有 fixture/SQL/模板/metadata、业务 Commands 与 D1 checker 只读复用。

禁止并发改动：Core/internal shared modules、migrations、routing、worker main、存储实现、#307/#294 文件或其运行环境。若这些成为必需写域，先报告具体冲突。仅为本 trial 新建随机 identity、private secrets、containers/network/volumes/ports、本 worktree 的 binaries / production Next build。`stop` 只停止已核验 identity 的确切服务，保留容器、卷、对象、staging、配置、secret refs 和证据；没有 reset/DELETE/prune/down 操作。

## 验证和非目标

必需：CLI 的 same-start/no-reseed、drift/未知 fail-closed、单 writer、外来 identity 拒绝、精确停止与不泄密测试；真实新运行零 batch/version baseline；浏览器三 RAW receipt/schema/hash/版本关系；重复 start/status 无重复事实；stop 后真实数据/对象/卷保留。记录确切 source、binary SHA、image ID/digest、workspace、ports、secret refs 与运行回执。未实跑记 NOT_RUN。

PR 2 同环境续用、B/C 加工/异常修复/复检/认证/真实下载、撤权负例留后续；OpenMetadata 未启用。人工解释 deferred / NOT_RUN，不关闭 #296/#298/#299/#307。仅安全泄漏、历史/成本丢失、contract 冲突、当前必需路径/CI 失败或声明 invariant 的并发/幂等破坏是 blocker。Draft 与当前 HEAD 的检查单列；不自动 Ready、merge、部署。
