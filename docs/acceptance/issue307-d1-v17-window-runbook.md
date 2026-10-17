# #307 D1 candidate07/v17 固定窗口运行手册

**2026-10-17实际结果：本次窗口在02:04:12Z发送前终止，已物理封存，heartbeat PAUSED。** [窗口回执](evidence/issue307-d1/delete-confirmed-v17-window01/window-result.json)记录：business原生readonly PASS，宿主新增raw-policy-SHA汇总错误地拒绝3份仅列表顺序变化的等价策略；0真实DELETE/claim/budget，59容器停止、32权限false。business四身份fresh关闭确认，另两组未启动且无新IAM读回，不能称全部步骤fresh确认。以下原顺序保留为本次授权/操作历史，不能用来重新开启本窗口。

当前 host 执行入口为 `remaining-window-review01/composition/window_support.py`。0ef2fb92 的 `delete-confirmed-v17-execution01` 准备包、11份raw索引及当时手册SHA均保留为历史；旧composition不再作为执行入口。独立评审发现的“当前包损坏阻断封存”和“归档失败跳过IAM关闭”只在host组合中修复，冻结candidate及其源码/binary/授权范围不变。

本手册只组合已冻结的 owner、IAM bootstrap、attester 和 Command。execution 为 `effa0385-9308-42eb-bd66-d0c379b6a989`，唯一发送窗口为 **2026-10-17 02:00:00Z ≤ now < 02:40:00Z**（新加坡10:00–10:40）。七份目标逐份最多一次、总计最多七次；UNKNOWN 不重发。11份保留版本、其他环境、全部备份保留；cleanup=0、hold-release=0、bypass=0。#323 保持 Draft，#307 保持 OPEN。

权威完整绑定仍为 [v17精确文本](issue307-d1-candidate07-confirmation.md)、[candidate](evidence/issue307-d1/remaining-candidate07/candidate.json)、[bindings](evidence/issue307-d1/remaining-candidate07/bindings.json)。本手册不修改这些字节，也不新增目标、窗口或发送实现。

## 1. 授权和定时续跑

- 新的人类直接按引用授权已记录于 [human-authorization.json](evidence/issue307-d1/delete-confirmed-v17-execution01/human-authorization.json)。`user_retyped_template=false`。实际原文、批准的冻结模板及明确标注两者区别的 envelope 分别存放在私有 `remaining-candidate07/authorized-execution01`。旧 authorization 未复用。
- 续跑使用本线程 heartbeat `307-d1-candidate07-v17`，只安排2026-10-17新加坡10:00一次。读取保存的创建时间、主机UTC+08时区和一次性 recurrence 展开核对日期。调度不保证准点；电脑、Codex app 和 Docker 必须可用。迟到不顺延窗口。
- 醒来先读最新人类消息，撤销或收窄优先。早于02:00只读等待；02:40以后不能启动任何 sender、wire 或 enable helper。未开始即过期则维持封存，记录 NOT_RUN 并停止本 heartbeat。失败、完成后也暂停同一 heartbeat，不能新建重复调度。
- 不能因为这份手册、历史 `status.json` 或定时器本身就打开权限。授权来源是当前人类消息；原 `remaining-candidate07/status.json` 是冻结前的历史快照，保持原样。

## 2. 窗口前检查与运行工具

工作目录：`C:\Users\Henry\.codex\worktrees\9400\data-product-platform`。

私有根：`C:\Users\Henry\.codex\fixture-private\remaining-candidate07`；新增运行证据写入 `authorized-execution01` 及本手册对应的 public execution 目录，不能覆盖冻结回执。

```powershell
python -B -X utf8 examples/lifecycle-d1/check-remaining-candidate07-freeze.py
python -B -X utf8 docs/acceptance/evidence/issue307-d1/remaining-window-review01/composition/window_support.py check
python -B -X utf8 docs/acceptance/evidence/issue307-d1/remaining-window-review01/composition/test_window_support.py
```

`check` 仅文件读取和 Docker inspect，不启动容器、不发 provider 请求、不写 worker human 文件、不启用 permission。原生 fresh 核验留到窗口；离线 PASS 不替代该核验。

复用原则：保留现有 Docker/MinIO bootstrap、Core Commands、Outbox→Asynq 和 cryptography Ed25519；新 host support 仅补齐一次性回执归档、实际配置撤销、H1可信签名和多步骤失败封存。未引入并行工作流、IAM 或发送基础设施。所有 mutations 模式在窗口开始前拒绝。`seal` 可以在窗口结束后继续关闭权限，但不能启用或发送。

## 3. 窗口内，发送前必须全部通过

