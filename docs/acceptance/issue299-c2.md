# #299/C2：固定用电表的原生 schema 观察验收

Goal: one PostgreSQL energy-table schema baseline, real native discovery,
drift explanation, disconnect and recovery in the existing source detail.
Authoritative scope: #299 and the approved C2 instruction. C1 facts remain owned
by the existing remediation/Execution/Quality/Certification commands.

Reuse: existing OpenMetadata Adapter/metadata Port, native PostgreSQL ingestion
package matching server 2.0.5, transactional outbox/worker, PostgreSQL locks,
CurrentEntitlementGate, Audit/Evidence and typed CostAllocation. No connector,
second scheduler, catalog, generic notification or ITSM implementation is added.
An isolated CLI launcher replaces the need for an additional Airflow scheduler.
The package's CCLA is recorded honestly; this trial is not commercial licensing
or production deployment approval. See the official PostgreSQL connector guide.

Invariants: source discovery is distinct from catalog health/revision and frozen
DatasetVersion QA. Request/physical attempt/observation/handling history is
append-only; same request and successful outbox replay do no new work; actual
retry invocations have independent cost identities. Current source entitlement
is checked before scanning and before accepting the observation. Manual handling
never changes QA, certification, release, or delivery gates. The fixed ingestion
schema contract is not widened automatically. Unknown downstream impact stays
UNKNOWN. Source credentials remain private to the adapter.

Coordination: #307 owns unpublished migrations 60–64 and its checkout. C2 uses
independent metadata files while migration and shared composition/routing order
is coordinated. Existing environments and volumes are retained.

2026-10-11/12，基于 main `1892cd76c4ee578896ddfb3c6f4a7252bfacc51b` 的隔离环境，技术验收 PASS。
人工解释按用户要求 **NOT_RUN / 暂缓**。提交 Draft；不关闭 #299、不自动合并。

## 实际证据

[runtime-verification.json](evidence/issue299-c2/runtime-verification.json) 保存精确 IDs、时间、列、
typed 成本、历史 count/hash、运行二进制 hash 与 D1 migration 文件 hash。来源是
`dpp_source.dpp_ingestion_source.dpp_source.energy`，规则为行业包
`park/source-observation/energy-schema-v1.json` / `1.0.0`，五个 TEXT / NOT_NULL 列。

| 检查 | 实际结果 |
| --- | --- |
| 原生发现 | 未修改的 MetadataWorkflow 读取真实 PostgreSQL，五列 MATCH；不是手工创建目录表或伪造状态 |
| schema 漂移 | 合成来源真实 ADD COLUMN `schema_probe text`，原生扫描 DRIFT；Edge 显示差异、责任人、规则版本/hash、时间/范围和处理理由 |
| 处置边界 | 浏览器记录 INVESTIGATING；仅来源侧在 probe 全 NULL 时恢复批准的五列契约；未改旧 RAW/质量/认证/发布 |
| 抽取 contract | 漂移时既有 Hop Prepare 返回 AUTHORITY_DENIED，无新增 Operation/RAW；固定 contract 未自动扩大 |
| 断连、恢复 | 本切片 OM stop/start；浏览器 STALE/UNKNOWN，历史不冒充当前健康；同一请求 4 次 UNAVAILABLE → MATCH |
| 重试成本 | 上述请求 5 次实际 native invocation，各有独立 CostEvent/typed allocation；不是按顶层 request 去重后吞掉重试 |
| 崩溃恢复 | attempt/cost 已落库且 scanner 正在运行时终止本切片 Worker；恢复先 containment，再追加 INTERRUPTED，同请求后来 MATCH；旧 start/cost 保留 |
| 幂等 | 8 个并发 same-key请求及 8 个 handling 重放，各保留一份事实；改变同-key reason 拒绝；零新增事实/成本 |
| Worker 并发 | 两个升级 Worker 各处理一个不同请求，同来源扫描区间不重叠、均 MATCH、分别计费；最终组合复验仍串行，复验对由同一 secondary Worker 调度，未冒充跨 Worker 争锁证据 |
| 隔离 | dpp_reader 可 SELECT，不能 INSERT/UPDATE/DELETE/CREATE schema；scanner 只挂载自己的只读 YAML，未挂载 Core/Hop/operator 凭证；5 个密钥未出现在 Core/cost/audit/evidence/outbox |
| 撤权扫描 | 先接纳请求、暂停本切片 Worker、走既有 RevokeAuthorization、恢复 Worker：DENIED 且 0 physical attempt；新请求、旧成功 key/handling 重放均 403，零新事实/成本 |
| 用户闭环 | Edge 从来源/批次进入实体审核、既有加工、质量/Compliance、独立认证和下载；5 行/640 B 实际下载 hash 与冻结版本一致；负数 RAW 仍 REJECTED |
| 撤权交付 | 浏览器先加载 ALLOWED，撤权后下载产生 BLOCKED / CURRENT_ENTITLEMENT_BLOCKED；同-key页面恢复展示 BLOCKED；两个旧 ISSUED 和旧 CERTIFIED 保留 |
| 历史保护 | 17 个非空历史表精确 membership/content hash 不变，包含 2 个真实 PUBLISHED release；projection 最终 PUBLISHED，故障发布实际重试 7 次 |
| PostgreSQL | 并发 request/cost、跨 workspace/actor 拒绝、immutable guards、primary/外部 ID 绑定替换拒绝、populated down fail closed 均 PASS |

