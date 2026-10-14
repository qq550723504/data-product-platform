# #307 D1：5d2274ee 环境的 exact-version 删除确认方案

2026-10-14，状态 **PENDING_USER_CONFIRMATION / 实际 DELETE NOT_RUN**。
本方案依据 #307 的 exact-list approval gate、lifecycle 主 contract 与
[有限引用需求 contract](issue307-reference-demand-contract.md)。#318 保持 Draft，
本轮已完成明确授权的非删除备份/恢复和 no-send 事务；不启用发送、不发 DELETE、不解除保护。
此方案替代“下一次执行”的旧 e1487ce7 提案；旧提案及其他环境的已消费批准保留原历史。

当前状态见[原生非删除准备](issue307-d1-reference-preparation.md)：六份 exact-version
原生备份/独立恢复、两条原 trial 的原子 NOT_SENT/release、schema68、194表/1879行的新 Core
checkpoint/独立恢复/SCRAM、关闭的发送组合均已验证。native IAM provision/activation、
新短期 deletion basis/approval、真实发送和当前环境队列 recovery 仍未执行。删除确认仍未收到。
旧方案机器清单、旧 Core1377行回执和失败资源保留为历史，不覆盖。

当前机器清单：[proposal-v2.json](evidence/issue307-d1/reference-disposal-preparation01/proposal-v2.json)。
原清单：[proposal.json](evidence/issue307-d1/delete-proposal-5d2274ee/proposal.json)，仅记录初始准备时点。
实时元数据：[topology](evidence/issue307-d1/delete-proposal-5d2274ee/topology-readback.json)；
[当前 Core readback](evidence/issue307-d1/reference-disposal-preparation01/current-core-readback.json) /
[READ ONLY SQL](evidence/issue307-d1/delete-proposal-5d2274ee/core-readback.sql)。
JSON 中含完整 copy/DatasetVersion identity、SHA256、容器 ID、镜像 ID 和保护观测时间。

## 精确环境与冻结范围

| 字段 | 值 |
| --- | --- |
| Project / 原环境 run | `dpp-lifecycle307-5d2274ee` / `5d2274ee-a671-4bdc-b406-22b8dad8fd61` |
| Workspace | `89eba3fd-9763-4f66-8cec-1a779104f155` |
| Provider instance / incarnation | `1980751b-c50e-4895-93a8-cb55960cd635` / `be784eda-96e4-49a9-ae47-eaa3203a2dc5` |
| 两份目标的 FINALIZED plan | `8f0e9340-18f0-4a73-bdd6-622196a9011a` |
| Plan SHA256-LIFECYCLE-PLAN-V1 | `d7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc` |
| 六份完整 inventory plan | `dfd47e35-b99a-4581-942f-96ab5d3941f1` |
| Inventory hash | `2b1df1ccd9e7a138bf47050cad9f031c02bd04664b64eb12aee0337ad670c9f8` |
| 当前 Schema / 非删除原生源码 | 68 / `4af0ee88db92dfbe7aad029b22916bb046155e27` |
| 原历史 native/source | `4650c435f8ba89fdb0b2dc0591152d25ded182d7`；原 LC-11/12 不冒充新源码重跑 |
| 新 Linux executor binary SHA256 | `e1d9d35e48210ffe4400edb3476bdc8a51cb71cab9bd05686aedbc791a0a5b30`；发送关闭；[完整 build binding](evidence/issue307-d1/reference-disposal-preparation01/build-source02.json) |
| 私有配置目录 | `C:/Users/Henry/.codex/fixture-private/dpp-lifecycle307-5d2274ee`，凭据不进入方案或 Git |
| 网络 | `dpp-lifecycle307-5d2274ee-private`，`10.234.151.0/24`，internal，无 host ports |
| Network ID | `33f303a57406f05c4e78b3cdb1355059a8f55f552e438162edc5bff129f4c02c` |
| 保留卷 | `dpp-lifecycle307-5d2274ee-postgres`、`dpp-lifecycle307-5d2274ee-objects` |
| 当前窗口 | config/reader/admin Enabled=false；封存回执为 reader/producer DISABLED、gateway DENY_ALL |

