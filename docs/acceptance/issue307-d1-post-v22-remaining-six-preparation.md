# #307 D1 剩余六份方案的准备边界

当前状态：旧dispatch闭合及17版本实际只读验收已完成；candidate13/v23仍**未冻结、未获新删除授权**。proposal execution为`4e524c04-2376-4bc9-9e85-452909d3dba4`，人类选择的方案窗口为2026-10-20新加坡14:00–14:40（UTC06:00–06:40），只用于准备；未安排调度。不是旧candidate12/v22的续跑。#332保持Draft、不合并；#307 OPEN。#323已按另外直接人类授权合并，旧Draft检查点原文保留。

原v22已经一次DELETE P1，原204/CONFIRMED ACCEPTED不可改写。P1仅通过独立新的只读切片追加absence及plan completion；原失败终态、旧预算、旧pins、旧授权及所有备份保留。P1无论后续对账结果如何均不能放回删除目标。

下列身份从原冻结模板逐行提取，仅作为未来新核验的预期范围，不能冒充当前bytes/SHA/hold/retention或源Core状态。business-races对应project dpp-lifecycle307-808a6960；late-hold对应dpp-lifecycle307-754ea8f0；mixed对应dpp-lifecycle307-dfda43b4；不同project的同名namespace/key绝不能混用。

## 六份预期目标

仅删除 P2 dpp-lifecycle307-online/fixture/p2.csv@d8040608-c5cd-4a6f-8a2d-b3f856cf5302；copy 6fabf3b5-76b8-4245-8f5d-01e64f610146；DV 4219373a-07a0-4c66-a666-305675304b14；SHA256 722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0；bytes 232；hold=False；retention=NONE；retain-until=None；plan bce1b1b1-d6e2-44fd-8b87-171142350ab9；hash d4758d9b64003d92aca60290fee1d2f22e7a857d2fe2be513d4a1ea2755ce6a3。

仅删除 T1 dpp-lifecycle307-online/fixture/t1.csv@39d4ef5d-e983-413d-a6c2-9759d456b750；copy f48755f1-976b-4f47-98da-70ead936a4c3；DV 699b8292-7fdb-4c22-af04-41f2eba761d4；SHA256 b5b9321c647c6dc75e27c25beb46ecf7f4cf10f9bdd0d288887e41c343ef3524；bytes 129328；hold=False；retention=NONE；retain-until=None；plan 247a2ca9-d2f3-4384-bcee-b10add497175；hash 7c4daf909a375bc21fcf8dd09b14d99960ae1f919ce138eb517ac0593faf1c1c。

仅删除 T2 dpp-lifecycle307-quarantine/fixture/t2.csv@90bd8ded-337a-4ca1-aec3-66c4ee6a1314；copy e09fd433-54cf-4e82-b8c4-098472505fa2；DV 70d9185e-5ee7-4d79-9853-64e8e8db3630；SHA256 a4ddf289321e3f5ede8528cb6268bd37d4165a727688477275a6497107fdaa4c；bytes 49；hold=False；retention=NONE；retain-until=None；plan 531c7b96-173f-4028-be97-2de9ed6c72f2；hash 84fbafb913dba44e5ceb43d670203e55f0244fad62a2dc2fd9ebb10b87e0cab5。

仅删除 H2 dpp-lifecycle307-online/fixture/h2-a.csv@453c6f42-fbb2-4fdf-9f86-00a20a54b7bc；copy c05307db-cb81-4220-b42e-be23f85821e3；DV 18721e15-7b01-47d3-a727-8b836d73dbcb；SHA256 b55663df28625841abe3cb9f14cf085c194e00f0a9bcefc53c1b7db83303a838；bytes 44；hold=False；retention=NONE；retain-until=None；plan d78a9339-0968-45c9-bab6-bda86810ec33；hash 205cff03fcbaf25ff4b5d3ad690e5261a8a45e626180aa0782f22e4e249bbffd。

