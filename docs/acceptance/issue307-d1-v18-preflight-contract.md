# #307 D1 v18 完整只读核验与新候选边界

目标：修正 v17 window01 的宿主误报，先评审提交的完整核验入口，再冻结 candidate08/v18。旧失败、原始 policy/provider raw SHA、全部 request journal、旧授权与候选保持原字节。#323 Draft、#307 OPEN；本轮没有发送授权。

唯一当前入口为 `examples/lifecycle-d1/verify-remaining-native.py`；文件语义检查由 `apps/platform/cmd/lifecycle-fixture-readonly-check` 完成。`freeze-remaining-executable.py` 复用同一 `collect_group`，窗口时不得另写 raw-SHA/HTTP-status 汇总。历史 v17 entry 和 window support 仅用于对应历史范围，不能作为 v18 启用入口。

Reuse Check：现有固定 reader 已正确使用 MinIO `policy.ParseConfig/Equals` 判策略并处理 NoSuchObjectLockConfiguration。宿主只补文件、owner、路径、时间与 Core 的绑定，不自建 IAM、policy evaluator、workflow 或队列。继续使用已引入 MinIO pkg/v3、Docker、Go 标准 JSON、既有 Core fingerprint 与 attester mount contract；OPA 等第二套策略引擎不需要引入。

有限 invariant：

- frozen candidate SHA 与 bindings SHA分别核对；baseline IAM snapshot与policy file SHA、原生 receipt及Core fingerprint均绑定到candidate/bindings，拒绝用任意两个目录互证。
- 只启动精确绑定的 Core/native和新建GET/HEAD-only技术helper。所有case/sender/wire/root-enable保持停止、实际mount中的permission=false；root技术配置独立挂载，不进入worker。保留全部helper、volume、备份，无cleanup。
- helper实际CID、image、mount、network、readonly/cap/noport/norestart、exit0、真实起止时间及Core前后相等必须通过；一组失败立即停止后续组，finally逐项关闭已启动资源并保留原失败。未关闭不能报整轮PASS。
- 完整receipt比较group、18份精确version/content/bytes/hold/retention、库存hash、disabled identities与policy全集。语义比较只使用MinIO；加action、扩resource、换version、去condition、改effect、缺失/额外policy、畸形或尾随JSON必须拒绝。
- raw provider SHA是不可变审计事实；Encoder落地文件SHA另列，不把不稳定序列化顺序当授权identity。JSON必须为单值，exact bytes比较不经float64舍入。
- 固定reader拥有provider响应语义；真实34请求中的14个400在reader证明NONE保护时可以成功。journal层保证GET/HEAD、完整start/observation配对及transport事实，不用2xx-only规则另造冲突门禁。技术provider调用如实计数，不称provider I/O=0。
- 离线回放使用真实公开回执、实际编译Go CLI和隔离的假技术transport，不读取fixture-private或真实凭据；它不替代新鲜原生验收。

新编译期contract：execution `b16a578d-6997-4f8c-b6ad-470bafba2b1f`，candidate08/v18，拟窗口 **2026-10-18 02:00–02:40Z（新加坡10:00–10:40）**。此为新提案，尚未授权或调度。sender/IAM复用pilot的唯一execution/profile/window常量，拒绝v17；新key、owner、recovery Redis、预算及配置均单独分配。既有18版本与9份备份可只读重新核对，不能借此复用旧授权。新确认必须包含所有新SHA/CID/预算、7目标和11保留版本；未冻结字段不得用旧值代填。

非目标：本轮不运行真实业务case、不Renew、不启用IAM/read/send、不写human-confirmation、不签发送topology、不DELETE、不hold-release、不bypass、不合并。准备阶段root仅check-disabled读取；失败封存保持旧事实，任何需要重新打开权限的后续都要先完成新精确确认。完整D1仍NOT_ACCEPTED。
