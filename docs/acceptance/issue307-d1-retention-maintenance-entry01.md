# #307 D1 — exact-two 维护入口第一步收口

本地源码、非删除回归及独立复评已完成，当前有限源码阻断为0。两份实际保护状态未写入，剩余六份未删除；新候选、execution、真实维护/删除预算、窗口与调度均未创建。新修复的远端 PR CI 尚未运行：当前 #332 分支仍锁定，旧 C23 维护授权不能复用。

权威有限契约：[contract.md](evidence/issue307-d1/retention-maintenance-entry01/contract.md)。它补充一次性维护入口的实现，保留上一轮提案/报告当时 NOT_IMPLEMENTED 的原始事实，不回写历史。复用官方 MinIO SDK、Go os.Root、现有冻结 Copy、Docker CLI 与 Ed25519，没有新增通用存储、授权或工作流基础设施。

| 验收项 | 状态 | 证据与边界 |
| --- | --- | --- |
| 两 exact survivor / GOVERNANCE 到固定期限 / hold不变 | PASS_LOCAL | 编译固定目标+原提案SHA；真实返回version/size/bytes/SHA与保护前后检查；不是实际延期 |
| 新维护授权、编译可信key、每请求撤销检查 | PASS_LOCAL | 默认build不可写；旧purpose/key/window/原文漂移及重复JSON反例 |
| 每版本max1/全局max2、独立预算、UNKNOWN不重发 | PASS_LOCAL | O_EXCL/fsync invocation与slot先于调用；并发仅一次PUT；403/500/断响应不推进 |
| 真实读回与落盘失败分离 | PASS_LOCAL | readback_confirmed单独保存；success/terminal持久化失败明确FAILED；后owner还要求可信exit0及PASS回执 |
| 宿主签名/挂载/私有文件与全组关闭 | PASS_LOCAL | 精确8文件、nlink、hostkey不入private/output；额外挂载/预算owner/失败终止反例；仅Docker doubles |
| Python正常及优化模式 | PASS_LOCAL | checks02原始stderr汇总：各59方法；整轮因后续R编译失败仍保留FAILED |
| P Go与实际独立R归档CI入口 | PASS_LOCAL | checks03：P4包、R6包实际通过；R不是从P工作树构建 |
| Linux默认禁写build | PASS_CROSS_BUILD | checks03；未设置可信key，未原生运行，不是批准执行binary |
| 独立终审与R编译复评 | PASS_BOUNDED | review01保留历史；review02实际从最终R归档编译，7顶层/35子案例通过；源码阻断=[] |
| 原C23证据与失败保留 | PASS_FILE_AUDIT | 本轮5954成员完整SHA/bytes通过、原terminal/public-private一致、原工作树tracked_diff=[]；不冒充fresh物理封存 |
| 当前GitHub边界 | PASS_READONLY | remote-readonly01：#332 OPEN/Draft/unmerged、head1841d39d；#307 OPEN；同rule锁定且全部其他保护保留。只读，mutation=0 |
| 新修复远端PR CI / main CI | NOT_RUN | 旧head1841d39d/run39952006581绿色不代表新修复；未解锁/推送/合并 |
| 实际维护binding/budget/anchor、原生17版本核验及保护写入 | NOT_CREATED / NOT_RUN / NOT_AUTHORIZED | 仅实现源码；需在发布CI后另准备具体维护方案，不申请删除窗口 |

修复中保留了全部失败：checks01证据runner仓库目录计算错误；checks02 host包路径拼写错误及真实R缺少产品custody helper的编译阻断。最终移除该跨制品依赖，用标准库检查固定Linux挂载点；原R712、失败spec、原stdout/stderr均保留，新增R提交而非覆盖旧事实。未修改任何源Core或部署schema。

最终R为 `ecf40e6c7a9b3aa11166e6a56a5163a29a15691d`，直接父 `71286535d2b14c5456726d0f6db0aae8879de0cf`，本次增量仅main.go。实际Git ZIP为6479934 bytes /774文件，SHA256 `5e86e36808f00f62b9e9c7eb9b6b76d0c306958f4c5118c6d9d4eb4ff00de092`，11共享成员LF精确一致。P/R原字节、LF和归档分别记录，未混称。

最终独立结论见 [independent-review02.json](evidence/issue307-d1/retention-maintenance-entry01/independent-review02.json)；[evidence-index01.json](evidence/issue307-d1/retention-maintenance-entry01/evidence-index01.json)为137项显式files-map-v1，SHA256 `8f0444fb002c790f41cd9cb103e8535a43b41a93e4c71687d083c7eb6b86d0eb`。既有严格audit_index及9项独立审查源码raw/LF绑定实际通过。索引不包含自身、后来audit/矩阵或提交绑定，避免自引用；两个本地默认禁写交叉构建binary只记SHA/bytes，不当批准执行制品提交。

下一步只为这些已评审源码取得精确HEAD远端PR CI：需要新的受限分支维护授权，实时重核保护后只暂改同rule的lockBranch，发布已审R/P、CI后重锁，其他保护保留。此步也不启动源资源、不分配实际预算、不写保护状态或删除。旧C23维护授权只绑定旧P1841/R6255，不能复用。后续实际延期仍须先完成独立的新维护绑定/封存anchor/新鲜非删除核验，再呈现可核对的保护写入方案供人类授权。
