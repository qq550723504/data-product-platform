# #307 D1 candidate12/v22 新授权与窗口前状态

当前：**新的人类按引用授权已收到；固定窗口尚未开启；发送权限关闭；本轮DELETE=0。**

直接人类原文是 `授权` 加末尾LF，共7个UTF-8字节，SHA256 `fce3771b1b29f5b8c466a7b4e76e01f1ab8436b481c706af4563f5819cc1e017`。消息 `msg_01a15200-7e0b-7f73-ace9-a9b0c107b802` 于2026-10-19T02:32:06.411Z发送。该消息紧接本聊天candidate12/v22完整精确文本及逐项核对PASS，明确按上下文引用该方案；`user_retyped_template=false`，不把机器文本伪称人类重新键入。旧授权不复用。

- execution：`8e41cfba-a41e-44dd-9ce8-d6365b3bb90e`。
- 固定发送窗口：2026-10-19新加坡14:00–14:40，即UTC06:00≤now<06:40；只读恢复最晚UTC08:40，不允许延长发送或重新enable身份。
- candidate：`f624a5f50ccb4fa4aa8b465f5a60a090f6325e78f60544b4c18f9ba2604ef243`；machine template：`bd46b276a422f75aa7e78b0568ab73b8412e265ad40897ce9fea2badc3645341`。
- source：`5c7e964dac83ebda0cf44743d49a546123735119`；运行/发布HEAD保持`7f3566cbfee6b22aded52403d5045681165a69d5`；packet containment anchor保持`30e954027bc1d5a74d683f0dcd8bd141aa7a678b`。
- 28项冻结批准清单全部保持原SHA；连同清单自身SHA与实际`remaining_window.py` SHA共30项引用文件另存精确manifest。完整方案/7目标/11保留/全部owner、binary、Core、IAM、gateway与预算绑定不变。
- 每目标最多一次、总计最多7次，UNKNOWN不重发；11份保留版本、其他环境、全部备份、旧pins与失败证据保留。cleanup=0、hold-release=0、bypass=0。

授权后已实际运行冻结review06 `window_support.py check`：新授权/private envelope/批准模板绑定闭合，58个本代资源停止、32权限false、封存anchor可用，source/provider/DELETE均0。实时GitHub核对精确HEAD PR CI39138927794已13/13成功、未解决线程0；#323仍Draft/Open、未合并，#307仍OPEN。PR CI不是main CI。

原`remaining-candidate12/status.json`与所有准备报告保留NOT_RECEIVED历史字节，不覆盖；这不否认后来单独收到的新授权。授权回执见`evidence/issue307-d1/delete-confirmed-v22-execution01/human-authorization.json`；实际原文、批准机器文本、完整聊天确认、host bindings、30项manifest和envelope分别保存在私有`remaining-candidate12/authorized-execution01`，尚未向任何worker materialize。

| 门禁/动作 | 当前状态 |
| --- | --- |
| 新人类按引用授权 | RECEIVED，原文7字节，旧授权不复用 |
| 30项引用与冻结候选 | MATCH；冻结源码/候选/模板/配置未改 |
| 授权后固定host check | PASS_PREWINDOW_CLOSED；58停止、32权限false |
| 当前HEAD的PR CI/评审状态 | 13/13 SUCCESS，线程0，Draft/Open |
| 定时续跑 | NOT_CREATED；冻结手册要求另行安排 |
| 窗口内fresh完整原生Core/IAM/18版本核验 | NOT_RUN，必须在窗口内执行 |
| 源Core六Renew、真实业务、H0/H1、7发送、H2/M1、恢复 | NOT_RUN |
| 全量D1 | NOT_ACCEPTED |

