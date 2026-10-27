# Q1 真实 Core 只读联调验收

2026-10-23，本地分支 `codex/agent-quality-readonly`，基线 `f7a90022aad5da4d67ea594ccb86f8b9925a3840` 加本地未提交改动。**10 项真实 API 检查、11 项实际浏览器检查通过；208 张 public 表的行数和内容 SHA-256 前后完全一致。**

本次链路是 `apps/web → 实际 cmd/api → 现有 PostgreSQL Repository → 独立 PostgreSQL 16`，没有 HTTP mock、代理响应或替代 Core。数据是预先写入独立数据库的合成历史事实，不是引擎实际生产/评测的成果。Q2 委托/计划持久化及 Q7 执行全流程仍未实施。没有远端 PR、合并或部署。

## 运行与数据边界

| 对象 | 本次实际配置 |
|---|---|
| 页面 | `http://127.0.0.1:4187/agent`，独立 `.next-agent-q1-live` standalone 构建 |
| 实际 Core | `http://127.0.0.1:18496`，当前隔离分支 API 二进制 |
| 测试依赖 | Compose project `dpp-agent-q1-live-20261023`，PostgreSQL 25493 / Redis 26493 / MinIO 29493；端口均仅绑定 loopback |
| 数据存储 | 独立数据库 `agent_q1_live`；PostgreSQL 和 MinIO 使用 tmpfs，没有业务保留卷挂载 |
| Core 数据库账号 | `agent_q1_reader`，非 superuser；208/208 表可 SELECT，0 表可 INSERT/UPDATE/DELETE/TRUNCATE，不能 CREATE public 对象，默认事务只读 |
| 实际连接 | `pg_stat_activity` 观察到 3 条 `agent_q1_reader` 空闲连接，与 Core 进程配置一致 |
| 测试主体 | 服务端固定合成 token / actor / workspace；不代表多用户 IAM，token 不进入浏览器 |
| 禁止项 | 全部 9 个 `POC_ENABLE_*_ACTIONS=false`；引擎、交付关闭；没有 worker、模型或业务 Command 调用 |

原有 4186/4459 合成 HTTP 预览的进程、构建和数据未改动；4178 原型及其他保留环境也未改动。新增构建复用项目已有 `NEXT_DIST_DIR` / `NEXT_TSCONFIG_PATH` 隔离机制。

测试空间为 `11111111-1111-4111-8111-111111111111`。主数据集 `aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa` 的当前版本为 v2，但验收明确选择 v1 `bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb`。v1 有 26 条评测：最新 PASS，最旧 `cccccccc-cccc-4ccc-8ccc-cccccccccccc` 为 FAIL，有 26 条发现项、1 条 Evidence、1 条 Audit 引用。另有同空间 v2 报告与其他空间的数据，用于错误归属测试。

合成 Evidence 的 `SHA256-EVIDENCE-V2` 使用既有 `evidence.ComputeHash` 计算，值为 `c390e595f3ef3a813c40006c1395e91c4a229d7168b5a4bb3c3252ca66ee4a44`。合成 DatasetVersion 的 URI/hash 只是历史元数据标记，本次没有读取数据内容、调用引擎或补写质量事实。

## 实际验证

`api-readonly.test.mjs` 的 10 项检查全部调用真实 Core GET，无 skip：上下文白名单/模板；缺失和伪造 token 401；其他空间 403；其他空间版本和数据集不匹配 404；历史第二页；指定旧 FAIL 的发现项/证据/审计；已有尝试；同空间其他版本报告身份；报告缺失 404；Quality 的空间权限。

浏览器通过 CUA 操作实际生产构建：

