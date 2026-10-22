#307 D1 有限修复收口：当前 HEAD `16aa4ec464a1fa14f1aa7ac21624b3855ecb39b3`，对应 [PR CI39757747005](https://github.com/qq550723504/data-product-platform/actions/runs/39757747005)（pull_request/attempt1）13/13 成功；实际 job/check、GitHub Actions 来源及未解决线程0已重新核对。不是 main CI。

本轮 contract 为执行/分支冻结边界及其必需 CI 路径：原生 GitHub 分支锁；R/P 显式独立制品；Git 归档固定 UTC/LF/store-only 并保持 exact SHA；精确 P HEAD 的全部必要 CI 在任何新资源分配前通过，build 后再次核相同 HEAD/完整字节；CI snapshot/head/run/回执交叉绑定；已终态准备禁止续跑。复用原生 Git、Go、GitHub 保护和现有校验入口，没有新增平行基础设施。

当前有限源码独评 PASS、剩余 blocker=0；独立 normal/-O 各27方法通过，45份评审证据/172份输入逐字节复核，原CI-R1两次finding均保留。源码评审绑定923c0672，当前发布HEAD的172份输入仍相同，发布后完整CI另已成功。原生分支锁演练和隔离schema/R-P证明均有独立边界；三源没有升级，本轮 source/provider/DELETE=0。

candidate20/v30（execution 5a54e01a-657e-4b8a-ae67-a2d6fd5be8f6）已 FAILED_CI_BLOCKED_UNFROZEN_PREPARATION_CLOSED，不能恢复。86个owner关闭、30项permission false；旧/新预算、pins、原build、失败归档和撤销前profile保留。其原生17版本核验、非删除准备与冻结均NOT_RUN，不把本次CI成功回写成候选验收。矩阵：[v30 preparation](https://github.com/qq550723504/data-product-platform/blob/16aa4ec464a1fa14f1aa7ac21624b3855ecb39b3/docs/acceptance/issue307-d1-v30-preparation.md)。

剩余六份后续仍需全新候选/execution/独立预算、17版本原生只读核验、冻结独评、最终精确HEAD CI及原生执行锁，之后才能展示完整确认方案并取得新授权。本轮没有新候选授权或调度，不复用旧授权。18:00窗口尚无可授权的新冻结方案，不延长或平移它。完整D1仍NOT_ACCEPTED；#332保持OPEN/Draft/unmerged，#307保持OPEN。