| 服务 | 容器 ID | 固定 image ID |
| --- | --- | --- |
| `dpp-lifecycle307-5d2274ee-postgres` | `c85b9ff408ed110f531ac79d27a3b99f66242d12766e1674e3de78067d140bd0` | `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea` |
| `dpp-lifecycle307-5d2274ee-minio` | `0a344aa06c7bc9c59e5b5ffe8d8d5584e22abf1daeb8b28c549a9735f5b6bf65` | `sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6` |
| `dpp-lifecycle307-5d2274ee-gateway` | `ba90451138af2c980f2ee77b58bfe2908ef67e06b1e03c96fd1a4e0836400af4` | `sha256:d8542f48d34a9cf4e4c11a478865229840e87e4c96ea3f439101f31a5d35f75f` |

三项服务在本轮只读核对时 running，身份、镜像、专属卷和网络与原回执匹配。
MinIO binary SHA256 `fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8`，
`DEVELOPMENT.GOGET` 固定受控基线；MinIO SDK `minio-go/v7 v7.0.83`、
admin SDK `madmin-go/v3 v3.0.94`。provider build/license 的旧核对不构成生产维护承诺。

## 拟确认的两份删除目标

仅允许以下两项 `DELETE_EXACT`，共 **129560 bytes**，每项最多一次 provider DELETE。
同 key 的其他 version、其他 bucket 或其他 provider incarnation 都不在范围内。

| Namespace | Key | Exact provider version | Bytes | Native legal hold / retention |
| --- | --- | --- | ---: | --- |
| `dpp-lifecycle307-online` | `fixture/a.csv` | `312365a9-f984-43de-b9d5-7e26009ccd82` | 232 | false / NONE / retain-until=null |
| `dpp-lifecycle307-quarantine` | `fixture/b.csv` | `846d0c7c-289d-4667-bc7c-aa04e44357d4` | 129328 | false / NONE / retain-until=null |

目标 A copy `8b1ff30c-3695-4847-b4cf-cc92cd6a399c`，DatasetVersion
`dd7aa6eb-50ea-4c5c-a6ff-79a244bcc152`；目标 B copy
`0a239dc6-9e1d-49eb-8604-e22a93e7b6a0`，DatasetVersion
`35b26496-f832-4496-bb91-1f50fb56e7fa`。
对应 bytes SHA256 分别为
`722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0`、
`b5b9321c647c6dc75e27c25beb46ecf7f4cf10f9bdd0d288887e41c343ef3524`。

这些保护状态来自本轮
[源 after 原生复查](evidence/issue307-d1/reference-disposal-preparation01/native-after/native-six-version.json)，
全部六份 identity/bytes/hash/protection 与 before 和隔离 restore 一致。具体保护观测时间逐项保存在
proposal-v2；它仍不替代未来发送时的 current protection。
执行前及每项发送前均须重新读取 native protection/inventory；新增 hold、未到期 retention、
UNKNOWN、身份漂移或 inventory 增减均使本提案停止，0 DELETE，不自动扩大范围。

## 明确保留的四份版本与影响

| Namespace / key | Exact provider version | Bytes | 保护状态与保留原因 |
| --- | --- | ---: | --- |
| online / `fixture/b.csv` | `159e90c3-adb6-4ea3-b2e8-2804e578e86f` | 129328 | native legal hold=true，retention=NONE |
| quarantine / `fixture/a.csv` | `5a7ce790-9093-4569-a8fd-0726009a5eb4` | 232 | legal hold=false，GOVERNANCE 至 `2026-10-15T00:58:47Z`（新加坡时间 10-15 08:58:47） |
| online / `fixture/standardized.csv` | `2139f293-87f4-4d49-a9f0-941b22275093` | 494 | legal hold=false，NONE；Entity 输出，不是删除目标 |
| online / `fixture/processed.csv` | `7c8ad93b-31ee-445c-9402-816dda0410ec` | 232 | legal hold=false，NONE；NATIVE 输出，不是删除目标 |

