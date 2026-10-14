# #307 D1：冻结候选04的5d删除前只读核验

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

[当前proposal-v5](evidence/issue307-d1/delete-readonly-candidate04/proposal-v5.json)
只刷新观察时间、证据和执行前条件；原source/binary/profile/plan/两目标不变。
[精确确认方案](issue307-d1-delete-proposal.md)仍待明确确认；真实删除、完整D1/D2未验收。
