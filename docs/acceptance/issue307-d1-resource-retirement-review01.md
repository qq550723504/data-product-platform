# #307 D1 退役接纳独立评审及验收矩阵

评审对象：本地 `81c8a1130d83379c8614d3b3082b237fd2073f73`。本记录追加评审事实，不覆盖原清理终态、失败、预算、pins 或原索引。原有限契约和实际结果仍分别由 `issue307-d1-resource-retirement-contract.md`、`issue307-d1-resource-retirement-result01.md` 持有。

状态：`PASS_BOUNDED_INDEPENDENT_RETIREMENT_CLOSURE_REVIEW`，当前有限切片剩余有效阻断为 0。

本轮仅检查退役接纳、预算核验及清理证据；没有追加 Docker 清理、创建业务候选或执行任何业务对象删除、保护状态写入。评审代码仍为原提交，未为得到通过而修改冻结计划或旧回执。

## 验收矩阵

| 检查 | 结果及范围 | 新证据 |
| --- | --- | --- |
| 独立源码评审 | PASS；当前有限切片有效阻断 0。29 项独立测试在普通、`-O` 各通过；录制快照组合接受 986 容器 / 106 卷 / 58 预算，并拒绝 PID、回执索引 SHA 和预算字节变化 | `resource-retirement-review01/independent-source-review01.json` |
| 独立证据评审 | PASS；公共 182、私有 13,043、容器回执 1,972、卷回执 424、PG 索引 108 项逐成员 bytes/SHA 一致；36 个实际 PG 压缩档解包后 80,861 个成员与原离线内容清单一致，全部早于对应卷删除完成；58 预算 tar、29 原引用及 344 条原挂载关系匹配 | `resource-retirement-review01/independent-evidence-review01.json` |
| 本轮非删除回归 | PASS；退役 29 项和既有准备/完整入口 31 项，普通及 `-O` 各 60 项。测试使用隔离临时材料和模拟，不代表真实 provider 验收 | `resource-retirement-review01/regression01.json` 及原始 stdout/stderr |
| 原公共证据索引 | PASS；182 项原始字节/SHA 与原固定索引一致 | `resource-retirement-review01/original-index-audit02.json` |
| 固定完整 catalog 的现场只读复验 | PASS；原 54,310 个文件、986 容器和 106 卷的精确回执及当前缺失/保留状态、全部 58 个预算当前物理字节均通过；99 个计划内 `live_required` 以及额外依赖和其他会话保全检查仍按实际入口执行 | `resource-retirement-review01/fresh-catalog02.json` |
| GitHub 实时边界 | PASS；P=`e54967e822847e97316e703ee413ffc1298cb627`，精确 PR CI `39983308178`、`event=pull_request`、13/13 成功，#332 OPEN/Draft/unmerged/locked，#307 OPEN | `resource-retirement-review01/github-boundary01/result.json` 与完整只读请求回执 |
| 业务 D1、17 版本原生核验、新保护写入及剩余六份删除 | 本轮 NOT_RUN；旧授权不复用，不创建新 execution、预算、窗口或调度。PR CI 不是 main CI，也不覆盖本地未发布评审记录 | 本记录及 `resource-retirement-review01/scope01.json` |

证据路径均相对于 `docs/acceptance/evidence/issue307-d1/`。本轮首个审计包装脚本遗漏必填 `expected_status`，严格入口在进入 catalog/Docker 前拒绝；原失败保存在 `fresh-catalog-failure01.json`，随后仅纠正调用参数。未改源门禁或旧材料。两位评审的重复原件扫描取消事实及实际完成范围也分别保留，不计为完整现场 PASS；全量现场结论来自 root 一次完成的固定完整入口。

## 历史清理结果的接纳范围

原终态记录的 986 个容器、106 个测试卷已经清理。本轮不重复执行。91 个候选卷、全部 58 个预算及旧 pins、失败、成本和 provider attempt 事实保留。36 个已退役 PG 副本有冻结逻辑备份和删除前的当前物理归档；归档不作为当前运行状态。

空间结果保持原测量边界：Docker 内部净增加空闲 2,857,058,304 bytes；宿主 C 盘当时净空闲变化为 -439,144,448 bytes。未做新的空间释放测量、Docker 停机或 VHD 压缩，不能把内部回收量改称宿主盘已释放量。

## 收敛分类与下一业务方案的设计前置

本轮维护期间必须证明其他会话与保留资源未被清理影响，因此原计划中的精确 `running_protected` / `live_required` State 比较仍是本轮保全证据的一部分，不能改写旧计划以放行漂移。

但将同一份整机运行快照永久用于未来候选，会把其他会话正常停启及 C22/C23 的历史资源继续耦合到新准备。这是后继方案必须先解决的架构边界，分类为 `FOLLOW_UP_BEFORE_NEW_CANDIDATE`，不否定本次清理，也不自动扩大本轮维护范围：

1. 维护期间保全结论继续由固定归档、实际执行源码、精确回执和当时前后快照持有。
2. 后继历史接纳继续逐字节核对不可变证据、预算、pins 与真实退役事实；不得把缺失或 Docker 失败投影成成功。
3. 新方案另行列出真正必需的当前依赖和角色约束，不能机械冻结无关会话历史 PID/StartedAt，也不能用历史归档 State 代替新鲜关闭证明。
4. 后继更改必须补齐“无关会话合法停启可接受、必需 owner/预算归属/挂载/字节漂移仍拒绝”的组合验收，之后才准备新候选；旧 C22/C23 execution 不续跑。

`originals` 只钉住既有列明文件，允许追加新文件；旧失败、预算、pins 和原权限事实仍不可回写。本轮没有新增此后继执行设计的实现或授权。

## 发布边界

本地维护分支与 #332 锁定分支分离。本轮不推送、不解锁、不 Ready、不合并；#307 保持 OPEN。原 182 项公共索引、13,043 项私有索引及旧清理终态保留，新评审材料由 `resource-retirement-review01/evidence-index01.json` 独立钉住，索引不包含自身和其校验回执，避免自引用或重写历史。
