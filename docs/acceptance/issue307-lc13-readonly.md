# #307 LC-13 processing 原生崩溃与 5d 删除前只读核验

2026-10-14，按[有限 contract](issue307-lc13-readonly-contract.md)完成。
#318 保持 Draft；真实 DELETE、merge、hold release、retention bypass、cleanup 均为0。
完整 LC-13 credential/containment 组合、D1/D2 和生产 DR 仍未验收，#307 保持 OPEN。

## 本轮验收矩阵

| 用例 | 结果与证明边界 | 证据 |
| --- | --- | --- |
| 原生 processing 读后崩溃 | 两份真实 Asynq task 各 RUNNING→三个输入 pins 原子提交→一次 READ_PINNED GET 入账→helper exit95；没有 Execution 输出。不是 SQL 模拟崩溃。 | [崩溃及自然使用期到期读回](evidence/issue307-d1/lc13-readonly01/crash-expired-before-recovery.json) |
| 孤儿任务与重复交付 | 两个原 crashed task IDs 都由真实 Asynq worker 再交付，RUNNING handler no-op；随后各一次显式新 enqueue，计数仍不变。Outbox dispatcher 本轮 NOT_RUN。 | [task identity](evidence/issue307-d1/lc13-readonly01/orphan-task-identity.json)、[worker 回执](../../examples/lifecycle-d1/results/04a6ff55-f366-4a6e-8a1b-9cc79bea15e0/lc13-orphan.json) |
| reader use_until 自然到期 | beta 使用期 `09:25:13.582499Z` 到期；独立 READ ONLY 连接仍读到两份 RUNNING、六 pins、零 termination。未改 persisted time/status、未凭到期解 pin。 | 同一崩溃读回 |
| 原生 lease recovery | started_at 分别 `09:23:15.611441Z` / `09:23:17.471606Z`；自然默认十分钟后，`09:33:46Z` 既有 NativeReconciler 追加恢复事实并终结 FAILED/NATIVE_RECOVERY_FAILED。两个 wrapper 都被旧 pin guard 拒绝；新增 GET/PUT/stop/output=0，六 pins 保留。 | [恢复回执](../../examples/lifecycle-d1/results/04a6ff55-f366-4a6e-8a1b-9cc79bea15e0/lc13-recovery.json) |
| restriction 与恢复 / fresh retry | 第二份恢复前追加 current restriction；旧 pin guard 仍先于 reader gate，不声称该恢复执行了 restriction reader gate。独立显式 Retry Command 被当前 reference admission 拒绝；零新 input read/output。每份 terminal recovery replay 计数完全不变。 | 同一恢复回执；此前 restriction 双方顺序证明见[前轮](issue307-d1-native-completion.md) |
| 历史与成本 | 原六 pins 和两条 READ_PINNED start rows 原样保留。fresh 环境34次 native calls/observations/typed costs，逐调用恰一次 allocation。另有4条 Core POC_ESTIMATE engine-invocation成本，其中2条是恢复活动；不是新增物理 GET。2组恢复 Evidence/Audit/Outbox 保留。 | [READ ONLY 账务](evidence/issue307-d1/lc13-readonly01/lc13-accounting-readonly.json)、[SQL](evidence/issue307-d1/lc13-readonly01/lc13-accounting-readonly.sql) |
| 5d 六 exact versions | native checkpoint reader/inspector复查六份 bytes/hash/hold/retention 和全 inventory，与原 after 证明逐项一致。新增20个维护调用只写新 auditCore，payload 留私有目录，源 journal仍92。 | [six versions](evidence/issue307-d1/lc13-readonly01/5d-03/native-six-version.json)、[audit cost](evidence/issue307-d1/lc13-readonly01/5d-03/audit-costs.json) |
| 5d 源 Core只读 / 新恢复 | 精确源 PG/MinIO身份、images、durable volumes、internal网络/0 ports、封存配置核对后启动；原gateway始终stopped。源194表/1879行before/after指纹完全相等。新 pg_dump→auditCore restore、fixture SCRAM role和owner/ACL hashes一致；未迁移/写入旧restore。 | [before](evidence/issue307-d1/lc13-readonly01/5d-03/source-before.json)、[restore](evidence/issue307-d1/lc13-readonly01/5d-03/audit-restored.json)、[after](evidence/issue307-d1/lc13-readonly01/5d-03/source-after.json) |
| 旧备份与最终封存 | 原 native tar/core dump hashes保持不变，不覆盖历史备份。成功阶段后原PG/MinIO/gateway与新auditCore/helper全部exited；业务/deletion配置未改。后续PG-only READ ONLY读回业务投影/counts并再次停止。 | [checkpoint](evidence/issue307-d1/lc13-readonly01/5d-03/readonly-checkpoint.json)、[最终停止](evidence/issue307-d1/lc13-readonly01/5d-03/retained-final-state.json)、[current readback](evidence/issue307-d1/lc13-readonly01/5d-03/current-core-readback.json) |

