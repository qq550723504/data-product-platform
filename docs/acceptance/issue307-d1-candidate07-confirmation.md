# #307 D1 candidate07/v17 精确确认方案

当前状态：`AUTHORIZED_WAITING_FIXED_WINDOW`。本线程已收到人类对完整candidate07/v17及全部绑定的新按引用授权，execution `effa0385-9308-42eb-bd66-d0c379b6a989`，只在2026-10-17新加坡10:00–10:40且所有前置通过后有效。[新授权回执](evidence/issue307-d1/delete-confirmed-v17-execution01/human-authorization.json)保存实际原文及批准模板SHA，`user_retyped_template=false`，旧授权未复用。#323保持Draft，#307保持OPEN。当前真实DELETE、发送claim、hold release、bypass、cleanup均为0；32份运行权限仍false，未签发可发送topology proof。

窗口前准备与定时续跑、root回执归档、H1实际闭锁及失败封存的有限操作顺序见[固定窗口运行手册](issue307-d1-v17-window-runbook.md)。冻结包的原 `status.json`、candidate、template、bindings、source和三个binary保持原字节；历史NOT_RECEIVED是当时准备快照，不覆盖成新授权事实。

本方案接续 candidate06-preparation/v16 的三组新 fixture；旧5d、旧 execution 与已消费确认不属于本轮。完整 D1 尚未验收，H2 的真实204后响应丢失与M1的真实 mixed outcomes 均 NOT_RUN。

## 1. 唯一冻结身份

| 绑定 | 精确值 |
| --- | --- |
| execution | `effa0385-9308-42eb-bd66-d0c379b6a989` |
| candidate SHA256 | `b9660f17cbb745e781a06d50e8ec0e81c87e654318420533b75b4ada29728949` |
| 精确确认文本 SHA256 | `f8153a89ee2c12a631be9e665e0c85e5951a2ad2cd8f99961fa24721336857b0` |
| 冻结源码 | `f3b5de45857add3f852bbac0348e8c30ea105ade`；812文件，包含embedded remaining scope JSON |
| source-freeze SHA256 | `76230f4e1e4a6deec8ad0b3d5e0b707821c6400834de895111122191d761b453` |
| Linux reader/producer/case binary | `8a9800bd3a77c75d3364cfe01712e7de1dcb060111a4b43c4bb90820b7b2a4b1` |
| Linux sender/wire/recovery binary | `ef6dbfd8561ac6a070a7e1d19c45caec0042924cbc268a8a58ff0f85a5bf2f9a` |
| Linux IAM bootstrap binary | `68ad4a761a105e7d5ad2a678546f80347222b6fab11c0b4b64b49f15989f9d42` |
| 七目标 digest | `e2e04f90d31bf3b60987172d142f3cd7ec850e12ea566d9b002236b2cf081034` |
| 冻结绝对窗口 | 2026-10-17 02:00:00Z–02:40:00Z；新加坡10:00–10:40；只适用本次已记录的新授权，不能顺延 |

完整逐项绑定见 [candidate.json](evidence/issue307-d1/remaining-candidate07/candidate.json)、[bindings.json](evidence/issue307-d1/remaining-candidate07/bindings.json) 和 [940份raw证据索引](evidence/issue307-d1/remaining-candidate07/evidence-index.json)。Case owner 7个、sender/wire/recovery owner 17个、独立root bootstrap owner 15个均绑定实际完整容器ID。原始失败seal与stderr采集续跑回执分别保留，未替换candidate/source/profile字节。

## 2. 有限设计及验收矩阵

| 项目 | 当前验收 | 证据 / 后续边界 |
| --- | --- | --- |
| 真实业务与受限身份 | PASS | 真实entity matching/native PUT产生两份保留输出；reader仅exact-version GET，producer仅两个固定key、signed If-None-Match；native完整单policy、无group继承、所有源身份disabled读回 |
| P1 | PASS_NON_DELETE | 真实Asynq/NATIVE GET在活动pin期间被restriction阻断处置；真实owner/handles闭合后才释放typed refs，最终无READY输出、fresh dispatch可admit；send claims0 |
| P2 | PASS_NON_DELETE | 排队早于restriction；真实worker、queue replay、显式Retry、Start与NativeReconciler均0新增GET/PUT、无新输出 |
| T1/T2 | PASS_NON_DELETE | 真实TCP 129328 bytes背压；取消仍保留pin至writer与flush闭合；ISSUED历史不改，same-key HTTP409及restriction后T2 HTTP403均0payload/I/O |
| H0/H1 | PASS_NON_DELETE | 最新冻结8a9800bd binary在schema69 restore clone重跑：实际PG fence wait；H1 revoked/unclaimed仅凭可信签名owner终止与disabled identity/deny-DELETE gateway记录NOT_SENT并释放reservation；该NOT_SENT无absence或provider invocation成本，metadata调用成本独立保留；H2-A仅READY_UNCLAIMED |
| M1/C | PASS_NON_DELETE | 原生C的GOVERNANCE retention实际阻断，0C attempt；同DV三版本身份保留；真实A absence/B UNKNOWN/C blocked组合尚NOT_RUN |
| policy续期 | PASS_PG | 显式Renew追加不可变policy单链；旧policy/pin/operation/checkpoint不改；双Renew一赢家、restriction/hold/pin六顺序真实waiter、跨scope/stale/pending output/超窗/失效authority反例PASS |
| 三组Core/native备份恢复 | PASS | schema69、每库194表全row/hash、roles/ACL；native tar文件SHA及恢复后的exact GET/库存/保护/disabled IAM均PASS；全部备份和restore volumes保留 |
| 新冻结前只读核验 | PASS | 三源Core fingerprint与checkpoint逐项相同；18 exact版本完整库存/bytes/SHA/hold/retention；root GET/HEAD-only journal，bootstrap是技术核验，不冒充业务CostEvent |
| 发送与恢复接线 | PASS_NON_DELETE | 单target持久budget先于Core claim、SDK retries=1、redirect禁止；真实上游204后才签wire witness；H2随后hold/revoke并丢响应；M1先C-only阻断mixed checkpoint再B lookup；恢复经Outbox→真实Asynq选定work，无resend入口 |
| 封存/离线反例 | PASS | 17实际operator与7实际case拒绝；source Core/native物理stopped；sender/wire由permission=false先拒绝，recovery由缺fresh signed readonly topology拒绝；Go offline validator、Python正常/-O三项及嵌套篡改反例PASS |
| 独立复评与最终检查 | PASS_BOUNDED_REVIEW / SEE_FINAL_CI_CHECKPOINT | 独立只读复评重核812源码、940 raw及最终73份native回归，当前切片BLOCKER=0；67个精确owner CID再次inspect均stopped/PID0/noports/no restart。最终精确HEAD的Linux PR CI在GitHub checkpoint记录 |
| 实际删除与完整D1 | NOT_RUN / NOT_ACCEPTED | 所有源版本仍在；七次未来发送预算目前为0。旧授权不复用；#307不关闭 |
| 新授权/窗口前准备 | AUTHORIZED_WAITING_FIXED_WINDOW | 新按引用授权与一次性本线程续跑已登记；32权限false、58个绑定容器停止/PID0；回执归档、H1配置单字段撤销及封存故障组合离线通过。真实fresh核验、签名、IAM动作和发送均留待已授权窗口 |

