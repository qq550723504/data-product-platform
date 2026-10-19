# #307 D1 candidate10/v20 待确认窗口手册

当前为 **PREPARED_NOT_AUTHORIZED**；以下是将来获得新确认后的有限顺序，不是现在的执行指令。#323 Draft、#307 OPEN；旧heartbeat保持PAUSED，无新调度。本轮真实DELETE=0、hold-release=0、bypass=0、cleanup=0。

唯一新execution为 `67de3acd-3a9d-4ece-bebb-cd53d0ab3ac5`，拟窗口为 **2026-10-18 10:00≤UTC<10:40（新加坡18:00–18:40）**。7目标逐份最多一次、总计最多7次，UNKNOWN不重发。11份保留版本、所有其他环境、全部备份和失败证据保留；不能延长、平移、重开窗口。

权威库存、完整机器载荷、所有SHA/CID/plan/expected-head见 [v20精确方案](issue307-d1-candidate10-confirmation.md) 和 `remaining-candidate10/{candidate,bindings,confirmation-template}.json/txt`（实际文件分别为candidate.json、bindings.json、confirmation-template.txt）。当前packet Git anchor为 `5eb3aec724f55e97ee05595439f74781d0e4b989`；它仅供已知owner封存定位，不授予启用或发送权限。

## 当前工具与准备回执

- 完整只读入口：`examples/lifecycle-d1/verify-remaining-native.py`，SHA `0e0a2fd22376135cefb1b22505497f2b6e7576af43daf1562c921e2a488521f7`。冻结准备和未来fresh使用同一sealed_gate/collect/aggregation/close组合，不得再临时添加raw policy SHA或2xx-only门禁。
- 当前host维护工具：`docs/acceptance/evidence/issue307-d1/remaining-window-review04/composition/window_support.py`，SHA `58494ed65aa033fe739bdd3a4d042ef132f0ffa18a6c827709ae59b4b2b4951d`；tests SHA `5028fb2060051bb8da3081713b468cf562ff6df3a897cca4541732da3e75c8d4`，normal/-O各14通过。07的review01、08的review02和09的review03均不是10入口。
- 当前拓扑签名器仍为提交中的 `examples/lifecycle-d1/attest-remaining-executable.py`，仅10/new budget；host私钥不挂worker，wire独立私钥只挂wire，fresh topology≤1分钟。
- 三组原生准备18版本、9备份、disabled IAM/政策、Core前后不变通过；另一次完整CLI实际PASS见 `remaining-preflight-native03/complete.json`。今日回执不代替未来窗口新鲜核验。
- Source71b17119、candidate、template、bindings、全部binary及host检查器和维护工具必须保持精确绑定。当前HEAD required PR CI须运行时实时核对，不能使用旧HEAD绿色结果，PR CI不是main CI。

## 新授权与开窗前条件

1. 先读本线程最新人类消息，撤销/收窄优先。尚无新确认时，只能保持32份permission=false、所有owner/source/gateway/Redis停止和gateway sealed；不能materialize human文本、签发送proof、启动case/enable/wire/sender。
2. 真正新授权到达后，实际原文、批准的冻结机器载荷、envelope分别留在私有 `remaining-candidate10/authorized-execution01`；公开脱敏绑定将写入 `delete-confirmed-v20-execution01/human-authorization.json`。当前这些授权记录尚不存在。按引用批准应记录user_retyped_template=false，不能伪称用户重新键入模板。旧07授权及08/09未授权快照全部不复用。
3. 新授权须同时绑定本精确方案中的机器模板、source/binary/Core/IAM/gateway/预算/owner、完整入口、review04工具与最终独立报告、execution、7目标/11保留、次数/UNKNOWN/窗口/保留限制。未来调度须单独按人类后续要求建立；本准备没有创建调度或重启旧调度。
4. 开窗前只能check与离线验证；不得因手册、packet anchor、CI或历史status获得发送权限。原始status.json保持NOT_RECEIVED历史快照，新授权另存。

## 窗口内完整新鲜核验

全部前置通过才允许后续业务；任何失败立即转封存，不能修改门禁续跑。

