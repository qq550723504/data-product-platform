# #307 D1 v22后R1/R2非删除验收

本轮有限修复合同见 `issue307-d1-post-v22-repair.md`。独立复评结论为 `PASS_BOUNDED_INDEPENDENT_POST_V22_REPAIR_REVIEW`，剩余有效阻断为0；本结论只接受R1/R2修复，完整D1仍为NOT_ACCEPTED。#323保持Draft、不合并；#307 OPEN。

| 验收项 | 结果与边界 |
| --- | --- |
| 前序Core absence门禁 | PASS；在native inspection与持久预算前核对同workspace的全部前序immutable copy回执；完整Core fence/库存门禁未弱化 |
| 四次业务发送组合 | PASS，loopback模拟provider；真实SDK/wire/LIST逐份交替，提前进入下一份被拒且不消耗预算，四份保留版本完整留下 |
| authority/budget/Core claim拒绝 | PASS；保留原错误，不查询不存在的observation，不创造provider invocation、Core NOT_SENT或成本 |
| 真实调用后观察存储报错 | PASS，loopback模拟；无论后续读回成功或失败，均保留transport原观察与原始存储错误；H2/M1B不吞错推进 |
| UNKNOWN与预算 | PASS，loopback模拟；UNKNOWN不重发，已消耗slot不回收，O_EXCL碰撞不覆盖原字节 |
| 真实PostgreSQL | PASS，独立scratch 3/3；顺序缺失回执、claim并发/late revoke成本、absence原生调用凭据与完整plan membership；provider为模拟事实，未访问原生对象 |
| Go未缓存回归 | PASS，remaining/reader/remaining-IAM/pilot/storage五包 |
| Python当前入口 | PASS，完整组合、native verifier、freeze closure各normal/-O；旧候选严格历史审计v17/v19/v20/v21/v22全部通过 |
| v22历史 | PASS，原冻结runtime 831 source/1305 raw；原checker、packet、手册、失败终态均保留，未用当前源码替换历史 |
| 独立复评 | PASS，原成功读回反例及修后整包由独立reviewer实跑；PG日志由reviewer只读复核，分别标注 |
| 原生P1精确缺失对账 | NOT_RUN；真实204/ACCEPTED不等于absence。旧只读期限已过，必须独立新只读绑定 |
| 剩余六份新候选/窗口/授权 | NOT_PREPARED；需先完成P1只读对账和新库存，不复制旧七份方案或续旧execution |
| 本轮源/原生DELETE | 0；旧execution仍终止，旧预算、pins、失败证据保留 |

证据目录：`evidence/issue307-d1/post-v22-repair01/`，独立报告位于其 `independent/independent-final-review.json`。原v22实际执行仍是1次DELETE、P1缺失未验证、其余6份未发送；原P2错误原文仅`no rows in result set`，不能靠本轮修复反向编造已丢失的claim错误。完整库存与批发顺序的冲突是确定的代码/合同根因；旧P2事务的实际拒绝原因仍需新的只读Core审计。

保留的非删除失败包括：scratch网络默认地址池耗尽、internal网络无host端口导致测试启动拒绝、测试最初漏两份精确产出、PG反例误将“额外无关版本”当作Core exact absence禁项，以及独立评审发现的成功读回吞错反例。均保留原日志；修复未放宽Core、IAM或历史校验。

P1独立只读切片须在claim前选择精确Outbox event，不能用handler拒绝其他事件来冒充未触及它们。复用现有Outbox/Asynq、RecoveryService和StorageInspector；新只读owner/期限、仅inspector临时启用、root与worker分离、读取与Core追加absence、失败关闭均须独立核验。此后续设计不是新的删除授权，也未在本轮修复验收中冒充完成。
