# #307 worker/outbox 恢复与对账受控验收

本轮完成 **DELETE_EXACT 的只读恢复链路**：durable work → frozen outbox obligation → 既有 Asynq → 当前权限 → instrumented native metadata → append-only result。实际 DELETE 为 **0**；实际删除后缺失对象的原生恢复仍为 **NOT_RUN / 待用户确认**。UNKNOWN 的原删除记录和 reservation 没有被释放或改写。#307 继续 OPEN，不能称完整 D1 或生产验收完成。

## 代码与环境

- 实现提交：`62e8f3aa72527b5a0b2736607ba69b68e74ae846`；原生 worker 二进制 SHA256：`0c508dc8780b94a04a65f3a0c15db648c3a92ec3494fb3fbc30613821d10d38f`。
- 后续测试与仅冻结待确认范围的 helper：`a4e95c448bef4c163a66d4ed4b419523d19e2f3e`。这是本地分支；尚未 push/提 PR/获得 exact-HEAD CI/review。
- main 基线：`d1772c085cd9615c2e43c2e74d5346e099d2279a`。新增 migration **63**；routing `c1-v26+external-core1+execution1+usage1+lifecycle-recovery1`。沿用 Asynq/Outbox/advisory lock/既有 native inspector，不引入第二个 runtime。
- 新环境：**dpp-lifecycle307-4abcfca5**；run `4abcfca5-583d-41ca-8548-10a0fc3270a1`。
- workspace `d9deb37f-0e58-4f00-830e-c7aad21895f8`；provider `b48e5dcf-d466-483d-ac89-f810ba976974`；incarnation `57c5084d-d929-4eeb-842d-180734158bb0`。
- internal network `2e5ddaa5edbbe88fc6f24a011d3d7095013f8e2651b06ae3e0e439f77e44e1c3`，PG/MinIO/gateway/Redis 四个成员；无 host ports。helper 共用 gateway network namespace，read-only rootfs，只挂 metadata/DB 凭据；不挂 bootstrap 凭据。
- 保留 volumes：`dpp-lifecycle307-4abcfca5-postgres`、`dpp-lifecycle307-4abcfca5-objects`、`dpp-lifecycle307-4abcfca5-redis-data`。
- gateway SHA256 `81b26869b3ee1074035798a4bb6ba8c455ec71d69b57b7a7434d0da4d6011662`：DELETE/PUT/POST/PATCH、admin、payload GET 全部关闭。SDK 身份 inspection-only；没有 delete/copy/bypass 能力。bootstrap 与本次恢复窗口均已 disabled。
- 固定 images、成员 container IDs、helper provenance 见 [worker-environment.json](worker-environment.json)、[worker-processes.json](worker-processes.json) 和各 `provenance-*.json`。

## 原生验收结果

SQL fault input 明确标为 `SYNTHETIC_SQL_FAULT_INPUT_NO_NATIVE_DELETE_APPROVAL`。其 dependency/approval/start 是测试输入，不是实际删除门禁或人类删除批准；没有 send claim，没有 asserted native invocation，也没有删除 CostEvent。实际发生的是 native **metadata** 调用和真正 Asynq 进程崩溃/重启。

| 场景 | 结果与证据 |
|---|---|
| Redis 连接不可达，随后接回真实 Redis | PASS：[FAILED/0 confirmations](redis-unavailable.json) → [PUBLISHED/1](redis-recovered.json)；原请求未丢失 |
| 重复入队、完成后的相同 work 重放 | PASS：相同 task/work identity；原 provider attempt 仍为 1，send claim 0，完成重放无新增 native 调用 |
| Run start 提交后、lookup 前退出进程 | PASS：exit 91；[崩溃时](db-crash-before.json) runs=2/results=1/native=30；[Asynq 恢复后](db-resumed-before.json) runs=3/results=2/native=32，原 start 保留 |
| native lookup observation/cost 提交后退出进程 | PASS：exit 92；[崩溃时](db-crash-after.json) native=33，真实 lookup [call ID](barrier-crash-after-lookup.json) 保留；重启新增独立 lookup/HEAD，不覆盖旧调用/成本 |
| 两个独立 worker 竞争同一原 attempt | PASS：[第二名 OWNER_BUSY](competing-owner.json)，provider requests=0；同一 session advisory lock 跨事务覆盖 native I/O；workspace/content transaction 不跨 I/O |
| durable pending scan 唤醒尚未投递的 work | PASS：[scanner-worker](scanner-worker.json)，由同一 Asynq 执行，不是另建 scheduler；全部 6 个 recovery wake 均 PUBLISHED、各 1 次 confirmation |
| 初始恢复权限撤销 | PASS：[调用前后 native 均=39](db-authority-denied.json)，追加 BLOCKED_AUTHORITY；0 新 provider 调用 |
| 对象存在的完整库存 + positive exact HEAD | PASS：5 个 UNKNOWN_PRESENT；原 uncertain/no-observation delete start 与 1 个 unresolved reservation 保留，未自动重发或释放 |
| 四个版本与保护值保留 | PASS：[before](before.json)、[after](after.json)、[native preservation](preservation-native.json)；完整库存仍恰为四份，legal hold/GOVERNANCE 值未变；没有 payload 重读 |
| 原有环境与所有 volumes 保留 | PASS：[preservation-environments.json](preservation-environments.json)：原有 2430 volumes 均在、29 个旧标记容器的 identity/image/state/mount 未变；旧对象未访问、未操作；新增仅三个 named volumes |

