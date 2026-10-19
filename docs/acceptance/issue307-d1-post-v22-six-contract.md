# #307 D1 剩余六份候选的有限准备合同

状态：设计与非删除准备，不是删除授权。新窗口按人类选择为2026-10-20新加坡14:00–14:40（UTC06:00–06:40），不得使用旧v22批准。已分配 proposal execution `4e524c04-2376-4bc9-9e85-452909d3dba4`（candidate13/v23），可执行候选尚未冻结。#332 Draft、不合并；#307 OPEN。#323已按随后单独授权合并，历史Draft要求和证据不回写。

前置已闭合：旧P2/T1/T2由audit `94f28318-cfd4-4c62-ba1a-bec9f35836c8` 可信NOT_SENT/release，旧事实不改写；17版本由audit `f4330324-98de-4fc4-ae3d-e1feab7adaae` 实际完整读取并独立通过，P1真实provider NoSuchVersion与完整namespace缺失一致。17=六份现存目标+十一survivor，74次技术调用单独留证，金额NOT_ASSESSED。以上不构成删除授权。

## 复用及最小边界

复用现有Disposition/Occupancy/ExactExecution Commands、fixture basis、SDK deleter/inspector、持久target/wire预算、签名拓扑及owner关闭证明、Outbox→Asynq恢复和备份/恢复工具。不新增Core实体、队列、workflow runtime或policy engine，不重新实现S3或IAM语义。现有MinIO SDK与官方policy ParseConfig/Equals继续处理协议和policy集合语义。

原operator锁定旧七目标/旧execution；本轮源码正改为一套新的严格六目标contract，旧版本只由已保存的二进制、Git anchor、原始source ZIP和历史validator解释；不能继续给旧授权加兼容分支。新来源身份仍为原三源，不重建/seed/替换原源，不复制旧permission为可发送许可。

一个必须保留的模型区别：Core仍注册18个历史copy identity（business八个包含已删除P1），而当前物理库存只有17。`RemainingDependencies.All`也用于核完整Core注册集，不能简单改成17而使ReferencesReleased永远UNKNOWN。新候选须分别绑定历史18、已确认缺失P1、当前17、六发送目标和十一survivor；P1历史身份/absence继续解释旧事实，永不回到target digest、sender、wire scope或新预算。

## 非删除有限验收

1. 六目标唯一顺序P2→T1→T2→H2A→M1A→M1B。先在真实SDK/loopback模拟provider组合与真实PostgreSQL事务中验收逐份“send→关闭发送→只读完整namespace absence→提交Core receipt→下一份”。模拟DELETE不连接任何保留Native；原三源不执行DELETE。
2. 在新admission前，旧未释放dispatch必须阻断；旧批准撤销及可信NOT_SENT/release后，只能由新批准、新dispatch、新attempt进入。旧attempt/失败/预算/pins/成本保持；旧批准不可重新激活，UNKNOWN不可重发。预算先于Core claim且O_EXCL不退还；下一份在absence未提交前不得消费新slot。
3. business只复用已完成的真实Processing/DirectData、owner/handle/pin/ref闭合及现存两个输出，不重跑生产、不生成新版本、不恢复P1。新execution的basis/批准/admission必须重新通过当前门禁，旧五项policy heads仅只读核对，不Renew、不新增RightsDeclaration。真实依赖路径不读取使用期限或Rights；新disposal basis/批准/admission不需要重开使用权。新business只有一个resume owner，合计六case/六recovery；CandidateUseUntil统一来自新窗口END。
4. H0/H1零发送；可信关闭证明仍需真实stopped/PID0/no-restart、撤销profile、全部enabler关闭、fresh disabled identity、sealed gateway，再追加NOT_SENT。H2A仅在真实上游204后witness/late Core holds A+B/revoke/响应丢失；H2B永不dispatch。M1A先absence，M1B再CONFIRMED UNKNOWN+C保护阻断checkpoint；原checkpoint复制核SHA后才B lookup，C永不删除。
5. 补齐六份成功组合、旧占用/旧批准、未关闭发送、预算扩张/重复、未提交absence、UNKNOWN重发和中途失败停止后续目标反例；mock、真实PG、Native只读、backup/restore和独立评审分别报告，不用一类证据冒充另一类。

## 冻结准备及确认范围