1. 实时核对实际HEAD、#323 Draft/#307 OPEN、该精确HEAD全部必要CI、824运行源码/source-freeze与candidate/template/bindings/三个Linux binary及host checker/entry/support/report SHA。离线审计用 `check-remaining-candidate10.py`。
2. 用完整只读CLI、独立新private/output目录重做18版本bytes/SHA/full inventory/hold/retention、disabled identities完整单policy/no groups/MinIO语义、Core REPEATABLE READ READ ONLY指纹与roles/ACL、前后不变。CLI需要精确candidate SHA、bindings SHA、checker SHA与reader路径；路径由入口统一归一化。不得重跑freeze/prepare入口。保留全部GET/HEAD start/observation及真实HTTP状态；技术root配置不挂worker。该入口结束时关闭其精确source/技术helper，保留全部资源。
3. 核对9份保留备份SHA、恢复Core/native CID与本次源分离。再次检查39个10 owner真实CID/image/command/mount/netns、32权限false、cap-drop/no-new-privileges/no ports/no restart/no socket/root credential，3internal network。budget volume ownership=新execution，挂载CID集合恰为该组Helpers，首次发送前持久预算为空；不能删除或重建budget。 完整CLI已经关闭资源后、任何重新启动Core/native之前，运行review04维护工具check；它要求新授权证据全部匹配且所有绑定资源仍stopped。
4. 后续业务前仅在已授权窗口内重新启动绑定Core/native，重新核对Core及本组必要native状态。此时不得再运行要求所有资源stopped的维护check；该check已在上一步完成。源开启的时间不是发送权限。root check-disabled已有O_EXCL输出，必须先archive-root保留原字节、SHA和journal再运行固定CID，exit0/PID0/真实时间才算fresh，旧或不可读输出不能采用。
5. 在本窗口内才把已批准template原字节materialize给所选固定owner，permission只更新Enabled/实际ConfirmedAt，其他所有SHA、窗口、max1/max7不变。未选owner继续false。安装冻结active gateway并核对实际mount SHA；固定enable-read先检查policy再只开启限定reader/producer/inspector，root不挂业务/发送/恢复worker。
6. 固定renew case使用新execution和冻结六expected heads追加Renew；UseUntil/RestoreUntil≤min(now+1h,10:40Z)。保留旧policy和所有历史期限。旧HEAD不能重新签发，任一漂移即封存。

## 业务、NOT_SENT、七个一次性发送与恢复

- P1/P2/T1/T2先运行固定business case，核对真实处理/DirectData owner、handle、pin/ref闭合，retry/replay/取消无新增输出；新鲜gate及受限IAM均通过后才选择各一个sender。
- 每次wire、sender、recovery前用固定attester签各自≤1分钟的实际topology；wire运行中每target也刷新wire proof。只有固定CID与冻结sender/wire能发送，SDK无自动重试，持久target budget先于Core claim。wire context3分钟；business可四send后关闭send再四recover，不能夹长恢复后假设wire仍有效。
- H0/H1均0发送。case-late成功exit0/PID0后，用review04 revoke-case只撤销其Base.Enabled并私有归档原profile；该owner不再启用。所有late operator/root permission=false，停止enable owners，固定disable-all后归档并跑≤45秒fresh check-disabled；gateway实际停止并sealed，才由h1-proof签一次closure。随即在sealed gateway上固定close-H1须≤1分钟完成Core NOT_SENT；不得填布尔值冒充真实检查或自动重签。后续恢复active只允许窗口内限定inspector，H1 owner保持撤销。
- H2只允许A一次；上游真实204后，wire才生成witness并提交late holds A/B、approval revoke和响应丢失。sender形成CONFIRMED UNKNOWN；B永不dispatch。晚到hold不能撤销在途DELETE，不能把UNKNOWN重发。
- M1先A一次并由recover-M1A确认exact absence；固定mixed-b随后准备B，B一次形成CONFIRMED UNKNOWN，再形成B-UNKNOWN+C-only保护阻断mixed checkpoint。在此之前禁止B lookup或全namespace查询。checkpoint原字节复制至recover-M1B和case-mixed-c并核SHA后，才允许B只读lookup；C永不删除，mixed-c核对C retention和零C attempt。
- 只读恢复使用既有Outbox→Asynq与固定recover-*、独立recovery profile及fresh proof。有限上限为2026-10-18 12:40Z；不能用于重新enable身份或延长发送窗口。若关闭IAM后读恢复无法继续，保留UNKNOWN/NOT_RUN，不额外重开权限。
- 未预期失败立即停止新增发送并封存；预期UNKNOWN只沿固定只读链推进。10:40Z起禁止新增wire/sender/enable，预算未用完保持未使用，不抢窗补发。

## 失败或完成后的固定封存

只在本次已批准的执行生命周期内使用review04 `seal`；该工具不启动source/enable/sender/wire，仅在仍运行的native上尝试固定disable-all，逐项继续所有关闭。窗口前身份本来就关闭，准备阶段关闭由完整只读入口自己的finally完成。

1. 32权限false；7 case与7独立recovery profile撤销，原字节/SHA留证；停止17 operator/wire/recovery、7 case及6 root-enable owners。
2. 各组尽力archive-root并固定disable-all；归档失败也继续关闭，旧/不可读status不能当fresh。enabler未实际关闭或无fresh disabled读回必须unconfirmed。policy后验失败与identity读回分别记，不包装成整轮PASS。
3. 当前packet损坏时，仅seal使用同一Git anchor+精确SHA定位已知关闭CID；该anchor不是发送授权。所有组gateway停止、保存原文件并安装冻结sealed文件，继续停止native/Core/恢复Redis/原business Redis/root helper。逐CID inspect/PID0/no restart，任一失败继续其余关闭。
4. 保留原失败、UNKNOWN、provider physical attempts/成本、预算、pin、全部备份和helper/volume。没有确认就不能写NOT_SENT/absence；不release旧pins、不hold-release、不bypass、不cleanup。
5. 输出逐项封存、实际完成量、NOT_RUN与未确认项，索引原始字节。若将来有新调度，完成/失败/过期后暂停同一个调度，不重复创建或续窗。没有新授权的本准备阶段保持无调度、无发送。
