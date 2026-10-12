# #307 归档源 exact-version 删除受控验收

2026-10-11 21:06 +08 实跑完成。用户明确确认 `dpp-lifecycle307-c9f47cab`，仅删除 online `fixture/a.csv@3fe1cc44-b0f8-428c-9bb8-9f92dac5d15d`；本次新增 DELETE=1，结果 **PASS_SCOPED_EXACT_VERSION_ABSENCE**。这不是全域销毁、介质擦除或生产验收。

| 范围 | 实际值 |
|---|---|
| Environment / run | dpp-lifecycle307-c9f47cab / c9f47cab-23c6-4d16-8102-a4a34bac1501 |
| Workspace | 5fe971c0-7682-4b6b-b4c6-741883247365 |
| Provider / incarnation | 24dba412-9b64-46c0-9cc5-ae56a5d3a124 / b19427cf-0d61-4417-a5e1-76c7dd894046 |
| Namespace / key / exact version | dpp-lifecycle307-online / fixture/a.csv / 3fe1cc44-b0f8-428c-9bb8-9f92dac5d15d |
| Copy / DatasetVersion | 1a0bfd6d-6092-41e8-9231-71ef2cf98713 / aa67944f-df64-4ad3-a9a5-4f205ebe8f97 |
| SHA256 / bytes | 818392d0732d499a777b5b5a74f1351ddffce3cf8a3b66f8d91dbfd98dfa1523 / 27 |
| Approved plan | 50b71a6c-f837-42e4-a670-c924aebceb22 |
| Frozen scope SHA256 | 7ccf841b84d919b3f9634983fe789e74fb82b0cbbf86017fd1ad95a841aed47e |
| Source candidate | 702e882ded77c81c357e591e1dc40c2d855ea03d |

复用现有 PrepareDeletePlan、DispositionService、ExactExecution、MinIO native SDK、shared delivery fence、immutable Journal/CostAllocation、Audit/Evidence/Outbox。测试处置依据原来硬编码四份库存/两份目标，现改为在同一 fence 下验证完整冻结 registry 与批准子集，仍只适用于两 namespace 的受控合成 fixture。没有业务保留期默认值、新 storage engine、队列、迁移或生产删除入口。完整九目标 inventory plan **仅登记范围，没有获得删除批准**；只有上述单目标 plan 获批。

fresh 门禁在 dispatch 时全部 SATISFIED：完整库存、全部 typed Core references 无消费者、provider retention、当前 ledger、已限制使用、无 hold、reader 排空，unresolved dispatch/copy intents=0。持久化 evaluation `19928801-743a-41cd-af28-4838a9b27a76`，blockers=[]。发送 claim 在共享 fence 下再次校验当前 caller/approval/dependencies；归档 sibling 必须有 immutable same-byte verification + ARCHIVED fact，并有最新正向 HEAD、完整 quarantine inventory。见 [持久化门禁与调用观察](delete-gate-db-readback.json) / [dispatch](delete-current-gate-1a0bfd6d-6092-41e8-9231-71ef2cf98713.json)。

只有一次 physical attempt `eb82dede-f7a1-4922-8dac-cb026a3133c5`、一次 send claim、一次 CONFIRMED/ACCEPTED observation。无自动重发、batch/bare-key DELETE、保护解除或 bypass。native IAM 仅允许固定单 ARN + `s3:versionid` 删除；gateway 对完整 URI、临时签名身份及方法做单版本匹配，其他 mutation/payload GET 始终拒绝。

provider ACK 后才做完整 native namespace LIST；没有用 bare HEAD404、delete marker 或 ACK 冒充 verified absence。scoped absence receipt `d4f752d4-2bd7-4a09-8060-d6e0c61add29`；plan completion `51e59f6e-bd42-4546-8fdb-c58eb4b698cf`，target_count=1。JSON 中 `delete_outcome=PROVIDER_ACCEPTED_ABSENCE_UNVERIFIED` 是发送后、验缺前的观察阶段；最终结果由其后的 receipt/completion 决定。见 [完整验收](delete-acceptance.json)。

其余八个 exact versions 全部保留，无额外版本或 delete markers。归档 `archive/a.csv@613c3231-5077-4326-9a7c-4a073a105e47` 仍在；四份恢复副本和未批准的 quarantine/B 均保留。逐份 positive HEAD、native retention/hold 与完整 inventory 对照前后身份、大小和保护；新环境 online/B LegalHold ON，quarantine/A GOVERNANCE 至 `2026-10-12T12:43:43Z`，两者只做 read-only prerequisite 阻断，DELETE=0。原始九版本清单及复制/恢复历史见 [归档阶段报告](archive-review.md)，本报告是其后续，未重写既有事实。

原 `dpp-lifecycle307-a6b30840` 的两份保护对象未参与本次对象请求；未改保护、schema、配置或重启该环境。a1647373 / bb44901c 失败运行继续保留，包括 bb44901c 的未验证 archive version。四环境共十二个服务容器、八个 named volumes 的 identity/image/owner label 只读核对通过，见 [独立保留核对](delete-preservation-readback.json)。独立核对读取持久化 native 回执及资源元数据，没有额外发送 provider object 请求。

执行结束后临时 native delete user disable 得到 HTTP200；bootstrap/copy/delete authority 均 disabled。gateway 关闭 admin 路由，所有 DELETE/PUT/POST/payload GET 拒绝。未清理其余对象或任何 volumes；所有成功/失败 helper 和事实保留。见 [窗口封口](delete-window-closed.json) / [执行前环境](delete-environment-before.json)。

当前合计154 physical storage starts/observations + 1 physical DELETE，对应155 CostEvent及155 typed allocations。Domain Event/Audit/Evidence各348；1 absence receipt、1 completion、1 approved plan，active readers=0、unverified copy intents=0；原有5 same-byte copy verifications、6 admission decisions保留。DatasetVersion 仍为 READY，原 storage URI/hash/byte_size 不改写，current use restriction 保持阻断。见 [只读 DB counters](delete-db-facts.json)。

测试：真实 PostgreSQL `go test -race -count=1 ./internal/lifecycle/... ./internal/platform/storage ./internal/platform/routing ./cmd/lifecycle-fixture-delete` PASS，包含完整九副本/单批准源、漏库存失败且不提交、幂等/历史不变及既有 fence/send/reader/copy barrier 回归；command unit、targeted vet、PowerShell parse、diff、私有 credentials 扫描 PASS。native execute/provision/disable 各在启动前保存 binary/source/topology/gateway hashes，见 [execute provenance](dpp-lifecycle307-c9f47cab-source-delete-execute-provenance.json)。本次 native source candidate为702e882d；后续证据提交不冒充新的 native 实跑或 exact-HEAD CI。

#307保持 OPEN。本切片 archive/restore + approved source absence 为 PASS；完整现有 worker/outbox recovery、全 D1 用例、生产 DR/cold tier、真实 rights/certified delivery、PR CI/review/merge/matching main CI 为 NOT_RUN。没有 push、PR、merge、Issue 关闭或长期删除授权。后续新增实际删除仍须先提供环境与 exact key/version 清单并取得明确确认。
