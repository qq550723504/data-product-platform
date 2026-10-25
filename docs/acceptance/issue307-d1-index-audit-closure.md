# #307 D1 索引审计修复收口矩阵

本轮修复已独立复评通过。原临时审计误把files映射当members列表，现由固定完整入口按显式contract核验全部plan条目。不能省略audit plan或其SHA，不能跨格式猜测或回退。此收口不是原生D1完成或执行授权。

| 验收项 | 事实与证据 |
| --- | --- |
| 修复源码 | c1dd1b61495fd3150052744d6ba91e096a428b99 |
| 两种索引 | files-map-v1与members-list-v1，各自严格字段、status、count、SHA和bytes；混合/交叉/重复/空/错误类型/路径逃逸/链接均拒绝 |
| 完整成功路径 | 真main→真typed audit→三次真collect→finally→complete，外部Docker/PG/SDK/checker均为doubles |
| non-delete回归 | normal02/-O各52方法，51PASS/1明确symlink账户权限SKIP；原50方法阶段和首次fixture失败保留 |
| B1独立反例 | 真实授权目录父junction原可绕过terminal；已逐层拒绝，原terminal保留，复验0host/native |
| B2独立反例 | Windows RUNNER~1/长路径原误判；先查逐级alias再规范化，真实8.3/长路径3组合各84成员PASS |
| 实际fixed CLI | saved-evidence audit-only成功84成员；实际旧execution terminal拒绝，0Docker/Source/provider/DELETE |
| 独立最终报告 | PASS_BOUNDED_INDEPENDENT_EVIDENCE_INDEX_REPAIR_REVIEW，remaining_blockers=[]；SHA b8e09798480023618c5af0451b67ea8c17e859d52e4fe26449f8ec661cfc5183 |
| 独立索引 | files映射49 regular成员逐条SHA/长度读回；SHA ded4076f2fe153f0930bac88122944d08796d4ff4bd89ad30482811771e2b9d6；真实junction本身留本地并排除发布 |
| 原v26 | 0/6 DELETE、FAILED terminal、整体IAM关闭UNCONFIRMED保持原事实；旧工作树76批准附件原字节仍匹配 |
| 新源码CI | Windows及Ubuntu新回归均已成功；最终发布HEAD的全部必要CI须按实际Actions读回，不能借旧HEAD绿灯 |
| 本轮原环境/预算/pins操作 | 0；没有新IAM/owner分配、Source或provider访问，没有复用旧授权 |
| #332 / #307 | Draft、不合并 / OPEN |
| 完整D1 | NOT_ACCEPTED |

[独立报告](evidence/issue307-d1/index-audit-repair01/independent-review01/independent-final-review01.json)、[初轮有效阻断](evidence/issue307-d1/index-audit-repair01/independent-review01/initial-blocked-review01.json)、[原Windows CI失败](evidence/issue307-d1/index-audit-repair01/ci-windows-first-failure.json)与作者/独立原始测试回执均保留。原失败未被覆盖为成功。

## 已在独立复评之后建立的下一准备方案

候选名称candidate17/v27，新的execution `6f40bca9-cfe2-4362-ad2b-4e38bbbd8453`，人类已选2026-10-21新加坡14:00–14:40（UTC06:00≤now<06:40），只读恢复上限08:40Z。[完整非删除准备方案](issue307-d1-candidate17-preparation-plan.md)列出六个预期exact目标、十一保留版本、v26终态保留及代际接线要求。

该准备方案尚未成为冻结可执行候选：新代编译配置/retained-v26门禁接线、全新原生备份恢复/17版本fresh及source/binary/Core/IAM/gateway/audit-plan冻结均仍待执行，不伪造新绑定或把历史84成员当新鲜provider证据。authorization=NOT_RECEIVED，schedule=NOT_CREATED。原当前生成器仅支持已终结v26的拒绝guard继续保留，不能只改时间/permission重跑。后续先完成这些非删除准备，再提供完整精确确认文本；旧授权不能复用。