表内 online/quarantine 分别为 `dpp-lifecycle307-online` / `dpp-lifecycle307-quarantine`。
retention 到期也不使保留版本进入删除范围。平台 hold table 当前为 0 不代表 native legal hold 为 0。

两份目标已在非删除阶段停止新使用、完成 7 pins/7 verified stops 与 23 个 exact-copy owner releases。
删除后，所列 physical copy 的源读取/Execution replay、mapping/Entity 重验、Quality 复算、
Certification 内容重验与 Delivery payload replay 无法再依赖该 copy。原 FK、DatasetVersion
URI/hash、Execution、Rights、Quality、Certification 和 ISSUED 历史不改写。
其他同 bytes copy 或新输出仍然保留，不自动获得“可替代目标”的 admission，也不绕过 current rights。

cleanup 清单为 **空**：0 bucket/object 清扫，0 container/network/volume 删除，
0 hold release，0 retention release/bypass，0 bare-key/delete-marker 请求。
全部原环境（包括 e1487ce7、9812f0fc、#298/#294/Pilot/demo）、Core/history、备份/恢复 payload、
provider 内部 media/replicas/backups 与 external recipient copies 均排除在 source absence receipt 外。
本方案不声称同 bytes 全域销毁或物理介质擦除。

## 执行前阻断与现有设施复用

本轮最终 READ ONLY Core snapshot：6 copies、23 requirements/releases、7 pins/terminations、
active=0、92 calls/observations/typed storage costs；2 NOT_SENT observations/2 dispatch releases，
send claims/absence/completion=0。新增40个源 provider只读调用；恢复副本另20个调用，不进入源ledger。
copy operations=0；2 processing output intents 对应 2 ready outputs。
FINALIZED 两份 plan 的 DB computed hash 与 stored hash 相同。

**旧非删除 dispatch 不是可以续发的删除 attempt。** 原批准
`34274662-4973-4621-ab4d-b89c0493f1dc` 的 basis 是
`CONTROLLED_NON_DELETE_ADMISSION_EXERCISE`，且已于 `2026-10-14T01:44:38.439783Z` 过期。
以下 immutable starts 已由可信 server transaction 原子追加 NOT_SENT/release；原 facts 保留：

| Copy | 原 dispatch | 原 attempt |
| --- | --- | --- |
| online/a | `8b9b4d0c-df37-41b5-b3d1-0630deae8675` | `0a86461d-d5c2-4c00-99ae-0d44ac6dde2a` |
| quarantine/b | `aa0dc82a-06b4-4b0d-a066-33d19ccc8f45` | `d8a52b7b-9cad-4ab4-a8d5-286d37ceafb9` |

不能更新它们的 approval ID、删除 start、改旧 basis 或手工 SQL 清 reservation。
[原生事务与同proof replay](issue307-d1-reference-preparation.md)已完成：可信 host重新核验原
helper退出、固定入口/binary/root-free mounts/native policies；服务端在 fence/plan/attempt锁下验签、
核对原批准与无claim/冲突事实。两成员一次提交 NOT_SENT/release/Event/Audit/Evidence/Outbox；
JSON签名字节失真导致的首次失败完整回滚并保留，新bin02和新proof成功。NOT_SENT无虚构invocation cost。
后续仅READ ONLY核对原closure，不再生成新proof或重复追加旧trial；新删除必须用新attempt。

`lifecycle_fixture_disposal_basis.run_id` 唯一且 immutable。
新受控执行 stage 预留 `614cd9ad-3f9d-40be-b110-3f7cf95f0790`，已用于关闭的新 composition 和 no-send 幂等身份；
**没有创建该 stage 的 deletion basis/approval/credential/enabled gateway 或发送attempt**。后续追加新的 synthetic basis 和短期批准；
原环境 run/labels 不变，不把旧非删除 run 重新命名成已获批删除。

