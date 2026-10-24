# #307 D1 candidate21/v31 完整确认逐项核对

本轮结论 **PASS，冻结绑定无漂移，当前方案核对范围0阻断**。这是新授权前的只读核对，不是删除授权或完整D1验收。

execution `9ba214bd-a481-499b-bef6-ca7a789ef501`；最终HEAD `f4f27c789f53b6c45910f55f83bee6cabfdee4f6`；固定新加坡时间2026-10-22 22:00–22:40，UTC14:00≤now<14:40；恢复截止UTC16:40，即新加坡10月23日00:40。

| 核对项 | 当前实际结论 |
| --- | --- |
| 02完整方案及聊天展示 | proposal SHA `50d88777d6ed5d9b54e33558c98b9866f5a07e630dc8c77e1c098404e3119989`；聊天137行机器文本/全部94表行与冻结原件一致 |
| 冻结候选、R/P、原始构建、完整引用 | PASS；R769/P2247源成员原字节/规范LF；12原构建；82公开绑定；1102审批成员；8索引6366引用；255发布成员 |
| 六份目标/十一保留版本 | namespace/key/version/copy/DV/bytes/SHA/plan/hash/hold/retention逐项一致，P1永久排除 |
| 最终HEAD/原生分支保护 | GitHub实时读回HEAD一致，rule `BPR_kwDOUdQLBs4FE_7b`及完整保护与冻结收据相同；lock/admin=true，fork-sync/force/delete=false |
| execution-binding | public/private同字节，SHA `f27a617f278fcbaf64db29ddf536d5816d88acaba8a0d760280eb23bbe55d0de`；全部制品字段一致 |
| 精确HEAD PR CI/评审线程 | run39773563527 attempt1，pull_request，13/13成功；未解决线程0；不是main CI |
| 当前host/预算/权限 | 固定门禁PASS；996已知CID停止/PID0/noports/norestart；30权限完整冻结字段false；3新预算空/归属与挂载集精确，27旧预算保留 |
| 备份/失败/pins | 57旧+9新共66备份SHA及历史保留门禁通过，旧失败/已用预算/pins未覆盖 |
| 授权/调度/实际发送 | NOT_RECEIVED / NOT_CREATED / NOT_RUN；DELETE=0；#332 Draft且未合并，#307 OPEN |

| 目标 | exact namespace/key@version |
| --- | --- |
| P2 | `dpp-lifecycle307-online/fixture/p2.csv@d8040608-c5cd-4a6f-8a2d-b3f856cf5302` |
| T1 | `dpp-lifecycle307-online/fixture/t1.csv@39d4ef5d-e983-413d-a6c2-9759d456b750` |
| T2 | `dpp-lifecycle307-quarantine/fixture/t2.csv@90bd8ded-337a-4ca1-aec3-66c4ee6a1314` |
| H2A | `dpp-lifecycle307-online/fixture/h2-a.csv@453c6f42-fbb2-4fdf-9f86-00a20a54b7bc` |
| M1A | `dpp-lifecycle307-online/fixture/m1-a.csv@e8a8418e-c515-464e-99e5-10bc4c64fa8d` |
| M1B | `dpp-lifecycle307-quarantine/fixture/m1-b.csv@54900849-d59d-459e-90bb-275a5975d0a6` |

十一保留版本的完整identity和保护元组见 `files03/exact-version-crosscheck.json`，与冻结binding一一相等。当前对账不启动Core/native，不再做源库存/IAM网络请求；既有17版本原生验收是已保存准备证据。未来固定窗口内仍须重新跑批准的完整入口，完成fresh Core/native/17库存/保护/IAM/备份/拓扑/预算核验，自动关闭后先Host.check，并逐次生成≤1分钟fresh host proof。没有新授权或任一前置不通过，发送门禁不放行。

每份max1、全局max6，UNKNOWN不重发；逐份send→disable-send→选定Outbox→Asynq只读恢复→Core absence→下一份。H0/H1零发送，H2/M1有限顺序不变。cleanup=0、hold-release=0、bypass=0；失败或过窗立即固定seal，不延长、平移、重开、补发、退款或释放旧pins。

只读核对脚本01/02的解析/schema假设错误及失败已保留，03使用冻结的完整operator/root schema成功；没有修改候选、运行门禁、源码、binary、HEAD或旧事实。新证据仅追加，不提交或推进已锁HEAD。
