# #307 D1 candidate20/v30 准备终态与 CI 修复矩阵

candidate20/v30 的 execution `5a54e01a-657e-4b8a-ae67-a2d6fd5be8f6` 已终止，状态为 `FAILED_CI_BLOCKED_UNFROZEN_PREPARATION_CLOSED`，不得恢复或复用。原提出窗口为 2026-10-22 新加坡18:00–18:40（UTC10:00–10:40）；它只是准备窗口，没有新删除授权或调度。#332 保持 OPEN/Draft/unmerged，#307 保持 OPEN。

## 验收矩阵

| 检查点 | 结果与实际边界 |
| --- | --- |
| 冻结后 PR HEAD 更新根因 | 已完成有限修复及独立复评，原生 GitHub 演练实际拒绝锁定分支快进更新。最终执行 HEAD、runtime source 和锁后回执分别绑定；本代尚未冻结实际 #332。 |
| 保留源 schema 与当前产品 P | 三源实际 max69/66 migration members、无 identity_kind；当前 P 保留 migration70/71 与 protected identity contract。三源没有启动或升级。隔离 PG 克隆升级证明只属于副本，不代表源 roles/ACL 或源升级验收。 |
| Fixture R / host P | R72ec972570a6f10a916ace39d90ab0c5593e8dd4 从既有 fixture 基线派生，仅支持固定 schema69；P 单独冻结，不回退当前产品模型。归档独立构建，GOWORK=off；原三组 SQL/门禁在隔离克隆上验证。原27cb/a233 native 事实不改写为后来源码，R72 实际四 binary 与八 build 日志字节等价另证。 |
| R/P 源码独评 | `candidate20-artifacts-review01/independent-final-review01.json` 有限 PASS、0 blocker。它明确不是候选冻结验收、原生17版本验收或远端 CI。原 SCHEMA-R1 阻断及旧 source review 保留。 |
| 已完成的 candidate20 阶段 | allocate、materialize、initialize-budgets。86 owner、15 volume、3 internal network，3 个本 execution 独立空预算；未执行 run-non-delete 或 freeze。 |
| 源码 CI | c79ad0c15062304e959d6dd1fc8baa6362abcf67 的 PR run39744232114 失败。Linux Go 真实日志报 `exact runtime archive drift`；同时修复遗留 retained-v26 测试的旧评审路径与 R/P fixture。原失败、用户提供的错误摘录和本地复现分别保留，不伪称下载到了完整 CI 日志。 |
| 归档真实根因与修复 | Git ZIP 本地时间导致 UTC/新加坡原字节 SHA 不同。统一原生 Git TZ=UTC、core.autocrlf=false、ZIP -0 store-only，recipe=GIT_ZIP_STORE_UTC_LF_V1；不放宽 raw SHA。四种继承时区输出同一 6,425,445-byte/769-member archive，SHA 38c63a14db564cecfe688da1ffb10a3784084c914b298a182b2aaa2c026bcb7c。原失败 archive190722… 保留。 |
| 分配前 CI 门禁 | 必须先核精确当前 P HEAD 的最新必要 pull_request CI；pending/failure/API unknown/外来 HEAD/app 在 history、build、目录与资源分配前停止。history/build 后再次核 source=CI HEAD、完整宿主字节一致，再允许创建资源。 |
| 构建/冻结 CI 交叉字段 | CI 回执、source_ci、原 snapshot 的 head/run/SHA 完整交叉绑定；重算回执 SHA 不能掩盖 snapshot run 不同。source-freeze、bindings、确认模板与固定完整入口继续验证。 |
| 准备终态门禁 | preparation-terminal 优先于 allocation/history/Docker 读取，阻止重用已失败的 execution、旧预算和已撤销 profiles；不会通过修改 host snapshot 让旧构建继续。 |
| 本地非删除回归 | 实际 workflow 中 73 条离线回归/静态检查命令成功；最后 CI run 交叉字段修复后独立正常/-O 各27方法通过。固定 CI R 重建命令在本机完成四个 command packages 与 pilot tests，归档 expected=observed；不冒充 Linux PR CI 或原生 provider 验收。 |
| 修复后的真实 Linux Go CI | 923c06722f3099e9049894489006615f0f0e7ce2 对应 pull_request run39755999260 的 go-platform 成功，单独钉住的 R 归档重建步骤实际成功，API 读回保存于 linux-go-ci-readback01.json；该读回时完整 run 仍有其他 job 在运行，不提前声称13/13，也不是后来发布 HEAD 或 main CI。 |
| CI 修复独立复评 | PASS_BOUNDED_C20_CI_REPAIR_SOURCE_REVIEW，绑定923c06722f3099e9049894489006615f0f0e7ce2，0 blocker；报告 SHA6cdad1521d106a10db798f3df08fa9d465f804ce3135c0bd1e7700eb49f37c8c。原 CI-R1 两次 finding 保留；45份评审证据与172份输入已用固定 typed audit 复核。报告不冒充后来发布 HEAD 的 CI。 |
| candidate20 可信关闭 | 08:58:32Z 终态：86 owner 停止/PID0、30 permission false、19 profile 已撤销且撤销前原字节私有保留；3 新预算与完整挂载集合未变，24 旧预算及57备份保留。实际源业务命令/provider/DELETE=0，IAM 未 provision。 |
| 原生17版本、restore/rehearsal、最终冻结 | 本代均 NOT_RUN，不能使用 v29 或隔离克隆结果替代。 |
| 新删除授权 / 调度 / 实际发送 | NOT_RECEIVED / NOT_CREATED / 0；旧授权未复用。六份目标仍未执行，P1 永久排除。完整 D1 NOT_ACCEPTED。 |

## 留证与后续边界

原终态：`evidence/issue307-d1/candidate20-planning01/ci-blocked-closure01/terminal.json`，SHA `1aff9d0c516b4be3247e276e9b6a9ab2f9bb05af48e62f7f1dbe0cdcce28e9ae`；私有 `remaining-candidate20/preparation-terminal.json` 是同字节终态。原 allocation SHA `e952f7c83eaf5af25dcf3878dd88a515ffe3766eb938170117327fa475a448c6`。准备回执、失败日志、原 build、已撤销 profile 前像、旧预算/pins 均保留，不退款、不清理、不重新物化。

本目录中 write-frozen-documents01.py、observe-host-bindings01.py、observe-final-cold01.py、observe-backups66.py 与 run-complete-entry01.py 是未执行的旧 candidate20 草稿，不能作为已完成验收或新的执行入口。旧 allocation 当时的 host c79 与 runtime R72 标注分别解释，不覆盖旧快照。

后续必须使用新的 candidate/execution、独立预算及重新确认的窗口，并把失败 candidate20 作为保留环境纳入完整门禁。顺序为：源码独评与同 HEAD CI → 新分配/非删除准备/17版本全量原生只读 → 冻结与独评 → 最终静态 HEAD CI → 原生锁定和锁后 execution binding → 完整精确方案 → 新人类授权。当前没有可提交授权的冻结方案，不为赶原窗口省略任何步骤，不自动延长或平移窗口。三源 schema 升级备选提案没有授权，本次未执行；无需把源升级当作 R/P 有限路径的默认前置。
