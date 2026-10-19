# #307 D1 剩余六份候选的有限准备合同

状态：设计与非删除准备，不是删除授权。新窗口按人类选择为2026-10-20新加坡14:00–14:40（UTC06:00–06:40），不得使用旧v22批准。新删除execution及可执行候选尚未冻结。#332 Draft、不合并；#307 OPEN。#323已按随后单独授权合并，历史Draft要求和证据不回写。

前置已闭合：旧P2/T1/T2由audit `94f28318-cfd4-4c62-ba1a-bec9f35836c8` 可信NOT_SENT/release，旧事实不改写；17版本由audit `f4330324-98de-4fc4-ae3d-e1feab7adaae` 实际完整读取并独立通过，P1真实provider NoSuchVersion与完整namespace缺失一致。17=六份现存目标+十一survivor，74次技术调用单独留证，金额NOT_ASSESSED。以上不构成删除授权。

## 复用及最小边界

复用现有Disposition/Occupancy/ExactExecution Commands、fixture basis与policy renewal、SDK deleter/inspector、持久target/wire预算、签名拓扑及owner关闭证明、Outbox→Asynq恢复和备份/恢复工具。不新增Core实体、队列、workflow runtime或policy engine，不重新实现S3或IAM语义。现有MinIO SDK与官方policy ParseConfig/Equals继续处理协议和policy集合语义。

现有operator当前contract锁定旧七目标/旧execution。下一代应是一套新的严格六目标contract，旧版本只由已保存的二进制、Git anchor、原始source ZIP和历史validator解释；不能继续给旧授权加兼容分支。新来源身份仍为原三源，不重建/seed/替换原源，不复制旧permission为可发送许可。

一个必须保留的模型区别：Core仍注册18个历史copy identity（business八个包含已删除P1），而当前物理库存只有17。`RemainingDependencies.All`也用于核完整Core注册集，不能简单改成17而使ReferencesReleased永远UNKNOWN。新候选须分别绑定历史18、已确认缺失P1、当前17、六发送目标和十一survivor；P1历史身份/absence继续解释旧事实，永不回到target digest、sender、wire scope或新预算。

## 非删除有限验收

1. 六目标唯一顺序P2→T1→T2→H2A→M1A→M1B。先在真实SDK/loopback模拟provider组合与真实PostgreSQL事务中验收逐份“send→关闭发送→只读完整namespace absence→提交Core receipt→下一份”。模拟DELETE不连接任何保留Native；原三源不执行DELETE。
2. 在新admission前，旧未释放dispatch必须阻断；旧批准撤销及可信NOT_SENT/release后，只能由新批准、新dispatch、新attempt进入。旧attempt/失败/预算/pins/成本保持；旧批准不可重新激活，UNKNOWN不可重发。预算先于Core claim且O_EXCL不退还；下一份在absence未提交前不得消费新slot。
3. business只复用已完成的真实Processing/DirectData、owner/handle/pin/ref闭合及现存两个输出，不重跑生产、不生成新版本、不恢复P1。新execution的basis/批准/admission必须重新通过当前门禁，依赖policy只做expected-head追加Renew，不翻转旧事实；CandidateUseUntil统一来自新窗口END。
4. H0/H1零发送；可信关闭证明仍需真实stopped/PID0/no-restart、撤销profile、全部enabler关闭、fresh disabled identity、sealed gateway，再追加NOT_SENT。H2A仅在真实上游204后witness/late Core holds A+B/revoke/响应丢失；H2B永不dispatch。M1A先absence，M1B再CONFIRMED UNKNOWN+C保护阻断checkpoint；原checkpoint复制核SHA后才B lookup，C永不删除。
5. 补齐六份成功组合、旧占用/旧批准、未关闭发送、预算扩张/重复、未提交absence、UNKNOWN重发和中途失败停止后续目标反例；mock、真实PG、Native只读、backup/restore和独立评审分别报告，不用一类证据冒充另一类。

## 冻结准备及确认范围

只读重查三源的当前policy heads、旧执行及终止事实、P1 absence、旧三项NOT_SENT/release和Core库存/roles/ACL，保存before/after相等的完整Core检查点。所有SQL只读，不迁移、不seed原源。

为当前17状态产生新的独立备份/restore检查点；旧九份及全部旧快照保留。restore必须使用新且明确归属的CID/卷/内网，不能别名原源，不向原源回滚，不恢复发送权限。恢复读回必须明确actual restore CID与original source CID，不能用原源核验冒充restore；配置只携带固定GET/HEAD技术凭据/读取期限，零删除，不能挂业务Token/预算/宿主签名私钥。沿用已有适配器及新的17对象检查，原源保持其全部历史事实。

新发送身份/policy须独立于旧身份，默认disabled；权限仅覆盖六份exact版本，新inspection仅覆盖当前17及必要namespace list。旧八身份及原policy继续disabled并保留旧绑定。任何新建IAM技术调用也先start再observation，新增费用按实际attempt留证，金额未知NOT_ASSESSED。当前只准备disabled身份，不启用发送。

完成上述门禁后才分配最终新execution、构建/冻结唯一当前source/binary、完整候选/IAM/active及sealed gateway、Core、backup/restore、实际owner/topology、全新空预算和全部profile窗口绑定。所有历史保全与当前完整入口必须共用同一核验路径；Docker raw wrapper保持bytes，唯一文本边界严格UTF-8。

精确确认方案列出全部冻结摘要、六份完整目标、十一份完整保留版本、P1缺失事实、上述固定窗口、每目标最多一次/全局最多六次/UNKNOWN不重发，cleanup=0、hold-release=0、bypass=0；失败/过窗立即封存，不延长/平移/重开。机器模板与真实人类原文分开记录，不提前伪造授权，不自动安排调度。完整D1仍NOT_ACCEPTED。