旧 `run-confirmed-race.py` / `run-confirmed-*-delete.ps1` / delete command profiles
固定了其他 run/key/version/schema，部分批准已消费；不执行、不改旧 profile 来冒充本次准备完成。
本环境的封闭 composition、可信 NOT_SENT verifier/原子bridge、六版本恢复和新Core检查点
已完成非删除验证。sender要求固定 binary/profile/IAM/gateway hashes、server-owned permission、
精确确认文本、fresh签名host topology及≤40分钟窗口；关闭状态在DB/provider之前拒绝。
原生Caddy配置语法通过，两个实际Caddyfile私有；IAM仅渲染，native provision尚未验证。
继续复用既有 exact execution、Storage Port/MinIO、delivery fence、Owner release、Outbox/Asynq/
只读 reconciliation、Cost/Audit/Evidence；不增加第二 runtime、不建设通用删除API。

## 确认后拟执行的验收步骤

| 顺序 | 动作 | 通过标准 / 停止条件 |
| --- | --- | --- |
| 1 | 绑定用户对本 project/provider/incarnation/plan hash/两份 version 的明确确认；冻结新 composition source、binary、profile、IAM/gateway hash | 无确认则没有 enabled permission；新 profile 的 fake/PG 非删除检查通过，所有范围/窗口/old-approval 拒绝路径有效；若改变 targets/environment，重新确认 |
| 2 | 保持 business reader/producer 封存，重新核对当前 generation02 Core 和六版本 checkpoint；若源facts已变化，另建独立新checkpoint | 194表/1879行、roles/owner/ACL/remote SCRAM、六份 version/bytes/hash/protection 独立恢复均已通过；发送前必须仍匹配当前源。副本隔离、不开业务、保留；失败0 DELETE |
| 3 | READ ONLY复核原两attempt已NOT_SENT/released；另行追加新 execution stage basis 与≤40分钟 current deletion approval | 原closure不重复写入；原attempt永不发送；新principal→workspace、basis/approval指向同一exact plan，旧non-delete批准不得复用 |
| 4 | 单独 provision 临时 delete executor 与只读 inspector/recovery 权限，建立 exact IAM + gateway 双重 allowlist | delete 权限仅两份 key+version，无 root payload mount、Put/Copy、bare key、bypass 或受保护/输出 DELETE；business windows 不重开；禁止 SDK/proxy 自动重发 |
| 5 | 在共享 fence/固定 parent-first 下 fresh authority/approval、全 inventory、native protection、qualified FK/reference releases、pending copies、pins/owner quiescence、ledger/revision；DB-first dispatch/start，再在 ClaimExactSend 重验 | 每项都是新 dispatch/attempt/request key；每次 dispatch 与 send 都 fresh gate；任何 UNKNOWN/DENIED、过期或 late hold 都 0 DELETE，保留阻断事实；不引用旧 eligible evaluation 授权 |
| 6 | 依次处理两份目标，每份最多一个 exact DELETE；复用 Caddy upstream-success/downstream-abort，并在原 executor observation 持久化后真实进程退出，制造 response loss/crash | 总 DELETE ≤2，非目标 DELETE=0；调用前 start/send claim，真实调用无论 outcome 都有 typed CostAllocation；CONFIRMED invocation + UNKNOWN outcome 追加保存，不能覆盖或盲重发 |
| 7 | 复用真实 Outbox→Redis/Asynq→只读 worker，单 owner reconcile 原新 attempts，读取完整 version inventory/权限/水位并追加 exact absence receipts | 200/204、delete marker、bare HEAD404 不能算 absence；第一项最多 1 receipt/0 completion，第二项才可 2 receipts/1 exact-scope completion；UNKNOWN/PARTIAL 诚实保留；worker 不持 DELETE 权限 |
| 8 | terminal replay/并发 terminal worker replay；独立 READ ONLY SQL 与 native survivor readback，封存新 IAM/permission/gateway | terminal replay 新 provider I/O/DELETE=0；四份 survivor identity/bytes/protection不变、old business fingerprints不变、active=0；每次实际 call/observation/cost 一一对应。资源全部保留，失败也不 cleanup |