仅删除 M1A dpp-lifecycle307-online/fixture/m1-a.csv@e8a8418e-c515-464e-99e5-10bc4c64fa8d；copy 5e712426-2109-4f0b-8791-df0a2be3a9f7；DV 44727cb2-5f3b-4b48-b891-931583488ea9；SHA256 e0f5abfc590c27d093d9250148f03bf4b169d262cc61d23af05f3e285c1de725；bytes 40；hold=False；retention=NONE；retain-until=None；plan c2596491-a620-4cda-83d1-4697c64e27f7；hash 8e4380ab68e62e934dae7609b2b4b1711e8c66d0fa2a5a742d0ce6cf059f3a75。

仅删除 M1B dpp-lifecycle307-quarantine/fixture/m1-b.csv@54900849-d59d-459e-90bb-275a5975d0a6；copy ad08947a-4c4d-4b0f-8c7e-2d89f287819f；DV 44727cb2-5f3b-4b48-b891-931583488ea9；SHA256 e0f5abfc590c27d093d9250148f03bf4b169d262cc61d23af05f3e285c1de725；bytes 40；hold=False；retention=NONE；retain-until=None；plan c2596491-a620-4cda-83d1-4697c64e27f7；hash 8e4380ab68e62e934dae7609b2b4b1711e8c66d0fa2a5a742d0ce6cf059f3a75。

## 十一份必须保留的预期版本

必须保留 dpp-lifecycle307-online/fixture/control-hold.csv@47026891-d10a-4400-a3cb-76915e3d51bf；copy 91d954ee-cce5-4ceb-840d-fd6bdab778be；DV 70b12ac5-1d86-462c-9176-fd5a681472cd；SHA256 ad335d6334c5e1213b18c16dd8fe5bbd5bb1c1f53351ab8f13ce6e3b04120a90；bytes 54；hold=True；retention=NONE；retain-until=None。

必须保留 dpp-lifecycle307-quarantine/fixture/control-retention.csv@2606d71f-207b-42a7-b671-4b7f4ca4d194；copy aa8ecd43-1c17-4d0b-b462-42600a426b1b；DV 6837388a-80b1-4512-8976-ce2a7bae4e85；SHA256 51c1c5e1a03e2347a4798ab8d9a312faef4853d028b7360ea84dfa8ea1616183；bytes 55；hold=False；retention=GOVERNANCE；retain-until=2026-10-23T06:25:44Z。

必须保留 dpp-lifecycle307-online/fixture/remaining-processed.csv@00d7e220-7714-4c31-a4c7-2dda08cfa306；copy 639c4cee-6a7c-4a13-aa68-37035d6e1811；DV a26216b5-95f6-4dbb-8359-c98f5f873be1；SHA256 d036fefe3225f322a8a2ad401bcc7e3e59127b98df0c6966d1b4c31018ff3fd0；bytes 232；hold=False；retention=NONE；retain-until=None。

必须保留 dpp-lifecycle307-online/fixture/remaining-standardized.csv@dac1dbc5-c9b5-412a-a2a6-bf082e109e22；copy da6cc6a8-c9c2-41a6-8811-2d46164a241d；DV 8cb255aa-a842-4aa5-a4ff-871798aeed0b；SHA256 f8a1a1c7f85ff910f44f011d0dc3aba1b410ace20cc7c4429e18ab6cccb1c77d；bytes 494；hold=False；retention=NONE；retain-until=None。

必须保留 dpp-lifecycle307-online/fixture/control-hold.csv@ca3b808e-734b-4d5e-8f2d-c78a16056ed6；copy a01751a1-9311-4357-8fb2-c833de6b8fc4；DV e6b6854f-b54a-462f-875d-9f74542b1fc3；SHA256 961b213e4d82865370d8da052e57af716297ddff615298ed8c43e938f39b1b7d；bytes 49；hold=True；retention=NONE；retain-until=None。

