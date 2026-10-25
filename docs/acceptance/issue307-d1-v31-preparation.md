# #307 D1 candidate21/v31 非删除准备验收矩阵

execution `9ba214bd-a481-499b-bef6-ca7a789ef501`，固定候选窗口2026-10-22新加坡22:00–22:40（UTC14:00≤now<14:40），只读恢复最晚16:40Z。本文件是最终提交前的准备快照。最终发布HEAD CI、原生分支锁与锁后execution-binding另存发布附录；本快照不冒称它们已经完成。当前无新删除授权、无定时续跑；#332 OPEN/Draft、不合并，#307 OPEN。

| 验收项 | 实际结果 | 证据 |
| --- | --- | --- |
| R/P严格双制品与跨字段回归 | PASS；R固定schema69/66，无源升级或兼容fallback；P保留当前产品模型 | candidate21-source-review01/independent-final-review01.json |
| 分配前P精确PR CI | PASS；cef8a7f32133400f34ed2b1be0f2f4b635350190 / run39766229920，13/13；event=pull_request | allocation.json内source_ci原收据/完整snapshot |
| 新代资源与预算 | PASS；86 owners、15新卷、3internal networks，3独立空预算归本execution，旧预算未复用 | remaining-candidate21-preparation01/allocation.json、budget-initialization.json |
| 三源Core只读/备份与分离restore | PASS；3组完成、9新备份、独立clean及rehearsal，源Core业务命令及object PUT/DELETE=0 | remaining-candidate21-preparation01/non-delete-terminal.json及原始索引 |
| 隔离业务重跑 | PASS；仅副本Command，历史事实multiset保留；不是六份源发送结果 | 各组rehearsal原始stdout及validation |
| 完整17版本原生只读入口 | PASS；17现存、6目标、11survivor，P1真实NoSuchVersion404；Core指纹/roles/ACL前后不变 | remaining-candidate21-readonly01/complete.json |
| 完整IAM语义与保护/库存 | PASS；47实际用户disabled，46 sole-policy/no-groups及固定NONE；15 fresh NoSuchUser，官方ParseConfig/Equals | 同一完整入口各组原始请求/聚合 |
| 真实技术请求与成本 | 167个start/observation对；amount=NOT_ASSESSED，非外部业务provider执行 | complete.json及每组technical原始回执 |
| candidate20及旧失败/预算/pins | PASS保留；907旧CID，27旧预算（含已用v28预算），失败terminal与旧pins不改写 | retained-candidate20-anchor01、retained-full-readonly02、最终cold gate |
| 全部备份 | PASS；57旧=54完整+3失败partial，9新，共66逐字节SHA | retained-backups66-readback01.json |
| 最终关闭状态 | PASS；996已知CID=907旧+86本代+3技术reader，全部stopped/PID0/noports/norestart；30permission=false | final-cold-inventory01.json |
| 当前新授权/调度 | NOT_RECEIVED / NOT_CREATED；旧调度PAUSED或ABSENT | authority-schedule-absence01.json |
| 四份实际审批文档Git字节 | PASS；真实blob/archive/autocrlf true与false checkout均原字节一致，含child attrs自身 | publication-bytes-regression01/result.json |
| 冻结候选独立评审 | PASS；0有效阻断，报告SHA `44410ec104478164e422661dabae95c5fc4c25db2b7cb83fb4ec677c9f7697ff` | candidate21-artifacts-review01/independent-final-review01.json |
| 最终静态索引/离线核验 | PASS；同一固定完整入口 --audit-only，审批manifest 1102成员 | final-static-evidence-audit-plan01.json、phase-final-static-audit01 |
| 最终发布HEAD PR CI | PENDING_PUBLICATION；必须另核精确HEAD，不能拿分配前CI代替 | 后续publication01追加回执 |
| exact原生分支锁/execution-binding | PENDING_FINAL_HEAD_CI；锁后禁止推进HEAD，不自动解锁 | 后续branch-freeze01追加回执 |
| 六份源真实发送/恢复/H2与mixed outcomes | NOT_RUN；P2→T1→T2→H2A→M1A→M1B每份max1/globalmax6仅未来新授权后允许 | 没有本代授权或发送结果 |
| 完整D1 | NOT_ACCEPTED；PR Ready/merge/main CI均不在本轮授权内 | #332 Draft / #307 OPEN |

完整静态方案见[确认文本](issue307-d1-candidate21-confirmation.md)及[v31手册](issue307-d1-v31-window-runbook.md)。最终锁后完整附录补齐前不得请求删除授权。旧原始失败、已用预算、pins、provider attempts/成本不覆盖；cleanup=0、hold-release=0、bypass=0。所有晚于本快照的事实只追加保存，不回写历史PENDING/NOT_RECEIVED字段。