最新证据：[业务原生重跑](evidence/issue307-d1/remaining-executable03/business-races/regression/remaining-business-regression.json)、[六顺序PG与历史保留](evidence/issue307-d1/remaining-policy-regression03/tests.txt)、[最新冻结binary H0/H1](evidence/issue307-d1/remaining-final-regression01/result.json)、[H0/H1完整证据](evidence/issue307-d1/remaining-final-regression01/evidence-index.json)、[M1/C补跑](evidence/issue307-d1/remaining-executable04/checkpoint-completion.json)。

[最终独立复评](evidence/issue307-d1/remaining-final-review01/independent-review.json)仅覆盖本轮有限切片。额外Windows全模块测试保留FAIL：既有metadata启动测试500ms父context内未取得invocation回执；同测试binary首次子进程实测1404ms，后续23–27ms，原测试隔离五次PASS仅用于诊断。相关两份源码与origin/main无改动，超时保持UNKNOWN且source gate为0；未改源码或放宽门禁。Linux精确HEAD PR CI仍为必须检查，不用隔离重跑覆盖失败，诊断与原日志见[独立回执索引](evidence/issue307-d1/remaining-final-review01/evidence-index.json)。

原HEAD `9a41e13f` 的CI暴露一份新增IAM测试漏gofmt，以及`remaining-case02/gateway.Caddyfile`本地原CRLF入Git时曾被转换为LF。修复只格式化该`*_test.go`、把原CRLF重新入库；候选、profile、binary、template及冻结f3源码对象不变。[严格冻结快照校验](../../examples/lifecycle-d1/check-remaining-candidate07-freeze.py)固定原index/candidate锚点，940 raw同时核工作区与Git blob，812源码从f3 archive核验并运行未改的原checker；当前源码只容许该测试等于固定gofmt SHA，其余逐字匹配。运行代码漂移、测试语义变化及raw/index改写反例均拒绝；不将当前PR HEAD冒充binary来源。

CopyPolicy原实现每DV唯一且不可变，并限定一小时；直接写明天期限或重复Bind均被正确拒绝。migration69补充显式expected-head续期与单链current view，维持workspace fence→同DV全部copy锁→previous head锁，拒绝restriction/hold/pin/reservation/UNKNOWN/未闭合output。未来窗口内必须比对冻结六个head再Renew，不能把今天的短时policy当成明天的使用授权。

Reuse Check：复用既有Commands/PG fence/immutable facts、FixtureSubjectPins、NATIVE/DirectData SPI、Outbox/Asynq、MinIO SDK与madmin官方API、Ed25519/cryptography。未新增通用IAM、scheduler、workflow runtime或自研密码协议。Core新增部分仅为平台fixture policy续期语义；provider SDK留在adapter/cmd组合。

## 3. 授权后才允许的有限顺序