只读重查三源的当前policy heads、旧执行及终止事实、P1 absence、旧三项NOT_SENT/release和Core库存/roles/ACL，保存before/after相等的完整Core检查点。所有SQL只读，不迁移、不seed原源。

为当前17状态产生新的独立备份/restore检查点；旧九份及全部旧快照保留。restore必须使用新且明确归属的CID/卷/内网，不能别名原源，不向原源回滚，不恢复发送权限。恢复读回必须明确actual restore CID与original source CID，不能用原源核验冒充restore；配置只携带固定GET/HEAD技术凭据/读取期限，零删除，不能挂业务Token/预算/宿主签名私钥。沿用已有适配器及新的17对象检查，原源保持其全部历史事实。

新发送身份/policy须独立于旧身份，默认disabled；权限仅覆盖六份exact版本，新inspection仅覆盖当前17及必要namespace list。旧八身份及原policy继续disabled并保留旧绑定。任何新建IAM技术调用也先start再observation，新增费用按实际attempt留证，金额未知NOT_ASSESSED。当前只准备disabled身份，不启用发送。

完成上述门禁后才分配最终新execution、构建/冻结唯一当前source/binary、完整候选/IAM/active及sealed gateway、Core、backup/restore、实际owner/topology、全新空预算和全部profile窗口绑定。所有历史保全与当前完整入口必须共用同一核验路径；Docker raw wrapper保持bytes，唯一文本边界严格UTF-8。

精确确认方案列出全部冻结摘要、六份完整目标、十一份完整保留版本、P1缺失事实、上述固定窗口、每目标最多一次/全局最多六次/UNKNOWN不重发，cleanup=0、hold-release=0、bypass=0；失败/过窗立即封存，不延长/平移/重开。机器模板与真实人类原文分开记录，不提前伪造授权，不自动安排调度。完整D1仍NOT_ACCEPTED。

## 本轮有限收窄与原生准备入口

旧Renew/权利创建是为旧业务生产/传输服务。已沿RegisterFixtureDisposalBasis→RemainingDependencies→InspectFixtureReferences→Dispatch/Claim确认不属于当前新disposal路径，不继续作为新候选前置。原设计轻审、旧packet及所有失败回执保留，后续评审以此收窄为准。

新IAM用户用独立execution前缀，六份新delete权限只有DeleteObjectVersion及exact version条件；三份inspector覆盖当前17份。官方policy库不支持对legal-hold/retention metadata附s3:versionid：metadata IAM按exact key，固定SDK/gateway再按exact version；GetObjectVersion仍有version条件。必须在新完整预检核14个已知disabled identities及完整sole-policy/no-group/官方Equals，不能把新IAM期望伪装为旧实际receipt。新expected-contract明确标FROZEN_SIX_EXPECTATION_NOT_OBSERVATION；旧18历史实际receipt不改写。

原生准备只允许新用户以官方SDK SetUser(...AccountDisabled)直接创建为disabled，禁止AddUser的默认enable；root-only helper检查加密请求中的status、exact sole policy及mutation replay，保存真实技术start/observation、amount NOT_ASSESSED。此操作无DELETE/object PUT/旧identity enable，root secret不挂business/sender/recovery worker。源Core保持全关系/roles/ACL不变，新Core/native备份须实际分离恢复，旧九备份保留。

三Core在全局六slot及恢复核验期间保持可读，否则不得签下一slot topology；每次proof从实际immutable absence和stopped/PID0/permission false推出顺序，不读写host进度projection冒充Core事实。current worker须实际UID10001:10001、两个固定tmpfs、完整mount/netns/noports/no-socket/no-restart/cap-drop/no-new-privileges，才能签NoRoot。新持久budget须初始化为该UID/GID且实际为空；初始化owner的独立CID/命令/只此新volume也纳入完整挂载集合，不删除或退款任何旧预算。

完整预检、source/restore map、原生新IAM/backup/restore结果、宿主维护/封存入口及最终独立评审尚未完成，不将本合同或源码回归当成candidate冻结/新授权。

## 当前唯一入口与分层检查点

