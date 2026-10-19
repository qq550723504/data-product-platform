# #307 D1 旧 dispatch 闭合验收矩阵

本矩阵记录新的非删除准备，不重开 v22；#323 Draft，#307 OPEN，真实 DELETE=0。本切片仍未完成，不能生成可执行候选或精确授权文本。

| 项目 | 实际结果 | 证据/限制 |
| --- | --- | --- |
| Docker 恢复后保全 | PASS | 77 CID 停止/PID0/noports/no-restart；46 旧控制文件、8 只读 profile、9 备份、3 预算组与原 anchor 一致 |
| 固定只读审计入口回归 | PASS | normal/-O 各 11 项；Docker/Core 为 mock，另有独立预算缺失及 SHA 漂移完整入口反例 |
| 审计入口独立复评 | PASS，blockers=[] | source-final-review01.json SHA a84cb16d31f96d2c62d0ef6bea79d2a9d4e5439d2135d1ba8be679c9deeff5fb；只覆盖审计入口/合同，不覆盖后续 mutation adapter |
| 实际 Core 只读审计 | PASS | audit 23587686-0a95-4eaa-a010-a8c896c9735b；REPEATABLE READ READ ONLY；全 relation/roles/ACL 前后一致；Core 最终 PID0，全部保全内容不变 |
| P2/T1/T2 旧占用 | 已逐项查实，尚未闭合 | 各 1 dispatch/attempt、0 claim/observation/release/absence、unresolved=1；三项旧 approval 均未撤销；T1 的 3 行 reader pins 原样保留 |
| 新鲜可信 owner/IAM/gateway proof | NOT_RUN | 当前审计不启动 Native/IAM helper、不签名；无冲突不等于可信 NOT_SENT |
| 固定 NOT_SENT adapter | DRAFT_NOT_REVIEWED | post_v22_no_send.go 初稿，未接执行入口、未做专属回归/PG验收，禁止据此运行 |
| 原源 revoke→NOT_SENT→release | NOT_RUN | 必须先完成新有限 authority、宿主采集、专属反例与 PG 回归、独立评审；仅复用现有 Command，不直接 SQL 插事实 |
| 17 版本全量只读 | NOT_RUN | 待三项可信闭合后；P1 不再进入目标，六目标+十一 survivor 必须全量 bytes/SHA/保护/库存刷新 |
| 六份新候选/execution/窗口/确认文本 | NOT_FROZEN | 原 v22 授权不复用；没有新的可执行删除许可 |

实际审计原始行和 before/after/终态均保存在 `evidence/issue307-d1/post-v22-dispatch-audit/23587686-0a95-4eaa-a010-a8c896c9735b/`；旧 v22、P1 audit01–04、预算及 pins 均不覆盖。新审计无 provider 调用、无成本伪录、无业务事实写入。

当前代码与证据先隔离保存，供 #323 同步最新 main 的独立复评使用。不得把本切片尚未完成的工作混入已有 R1/R2/P1 的评审结论或冒充完整 D1 验收。