1. **身份/源码/CI**：复核实际HEAD、#323仍Draft及该HEAD CI。上次已通过的 HEAD `37094e56c2a02d46ec3f02e4f6fa1e4b3457eb1f` / PR CI `38882381488` 是历史基线；如本次手册提交推进HEAD，应核对新HEAD对应CI。冻结运行源码仍为f3b5de45，三个binary不能重建替换；严格freeze checker必须PASS。不得把PR CI称为main CI。
2. **物理资源**：从candidate/bindings读取完整CID，不按模糊name搜索后启动。核对3源Core/native/gateway、3恢复Redis、business原处理Redis `25a4e218f72527e8f93a1df2eeb7a09833e5028c03d5ee178009bd9c71ee33e6`、39个owner、3internal network、无ports、readonly mounts、无root/socket、cap-drop ALL、no-new-privileges、no restart。restore clone和旧5d保持停止。每个budget volume ownership必须为本execution，挂载CID集合恰为本组Helpers；首次发送前预算为空，不能清空或重建budget。
3. **备份**：逐组核对 `remaining-executable03/<group>/checkpoint/{core.dump,globals.sql,native.tar}` 的9个SHA与candidate一致；保留所有原备份、restore容器和volume。不能把新的快照替换批准的检查点。
4. **fresh只读**：先启动绑定Core/native，send权限全部false。沿用 `freeze-remaining-executable.py` 的 GET/HEAD-only `remaining-verify-native` 组合，在新的技术helper目录创建短期只读配置，使用冻结reader。不得重跑freeze/prepare整入口。核对3源Core REPEATABLE READ READ ONLY fingerprint、roles/ACL与冻结 `core-readonly.json` 完全一致；18版本全集、bytes/SHA/hold/retention、无额外版本、完整IAM单policy/noMemberOf/OSS policy equality和所有身份disabled，核验前后Core不变。技术root核验凭据不得挂载worker；请求journal只允许GET/HEAD。
5. **一次性root回执**：冻结 `check-disabled` 已有旧 `result.json`，重新运行前用下述归档模式保留原字节/哈希。运行固定CID必须exit0并保存新的实际时间和journal。不能忽略 O_EXCL 错误把旧回执当fresh。重复使用 `enable-read`、`disable-all` 等同样先归档；不删除journal、不重建helper。

```powershell
python -B -X utf8 docs/acceptance/evidence/issue307-d1/remaining-window-review01/composition/window_support.py archive-root --group late-hold --action check-disabled
```

6. **materialize批准载荷**：验证新授权receipt的实际原文SHA、envelope SHA、candidate/template/bindings/source/binary/digest及窗口、max1/7、survivor11完全一致；在本次已授权窗口内，把已批准的 `approved-referenced-template.txt` 原字节写入所选固定owner的 `human-confirmation.txt`。它是机器核验的批准载荷，不能标注成人类重新键入。permission仅更新 `Enabled` 与真实 `ConfirmedAt`；保留冻结candidate/template/binary SHA、NotBefore/ExpiresAt及预算。未选择的owner继续false。无新的真实人类授权、当前消息已撤销或任一绑定不符均停止，不能自行合成新的批准。
7. **active读路由与enable-read**：安装本组冻结active Caddyfile并核对其实际mount SHA，启动固定gateway/Redis。9000只有受限读/业务PUT路由，无DELETE；9002仅冻结wire可提供。固定root-enable-read先严格检查IAM再启用本组限定reader/producer/inspector并读回。独立root权限不传给case/sender/recovery。六个expected-head policy Renew仅通过固定`renew` case追加，期限≤02:40Z，不能改旧policy。

任何一步失败：不启用send、不新增业务动作，直接执行§6封存。不能用新candidate、临时writer或跳过核验来消耗本授权。

## 4. H0/H1与可信NOT_SENT

