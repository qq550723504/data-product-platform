# #307 D1：冻结候选04的5d删除前只读核验

最新观察见[v13基于v12的新原生只读观察](#v13基于v12的新原生只读观察)。
下文audit-core-04/06/07/08/09/10/11及v6至v12独立评审保留各自原时点；新回执另存audit-core-12，不覆盖旧证据。
v12的[独立评审](#v12独立评审与收敛检查)作为本次刷新基线；新v13为主线程原生只读及离线验收，尚未新增独立评审。
主线程当前有限切片有效blocker=0；新verification/index明确记录该边界。

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
Core业务mutation、IAM写入、hold/retention release、bypass或cleanup；原生只读PASS不构成删除批准。
该刷新verification保留当时尚未新增独立评审的事实；后续评审单独记录如下。

## v8独立评审与收敛检查

独立reviewer对HEAD `a29bfe6019cd5b38415fd378394fcd537662525c` 完成v8精确方案、
离线校验、新只读回执及固定07代operator扩展评审。结论 **PASS，当前切片有效blocker=0**。
本轮只修正方案的一处文字计数，无operator或运行源码修复，也未启动服务或新增原生读取。
原保护观察时间仍为`2026-10-15T02:14:11Z`，评审不刷新snapshot或生成删除权限。

| 条目 | 独立复核 / 本轮收口 |
| --- | --- |
| 精确范围与预算 | 与v7相同的环境、candidate04、两目标/四survivor、129560 bytes、每目标一次/总计两次、closed authority；retention到期不扩范围 |
| 原字节与历史 | 96份indexed payload（含新20份）工作区=HEAD=长度/SHA；旧六组payload/index共82份与4ee5f1a6原字节相同；原verification/index/失败历史保留 |
| operator与候选绑定 | 5项源码摘要与a29提交一致；candidate driver hash及operator base-head4ee5f1a6对应；工作树改动已显式记录；317项运行源码及binary/profile/IAM/gateway仍冻结 |
| Core与时间闭合 | source before=after=07 restored=前驱v7，194表/1879行及role/owner/ACL指纹一致；6次保护+2次inventory晚于v7且在02:14:03.010259～02:14:12.376150Z内，proposal创建于02:17:12.011406Z |
| 账务与IAM | source journal92、clone112 calls/observations/typed costs，unaccounted0；本轮4个management GET单列，旧8个保留；NoSuchUser不代表全域credential销毁 |
| 固定07与失败恢复 | 08/arbitrary/delete/confirm参数拒绝；复用TCP readiness；独立停止尝试及嵌套finally封存路径成立；不新增发送开关或runtime |
| 非删除回归 | reviewer独立执行正常/-O各25项、v6/v7/v8三个CLI均PASS；14个额外内存内层反例全部拒绝，涵盖保护/库存/DELETE/source/binary/root/IAM/credentials/pins/closure/relation/ACL/cost/permission |
| V8-DOC-1 | 非阻塞文字计数修正：精确方案原第174行把旧8个operator IAM GET写成本轮；已明确为本轮4个、旧8个保留；实际回执和账务从未改写 |
| 当前宿主回读 | 主线程另核103份原字节、同48个容器IDs/images/states均exited/PID0/无host ports、原tar/dump SHA及15份Enabled=false配置/07 maintenance封存一致；该宿主检查不读取provider或DB |
| 执行边界 | 精确确认仍NOT_RECEIVED；新basis/approval、native delete IAM/active gateway/permission及fresh topology未创建。实际DELETE/删除后恢复、完整D1/D2仍NOT_RUN / NOT_ACCEPTED，继续由#307验收；#318 Draft |

[独立评审回执](evidence/issue307-d1/delete-proposal-v8-review01/independent-review.json)与
[主线程完整性/宿主回读](evidence/issue307-d1/delete-proposal-v8-review01/primary-audit.json)另存新目录。
按AGENTS §17冻结本切片contract：精确范围与关闭authority、历史原字节、候选/operator provenance、
只读观察/账务闭合及失败恢复；Git/index提供离线完整性边界。通用IAM/owner/DR及真实删除恢复继续归#307后续验收。
本轮provider I/O、Core mutation、容器启动、IAM写入、DELETE、merge、hold/retention release、bypass和cleanup均0。

## v9基于v8的新原生只读观察

2026-10-15 **11:01:24.823～11:01:25.065 +08:00** 完成六版本保护和库存的新观察，
精确UTC范围为`2026-10-15T03:01:24.823375493Z`～`2026-10-15T03:01:25.064935255Z`。
基线为v8独立评审HEAD `a29bfe60`、收口HEAD `42d47ffa`；candidate04/source055aa1df、
317项运行源码、binary/profile/IAM/gateway、两目标/四survivor及预算均未变，没有重新构建binary。
本轮仅增加固定08代入口，复用现有只读collector、TCP readiness及root-free payload helper；
前驱v8/index/review先校验，再建立新audit-core-08，拒绝09/arbitrary/delete/confirm，未重用07或旧restore。
新operator工作树摘要另记verification；本节不把原v8评审延伸为新v9独立评审。

| 范围 | namespace / key | exact version | bytes | 本次原生保护 |
| --- | --- | --- | ---: | --- |
| 目标A | online / fixture/a.csv | 312365a9-f984-43de-b9d5-7e26009ccd82 | 232 | hold=false，NONE，retain-until=null；03:01:24.823375493Z |
| 目标B | quarantine / fixture/b.csv | 846d0c7c-289d-4667-bc7c-aa04e44357d4 | 129328 | hold=false，NONE，retain-until=null；03:01:25.064935255Z |
| 保留 | online / fixture/b.csv | 159e90c3-adb6-4ea3-b2e8-2804e578e86f | 129328 | hold=true，NONE |
| 保留 | quarantine / fixture/a.csv | 5a7ce790-9093-4569-a8fd-0726009a5eb4 | 232 | hold=false；metadata仍GOVERNANCE，retain-until=2026-10-15T00:58:47Z，已过期但仍排除删除 |
| 保留 | online / fixture/standardized.csv | 2139f293-87f4-4d49-a9f0-941b22275093 | 494 | hold=false，NONE；Entity输出 |
| 保留 | online / fixture/processed.csv | 7c8ad93b-31ee-445c-9402-816dda0410ec | 232 | hold=false，NONE；processing输出 |

namespace仍为`dpp-lifecycle307-online`/`dpp-lifecycle307-quarantine`。
[六版本回执](evidence/issue307-d1/delete-readonly-v8-refresh01/5d-08/native-six-version.json)保留各自完整SHA256、bytes及纳秒时间；
online4/quarantine2，无缺失、额外version或delete marker。内容及保护metadata与registry、原六版本恢复及v8一致；
6次保护和2次inventory均在source before `03:01:17.456899Z`/after `03:01:25.767591Z`之间。
retention到期未执行release，也未扩大两目标共129560 bytes的删除范围。

| 执行前条件 | 本次核对结果 | 发送前仍需满足 |
| --- | --- | --- |
| environment/provider/incarnation/卷/internal网络 | 原48容器IDs/images/停止状态与v8相同；只新增08 audit/IAM/native helper；最终51个均exited/PID0/无host ports | 激活前重新核对精确拓扑，不重初始化源或旧restore |
| 冻结候选与exact plan | source055aa1df、binary/profile/IAM/gateway及plan/hash不变；317运行源码匹配 | 实际发送必须绑定同一composition与预算 |
| source/Core/新独立恢复 | schema68、194表/1879行；before=after=08 restored=v8，完整relation和role/owner/ACL指纹一致 | facts变化则另建checkpoint；snapshot不是CurrentGate授权 |
| reference/owner closure | 27项完整FK资格、54项target checks unreleased0、3个函数正文匹配migration67；23 requirements/releases闭合 | dispatch/send在共享fence下fresh核验，UNKNOWN立即阻断 |
| pins/outputs/旧trial | 7pins/7terminations、active0；2output intents各有ready、copy operations0；原2NOT_SENT/2linked releases不变，无send claim/absence/completion | 不重写旧basis/start、不重生成proof、不重发旧trial |
| 原生IAM | reader/producer disabled，inspector enabled只读；拟建普通executor权威NoSuchUser；固定4个GET | 不代表全域credential销毁；未provision/启用删除用户 |
| 调用与成本 | source journal92；08 clone112 calls/observations/typed costs，新增20读取、unaccounted0；本轮4次management GET单列 | 前驱v8的4次及v7的8次各保留原回执；不冒充storage业务cost/absence |
| 备份/业务配置/封存 | 原tar/dump SHA不变；15份source business/delete Enabled=false；08 maintenance PreparationEnabled=false；source gateway始终停止 | 新旧资源全部保留，不开启业务或旧恢复窗口 |
| 精确确认/新批准 | NOT_RECEIVED；614cd9ad current basis/approval尚未创建 | 新批准≤40分钟，旧non-delete expired approval不可复用 |
| 新发送组合及fresh topology | delete IAM/active gateway/permission/Redis恢复组合及签名send topology尚未创建 | 不因只读PASS启用；每项dispatch/ClaimExactSend重新执行全部门禁 |
| 真实DELETE/删除后恢复/完整D1 | NOT_RUN / NOT_ACCEPTED | #307 OPEN，#318 Draft；不合并、不真实删除 |

[proposal-v9](evidence/issue307-d1/delete-readonly-v8-refresh01/proposal-v9.json)、
[执行前条件](evidence/issue307-d1/delete-readonly-v8-refresh01/execution-preconditions.json)、
[验证记录](evidence/issue307-d1/delete-readonly-v8-refresh01/verification.json)、
[最终拓扑](evidence/issue307-d1/delete-readonly-v8-refresh01/retained-topology.json)及
[106份历史原字节/宿主预检](evidence/issue307-d1/delete-readonly-v8-refresh01/preflight-host.json)另存新目录。
正常/-O各29项PASS（v6 7、只读9、v7 5、v8 4、v9 4），v6/v7/v8/v9四个CLI均PASS。
v9离线检查20份固定indexed payload与完整提案，递归校验前驱v8及独立评审，要求新观察晚于v8；
scope/authority改变、旧观察、source identity/active pins/running closure、index原字节或membership破坏均拒绝。
本轮无operator失败、Core业务mutation、IAM写入、DELETE、hold/retention release、bypass或cleanup；旧回执和全部失败历史保持原字节。

## v9独立评审与收敛检查

独立reviewer针对HEAD `d01c6cdc5918e04eb0981b4bff2a152bb72ee0a6` 的v9精确方案、
离线校验、新08代只读回执及固定08代operator扩展完成只读评审，结论
**PASS_INDEPENDENT_OFFLINE_REVIEW，当前切片有效blocker=0**。没有需要修复的operator/runtime问题或事实文字错误。
本节只追加评审结论；原native verification/index、观察时间及此前所有评审/失败历史保持原字节。

| 有限contract / 检查 | 独立结论与本轮收口 |
| --- | --- |
| 精确范围与关闭authority | candidate04/source055aa1df、两目标/四survivor、129560 bytes、每项最多一次/总计两次、unknown不重发与v8相同；retention到期不扩大范围；确认NOT_RECEIVED |
| 原字节与历史 | 118份indexed payload及9份index共127文件，工作区=d01 Git raw blob=长度/SHA；其中106份历史文件与42d47ffa原字节相同；原verification/index不改写 |
| operator与候选绑定 | 5项operator源码摘要与d01提交及native verification一致；candidate driver hash与operator base42d47ffa对应，worktree_changes已显式记录；317项运行源码与binary/profile/IAM/gateway仍冻结 |
| Core与时间闭合 | source before=after=08 restored=v8，194表/1879行及完整relation/role/owner/ACL指纹一致；6保护+2inventory均晚于v8，处于03:01:17.456899～03:01:25.767591Z内；proposal创建于03:03:39.522469Z |
| owner / 原trial / 账务 | 27项FK资格、54项目标检查、3函数、23 requirements/releases、7pins/terminations闭合且active0；原2NOT_SENT/releases不变；source journal92、clone112 calls/observations/typed costs，unaccounted0；本轮4个management GET单列 |
| 固定08与失败恢复 | 复用TCP readiness、只读collector及独立停止/嵌套finally封存；09/arbitrary/delete/confirm/source/generation等越界参数拒绝；没有发送开关或新runtime |
| 非删除回归 | reviewer独立执行正常/-O各29项、v6/v7/v8/v9四个CLI均PASS；16个额外08内层语义反例全部拒绝，固定08入口6组越界参数均exit2 |
| 拓扑 / 主线程回读 | 旧48项拓扑不变，仅新增3个08helper；51项回执均exited/PID0/无host ports。主线程只读宿主元数据复核同51个ID/image/state、原tar/dump SHA、15份Enabled=false配置及08 maintenance封存一致，没有启动容器或访问provider/DB |
| 执行边界与follow-up | 新basis/approval、native delete IAM/active gateway/permission及fresh topology未创建；真实DELETE/删除后恢复、完整D1/D2仍NOT_RUN / NOT_ACCEPTED，继续由#307验收；#318 Draft |

[独立评审回执](evidence/issue307-d1/delete-proposal-v9-review01/independent-review.json)与
[主线程完整性/宿主回读](evidence/issue307-d1/delete-proposal-v9-review01/primary-audit.json)另存新目录。
按AGENTS §17冻结本切片contract：精确范围/关闭authority、历史原字节、候选/operator provenance、
只读观察/账务闭合及失败恢复；Git/index是离线完整性边界。通用IAM/owner/DR及真实删除恢复继续归#307后续验收。
本轮provider I/O、Core mutation、容器启动、IAM写入、DELETE、merge、hold/retention release、bypass和cleanup均0。

## v10基于v9的新原生只读观察

2026-10-15 **11:41:15.913～11:41:16.126 +08:00** 完成六版本保护和库存的新观察，
精确UTC范围为`2026-10-15T03:41:15.91340994Z`～`2026-10-15T03:41:16.125953765Z`。
基线为v9独立评审HEAD `d01c6cdc`、收口HEAD `b68dc143`；candidate04/source055aa1df、
317项运行源码、binary/profile/IAM/gateway、两目标/四survivor及预算均未变，没有重新构建binary。
本轮仅增加固定09代入口，复用现有只读collector、TCP readiness及root-free payload helper；
前驱v9/index/review先校验，再建立新audit-core-09，拒绝10/arbitrary/delete/confirm，未重用08或旧restore。
新operator工作树摘要另记verification；本节不把原v9评审延伸为新v10独立评审。

| 范围 | namespace / key | exact version | bytes | 本次原生保护 |
| --- | --- | --- | ---: | --- |
| 目标A | online / fixture/a.csv | 312365a9-f984-43de-b9d5-7e26009ccd82 | 232 | hold=false，NONE，retain-until=null；03:41:15.91340994Z |
| 目标B | quarantine / fixture/b.csv | 846d0c7c-289d-4667-bc7c-aa04e44357d4 | 129328 | hold=false，NONE，retain-until=null；03:41:16.125953765Z |
| 保留 | online / fixture/b.csv | 159e90c3-adb6-4ea3-b2e8-2804e578e86f | 129328 | hold=true，NONE |
| 保留 | quarantine / fixture/a.csv | 5a7ce790-9093-4569-a8fd-0726009a5eb4 | 232 | hold=false；metadata仍GOVERNANCE，retain-until=2026-10-15T00:58:47Z，已过期但仍排除删除 |
| 保留 | online / fixture/standardized.csv | 2139f293-87f4-4d49-a9f0-941b22275093 | 494 | hold=false，NONE；Entity输出 |
| 保留 | online / fixture/processed.csv | 7c8ad93b-31ee-445c-9402-816dda0410ec | 232 | hold=false，NONE；processing输出 |

namespace仍为`dpp-lifecycle307-online`/`dpp-lifecycle307-quarantine`。
[六版本回执](evidence/issue307-d1/delete-readonly-v9-refresh01/5d-09/native-six-version.json)保留各自完整SHA256、bytes及纳秒时间；
online4/quarantine2，无缺失、额外version或delete marker。内容及保护metadata与registry、原六版本恢复及v9一致；
6次保护和2次inventory均在source before `03:41:07.876277Z`/after `03:41:16.781464Z`之间。
retention到期未执行release，也未扩大两目标共129560 bytes的删除范围。

| 执行前条件 | 本次核对结果 | 发送前仍需满足 |
| --- | --- | --- |
| environment/provider/incarnation/卷/internal网络 | 原51容器IDs/images/停止状态与v9相同；只新增09 audit/IAM/native helper；最终54个均exited/PID0/无host ports | 激活前重新核对精确拓扑，不重初始化源或旧restore |
| 冻结候选与exact plan | source055aa1df、binary/profile/IAM/gateway及plan/hash不变；317运行源码匹配 | 实际发送必须绑定同一composition与预算 |
| source/Core/新独立恢复 | schema68、194表/1879行；before=after=09 restored=v9，完整relation和role/owner/ACL指纹一致 | facts变化则另建checkpoint；snapshot不是CurrentGate授权 |
| reference/owner closure | 27项完整FK资格、54项target checks unreleased0、3个函数正文匹配migration67；23 requirements/releases闭合 | dispatch/send在共享fence下fresh核验，UNKNOWN立即阻断 |
| pins/outputs/旧trial | 7pins/7terminations、active0；2output intents各有ready、copy operations0；原2NOT_SENT/2linked releases不变，无send claim/absence/completion | 不重写旧basis/start、不重生成proof、不重发旧trial |
| 原生IAM | reader/producer disabled，inspector enabled只读；拟建普通executor权威NoSuchUser；固定4个GET | 不代表全域credential销毁；未provision/启用删除用户 |
| 调用与成本 | source journal92；09 clone112 calls/observations/typed costs，新增20读取、unaccounted0；本轮4次management GET单列 | 前驱v9的4次、v8的4次及v7的8次各保留原回执；不冒充storage业务cost/absence |
| 备份/业务配置/封存 | 原tar/dump SHA不变；15份source business/delete Enabled=false；09 maintenance PreparationEnabled=false；source gateway始终停止 | 新旧资源全部保留，不开启业务或旧恢复窗口 |
| 精确确认/新批准 | NOT_RECEIVED；614cd9ad current basis/approval尚未创建 | 新批准≤40分钟，旧non-delete expired approval不可复用 |
| 新发送组合及fresh topology | delete IAM/active gateway/permission/Redis恢复组合及签名send topology尚未创建 | 不因只读PASS启用；每项dispatch/ClaimExactSend重新执行全部门禁 |
| 真实DELETE/删除后恢复/完整D1 | NOT_RUN / NOT_ACCEPTED | #307 OPEN，#318 Draft；不合并、不真实删除 |

[proposal-v10](evidence/issue307-d1/delete-readonly-v9-refresh01/proposal-v10.json)、
[执行前条件](evidence/issue307-d1/delete-readonly-v9-refresh01/execution-preconditions.json)、
[验证记录](evidence/issue307-d1/delete-readonly-v9-refresh01/verification.json)、
[最终拓扑](evidence/issue307-d1/delete-readonly-v9-refresh01/retained-topology.json)及
[130份历史原字节/宿主预检](evidence/issue307-d1/delete-readonly-v9-refresh01/preflight-host.json)另存新目录。
正常/-O各33项PASS（v6 7、只读9、v7 5、v8 4、v9 4、v10 4），v6/v7/v8/v9/v10五个CLI均PASS。
v10离线检查20份固定indexed payload与完整提案，递归校验前驱v9及独立评审，要求新观察晚于v9；
scope/authority改变、旧观察、source identity/active pins/running closure、index原字节或membership破坏均拒绝。
主线程离线收口首次误写前驱子目录为5d-07，在新收口文件生成前失败；纠正为前驱v9的5d-08后完成，
私有诊断保留，未重跑原生采集或修改其回执。该本地路径错误不计为native业务失败。
本轮无operator原生失败、Core业务mutation、IAM写入、DELETE、hold/retention release、bypass或cleanup；旧回执和全部失败历史保持原字节。

## v10独立评审与收敛检查

独立reviewer针对HEAD `cf65cbad0bf93601e2b8a2a24ea2ffcc09247774` 的v10精确方案、
离线校验、新09代只读回执及固定09代operator扩展完成只读评审，结论
**PASS_INDEPENDENT_OFFLINE_REVIEW，当前切片有效blocker=0**。没有需要修复的operator/runtime问题或事实文字错误。
本节只追加评审结论；原native verification/index、保护观察时间及此前所有评审/失败历史保持原字节。

| 有限contract / 检查 | 独立结论与本轮收口 |
| --- | --- |
| 精确范围与关闭authority | candidate04/source055aa1df、两目标/四survivor、129560 bytes、每项最多一次/总计两次、unknown不重发与v9相同；retention到期不扩大范围；确认NOT_RECEIVED |
| 原字节与历史 | 140份indexed payload及11份index共151文件，工作区=cf65 Git raw blob=长度/SHA；其中130份历史文件与b68dc143原字节相同；原verification/index不改写 |
| operator与候选绑定 | 5项operator源码摘要与cf65提交及native verification一致；candidate driver hash与operator base b68dc143对应，worktree_changes已显式记录；317项运行源码与binary/profile/IAM/gateway仍冻结 |
| Core与时间闭合 | source before=after=09 restored=v9，194表/1879行及完整relation/role/owner/ACL指纹一致；6保护+2inventory均晚于v9，处于03:41:07.876277～03:41:16.781464Z内；proposal创建于03:41:54.669332Z |
| owner / 原trial / 账务 | 27项FK资格、54项目标检查、3函数、23 requirements/releases、7pins/terminations闭合且active0；原2NOT_SENT/releases不变；source journal92、clone112 calls/observations/typed costs，unaccounted0；本代4个management GET单列 |
| 固定09与失败恢复 | 复用TCP readiness、只读collector及独立停止/嵌套finally封存；10/arbitrary/delete/confirm/source/generation等越界参数拒绝；没有发送开关或新runtime |
| 非删除回归 | reviewer独立执行正常/-O各33项、v6/v7/v8/v9/v10五个CLI均PASS；16个额外09内层语义反例全部拒绝，固定09入口6组越界参数均exit2 |
| 拓扑 / 主线程回读 | 旧51项拓扑不变，仅新增3个09helper；54项回执均exited/PID0/无host ports。主线程只读宿主元数据复核同54个ID/image/state、原tar/dump SHA、15份Enabled=false配置及09 maintenance封存一致，没有启动容器或访问provider/DB |
| 执行边界与follow-up | 新basis/approval、native delete IAM/active gateway/permission及fresh topology未创建；真实DELETE/删除后恢复、完整D1/D2仍NOT_RUN / NOT_ACCEPTED，继续由#307验收；#318 Draft |

[独立评审回执](evidence/issue307-d1/delete-proposal-v10-review01/independent-review.json)与
[主线程完整性/宿主回读](evidence/issue307-d1/delete-proposal-v10-review01/primary-audit.json)另存新目录。
按AGENTS §17冻结本切片contract：精确范围/关闭authority、历史原字节、候选/operator provenance、
只读观察/账务闭合及失败恢复；Git/index是离线完整性边界。通用IAM/owner/DR及真实删除恢复继续归#307后续验收。
本轮provider I/O、Core mutation、容器启动、IAM写入、DELETE、merge、hold/retention release、bypass和cleanup均0。

## v11基于v10的新原生只读观察

2026-10-15 **12:21:10.378～12:21:10.610 +08:00** 完成六版本保护和库存的新观察，
精确UTC范围为`2026-10-15T04:21:10.377740646Z`～`2026-10-15T04:21:10.609535889Z`。
基线为v10独立评审HEAD `cf65cbad`、收口HEAD `3a5b68c8`；candidate04/source055aa1df、
317项运行源码、binary/profile/IAM/gateway、两目标/四survivor及预算均未变，没有重新构建binary。
本轮仅增加固定10代入口，复用现有只读collector、TCP readiness及root-free payload helper；
前驱v10/index/review先校验，再建立新audit-core-10，拒绝11/arbitrary/delete/confirm，未重用09或旧restore。
新operator工作树摘要另记verification；本节不把原v10评审延伸为新v11独立评审。

| 范围 | namespace / key | exact version | bytes | 本次原生保护 |
| --- | --- | --- | ---: | --- |
| 目标A | online / fixture/a.csv | 312365a9-f984-43de-b9d5-7e26009ccd82 | 232 | hold=false，NONE，retain-until=null；04:21:10.377740646Z |
| 目标B | quarantine / fixture/b.csv | 846d0c7c-289d-4667-bc7c-aa04e44357d4 | 129328 | hold=false，NONE，retain-until=null；04:21:10.609535889Z |
| 保留 | online / fixture/b.csv | 159e90c3-adb6-4ea3-b2e8-2804e578e86f | 129328 | hold=true，NONE |
| 保留 | quarantine / fixture/a.csv | 5a7ce790-9093-4569-a8fd-0726009a5eb4 | 232 | hold=false；metadata仍GOVERNANCE，retain-until=2026-10-15T00:58:47Z，已过期但仍排除删除 |
| 保留 | online / fixture/standardized.csv | 2139f293-87f4-4d49-a9f0-941b22275093 | 494 | hold=false，NONE；Entity输出 |
| 保留 | online / fixture/processed.csv | 7c8ad93b-31ee-445c-9402-816dda0410ec | 232 | hold=false，NONE；processing输出 |

namespace仍为`dpp-lifecycle307-online`/`dpp-lifecycle307-quarantine`。
[六版本回执](evidence/issue307-d1/delete-readonly-v10-refresh01/5d-10/native-six-version.json)保留各自完整SHA256、bytes及纳秒时间；
online4/quarantine2，无缺失、额外version或delete marker。内容及保护metadata与registry、原六版本恢复及v10一致；
6次保护和2次inventory均在source before `04:21:02.788585Z`/after `04:21:11.321418Z`之间。
retention到期未执行release，也未扩大两目标共129560 bytes的删除范围。

| 执行前条件 | 本次核对结果 | 发送前仍需满足 |
| --- | --- | --- |
| environment/provider/incarnation/卷/internal网络 | 原54容器IDs/images/停止状态与v10相同；只新增10代audit/IAM/native helper；最终57个均exited/PID0/无host ports | 激活前重新核对精确拓扑，不重初始化源或旧restore |
| 冻结候选与exact plan | source055aa1df、binary/profile/IAM/gateway及plan/hash不变；317运行源码匹配 | 实际发送必须绑定同一composition与预算 |
| source/Core/新独立恢复 | schema68、194表/1879行；before=after=10 restored=v10，完整relation和role/owner/ACL指纹一致 | facts变化则另建checkpoint；snapshot不是CurrentGate授权 |
| reference/owner closure | 27项完整FK资格、54项target checks unreleased0、3个函数正文匹配migration67；23 requirements/releases闭合 | dispatch/send在共享fence下fresh核验，UNKNOWN立即阻断 |
| pins/outputs/旧trial | 7pins/7terminations、active0；2output intents各有ready、copy operations0；原2NOT_SENT/2linked releases不变，无send claim/absence/completion | 不重写旧basis/start、不重生成proof、不重发旧trial |
| 原生IAM | reader/producer disabled，inspector enabled只读；拟建普通executor权威NoSuchUser；固定4个GET | 不代表全域credential销毁；未provision/启用删除用户 |
| 调用与成本 | source journal92；10代clone112 calls/observations/typed costs，新增20读取、unaccounted0；本轮4次management GET单列 | 前驱v10/v9/v8各4次及v7的8次各保留原回执；不冒充storage业务cost/absence |
| 备份/业务配置/封存 | 原tar/dump SHA不变；15份source business/delete Enabled=false；10代maintenance PreparationEnabled=false；source gateway始终停止 | 新旧资源全部保留，不开启业务或旧恢复窗口 |
| 精确确认/新批准 | NOT_RECEIVED；614cd9ad current basis/approval尚未创建 | 新批准≤40分钟，旧non-delete expired approval不可复用 |
| 新发送组合及fresh topology | delete IAM/active gateway/permission/Redis恢复组合及签名send topology尚未创建 | 不因只读PASS启用；每项dispatch/ClaimExactSend重新执行全部门禁 |
| 真实DELETE/删除后恢复/完整D1 | NOT_RUN / NOT_ACCEPTED | #307 OPEN，#318 Draft；不合并、不真实删除 |

[proposal-v11](evidence/issue307-d1/delete-readonly-v10-refresh01/proposal-v11.json)、
[执行前条件](evidence/issue307-d1/delete-readonly-v10-refresh01/execution-preconditions.json)、
[验证记录](evidence/issue307-d1/delete-readonly-v10-refresh01/verification.json)、
[最终拓扑](evidence/issue307-d1/delete-readonly-v10-refresh01/retained-topology.json)及
[154份历史原字节/宿主预检](evidence/issue307-d1/delete-readonly-v10-refresh01/preflight-host.json)另存新目录。
正常/-O各37项PASS（v6 7、只读9、v7 5、v8 4、v9 4、v10 4、v11 4），v6/v7/v8/v9/v10/v11六个CLI均PASS。
v11离线检查20份固定indexed payload与完整提案，递归校验前驱v10及独立评审，要求新观察晚于v10；
scope/authority改变、旧观察、source identity/active pins/running closure、index原字节或membership破坏均拒绝。
本轮无operator或离线收口失败、Core业务mutation、IAM写入、DELETE、hold/retention release、bypass或cleanup；旧回执和全部失败历史保持原字节。

## v11独立评审与收敛检查

独立reviewer针对HEAD `fb0ff60e2fc8e6fb808e96595e0ddae418bef1cd` 的v11精确方案、
离线校验、新10代只读回执及固定10代operator扩展完成只读评审，结论
**PASS_INDEPENDENT_OFFLINE_REVIEW，当前切片有效blocker=0**。没有需要修复的operator/runtime问题或事实文字错误。
本节只追加评审结论；原native verification/index、保护观察时间及此前所有评审/失败历史保持原字节。

| 有限contract / 检查 | 独立结论与本轮收口 |
| --- | --- |
| 精确范围与关闭authority | candidate04/source055aa1df、两目标/四survivor、129560 bytes、每项最多一次/总计两次、unknown不重发与v10相同；retention到期不扩大范围；确认NOT_RECEIVED |
| 原字节与历史 | 162份indexed payload及13份index共175文件，工作区=fb0 Git raw blob=长度/SHA；其中154份历史文件与3a5b68c8原字节相同；原verification/index不改写 |
| operator与候选绑定 | 5项operator源码摘要与fb0提交及native verification一致；candidate driver hash与operator base 3a5b68c8对应，worktree_changes已显式记录；317项运行源码与binary/profile/IAM/gateway仍冻结 |
| Core与时间闭合 | source before=after=10 restored=v10，194表/1879行及完整relation/role/owner/ACL指纹一致；6保护+2inventory均晚于v10，处于04:21:02.788585～04:21:11.321418Z内；proposal创建于04:21:32.599572Z |
| owner / 原trial / 账务 | 27项FK资格、54项目标检查、3函数、23 requirements/releases、7pins/terminations闭合且active0；原2NOT_SENT/releases不变；source journal92、clone112 calls/observations/typed costs，unaccounted0；本代4个management GET单列 |
| 固定10与失败恢复 | 复用TCP readiness、只读collector及独立停止/嵌套finally封存；11/arbitrary/delete/confirm/source/generation等越界参数拒绝；没有发送开关或新runtime |
| 非删除回归 | reviewer独立执行正常/-O各37项、v6/v7/v8/v9/v10/v11六个CLI均PASS；16个额外10代内层语义反例全部拒绝，固定10入口6组越界参数均exit2 |
| 拓扑 / 主线程回读 | 旧54项拓扑不变，仅新增3个10代helper；57项回执均exited/PID0/无host ports。主线程只读宿主元数据复核同57个ID/image/state、原tar/dump SHA、15份Enabled=false配置及10代maintenance封存一致，没有启动容器或访问provider/DB |
| 执行边界与follow-up | 新basis/approval、native delete IAM/active gateway/permission及fresh topology未创建；真实DELETE/删除后恢复、完整D1/D2仍NOT_RUN / NOT_ACCEPTED，继续由#307验收；#318 Draft |

[独立评审回执](evidence/issue307-d1/delete-proposal-v11-review01/independent-review.json)与
[主线程完整性/宿主回读](evidence/issue307-d1/delete-proposal-v11-review01/primary-audit.json)另存新目录。
按AGENTS §17冻结本切片contract：精确范围/关闭authority、历史原字节、候选/operator provenance、
只读观察/账务闭合及失败恢复；Git/index是离线完整性边界。通用IAM/owner/DR及真实删除恢复继续归#307后续验收。
本轮provider I/O、Core mutation、容器启动、IAM写入、DELETE、merge、hold/retention release、bypass和cleanup均0。

## v12基于v11的新原生只读观察

2026-10-15 **12:58:54.076～12:58:54.422 +08:00** 完成六版本保护和库存的新观察，
精确UTC范围为`2026-10-15T04:58:54.07613659Z`～`2026-10-15T04:58:54.422249145Z`。
基线为v11独立评审HEAD `fb0ff60e`、收口HEAD `c7819b7f`；candidate04/source055aa1df、
317项运行源码、binary/profile/IAM/gateway、两目标/四survivor及预算均未变，没有重新构建binary。
本轮仅增加固定11代入口，复用现有只读collector、TCP readiness及root-free payload helper；
前驱v11/index/review先校验，再建立新audit-core-11，拒绝12/arbitrary/delete/confirm，未重用10或旧restore。
新operator工作树摘要另记verification；本节不把原v11评审延伸为新v12独立评审。

| 范围 | namespace / key | exact version | bytes | 本次原生保护 |
| --- | --- | --- | ---: | --- |
| 目标A | online / fixture/a.csv | 312365a9-f984-43de-b9d5-7e26009ccd82 | 232 | hold=false，NONE，retain-until=null；04:58:54.07613659Z |
| 目标B | quarantine / fixture/b.csv | 846d0c7c-289d-4667-bc7c-aa04e44357d4 | 129328 | hold=false，NONE，retain-until=null；04:58:54.422249145Z |
| 保留 | online / fixture/b.csv | 159e90c3-adb6-4ea3-b2e8-2804e578e86f | 129328 | hold=true，NONE |
| 保留 | quarantine / fixture/a.csv | 5a7ce790-9093-4569-a8fd-0726009a5eb4 | 232 | hold=false；metadata仍GOVERNANCE，retain-until=2026-10-15T00:58:47Z，已过期但仍排除删除 |
| 保留 | online / fixture/standardized.csv | 2139f293-87f4-4d49-a9f0-941b22275093 | 494 | hold=false，NONE；Entity输出 |
| 保留 | online / fixture/processed.csv | 7c8ad93b-31ee-445c-9402-816dda0410ec | 232 | hold=false，NONE；processing输出 |

namespace仍为`dpp-lifecycle307-online`/`dpp-lifecycle307-quarantine`。
[六版本回执](evidence/issue307-d1/delete-readonly-v11-refresh01/5d-11/native-six-version.json)保留各自完整SHA256、bytes及纳秒时间；
online4/quarantine2，无缺失、额外version或delete marker。内容及保护metadata与registry、原六版本恢复及v11一致；
6次保护和2次inventory均在source before `04:58:43.197790Z`/after `04:58:55.213211Z`之间。
retention到期未执行release，也未扩大两目标共129560 bytes的删除范围。

| 执行前条件 | 本次核对结果 | 发送前仍需满足 |
| --- | --- | --- |
| environment/provider/incarnation/卷/internal网络 | 原57容器IDs/images/停止状态与v11相同；只新增11代audit/IAM/native helper；最终60个均exited/PID0/无host ports | 激活前重新核对精确拓扑，不重初始化源或旧restore |
| 冻结候选与exact plan | source055aa1df、binary/profile/IAM/gateway及plan/hash不变；317运行源码匹配 | 实际发送必须绑定同一composition与预算 |
| source/Core/新独立恢复 | schema68、194表/1879行；before=after=11 restored=v11，完整relation和role/owner/ACL指纹一致 | facts变化则另建checkpoint；snapshot不是CurrentGate授权 |
| reference/owner closure | 27项完整FK资格、54项target checks unreleased0、3个函数正文匹配migration67；23 requirements/releases闭合 | dispatch/send在共享fence下fresh核验，UNKNOWN立即阻断 |
| pins/outputs/旧trial | 7pins/7terminations、active0；2output intents各有ready、copy operations0；原2NOT_SENT/2linked releases不变，无send claim/absence/completion | 不重写旧basis/start、不重生成proof、不重发旧trial |
| 原生IAM | reader/producer disabled，inspector enabled只读；拟建普通executor权威NoSuchUser；固定4个GET | 不代表全域credential销毁；未provision/启用删除用户 |
| 调用与成本 | source journal92；11代clone112 calls/observations/typed costs，新增20读取、unaccounted0；本轮4次management GET单列 | 前驱v11/v10/v9/v8各4次及v7的8次各保留原回执；不冒充storage业务cost/absence |
| 备份/业务配置/封存 | 原tar/dump SHA不变；15份source business/delete Enabled=false；11代maintenance PreparationEnabled=false；source gateway始终停止 | 新旧资源全部保留，不开启业务或旧恢复窗口 |
| 精确确认/新批准 | NOT_RECEIVED；614cd9ad current basis/approval尚未创建 | 新批准≤40分钟，旧non-delete expired approval不可复用 |
| 新发送组合及fresh topology | delete IAM/active gateway/permission/Redis恢复组合及签名send topology尚未创建 | 不因只读PASS启用；每项dispatch/ClaimExactSend重新执行全部门禁 |
| 真实DELETE/删除后恢复/完整D1 | NOT_RUN / NOT_ACCEPTED | #307 OPEN，#318 Draft；不合并、不真实删除 |

[proposal-v12](evidence/issue307-d1/delete-readonly-v11-refresh01/proposal-v12.json)、
[执行前条件](evidence/issue307-d1/delete-readonly-v11-refresh01/execution-preconditions.json)、
[验证记录](evidence/issue307-d1/delete-readonly-v11-refresh01/verification.json)、
[最终拓扑](evidence/issue307-d1/delete-readonly-v11-refresh01/retained-topology.json)及
[178份历史原字节/宿主预检](evidence/issue307-d1/delete-readonly-v11-refresh01/preflight-host.json)另存新目录。
正常/-O各41项PASS（v6 7、只读9、v7 5、v8/v9/v10/v11/v12各4），v6/v7/v8/v9/v10/v11/v12七个CLI均PASS。
v12离线检查20份固定indexed payload与完整提案，递归校验前驱v11及独立评审，要求新观察晚于v11；
scope/authority改变、旧观察、source identity/active pins/running closure、index原字节或membership破坏均拒绝。
本轮无operator或离线收口失败、Core业务mutation、IAM写入、DELETE、hold/retention release、bypass或cleanup；旧回执和全部失败历史保持原字节。

## v12独立评审与收敛检查

独立reviewer对HEAD `eed11a97476a0b8c8ac5af40d7a5e2bdacfb3d8a` 的v12精确方案、
离线校验、新11代只读回执及固定11代operator扩展完成本轮实际审阅与离线执行，结论
**PASS_INDEPENDENT_OFFLINE_REVIEW，当前切片有效blocker=0**。无需operator/runtime修复或事实文字修正。
本节只追加独立结论；原native verification/index、12:58:54观察及所有历史/失败证据原字节不变。

| 有限contract / 检查 | 独立结论与本轮收口 |
| --- | --- |
| exact scope / authority | candidate04/source055aa1df、两目标/四survivor、129560 bytes、每项最多一次/总计两次、unknown不重发与v11相同；retention到期不扩范围；精确确认NOT_RECEIVED |
| 历史和原字节 | 184份indexed payload+15份index共199 raw均等于eed11a97 Git blob、长度/SHA；其中178份历史raw与c7819b7f相同；新v12 payload20份，旧verification/index不改写 |
| operator / runtime provenance | 5项operator摘要与eed11a97/native verification一致，candidate driver和operator base c7819b7f对应、worktree_changes=true已记录；317项runtime向原frozen raw/LF-CRLF等价锚点证明，binary/profile/IAM/gateway保持冻结 |
| Core / 时间闭合 | before=after=11 restored=v11，194表/1879行及完整relation/role/owner/ACL指纹一致；6保护+2inventory均晚于v11，处于04:58:43.197790～04:58:55.213211Z内；Core as_of04:58:33.690146Z晚于前驱，proposal04:59:33.122948Z晚于回读 |
| references / owner / 原trial | 27完整guard、54target checks unreleased0、3函数正文/SHA匹配migration67；23 verified-quiescent requirements/releases；7pins/terminations active0、2ready outputs、copy operations0；原2NOT_SENT(invoked_at=null)/2linked releases不变，无send claim/absence/completion |
| native IAM / 调用成本 | reader/producer disabled、inspector enabled、拟建普通executor NoSuchUser；source journal92，新11 clone112 calls/observations/typed costs、unaccounted0；本代4management GET单列，旧批准仍过期 |
| 固定11 / 失败恢复 | 扩展限于有限白名单/前驱绑定和wrapper/checker/tests/CI；复用collector、TCP readiness、独立停止及nested-finally封存，无send开关或新runtime；12/arbitrary/delete/confirm/source/generation越界拒绝 |
| 独立非删除回归 | 正常/-O各41项、v6/v7/v8/v9/v10/v11/v12七CLI均PASS；16项新11代内层语义反例直接经expected_proposal拒绝，未依赖外层index；固定11入口6组实际越界args均exit2 |
| retained topology / 主线程回读 | 公共回执原57条目不变，仅增加固定11代audit/IAM/native helper三项，60项均exited/PID0/无host ports；reviewer未访问Docker。主线程宿主元数据核对同60 ID/image/state、原tar/dump SHA、15份Enabled=false、11代maintenance/permission和gateway封存一致，未启动容器或访问provider/DB |
| 执行门禁与follow-up | 新basis/approval、native delete IAM/active gateway/permission/fresh signed topology仍未创建；真实DELETE/删除后恢复、完整D1/D2仍NOT_RUN / NOT_ACCEPTED，继续归#307后续验收；#318 Draft |

[独立评审回执](evidence/issue307-d1/delete-proposal-v12-review01/independent-review.json)与
[主线程完整性/宿主回读](evidence/issue307-d1/delete-proposal-v12-review01/primary-audit.json)存于新目录。
按AGENTS §17冻结有限contract：exact scope/关闭authority、历史原字节、候选/operator来源、
只读观察/账务闭合及失败恢复；Git/index为离线完整性边界。通用IAM/owner/DR、真实删除恢复及完整D1/D2继续归#307。
本轮provider/DB I/O、Core mutation、容器启动、IAM写入、DELETE、merge、保护release、bypass和cleanup均0。

## v13基于v12的新原生只读观察

2026-10-15 **13:32:35.188～13:32:35.712 +08:00** 完成六版本保护和库存的新观察，
精确UTC范围为`2026-10-15T05:32:35.1886766Z`～`2026-10-15T05:32:35.712494743Z`。
基线为v12独立评审HEAD `eed11a97`、收口HEAD `ea51c7ca`；candidate04/source055aa1df、
317项运行源码、binary/profile/IAM/gateway、两目标/四survivor及预算均未变，没有重新构建binary。
本轮仅增加固定12代入口，复用现有只读collector、TCP readiness及root-free payload helper；
前驱v12/index/review先校验，再建立新audit-core-12，拒绝13/arbitrary/delete/confirm，未重用11或旧restore。
新operator工作树摘要另记verification；本节不把原v12评审延伸为新v13独立评审。

| 范围 | namespace / key | exact version | bytes | 本次原生保护 |
| --- | --- | --- | ---: | --- |
| 目标A | online / fixture/a.csv | 312365a9-f984-43de-b9d5-7e26009ccd82 | 232 | hold=false，NONE，retain-until=null；05:32:35.1886766Z |
| 目标B | quarantine / fixture/b.csv | 846d0c7c-289d-4667-bc7c-aa04e44357d4 | 129328 | hold=false，NONE，retain-until=null；05:32:35.712494743Z |
| 保留 | online / fixture/b.csv | 159e90c3-adb6-4ea3-b2e8-2804e578e86f | 129328 | hold=true，NONE |
| 保留 | quarantine / fixture/a.csv | 5a7ce790-9093-4569-a8fd-0726009a5eb4 | 232 | hold=false；metadata仍GOVERNANCE，retain-until=2026-10-15T00:58:47Z，已过期但仍排除删除 |
| 保留 | online / fixture/standardized.csv | 2139f293-87f4-4d49-a9f0-941b22275093 | 494 | hold=false，NONE；Entity输出 |
| 保留 | online / fixture/processed.csv | 7c8ad93b-31ee-445c-9402-816dda0410ec | 232 | hold=false，NONE；processing输出 |

namespace仍为`dpp-lifecycle307-online`/`dpp-lifecycle307-quarantine`。
[六版本回执](evidence/issue307-d1/delete-readonly-v12-refresh01/5d-12/native-six-version.json)保留各自完整SHA256、bytes及纳秒时间；
online4/quarantine2，无缺失、额外version或delete marker。内容及保护metadata与registry、原六版本恢复及v12一致；
6次保护和2次inventory均在source before `05:32:23.414824Z`/after `05:32:36.675783Z`之间。
retention到期未执行release，也未扩大两目标共129560 bytes的删除范围。

| 执行前条件 | 本次核对结果 | 发送前仍需满足 |
| --- | --- | --- |
| environment/provider/incarnation/卷/internal网络 | 原60容器IDs/images/停止状态与v12相同；只新增12代audit/IAM/native helper；最终63个均exited/PID0/无host ports | 激活前重新核对精确拓扑，不重初始化源或旧restore |
| 冻结候选与exact plan | source055aa1df、binary/profile/IAM/gateway及plan/hash不变；317运行源码匹配 | 实际发送必须绑定同一composition与预算 |
| source/Core/新独立恢复 | schema68、194表/1879行；before=after=12 restored=v12，完整relation和role/owner/ACL指纹一致 | facts变化则另建checkpoint；snapshot不是CurrentGate授权 |
| reference/owner closure | 27项完整FK资格、54项target checks unreleased0、3个函数正文匹配migration67；23 requirements/releases闭合 | dispatch/send在共享fence下fresh核验，UNKNOWN立即阻断 |
| pins/outputs/旧trial | 7pins/7terminations、active0；2output intents各有ready、copy operations0；原2NOT_SENT/2linked releases不变，无send claim/absence/completion | 不重写旧basis/start、不重生成proof、不重发旧trial |
| 原生IAM | reader/producer disabled，inspector enabled只读；拟建普通executor权威NoSuchUser；固定4个GET | 不代表全域credential销毁；未provision/启用删除用户 |
| 调用与成本 | source journal92；12代clone112 calls/observations/typed costs，新增20读取、unaccounted0；本轮4次management GET单列 | 前驱v12/v11/v10/v9/v8各4次及v7的8次各保留原回执；不冒充storage业务cost/absence |
| 备份/业务配置/封存 | 原tar/dump SHA不变；15份source business/delete Enabled=false；12代maintenance PreparationEnabled=false；source gateway始终停止 | 新旧资源全部保留，不开启业务或旧恢复窗口 |
| 精确确认/新批准 | NOT_RECEIVED；614cd9ad current basis/approval尚未创建 | 新批准≤40分钟，旧non-delete expired approval不可复用 |
| 新发送组合及fresh topology | delete IAM/active gateway/permission/Redis恢复组合及签名send topology尚未创建 | 不因只读PASS启用；每项dispatch/ClaimExactSend重新执行全部门禁 |
| 真实DELETE/删除后恢复/完整D1 | NOT_RUN / NOT_ACCEPTED | #307 OPEN，#318 Draft；不合并、不真实删除 |

[proposal-v13](evidence/issue307-d1/delete-readonly-v12-refresh01/proposal-v13.json)、
[执行前条件](evidence/issue307-d1/delete-readonly-v12-refresh01/execution-preconditions.json)、
[验证记录](evidence/issue307-d1/delete-readonly-v12-refresh01/verification.json)、
[最终拓扑](evidence/issue307-d1/delete-readonly-v12-refresh01/retained-topology.json)及
[202份历史原字节/宿主预检](evidence/issue307-d1/delete-readonly-v12-refresh01/preflight-host.json)另存新目录。
正常/-O各45项PASS（v6 7、只读9、v7 5、v8/v9/v10/v11/v12/v13各4），v6/v7/v8/v9/v10/v11/v12/v13八个CLI均PASS。
v13离线检查20份固定indexed payload与完整提案，递归校验前驱v12及独立评审，要求新观察晚于v12；
scope/authority改变、旧观察、source identity/active pins/running closure、index原字节或membership破坏均拒绝。
本轮无operator或离线收口失败、Core业务mutation、IAM写入、DELETE、hold/retention release、bypass或cleanup；旧回执和全部失败历史保持原字节。

### v13分支同步与CI入口修复

原生刷新提交 `4cee7982f63502be2abf1df32aa0000dc46e30d7` 推送后，#318因main新接线与
`run-copy-recovery-step.ps1` 冲突而显示CONFLICTING，没有产生该HEAD的PR CI；不把前驱绿色CI当作当前验收。
在本分支同步main `d7c9ccdfe4fba066b512fe4adbcd1df281a2d0d9`，保留四个D1 PowerShell包装脚本及README
在4cee7982的Git原字节，维持schema68、StartOnly barrier、完整crash退出码和依赖原始Docker元数据的保留契约。
main通用回收adapter及其他变更正常同步；D1后续接线必须先完成metadata依赖闭合，仍归#307。
本操作是分支集成，没有合并#318，也未启动任何业务/helper或执行资源回收。

317项冻结runtime、5项只读operator和原223份indexed raw证据均与刷新提交相同；原native verification/index不改写。
新[集成回执](evidence/issue307-d1/delete-readonly-v13-integration01/integration-verification.json)及index另存，
记录正常/-O各45项、8个CLI、临时容器契约正常/-O各9项、required-check policy测试与5份PowerShell语法解析PASS。
Node默认reporter格式导致的首次本地计数采集失败另留诊断；测试命令本身exit0，随后固定TAP reporter重新离线收口。
没有重跑native、没有新增保护观察；13:32:35源观察、exact scope/关闭authority均保持原值。
当前main CI增加Linux/Windows临时容器契约两项，完整PR CI应核对当前HEAD的13项job；其结果独立记录在PR/Issue检查点。

### v13显式retain与新run默认回收的有界集成

前节4cb6c506同步时保留整套旧wrapper的办法会撤销main对未来新run的默认回收，边界过宽；
现已复用main的New/Complete-DppTemporaryContainer，默认新建同步成功helper归档/退役。
四个入口支持显式 `-RetainHelper`；reader/copy预期非零故障、plane扩展D1模式自动retain，
copy StartOnly在Complete前返回，保留schema68、barrier及94/95/91/92/93/96等原恢复退出码语义。
没有把旧run追认进registry，不热改已封存operator/配置或重开旧环境。

[有界接线回执](evidence/issue307-d1/delete-readonly-v13-integration02/retain-boundary-verification.json)保留9项静态D1 invariant、
317冻结runtime/5只读operator/原225份raw未变的核对，adapter正常/-O各9项、8CLI与5份PS解析PASS；
[wrapper行为回归](evidence/issue307-d1/delete-readonly-v13-integration02/wrapper-wiring-regression.json)另记录17个实际离线用例，
覆盖默认回收、显式/故障保留、异常不Complete、StartOnly不Complete、准确退出码和receipt先于completion。
用例执行真实wrapper片段/Run AST并stub外部边界，未调用Docker executable/provider/DB；同一回归已接入现有Ubuntu/Windows CI matrix。
新wrapper回执分别记录执行时Windows原始文本摘要与Git LF文本摘要，证明CI文本的LF/CRLF等价；不转换binary/profile/IAM摘要。
fixture首次提取scriptblock时PSScriptRoot为空的参数绑定失败已修正，并记录在新回执；不冒充native或业务失败。
当前D1索引共228 raw（209 payload/19 index），原native保护观察与历史回执不改写；v13仍未新增独立评审，#318 Draft。