步骤 5–7 按目标串行完成：先 online/a 的 fresh gate→单次 send→只读 recovery→absence
receipt，再处理 quarantine/b。初始 native inventory 必须等于六份 registry；第一项 absence
被可信 receipt 证明后，第二项 fresh gate 的 present inventory 为五份，最后为四份。
只有本 plan 的 exact absence receipt 可以解释缺失，不能因 HEAD404 或试验批准把缺失条目
从 expected set 中删掉。复用现有 delete dependency 的 receipt-backed inventory closure；
不能直接沿用非删除 driver 的“永远六份 present”假设。
新 profile已区分“可信 NOT_SENT/released 的旧trial”和“UNKNOWN/可能发送”的attempt；
任何non-trial新attempt存在时，sender停止，复用只读recovery，不盲重发。

当前 backup资源实际已保留：`dpp-lifecycle307-5d2274ee-predelete-objects`、
`dpp-lifecycle307-5d2274ee-predelete-postgres-02` 及专属同名卷，精确ID见
[新阶段回执](issue307-d1-reference-preparation.md)。原generation01 replica/network/helper同样保留。
未来发送辅助资源仍未创建：`dpp-lifecycle307-5d2274ee-reference-delete-gateway`、
`dpp-lifecycle307-5d2274ee-reference-send-a/b`、
`dpp-lifecycle307-5d2274ee-delete-recovery-redis` / 同名加 `-data` 的专属卷。

Redis 使用本机已存在 image ID
`sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499`，
不凭 mutable tag 选镜像。一次性 backup/admin/executor/read-only worker helpers
均归本方案、无 host ports，并保留。create 前后复用 `Assert-RetainedServiceStorage`；
本方案不允许重初始化原 Core、down/prune/remove 或开启旧业务窗口。

## 结果口径与待确认文本

初始 proposal-only阶段 provider I/O/Core mutation=0；本轮明确非删除阶段新增60个provider读取
（源40/独立副本20）、2NOT_SENT+2release、schema68及维护history/cost；DELETE=0。
新generation02 Core与六版本副本全部STOPPED_RETAINED，maintenance authority封存，原source服务未restart。
初始 proposal-only READ ONLY query 用了不存在的 pending table，事务失败无 mutation；
检查实际 schema 后改为 copy operation/output intent-ready 查询成功，
[operator diagnostic](evidence/issue307-d1/delete-proposal-5d2274ee/operator-readback-diagnostic.json) 保留。
这不计为 native 业务失败，也不隐藏原失败。

上述执行即使全部通过，结论也仅为“两份 source exact absence 与该范围 completion”。
已有非删除 native LC-11/12 证据绑定 source4650c435，不能把封存环境中的新 DELETE
冒充再次执行了活动 Processing/DirectData 双向竞争。
native pre-ISSUED pause、剩余 in-flight worker fault combinations、archive/restore rerun、
完整 D1 仍分别验收，#307 不因此自动关闭；#318 不 Ready/merge/deploy。

供后续明确确认的文本（**本轮尚未收到**）：

> CONFIRM_DELETE_EXACT_TWO；execution 614cd9ad-3f9d-40be-b110-3f7cf95f0790。
> 确认 project dpp-lifecycle307-5d2274ee、provider 1980751b-c50e-4895-93a8-cb55960cd635、
> incarnation be784eda-96e4-49a9-ae47-eaa3203a2dc5，plan 8f0e9340-18f0-4a73-bdd6-622196a9011a、
> hash d7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc，
> 仅 dpp-lifecycle307-online/fixture/a.csv@312365a9-f984-43de-b9d5-7e26009ccd82 与
> dpp-lifecycle307-quarantine/fixture/b.csv@846d0c7c-289d-4667-bc7c-aa04e44357d4 两份源版本，
> 绑定 source4af0ee88、Linux binary e1d9d35e48210ffe4400edb3476bdc8a51cb71cab9bd05686aedbc791a0a5b30
> 及 proposal-v2/profile所列hash；在所有执行前条件通过后，每项最多一次exact DELETE，
> ≤40分钟窗口，并执行本方案只读对账/封存。
> 四份 survivor、全部其他环境和备份保留；cleanup=0、hold-release=0、bypass=0；#318仍Draft、不合并。

本段是待确认范围的模板，不是批准事实；“确认方案但暂不执行”也不等于允许发送 DELETE。