[最终数据库回读](db-post-review.json)：6 work、8 runs、6 results（5 UNKNOWN_PRESENT + 1 BLOCKED_AUTHORITY）；1 synthetic original provider attempt，0 send claim/observation/delete allocation，1 unresolved reservation；0 absence receipt/completion。平台 native metadata starts/observations/typed CostAllocations **各 53**，一一对应的 activity IDs 见 [storage-activities.json](storage-activities.json)。新增 scope review 后 Outbox/Audit/Evidence 各 **133**。

## PostgreSQL / 本地检查

- `go test ./...` PASS；未配置外部集成环境的测试仍 SKIP，不归为实际 provider 验收。
- lifecycle application 的真实 PostgreSQL 全包 PASS；六个 recovery fault/concurrency tests PASS：unknown presence、crash/双 worker、partial exact receipts、撤权/六次重试上限、delete marker fail closed、已提交全部 receipts 后崩溃恢复 completion 且 0 新 lookup。
- 独立已迁移 scratch DB 的 outbox integration package PASS。最初使用无 schema 的 admin DB 导致 relation missing，改为隔离已迁移库后重跑通过；没有改动保留环境。
- scoped `go vet`、routing/worker wiring、strict queue envelope、read-only window/scope/expiry、review-only principal tests PASS。

当前 slice 的队列对象是 DELETE_EXACT 原 attempt。COPY/RESTORE 的 UNKNOWN destination/returned-version 恢复尚未接入这个 worker；现有未核验 COPY 必须继续保留/隔离，不能当作恢复完成或重发 COPY 的依据。实际删除后缺失版本的原生 receipt/finalization、完整 D1、HTTP/browser 路径、生产 policy/IAM、PR CI/review/merge 与 matching main CI 均 **NOT_RUN**。

## 新增实际删除：仅下列一份，等待确认

环境 **dpp-lifecycle307-4abcfca5**。仅准备范围，不批准、不 dispatch、不创建 native delete 身份。

| Namespace / exact key | Exact version | 本次处理 |
|---|---|---|
| dpp-lifecycle307-quarantine / fixture/b.csv | **ce87f789-a4f4-4ce7-bf1f-a044ac53d1d2** | **唯一待确认的实际 DELETE**；26 bytes；NONE / legal hold OFF |
| dpp-lifecycle307-online / fixture/a.csv | 4a225512-f6ac-4fc9-b0c0-bd6e016a2a01 | 保留；SQL fault input 的 UNKNOWN reservation 不释放 |
| dpp-lifecycle307-online / fixture/b.csv | 7cd23e55-140c-4566-b881-e58c423c7a8a | 保留；legal hold ON，不解除 |
| dpp-lifecycle307-quarantine / fixture/a.csv | 936c8741-87f6-4279-87b9-742936cb2dd2 | 保留；GOVERNANCE 至 2026-10-12T13:39:45Z，不解除、不 bypass |

新单对象 plan `872ca588-6856-420f-b450-e5cd4e8008f4`；scope hash `7184d41a0b1578c09d5f358a31dfce819946154bc6237e259ae5beb680c5c04b`；[冻结范围清单](pending-delete-plan.json)。FINALIZED 仅说明范围冻结，approval 仍 **PENDING_USER_CONFIRMATION**。

确认后必须重新执行当前 caller/approval/retention/reference/reader/hold/完整库存等删除门禁，门禁全部满足才准该 exact version 的一次实际 DELETE；任何不满足都阻断。随后用现有 worker 做只读恢复/对账，验证 actual post-delete receipt/finalization 和重复投递无再次 DELETE。其余对象、原有四份环境、所有 volumes 全部保留；不解除保护、不 bypass、不清理，不使用已消费的旧确认。
