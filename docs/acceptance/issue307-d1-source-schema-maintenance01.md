# #307 D1 三源 Core schema 维护精确方案（待单独确认）

当前没有源升级授权，没有新删除授权或调度。candidate20/v30 仍 BLOCKED_SCHEMA_RUNTIME_COMPATIBILITY；拟定18:00–18:40只是准备窗口，不能据此执行。

本方案只维护下面三个现有源 Core 的 lifecycle307 数据库；全部 MinIO、gateway、sender、业务 worker 和旧 execution 保持关闭。不是重建环境，也不替换原 System of Record，不删除对象或预算，不释放 pins。

| 组 | 精确 Core CID | 保留原 Core dump SHA256 |
| --- | --- | --- |
| business-races | 9bc6e9ec894f9fecb1566940d9b4680e0a9ddee52f31f9aec1f45afdb35c7ab0 | f9255090e65f441f03240c493256263d423ec0ea7720348d3c3478687c21d218 |
| late-hold | 982c4e8d2e13331ee94f62148f1bfa6846418a743599cce2270748f8205ed0db | 15241ad90215843014acaa69f6b67767009dae1fb34d71cdc682e36053d10e73 |
| mixed | cb50e06b7cd575e7da04b5ff64498ec61a430607f270d4bf3102c1372dca2a82 | aef0cd7121a315510cac8e9cd89e65436922f66d0ae220588522962b0d8a2503 |

官方现有 cmd/migrate Up 只补实际缺少的70/71；既有版本集合必须与已验证备份一致，实际缺集必须精确为[70,71]。不手改 schema_migration，不去掉 identity_kind 条件，不增加旧schema fallback。每个 Core 最多一次 Up，全局最多三次，未知结果不重试。

| 精确 migration | SHA256 |
| --- | --- |
| 000070_workspace_protected_object_identity.up.sql | 95ff03fbff2eff274cc126dcba09d32041893f91f6e19238b474e0409b27e349 |
| 000071_operator_custody.up.sql | 538cae2e7d7df00315c0a099664d026f0caf4c191f011aa8715f1f1959e61308 |

冻结的 Linux migrator SHA256：43de9253c3a3cc9df3656432703028320c8c6f0a73e6481698f178399090a26e。完整计划 JSON SHA256：0efdd59724c921e032387b63e29868dc56c0c368b6f17081d52036edfa365ffb。维护 ID：dda561ce-5237-4e54-8f30-41545cd271ab。计划列出全部 up 文件SHA、旧baseline SHA、clone terminal SHA及失败terminal SHA；执行时逐项核字节，拒绝漂移。

已完成的非删除证据：三个独立恢复数据库实际旧版本MAX=69、66条迁移记录；Runner精确新增70/71后MAX=71、68条记录。原查询先复现SQLSTATE42703，再返回精确copy。每组194张旧表按原列及原schema_migration成员投影，原行/旧pins/成本/历史事实SHA均未变化；只新增schema版本及新schema对象。唯一隔离PG helper和新卷已保留、PID0/noports/norestart；所有六个源Core/Native始终stopped。

执行顺序与验收：

1. 仅在新的直接人类维护确认后即时运行一次，保留实际原文及本计划SHA；它不授予任何删除、调度、Ready或merge权限。
2. 重核旧v29 terminal、820旧owner关闭、24旧预算及57旧backup保留。只启动三个固定Core做REPEATABLE READ READ ONLY版本/DDL/全关系/roles ACL核对，必须匹配原baseline；所有Native保持停止。先保存每源新的O_EXCL core.dump与globals.sql、原列清单及原行/旧pins投影，不覆盖57份旧备份。
3. 为每源建立独立受限维护helper，固定source Core网络命名空间，无ports/socket/重启，read-only root、cap-drop ALL、no-new-privileges。只挂冻结migrator和迁移文件只读，以及该Core的限定数据库连接配置，不挂provider root、host/wire私钥。启动前保存CID/image/command/mount/nonce绑定。不是使用任何旧execution的root owner。
4. 每Core先精确核缺集[70,71]与原表/原列/旧事实，持久化本次Up intent再只运行一次现有Runner。不得运行fixture basis/approval/dispatch/Renew/Processing/DirectData/release/pin-ref closure等业务Command。
5. 验收精确新增migration70/71，无其他版本；原194表旧列/原行及旧migration成员SHA相同、旧pins/预算/backup不变；旧ProviderVersion Copy只新增identity_kind=PROVIDER_VERSION、protected_verification_id=NULL，namespace新增空prefix，不扩大或重写旧对象选择器。保存新完整Core fingerprint为追加维护checkpoint，不覆盖旧baseline。
6. 原角色属性/密码哈希及旧表ACL不得变化；新表/函数/约束及函数定义变化须完全由冻结migration解释。查询原六目标的当前repository路径成功；这仍不是native删除或业务发送验收。
7. 任何异常停止后续Up；官方Runner按单migration事务提交，若只完成70或不同组结果不一致，明确记PARTIAL/UNKNOWN，保留事实，不自动Down、不回滚恢复旧库、不重试、不进入新候选。继续停止所有新helper及已开启Core并逐项PID0封存；一组失败不跳过其他关闭。
8. 维护通过后才准备新的当前schema baseline绑定、重做源/restore非删除验收、17版本全量只读、独立评审和最终HEAD CI；最终branch lock及execution-binding闭合后，再提供六目标的完整新删除确认。18:00窗口来不及准备则保持未授权，不借源维护确认发送或延长窗口。

本方案涉及保留源数据库的真实schema写入，超出现有固定verify-six-native.py“no migration”只读入口的边界，因此需要单独确认。本轮只完成方案及副本验证，尚未对源执行。#332保持Draft、不合并，#307保持OPEN。
