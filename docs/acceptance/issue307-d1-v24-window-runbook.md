> 历史准备稿：c14/v24已在独立副本试跑失败并封存，未冻结、未授权；本文件不得作为当前执行入口。当前准备为[v25手册](issue307-d1-v25-window-runbook.md)，旧源码仅由d9bf Git解释。

# #307 D1 candidate14/v24 窗口手册

当前为准备稿：disabled bootstrap已完成本地、同镜像原生及独立评审；本代原生备份恢复和候选冻结尚未完成，不是执行指令或删除授权。人类选择的拟窗口为2026-10-20新加坡14:00–14:40，即UTC06:00≤now<06:40；只读恢复最晚UTC08:40。唯一拟execution为`afc0e66c-535b-426e-a937-964d0c9ca0d6`。候选冻结、实际完整入口和独立结果评审通过后，必须另取得本代精确授权。没有新调度，不复用任何旧授权。#332保持Draft、不合并，#307保持OPEN；#323已按另一次授权合并，不回写历史。

## 唯一范围与入口

六目标严格按P2→T1→T2→H2A→M1A→M1B。每份最多一次、全局最多六次，UNKNOWN不重发。Core的18历史copy、17现存版本、六目标和十一survivor分别核对；P1已确认缺失，只作历史前序事实，绝不进入新目标或预算。exact环境、namespace/key/version、copy/DV、plan/hash、保护状态和所有冻结绑定以本代最终完整确认文本及`remaining-candidate14` packet为准；冻结前不得从相似key或旧模板猜测范围。

本代只使用以下当前入口：

- `prepare-six-candidate.py allocate`：一次性分配全新停止资源，禁止复用已有同名卷/网络。
- `six_candidate_runtime.py materialize/initialize-budgets/check/run-non-delete`：本代全新资源的单次准备入口。每阶段O_EXCL，三新预算各只运行一次固定initializer；完整case/topology/budget门禁早于Source。原c13失败入口绝不重跑，旧CP01/02仅解释其历史字节；本代无旧source fallback。
- `freeze-six-candidate.py`：仅在三组实际非删除终态PASS后冻结，无授权或启用功能。
- `verify-six-native.py`：冻结后及未来窗口内使用的完整只读入口，精确传入candidate/bindings/checker SHA和新的证据输出目录。case/recovery跨字段语义先于permission及Docker I/O。
- `window_six_support.py check/archive-root/revoke-case/h1-proof/seal`：本代固定宿主维护入口。复用原review06的归档及逐步封存原语，当前范围收窄为六份、十五个已知IAM身份和真实三挂载case。
- `attest-remaining-executable.py`：实际拓扑签名，host私钥不挂worker，独立wire私钥只挂wire。

原七目标生成器、完整预检、旧permission和旧维护入口只解释其历史Git/二进制，不能用于本代启用或发送。准备源码CI、模拟组合、隔离PG、原源只读、独立restore和人类授权分别留证，不能互相代替。

## 冻结与未来授权前

1. 新资源须有execution及独立allocation nonce；15个新卷的local driver/options、实际完整CID挂载图、独立restore网络和source/restore不别名全部通过。唯一旧卷例外是固定原native卷在archive/compare中的`/source`只读挂载。所有原环境、旧预算、旧pins、全部备份和失败保留。
2. 新预算仅由固定无网络、仅CHOWN能力的initializer初始化为UID/GID10001，三个新预算实际为空。不能删除、重建、退款旧预算，不能用复制的新预算规避旧失败。
3. 三源Core全关系/roles/ACL前后不变。每组新Core dump/globals/native archive须恢复到独立clean checkpoint；另一个独立rehearsal Core用于业务重跑，不把原源或clean checkpoint当业务试跑库。新用户以官方SetUser(AccountDisabled)创建，随后由固定单成员ImportIAMV2导入单user→policy映射，四类原始结果及fresh disabled/sole-policy/no-groups/官方Equals全部通过；SDK MaxRetry=1，mutation guard独立阻止重放/UNKNOWN。源的旧/新发送身份始终关闭。仅restore上限时开启新受限inspector，结束后固定disable并新鲜读回；失败为UNCONFIRMED。
4. 最终packet、source archive、raw/LF source-freeze、三个Linux binary、Windows file-only checker、Core/IAM/gateway/owners/mounts/networks/budget、九新备份及九旧备份、完整入口/维护入口/独立报告均须精确绑定。embedded vcs.revision不作声明；实际Git source、build recipe及binary SHA单独记录。
5. 本代完整只读入口实际PASS、独立结果评审无有效阻断后才提供完整确认文本。30份operator/root permission保持false，不启动case owner，所有owner/source/gateway/Redis停止，gateway保持sealed。case profile按固定窗口描述，其冻结Base字段不能临时翻转或重写。packet Git anchor只供已知CID封存，不授予发送。
6. 将来实际授权原文、批准机器模板、reference manifest和envelope分别留证；按引用批准须user_retyped_template=false，不能伪称人类重新键入模板。准备status的历史NOT_RECEIVED保持不变，后来的新授权另存。未来定时续跑须另按人类要求建立，本手册不创建调度。

## 将来已授权窗口内的前置

先读本聊天最新真实人类消息，撤销/收窄优先。早醒只读/离线检查并保持发送关闭；晚醒或过窗封存，不延长、平移、重开或补发。全部新鲜前置通过才允许后续业务，任一非预期失败立即封存，不修补冻结字节续跑。