必须保留 dpp-lifecycle307-online/fixture/h0.csv@369808db-a39a-4327-b9ed-5eb83aef7477；copy b3c4e113-f5c5-4421-a9d6-08403b1b9d0a；DV 8d59d129-662b-4dae-a389-894943c716df；SHA256 e35bd4635f1d7ab38c85a290a86a94275085b49bdae8e1a952712d5304816d8e；bytes 44；hold=False；retention=NONE；retain-until=None。

必须保留 dpp-lifecycle307-online/fixture/h1-a.csv@28d9f84a-1889-498d-b033-df26cbf209f2；copy 0d8b77a8-195d-4a83-bd52-04f800d7dc68；DV 6d7d6978-cbbf-4c78-b9d1-82fad7271f0a；SHA256 838fddef805d918cd692a88f720d23705e6763eb0748424274c22d9bcfbc7ba0；bytes 44；hold=False；retention=NONE；retain-until=None。

必须保留 dpp-lifecycle307-quarantine/fixture/h1-b.csv@bae40a85-7539-4fe1-86b7-2cbb17df7812；copy e735d90a-8352-4a01-ad16-8573297bc684；DV 6d7d6978-cbbf-4c78-b9d1-82fad7271f0a；SHA256 838fddef805d918cd692a88f720d23705e6763eb0748424274c22d9bcfbc7ba0；bytes 44；hold=False；retention=NONE；retain-until=None。

必须保留 dpp-lifecycle307-quarantine/fixture/h2-b.csv@448112b2-baf3-4569-b06f-59011492cf80；copy a37c347d-dbff-492e-9c7a-317a1f188184；DV 18721e15-7b01-47d3-a727-8b836d73dbcb；SHA256 b55663df28625841abe3cb9f14cf085c194e00f0a9bcefc53c1b7db83303a838；bytes 44；hold=False；retention=NONE；retain-until=None。

必须保留 dpp-lifecycle307-online/fixture/control-hold.csv@a59df17d-70d7-40c5-88e8-8cfe77a915dd；copy f3167994-3790-4a6c-918e-ea6d95248b79；DV 57e8c123-6c69-44bc-98ba-d5db8c2b7aac；SHA256 367afd5e5ed47c5f2cadab63af56e0a5e82a1a32afa88f0c3f99448aab883821；bytes 45；hold=True；retention=NONE；retain-until=None。

必须保留 dpp-lifecycle307-quarantine/fixture/m1-c.csv@81a8402e-390f-4247-9d34-552df5eadeb3；copy 13fd8ecf-07fd-4981-b553-5ce66b67444f；DV 44727cb2-5f3b-4b48-b891-931583488ea9；SHA256 e0f5abfc590c27d093d9250148f03bf4b169d262cc61d23af05f3e285c1de725；bytes 40；hold=False；retention=GOVERNANCE；retain-until=2026-10-23T06:33:33Z。

## 冻结前必须解决的有限门禁

1. P1新的受限只读对账必须留下实际全namespace缺失、原生七版本HEAD/保护、精确Outbox→Asynq→Core absence和最终关闭事实；HEAD/保护不替代新六份候选的全部bytes/SHA核验。
2. 原P2/T1/T2曾有dispatch admission，必须以新的受限只读Core核对其实际attempt/claim/observation/release和当前占用。代码中UnresolvedDispatches以未释放dispatch计数；不能把预算空缺、缺少成本、旧stderr或“未发送”的外部描述当作可信NOT_SENT。原P2 claim错误已经丢失，不能补造其原错误。
3. 若确有未闭合的旧无调用dispatch，单独定义有限NOT_SENT闭合：复用DispositionService.RecordProviderObservation及其CapabilityObserveAttempt和既有签名owner终止证明；核真实旧owner/PID0/no-restart、全旧权限关闭、fresh delete identity disabled、gateway sealed、精确预算/wire日志和Core无冲突。仅追加可信NOT_SENT及dispatch release；不释放旧pins，不退预算，不变更原批准/失败事实。现有RemainingNoSendVerifier只覆盖late-hold H1，不能直接套用给P2/T1/T2；必须有当前六份准备所需的严格适配与非删除/真实PG回归、独立评审。该闭合不在P1只读合同范围内。
4. 先用非删除组合完成“每份send→禁用发送→只读absence→下一份”，实际错误/UNKNOWN保持，旧占用阻断反例和可信闭合后新admission正例均通过；不放宽Core完整库存/fence/旧事实约束。
5. 新候选必须实际只含上述六目标，11保留及原已删P1状态有独立绑定；全量新鲜17版本bytes/SHA/保护、三源Core、IAM语义、gateway、binary/source/owner/topology/预算与备份均须重新核验。原v22七目标预算/授权/配置不得复制成新的可发送许可。
6. 上述门禁和独立评审闭合后才生成新的execution、冻结绑定和完整确认文本，再选择未来固定窗口并重新取得人类精确授权；不提前制造将过期的窗口。当前所有发送权限关闭，不执行真实DELETE。