1. 固定 `case-late` 运行成功，保留H0/H1/H2-ready输出；核对H0无dispatch，H1 revoked但无send claim/observation/absence，H2只有A READY_UNCLAIMED。H2-B永不dispatch。
2. 实际 `case-late` 必须成功退出、PID0、no restart。执行 `revoke-case --group late-hold --stage late`：私有归档原profile字节，唯一允许的变化为 `Base.Enabled:true→false`；原SHA必须为candidate中的6325502d…，生成撤销证据。其他业务字段、独立 `case-close-H1` profile及candidate均不改。后续不得重新启用case-late。
3. 将late所有operator/root permission设false，停止所有late operator及root-enable owners。固定 `disable-all` 关闭两身份；归档旧check-disabled回执，再运行固定check-disabled，核对exit0和fresh全部disabled/policy PASS。root helper进入native netns，无需gateway放行。
4. 停止gateway/PID0，再把实际挂载文件换为冻结sealed Caddyfile。运行 `h1-proof`；工具检查实际owner/原字节撤销、完整mounts、所有operator/root关闭、45秒内真实成功IAM检查、实际gateway停止且sealed，再用既有host key签固定closure binding/owners。输出 `case-close-H1/closed-H1.json` 一次性创建；不支持伪造布尔状态或自动续签。
5. 立即启动**sealed** gateway，运行固定 `case-close-H1` CID，在proof≤1分钟内提交Core NOT_SENT，保存 `remaining-H1-NOT_SENT.json`；该Command还在Core中检查无claim/observation/absence/release及当前execution revoked事实。若proof过期或Command失败，封存，不自动重签。
6. 再停止gateway，安装冻结active文件、核对SHA并启动。归档前一次enable-read结果，窗口内重新核验/开启仅限定inspector，才准备H2 sender。H1 owner保持配置false；原freeze历史与配置撤销receipt分开保存。

## 5. 七个一次性发送与只读恢复

- **P1/P2/T1/T2**：固定business case成功后，先验证真实owner、read/write handles、pins和typed refs闭合，以及retry/replay/取消后的0新增输出。选择对应四sender；每次fresh gate，持久O_EXCL target budget先于Core claim。可在一次wire存活期间顺序完成四sender，然后关send并执行四recover；wire主context只有3分钟，不能把长时间恢复夹在一次wire存活假设内。
- **每次启动前**：分别给wire、sender及恢复owner运行既有 `attest-remaining-executable.py --group <group> --stage <stage>`，≤1分钟actual Docker topology，host私钥不挂worker。wire活跃期间每个新target前也刷新wire proof，sender/recovery使用各自proof。启动前确认尚在窗口、target预算未消费、无旧claim/UNKNOWN。固定root-enable-send只开本组exact-version删除身份。SDK单次且禁止redirect；不能用curl/mc/其他SDK手动补发。
- **H2**：仅A一次，必须由wire证实上游真实204，之后提交late holds A/B与approval revoke并丢响应；sender预期 `CONFIRMED UNKNOWN`，不是普通失败重试。关闭send及wire后经固定recover-H2查询；A absence与B保留分别留证。若204/hold/revoke链任一不成立，保留实际结果，不能伪称H2 PASS。
- **M1**：固定mixed case→A一次→recover-M1A提交A exact absence→固定mixed-b才准备B→B一次返回 `CONFIRMED UNKNOWN`→sender先完成**仅C保护查询和Core阻断**的 `M1-mixed-checkpoint.json`。检查点形成前禁止B lookup或全namespace库存查询。将该文件原字节分别复制到固定recover-M1B和case-mixed-c private目录，核对SHA；之后才允许recover-M1B lookup。固定mixed-c复核检查点与C retention阻断、0C attempt。C永不发送，不能用B恢复完成去倒推之前mixed状态。
- 恢复只用冻结recover-*、独立recovery.json、fresh readonly topology及实际Outbox→Asynq选择工作，不启动全库扫描器、不resend。有效期上限04:40Z只授权GET/HEAD恢复，不能延长send/enable-read/enable-send或重新签发window proof。若窗口结束后身份已disabled导致恢复不能继续，保留UNKNOWN/NOT_RUN，不额外开启身份。
- 02:40Z起停止新增sender/wire/enable动作；未花完预算保持未使用，不赶窗口补发。窗口内任一非预期结果立即转§6；预期UNKNOWN只沿冻结只读链前进。需要新的wire进程时，先确认旧wire停止、没有未决target，再核验其未消费target集合与fresh authority；绝不通过重启重发已消费budget。

## 6. 失败封存（同样适用于完成后的最终关闭）

```powershell
python -B -X utf8 docs/acceptance/evidence/issue307-d1/remaining-window-review01/composition/window_support.py seal
```

该模式只启动固定`disable-all` root helper，绝不启动native/Core/gateway/sender/wire/enable helper。窗口前拒绝；窗口结束后仍可执行关闭操作。仅seal从本地Git anchor `37094e56c2a02d46ec3f02e4f6fa1e4b3457eb1f` 读取同一candidate/bindings，并强制原SHA，以确定已批准的固定关闭CID；当前candidate/template损坏不能阻止已知owner封存。此快照不授予任何发送权限，check/H1等其他模式仍严格校验当前文件。若本地批准快照自身不可用或SHA不符，不能猜测CID，须报告封存未确认。步骤独立收集结果，一项失败不能跳过其他组：