| 编号 | 用例 | 结果 |
|---|---|---|
| L01 | 工作台 → 数据集 → 明确 v1 → 历史第二页 | 仅显示授权空间来源，没有跟随 dataset.current_version_id 切到 v2 |
| L02 | 精确旧报告 | FAIL、指定 assessment ID、证据和审计各 1；修复 FAIL 原来误用中性颜色的问题，实际 DOM 为 `badge-bad` |
| L03 | 发现项第二页 | 仅 `Q1-SYNTHETIC-26`；保留历史页、版本和报告 |
| L04 | 核实已有尝试 | GET 返回 SUCCEEDED 并关联旧报告；两个分页 offset=25 保留 |
| L05 | 历史第一页与刷新 | 最新 PASS 可见，同时所选旧 FAIL 报告保持；无 latest 替代 |
| L06 | 指定报告不存在 | Core 404，只有报告 UNKNOWN；来源和历史保留 |
| L07 | 指定同空间其他版本的报告 | Core 200，前端仍拒绝将该报告接到 v1，不展示其证据 |
| L08 | 指定其他空间版本 | context 404，页面 UNKNOWN；后续 Quality 查询数为 0 |
| L09 | 尝试不存在 | 仅尝试 UNKNOWN，旧报告仍可见，没有重试/新评测 |
| L10 | 浏览器网络与凭据 | 未截断窗口 16 个请求，全部 GET、只访问本机 Next；无 HTTP 错误、loadingFailed 或服务 token |
| L11 | 响应式 | 默认宽度 1265px 和移动视口（布局宽度 375px）均无页面横向溢出；已恢复默认视口 |

Node `diagnostics_channel` 只观察实际 Next 服务端请求，不替换传输。完整记录共 **52 次 Core GET**，49 个 200、3 个预期 404；仅存时间、method、path、status，不存 token、headers 或响应内容。这和浏览器到 Next 的 16 请求窗口是不同观察边界。

前后快照比较全部 208 张 public 表的行数及按内容排序计算的 SHA-256，差异为 0。包括既有合成 Quality/Audit/Evidence；Execution、Cost、outbox、remediation、Delivery 等没有新增记录。数据库快照不等于对象存储或 Redis 零写入证明。

## 启动耦合与基础设施影响

实际 API 在注册只读查询前仍要求对象存储初始化和 Redis 探活，启动会建立空测试 bucket 并入队健康探针；本次两次 Core 启动留下两个未执行的健康探针。它们是独立测试依赖的启动行为，不是 Agent 业务工作；不能将本次验收称为“所有基础设施零写入”。没有 worker 消费队列。

该耦合已在 Q1 实施记录列为架构缺口：若要求只读控制台在队列/对象存储故障时仍可运行，应单独拆分非必要启动副作用和各路由 readiness。本次没有另造一个只读服务来规避现有架构。

Docker 自动子网池耗尽后，测试依赖使用既有默认 bridge；原生 Core 经 loopback 访问它们，不依赖 Compose DNS。没有清理现有 network、container 或 volume，因此不能声称依赖运行在专用 Docker 网络内。

仓库原有 MinIO 镜像拉取遇到 TLS timeout 后，复用现有本地项目构建，固定内容摘要 `sha256:4b343185d3ce9148dbcb2ca720379faa793a140d54d6998b1cfe26f514fbb7fa`。二进制 `--version` 的 commit 为 `9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a`，与 `deploy/trial/runtime.py` 的源码 pin 一致，license 为 AGPLv3；没有新增对象存储实现、SDK 或 Core 依赖。该本地镜像不保证其他机器可直接拉取，复验前应按仓库已有源码 pin 准备。

## 证据与复验

入口和逐步命令见 [`tests/agent-quality-live/README.md`](../../tests/agent-quality-live/README.md)。本地证据位于 `.data/agent-q1-live/`，未纳入 Git：

- `evidence/browser-cases.json`、`browser-network-summary.json`：11 项实际观察与完整网络窗口。
- `evidence/api-readonly-test.log`、`build-web.log`：10/10 API、完整前端回归、编译、类型和 lint；另行 `typecheck` / `lint` 也通过。
- `core-requests.jsonl`、`evidence/readonly-summary.json`：真实服务端读取明细及只读断言结果。
- `evidence/database-before.json` / `database-after.json`：208 表计数和内容摘要；`database-role.json` / `core-database-connections.json`：权限及实际连接。
- `evidence/quality-live-full.jpg`、`quality-live-desktop.jpg`、`quality-live-mobile.jpg`、`missing-report.jpg`：实际截图。
- `processes.json`、`evidence/listeners.json`：仅本次 Core/web 进程身份与 loopback 监听；停止脚本核对 PID、创建时间、进程名和端口。

本次只读验收完成，预览保持运行。模板仍为 `NOT_EVALUATED`，不表示输入/当前授权已核验。模型、真实评测/修复、Q2 及完整 Q7 业务闭环均为 **NOT_RUN**。