LC-13 新环境 run `04a6ff55-f366-4a6e-8a1b-9cc79bea15e0`、workspace
`cdafbd76-a3e2-4da6-8192-f43bcb324f1e`，schema68/base074a33ba；网络
`10.234.158.0/24`、内部、0 host ports，专属 PostgreSQL/MinIO/Redis named volumes。
原四份 fixture和一份 Entity standardized输出均保留；表内 processing PUT1/output1是setup的Entity输出，
两份崩溃 Execution 的 PUT/output均0。当前所有业务窗口false、gateway全403，
六个未知 stop pins有意保留。没有新的 owner终止协议或自动解 pin。

三个新auditCore的只读维护配置在服务停止后由host单独封存为 PreparationEnabled=false，
原配置副本私有保留。源/旧restore配置未动，0 provider/SQL动作；见
[维护窗口封存](evidence/issue307-d1/lc13-readonly01/audit-maintenance-sealed.json)。
这项最后的host封存不声称由本轮driver finally自动完成。

复用现有 Ports/SPI、queue/Handler、PinnedEngine、NativeReconciler、shared fence、session lock、
call journal、Cost/Audit/Evidence/Outbox、MinIO 与 pg_dump/pg_restore；生产 guards/recovery未改。
LC13 actual binary来源 `83de3b58`，初始 preparation/provision来源 `74b3bddf`；
5d 使用原已验证 `4af0ee88` 的 e1d9二进制，当前只读driver冻结 `69529668`。
不把缺失VCS build metadata当成最终HEAD标识；各二进制/hash/源码绑定见本目录回执。

## 失败保留、评审与回归

前两代5d auditCore均已成功恢复194表/1879行，但helper分别在CRLF shell和错误flag入口处失败，
尚未native调用，原服务finally均停止。副本、helper、named volumes和私有诊断全部保留；
没有覆盖旧generation。独立review还阻断了未写完整PROFILE的入口及finally首个异常短路源stop的风险。
修复使用固定LF/PROFILE/MODE；只停止已确认创建的helper ID，逐资源独立尝试、最后汇总失败。
7个pure tests包含拒绝错误auditDSN/root/source identity/保护变化、无DELETE CLI和失败停止注入，
正常及Python优化模式均PASS。

相关真实PG race回归：4包/134个测试及子测试PASS，0 fail/0 race；fixture command编译通过、无测试文件。
schema68安装到新的保留testDB，不复用固定ID旧DB。相关 `go vet` PASS。
前轮全量89包/1117项结果仍保持原source证明边界；本轮current-head PR CI另核对，不冒充main CI。
独立review是冻结源码与证据的只读审查，未替root执行SQL/tests/provider操作。

## 下一步确认边界

5d源事实仍为6 copies、23 requirements/releases、7 pins/verified stops、active0、
92 storage calls/observations/cost allocations、2 NOT_SENT/released、0 send claim/absence/completion。
新auditCore含导出的原92条和本轮20条维护账目，总112；这些新增账目不写回源。
原tar SHA256 `f72efeab2c49802be1661a431164167d7a34968fef877163f08b25878d9554cc`，
原Core dump SHA256 `19dba12b9992fdd11c237d6f0df489c7a961696acd54b8a3403b941e7005c98a`。
本轮另保留独立只读导出及新auditCore，不声称重新恢复了原native tar。

[精确方案](issue307-d1-delete-proposal.md)和[proposal-v3](evidence/issue307-d1/lc13-readonly01/proposal-v3.json)
只更新当前观测和验收，不改变两项version范围或执行预算。未收到删除确认，未创建新deletion
basis/approval，未开启delete IAM或发送路由。未来每项发送前仍须fresh授权、保护和inventory。
本轮LC13新环境、全部恢复副本、备份、卷及其他环境均不在删除清单。
旧04a6同Execution续跑、通用外部owner stop/自动解pin仍未验收；它的六pins保持原未知状态。
后续明确选择新Retry Execution，闭合的native owner stop与credential containment已在
[独立新环境矩阵](issue307-lc13-continuation.md)验收，不反写这里的旧源码/二进制证明。
真实DELETE fault/race、完整D1/D2仍未验收。