新一代使用 `prepare-six-candidate.py allocate` 分配全新stopped资源，`six_candidate_runtime.py materialize/initialize-budgets/check/run-non-delete` 进行有限非删除准备，`freeze-six-candidate.py` 只在真实三组source/restore验收通过后冻结。每阶段O_EXCL保存独立记录，失败保留、关闭全部已知CID，不回写旧快照。旧七目标生成器/预检/维护入口只解释其历史Git和二进制，本代不得调用它们作为发送/候选入口。

`verify-six-native.py` 是本代最终完整只读入口：六case/六recovery实际跨字段语义先于permission及Docker读取；之后三组完整owner/卷/网络/预算挂载图、30份false permission、九份新备份及旧证据核对，再启动固定Source，运行实际SDK十七版本、十四已知disabled identity、官方policy语义与完整Core前后指纹。完整入口只读；它不会启用IAM或产生发送授权。当前文件仍待最终入口独立评审和真实运行验收。

新分配恢复卷和internal network必须先只读证明同名不存在，创建后核execution+独立allocation nonce、local driver/options及全部CID挂载集合；任何同名已有/归属错误立即拒绝，不能启动其数据。新Caddy用tmpfs覆盖镜像默认卷，避免产生未绑定匿名卷；唯一旧卷挂载例外为固定Source native volume在archive/compare的`/source`只读挂载。新空预算由独立无网络、仅CHOWN能力的initializer初始化，旧预算从未初始化或退款。

本代实际冷态分配后发现Docker创建配置与运行endpoint不是同一事实：created/PID0、零StartedAt/FinishedAt且完全空endpoint时，须以精确HostConfig.NetworkMode、唯一批准network name及独立network inspect证明预绑定；已启动后缺NetworkID不得适用此分支。Docker inspect中的initializer能力只允许canonical CAP_CHOWN。原失败和allocation不覆盖，完整冷态复评通过前不得初始化预算或启动源。

初始allocation构建源与修复后的effective source以单一mandatory `source-checkpoint.json`明确区分。`six_candidate_runtime.py bind-source`仅在源码提交、冷态复评和Go/migration内容完全不变后，于新目录重编译四binary；只有四SHA与旧活跃文件全部相同，才O_EXCL追加checkpoint，绑定原allocation/CID/nonce/失败/完整重编译成员。原binary和allocation字节保持，缺checkpoint没有旧source fallback。最终source-freeze/archive/candidate绑定新的effective commit并同时保留initial build与重编译证明；该续接不授予启动、IAM enable或发送权限。

`window_six_support.py` 复用已独立评审的归档/逐步封存原语，收窄为本代真实三挂载case、完整十四身份和六份上限；不提供enable/send/schedule。H1签名仅在实际owner成功exit/PID0、原profile一字段撤销、所有late enablers及operators关闭、四个late身份fresh disabled及实际gateway封闭后签一次。准备阶段只在独立restore native上临时开启受限inspector，Source的旧/新发送身份始终关闭；restore enable失败也继续固定disable与物理关闭，未新鲜读回则UNCONFIRMED。

## 预算准备的有限续接

原source checkpoint01及四份相同字节重编译已实际通过；随后三个组的冷态profiles已独立核对。首次预算初始化中business owner成功exit0/PID0，但调用共享budget_archive时遗漏Helpers，导致KeyError；late/mixed从未启动。原失败、原预算tar、退出日志、partial receipt和checkpoint01均保留，不回写成成功。

当前入口统一由budget_candidate生成完整三组Copies+实际Helpers，复用既有remaining_sequence.targets/budget_archive进行六目标与真实TAR验证，不放宽共享校验器。source-checkpoint02强制引用原checkpoint01的固定SHA和原预算失败SHA，并再次在独立source-rebuild02目录编译四binary为完全相同字节。只有已评审五个host路径允许改变，Go及migration内容不变；没有checkpoint01/02 fallback。

finish-budget-initialization首先只读验证business原exit0/退出时间、原wait与tar SHA、当前空UID10001预算、所有权限false；late/mixed必须仍created、零启动、空root-owned预算。全部通过后才记录O_EXCL continuation-start，仅运行另外两个固定initializer各一次。business绝不重启或chown；任一未知/曾运行/非空/归属漂移拒绝。结果与各组前后tar追加为02，全部三个initializer均尽力关闭，关闭未确认即失败退出。完整非删除入口及冻结还要求此追加结果PASS、原始tar SHA及当前空预算复核；不删除或退款任何旧预算，不授予发送。
