# #325 第一阶段 — 固定 PostgreSQL 三 RAW 正常试用

2026-10-19，Windows 宿主 + 本机 Docker Linux、单操作人、仅 loopback、明确合成数据。正常入口和交接步骤见 [deploy/trial/README.md](../../deploy/trial/README.md)，开工复用与写域见 [checkpoint](issue325-stage1-checkpoint.md)。本报告只覆盖 PR 1，不表示完整 B/C、restart/resume、生产准入或人工解释验收。

## 候选与实际运行

基线 main `d4015cb8962ca84b91622434ff9a512d64542857`。本次完整实跑代码 HEAD `1687a6ef4bd5bd998696f95255a9e3885607a416`；至 `744cfdbce195f48cc0c6abc24f061069aa2c89ef` 的交付改动仅为本报告/checkpoint 与非 secret 证据，两者运行代码指纹相同。后续独立评审修正了停止路径的健康依赖，验证范围见下节；完整浏览器实跑仍绑定上述代码，不能把新停止分支描述为已在该环境故障注入实跑。Next 自动生成的 `next-env.d.ts` 不是运行源码；运行的是实际 standalone `server.js` 和已记录的完整构建文件，不依赖可变的开发服务器。

- Run：`1b7e1949e8224ad8b870d28b96a3c6c2`。
- Workspace：`e7469d12-05bc-4427-b79a-356a49cfd13c`。
- Operation：`31a4bee4-e24f-405e-b53d-53c7807ac797`；Attempt：`600d0205-4372-48d0-a884-5e5ffed1fdb1`。
- 本次浏览器地址 `http://127.0.0.1:60087/batches`，停止后不能当作仍在线的服务。
- 具体 image refs/IDs、MinIO 官方源码/实际 binary hash、API/Worker/prerequisites/Node SHA256、Go build info、production web build identity、容器/卷/network/端口和 secret **引用**见 [identity.json](evidence/issue325-stage1/identity.json)。本次 Go build info 未携带 VCS stamp，不由它推断 clean/source revision；源码 HEAD + 逐文件运行代码指纹与实际 binary SHA256 联合绑定本次制品。构建时待提交项仅为验收文档/截图。
- 本机 registry：worktree 的 `.data/trial/state.json`；真实 private config/secret 保留在 registry 指向的用户私有目录，未提交 secret 或环境转储。

## 本次结果

| 检查 | 结果及证明边界 |
|---|---|
| preflight / 正常 start | PASS：固定 PG16、Hop 2.19.0/JDBC 42.7.14、原生 MinIO + 既有 Caddy mitigation、Redis、当前源码编译的 API/Worker、生产 standalone Web。正常启动没有导入 Trial/FaultProxy/browser fixture |
| 仅准备前置资源 | PASS：[initial-facts](evidence/issue325-stage1/initial-facts.json) 为 3 Resource、3 空 RAW Dataset、6 Audit；batch/version/workflow/execution/quality/certification/delivery 均为 0。没有自动抽取、审核或加工 |
| 浏览器实际三 RAW 接纳 | PASS：Edge 真实登记 → 一次 dispatch → 核对同一 Hop remote run → accept；SUCCEEDED 与 ACCEPTED 分开。见 [接纳](evidence/issue325-stage1/raw-accepted.jpg)、[三 RAW](evidence/issue325-stage1/raw-rows.jpg) |
| schema / 内容 / manifest / 关系 | PASS：独立只读 oracle 通过原生 SDK GET 三对象，重算完整 bytes SHA256、CSV header/逐值与冻结 fixture 比对，重算 manifest canonical hash；三个 RAW 均为 v1 / READY，current version 与 receipt/dataset 关系相符。见 [oracle](evidence/issue325-stage1/native-oracle.json)、[关系](evidence/issue325-stage1/relationship-proof.json)、[源行数](evidence/issue325-stage1/source-proof.json) |
| RAW 版本只读页面 | PASS：从实际批次点击 enterprise RAW 版本，显示 READY / 6 行 / 完整 checksum，未配置认证凭据也能读取事实；不启用认证操作。见 [版本页](evidence/issue325-stage1/raw-version.jpg) |
| 重复 start / status | PASS：[repeat-proof](evidence/issue325-stage1/repeat-proof.json) 比较完整 operation/receipt/version/binding/对象与数量，保持同一 run/workspace、1 batch、3 versions、23 Audit、15 Evidence、6 Cost；未产生新资源或业务事实 |
| actual physical invocation cost | PASS：[cost-proof](evidence/issue325-stage1/cost-proof.json) 保存 invocation → 强类型 CostAllocation/CostEvent 的真实 SQL join 统计，数量合计为 3 invocation；6 个 allocation rows 包含 intent 与 actual facts，不能当作 6 次实际调用。UNKNOWN intent 保留，不翻写历史或推断零成本；不把 CLI status 的只读环境检查当成新业务 attempt |
| secret isolation | PASS：[secret-proof](evidence/issue325-stage1/secret-proof.json) 扫描 2224 个表面、254313065 bytes：实际 binaries、生产 standalone、HTML、运行/容器日志、公开回执、Core Audit/Evidence/Outbox，已知随机密钥及 Hop Basic 编码匹配为 0。范围是本次已知凭据，不声称一般 DLP 证明 |
| stop / 停止后 status / 数据保留 | PASS：[retention-proof](evidence/issue325-stage1/retention-proof.json) 验证同一 6 容器停止、6 named volumes 保留；用 Docker cp 从停止容器读取 PG16 数据文件及非空原生 RAW 文件，逐文件内容 hash 与停止前一致。停止后 status 读取同一 identity；不声称已完成 restart/SQL 恢复验收 |
| 本地开发检查 | PASS：实跑候选 11 项 Windows ownership/lifecycle 回归；评审停止修复后 14 项回归及真实本机 preflight PASS。`go test ./cmd/trial-prepare ./internal/ingestion/...`、trial command vet/build；web lint/typecheck、生产 build 及其既有 command tests；diff check |
| PR CI / 独立 review | 单列于 PR #326 当前 HEAD 的 checks / review。历史 CI 或本次本机 PASS 不替代当前 HEAD CI；独立 review 尚未完成，不自动 Ready/merge |
| OpenMetadata / PR 2 / 人工解释 | NOT_ENABLED / NOT_RUN / deferred・NOT_RUN。没有执行认证、交付、撤权负例、D1、真实 Provider DELETE、hold-release 或 cleanup |

