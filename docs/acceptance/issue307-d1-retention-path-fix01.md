# #307 D1 — 受限发布的 Windows CI 失败与本地路径修复

已按人类精确授权发布 P `f2dbe4106ff5c27ff6b250091bbf9f9d79d71665` 与 R `ecf40e6c7a9b3aa11166e6a56a5163a29a15691d`。原 C23 R6255 分支未变。新 P 的 PR CI run39979325471 中，Windows 宿主成功组合误报 `MAINTENANCE_MOUNT_DRIFT`；没有将本次失败声称为验收通过。按方案已立即把同一 rule `BPR_kwDOUdQLBs4FE_7b` 的 lockBranch 锁回，其余保护完整相等；#332 保持 Draft/Open/unmerged，#307 保持 OPEN。

失败来自新增宿主入口遗漏了 Windows 路径身份规则。GitHub 原始日志显示两个必需成功组合在 mount 比较处失败；本机用真实文件系统复现：同一目录的 canonical 拼写通过，大小写和真实8.3拼写均被误拒绝。它们实际 `samefile=true`；不是额外挂载或授权扩大。原日志和复现见原工作树 `d1-renew/docs/acceptance/evidence/issue307-d1/retention-maintenance-publication01/`，本地副本及原始 SHA 另存于本目录 evidence。

修复仅改两个宿主 Python 文件：复用现有 `evidence_indexes.unlinked_absolute`，先检查路径组件无 symlink/junction，再对 bind 两端规范化比较。原始 Docker descriptor SHA、CID、完整挂载集合、类型/RW、预算 volume 名与私有文件白名单均不放宽。重新哈希 descriptor 后的错误目录和实际 junction 仍被拒绝。没有新增通用路径库，也不改变 Core/R/worker/候选/预算/保护状态。

| 项目 | 状态 | 边界 |
| --- | --- | --- |
| 原 P/R 精确发布 | PASS | P=f2dbe410，R=ecf40e6c，原R6255不变 |
| 原精确P PR CI | FAILED | run39979325471 的 Windows host test 两个成功组合失败；原失败保留 |
| 失败后分支关闭 | PASS_READBACK | 同rule只改lockBranch=true，其他保护完整相等；无后续push |
| 原始路径问题复现 | PASS_REPRODUCED | 真实Windows长路径/大小写/8.3实际同目录，原入口误拒绝 |
| 修复后的原成功组合及alias组合 | PASS_LOCAL | normal/-O各8个维护方法通过，无skip，包含真实junction拒绝 |
| 共享无链接边界关联回归 | PASS_LOCAL_WITH_EXISTING_SKIP | evidence_indexes normal/-O各12方法：11 PASS、1既有symlink权限条件skip；本轮实际junction反例另已通过 |
| 独立源码复评 | PASS_BOUNDED | 2个宿主文件，剩余源码阻断=[]；独立重跑上述范围 |
| 修复新P的远端发布及CI | NOT_AUTHORIZED / NOT_RUN | 当前授权只允许P=f2dbe410；不能据此推送新的修复SHA |
| 原生维护/删除 | NOT_RUN | source start=0，retention/hold/IAM write=0，DELETE=0；旧预算/pins/失败/备份保留 |

本地修复在隔离工作树 `d1-retention-path`，基于已发布 f2dbe410。R ecf40e6c 和其归档 SHA `5e86e36808f00f62b9e9c7eb9b6b76d0c306958f4c5118c6d9d4eb4ff00de092` 保持不变；未创建新候选/execution/实际预算/窗口/调度。当前只准备下一次可核对的源码发布差量，不复用旧授权或推进受保护远端 HEAD。

若人类另行授权修复发布，起点必须是已知失败且已锁回的精确 f2dbe410/run39979325471；核对失败事实、终态、HEAD和完整保护后才允许维护。不能要求失败基线先变绿才能发布修复，也不能把该修复维护门禁用于 allocation/freeze/send。新的精确P CI仍须完整成功，失败则再次锁回；`execution_freeze.ready`、候选执行和发送门禁没有任何修改或豁免。