开窗前保持发送关闭、不启动source/case/enable/wire/sender、不签发送proof。真正执行仍须按冻结手册完成新鲜完整入口、资源关闭后的维护check、受限身份、六Renew、业务owner闭合与逐发送新鲜证明；任何漂移/失败/过窗停止新增发送并逐组封存。一项封存失败仍继续其他关闭；未确认项保留UNCONFIRMED，不重发UNKNOWN，不放宽门禁。

#323保持Draft、不Ready、不合并；#307保持OPEN。新回执与本矩阵为追加事实，暂不提交以维持已绑定HEAD；不得把这些新未跟踪授权证据当运行源码漂移，也不得据此忽略任何tracked或冻结字节漂移。

## 2026-10-19T02:52:59.415768+00:00 固定窗口调度追加事实

人类已单独选择“安排固定窗口续跑（推荐）”，目标为同一execution与固定日期；上表NOT_CREATED保留为授权初次留证时的历史快照。当前调度为 **ACTIVE / SCHEDULED**，已读回确认应用调度行的next_run_at与nominal均为 **2026-10-19T06:00:00Z（新加坡14:00）**。

- heartbeat：`307-d1-candidate12-v22`，绑定本聊天 `01a13579-573d-7a32-b1e1-3297fe78380a`；仅本固定日期有效。
- 执行范围、新授权、全部冻结字节、30项引用和HEAD均保持不变；旧07/10/11调度实际仍PAUSED；未创建重复调度，本调度尚未运行。
- 开窗后先做fresh完整入口、资源关闭后的Host.check及全部实时前置；仅UTC06:00≤now<06:40且全部前置通过才允许有限业务/发送。完成、失败或过期暂停同一heartbeat；无变化安静，不延长、平移、重开或重发UNKNOWN。
- 初次调度prompt字节对比检查的失败已保存：调度器仅去除了所提交prompt的末尾LF。提交字节与实际读回字节分别索引，实际DB与TOML的prompt逐字符一致；没有归一化人类授权原文，也没有修改冻结门禁。

回执：`evidence/issue307-d1/delete-confirmed-v22-execution01/scheduling-consent.json`、`scheduled-resume.json`、`scheduled-prompt.txt`与原始调度配置快照。本轮仅创建调度和留证，source/enable/case/wire/sender未启动，未向worker materialize文本，provider I/O=0、DELETE=0；窗口内新鲜核验及后续业务/删除/恢复仍NOT_RUN，全量D1仍NOT_ACCEPTED。#323 Draft、不合并；#307 OPEN。

## 2026-10-19 UTC06:00固定窗口执行终态（后续追加，覆盖当前状态）

**本次执行已失败封存，不能在本execution或授权下续跑。** 新鲜核验及真实业务通过后，P1发生一次exact DELETE并得到真实上游204及Core的CONFIRMED/ACCEPTED观察；P2在预发送阶段失败，立即停止后续发送并完成封存。以下为最新事实，前文的窗口未开启、NOT_RUN和ACTIVE调度状态保留为此前快照。