本次 manifest hash：`99106c442732f8aec9ce270e971c4774a125c628a4e28883ff75ae47c8001c55`。

| RAW | 行数 | bytes | 实际 SHA256 |
|---|---:|---:|---|
| enterprise_raw | 6 | 1277 | `4f6f887c4b4743a2268f9b74dae5eb41d154bddd601b138e489c3fd6f493249d` |
| lease_raw | 10 | 1323 | `f73bc47190f8e0b8cd18ec99e2afd0265d980cd55a5a9cbc25d3b3ccf85a9d4a` |
| energy_raw | 14 | 1318 | `c831d1ad67c050c367286e56dff27f2ffc84571f2abca6fe9f0bac3d360b0686` |

## 诊断、修复和保留

独立评审在 `744cfdb` 复现一个停止路径 blocker：RUNNING 时 Core/对象服务不可读，或构建工具/fixture 不可用，会在确切归属检查或真正停止前退出。根因是将启动/业务健康前置条件用于 containment stop。修复把本机 Docker identity 从完整 preflight 中分离；stop 继续核验 daemon、容器 ID/image/label 和 Windows process creation identity，事实读取失败则追加独立 UNKNOWN observation、保留先前成功回执并继续精确停止。没有放宽归属校验。

回归先在旧实现复现 4 个失败分支（业务读取的 Refused/OSError/ValueError 与构建前置失败），修复后全部 14 项测试 PASS，包含 daemon 漂移拒绝、UNKNOWN 不覆盖上次成功回执和不触发删除。新的 preflight 对本机实际 daemon PASS；测试不对保留环境注入故障或执行容器变更。原实跑 registry 的 source identity 不改写，当前源码变化时 status 仍按原 contract 拒绝漂移；PR 2 必须设计显式制品重绑定与同身份续用，不能把重新 seed 当作恢复。

没有把开发失败涂改成成功，也没有用重新 seed 冒充续用：

| 先前尝试 | 实际结果与保留 |
|---|---|
| `8accb54ce9854fe5a1f082af108c7d2b` | Docker Hub TLS 超时；UNKNOWN，尚无容器/卷/network。诊断后停止并保留 registry/private，不借旧镜像或旧 Core 报 PASS |
| `9f1ee82a2cfb41d883b8489e1d82fb40` | 自动生成 Next 类型声明被错误归入运行源码，startup 拒绝；根因修正为区分生成类型与实际 standalone 制品。Docker 运行时默认值也不再误判为声明配置漂移，精确 owner 的异常配置容器仍可安全停止。前置数据、六个停止容器与 named volumes 保留 |
| `db8724868d82478a8ce5725f3dfeffcf` / `c55842dd` | 三 RAW、repeat、secret scan 与 stop/物理保留 PASS，但 RAW 详情因认证 Profile 凭据耦合 FAIL。已修复 RAW-only 页面的认证配置/表单 gate；该旧运行保留自己的三份历史、registry、private 与停止资源，其 PASS 不代替上述最终实跑 |

这些 registry 分别归档在本 worktree `.data/trial-build-failed-<run>`、`.data/trial-startup-drift-<run>`、`.data/trial-ui-red-<run>`；移动前校验了绝对工作区路径、确切 identity 和停止状态，未移走任何数据卷或外部 private directory。旧 B/C、#307、#294、Label Studio 与其他仓库运行环境没有作为测试目标。

## 复核入口与交接

正常操作只用 [trial CLI 文档](../../deploy/trial/README.md)。验收用的 [native oracle source](evidence/issue325-stage1/native-oracle.go) 与 [只读本机检查](evidence/issue325-stage1/verify-local.py) 是复核工具，不是普通操作者动作，更不能替代 `/batches` 的 Commands。复核 native oracle 时将源码复制到 `apps/platform/.data/trial-oracle/main.go`，从 platform module 编译，然后传入确切 registry 与 `latest-facts.json`。Python 检查复制到 worktree `.data/trial/verify_local.py`，仅对当前确切 registry 执行；不要拿不同运行的证明文件交叉拼接。

保留意味着数据与身份仍在，不等于恢复验证。PR 2 必须基于上述同一最终运行设计显式 restart/resume，再验证当前源码的完整 B/C 和真实交付路径；本 PR 的新运行或 #324 的历史定向回归都不能替代那项工作。#296/#298/#299/#307 不由本报告关闭。