完整D1仍NOT_ACCEPTED；P1对账、R1/R2修复及后续六份验收分别留证。

## 最新准备检查点

P1门禁已由新audit d64ad338-00d2-4569-bd62-9e132ddaf578 在2026-10-19T10:58:17Z实际闭合：absence b2d74d75-ed07-4b0a-b530-6c5d308cf1b8、completion16562b28-3896-49c0-933e-a6e73059bddc；24项新的instrumented读取及Cost、原旧行保留和完整封存另见P1验收矩阵。本轮未新DELETE。其余门禁保持，尤其旧P2/T1/T2占用未做新Core逐项查询或可信NOT_SENT闭合，17版本全量bytes/SHA/IAM三源刷新尚未做；没有新删除候选、execution、窗口或授权。

后续检查点：新的只读 audit `23587686-0a95-4eaa-a010-a8c896c9735b` 已逐项核实 P2/T1/T2 各有一个未释放旧 dispatch、无 claim/observation/release/absence，三项旧批准尚未撤销。全 Core 指纹和旧控制/预算/备份保持一致，Core 已重新停止。此结果尚不是可信 NOT_SENT；后续新鲜宿主/IAM proof、Command 闭合、17 版本全量刷新及新候选均仍 NOT_RUN/NOT_FROZEN，详见旧 dispatch 闭合验收矩阵。旧检查点原文保留为当时状态。

最新实际结果：非删除audit `94f28318-cfd4-4c62-ba1a-bec9f35836c8` 已对原P2/T1/T2各追加旧批准撤销、可信NOT_SENT及dispatch release，三项unresolved=0；旧attempt/approval/pins/预算/成本保留，87唯一CID已停止，真实DELETE=0。执行终态 SHA `3ce05b24b298718b63dcd2c553c993b6f40a387c3a0eb26e13fda695ed5692cc`，独立实际结果终评 SHA `33bcf7eb3fd742d932996f5378cfa9870cca2e9009245f5b815152d485b7c80b`，均PASS且剩余阻断为空。8次真实技术IAM GET/观测单独保留，amount NOT_ASSESSED，不冒充零外部调用。详见旧dispatch闭合验收矩阵的最新追加。#323已按另一次直接人类授权合并，实际f22587d及其push/main CI39250314443全绿；当前后续代码在Draft #332，#307 OPEN。17版本全量刷新及新的六份候选仍NOT_RUN/NOT_FROZEN，没有新删除execution、窗口或授权。


### 17版本入口实现检查点（2026-10-19，尚未运行原生17核验）

旧P2/T1/T2已按可信终止、identity/gateway闭锁证明追加NOT_SENT及dispatch release，原预算/pins/失败事实保留。新只读合同见 `issue307-d1-post-v22-seventeen-contract.md`；当前实现分离原18-policy、唯一P1真实provider错误码HEAD absence及显式present17。维护profile不含业务Token/DB/删除权限/预算，Root凭据仍有provider root权限，只读由冻结GET/HEAD helper强制。

