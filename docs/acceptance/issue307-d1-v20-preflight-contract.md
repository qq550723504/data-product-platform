# #307 D1 candidate10/v20 有限准备合同

本切片仅重新准备已过期、从未获授权的candidate09/v19。继承v19策略语义、完整核验与P/H/M有限顺序，不扩大18版本、7目标、11保留及9份备份的范围，不修改旧candidate/status/失败证据。

用户同意继续非删除准备并询问今天是否可安排。新提案窗口为 **2026-10-18新加坡18:00–18:40（UTC10:00–10:40）**，execution **67de3acd-3a9d-4ece-bebb-cd53d0ab3ac5**，profile **candidate10/v20**。这不是精确删除授权，不创建调度，不复用旧授权，不启用发送。若新授权/前置未就绪或过窗即不执行，不顺延。

唯一运行常量继续由pilot.Remaining*共享；仅更新编译期execution/profile/绝对窗口，生成新的keys、binary、owner、Redis、持久预算和冻结载荷。原source数据、plan、expected-head、Core和全部备份保留，准备入口必须先核对07/08/09全部owner实际关闭及permission=false，再复用同一完整只读sealed_gate/collect/aggregation/close。禁止增加第二套native或策略判定逻辑。

先完成源码差异及历史CI归属的独立评审，再冻结新候选；新候选之后另实际运行完整CLI并做最终绑定独立评审。旧v17/v19 strict checker分别在原f3/650源码及各自Git证据锚上完整运行，当前10单独严格校验，不放宽或改写历史checker。

验收要求：目标Go包、完整入口normal/-O回归通过；18版本bytes/SHA/库存/hold/retention、disabled IAM/policy、Core前后与9备份一致；新17 operator/7 case封闭拒绝、32权限false、实际拓扑与空预算可核；所有本次资源finally关闭，保留失败与原始HTTP状态。确认文本必须列全执行、冻结绑定、7目标、11保留、固定窗口、次数和UNKNOWN限制。

当前#323保持Draft、#307保持OPEN；新授权NOT_RECEIVED、新调度未创建、旧heartbeat PAUSED；DELETE/cleanup/hold-release/bypass=0。真实业务case、Renew、enable、human-confirmation materialization、发送proof与真实provider attempts均NOT_RUN；完整D1仍NOT_ACCEPTED。独立复评/精确HEAD PR CI不授予删除权限。

独立评审发现并在冻结前修正旧recovery ValidUntil硬编码：现在只从END+2h计算，本提案只读恢复上限为2026-10-18T12:40:00Z（新加坡20:40）。该期限不允许重新开启身份或延长发送窗口；最终核对7份实际recovery profile的截止和无发送credential，保留修复前评审发现。
