# #317 Draft review checkpoint

2026-10-13，review 基线 `ce8b48083602110eb2c2adea147df192ca9bb4f1`。
本补充不修改两份冻结 receipt manifest 或原阶段结果。

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
