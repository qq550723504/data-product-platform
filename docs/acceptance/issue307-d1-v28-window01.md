# #307 D1 candidate18/v28 固定窗口结果：首项403后封存

execution `4fd5a7e7-779e-41d7-a35a-3bab7dd615b5`，授权窗口2026-10-21新加坡22:00–22:40（UTC14:00≤now<14:40）。本轮已终结且no_resume=true，不能在剩余窗口继续发、重发P2或复用本授权。

**结果：首项P2的真实exact DELETE调用返回HTTP403，Core记录CONFIRMED / DENIED。成功删除确认为0；P2 sender预算与wire预算各消耗1个对应记录，保持原样。其余五项发送NOT_RUN。** 未将本次失败称为“没有调用provider”或“预算未使用”。

| 验收项 | 实际证据与结果 |
| --- | --- |
| 人类授权及冻结绑定 | 实际新删除授权、独立调度授权、1062项批准引用、当前HEAD与全部冻结字段核对通过；没有复用旧授权或修改source/binary/candidate/config绕过门禁。 |
| 窗口内GitHub | UTC14:02新鲜读回：HEAD/PR head=`a8930b31aa84fe0c003e2744253358e36c2f12f7`，精确HEAD PR CI run39577806574/event=pull_request，13/13成功、未解决线程0；#332 OPEN/Draft/unmerged、#307 OPEN。不是main CI。 |
| 固定完整原生只读入口 | UTC14:02:52–14:05:23实际PASS_COMPLETE_SIX_NATIVE_ENTRY。10个显式索引/4418项引用、实际case/window、完整owner/mount/network/budget、三源Core前后不变、17版本及保护/库存/bytes、35 disabled IAM及9 absent、备份及角色隔离通过；137技术请求start/observation对保留。 |
| 完整入口后关闭检查 | UTC14:06:13–14:07:39，先通过冻结Host.check的PASS_PREWINDOW_CLOSED，再重启绑定Source；未倒置顺序。30份权限关闭、6case/6recovery与3新空预算、86本代owner及固定历史门禁通过。 |
| 实际Source准备 | 仅固定三Core/native、受限inspector和独立恢复Redis按方案启用。固定remaining-resume-business完成P2/T1/T2本execution新批准/dispatch；旧Processing/DirectData没有重跑、新输出0、P1未重跑。新T1/T2 admission不等于已经发送。 |
| 首项P2 | exact版本`dpp-lifecycle307-online/fixture/p2.csv@d8040608-c5cd-4a6f-8a2d-b3f856cf5302`，copy`6fabf3b5-76b8-4245-8f5d-01e64f610146`，attempt`4d930776-fd98-4a86-8ed2-0a1eaad7dd34`，dispatch`eacd910a-3ecd-448e-9c87-13bdf14d8817`。固定wire/sender各自新鲜实际拓扑通过，sender UTC14:14:12.547启动、14:14:14.248退出1。 |
| 原始发送事实 | P2-returned.json：budget_consumed=true、claim_confirmed=true、provider_observation_present=true、provider_invocation=CONFIRMED、certainty=CONFIRMED、outcome=DENIED，automatic_resend=false、automatic_recovery=false。InvokedAt=14:14:13.759550Z，ObservedAt=14:14:14.058613Z。stderr保留storage denied及contract violation；没有覆盖失败或把DENIED改写为NOT_SENT/ACCEPTED。 |
| HTTP403证据 | 按原ObservationHash的Attempt/Status/Outcome/Observed结构重算，403精确匹配原ProofSHA256 `d41fbc8ab1286069830157251e7d3d1fd00ca61be7325c9671dc7a49a4653680`；401不匹配。wire预算已经创建，原wire代码只有通过请求/attempt/fresh/budget门禁后才转发上游；未产生204 witness。 |
| 拒绝根因边界 | 尚不能确认是有效IAM条件、签名/转发上下文或其他native授权原因。冻结delete transport刻意不保存provider错误文本，wire仅为204保存witness，现有事实不能区分具体native错误码；不得凭403猜一个根因、修改IAM或重新DELETE验证。下一步应先在封存证据上归因。 |
| 失败即停止 | 首项异常后没有执行T1/T2/H2A/M1A/M1B，没有重发P2，也没有lookup/recover来掩盖原失败。H0/H1、H2真实late hold和M1 mixed outcomes均NOT_RUN。 |
| 固定失败封存 | 使用原window_six_support.py seal，all_steps_confirmed=true。三组实际disable-all读回全部known身份disabled、enablers关闭、policy验证通过、归档完整；30permission/case/recovery撤销、gateway停止并封网、Source/Core/native/恢复Redis和原business Redis停止。 |
| 封存后实际读回 | 715个已知CID（原712+本次完整入口3个新reader）全部停止/PID0/noports/no-restart；30权限false、所有case/recovery配置撤销、三个gateway实际sealed。business新预算保留P2 sender+wire两文件，late/mixed两预算仍为空；48备份实际SHA不变。失败后Source重启/provider lookup/新增DELETE=0。 |
| 调度收口 | 按本次heartbeat生命周期要求删除`307-d1-candidate18-v28`；原实际TOML在删除前按字节归档。工具确认deleted，SQLite实际matching schedule=0、TOML已不存在；不会继续唤醒、创建重复调度或移动窗口。删除的是调度，不是fixture环境或证据。 |
| 成本与保留 | 137次完整预检技术请求、267次root admin请求各有原start/observation对；另有1次真实exact DELETE被拒绝。sender/case的inspection请求保存在Source Core，封存后未再启动Core重数，因此不能把这些局部计数当成全部provider调用总数。amount仍NOT_ASSESSED；旧/新预算、pins、全部备份、原失败/UNKNOWN/attempt/成本事实保留，cleanup=0、hold-release=0、bypass=0。 |
| 最终验收 | 完整D1 NOT_ACCEPTED；#332继续Draft、不合并，#307继续OPEN。当前需要先解决真实native拒绝并处理本次新增dispatch/claim保留状态，再讨论新候选/新execution/独立预算与新授权；本轮没有自动准备新候选或申请新窗口。 |

原始公开证据位于[evidence/window01](evidence/issue307-d1/delete-confirmed-v28-execution01/window01/)，包括固定完整入口、真实发送返回、root enable/disable原始请求对、阶段退出码、失败seal、预算原字节索引、HTTP403证明及715CID封存读回。私有同一authorized-execution01保存terminal、预算tar、permission/seal/gateway历史及全部原始授权。

终态`FAILED_FIRST_P2_CONFIRMED_DENIED_SEALED_NO_CONFIRMED_DELETION`仅记录本轮失败。失败后未重新核验live版本库存，故不把“0份成功删除”扩写为一次新鲜的“17版本当前仍全在”证明；原14:05完整只读库存事实保留。原准备、核对、授权及调度快照不覆盖，旧代失败及UNKNOWN不回写。
