# #317 Draft review checkpoint

2026-10-13，review 基线 `ce8b48083602110eb2c2adea147df192ca9bb4f1`。
本补充不修改两份冻结 receipt manifest 或原阶段结果。
两份清单合计120条：119条证据文件仍匹配当前Git blob；另一条是本轮已修复的
driver源码，必须按冻结阶段commit `ce8b48083602110eb2c2adea147df192ca9bb4f1`
验证，不能拿新driver去匹配旧源码哈希。补充绑定见
[receipt source revisions](evidence/issue307-d1/receipt-source-revisions.json)。

有效 P1：`run-confirmed-race.py` 用 `assert` 检查确认、备份、拓扑及旧 attempt，
Python `-O` / `PYTHONOPTIMIZE` 可移除这些安全前置条件。全部替换为显式拒绝；
相关检查在优化解释器下同样执行。

同根因自查：失败处理恢复旧 reader/admin 路由，不等于封存环境继续隔离。
改为只有本次已更改网关时才封闭全部对象/admin 路径，仅保留健康检查。
无效确认、已封存 scope、过期/disabled 权限拒绝不改网关；拓扑拒绝不启动 helper。
已提交的 `confirmed-race-sealed.json` 消耗这一次确切批准，所有 phase 在读取私有
凭据或 Docker 操作前拒绝重放，不能重新 mint 旧批准的 authority。

验证：标准库 unittest 的 7 组回归在普通解释器和 `-O` 下均 PASS。
涵盖全部 12 个 phase 的已封存拒绝、错误确认、disabled/过期权限、备份失败、
旧 attempt、foreign container/image/volume、host port、Tmpfs、network/schema
拒绝，以及 helper 失败后 health-only 网关。优化 CLI 子进程也覆盖全部 phase。
已加入既有 `demo-lifecycle` CI；无新依赖、Core、migration 或 gate 改动。

本轮没有真实 Docker/provider I/O、DELETE 或重建验收环境；只运行隔离临时目录、
模拟依赖及已封存 entry point 的拒绝验证。实际两次 DELETE 仍只对应
`ced51ff187c28be6ff043307c228fc1cfd55513f`，不声称新 HEAD 重跑删除。
原证据、卷、保护、备份及 UNKNOWN 观察均保留。

完整 D1、真实 Execution/认证 DirectData 的 LC-11/12、material-reference owner
事实、原生 archive/restore rerun 和剩余 in-flight fault 仍由 #307 跟踪。
人工解释暂缓；PR 保持 Draft，不自动 Ready/merge，不进入 D2。

## Ready-triggered review checkpoint

用户于2026-10-13追加授权条件式收口；PR 在12:04:14Z转为Ready，
触发的评审基线为 `9020ac77da900b5d002b962a1eb2e55b9197a5e7`。
前述Draft阶段记录保留其原语义，不作为Ready评审批准。

有效P1按同一provider执行边界修复：预期exit93的发送helper返回后立即恢复
既有禁止DELETE的网关配置，不能把响应丢失后的发送窗口留到后续seal阶段；
异常路径仍封闭全部对象/admin I/O。worker-a/worker-b在权限校验后、启动helper
前执行相同container/image/volume/network/schema preflight，不能拿漂移环境的
inventory签署原incarnation的absence receipt。复用既有配置和preflight，无新Port、
Core、SQL、队列或business gate修改。

先新增回归，旧代码出现17个失败断言（包含两个worker的7类拓扑拒绝），
确认覆盖评审根因；最小修复后9组测试在普通Python与`-O`下均通过。
新增覆盖预期exit93后DELETE封闭、恢复启动顺序，以及两个worker的拓扑漂移拒绝；
过期/disabled recovery权限仍在Docker操作前拒绝。
本轮没有重新开启保留环境或执行DELETE；119份当前证据及1份历史driver source
仍按原manifest/补充commit绑定独立核验。仅在新HEAD评审👍与CI全通过后才允许
合并，再单独验证对应merge SHA的push/main CI；完整D1/人工解释边界不变。
