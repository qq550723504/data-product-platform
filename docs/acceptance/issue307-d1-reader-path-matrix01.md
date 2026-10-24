# #307 D1：Windows reader 路径绑定修复验收

本轮状态：本地非删除验收通过；新 HEAD 尚未发布，PR CI 尚未运行。#332 保持锁定、Draft、OPEN、未合并；#307 OPEN。本轮不解锁、推送、启动保留源环境、写业务保护或删除，不复用旧授权。

根因已用真实 Windows 路径复现：测试 fixture 在未规范化的根目录上生成 reader 路径并签名，生产 Composition 则先将 control 规范化，再按精确字符串核对 reader 路径。同一个真实目录的大写、8.3 别名因此被正确的生产门禁拒绝。原源码在普通目录上的 Python 3.13.16 测试通过，故不能把 Python 版本本身当作根因。原 CI 没有打印两端完整路径，其确切别名拼写仍未确定。

修复只在测试 fixture 的首次文件写入、路径生成和签名之前复用既有 `indexes.unlinked_absolute`。该函数先拒绝原始路径中的链接及 `..`，再解析规范根。未严格解析尚未创建的 reader 文件；未采用 casefold/samefile 放宽生产门禁。生产 Composition、路径工具、Core、Go、工作流、R4 及依赖均无修改。

| 验收项 | 结果及范围 |
| --- | --- |
| 原失败保留 | P `16ec447f2c60ddf08a514207963f478bc70c7e09`；PR run `40099260254` attempt1，11成功/2失败；Windows job `120887885664` 原日志及绑定原字节保留 |
| 修复前原生复现 | 官方 Windows CPython 3.13.16：普通 TEMP 原9方法通过；同一物理目录真实大写/8.3 两种别名均复现 `COMPOSITION_ANCHOR_READER_PATH_BINDING` |
| 成功组合 | canonical 路径及 Windows 真实大写/8.3 路径，各走完整 mocked before17 → 两组顺序 worker → after17 → closure；Windows 此套件无 skip |
| 错误绑定反例 | foreign root、other group、other phase、`..`、未规范大小写、其他已知 native CID；重算 anchor/packet 哈希后仍精确拒绝，零 operations、零 run 目录 |
| 根路径反例 | 原始 `..`、真实符号链接或 Windows junction 在继承 fixture 的首次 IO 前拒绝 |
| 独立 Windows 3.13.16 | 26套件/52条 normal与-O命令全通过；每模式185方法，原有 evidence_indexes 平台 skip 各1；composition 每模式12方法、无 skip |
| 独立 Linux 3.13.16 | 26套件/52条 normal与-O命令全通过；每模式185方法、无 skip；network-none、UID10001、只读源码、无保留挂载的独立容器已退出/PID0 |
| 独立文件边界 | 两平台用202成员的新副本；禁止旧 worktree/control 读取的 Python 审计为0拒绝；它是测试审计，不声称 Windows OS 沙箱 |
| 生产约束 | 生产路径绑定、execution_freeze.ready/preallocation 均不变；失败旧 CI 仍拒绝业务准备 |
| R4 | `0446fde5985b51de7da5863b45df3f1b47b6167c`；ZIP `a60ddfeb8a81d4414bc56e1ddd05fa01ca7f2e0e9579a7d403783bdd318d115a` / 6515425 bytes / 781成员，未变 |
| 业务验收 | 源启动、业务 provider、保护写入、DELETE、预算重置、pins释放均0；17版本实时业务核验 NOT_RUN，完整D1不提前接受 |

独立评审及最终字节索引保存在 `evidence/issue307-d1/reader-path-fix01/`。本轮不重新跑浏览器：改动仅 Python 测试 fixture，原 P 的实际 browser CI 已通过。新本地 HEAD 必须另获本代受限发布授权，并在发布后通过该精确 HEAD 的完整 PR CI；本地通过不等于 PR CI 或 main CI。