新Go读取器/文件聚合器及全部既有两包回归共114个PASS test事件、0 FAIL（source-local01原日志）；host完整成功组合、10种失败组合、budget归属/挂载5反例及未知inspect关闭，normal/-O各4方法通过，统一audit55分钟截止的补充回执见source-local02。source-local01的host输出是统一截止接线前快照，不伪称最终字节；原始评审harness/失败日志保留，当前source终复评仍待同一冻结HEAD。

17份原生读取仍NOT_RUN，六份新候选仍NOT_FROZEN；不创建新删除窗口/授权，不复用旧v22批准，不将mock视为native/IAM/Core事实。#332继续Draft，#307保持OPEN。


最新17版本结果（原检查点不覆盖）：audit f4330324-98de-4fc4-ae3d-e1feab7adaae 已实际PASS_COMPLETE_POST_V22_SEVENTEEN_READONLY，74次技术GET/HEAD、17份全部bytes/SHA/保护/P1精确缺失、三源Core不变及最终封存通过；独立结果终评SHA c6508108d16f6095889b7dce66bb26d4940073faf1934a8b9b1fed9ec69ab616，remaining_blockers=[]。详见issue307-d1-post-v22-seventeen-acceptance.md。人类另选新方案窗口2026-10-20新加坡14:00–14:40，仅用于准备，不构成新删除授权；六份contract见issue307-d1-post-v22-six-contract.md。六候选尚NOT_FROZEN；原预算/pins/失败/所有备份保留。

### 六份候选源码准备检查点（尚未分配或运行新原生环境）

| 有限工作 | 当前结果 | 证据边界 |
| --- | --- | --- |
| 旧P2/T1/T2可信NOT_SENT/release | PASS | 真实原源与独立结果复评，旧行/预算/pins保留 |
| 17版本及P1精确缺失 | PASS | 真实74次GET/HEAD；与新一代IAM/备份验收区分 |
| 新批准/旧dispatch阻断 | PASS | 真实隔离PG三方法；无真实provider删除 |
| 六份SDK→wire→关发送→恢复组合 | PASS_LOCAL | loopback/double，无保留Native DELETE |
| 当前Go源码回归 | 218 PASS、1 SKIP | 缺DSN的旧adapter测试明确skip，不冒充PG结果 |
| 准备入口 H1–H4 独立评审 | PASS_BOUNDED_SOURCE | report `host-source-final-review02.json` SHA37e575297ce74367fba2734ced5e05998a5fc3dba73261e403db3500303ced07；排除最终新三入口/native |
| 新冻结/完整只读/H1封存入口 | PASS_BOUNDED_SOURCE | `new-entry-source-final-review02.json` SHA1d0d9f0bbff1fce4c82e05adaa54e52e1b9b135bc10a3544058b60180dfa918c；原E1/E2失败模型保留；完整collect、H1真实本地签名和103步seal模型正常/-O通过，不冒充actual |
| 新IAM、九新备份、分离恢复及业务重跑 | NOT_RUN | 新allocation尚0；Source发送权限未开启 |
| candidate13冻结及完整精确文本 | NOT_FROZEN | proposal execution/窗口不是授权，完整D1仍NOT_ACCEPTED |

当前实现/权威入口和非目标见`issue307-d1-post-v22-six-contract.md`。所有早期NOT_RUN/无窗口等段落是当时检查点，不覆盖上方最新状态，不据此否认后续单独留证的真实结果。

### 冷态分配前检查点

本代运行源码已提交为`a0f938943a7b396ecfed354ee4294506bd3d632e`；精确该HEAD的PR CI39286329231及required汇总均SUCCESS。原始API读回单独保留于`post-v22-six-preparation01/source-pr-ci01*`：13项中12项摘要SUCCESS，web-console全部步骤（含Complete job）SUCCESS而其单项摘要暂未更新，故不将该次读回写成13/13摘要通过，更不称为main CI。

