# #307 D1 candidate11/v21 execution01 授权与窗口准备

状态：**AUTHORIZED_WAITING_FIXED_WINDOW**。本轮收到本聊天真实人类新授权，原文与批准机器模板分别保存；按引用授权，user_retyped_template=false，旧授权不复用。原 candidate11 status、冻结确认文本、手册及所有旧失败保留原字节。

- execution：`602acafe-065a-4385-b739-5776408f5448`。
- 固定发送窗口：**2026-10-18 新加坡22:00≤now<22:40 / UTC14:00≤now<14:40**。只读恢复上限UTC16:40；不能延长、平移或重开发送窗口。
- 7 exact目标每份最多1次、全局最多7次；UNKNOWN不重发。11保留版本、全部其他环境、备份、旧pins及失败证据保留；cleanup=hold-release=bypass=0。
- 授权消息：`msg_01a14ecd-d08b-78e1-85f0-3ba0aeb00b5f`；公开授权SHA256 `1bef2f6e7bb5739a6d85349cab3bdd87f0f2f5fcca8a5faf4c08fb0d4c2999f7`。实际原文UTF8 568字节，段落及末尾LF原样保留；26项批准引用字节均匹配。
- 当前HEAD `8eef47a2edfbd9df2dc2a83467d9361a3fa4ec96`；精确该HEAD的PR CI39121716342、13/13 SUCCESS、event=pull_request，评审线程0；这不是main CI。#323 Draft/Open、未合并；#307 OPEN。
- 本聊天一次性续跑 `307-d1-candidate11-v21` 已ACTIVE，2026-10-18新加坡22:00；保存prompt及TOML实际读回。旧v17/v20调度保持PAUSED。电脑及桌面应用须保持运行。

| 验收项 | 本轮实际结果 |
|---|---|
| 新人类授权、冻结引用、独立报告绑定 | PASS；实际原文/批准模板/envelope分别保存；冻结既有独立R1及最终评审PASS，不声称本轮有新独立授权评审 |
| 当前离线源码/证据 | PASS，829 source / 1122 raw；历史NOT_RECEIVED快照不覆盖 |
| 冻结review05关闭检查 | PASS，58本代容器stopped/PID0，32 permission=false |
| 全部保留资源关闭 | PASS，248 CID stopped/PID0/noports/norestart |
| 实际profile与保留资源 | PASS，7 case/7 recovery实际字节及统一END语义，9备份SHA，3 budget新execution/完整挂载集合/空 |
| 额外宿主检查错误 | 两次只读脚本schema字段错误分别保留；按实际operator/root不同schema的03核验PASS。无候选或权限修改，无source启动 |
| 窗口内完整fresh Core/native/IAM/库存/保护状态/拓扑 | **NOT_RUN，开窗后必需**；窗口前关闭检查不能替代 |
| 源Core六Renew、业务、受限enable、sender/wire/只读恢复 | **NOT_RUN**；目前未向worker materialize human文本，发送权限关闭 |
| 本轮provider调用 / 真实DELETE | **0 / 0**；旧准备技术读取和旧失败成本事实原样保留 |
| 完整D1 | **NOT_ACCEPTED**，按未来实际运行量逐项填写 |

窗口内先核最新人类消息、时钟、精确HEAD/PR CI、全部冻结SHA，再运行冻结完整只读入口。入口关闭后、重新启动源前通过review05 check；只有全部新鲜门禁通过才按原有限顺序继续。失败或过窗立即停止新发送并逐组固定seal；没有fresh身份读回或enable owner闭合时记UNCONFIRMED，不宣称整轮PASS。完成/失败/过期暂停同一调度。

完整范围与有限顺序仍以 [冻结精确方案](issue307-d1-candidate11-confirmation.md)、[冻结窗口手册](issue307-d1-v21-window-runbook.md) 为准；追加证据见 [execution-preparation.json](evidence/issue307-d1/delete-confirmed-v21-execution01/execution-preparation.json)。本轮新增证据及本矩阵保留未跟踪状态，不改变绑定HEAD或冻结运行字节。
