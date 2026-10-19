# #307 D1 P1新只读对账验收

本切片权威边界为 `issue307-d1-post-v22-p1-readonly.md`。这是新只读audit，不重开原v22 execution、不复用过期删除/读取权限。原v22一次P1 DELETE/204/CONFIRMED ACCEPTED及P2预发送失败、2个target预算槽/1个wire槽、旧pins和失败终态均保留。完整D1仍NOT_ACCEPTED。#323 Draft，不合并；#307 OPEN。

| 核验 | 事实与边界 |
| --- | --- |
| R1/R2 | 已收口：逐份发送前要求前序absence，保留claim/observation原错误；源f59bf8b0对应13项PR CI成功，非main CI |
| P1源码 | Go recovery/remaining-IAM/outbox三包未缓存通过；新精确Outbox及既有三项真实scratch PG共4/4通过，provider为模拟事实 |
| B1/B2/B3 | 关闭异常仍逐项stop已知CID；实际DSN/Core名称/DNS绑定；Docker stdout/stderr原字节分别保留；独立反例均通过 |
| B4 | 真实Windows manifest暴露反斜杠/正斜杠不一致，原audit01在启用前阻断，源/身份/DELETE=0；原manifest、4个新增停止CID和两份false配置保留 |
| B4修后 | 生成及消费统一相对路径，拒绝绝对/上溯/规范化碰撞；实际backslash输入的Enabled成功组合与deadline扩权拒绝回归，normal/-O各8通过，独立原反例修后PASS |
| 新只读audit02 | 669e16cb-c671-4f36-92ad-ab3a3b051b81；绑定manifest cd7cd06bfb46ab0703252af9f2668ed6453a4da4b84e860a736d80fa979ddb14；最多40分钟，无发送能力 |
| audit02实际manifest复评 | PASS，报告c690ebd0；此准备复评不等于实际读取成功 |
| audit02实际原生/Outbox/Asynq/Core absence | FAILED；worker原stderr为lifecycle storage observation unknown，未产native-precheck/absence；未完成项仍NOT_RUN |
| audit02最终身份与物理封存 | PASS；6/6关闭、2profilefalse、4身份freshdisabled；旧预算/控制/9备份原字节一致。失败路径未产Core-after，不声称Core行级不变 |
| B5/B6修复与新audit03 | 路由初始化移到首个journal前；Start/Observe原cause穿过SDK/inspectionError保留，fail-closed UNKNOWN分类不变；真实PG+SDK loopback通过，源独立复评04通过，剩余阻断0，audit03尚未prepare |
| 全量D1 | NOT_ACCEPTED |

只读audit02的实际source HEAD为86cec9c804f2d54ffc74e13aa210547a1ecdca7a；源独立复评03 SHA aa8e3d0312da62cc5beada3aa96dbab1724c11e37127deb2e402791701e6eb0d。SOURCE复评不是实际manifest复评，也不是删除授权。原audit01阻断复评SHA bbb66dd3f41c1a81d8b5f3880a718c307fb4550bba56192f4eb0338bcd6a0d9f；原v22终态未改。

证据目录 `evidence/issue307-d1/post-v22-p1-readonly01/`保留第一次准备及阻断，`post-v22-p1-readonly02/`保留当前新源、独立报告及实际审计。范围草案见 `issue307-d1-post-v22-remaining-six-preparation.md`；六目标和11保留仅按旧冻结身份列出，尚无新候选、删除execution、窗口或授权。原P2/T1/T2占用需要新只读核实和有限可信闭合，不能用缺少调用/成本猜NOT_SENT，不能退旧预算或释放旧pins。

## audit02失败及修后组合回归

旧audit02终态原字节保留，不能复用或重跑。静态调用链证明旧root在前检后才配置Outbox路由，journal必需路径无法成功；原83字节stderr未保留底层cause，不能声称唯一还原了那次内部错误。Start与Observe两层包装及inspectionError均保持errors.Is分类和原cause，CLI原stderr分通道保存。

新PG测试显式清空全局路由，不依赖planDatabase的默认初始化；真实SDK loopback负例在wire前被拒、Start事务回滚；随后使用P1实际pilot.WithRecoveryRouting启动helper，成功产生一项真实测试HTTP/start/obs/Cost；第三步真实HTTP后撤路由导致observe事务失败，wire2/start2/obs1，原cause仍存。后一次调用的Cost未成功落库，保持未知/待对账，不能声称0成本或已计费；这是受控故障测试，不访问保留原生环境。

audit02固定root helper实际26次admin请求（21 GET、5身份状态PUT，HTTP200），DELETE=0；SDK请求量NOT_ASSESSED，amount=NOT_ASSESSED。最初只读投影错读status字段的报告保留，后续02报告读取真实http_status，不改原始start/observation。

B5/B6源终复评04：`c0b13b02488144eec937bcf54695032ce7606008b784dc05a13a68cd96cc51fb`。旧86 inspector覆盖原cause反例失败、当前源通过；首次overlay路径无效尝试单列INVALID，不冒充旧源测试证据。