1. 32份permission设false；7个case Base.Enabled和7份独立recovery Enabled同时撤销，原字节私有保留。每个失败单独记录，不先校验无关策略再决定是否关闭。
2. 按candidate完整CID停止全部17 operator/wire/recovery、7 case和6个root-enable owners；防止并发重新启用。停止发送进程会保留已花费budget和原provider attempt/UNKNOWN，不能据此判NOT_SENT。
3. 若本组原native仍运行且固定disable-all helper的binary/rootconfig/netns/mounts可信，尝试归档其旧O_EXCL回执；归档失败单独记未完成，仍运行同一固定disable-all，每组均尝试，30秒限时。底层Go先disable/readback再验证policy。启动前已存在或不可读的status摘要一律不能当作本次fresh读回；只有本次O_EXCL新建的回执可采信。策略后验失败但fresh `status-changes.json` 全部disabled且unconfirmed空，可分别报告最后读回disabled、policy失败；若任何enable owner仍未实际关闭，稳定identity关闭也必须unconfirmed。归档失败、身份未确认或policy失败均使整轮封存未通过；native已停不能冒充fresh IAM确认。
4. 各组停止gateway、保存原配置字节后安装冻结sealed文件；再停止native/Core、3恢复Redis、business处理Redis和全部root helpers。逐个inspect验证实际停止/PID0/no restart。即使IAM或文件封存失败，仍继续停止所有绑定资源。
5. 私有 `seal-<uuid>.json` 保存逐步骤结果；命令失败原stdout/stderr只写私有diagnostics。对账provider start/observation、wire witness、7target预算、claim/absence/completion和所有调用成本；保留UNKNOWN。未确认不能包装为NOT_SENT或absence，不能释放旧pins、解除hold、清空预算或清理资源。
6. 生成公开脱敏封存摘要与SHA索引，明确未确认项、实际DELETE数量和NOT_RUN；暂停同一heartbeat。若电脑/Docker不可用导致无法执行关闭，报告封存未确认；冻结sender/wire仍有硬窗口/3分钟context，不能把时间失效宣称为已实测IAM关闭。

## 7. 窗口前验收矩阵

| 项目 | 当前结果 | 执行时必须再核对 |
| --- | --- | --- |
| 新授权及全部冻结绑定 | RECEIVED_BY_REFERENCE；原文与批准载荷分开 | 最新人类消息未撤销、字节及SHA一致 |
| 一次性续跑 | REGISTERED；2026-10-17 10:00+08 | 实际醒来时间、host/app/Docker可用，不能顺延 |
| 权限与物理封存 | PASS_PREWINDOW_CLOSED；32 false、58个绑定容器停止/PID0 | 开窗后 fresh IAM/18版本/Core/backup/budget |
| 回执重用及H1证明 | 独立复评PASS；原字节归档/单字段撤销/实际状态签名反例通过 | 真实IAM fresh回执及NOT_SENT仍NOT_RUN |
| 失败封存 | 正常及-O各14项PASS，含归档失败/旧摘要/enable进程未关闭组合；真实source未启动 | 实际IAM readback、deny gateway、PID0逐项留证 |
| 窗口前独立评审 | PASS_BOUNDED_REVIEW；B1/B2关闭，BLOCKER=0；[复评回执](evidence/issue307-d1/remaining-window-review01/independent-final-review.json) | 当前包损坏仍进入同一批准CID关闭；归档失败仍尝试IAM但拒绝旧摘要；enable未关停不确认稳定身份关闭。timer须绑定本版实际SHA及最终精确HEAD/CI |
| 七目标删除/完整D1 | NOT_RUN / NOT_ACCEPTED | 原生结果与11份survivor、成本/事实历史全对账 |
| 10/17实际窗口 | ABORTED_BEFORE_ARMING / PHYSICALLY_SEALED | 静态前置PASS；business 8版本与4策略官方Equals PASS；宿主raw hash误报终止；其余fresh场景NOT_RUN。0DELETE/claim/budget、59容器停止、32权限false，heartbeat PAUSED |

窗口前工具测试是离线验证，当时未运行真实IAM关闭、H1签名或删除演练；10/17仅business组实际执行了失败后的IAM关闭，H1与删除均未运行。新结果按事实追加，不改冻结包的准备快照。

窗口后诊断：官方MinIO policy.ParseConfig/Equals离线重放4/4等价，真实记录的3个raw差异均仅Action/Resource列表顺序；扩action、换resource/version及畸形policy反例拒绝。原生verifier已有正确OSS语义比较，错误来自未在窗口前回放评审的宿主fresh汇总附加raw hash判断。后续必须预先提交/评审该汇总入口与真实回执的排序/扩权反例；raw SHA继续保留审计用途，不能替代策略集合语义。没有改冻结binary、重启环境或恢复本次运行。