1. 当前人类已对下方唯一文本及全部绑定按引用授权；实际人类原文与逐字批准载荷分开留证，不声称用户重新键入模板。核对当前PR HEAD/Draft和源码/三个binary/备份/容器完整ID。发送前重新做三源只读Core与18版本库存、保护、IAM核验；当前baseline若漂移，停止，不沿用旧授权。新确认不得转用于其他execution或窗口。
2. 启动绑定的三源Core/native/gateway及各自固定Redis；gateway安装绑定active配置，9000端口始终无DELETE路由，9002仅由冻结wire提供。root bootstrap与所有business/sender/recovery挂载分离。
3. 只在固定UTC窗口内，验证本次按引用授权原文、envelope、全部冻结绑定及批准模板原字节，再将批准载荷写入对应private human文件和bounded permission（ConfirmedAt、NotBefore/ExpiresAt、max1/7、全部SHA）。不得仅从template自动伪造人类授权。root `enable-read`先验证冻结IAM manifest→每个golden policy文件SHA→OSS set equality→完整单policy关联，再启用限定reader/producer/inspector并读回。任何失败先执行独立 `disable-all`；闭锁不依赖策略校验成功。所有root helper再次运行前保留归档旧O_EXCL回执，不把旧结果冒充fresh。
4. business的`renew` owner先核对六个expected previous policy IDs，追加本execution的新policy，期限≤min(now+1h,02:40Z)。随后`business` owner运行真实P1/P2/T1/T2并证明owner/pin/typed refs全部闭合。late `late` owner先完成H0/H1及H2准备；H1使用冻结close配置的独立closure binding签可信owner终止证明，再运行`close-H1`，不产生claim、absence或provider invocation成本；metadata调用成本独立保留。mixed `mixed`仅先准备A。
5. 每个sender/wire/recovery启动前运行源码化 [host attester](../../examples/lifecycle-d1/attest-remaining-executable.py)，它从actual Docker inspect核对CID、image、network/netns、readonly mounts、config/binary SHA、无ports/socket/root-secret、no restart、gateway hash和persistent budget，才签≤1分钟topology。host私钥永不挂worker；wire使用独立key。
6. root `enable-send`仅启用该组冻结七目标子集的exact-version删除身份并读回。每个target先fresh gate，持久O_EXCL budget再claim；P1/P2/T1/T2各一次，再经绑定`recover-*`实际Outbox/Asynq只读exact absence核对。H2只允许A一次；真实上游204后签witness、提交approval revoke与A/B late holds后丢响应；B永不dispatch，A UNKNOWN仅lookup不重发。M1先A一次→exact absence，再B一次→CONFIRMED UNKNOWN→C-only native protection+Core阻断检查点；其后才可复制精确checkpoint至`recover-M1B`并lookup，C永无发送预算。
7. 只读对账7目标/11survivor、observations/absence/completion及每次真实调用成本；保存UNKNOWN与原start事实。立即关闭send identity与所有payload reader/producer；仅在独立只读恢复权限有效时保留inspector，完成后也关闭。实际stop/PID0/no restart、deny-all gateway封存、所有backup/volumes/helpers保留，cleanup=0。不改变#323 Draft，不关闭#307。

本页不给真实删除授权。若拟议窗口未获授权或已过期，须新execution/新冻结/新精确文本；不能平移或重开本方案。

## 4. 唯一完整精确确认文本

原字节文件：[confirmation-template.txt](evidence/issue307-d1/remaining-candidate07/confirmation-template.txt)。以下逐字展示；原文件SHA256如上。

