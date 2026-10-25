# #307 D1：CI 可移植性与查询导航收口

本轮仅本地修复、隔离非删除回归、独立评审和受限发布提案。#332 保持 locked / OPEN / Draft / unmerged，#307 OPEN。旧授权不复用；源环境、业务 provider、retention/hold/IAM、业务删除、预算与 pins 均无操作授权。

## 事实与最小设计

1. **Python 路径依赖**：组合测试调用实际 `known_sources()`，却读取 `C:/Users/Henry/.codex/worktrees/d1-c22/...`。测试现在注入仓库内同一不可变 candidate22 文件（SHA256 `d7976ba7462ae30f53783b5111ed84d0656a58e62b4c5610bc93a73e538183db`）；生产路径、SHA/身份校验和 missing/drift 拒绝不改。
2. **离线测试时间依赖**：三个组合套件的保存候选期限与宿主当前时间混用，过期后成功样例失败。仅在各自测试上下文固定 `NotBefore` UTC；生产 `native_protection` 和过期反例仍真实执行。
3. **测试交接**：原失败 trace 证明 ALL 与 NO_ATTEMPT 都有25行，行数不足以证明筛选已提交。测试逐步等待精确 URL、aria-current、分页/跨区 href；attempt 连续四次点击也逐页核对25/50/75/100及分页范围。无 sleep、自动 retry 或降低最终参数断言。
4. **页面查询身份**：仅加测试等待后，仍出现正确 RSC 200但客户端未提交。新受控反例在原未插桩构建中保持下一页 GET 挂起，证实旧 Suspense 身份会让新 query 与 UNKNOWN 无法提交。页面现在用固定顺序、规范化 filter/offset及保留链接上下文作为每个独立 Suspense 的稳定 key；不同分区保留独立揭示，不采用整页 key、聚合等待、框架 fork 或依赖升级。依据 [React 官方导航 reset 机制](https://react.dev/reference/react/Suspense#resetting-suspense-boundaries-on-navigation)。

原间歇失败的 React 内部调度原因未宣称已被证明；相似上游 issue、版本号和插桩构建16次通过均不作为验收。验收针对本应用的查询身份、真实导航及受控 pending 行为，使用未插桩构建。

## 验收矩阵

| 条目 | 状态 / 证据 |
| --- | --- |
| 原远端 P / CI | `1d3e417bd059aaaee099161cf6e12b4c43fed81c`；PR run `40090889280` attempt1，9 success / 3 failure / 1 cancelled，原失败不覆盖 |
| 实际 CI 检出基线 | merge `62ae34202c97c5906b4958f06d8f035787eb7aed`，tree `74163482794851f535dc15f649df89552d4d4af2`；本地先精确整合固定 base `f7a90022aad5da4d67ea594ccb86f8b9925a3840`，不是修复不存在该浏览器路径的旧 P 单独检出 |
| 独立 Windows Python | PASS：3.14.6，26套件 / 52命令，normal与-O各182方法；平台不支持的 symlink 反例每模式1项skip，未冒充CI的3.13 |
| 独立 Linux Python | PASS：3.13.17，26套件 / 52命令，normal与-O各182方法，无skip；readonly导出挂载、network=none、UID10001，无旧宿主路径 |
| 两平台导出 | 同199原始源/fixture成员加runner及collection；实际读取审计无旧工作区/保留私有路径。Python open audit只是断言；Linux另有物理挂载/网络边界 |
| 浏览器最终真实构建 | 最终回执见 `ci-portability-fix01/final-browser-summary01.json`；无retry，保留所有先前失败 |
| 源环境 / R4 | 6个源CID仅inspect，均stopped/PID0；R4 `0446fde5985b51de7da5863b45df3f1b47b6167c`及ZIP `a60ddfeb8a81d4414bc56e1ddd05fa01ca7f2e0e9579a7d403783bdd318d115a`（6515425bytes/781成员）不变 |
| 独立整体终审 | 最终结论见本轮 `independent-final-review01.json`；先前BLOCKED结论保留，只有最终源码与完整证据可闭合 |
| 新 P 远端 CI | NOT_RUN；本地结果不是 PR CI，更不是 main CI。仅准备新 HEAD 受限发布方案 |
| 实际业务/保护验收 | NOT_RUN / NOT_AUTHORIZED；未分配新 MaintenanceID、execution、预算、窗口或删除授权 |

证据根：`docs/acceptance/evidence/issue307-d1/ci-portability-fix01/`。原发布失败回执、原浏览器 artifact/trace、Linux前两次失败、Windows定向失败、过期样例和辅助脚本失败均另存；不将后来的通过回写原失败。

## 发布边界

新本地 P 第一 parent 为当前远端 `1d3e417b`，第二 parent 为固定 CI base `f7a90022`，普通 FF 祖先关系必须成立。已发布主线内容和旧证据保留；只有本轮测试/查询边界修复为自有源码增量。R4不重构建、不改绑定。

这是明确的 **失败 CI 修复发布**：旧 P 的已知失败 run作为只读修复依据，不能伪造旧CI绿色；必须有独立回归与终审通过，且实时读回原HEAD/同一规则/完整保护/Draft/Open/评审线程不漂移，才可让人类考虑新的有限发布授权。该例外仅属于未来发布前置设计，`execution_freeze.ready`、preallocation 和实际执行绿CI门禁完全不改，业务准备仍须新P精确完整PR CI成功。

新HEAD及机器提案另存，避免为写入自身SHA推进已冻结提交。此次没有解锁、push、workflow rerun、Ready、merge、源启动、业务保护写入或删除。