1. 实时核实际HEAD、#332 Draft/#307 OPEN、该精确HEAD全部必要PR CI及评审线程；不能用旧HEAD绿灯，也不能把PR CI称为main CI。核最终批准的完整reference清单、source/binary/candidate/template/bindings/profile-windows及宿主入口的实际字节。
2. 在新O_EXCL private/output中运行`verify-six-native.py`。先核六case的Batch、统一START/END、CandidateUseUntil=END及六recovery=END+2h，再按各自冻结descriptor核全部owner的image/command、完整mount/netns、noports/no-socket/no-restart及卷归属。case/operator/root bootstrap/read-helper等受限worker须UID10001:10001、两固定tmpfs、cap-drop ALL及no-new-privileges；case/operator/recovery不得挂provider root凭据，root bootstrap与它们物理分离。预算initializer固定UID0:0、无网络、仅CHOWN及其唯一新budget挂载；PG/native/Redis/Caddy按各自固定service角色核对，不能误套worker UID/tmpfs。再核30份false及三个新空预算的归属/挂载全集。任何类型错误、卷扩张或非空预算均停止，不重建资源。
3. 同一入口重查17份完整bytes/SHA/version、全namespace inventory、hold/retention及P1真实NoSuchVersion；完整十五已知IAM身份disabled；十四份sole policy/no groups及官方ParseConfig/Equals；唯一已证明c13 partial i4e524c04808a6960为严格NONE/no-groups，其unassigned canned policy单独Equals，五个未创建c13用户名须fresh NoSuchUser。metadata IAM按exact key，固定SDK/gateway强制exact version；不能把raw policy SHA或2xx-only当语义证明。保存全部技术GET/HEAD的start/observation/真实HTTP及成本，金额未知为NOT_ASSESSED。
4. 同一入口重查三源Core REPEATABLE READ READ ONLY全关系/roles/ACL前后不变、九新/九旧备份、source/restore CID分离。入口结束自动关闭其source/helper。任何重新启动Core/native之前，运行`window_six_support.py check`，要求实际新授权闭合及全部绑定资源仍stopped；不能先启动再做要求stopped的check。
5. 只有窗口内才启动冻结的三源Core/native，并复核状态。全局六slot及恢复期间三个Core保持可读，下一slot的proof直接读取实际immutable absence，不读host进度projection。无Renew、无新RightsDeclaration、无重新生产/传输、无P1重跑；五个business policy heads只读核对。
6. root O_EXCL result/status-changes已存在时，先archive-root保存原字节、SHA及journal；归档失败禁止启用。只将批准模板原字节给选中固定owner，permission仅改Enabled/真实ConfirmedAt，其他SHA、窗口、max1/max6保持不变；未选owner继续false。安装冻结active gateway并核实际mount SHA，固定enable-read只开启本代受限inspector，业务/发送/恢复worker不挂root凭据。

## 六个一次性slot与只读恢复

每个sender/wire/recovery前必须各自取得≤1分钟新鲜实际host topology；wire context最多3分钟，运行中每target刷新wire proof。SDK无重试，持久target/wire预算先于Core claim。严格每份send→禁用发送→选定Outbox事件经既有Asynq只读恢复→提交Core absence→下一份；不能沿用旧四send后四recover的批次。未关闭前序发送或未提交前序absence时，下一个slot不得消费预算。

- business resume复用已完成Processing/DirectData及可信owner/handles/pin/ref闭合，只新增本execution的fixture basis/批准/dispatch；旧已撤销批准及旧attempt不复活。P2、T1、T2逐份完成上述闭环。
- H0/H1零发送。case-late成功exit0/PID0后，revoke-case仅撤销Base.Enabled并私有归档原profile；该owner永不重开。全部late operator/root false、enablers停止、固定disable-all、≤45秒fresh check-disabled、gateway实际停止且sealed后才h1-proof签一次。随后≤1分钟固定close-H1在sealed gateway追加NOT_SENT；不能手填布尔值、用旧读回或自动重签。后续限定inspector仅在固定窗口内受控开启，H1 case继续撤销。
- H2只允许A一次，真实上游204之后才witness、late Core holds A+B、approval revoke及响应丢失，形成CONFIRMED UNKNOWN；B永不dispatch。预期UNKNOWN沿固定只读链处理，绝不重新发送。
- M1A先一次发送并提交absence。M1B一次发送形成CONFIRMED UNKNOWN及C-only保护阻断mixed checkpoint；原checkpoint复制到recover-M1B及case-mixed-c并核SHA之前禁止B lookup或全namespace查询，之后才B只读lookup。C永不删除。
- 六份恢复均只经冻结recover-*、选定Outbox→Asynq及独立只读profile，最晚2026-10-20T08:40Z。不能重新启用身份或延长发送窗口；IAM关闭后无法lookup即保留UNKNOWN/NOT_RUN。06:40Z起禁止新增wire/sender/enable，不抢窗使用剩余预算。

## 失败、完成或过窗封存

使用本代`window_six_support.py seal`：先关闭全部30份permission及case/recovery，停止sender/wire/enable owners，各组只运行固定disable-all，再停/封gateway，并继续关闭全部绑定source/Core、恢复Redis及原business Redis至PID0。inspect失败仍尝试关闭已知完整CID；一项失败继续其他组。seal不启动source/enable/sender/wire，不cleanup/hold-release/bypass。归档失败仍尝试固定disable-all，但归档不完整、enabler未闭合、无fresh identity读回或未知owner不能记整轮PASS，保留UNCONFIRMED。

原失败/UNKNOWN/真实provider attempts/成本/旧pins/新旧预算及备份全部保留；不resend、不release、不退款、不覆盖旧事实。终态和原始字节SHA索引另存；若该execution已有terminal，只读报告，不重复发起。未运行项目明确NOT_RUN，完整D1不提前验收。若将来另获调度授权，完成、失败或过期后暂停同一调度，不创建重复任务。