```text
CONFIRM_DELETE_EXACT_SEVEN；execution effa0385-9308-42eb-bd66-d0c379b6a989；candidate07/v17。
确认 candidate SHA256 b9660f17cbb745e781a06d50e8ec0e81c87e654318420533b75b4ada29728949；source f3b5de45857add3f852bbac0348e8c30ea105ade；source-freeze 76230f4e1e4a6deec8ad0b3d5e0b707821c6400834de895111122191d761b453；
Linux reader 8a9800bd3a77c75d3364cfe01712e7de1dcb060111a4b43c4bb90820b7b2a4b1；Linux sender/wire/recovery ef6dbfd8561ac6a070a7e1d19c45caec0042924cbc268a8a58ff0f85a5bf2f9a；target digest e2e04f90d31bf3b60987172d142f3cd7ec850e12ea566d9b002236b2cf081034；
完整环境/原生保护/Core 绑定 bindings SHA256 ede110b20b2e0815060a7ba1e599db073e56c4c476a3d92d358e20a6ec898562。
固定窗口 2026-10-17T02:00:00Z 至 2026-10-17T02:40:00Z（新加坡 2026-10-17 10:00–10:40），过期不能平移、延长、重开或重试。
环境 dpp-lifecycle307-808a6960；workspace a4aa2689-6e8f-4180-a567-dfb875407eac；provider cc068c8c-4f00-44cd-a825-377631253d59；incarnation d5f220ca-6133-42a0-a31c-1cf717d05274；manifest 2fd4356702a7903af53d4f1f6f8dc6c40372bed11b7069bb32beb130a5077c35。
Core 9bc6e9ec894f9fecb1566940d9b4680e0a9ddee52f31f9aec1f45afdb35c7ab0；native f16efe4cfa33694dc57b6cc2afe0a372379abce29846ba523ddd1dce455f43ce；gateway 1c5877bbfabdd1a812fa658e62e594ad76a8c7ae815aa41d5ee06274eb6ed39a；network 6354fa508f7adb9ac66fc895e539786a5575e0b3754d9fdb2677d0557ab7c942。
Core dump 75f02109f039c6ee2735e976ee3c1c7bb9d5f2fd643cface5849e206eed11bab；globals d738e649fe404e45e778808ab074d93ca0ba2d90cb5c39a1c6592bb25aa5f360；native archive 40fdf5125d8df98b97f91eaaf2aad0eb565322e79c5c95704d580f8414973255；restore Core ee8ca5d8fd9818c3bbcf1d194c0cb37e397a85ac9163929054c16ac3ef233290；restore native 728e4dbdbf94713c5467edd31e4f9b5669535f6dc23878714504c6016405fe3c。
Core readonly 2b94c8d837ddc606816c6c49f6f4b3c5ffe27cbb7081f1acca3fbfdada159124；native readonly f4f291a0a7147d09a6fba86a5d750f289b5bd66d78814c74a3ff0b90ae261c4b；IAM 4997eeddaebd6b9584f1a21b76ea3abe690c8e0770f3f60f78c905b494635d2a；active gateway 08a5dd89018781146ce35f97c070704d53ea88a384eb91c38ea39be86af09c07；sealed gateway 6cbf354b0f6e12adf155dd50110443fdffc8e9d1a76b521a51f9eaa2dcc5b1ad。
必须保留 dpp-lifecycle307-online/fixture/control-hold.csv@47026891-d10a-4400-a3cb-76915e3d51bf；copy 91d954ee-cce5-4ceb-840d-fd6bdab778be；DV 70b12ac5-1d86-462c-9176-fd5a681472cd；SHA256 ad335d6334c5e1213b18c16dd8fe5bbd5bb1c1f53351ab8f13ce6e3b04120a90；bytes 54；hold=True；retention=NONE；retain-until=None。
必须保留 dpp-lifecycle307-quarantine/fixture/control-retention.csv@2606d71f-207b-42a7-b671-4b7f4ca4d194；copy aa8ecd43-1c17-4d0b-b462-42600a426b1b；DV 6837388a-80b1-4512-8976-ce2a7bae4e85；SHA256 51c1c5e1a03e2347a4798ab8d9a312faef4853d028b7360ea84dfa8ea1616183；bytes 55；hold=False；retention=GOVERNANCE；retain-until=2026-10-23T06:25:44Z。
仅删除 P1 dpp-lifecycle307-online/fixture/p1.csv@fed0102b-bc8b-4f47-b9e2-fae8d5c563ce；copy 5a4c9789-3e67-4e54-ab58-128d111cbd65；DV 76f7f44e-4a2a-46c6-a6dc-a529f9ade834；SHA256 722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0；bytes 232；hold=False；retention=NONE；retain-until=None；plan 621893ed-16fb-4f21-a6c6-2711d349107c；hash a852eb63484945a92996667f6ccee9d39e6dc0dbf20c864ff62310a383e3a0b8。
仅删除 P2 dpp-lifecycle307-online/fixture/p2.csv@d8040608-c5cd-4a6f-8a2d-b3f856cf5302；copy 6fabf3b5-76b8-4245-8f5d-01e64f610146；DV 4219373a-07a0-4c66-a666-305675304b14；SHA256 722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0；bytes 232；hold=False；retention=NONE；retain-until=None；plan bce1b1b1-d6e2-44fd-8b87-171142350ab9；hash d4758d9b64003d92aca60290fee1d2f22e7a857d2fe2be513d4a1ea2755ce6a3。
必须保留 dpp-lifecycle307-online/fixture/remaining-processed.csv@00d7e220-7714-4c31-a4c7-2dda08cfa306；copy 639c4cee-6a7c-4a13-aa68-37035d6e1811；DV a26216b5-95f6-4dbb-8359-c98f5f873be1；SHA256 d036fefe3225f322a8a2ad401bcc7e3e59127b98df0c6966d1b4c31018ff3fd0；bytes 232；hold=False；retention=NONE；retain-until=None。
必须保留 dpp-lifecycle307-online/fixture/remaining-standardized.csv@dac1dbc5-c9b5-412a-a2a6-bf082e109e22；copy da6cc6a8-c9c2-41a6-8811-2d46164a241d；DV 8cb255aa-a842-4aa5-a4ff-871798aeed0b；SHA256 f8a1a1c7f85ff910f44f011d0dc3aba1b410ace20cc7c4429e18ab6cccb1c77d；bytes 494；hold=False；retention=NONE；retain-until=None。
仅删除 T1 dpp-lifecycle307-online/fixture/t1.csv@39d4ef5d-e983-413d-a6c2-9759d456b750；copy f48755f1-976b-4f47-98da-70ead936a4c3；DV 699b8292-7fdb-4c22-af04-41f2eba761d4；SHA256 b5b9321c647c6dc75e27c25beb46ecf7f4cf10f9bdd0d288887e41c343ef3524；bytes 129328；hold=False；retention=NONE；retain-until=None；plan 247a2ca9-d2f3-4384-bcee-b10add497175；hash 7c4daf909a375bc21fcf8dd09b14d99960ae1f919ce138eb517ac0593faf1c1c。
仅删除 T2 dpp-lifecycle307-quarantine/fixture/t2.csv@90bd8ded-337a-4ca1-aec3-66c4ee6a1314；copy e09fd433-54cf-4e82-b8c4-098472505fa2；DV 70d9185e-5ee7-4d79-9853-64e8e8db3630；SHA256 a4ddf289321e3f5ede8528cb6268bd37d4165a727688477275a6497107fdaa4c；bytes 49；hold=False；retention=NONE；retain-until=None；plan 531c7b96-173f-4028-be97-2de9ed6c72f2；hash 84fbafb913dba44e5ceb43d670203e55f0244fad62a2dc2fd9ebb10b87e0cab5。
case owner renew=28cb26b50156b9cf0d76a0c564944fede7189738b897c4dd57800408e19723ec；profile SHA256 10e83e8d0dace68454c19c4780890238e575e3d23acd98ba8ac0cb5aedd01fb0。
case owner business=650b2d2628ecea13ffdca72ba74dcbfdb239069850b26606021193d08b0dbcab；profile SHA256 10e83e8d0dace68454c19c4780890238e575e3d23acd98ba8ac0cb5aedd01fb0。
operator owner wire=e6bcb61bd4a017592aaa0f60d3282b0b068552d36c502634a9a1ae0c5b1cb262；config SHA256 c81cd73e787239d2d628eb235b04b0bc40ff0e29a2f71211b86443092a50b884。
operator owner P1=82d23cd675b64152050f79945269644f9d91979da526ff3dea84c3c0bc66597d；config SHA256 0ef3300590dbf690eb13d9003eb96b48380979c1a5eb595a5d8f011837132b61。
operator owner recover-P1=37e5b24eeda1bec7fadbd36aba7979a5a0cb0ce575f6888321488b957bfa379f；config SHA256 14d59634a14f89395e68376339410538b259da477e0d8ac65947ed14f534b675。
operator owner P2=3706f1d897bb9e6c4b75ee5998544397f848b0d6370ef689636409b63668813b；config SHA256 0ef3300590dbf690eb13d9003eb96b48380979c1a5eb595a5d8f011837132b61。
operator owner recover-P2=ad85170ff48a917eea1d7e5f140ee54871c96b34d7cff81feee2dbc5f8730880；config SHA256 14d59634a14f89395e68376339410538b259da477e0d8ac65947ed14f534b675。
operator owner T1=105de0890e02ab070730b2243f9d6581107f809edb7878147c95fafac47daace；config SHA256 0ef3300590dbf690eb13d9003eb96b48380979c1a5eb595a5d8f011837132b61。
operator owner recover-T1=ccbc5f9e83df921bd337a937c4bc25953138b3e5a7c299ae84de17235a15b210；config SHA256 14d59634a14f89395e68376339410538b259da477e0d8ac65947ed14f534b675。
operator owner T2=966334e63550442035452aeda0dd7759b1097314b878d9b49b56d4f7db7cbe21；config SHA256 0ef3300590dbf690eb13d9003eb96b48380979c1a5eb595a5d8f011837132b61。
operator owner recover-T2=ce7de7cda7e3225995e7dedf37eed450b8294abf1809c0eee810f5b5d5c70834；config SHA256 14d59634a14f89395e68376339410538b259da477e0d8ac65947ed14f534b675。
未来续期 expected previous policy：DV 76f7f44e-4a2a-46c6-a6dc-a529f9ade834 -> d32df045-735f-4e1c-9043-4acfb41d6de1；漂移即拒绝。
未来续期 expected previous policy：DV 4219373a-07a0-4c66-a666-305675304b14 -> 1dce673b-2d6c-4932-82a5-3c63be658b27；漂移即拒绝。
未来续期 expected previous policy：DV a26216b5-95f6-4dbb-8359-c98f5f873be1 -> 7d5d599b-1965-45b2-9008-8a0cdf0d75f6；漂移即拒绝。
未来续期 expected previous policy：DV 8cb255aa-a842-4aa5-a4ff-871798aeed0b -> ec9c4b58-288a-4e0d-a4a3-23bd54c73a90；漂移即拒绝。
未来续期 expected previous policy：DV 699b8292-7fdb-4c22-af04-41f2eba761d4 -> e369ebcd-4b7a-4490-a35d-2131fa61faf9；漂移即拒绝。
未来续期 expected previous policy：DV 70d9185e-5ee7-4d79-9853-64e8e8db3630 -> 105fd8c9-5ab3-4940-a87a-4071f4416fb0；漂移即拒绝。
环境 dpp-lifecycle307-754ea8f0；workspace 64f71fd8-def6-46a9-a314-f4645b1595b1；provider ab8ba084-1768-401a-9f27-fd4dc0c27dad；incarnation 9d5c2daf-40be-4155-811a-879f24ad0314；manifest fa787f45239d7b8d02367fff7827fbb0a04613f55fdaf5e965cb66d0916b2a80。
Core 982c4e8d2e13331ee94f62148f1bfa6846418a743599cce2270748f8205ed0db；native f26f011ae3d6e83d012fd914462ee00c153a077266d38e37a57856b2e5148061；gateway 2e2192530e9ff4d45bfdae8fc797089fd8ce0969d37cba97e00d63986f4e194c；network e27a29ee55732447bff932d15bcb83339784089c00f03b9139232417fe1a8847。
Core dump 63651c3f5f2b93c393e12c8ddec627338132fc5261d8193083522d6736f3a5da；globals 06c767c194d83e91552c72b9daf5e1637480506f5a432b7d2ff342ec18874fce；native archive a51518f5ec922fe2a887a346685192f219650409f76f64518facaaf7b29aa25a；restore Core 7ba6a70d54b49021ab50156a9fdf3bd7a57bdd9b93dbbb441b3837614a665639；restore native 82024ecd2d4cd8afa46b33d1d28abfddffa4093d6b9dd9b24d67fc046bb6eabd。
Core readonly 7aafb25df2004970410fc80ebbe1206c49ebf2d3315d4294a95c7c903b10c01b；native readonly 6e8ea1582d576f961a61e6b9a16c2f4242d91cd8be8c9843854f6412528d5718；IAM 4d1c586b68f3e9482dd9d6cf550ac0ccfd764e6bcdc69c87c8b0601432882ab5；active gateway 648888f99ed5a3fa95026182d6992869ac8b783e6c691e912d5feb7f0d157d75；sealed gateway 6cbf354b0f6e12adf155dd50110443fdffc8e9d1a76b521a51f9eaa2dcc5b1ad。
必须保留 dpp-lifecycle307-online/fixture/control-hold.csv@ca3b808e-734b-4d5e-8f2d-c78a16056ed6；copy a01751a1-9311-4357-8fb2-c833de6b8fc4；DV e6b6854f-b54a-462f-875d-9f74542b1fc3；SHA256 961b213e4d82865370d8da052e57af716297ddff615298ed8c43e938f39b1b7d；bytes 49；hold=True；retention=NONE；retain-until=None。
必须保留 dpp-lifecycle307-online/fixture/h0.csv@369808db-a39a-4327-b9ed-5eb83aef7477；copy b3c4e113-f5c5-4421-a9d6-08403b1b9d0a；DV 8d59d129-662b-4dae-a389-894943c716df；SHA256 e35bd4635f1d7ab38c85a290a86a94275085b49bdae8e1a952712d5304816d8e；bytes 44；hold=False；retention=NONE；retain-until=None。
必须保留 dpp-lifecycle307-online/fixture/h1-a.csv@28d9f84a-1889-498d-b033-df26cbf209f2；copy 0d8b77a8-195d-4a83-bd52-04f800d7dc68；DV 6d7d6978-cbbf-4c78-b9d1-82fad7271f0a；SHA256 838fddef805d918cd692a88f720d23705e6763eb0748424274c22d9bcfbc7ba0；bytes 44；hold=False；retention=NONE；retain-until=None。
必须保留 dpp-lifecycle307-quarantine/fixture/h1-b.csv@bae40a85-7539-4fe1-86b7-2cbb17df7812；copy e735d90a-8352-4a01-ad16-8573297bc684；DV 6d7d6978-cbbf-4c78-b9d1-82fad7271f0a；SHA256 838fddef805d918cd692a88f720d23705e6763eb0748424274c22d9bcfbc7ba0；bytes 44；hold=False；retention=NONE；retain-until=None。
仅删除 H2 dpp-lifecycle307-online/fixture/h2-a.csv@453c6f42-fbb2-4fdf-9f86-00a20a54b7bc；copy c05307db-cb81-4220-b42e-be23f85821e3；DV 18721e15-7b01-47d3-a727-8b836d73dbcb；SHA256 b55663df28625841abe3cb9f14cf085c194e00f0a9bcefc53c1b7db83303a838；bytes 44；hold=False；retention=NONE；retain-until=None；plan d78a9339-0968-45c9-bab6-bda86810ec33；hash 205cff03fcbaf25ff4b5d3ad690e5261a8a45e626180aa0782f22e4e249bbffd。
必须保留 dpp-lifecycle307-quarantine/fixture/h2-b.csv@448112b2-baf3-4569-b06f-59011492cf80；copy a37c347d-dbff-492e-9c7a-317a1f188184；DV 18721e15-7b01-47d3-a727-8b836d73dbcb；SHA256 b55663df28625841abe3cb9f14cf085c194e00f0a9bcefc53c1b7db83303a838；bytes 44；hold=False；retention=NONE；retain-until=None。
case owner late=ea209fc734d02abcda12ccd95295a755f5a7c8ed571ca2666350fbf5908755c6；profile SHA256 6325502d144608989bbf61dc0098d74cd76a966fe2dc34f60d5f6dee003e44d2。
case owner close-H1=a67282aae29960324a70182914626ba7a48e75d777084dd856b5f1615370bd70；profile SHA256 c726e3e2029a34287258a3cf386fe954d628195b38844ae4a1831fd82e6b9550。
operator owner wire=7fca52a40ce1763e1576952e7d43adb21c07289f0979a22fb48f287a1bc5bda2；config SHA256 54760a647cb1168a988472a6c7d49baffbc74490ed01842798f07d7d1ec79396。
operator owner H2=0891f293ef97e9166e15ede5401c39c407d9b83c60d635dec8d9948c3b53e4bb；config SHA256 80ec3e6c1a3ccd14f122ebfe78e7a75a55937931b59a8b4e75d903edfe3b3b81。
operator owner recover-H2=453913cfc6f0fa64666d0d5b088257820784b0a01efbe7aa9290b6df86892ae1；config SHA256 2ce6dbd356e74ddd1b079f7b89764df8eb0523103705e236e80afe822a3be687。
环境 dpp-lifecycle307-dfda43b4；workspace 91f84817-9f5a-4654-a793-92e51f1fb7a6；provider 01845fb6-7cc2-4010-8f2a-316576391e67；incarnation 2b760525-880f-468a-a027-a6851cd06487；manifest 7fd71687b4ed7151a3c37a789c6fd5beebea2bd1d61c452c0051cc7a1a08e972。
Core cb50e06b7cd575e7da04b5ff64498ec61a430607f270d4bf3102c1372dca2a82；native 66c17878149ff1959339bc60e319f3b0494786e62da33b77017bfc8aab13613d；gateway 5ce094c5beac39c734225169cef76f1adb41b598b1993d203bd2fa71a21f83c3；network 77f72e9ef0781527a7e8e5ce4a6cd8d36018e527a9ea853ae72d5f8fff9ea0eb。
Core dump 303ef33e8184f09a3de4d91eda8abb3043578c6de30cb0acde57a5922e15e343；globals cc68a7d8f23cf18c9754719b404a4a86a35d83c91d87b49bb05ff92d28ddd2f4；native archive e404b58c3e700716497a8ce2b4b8ab2ce6be180923e814a2e5bfa1ce9b7e4940；restore Core 67cbb08fce688d3e3ed446065eceae350568e04a5b59a367a974a4301a77ac8f；restore native 082d4921b7a430f0c7f667b528fd5f8d1b049a51c24f98931315345e6bff0ed3。
Core readonly 2c670718760a3a7306a9fdba4946e99d9fb024f7258235f4c7f6d880064afd55；native readonly 5d18b8fc4f9ea9dc2404f02450e8715855d539b38932e947e145fc2e3ecd7410；IAM 8fe154cee297199dd954fb7b3d98bdab339ca99ad3930bf95efe9bbe668c7df9；active gateway b32748ec83e80d1475b6b1f3f2ae54bbd0629d155acf80e8ce1225a2315a889f；sealed gateway 6cbf354b0f6e12adf155dd50110443fdffc8e9d1a76b521a51f9eaa2dcc5b1ad。
必须保留 dpp-lifecycle307-online/fixture/control-hold.csv@a59df17d-70d7-40c5-88e8-8cfe77a915dd；copy f3167994-3790-4a6c-918e-ea6d95248b79；DV 57e8c123-6c69-44bc-98ba-d5db8c2b7aac；SHA256 367afd5e5ed47c5f2cadab63af56e0a5e82a1a32afa88f0c3f99448aab883821；bytes 45；hold=True；retention=NONE；retain-until=None。
仅删除 M1A dpp-lifecycle307-online/fixture/m1-a.csv@e8a8418e-c515-464e-99e5-10bc4c64fa8d；copy 5e712426-2109-4f0b-8791-df0a2be3a9f7；DV 44727cb2-5f3b-4b48-b891-931583488ea9；SHA256 e0f5abfc590c27d093d9250148f03bf4b169d262cc61d23af05f3e285c1de725；bytes 40；hold=False；retention=NONE；retain-until=None；plan c2596491-a620-4cda-83d1-4697c64e27f7；hash 8e4380ab68e62e934dae7609b2b4b1711e8c66d0fa2a5a742d0ce6cf059f3a75。
仅删除 M1B dpp-lifecycle307-quarantine/fixture/m1-b.csv@54900849-d59d-459e-90bb-275a5975d0a6；copy ad08947a-4c4d-4b0f-8c7e-2d89f287819f；DV 44727cb2-5f3b-4b48-b891-931583488ea9；SHA256 e0f5abfc590c27d093d9250148f03bf4b169d262cc61d23af05f3e285c1de725；bytes 40；hold=False；retention=NONE；retain-until=None；plan c2596491-a620-4cda-83d1-4697c64e27f7；hash 8e4380ab68e62e934dae7609b2b4b1711e8c66d0fa2a5a742d0ce6cf059f3a75。
必须保留 dpp-lifecycle307-quarantine/fixture/m1-c.csv@81a8402e-390f-4247-9d34-552df5eadeb3；copy 13fd8ecf-07fd-4981-b553-5ce66b67444f；DV 44727cb2-5f3b-4b48-b891-931583488ea9；SHA256 e0f5abfc590c27d093d9250148f03bf4b169d262cc61d23af05f3e285c1de725；bytes 40；hold=False；retention=GOVERNANCE；retain-until=2026-10-23T06:33:33Z。
case owner mixed=7f636ed273048300ff3950efb8ed91623f1da3cae3dd4589fcdfa20c87ebdbcf；profile SHA256 635aa3b751c884d68d3844297d13e183560a4112b0439125e9800ec0276c6308。
case owner mixed-b=6640d650a84734f873b56dea9959996ac5a2ba33e16ca36bb43190db2594e377；profile SHA256 635aa3b751c884d68d3844297d13e183560a4112b0439125e9800ec0276c6308。
case owner mixed-c=307c12b5480d3574fff067a4b6d9131a0ad3acf0cc98bf206e08414f0700b704；profile SHA256 635aa3b751c884d68d3844297d13e183560a4112b0439125e9800ec0276c6308。
operator owner wire=1d8d27cfffdb79fa4a0eee9474f7ecda6315b4673681915a419f618bee5df818；config SHA256 85dd6a5e07acb7e6ba8dca259f6830377b611d0e08fb671fed533a9219591c07。
operator owner M1A=b53d074da77c5eb9eb9a027bbdd1d3091edfbc9f4c0de34580829b3482ddf98b；config SHA256 1a17abb227fbfd1cb7a671466e68d971543bcd8214a217887ff927e7c2c6ab99。
operator owner recover-M1A=f245e90faa3b9d6034120e921f350b42e370998560060178c639950d521172ba；config SHA256 eb37eb4ce9cb57ed27903b0c641f51c1a5d57c5125407d3a2957b2f788781efc。
operator owner M1B=563ab503df6beead199b9d6f224c8e6a965ba6f728e77e1d80e96b1f7b1c1436；config SHA256 1a17abb227fbfd1cb7a671466e68d971543bcd8214a217887ff927e7c2c6ab99。
operator owner recover-M1B=a3d3878263312fd8a94665bbc17dbf7973f071e4a9319559a5e18a3caf1bb2c4；config SHA256 eb37eb4ce9cb57ed27903b0c641f51c1a5d57c5125407d3a2957b2f788781efc。
bootstrap business-races/check-disabled owner 7ff8c0cc8db585bca84e19b7fca1b613cb61387bc3b33f5717d0608d842aa733；root-only profile SHA256 c2c6e725cae1e486893a7a6e1dc4a8f007a0bdb2c25dee49a89684ae013e2fe9。
bootstrap business-races/enable-read owner d875e57994beea8b3a153a70f88bd503124988a6220b1a8d15054c46e35a2f35；root-only profile SHA256 c2c6e725cae1e486893a7a6e1dc4a8f007a0bdb2c25dee49a89684ae013e2fe9。
bootstrap business-races/enable-send owner 136b9d24001b77883cb4dba5e1511970ba104ae1b27e008c528dd0ee6ff2e124；root-only profile SHA256 c2c6e725cae1e486893a7a6e1dc4a8f007a0bdb2c25dee49a89684ae013e2fe9。
bootstrap business-races/disable-send owner 7e21c7605beab20ebfad35b5970ebd7fc8e738b66dd665d5eef6f195eac07123；root-only profile SHA256 c2c6e725cae1e486893a7a6e1dc4a8f007a0bdb2c25dee49a89684ae013e2fe9。
bootstrap business-races/disable-all owner 07e2dde16a5e197773f431520d864b34c90fa1779c1e5b3c0958853de46a882c；root-only profile SHA256 c2c6e725cae1e486893a7a6e1dc4a8f007a0bdb2c25dee49a89684ae013e2fe9。
bootstrap late-hold/check-disabled owner 97662443d079b9144c1adf385baf71a54a163ad2e8fc607c225ae581c8cff613；root-only profile SHA256 97dbb0fc94250e6e6f940aca01a5aaa1dcb9ddae448f6b485f7d93c5976f020c。
bootstrap late-hold/enable-read owner 933e289b320dde7e7fc37297fa3a521a35475fc986ce79120fa0fd59949da9dc；root-only profile SHA256 97dbb0fc94250e6e6f940aca01a5aaa1dcb9ddae448f6b485f7d93c5976f020c。
bootstrap late-hold/enable-send owner cd98ac6fa1bd07eaed2c8079dae9f9580dd549cbef35b76527c1b9ad3b473262；root-only profile SHA256 97dbb0fc94250e6e6f940aca01a5aaa1dcb9ddae448f6b485f7d93c5976f020c。
bootstrap late-hold/disable-send owner 7f7141b8712bb27a56f148cb07c56209a313c61f0c4eaa0f5f5e5aee482a2437；root-only profile SHA256 97dbb0fc94250e6e6f940aca01a5aaa1dcb9ddae448f6b485f7d93c5976f020c。
bootstrap late-hold/disable-all owner a8666ab206d2901c58b3f5d6b0b7875fc7909074b5b619e715eb2079c92e1bf6；root-only profile SHA256 97dbb0fc94250e6e6f940aca01a5aaa1dcb9ddae448f6b485f7d93c5976f020c。
bootstrap mixed/check-disabled owner 9c51575877ad7335014040b306b2204c5c41e7b59aa725ad2152d4482df6a576；root-only profile SHA256 98950119a9c7e1318fbd23e34d3ea64f35f98038ab70ae5474a3d2083a22dec7。
bootstrap mixed/enable-read owner 9262ab6009c42195ce9367ebb7f2125ef60489b8b3f851f1643bdcd872ebc192；root-only profile SHA256 98950119a9c7e1318fbd23e34d3ea64f35f98038ab70ae5474a3d2083a22dec7。
bootstrap mixed/enable-send owner 7746e87ae9dc03cc750f5cd65e4aa2a44c99455df7367a67b76da78a77e411fb；root-only profile SHA256 98950119a9c7e1318fbd23e34d3ea64f35f98038ab70ae5474a3d2083a22dec7。
bootstrap mixed/disable-send owner 31be64275fbe1e358a4e44027ed0a0082a6e439b58a985df928ad0f582d8ce49；root-only profile SHA256 98950119a9c7e1318fbd23e34d3ea64f35f98038ab70ae5474a3d2083a22dec7。
bootstrap mixed/disable-all owner 2928c05a09955234a3f2ddf4a2380da8ab048ab47085bd4e478f509668e68f7b；root-only profile SHA256 98950119a9c7e1318fbd23e34d3ea64f35f98038ab70ae5474a3d2083a22dec7。
受限 bootstrap Linux binary 68ad4a761a105e7d5ad2a678546f80347222b6fab11c0b4b64b49f15989f9d42；仅 MinIO admin 精确用户状态/策略读取，禁止S3 DELETE/COPY/hold API，root不挂worker。
同意仅在全部执行前条件通过后，七项各最多一次 exact DELETE、全局最多七次；UNKNOWN 不重发。
窗口开始后、任何业务 I/O 前，以新 execution 和冻结 expected heads 执行显式 Renew，UseUntil/RestoreUntil≤min(当前时刻+1小时,固定窗口结束)；保留旧policy及所有历史期限。
P1/P2/T1/T2 先走真实业务与owner闭合；H0/H1均0发送且H1只在可信owner终止、identity关闭及gateway封存的签名证明后记NOT_SENT；H2只允许A一次并制造真实204后的响应丢失，late hold不能撤销在途删除，B不得dispatch；M1先A exact absence，再B CONFIRMED UNKNOWN与C-only保护阻断mixed检查点，之后才允许B只读lookup，C永不删除。
保留全部十一份survivor、其他环境、所有备份及失败证据；cleanup=0、hold-release=0、bypass=0；#323保持Draft、不合并；#307保持OPEN。
旧授权全部不复用。本确认仅绑定上述候选、execution、精确版本和绝对窗口；缺少新确认或任何绑定/保护/前置漂移即停止。
```
