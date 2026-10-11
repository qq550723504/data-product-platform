# #307 受控 exact-version 删除验收

2026-10-11 19:39（UTC+08）；**已确认范围 PASS，完整 D1 NOT_RUN，Issue 保持 OPEN**。

用户在本聊天明确确认 `dpp-lifecycle307-a6b30840` 及两份无保护对象的 exact version。批准只覆盖 plan `83d0ad2c-0df0-4245-8fbf-6d7190bb77cc`、hash `7c2847699997663ca453db54e0fab1565ad67fc9434c0c2c507c6adaa056a09f`。原 [manifest](manifest.json) 和 [candidate](deletion-candidate.json) 保留为删除前历史快照。

| namespace / key | exact version | 本次结果 |
|---|---|---|
| dpp-lifecycle307-online / fixture/a.csv | cc359f61-4921-4487-852f-b6ab6d139278 | DELETE 接受后，完整版本盘点证明 exact version 不存在；absence receipt 已提交 |
| dpp-lifecycle307-quarantine / fixture/b.csv | 2ae6479b-2b96-441a-9159-80a07e3ec4e0 | 首次拒绝保留；fresh exact HEAD 对账 + 新 gate/attempt 后删除，盘点证明不存在；absence receipt 已提交 |
| dpp-lifecycle307-online / fixture/b.csv | 70e379f9-c782-41ec-bc71-12c8de2b1538 | 仍存在，Legal Hold ON；仅只读领域 prerequisite 阻断核验，native DELETE=0 |
| dpp-lifecycle307-quarantine / fixture/a.csv | 0d09ec19-c3c9-45e3-afbe-2d07068954e2 | 仍存在，GOVERNANCE 至 2026-10-12T10:41:09Z；仅只读领域 prerequisite 阻断核验，native DELETE=0 |

[实际执行和 final inventories](delete-acceptance.json) / [独立 SQL 读回](delete-database-readback.json) / [完成后环境](delete-environment-after.json)。每个 namespace 最终仅剩对应保护版本，无新版本、额外对象或 delete marker。两份 receipts 精确覆盖两成员计划，completion `fee83e4d-6f03-4d76-a85a-5f3a41b19bab`。DatasetVersion identity、原 URI、hash、字节长度及历史状态没有改写。原来的四成员 inventory plan 没有被批准或完成。

DB 读回：4 copies、2 restrictions、1 synthetic test policy basis、1 approval、4 gate evaluations（1 旧 BLOCKED）、3 dispatch/start/send claims/observations（1 DENIED + 2 ACCEPTED）、1 denied reconciliation release、2 absence receipts、1 completion、61 inspection starts/observations、64 invocation cost allocations（quantity 总和64，amount=NULL）、142 Domain Event/Outbox +142 Audit +142 Evidence、reader pins=0。原 DENIED physical attempt 和成本仍存在；新调用另分配新 physical identity。

门禁下使用显式 `CONTROLLED_SYNTHETIC_TEST_ONLY_ZERO_MINIMUM_RETENTION` basis 与真实 start event；不是客户/生产/法定保留政策。完整 registry/native inventory、已生效 restriction、无 reader、保全和当前批准参与 admission；对所有 typed DatasetVersion FK 依赖保守扫描，唯独自身 registry/restriction 控制元数据不视为 bytes consumer。新/未知 consumer FK 的 PostgreSQL regression 会阻断；不把别的 owner 的历史关系自动降级。

发送 claim 调用前持久化，只允许一次发送；HTTP/SDK 不隐式重试。原 native IAM 首先两次拒绝错误 condition casing，未创建批准或删除。随后初次获批调用因 native action 权限不足被拒绝。均保留 [IAM provisioning receipts](executor-admin-receipts.jsonl) 和旧 BLOCKED/denied 记录。固定构建要求小写 `s3:versionid`，并对 version DELETE 检查 `s3:DeleteObject`、对 `DeleteObjectVersion` 做 deny 检查：见 [pinned condition key](https://github.com/minio/pkg/blob/v3.1.3/policy/condition/keyname.go#L98) 和 [pinned authorization](https://github.com/minio/minio/blob/9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a/cmd/auth-handler.go#L430)。最终 [native policy](delete-executor-policy-v5.json) 两项 action **都受同一个 exact version 条件约束**，没有 bare-key、其他版本、protected ARNs、Put 或 Bypass 权限。

原 DENIED attempt 只在新真实 positive exact HEAD 后追加 [对账释放](denied-reconciliation-d48516df-b126-4af3-826d-3f64362d8cea.json)，不是 NOT_SENT，不擦除已发生成本。再次执行需要新 finite/current gate 和 physical start；UNKNOWN/start-only 不适用这条释放路径。

执行二进制为 [v5 window](delete-execution-window-v5.json) 的 SHA；历史 v3/v4/原 bootstrap 二进制分别保留，没有覆盖对应 hashes。fixture DB 保留旧54 baseline，加装经过测试的 additive58+59 tables；`schema_migration` 仍为54，没有安装重复57、伪造完整升级或擦除旧库。Fresh PostgreSQL tests 按 main55 + D157/58/59 安装通过；C1 尚预留56，发布前仍需串行核对实际 merge 顺序和共享 routing。

执行窗口已关闭：两种 fixture principal 配置 disabled，native executor SetUserStatus=disabled（HTTP200），[网关 DELETE 全拒绝](delete-window-closed-v5.json)，PUT/POST/payload GET 关闭。内部网络仍只有原三个服务，无 host ports；原两个 named volumes 保留。object cleanup=0、volume cleanup=0；没有解除 hold/retention、没有 bypass。首次脚本预检的 PowerShell 命令重名/自动参数问题在任何 schema/provider 工作前修正，不把预检卡住算执行。

验证：Domain/Application/Adapter/routing race tests PASS；真实 PostgreSQL 验证 send claim 并发、撤销后的成本保留、UNKNOWN 不释放、DENIED 对账新 attempt、裸 absence/marker/部分 membership 拒绝、不可改写 receipts、未来 FK consumer 阻断。实际 native 执行与 SQL/环境读回如上。模块 test/vet PASS。保护对象的 native DELETE 拒绝没有实跑，也无需实跑；其报告只证明 native 状态 + 只读领域门禁阻断。

范围仅为**provider 可访问命名空间内列明的 exact versions 已验证不存在**。provider 内部介质、备份/replicas、外部 recipients 未确认。Archive/restore、真实业务 reader/processing/delivery 接入、自动 worker/reconciliation recovery、独立恢复 ledger/watermark、完整 D1/LC matrix 仍 NOT_RUN/待开发。没有 PR、push、merge、PR CI 或 matching main CI；#307 不关闭。