分阶段 artifact hash 如实保留：DRIFT、断连/恢复、跨 Worker 争锁发生在前一组合。
最后 Go 改动只增加当前 primary binding 在 RUN 前和接收结果前仍匹配的要求；
最终组合实际跑了新 native MATCH、双 Worker 串行扫描、撤权排队拒绝、浏览器下载/阻断，
并用真实 PostgreSQL 验证绑定替换拒绝；未把旧组合的 hash 标为最终 hash。
版本校验错误、失联、token 到期和 Worker 崩溃的历史均保留。凭证刷新仅轮换同一隔离 OM 账户既有访问。

![原生 schema 漂移与处理](evidence/issue299-c2/drift.png)

![撤权后的扫描拒绝](evidence/issue299-c2/revoked-schema.png)

## 复用、迁移与部署边界

既有目录 GetTable 只说明目录响应，不能证明刚扫描来源，因此通过现有 Adapter 的 SourceSchemaObserver
Port 调用原生包，要求本轮原生迭代器恰好输出一次精确表、全部步骤无 failure，
再核对 catalog 身份及列与本轮记录一致。成熟方案是
[OpenMetadata 原生 PostgreSQL Connector](https://docs.open-metadata.org/v2.0.x/connectors/database/postgres/yaml)；
未自建 Connector、第二套 Airflow/Temporal、告警或 ITSM。部署成本为已有 OM + 有时限的临时 scanner。
安装分发版本实际 **2.0.5.0**，按 PEP 440 规范化等价校验 contract 2.0.5。
分发包 license metadata 是 **Collate Community License Agreement 1.0**，不能写成 Apache；
本次本地技术接入不声明生产/商业 license admission 已通过。

#307 writer 确认 D1 60–64 与 C2 65/注册文件的写域；C2 未修改 D1 checkout。
自己的独立 PostgreSQL 按 D1 文件快照 60–64 → C2/65 真迁移，
[postgres-after-d1.log](evidence/issue299-c2/postgres-after-d1.log) 与 JSON 内 10 个文件 hash 可复核。
empty C2 down/up 保留 D1 cost subjects；populated down 在排他锁内拒绝并保持事实。
**Ready 前须按实际 D1 → C2 合入顺序 rebase、核对迁移号/共享注册、重跑组合迁移。**
不能先在 main 安装 65，再指望 runner 补跑较小的 D1 60–64。

真实运行发现原 routing_version + governance 超过 DB varchar(64)，已改短版本并在 NewRouter 校验。
使用专有 frozen handler `source-schema-observation`，避免旧 metadata handler 忽略新 event 却 ACK。
**启用 C2 profile/API 前须升级所有消费该 Outbox 的 Worker**；旧 Worker 缺少 handler 时明确失败。
本切片原 keeper Worker 已暂停以保留 provider ownership，仅新 Worker 消费 C2。
停止试用给最新 recompose output 写 `STOP` 文件，只关闭本组合 API/web/worker，保留原 runtime 容器/volume。
恢复旧 Worker 前先停止新组合并确认无 C2 待处理事件，不让旧 Worker 消耗新事件重试预算。
OM stop/start、scanner `--rm`/containment 只作用于本切片具名对象。Compose start 同时触发其
depends_on 的保留 execute-migrate-all，一次性服务退出 0，时间/trigger 见 JSON；未 down -v 或清理其他任务环境。

## 复跑与校验

复用 examples/enterprise-activity/postgres-ingestion 的 B3 fixture/runtime。
`Dockerfile.metadata-ingestion` 安装匹配原生包；`c2_runtime.py` 建自己的 source/Hop/Core；
`c2_connector_config.py --runtime ... --metadata ...` 生成私有 native YAML 与冻结 plan。
编译 `.data/pgi-b3-api-c2-final.exe` / `.data/pgi-b3-c2-final-worker.exe` 后用 `c2_recompose.py`，首次不加 reuse-build。
Edge 使用输出的 loopback URL，成功必须真实产生后再用 `c2_verify.py` snapshot/stress/concurrent 复核。
outage 用本项目 server stop/start；恢复等待既有 Outbox backoff，dead letter 只用既有 outbox-replay Command，
不直接 UPDATE queue。撤权最后执行并保留撤权状态。

go test ./...、go vet ./...、metadata/Outbox/routing race、web typecheck/production build、
真实 PostgreSQL integration、Edge 均 PASS。普通 go test 的 DSN 未配置 integration SKIP 与单独真实 DB PASS 分开记录。
人工解释、生产部署、全面血缘、实时新鲜度 SLA、自动 schema 接纳、通用告警/审批及商业 license admission 未声明完成。

Non-goals: live-source freshness SLA, full lineage, generic alerting/approval,
multi-operator IAM, production rollout, automatic schema acceptance or repair.

## PR #313 评审修复复验（2026-10-12）

修复两条有效意见：零目标表发现不能把旧 catalog 认作本轮 schema；配置读取故障不能
变成 terminal DENIED。Adapter 只观察原生 CreateTableRequest 并原样传给原生 sink，
不实现 PostgreSQL 发现逻辑。已加载但 disabled、语义无效或与冻结 plan 不符仍拒绝；
文件/解析/基础设施读取失败返回 retryable error，不 ACK 成功或永久拒绝。

[review-native-target.json](evidence/issue299-c2/review-native-target.json) 记录本次修复文件 hash：
新建独立空合成来源，真实原生 Connector 健康扫描五列；将其表改名后，即使
markDeletedTables=false 留下同一 catalog 实体，Adapter 退出 1 且不输出结果；
恢复原表名后返回原实体 ID/五列。另有原生 SDK contract tests 覆盖空/错表、sink failure
及 source/catalog 列不一致。该证据只证明 Adapter contract；未重新授权原已撤权来源，
未把它当作新的浏览器或 Core 权利验收。

[review-d1-combination.json](evidence/issue299-c2/review-d1-combination.json) 与
[review-postgres-race.log](evidence/issue299-c2/review-postgres-race.log) 对应 D1 已提交
`2cb3c96dc6cb842cd7c57174ee35fc7107d31a30` 的精确 Git blobs。在自己的新库真安装
main 1–59 → D1 60–64 → C2 65，empty down/up 保留 D1 cost guard，populated down
拒绝且七表 count/hash 不变。真实 PostgreSQL 回归证明同一请求在 profile 读取故障时
无 terminal observation/扫描，恢复后 MATCH、一次扫描/成本，成功重放无新增；已加载
disabled plan 仍 DENIED。此回归使用受控 Observer/entitlement Port，不代表外部 provider
或真实权利已获验收。go test ./...、go vet ./...、上述 PostgreSQL race 与 native SDK 四项
回归均 PASS；普通 Go suite 的 integration SKIP 仍与专门真实 PostgreSQL PASS 分开。

#307 writer 已确认 D1 → C2，不占用 C2 路由/Worker 写域。本次组合证明只绑定上面的
已提交快照；#307 正在补 processing/delivery pin，并计划更新未发布的 64。若最终
D1 新增独立迁移达到 65 或更高，C2 writer 将把自己的迁移排到其最终最大号之后。
Ready 前必须按最终 D1/main 快照 rebase 并重跑组合，不能以本次旧快照替代最终门禁。
本轮保持 Draft、不自动合并；人工解释继续 NOT_RUN。

复评另两条有效意见已补：最终 exact binding 检查持 tuple FOR UPDATE 直到观察提交，
binding repository 按 resource → binding 锁顺序更新；真实 PostgreSQL 观察先行时
refresh/demotion 和 direct identity UPDATE 均等待，mutation 先行时追加 DENIED，旧 MATCH
历史保留。扫描配置或 OM 被禁用后，HTTP 仍查询历史，仅 enabled=false；真实 PostgreSQL
HTTP 回归保留 request/DRIFT/handling，并拒绝匿名及 foreign workspace。
[review-binding-history.json](evidence/issue299-c2/review-binding-history.json) 和
[review-binding-history-postgres.log](evidence/issue299-c2/review-binding-history-postgres.log) 保存修复文件 hash/日志。

本轮 CI 的 live-core 在既有 T3/B2 down guard 争锁测试发生 SQLSTATE 40P01；该测试原先
对共享 DSN 取多表排他锁，会与其他并行 package 互相阻塞。现复用已有 scratchDatabase(t,18)，
保留真实 business/down 并发断言，-race 连跑 5 次 PASS，生产 down/runner 不变。
独立真实 B2 复跑 PASS；本机默认 Docker 地址池耗尽，wrapper 仅为本次两个新临时网络
分配经 inventory 确认不重叠的 10.233.23.0/24、10.233.24.0/24，cleanup PASS。旧 CI B2 的实际断言日志
未上传，不能据此声称已证明环境偶发；harness 已在密钥检查通过后输出有界失败明细。

**D1 `2cb3c96d` 全组合回归 FAIL：** 在新库以 TEST_POSTGRES_DSN 执行全模块测试，
metadata/迁移及其他 packages PASS，但 externalintegration/integration 失败。D1/60 重建
CostAllocation single-subject constraint 和 workspace guard 时漏掉 main/57、58 的
external_provider_call_id / external_usage_call_id，导致既有调用成本不能落库。已交 #307
writer 修正其未发布 60 up/down；C2 不在 65 中补漏以掩盖依赖缺陷。本 PR 自身的
main + C2/65 新库全模块 PostgreSQL 回归 PASS。前文 D1 的安装、source-schema 和 empty/
populated down PASS 只证明对应局部检查，不能代替全组合回归；Ready 仍须等待 D1 稳定
修复快照后重跑。原 Core 撤权事实及人工解释 NOT_RUN 均保留。

最后一条复评补齐 frozen connector contract：Application 接收结果时还要求
result.ConnectorHash == request.Plan.ConnectorHash，不能只相信 Adapter 自己加载的配置。
真实 PostgreSQL + 受控 Port 模拟两次 gate 均见 A、结果却来自 B 或缺少 hash，均保留
UNAVAILABLE/调用成本；恢复 A 后同请求 MATCH，共两次调用/两份 typed 成本，成功重放
无新增。metadata -race、go test ./...、go vet ./... PASS。证据与实际文件 hash 见
[review-frozen-connector.json](evidence/issue299-c2/review-frozen-connector.json)。

成功 Scan 的 Port contract 明确为原生发现完成且进程已退出（现有 Adapter 已用成功
cmd.Wait 证明）；Application 不再执行第二次 Stop，避免读取配置失败丢弃有效结果。
失败/未知扫描及 crash recovery 仍必须确认 Stop。真实 PostgreSQL/受控 Port 回归证明
成功时 cleanup profile 不可读仍 MATCH、一次 attempt/成本、重放无新增；未知完成且
containment 失败时不接受结果，确认停止后保留 INTERRUPTED 再复扫，成本逐次保留。
metadata -race、go test ./...、go vet ./... PASS，精确 hash/日志见
[review-completed-scan.json](evidence/issue299-c2/review-completed-scan.json)。