| 门禁/业务 | 本次实际结果 |
| --- | --- |
| 新授权原文、30项冻结引用、HEAD/PR状态 | MATCH；HEAD仍为7f3566cbfee6b22aded52403d5045681165a69d5，精确HEAD PR CI13/13成功，评审线程0 |
| 831源码/1305原始证据离线核对 | PASS，历史NOT_RECEIVED准备快照保留 |
| 窗口内完整原生只读入口 | PASS；18精确版本、保护状态、disabled IAM完整语义、三源Core/roles/ACL、9备份、case/topology/network/budget全入口 |
| 完整入口关闭后的Host.check | PASS；58个绑定资源停止、32权限false |
| 三源重启及限定读取身份 | PASS；重启前后Core指纹不变，发送身份仍关闭 |
| 六expected-head Renew及真实Processing/DirectData | PASS；6项新事实截止UTC06:40，真实业务owner/handles/pins/refs闭合，4项dispatch admitted，业务阶段send_claims=0 |
| P1 | 1次真实exact DELETE；signed wire证明上游204，Core观察CONFIRMED/ACCEPTED；缺失对账/verified absence仍NOT_RUN |
| P2 | sender exit1，当前调用stderr为“no rows in result set”；已消耗1个target预算槽，没有wire槽或转发见证，没有实际DELETE；不伪造Core NOT_SENT事实 |
| T1/T2/H2/M1A/M1B | 0次DELETE、NOT_RUN |
| H0/H1、H2 late hold、M1 mixed checkpoint | NOT_RUN |
| P1及其他只读恢复、发送后survivor原生重读 | NOT_RUN；失败后未重启Core/native或额外enable身份补做 |
| 预算 | 2个target槽消耗、1个wire槽；保留原字节，不删除、重建或补发 |
| 固定seal | 110/110步骤confirmed；三组实际disabled身份读回确认、gateway封存 |
| 封存后只读核对 | PASS；58个绑定资源及3个技术helper共61 CID stopped/PID0/noports/norestart，32权限false、7 case和7 recovery撤销；9备份原SHA匹配 |
| 定时续跑 | 同一307-d1-candidate12-v22已PAUSED，next_run_at=null；旧三个heartbeat仍暂停 |
| 全量D1 | NOT_ACCEPTED |

P1精确目标为 `dpp-lifecycle307-online/fixture/p1.csv@fed0102b-bc8b-4f47-b9e2-fae8d5c563ce`，attempt为 `009f4317-8855-4f99-80ec-cafc225f654e`。204、ACCEPTED和签名wire证据不替代完整原生库存的exact absence验收。其余6个批准目标未发生DELETE；11个survivor不在发送范围，窗口前18版本核验通过，封存后未重新启动native重读，不把此前证明伪称发送后新鲜证明。

当前有效阻断：

- **R1：批量发送顺序与完整库存门禁冲突。** 手册允许四send后四recover，但`RemainingDependencies`仍要求没有absence receipt的登记版本存在于完整库存。P1成功删除后、其absence尚未提交时，下一同namespace发送不能满足该集合。离线反例展示6个期望版本与5个模拟剩余版本的冲突；这是冻结代码的确定性顺序缺陷。未从封存后的Core提取P2原拒绝事务，因此具体原始拒绝错误仍保留核验限制。后续应先明确“逐份发送、只读确认absence、再下一同namespace发送”的有限组合，不放宽库存门禁。
- **R2：sender掩盖预发送原始错误。** `DeleteExactCopy`返回后无条件查询provider observation；claim失败且没有真实调用时，查询的“no rows”覆盖了原错误。须保留原始claim/deleter错误，明确无调用、已调用UNKNOWN及预算消耗，不伪造provider invocation、Cost或NOT_SENT事实。

原独立评审PASS保留，但本次原生执行暴露上述阻断；未修复、未做新的独立复评，候选不能宣称通过本次D1验收。没有修改冻结源码、binary、candidate或门禁。P1已受理，之后的新方案必须以实际剩余库存和旧attempt恢复事实为基础，不能重用原七目标整套授权。

完整入口实际技术读取76次；冻结root helper另有84次真实admin请求（70 GET、14身份状态PUT，均HTTP200，DELETE=0）。真实业务/S3读取与P1provider调用及Cost事实保留于原Core named volumes，未为提取计数而在封存后重启Core；amount=NOT_ASSESSED，不能把已提取的部分计数当全轮成本总量。所有备份、旧pins、失败事实、预算与其他环境保留；cleanup=hold-release=bypass=0。

终态与回执见 `evidence/issue307-d1/delete-confirmed-v22-execution01/window01/`，尤其`execution-terminal.json`、`seal.json`、`post-failure-readonly-audit.json`、`runtime-blockers.json`、`ordering-counterexample.json`与`heartbeat-paused-readback.json`。旧授权/准备/调度快照均未覆盖。#323保持Draft、不Ready、不合并；#307保持OPEN。

