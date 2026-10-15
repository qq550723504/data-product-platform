# #307 D1：5d2274ee 环境的 exact-version 删除确认方案

2026-10-15（Asia/Singapore），状态 **PENDING_USER_CONFIRMATION / 实际 DELETE NOT_RUN**。
本方案依据 #307 的 exact-list approval gate、lifecycle 主 contract 与
[有限引用需求 contract](issue307-reference-demand-contract.md)。#318 保持 Draft，
既有非删除备份/恢复和 no-send 事务保留；当前v9已完成新只读核验及独立评审，不启用发送、不发 DELETE、不解除保护。
此方案替代“下一次执行”的旧 e1487ce7 提案；旧提案及其他环境的已消费批准保留原历史。

当前状态见[原生非删除准备](issue307-d1-reference-preparation.md)：六份 exact-version
原生备份/独立恢复、两条原 trial 的原子 NOT_SENT/release、schema68、194表/1879行的新 Core
checkpoint/独立恢复/SCRAM、关闭的发送组合均已验证。native IAM provision/activation、
新短期 deletion basis/approval、真实发送和当前环境队列 recovery 仍未执行。删除确认仍未收到。
旧方案机器清单、旧 Core1377行回执和失败资源保留为历史，不覆盖。

LC-13新增[可信停止/新Retry/credential containment非删除矩阵](issue307-lc13-continuation.md)：
f3/7cd/812三个独立run均已封存且资源保留，全部排除在本删除范围外。
本轮[v9只读核验矩阵](issue307-d1-candidate04-readonly.md#v9基于v8的新原生只读观察)刷新5d六份库存/bytes/hash/保护及执行前条件；
源PG/MinIO仅为读取短暂启动后恢复stopped，gateway始终停止，未重开业务或发送窗口。
保护新观察为2026-10-15 11:01:24.823～11:01:25.065 +08:00；精确UTC范围
`2026-10-15T03:01:24.823375493Z`～`2026-10-15T03:01:25.064935255Z`，未来每项发送前须fresh复核。
当前代码依赖更新为minio-go/v7 v7.0.88及MinIO policy v3.1.3；e1d9/4af0与v7.0.83
仍是原准备binary的真实历史。本轮沿用已冻结source055aa1df及candidate04 Linux binary/profile/IAM/gateway，没有重新构建；
新LC13/native PASS不授权复用旧批准或发送，不代表真实DELETE recovery验收。

当前机器清单：[proposal-v9](evidence/issue307-d1/delete-readonly-v8-refresh01/proposal-v9.json)，
继承已独立评审通过的[v8](evidence/issue307-d1/delete-readonly-v7-refresh01/proposal-v8.json)，仅更新audit-core-08
只读回执、观察时间和由观察时间计算的retention状态；candidate04、scope、budget和关闭authority逐项保持相同。
v6/v7/v8/index及其独立评审保持原字节；此前v7原生读取与主线程一致性检查之后，
独立reviewer已对HEAD `e122a613322fba6009ec8175539e9eb9e3e7f05e` 的精确方案、离线校验、
新回执及operator修复给出PASS，当前切片有效blocker=0。
[独立评审矩阵](issue307-d1-candidate04-readonly.md#v7独立评审与收敛检查)及
[单独回执](evidence/issue307-d1/delete-proposal-v7-review01/independent-review.json)保留该评审时点；
前轮v8原生只读与主线程检查之后，独立reviewer对HEAD `a29bfe6019cd5b38415fd378394fcd537662525c`
的精确方案、离线校验、当时新回执及固定07代operator扩展给出PASS，当时切片有效blocker=0。
前轮修正一处operator管理GET文字计数，评审未改operator/运行源码。
[v8评审矩阵](issue307-d1-candidate04-readonly.md#v8独立评审与收敛检查)及
[单独回执](evidence/issue307-d1/delete-proposal-v8-review01/independent-review.json)不覆盖原native verification/index，
保留原10:14:11保护观察。随后独立reviewer对HEAD `d01c6cdc5918e04eb0981b4bff2a152bb72ee0a6`
的v9精确方案、离线校验、08代新只读回执及固定08代operator扩展给出PASS，当前切片有效blocker=0，
无需operator/运行源码或事实文字修复。[v9评审矩阵](issue307-d1-candidate04-readonly.md#v9独立评审与收敛检查)及
[单独回执](evidence/issue307-d1/delete-proposal-v9-review01/independent-review.json)记录本次结论；
原v9 native verification/index仍保留采集时的主线程验收口径与11:01:24～25保护观察，不回写历史、不增加删除权限。
v6修正v5继承的audit-core-03/checkpoint旧字段冲突，统一为audit-core-04证据。
[v5](evidence/issue307-d1/delete-readonly-candidate04/proposal-v5.json)及其index原字节保留，
不再作为下一次执行的当前清单；[proposal-v4](evidence/issue307-d1/lc13-continuation01/proposal-v4.json)保留原候选冻结时点。
本轮20次原生对象读取仅记入新audit-core-08；本轮4次operator IAM GET另列回执，前驱v8的4次及v7的8次保持各自原历史，
源Core/旧restore mutation与DELETE均0。audit-core-05初始化失败及其资源保留；没有重写或重跑旧trial。
当前离线校验为`python examples/lifecycle-d1/check-reference-v9.py`；
原`check-reference-proposal.py`、`check-reference-refresh.py`及`check-reference-v8.py`继续核对固定历史v6/v7/v8。
PASS只表示公开证据与当前提案一致，不能生成批准或替代fresh gate。
跨平台检查仅对317项运行源码按固定Git source的LF文本摘要比较；原始候选binding仍保留，
binary/profile/IAM/gateway摘要不做换行转换。[前轮主线程收口](evidence/issue307-d1/delete-proposal-v6/closure.json)
记录当时11项正常及-O测试、22个拒绝案例、40个容器继续停止及原备份hash不变。
v6前轮独立评审发现并修正两类离线一致性阻断：未消费的提案字段仍可得到PASS，以及LF清单
未向原始冻结源码摘要证明文本等价。checker现在先校验现有v6 index的三份完整artifact原字节，
再要求输入等于固定v6；317项源码同时向原frozen raw hash证明LF/CRLF等价。
Git已评审仓库/index是此离线检查的完整性边界；该index不是签名或删除授权，
任何修改固定提案/index的请求都须新增提案版本并重新评审，不能就地改旧证据。
[v6复评矩阵](issue307-d1-candidate04-readonly.md#v6独立评审与修复验收)保留当时结论；
原v6/v7/v8 proposal、LF清单、closure/index和candidate04均保持原字节；新观察只写v9新目录。
当前[候选绑定](evidence/issue307-d1/lc13-continuation01/delete-candidate04/source-binding.json)和
[关闭profile](evidence/issue307-d1/lc13-continuation01/delete-candidate04/bounded-profile.json)
由新binary在两个network=none、root-free、read-only helper中渲染/验证关闭send gate；
provider I/O/Core mutation/DELETE均0。两helper已退出并保留；没有native IAM provision、批准、
delete credential、enabled permission或gateway。delete/archive命令6项非删除回归PASS。
公开[exact IAM原始字节](evidence/issue307-d1/lc13-continuation01/delete-candidate04/delete-executor-policy.json)
690 bytes/SHA256与绑定一致；另存展示JSON，不把格式化后的字节当作执行policy hash。
旧[proposal-v3.json](evidence/issue307-d1/lc13-readonly01/proposal-v3.json)保留原历史来源，
其旧binary确认模板不能作为新发送授权。源码/binary/profile变化时必须另行冻结并重新确认。
前轮[5d只读核验](issue307-lc13-readonly.md)于 `2026-10-14T09:34Z` 更新六份原生观测：
六version/bytes/hash/保护与原after一致；源Core194表/1879行前后指纹相等，源journal仍92。
20个新增维护调用只写新auditCore，总112 calls/observations/typed成本；原PG/MinIO/gateway
最终全部stopped，业务/deletion窗口仍关闭。原tar/dump字节hash未变；另保留新只读导出恢复副本。
旧 [proposal-v2.json](evidence/issue307-d1/reference-disposal-preparation01/proposal-v2.json) 保留为历史。
原清单：[proposal.json](evidence/issue307-d1/delete-proposal-5d2274ee/proposal.json)，仅记录初始准备时点。
当前宿主只读元数据：[topology](evidence/issue307-d1/delete-readonly-v8-refresh01/retained-topology.json)；
[当前 Core readback](evidence/issue307-d1/delete-readonly-v8-refresh01/5d-08/current-core-readback.json) /
[READ ONLY SQL](evidence/issue307-d1/delete-readonly-v8-refresh01/5d-08/current-preconditions-readonly.sql)。
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
| 当前 Schema / 候选源码 | 68 / `055aa1df063e560c6796e92dfff4e03c7b9061e0`；候选运行源码未变，辅助只读driver/证据单列 |
| 历史5d非删除原生源码 | `4af0ee88db92dfbe7aad029b22916bb046155e27`，原e1d9 binary/SDK7.0.83只绑定该准备阶段 |
| 原历史 native/source | `4650c435f8ba89fdb0b2dc0591152d25ded182d7`；原 LC-11/12 不冒充新源码重跑 |
| 当前 Linux executor binary SHA256 | `01bff6e2a242a202e73519d7b5a4f1fc109430614b25a5189eacfb0ebb2cddf3`；go1.26.5/linux-amd64/CGO=0/trimpath；发送关闭 |
| 当前 profile SHA256 / target digest | `97d565f9c66d8cfcb66997d034e7acb2d1519864898950a9ddb2ced2cdd5c4ee` / `4cbb36e5aebbc9451603efd276aa309a2c4afdefcf252107571b0f8334360a38` |
| 当前 IAM / active / sealed gateway SHA256 | `ab24ab862bf0e21d23ee21989f4140b3c7200cdecee22d4a708cc45621da8b54` / `7b24ad32506bfd189c9289ba85a65ab242069dfee293d61b330104a90306cd0a` / `6a08b8647cc63cb389e2a511a5ad944b82d14e1ec9721f1a5ae45cb984bed4f6` |
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

三项服务在原方案只读核对时 running，身份、镜像、专属卷和网络与原回执匹配。
2026-10-14追加[运行时核对](evidence/issue307-d1/native-completion01/runtime-incident.json)：
Docker Desktop停止/恢复后，这三个相同ID服务为 **stopped/retained**。
本轮仅PG/MinIO为只读核验短暂启动，gateway一直停止；核验后均恢复stopped。
旧业务配置仍封存，源六版本保护状态已由同一冻结候选于 `2026-10-15T03:01:24.823375493Z`～`03:01:25.064935255Z` 原生读取刷新。
执行前必须重新核验其持久卷/identity/Core、native protection/inventory和current gate。
新[非删除矩阵](issue307-d1-native-completion.md)的四个环境全部排除在本方案删除范围外；
其worker恢复证明不等于5d环境的真实DELETE recovery验收。
MinIO binary SHA256 `fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8`，
`DEVELOPMENT.GOGET` 固定受控基线；当前候选MinIO SDK `minio-go/v7 v7.0.88`、
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
[本轮源原生复查](evidence/issue307-d1/delete-readonly-v8-refresh01/5d-08/native-six-version.json)，
全部六份 identity/bytes/hash/protection 与 before 和隔离 restore 一致。具体保护观测时间逐项保存在
proposal-v9；两目标分别观测于 `2026-10-15T03:01:24.823375493Z` 和 `2026-10-15T03:01:25.064935255Z`，
它仍不替代未来发送时的 current protection。
执行前及每项发送前均须重新读取 native protection/inventory；新增 hold、未到期 retention、
UNKNOWN、身份漂移或 inventory 增减均使本提案停止，0 DELETE，不自动扩大范围。

## 明确保留的四份版本与影响

| Namespace / key | Exact provider version | Bytes | 保护状态与保留原因 |
| --- | --- | ---: | --- |
| online / `fixture/b.csv` | `159e90c3-adb6-4ea3-b2e8-2804e578e86f` | 129328 | native legal hold=true，retention=NONE |
| quarantine / `fixture/a.csv` | `5a7ce790-9093-4569-a8fd-0726009a5eb4` | 232 | legal hold=false；metadata仍GOVERNANCE，期限 `2026-10-15T00:58:47Z`（新加坡08:58:47）已过；仍排除删除 |
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

当前 READ ONLY Core snapshot：6 copies、23 requirements/releases、7 pins/terminations、
active=0、92 calls/observations/typed storage costs；2 NOT_SENT observations/2 dispatch releases，
send claims/absence/completion=0；源ledger本轮不变。既有非删除准备阶段的源40/恢复20调用保留原历史。
copy operations=0；2 processing output intents 对应 2 ready outputs。
FINALIZED 两份 plan 的 DB computed hash 与 stored hash 相同。

本轮额外核对27项完整FK trigger资格、54项目标reference counts、3个fence函数正文，
23份requirements/releases、7pins/terminations及原2NOT_SENT/linked releases仍闭合。
regular reader/producer原生状态disabled；拟建普通executor为权威NoSuchUser。
新audit-core-08恢复194表/1879行，并仅在该clone承接20个新的native checkpoint调用（总112），
源journal仍92，原Core业务mutation0；本轮4个operator IAM GET另列回执，此前各轮保留原历史。
源、新audit和helper已全部停止、maintenance authority封存；原tar/dump hash不变。
这些PASS均为只读snapshot；[逐项条件](issue307-d1-candidate04-readonly.md)明确列出
确认、新basis/approval、native IAM/active gateway/permission和fresh send topology仍未创建，
因此真实发送继续阻断，不能把本次核对视为授权。

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
| 2 | 保持 business reader/producer 封存，重核当前源、原generation02备份、当前audit-core-08只读恢复和六版本checkpoint；若源facts变化，另建新checkpoint | 当前源194表/1879行、roles/owner/ACL恢复hash一致；原六版本native备份恢复及本轮重读分别保留。发送前须仍匹配当前源，失败0 DELETE，不重开业务或旧恢复窗口 |
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

发送与恢复使用已有的两个独立入口：发送为`lifecycle-fixture-delete`的固定reference profile
下`send-a` / `send-b`；恢复为`lifecycle-fixture-recovery`，复用
`pilot.BuildRecoveryInspectorAt`、`RecoveryService`和现有Outbox/Asynq handler。
后者的配置不含delete/root credential，仅允许只读inspector。不能调用其他旧delete profile，
也不能用`inspect-source`充当发送后的恢复：它的六份payload读取只适用于删除前checkpoint。
确认后先绑定同一冻结源码构建的恢复binary、5d workspace/provider/incarnation、只读IAM、
新attempt/work identity和manifest hash，再按`outbox → worker → once terminal replay`验收；
恢复配置/Redis/worker资源当前尚未激活，当前5d真实队列恢复仍NOT_RUN。
上述入口闭合结论来自前轮主线程静态检查；v6/v7/v8独立评审覆盖各自精确方案、离线校验，
v8还核对当时07代只读回执和operator扩展；本次v9独立评审覆盖新08代只读回执及固定08扩展，
该非删除准备结论不延伸为5d真实发送/删除后队列恢复PASS。

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

初始 proposal-only阶段 provider I/O/Core mutation=0；既有非删除准备阶段新增60个provider读取
（源40/独立副本20）、2NOT_SENT+2release、schema68及维护history/cost；DELETE=0。
前轮generation02 Core与六版本副本全部STOPPED_RETAINED，maintenance authority封存。
本轮源PG/MinIO只读启动后恢复停止，新audit-core-08独立恢复194表/1879行并承接20次维护调用；
源Core业务mutation=0，旧备份/旧restore未写入，gateway始终stopped，六version均未删除。
初始 proposal-only READ ONLY query 用了不存在的 pending table，事务失败无 mutation；
检查实际 schema 后改为 copy operation/output intent-ready 查询成功，
[operator diagnostic](evidence/issue307-d1/delete-proposal-5d2274ee/operator-readback-diagnostic.json) 保留。
这不计为 native 业务失败，也不隐藏原失败。

上述执行即使全部通过，结论也仅为“两份 source exact absence 与该范围 completion”。
已有非删除 native LC-11/12 证据绑定 source4650c435，不能把封存环境中的新 DELETE
冒充再次执行了活动 Processing/DirectData 双向竞争。
native pre-ISSUED pause、worker fault combinations、archive/restore rerun 已有[前轮非删除证明](issue307-d1-native-completion.md)，
LC-13有限可信停止、新Retry与原生credential containment已有[本轮证明](issue307-lc13-continuation.md)。
其余实际DELETE竞争/故障、通用external owner/credential与完整D1仍分别验收，
#307不因此关闭；#318不Ready/merge/deploy。

供后续明确确认的文本（**本轮尚未收到**）：

> CONFIRM_DELETE_EXACT_TWO；execution 614cd9ad-3f9d-40be-b110-3f7cf95f0790。
> 确认 project dpp-lifecycle307-5d2274ee、provider 1980751b-c50e-4895-93a8-cb55960cd635、
> incarnation be784eda-96e4-49a9-ae47-eaa3203a2dc5，plan 8f0e9340-18f0-4a73-bdd6-622196a9011a、
> hash d7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc，
> 仅 dpp-lifecycle307-online/fixture/a.csv@312365a9-f984-43de-b9d5-7e26009ccd82 与
> dpp-lifecycle307-quarantine/fixture/b.csv@846d0c7c-289d-4667-bc7c-aa04e44357d4 两份源版本，
> 绑定 source055aa1df063e560c6796e92dfff4e03c7b9061e0，
> Linux binary 01bff6e2a242a202e73519d7b5a4f1fc109430614b25a5189eacfb0ebb2cddf3，
> proposal-v9/candidate04 profile 97d565f9c66d8cfcb66997d034e7acb2d1519864898950a9ddb2ced2cdd5c4ee，
> target digest 4cbb36e5aebbc9451603efd276aa309a2c4afdefcf252107571b0f8334360a38，
> IAM ab24ab862bf0e21d23ee21989f4140b3c7200cdecee22d4a708cc45621da8b54，
> active gateway 7b24ad32506bfd189c9289ba85a65ab242069dfee293d61b330104a90306cd0a，
> sealed gateway 6a08b8647cc63cb389e2a511a5ad944b82d14e1ec9721f1a5ae45cb984bed4f6；
> 在所有执行前条件通过后，每项最多一次exact DELETE，
> ≤40分钟窗口，并执行本方案只读对账/封存。
> 四份 survivor、全部其他环境和备份保留；cleanup=0、hold-release=0、bypass=0；#318仍Draft、不合并。

本段是待确认范围的模板，不是批准事实；“确认方案但暂不执行”也不等于允许发送 DELETE。
