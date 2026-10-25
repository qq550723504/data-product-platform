# #307 D1 历史测试资源清理收口（2026-10-24 新加坡时间）

状态：`PASS_BOUNDED_RESOURCE_RETIREMENT_COMPLETED`。本次完成已批准的 Docker 测试资源清理，#307 仍 OPEN，#332 仍 Draft/unmerged/locked；不代表完整 D1 或新的业务删除验收。没有创建新候选、execution、预算、窗口或调度。

## 实际完成

| 项目 | 结果 |
| --- | --- |
| 精确旧容器 | 986/986 已删除；整机容器 2054 → 1068；每个完整 CID 有原 inspect、原始日志、意图及确认不存在回执 |
| 精确旧测试卷 | 106/106 已删除；整机卷 3170 → 3064；70 个无文件内容卷 + 36 个有完整逻辑备份和当前物理归档的 PG 测试副本 |
| 197 个候选卷的其余部分 | 91 保留：37 个预算、4 个非空 Redis 队列、18 个独特 PG 恢复状态、32 个 MinIO 对象/恢复状态 |
| 全部预算事实 | 58 个预算卷保留；51 个候选07–23预算 + 7 个独立演练预算；现场文件与原始 tar 及清理前后语义树 SHA 全部一致，共5个既有预算文件，不重置、不退款 |
| 原始材料 | 54,310 个 #307 原文件 SHA 保持；旧失败、UNKNOWN、provider attempts/成本、pins、权限、备份不回写 |
| 当前 PG 额外保护 | 36 个测试副本当前物理内容完整压缩归档并逐文件/UID/GID/mode校验，合计347,632,160 bytes；演练独有数据也保留 |
| 保留运行依赖 | 7 个源/原业务服务、candidate22 的86个、candidate23 的86个、6个共享netns根、139个外部关联者保留；24个清理开始快照中的运行容器状态不变 |
| 后继门禁 | 完整 catalog 实际通过：容器/卷精确回执、原始材料、PG归档、现场不存在/保留状态、当前预算字节均核对；归档 State 不充当现场 State |
| 回归 | 退役29个方法在普通及 -O 各通过；另有既有准备/完整入口31个方法在普通及 -O 各通过；收口为owner复查，不冒称另派独立代理 |

## 空间实测

| 边界 | 字节 / 结果 |
| --- | --- |
| 已删卷原实际分配 | 2,769,723,392 bytes（2.580 GiB），由原现场du -sk获得 |
| 已删容器逻辑可写层 | 11,264,000 bytes（10.7 MiB），与物理分配分开统计 |
| Docker内部文件系统净空闲增加（全机观测） | 2,857,058,304 bytes（2.661 GiB） |
| 宿主C盘清理边界实测 | 27.967 → 27.559 GiB，净变化 -439,144,448 bytes（-418.8 MiB） |

宿主盘本次没有增加空闲空间。必要压缩归档及证据保存在宿主盘；没有停止Docker或离线压缩VHD，也没有prune/down -v/force。内部回收量不作为宿主释放量。测量原件及时间见物理结果回执。

## 保留项与未完成边界

37个候选预算卷必须保留，另有21个范围外预算一起核对。4个非空Redis卷未证明可弃；18个独特PG恢复卷、32个MinIO对象/恢复卷未纳入本轮完整重建证明，保留原内容。这些91个卷逐项有名称、类型、内容量、证据SHA与保留依据，不能将它们标作已回收。

`dpp-agent-q2-gateway-20261023` 在首个清理rm之前已退出0（退出原因未据此判断）；清理未停止、重启或删除它。所有非目标原CID仍存在且descriptor一致，其他卷没有消失；该时间线单独留证，不能把历史归档当现场状态。

业务D1仍未完成：本次没有启动源环境、执行17版本原生业务复核、写保留期/hold/IAM、执行provider对象DELETE、释放pins或复用任何旧业务授权。candidate23及旧授权/预算/失败均保留。P仍为e54967e822847e97316e703ee413ffc1298cb627，R仍ecf40e6c7a9b3aa11166e6a56a5163a29a15691d；精确PR CI run39983308178/event=pull_request 13/13成功，不是main CI。退役接纳源码只在隔离维护分支，本轮未更新#332分支。

## 证据

- 完整已删容器名称/CID：`evidence/issue307-d1/resource-retirement01/removed-containers01.json`
- 全部197个卷的精确决策：`evidence/issue307-d1/resource-retirement01/volume-decisions01.json`
- 91个保留依赖逐项依据：`evidence/issue307-d1/resource-retirement01/preserved-volume-dependencies01.json`
- 原始空间/身份回执：`evidence/issue307-d1/resource-retirement01/cleanup-physical-result01.json`
- 最终终态：`evidence/issue307-d1/resource-retirement01/terminal01.json`
- 私有归档：`C:\Users\Henry\.codex\fixture-private\resource-retirement01\capture06`；私有索引 0ba00a69cdf9a2760cb40247a9ae05f3e2af0105b5385167990c4114911585a7，13043个成员已校验。容器回执1972、卷回执424、当前PG归档108个成员分别另有固定索引。

根因修复：33的硬编码遗漏7个演练预算；当前全宿主58、原范围40、加candidate23后的43是不同集合。新入口按原packet/allocation/演练事实取精确卷身份并集、去重并拒绝冲突/扩张；原capture04失败和capture05范围过宽的未完成验证不改写。未扫描或冻结其他会话私有目录作为新门禁依赖。
