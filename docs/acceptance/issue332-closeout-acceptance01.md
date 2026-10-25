# #332 有限合并收口验收矩阵

权威有限范围：[合并契约](issue332-finite-merge-contract.md)。已发布基线为 `1d142fa332b639144c232bb686be214521b9f352`；本地DNS修复尚未推送。#332保持锁定、Draft、未合并；#307 OPEN。无新业务候选、execution、预算、窗口或调度。

| 项目 | 验收结果 |
| --- | --- |
| 原始故障 | 27个已关闭必需owner仅HostConfig.Dns null→[]；原拒绝与原raw hash原字节保留，未更改旧快照或冻结契约 |
| 实现 | 内层依赖schema v2 +固定算法；canonical hash和原raw hash同时受检；仅单字段null/[]可等价，非空原DNS仍严格相等；r.descriptor/catalog/Go/R4/执行门禁未改 |
| 成功组合 | 四种冻结/当前null/[]组合通过；完整CLI对等价表示返回同一稳定history；输入及历史文件不变，原维护比较仍严格拒绝 |
| 拒绝组合 | 自定义DNS/缺字段/错误类型；Search/Options、identity/image/command/env/user、权限/mount/netns/运行PID；旧schema/缺失或伪造tag/hash（包括canonical不能覆盖原raw）；预算bytes/owner/mount/missing继续拒绝 |
| Windows Python3.13.16 | 26套件52条normal/-O命令PASS，每模式192方法；仅既有evidence_indexes每模式1项平台skip，successor套件23方法无skip |
| Linux Python3.13.16 | 相同26套件52命令PASS，每模式192方法，无skip；唯一network=none/read-only/UID10001测试helper由既有归档回收入口确认退出后删除，无遗留新容器 |
| 真实宿主DNS核验 | 新完整依赖比较PASS：184 owner，157 raw完全匹配、27仅空DNS等价；原CID集合及原raw descriptor完全相同，134保留卷及挂载图通过；不访问S3对象 |
| 固定历史/依赖完整CLI | `verify-successor-history.py`实际PASS：7源角色、184 owner、134卷、58个当前物理预算、54310份原文件、986容器及106卷原退役回执；不是业务native17版本或真实写保护验收 |
| 原始输出 | 原始stdout/stderr、逐suite审计、独立副本和唯一helper归档保留在私有merge-closeout01；公共只增加精简摘要和显式SHA索引，避免继续复制大量日志进PR |
| 副作用 | 保留源启动0、业务provider0、保护写入0、业务DELETE0、预算重置0、pins释放0；不创建新候选或窗口，不解锁或推送 |
| 独立终审 | 以本轮另存final review为准；不能用此前1d142fa3的评审/CI冒充新修复已发布 |
| 远端与交付 | 当前远端1d142fa3的PR CI40103985438为13/13成功、线程0；新本地修复HEAD的远端CI为NOT_RUN，需另行授权受限发布后验收 |

## 收口分类

本轮已知功能阻断DNS表示问题在本地修复并完成上表验证。独立终审若无有效代码阻断，本地技术清单归零；**当前PR仍有交付阻断B1：新修复不在远端，尚无该修复HEAD的完整PR CI**。Ready/合并是独立的人类授权边界，当前按要求保持Draft，不把权限约束算作新代码缺陷。

两份真实retention延期、17版本before/after真实native验收、剩余六份删除及完整D1接纳继续留在#307，分别准备和授权，不阻止本PR按有限代码契约收口，也不声称它们已完成。额外性能/全量环境治理/未来provider能力为follow-up；不再机械新增候选或窗口。

原始r.descriptor、历史空DNS原值、失败记录、已用预算、pins及旧执行授权不回写。新契约仅供未来新的受控准备；旧v1执行包不能用新语义重新解释或借此续发。