窗口准备手册的独立一致性评审已PASS，报告SHA`53987dc760b2059f1524e74fac201b278936a23c8fea3bfefecb2407e8cf74d0`，手册SHA`e2507fcf21459c6f9bfa1b2a666f6bdd6630d64a3283fd2f064f9bfaeaead6a2`；仅关闭角色前置措辞问题，86项已评审源码无漂移。该结果不是actual allocation、Native、候选冻结或授权证明。

现在只启动一次性冷态分配：编译精确源码并创建停止资源，原源/身份未启动或开启。实际CID/卷/挂载及预算初始化结果须另存并独立核对，三组非删除原生验收仍NOT_RUN；候选尚未冻结、授权NOT_RECEIVED、调度NOT_CREATED、完整D1 NOT_ACCEPTED。

### 实际冷态分配与前置失败（后续追加，不覆盖原记录）

新allocation已实际创建86个停止owner、15个新卷及三个独立internal恢复网络。原始allocation SHA`91198e014b72d289ddf9d071e5ba888c904f7638fbb70c9a01dfc6e111da80a7`；nonce `d54522c4-20db-4be6-9bb7-f2f8ed9b3aa3`。原初四份构建来自a0f938，完整CID/descriptor/map/关闭回执均保留于`remaining-candidate13-preparation01`。

首次materialize在任何profile写入或owner启动之前被冷态门禁拒绝：从未启动的恢复容器只有创建配置中的精确network ID，runtime endpoint的NetworkID为空。独立实际冷态评审另外发现initializer的Docker实际cap值为CAP_CHOWN，与原字符串CHOWN检查不符。原失败`materialize-failure01.json`及原BLOCKED独立报告SHA`09613e8bf682a8b4bb51fc36564c547383e2728a0439b8afc437e7f28b549e3a`保持不变。86个新owner均created/PID0，六个原source Core/Native保持关闭；56个受限worker、15卷完整挂载图、三网络及六处旧卷只读挂载实际核对通过。预算尚未初始化、Source/IAM/restore仍NOT_RUN，真实DELETE=0。

修复严格区分created/PID0/零StartedAt且完全空endpoint的网络预绑定与已启动endpoint；两者都要求固定HostConfig网络ID及唯一批准网络名，foreign/extra/未知/曾启动后空endpoint拒绝。initializer只接受实际canonical CAP_CHOWN，不扩大cap集合。当前新增实际Docker投影正例、网络/状态/额外endpoint、cap扩张及source跨绑定反例；四组相关Python入口正常/-O均通过。这是本地回归，尚不是实际冷态终复评或Native验收。

原设计把allocation时的构建源与后续宿主driver变更绑成同一个可变快照，不能为修复而回写旧allocation。当前改为单一mandatory追加source checkpoint：明确保留initial_build_source，绑定effective reviewed commit、原allocation SHA/86CID/nonce、原四binary、原失败SHA和独立目录四份重编译证明。apps/platform及migrations必须逐字节不变，只有已评审五个host Python/测试路径允许变更；四份重编译SHA必须完全等于原活跃binary，原文件不替换。缺checkpoint无旧source fallback，任何差异拒绝继续。checkpoint及重编译尚未实际执行，独立终评后才绑定；不是新授权或候选冻结。

后续独立终复评`cold-source-independent-final-review03.json` SHA`107837f89bac730d7d9e81f7093badab00fe8e129213cb16850d4ee9cf286da2`已PASS、remaining_blockers=[]：COLD1/COLD2均闭合，实际冷态入口于16:57:28Z对86原CID通过。追加SOURCE1反例（重算receipt/checkpoint后省略编译通道）已修复为四binary+八原编译通道固定12成员、实际目录全集/regular/无链接、完整SHA map及跨字段校验；原BLOCKED报告SHA`70fe77016c02d9903b187751a8e94bd7102fa7a1d14caaa0b2c643aa63c0ad45`不覆盖。最新受影响runtime正常/-O各10方法独立通过，其他三套已通过且源码未变。下一步提交精确已评审修复、追加实际相同字节重编译/source checkpoint及cold profiles验收；这些实际步骤仍NOT_RUN，不将源码/冷态PASS当成Native或授权。
