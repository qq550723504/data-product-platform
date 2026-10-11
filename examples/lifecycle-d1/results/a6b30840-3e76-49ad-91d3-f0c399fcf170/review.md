# #307 删除前待确认清单

状态：**PENDING_USER_CONFIRMATION**。仅受控合成对象；DELETE=0，对象清理=0，volume 清理=0。确认来源要求是用户的“生成实际对象 key/version 和环境清单给我确认；确认前不要执行删除或对象清理”。

## 环境

| 项目 | 实际值 |
|---|---|
| Run / project | `a6b30840-3e76-49ad-91d3-f0c399fcf170` / `dpp-lifecycle307-a6b30840` |
| Workspace | `39dd70e6-691a-4d69-b971-6c75533b72d6` |
| Provider instance / incarnation | `5e8a5758-96f6-4bf4-a088-1decd62809b0` / `78ebff25-faa6-4f9b-8275-6204d7b786ab` |
| Network | `dpp-lifecycle307-a6b30840-private`；internal=true，仅专属 PostgreSQL/MinIO/gateway 三个成员 |
| 服务容器 | `dpp-lifecycle307-a6b30840-postgres`、`-minio`、`-gateway`；均运行中；`-prepare` 和只执行 SQL 的 `-scope` helper 已成功退出 |
| 持久化 volumes | `dpp-lifecycle307-a6b30840-postgres`、`dpp-lifecycle307-a6b30840-objects`；保留，无复用其他项目 volume |
| 实际宿主端口 | **无**。Gateway `127.0.0.1:9000` 只在其 network namespace 内可用；最初请求的 loopback publish 没有获得实际 binding，详见 JSON |
| 普通检查身份 | 固定四个 key 的 inspection policy；无 Put/Delete/Bypass；payload GET 被 gateway 拒绝；fixture principal 已禁用 |
| Provider | MinIO image `sha256:a9eb76492485dd496bfd69b4114e47137f2da1a1a87e4a9b273cb21476cb1a98`；binary SHA256 `fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8` |
| Native build | `DEVELOPMENT.GOGET`；source `9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a`；Go 1.26.5；AGPL-3.0；minio-go v7.0.83 / madmin v3.0.94 |
| PostgreSQL / gateway images | `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea` / `sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f` |

完整 container IDs、network ID、配置/程序/source hash 见 [manifest.json](manifest.json) 和 [environment-readback.json](environment-readback.json)。私有凭据未包含在清单内。

## 四个实际对象版本

| Namespace | Key | Exact native version | Bytes | 当前保护 / 后续处置边界 |
|---|---|---|---:|---|
| `dpp-lifecycle307-online` | `fixture/a.csv` | `cc359f61-4921-4487-852f-b6ab6d139278` | 27 | 无 native hold/retention；只能在批准后且完整门禁允许时作为 exact-version 删除测试对象 |
| `dpp-lifecycle307-online` | `fixture/b.csv` | `70e379f9-c782-41ec-bc71-12c8de2b1538` | 26 | legal hold=ON；后续仅验证阻断，不解除 hold，不执行 provider DELETE |
| `dpp-lifecycle307-quarantine` | `fixture/a.csv` | `0d09ec19-c3c9-45e3-afbe-2d07068954e2` | 27 | GOVERNANCE 至 **2026-10-12 18:41:09 +08:00**；后续仅验证保护阻断，不 bypass，不执行 provider DELETE |
| `dpp-lifecycle307-quarantine` | `fixture/b.csv` | `2ae6479b-2b96-441a-9159-80a07e3ec4e0` | 26 | 无 native hold/retention；只能在批准后且完整门禁允许时作为 exact-version 删除测试对象 |

A SHA256=`818392d0732d499a777b5b5a74f1351ddffce3cf8a3b66f8d91dbfd98dfa1523`；B SHA256=`aafd9ae024f2444eefea042c59ffd87c29fac3f4675c49124785b417da34b784`。四份均重新读取同一 exact version 校验 bytes/hash；Core copy IDs / DatasetVersion IDs、最后 observation 时间、完整 namespace native inventory、无额外版本/marker 见 JSON。

## 冻结范围与实际证据

四个 targets 的范围冻结计划 `de994857-c0aa-4f86-beb7-0012f06f7a4a`；算法 `SHA256-LIFECYCLE-PLAN-V1`；scope hash：

`a68edc4456d3153316d7a585b64818b1275300e090e0258e37f51a71b9118f04`

FINALIZED 仅表示范围已冻结，**不是批准或可删除**。真实 PostgreSQL 再计算 hash 与其一致。两条 use restrictions 覆盖两个 DatasetVersions 的全部 copies；没有正常 reader admission。Native hold/retention 是 provider protection，不冒充 Core legal/legal-rights 判断或 business hold 命令。

真正请求后续删除测试的范围另有 **两份无保护对象**的冻结候选：[deletion-candidate.json](deletion-candidate.json)。仅包括 online/A `cc359f61-4921-4487-852f-b6ab6d139278` 和 quarantine/B `2ae6479b-2b96-441a-9159-80a07e3ec4e0`。

候选计划 ID `83d0ad2c-0df0-4245-8fbf-6d7190bb77cc`；scope hash：

`7c2847699997663ca453db54e0fab1565ad67fc9434c0c2c507c6adaa056a09f`

候选通过现有 PrepareDeletePlan 创建，预先比对 registry 的完整 immutable identity；**storage I/O=0**、approval=0。它没有解除任何保护，也没有更新原四对象计划。[额外计划读回](deletion-plan-readback.json) 证明两份计划 hash 均与 DB 一致，calls/cost 仍19，新增一套 plan Event/Audit/Evidence 后各为42，dispatch/delete-attempt 仍0，principal 再次禁用。

[database-readback.json](database-readback.json) 独立读回：4 registered copies、2 restrictions、19 physical inspection starts / observations / cost allocations（quantity 总计19，amount 未定）、41 Event / Audit / Evidence；approval=0、dispatch=0、delete attempt=0、reader pin=0。19 包括5次接入检查和最后14次完整检查，失败/此前未识别的 native protection 响应未被擦除或吞掉成本。[bootstrap receipts](bootstrap-receipts.jsonl) 保留创建/bytes 验证记录；[inspection policy](inspection-policy.json) 保留无删除权限的 policy。

PASS：实际 version、exact bytes/hash、当前 native hold/retention、完整单页 inventory、平台 Journal/成本原子记录。NOT_RUN：任何删除、absence finalize、archive/restore、业务 reader、完整 D1/LC 验收、PR CI/review/main CI。

## 请求确认的范围

请确认以上**这一套环境及两对象候选 scope hash `7c284769...`**可用于后续 D1 受控删除验收：两份无保护对象在 scoped approval 和 current gate 允许后进行 exact-version 删除测试；另外两份保留保护，只验证门禁阻断。实际执行前仍复核同一 container/volume/incarnation、exact inventory、当前保护与冻结计划；任何 scope/version 扩张或更换都不继承本确认。

**不包含**解除 hold、retention bypass、更换/扩张 namespace、bare-key 删除、额外 versions、#298 demo/retained 对象、其他项目/container/volume、外部备份/recipient、真实客户数据。对象/bucket/volume 的收尾清理仍待单独明确确认；保留数据库与 Evidence/Audit/Cost 历史，不做 down/truncate/软删除伪装修正。
