# #307 D1：冻结候选04的5d删除前只读核验

最新观察见[v8基于v7的新原生只读观察](#v8基于v7的新原生只读观察)。
下文audit-core-04/06与v6/v7独立评审保留各自原时点；新回执另存audit-core-07，不覆盖旧证据。
v7的[独立评审](#v7独立评审与收敛检查)作为本次刷新基线；v8是新的原生只读及主线程离线验收，不新增独立评审结论。

2026-10-15（Asia/Singapore）。新候选源码055aa1df、Linux binary
`01bff6e2a242a202e73519d7b5a4f1fc109430614b25a5189eacfb0ebb2cddf3`
及profile/IAM/gateway hashes未变；317项运行源码匹配。
#318仍Draft；DELETE/merge/hold-release/bypass/cleanup均0。

## 库存与保护逐项核对

读取时间为2026-10-15 **00:22:28 +08:00**，即原生回执`2026-10-14T16:22:28Z`。
online4份、quarantine2份；无额外/缺失version或delete marker。
全部exact-version bytes/SHA256与registry、原六版本恢复回执一致；payload只落私有目录。
[原生回执](evidence/issue307-d1/delete-readonly-candidate04/retry02/5d-04/native-six-version.json)。

| 范围 | namespace / key | exact version | bytes | legal hold / retention | 结论 |
| --- | --- | --- | ---: | --- | --- |
| 目标A | online / fixture/a.csv | 312365a9-f984-43de-b9d5-7e26009ccd82 | 232 | false / NONE | 当前未保护；未来发送前重读 |
| 目标B | quarantine / fixture/b.csv | 846d0c7c-289d-4667-bc7c-aa04e44357d4 | 129328 | false / NONE | 当前未保护；未来发送前重读 |
| survivor | online / fixture/b.csv | 159e90c3-adb6-4ea3-b2e8-2804e578e86f | 129328 | true / NONE | 保留，禁止解除hold或删除 |
| survivor | quarantine / fixture/a.csv | 5a7ce790-9093-4569-a8fd-0726009a5eb4 | 232 | false / GOVERNANCE至2026-10-15 08:58:47 +08:00 | 保留；到期不扩范围 |
| survivor | online / fixture/standardized.csv | 2139f293-87f4-4d49-a9f0-941b22275093 | 494 | false / NONE | Entity输出，保留 |
| survivor | online / fixture/processed.csv | 7c8ad93b-31ee-445c-9402-816dda0410ec | 232 | false / NONE | processing输出，保留 |

online/quarantine分别为`dpp-lifecycle307-online` / `dpp-lifecycle307-quarantine`。
该结果是本次只读snapshot，不构成CurrentGate/send授权。

## 执行前条件

| 条件 | 本轮结果 | 仍需完成的门禁 |
| --- | --- | --- |
| source/provider/incarnation/容器/专属卷/internal网络 | 相同IDs、0 host ports；最终全部stopped | 激活前重核当前拓扑 |
| exact plan/membership/hash | schema68、FINALIZED目标2份；DB computed/stored hash相等 | 不复用旧eligible evaluation授权 |
| Core/owner/ACL/备份 | 194表/1879行before=after；新audit恢复一致；原tar/dump SHA不变 | 如facts变化另建检查点；旧恢复窗口不开启 |
| reference资格 | 27项完整trigger资格；3个fence函数正文匹配migration67 | dispatch/send在共享fence内重新核对 |
| material owner closure | 两目标54项FK检查unreleased=0；23份exact-copy requirements/releases保留 | 新引用/UNKNOWN立即阻断 |
| reader/pending-copy | 7pins/7terminations、active0；2output intents各有ready、copy operations0 | lease/status不能代替可信停止；发送前再核对 |
| 原两个trial | 原start/dispatch不变；2NOT_SENT（invoked_at=null）与2linked releases | 不重发、不重生成proof、不改旧basis |
| 原生IAM/窗口 | regular reader/producer disabled；inspector enabled只读；拟建普通executor NoSuchUser；源configs/gateway封存 | 新executor IAM、bounded gateway、permission尚未provision/启用 |
| 人类精确确认/614cd9ad新批准 | NOT_RECEIVED；只有旧non-delete expired approval | 真实删除仍阻断；新basis/current approval≤40分钟 |
| host topology/ClaimExactSend | 新激活组合尚未创建 | 确认后绑定精确组合并fresh验签；每项send前重验全部门禁 |
| 真实DELETE/只读worker对账 | NOT_RUN | 每目标最多一次；unknown不重发；receipt解释6→5→4 inventory |

详细[条件机器清单](evidence/issue307-d1/delete-readonly-candidate04/execution-preconditions.json)、
[Core读取](evidence/issue307-d1/delete-readonly-candidate04/retry02/5d-04/current-core-readback.json)、
[完整reference资格](evidence/issue307-d1/delete-readonly-candidate04/retry02/5d-04/reference-qualification-readback.json)、
[native IAM](evidence/issue307-d1/delete-readonly-candidate04/retry02/5d-04/native-iam-readonly.json)。
原生IAM探针复用madmin SDK，仅允许4个固定角色的GET user-info，拒绝其他HTTP方法/路径和重发；
Root仅在独立management helper中，不进入payload helper，不挂Source PG credentials。
NoSuchUser只证明拟建普通IAM用户不存在；不冒充对象absence或全域credential销毁。

## 账务、错误与保留

源Core业务mutation0、journal仍92。全新audit-core-04独立恢复后记录20个checkpoint读取，
calls/observations/typed成本总112，各invoked call恰一份allocation，unaccounted0。
[账务](evidence/issue307-d1/delete-readonly-candidate04/retry02/5d-04/audit-costs.json)。
8个管理GET（包含失败探针4个）分别保留operator回执，不冒充storage/delivery业务attempt或其成本。
旧tar/dump、旧audit/restore数据库均未写入或重开；新dump/restore/私有payload/全部40个容器保留。
新maintenance配置已封存；源PG/MinIO/gateway和新audit/helpers全部退出、PID0、cleanup0。

第一次提取器误解`$$ LANGUAGE plpgsql;`，在native I/O前停止；第二次探针误把regular UserInfo的
disabled/enabled与service-account on/off混用，真实回执已满足目标状态，仍按错误退出。
两次都恢复原停止状态、未创建audit Core、未产生storage调用；修复均为operator解析根因，
未改数据库或native状态。两份[诊断](evidence/issue307-d1/delete-readonly-candidate04/5d-04/operator-diagnostic.json)
和[状态诊断](evidence/issue307-d1/delete-readonly-candidate04/retry01/5d-04/operator-diagnostic.json)保留。
最后独立目录重跑完整通过；Python30项正常及-O PASS。

[当时proposal-v5](evidence/issue307-d1/delete-readonly-candidate04/proposal-v5.json)
保留本次观察历史；其继承字段中仍有audit-core-03/checkpoint旧引用。
当前[proposal-v6](evidence/issue307-d1/delete-proposal-v6/proposal.json)统一为上述audit-core-04证据，
新增离线一致性检查；原source/binary/profile/plan/两目标和原生观察时间不变，没有重跑provider读取。
[精确确认方案](issue307-d1-delete-proposal.md)仍待明确确认；真实删除、完整D1/D2未验收。

## v6前轮主线程离线一致性收口

本次未启动服务、未刷新原生保护snapshot。主线程核对发送/恢复分别复用既有
`lifecycle-fixture-delete` / `lifecycle-fixture-recovery`，没有新增runtime；
当前5d的真实Outbox/Asynq恢复仍待单独执行验收。

| 检查 | 结果 |
| --- | --- |
| 当前机器清单 | v6仅保留一套当前只读证据，7份回执固定为audit-core-04代次及SHA256；v5/index原字节保留 |
| 候选与范围 | source055aa1df、原binary/profile/IAM/gateway、两target/四survivor、129560 bytes均不变 |
| 源码跨平台绑定 | 317项源码对固定Git source取LF摘要；44份原始字节相同、273份仅CRLF差异；binary/profile/IAM/gateway仍按原始字节核对 |
| 离线拒绝回归 | 每种Python模式11项测试通过，含22种旧代次/错误identity/version/protection/超预算/伪造授权拒绝案例；重复JSON key/path escape及源码变更拒绝，LF checkout通过 |
| 停止状态与备份 | 40个相同label容器exited/PID0、0 host ports；原tar/dump SHA不变，business/delete/maintenance配置仍封存 |
| 本次副作用 | provider I/O/Core mutation/DELETE/merge/cleanup均0；只完成离线检查及宿主只读回读 |

[收口回执](evidence/issue307-d1/delete-proposal-v6/closure.json)记录本次时间、测试与停止状态。
这是主线程一致性审计，不新增独立评审或原生业务PASS；没有改变待确认与fresh gate条件。

## v6独立评审与修复验收

独立reviewer首先评审HEAD `7792421cb9d82072ff3db9afa95ee3a4151bca6d`，
只读取代码/公开证据并运行非删除Python测试与内存反例；未启动环境、访问provider/DB或改IAM。
按AGENTS §17，只有下面两类当前离线一致性缺陷重开本切片，不增加通用信任/审批服务。

| 条目 | 评审结论 / 修正 | 验收状态 |
| --- | --- | --- |
| V6-R1：完整提案一致性 | P2 BLOCKER；network/inventory plan/source ID、cleanup/重发/顺序、新批准要求、旧trial/备份等字段被改仍PASS | 已增加固定v6 index的三份artifact完整字节/精确membership核对；输入须等于已索引v6，新增/未消费字段变化同样拒绝；12种完整提案反例和index破坏拒绝回归PASS |
| V6-R2：源码冻结锚点 | P2 BLOCKER；同时改源码、LF清单和清单ref可自称原source未变 | 当前317份源码各自与原frozen raw digest证明LF/CRLF等价；协同改动反例即使绕过外层index，内层冻结校验也拒绝；LF checkout仍PASS |
| 分层回归 | 完整性核对与事实语义核对均需验收 | 原22种语义反例单独验证内层事实检查，避免仅靠固定提案比较掩盖语义回归；checker共7项、既有readonly7项，正常及-O各14项PASS |
| 历史与候选 | 原v6/index、原37份readonly证据、source055aa1df/binary/profile/IAM/gateway/两目标不变 | 只新增review01回执，不覆盖旧closure或原始失败历史 |
| 独立复评 | reviewer对修复代码/测试/精确方案及矩阵草稿按SHA256绑定复核 | PASS；正常/-O各自独立跑checker7项和CLI，重构10个原反例全部拒绝，核对49个原tracked证据文件原字节未变；本切片有效blocker=0 |
| 5d真实验收 | 新原生观察/发送/Outbox-Asynq recovery仍未执行 | NOT_RUN；保护snapshot仍2026-10-14T16:22:28Z，每次实际发送前fresh gate必需 |
| 执行权限 | 明确确认、新短期basis/approval、native IAM/active gateway/permission、fresh topology仍未具备 | BLOCKED；#307 OPEN、#318 Draft，不合并、不真实删除 |

本轮只修离线校验和证据口径，provider I/O/Core mutation/DELETE/cleanup均0。
完整D1/D2、其他真实删除故障/竞争与通用IAM/owner能力继续由#307分别验收。

[初评](evidence/issue307-d1/delete-proposal-v6-review01/initial-review.json)、
[独立复评](evidence/issue307-d1/delete-proposal-v6-review01/independent-re-review.json)、
[主线程前后反例及14项两模式回归](evidence/issue307-d1/delete-proposal-v6-review01/regression.json)和
[宿主只读回读](evidence/issue307-d1/delete-proposal-v6-review01/retained-host-readback.json)分别保存。
40个保留容器仍exited/PID0、无host ports，原tar/dump hash不变；这些宿主核对没有刷新原生保护snapshot。

## v7基于v6的新原生只读观察

2026-10-15 **09:31:22 +08:00**（UTC `2026-10-15T01:31:22Z`）完成新读取。
本轮沿用已独立复评通过的v6及candidate04 binary；仅新增固定audit代次和v7观察版提案。
v6 checker、其测试、原proposal/index/独立复评均未改；本次原生观察与operator改动先由主线程验收，
其原始verification保留当时尚无新独立评审的事实；后续独立评审另列下节。
这些检查不新增真实DELETE或实际删除后的Outbox/Asynq recovery PASS。

| 范围 | namespace / key | exact version | bytes | 本次原生保护 |
| --- | --- | --- | ---: | --- |
| 目标A | online / fixture/a.csv | 312365a9-f984-43de-b9d5-7e26009ccd82 | 232 | hold=false，NONE |
| 目标B | quarantine / fixture/b.csv | 846d0c7c-289d-4667-bc7c-aa04e44357d4 | 129328 | hold=false，NONE |
| 保留 | online / fixture/b.csv | 159e90c3-adb6-4ea3-b2e8-2804e578e86f | 129328 | hold=true，NONE |
| 保留 | quarantine / fixture/a.csv | 5a7ce790-9093-4569-a8fd-0726009a5eb4 | 232 | hold=false；metadata为GOVERNANCE，retain-until=2026-10-15T00:58:47Z，观察时已到期，仍排除删除 |
| 保留 | online / fixture/standardized.csv | 2139f293-87f4-4d49-a9f0-941b22275093 | 494 | hold=false，NONE；Entity输出 |
| 保留 | online / fixture/processed.csv | 7c8ad93b-31ee-445c-9402-816dda0410ec | 232 | hold=false，NONE；processing输出 |

namespace仍分别是`dpp-lifecycle307-online`、`dpp-lifecycle307-quarantine`。
[六版本原生回执](evidence/issue307-d1/delete-readonly-v6-refresh02/5d-06/native-six-version.json)
逐项保存精确SHA256、bytes和纳秒级观测时间；online4/quarantine2，无缺失、额外version或delete marker。
全部内容identity/bytes/hash和原备份恢复一致。retention到期是时钟结果，没有执行release或扩范围。

| 执行前条件 | 新回读结果 | 执行边界 |
| --- | --- | --- |
| environment/provider/incarnation/卷/网络 | 同一源IDs、internal网络、0 host ports；全部资源最终exited/PID0 | 激活前重新核对，不重初始化源/旧restore |
| source Core/role/owner/ACL | schema68，194表/1879行；before=after，新audit-core-06恢复一致 | 原源数据/业务mutation0，旧tar/dump SHA不变 |
| exact plan/owner/reference | 2 FINALIZED plan hash不变；27项qualified FK guards、54项target checks、3个fence函数正文与原事实一致；23 releases闭合 | snapshot不替代共享fence下fresh gate |
| pins/pending outputs/旧trial | 7pins/7terminations、active0；2intents各有ready；原2NOT_SENT/2linked releases保留，无send claims/absence/completion | 不改旧basis/start，不重生成proof、不重发旧trial |
| 原生IAM | reader/producer disabled，inspector enabled，拟建普通executor权威NoSuchUser；每次探针固定4 GET | 不证明所有credential类型已销毁；未provision/启用删除用户 |
| source/audit调用账 | 源仍92；新clone112 calls/observations/typed costs，新增20读取、unaccounted0 | 8次management GET（失败05四次+成功06四次）另记operator回执 |
| 明确确认/新批准/发送组合 | NOT_RECEIVED；614cd9ad新basis/approval、native IAM/active gateway/permission、fresh signed topology仍未创建 | BLOCKS_SEND；≤40分钟及每项fresh gate仍必需 |
| 原生DELETE/队列恢复/完整D1 | NOT_RUN / NOT_ACCEPTED | #307 OPEN、#318 Draft，不合并、不真实删除 |

首次audit-core-05的Unix就绪探针命中了官方PostgreSQL镜像的临时初始化服务器，随后socket消失，
globals恢复命令失败；此时仅完成4次IAM GET，storage调用0，源Core mutation0。
已核对固定镜像entrypoint的临时服务器使用`listen_addresses=''`，改用`127.0.0.1:5432`就绪检查，
仅在最终服务器可用后恢复；增加回归验证TCP探针。失败05全部文件/部分audit及卷保留并停止，
另建06后完整通过，没有重新初始化失败库、源库或复用旧restore。
[失败诊断](evidence/issue307-d1/delete-readonly-v6-refresh01/operator-diagnostic.json)。

当前[proposal-v7](evidence/issue307-d1/delete-readonly-v6-refresh02/proposal-v7.json)
完整继承v6的环境/scope/budget/关闭authority，仅替换新观察与回执引用；
`check-reference-refresh.py`先核对固定v7全部19份索引原字节，再与已审v6及新原生事实逐项比较。
原v6整份提案检查及317源码冻结锚点继续运行；保护期限已过也不会让survivor成为target。
正常和-O模式各20项通过（v6 7项、只读driver8项、v7 5项）。
[验证记录](evidence/issue307-d1/delete-readonly-v6-refresh02/verification.json)、
[执行前条件](evidence/issue307-d1/delete-readonly-v6-refresh02/execution-preconditions.json)及
[最终资源状态](evidence/issue307-d1/delete-readonly-v6-refresh02/retained-topology.json)分别保存。
共45个同label容器保留、全部exited/PID0、0 host ports；新maintenance权限已封存，DELETE/cleanup/hold-release/bypass均0。

## v7独立评审与收敛检查

独立reviewer对HEAD `e122a613322fba6009ec8175539e9eb9e3e7f05e` 完成只读评审，
范围为v7精确方案、离线校验、失败05/成功06新回执及operator修复；不启动服务或访问provider/DB。
结论 **PASS，当前切片有效blocker=0**；本轮无需追加代码修复。
按AGENTS §17冻结此有限contract：已评审Git/index提供离线完整性边界，
两目标/四保留版本、候选/source/hash、预算、关闭authority与原始历史须一致；snapshot不授予发送权限。

| 条目 | 独立复核与验收结果 |
| --- | --- |
| 精确范围与保护 | v7保持v6的环境、两目标共129560 bytes、每目标一次/总计两次预算及关闭authority；survivor retention到期未扩大范围 |
| 离线回归 | 正常及-O各20项、v7 CLI两模式均PASS；包含固定artifact原字节/membership、scope/authority变更拒绝、内层事实检查及TCP readiness |
| 新回执与源码绑定 | refresh01/02共30份indexed payload的工作区/HEAD原字节、长度、SHA256一致；4项operator源码摘要及candidate driver/base-head绑定一致 |
| 观察时间与恢复 | 六版本保护/inventory时间均位于source before/after之间；194表/1879行及role/ACL一致；失败05源码/诊断保留，独立06成功，无旧历史覆盖 |
| operator根因修复 | TCP探针避开仅Unix socket的临时bootstrap；源PG/MinIO恢复各自独立，maintenance seal失败不会跳过恢复 |
| 宿主只读回读 | 主线程另核同45个容器均exited/PID0、无host ports，原tar/dump SHA及封存配置一致；未刷新native保护观察时间 |
| 当前切片阻断 | 0；不把未来通用IAM/owner/DR或真实删除恢复扩为本次离线评审必需项 |
| 真实执行门禁 | 明确确认仍NOT_RECEIVED；新basis/approval、删除IAM/active gateway/permission及fresh topology未创建；原生DELETE/删除后恢复、完整D1/D2仍NOT_RUN / NOT_ACCEPTED，继续由#307验收 |

[独立评审回执](evidence/issue307-d1/delete-proposal-v7-review01/independent-review.json)与
[主线程完整性及宿主回读](evidence/issue307-d1/delete-proposal-v7-review01/primary-audit.json)另存新目录；
原v6/v7 proposal、index、verification及全部native回执原字节保留。
本轮provider I/O、Core mutation、容器启动、IAM变更、DELETE、merge和cleanup均0；#318保持Draft。

## v8基于v7的新原生只读观察

2026-10-15 **10:14:11 +08:00**（UTC `2026-10-15T02:14:11Z`）完成新读取。
基线为v7独立评审已通过的HEAD `e122a613`、文档收口HEAD `4ee5f1a6`；
candidate04/source055aa1df、317项运行源码、binary/profile/IAM/gateway及精确范围均未变。
本轮仅为现有只读driver增加固定07代，复用TCP readiness、Core collector及受限MinIO/madmin读取；
不增加runtime，不复用06或旧restore，不启用发送。operator实际源码摘要和工作树状态另记新verification。
v6 checker不变；共用refresh checker增加固定07代与v7前驱校验，原v7调用及原字节index仍通过。

| 范围 | namespace / key | exact version | bytes | 本次原生保护 |
| --- | --- | --- | ---: | --- |
| 目标A | online / fixture/a.csv | 312365a9-f984-43de-b9d5-7e26009ccd82 | 232 | hold=false，NONE，retain-until=null |
| 目标B | quarantine / fixture/b.csv | 846d0c7c-289d-4667-bc7c-aa04e44357d4 | 129328 | hold=false，NONE，retain-until=null |
| 保留 | online / fixture/b.csv | 159e90c3-adb6-4ea3-b2e8-2804e578e86f | 129328 | hold=true，NONE |
| 保留 | quarantine / fixture/a.csv | 5a7ce790-9093-4569-a8fd-0726009a5eb4 | 232 | hold=false；metadata为GOVERNANCE，retain-until=2026-10-15T00:58:47Z，已到期，仍禁止纳入删除范围 |
| 保留 | online / fixture/standardized.csv | 2139f293-87f4-4d49-a9f0-941b22275093 | 494 | hold=false，NONE；Entity输出 |
| 保留 | online / fixture/processed.csv | 7c8ad93b-31ee-445c-9402-816dda0410ec | 232 | hold=false，NONE；processing输出 |

namespace仍分别为`dpp-lifecycle307-online`、`dpp-lifecycle307-quarantine`。
[六版本回执](evidence/issue307-d1/delete-readonly-v7-refresh01/5d-07/native-six-version.json)保存逐项SHA256与纳秒级观测时间；
online4/quarantine2，无缺失、额外version或delete marker。6份bytes/hash与registry、原六版本恢复及v7一致。
6次保护和2次inventory观测均位于source before/after之间；2目标共129560 bytes，scope和预算不扩大。

| 执行前条件 | 本次核对结果 | 发送前仍需满足 |
| --- | --- | --- |
| 环境/instance/incarnation/卷/internal网络 | 原45容器IDs/images/停止状态与v7相同；新增07 audit/IAM/native helper；最终48个均exited/PID0/无host ports | 激活前重新核对精确拓扑，不重初始化源或旧restore |
| 候选/source/profile/IAM/gateway | 冻结hash全部相同，无binary rebuild | 实际发送必须绑定同一exact composition |
| 源Core/新独立恢复 | schema68、194表/1879行；before=after=07 restored=v7；完整relation与role/owner/ACL指纹一致 | facts变化时另建checkpoint；snapshot不替代current gate |
| exact plans/owner/reference | 2 FINALIZED目标与原plan/hash不变；27项完整FK资格、54项target检查unreleased0、3个函数正文匹配migration67；23 requirements/releases闭合 | dispatch/send在共享fence内重新校验 |
| pins/pending outputs | 7pins/7terminations、active0；2output intents各有ready；copy operations0 | 不能以lease到期替代可信停止；新引用/UNKNOWN阻断 |
| 原两trial | 原2NOT_SENT（invoked_at=null）+2linked releases保留；无send claims/absence/completion | 不重写旧basis/start、不重生成proof、不重发 |
| native IAM | reader/producer disabled，inspector enabled只读，拟建普通executor权威NoSuchUser；固定4个GET | 仅证明该普通用户不存在，不冒充全域credential销毁；未provision/启用删除IAM |
| 调用/账务 | 源仍92；07 clone112 calls/observations/typed costs，新增20读取、unaccounted0；4次management GET单列 | 所有真实调用逐attempt入账；旧失败05及旧8个管理GET保持各自回执 |
| 原备份/配置/封存 | 原tar/dump SHA未变；源business/delete Enabled=false；07 maintenance PreparationEnabled=false；源gateway始终停止 | 不重开业务或旧恢复窗口；全部新旧资源保留 |
| 精确确认/新basis/approval | NOT_RECEIVED / NOT_CREATED | 新614cd9ad current basis及≤40分钟approval仍必需 |
| 新发送组合/fresh host topology | native delete IAM/active gateway/permission/Redis发送恢复资源及签名topology尚未创建 | 不因本轮PASS启用；每项dispatch及ClaimExactSend都须fresh全门禁 |
| 实际DELETE/删除后队列恢复/完整D1 | NOT_RUN / NOT_ACCEPTED | #307 OPEN；#318 Draft，不合并、不真实删除 |

[proposal-v8](evidence/issue307-d1/delete-readonly-v7-refresh01/proposal-v8.json)、
[执行前条件](evidence/issue307-d1/delete-readonly-v7-refresh01/execution-preconditions.json)、
[验证记录](evidence/issue307-d1/delete-readonly-v7-refresh01/verification.json)、
[最终拓扑](evidence/issue307-d1/delete-readonly-v7-refresh01/retained-topology.json)和
[82份历史原字节/宿主预检](evidence/issue307-d1/delete-readonly-v7-refresh01/preflight-host.json)均另存新目录。
正常/-O各25项PASS（v6 7、只读9、v7 5、v8 4）；v6/v7/v8三个离线CLI均PASS。
v8检查20份固定indexed payload原字节和完整提案，要求新观察晚于已审v7；scope/authority变化、旧观察与打开的closure均拒绝。
原v6/v7/index/verification/独立评审及所有失败回执保持原字节。本次无新operator失败、真实DELETE、
Core业务mutation、IAM写入、hold/retention release、bypass或cleanup；新原生只读PASS不构成删除批准或新独立评审。
