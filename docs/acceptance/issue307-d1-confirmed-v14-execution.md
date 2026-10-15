# #307 D1：v14 精确确认后的拒绝执行与封存

2026-10-15，**STOPPED_CONFIRMED_DENIED：A 请求一次、B 未发送，成功删除 0，absence/completion 均 0**。
#318 保持 OPEN / Draft，不合并。v14 确认已收到；不能继续将当前状态写为 NOT_RECEIVED / NOT_RUN。
[本次独立 evidence 目录](evidence/issue307-d1/delete-confirmed-v14-execution01/)追加当前事实；原255份历史 raw 保留原字节。

## 精确授权与实际范围

[用户确认回执](evidence/issue307-d1/delete-confirmed-v14-execution01/human-authorization.json)绑定
execution `614cd9ad-3f9d-40be-b110-3f7cf95f0790`、project `dpp-lifecycle307-5d2274ee`、
provider `1980751b-c50e-4895-93a8-cb55960cd635`、incarnation `be784eda-96e4-49a9-ae47-eaa3203a2dc5`、
plan `8f0e9340-18f0-4a73-bdd6-622196a9011a` / hash
`d7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc`。
实际 sender 始终使用 source055aa1df 的原 Linux binary
`01bff6e2a242a202e73519d7b5a4f1fc109430614b25a5189eacfb0ebb2cddf3`；
profile、target digest、原690 bytes IAM和active/sealed gateway hashes均与用户确认一致。
未用后续源码修复替换获批 binary/policy；原候选和旧评审回执不改写。

| 项目 | 实际结果 |
| --- | --- |
| A | `dpp-lifecycle307-online/fixture/a.csv@312365a9-f984-43de-b9d5-7e26009ccd82`，发送一次，HTTP403，CONFIRMED/DENIED，版本仍在 |
| B | `dpp-lifecycle307-quarantine/fixture/b.csv@846d0c7c-289d-4667-bc7c-aa04e44357d4`，sender始终未启动，无新attempt/claim |
| 四份 survivor | identity、native bytes/SHA256、legal hold及retention全部与本次删除前核验一致 |
| 时间边界 | permission固定为08:37:14Z～09:12:00Z，34分46秒；09:05:52Z全部封存，未延长窗口 |
| 实际副作用 | 一份新basis/current approval、一个新dispatch/start/send claim、一个CONFIRMED/DENIED调用及一份真实invocation成本；成功删除0 |
| 最终限制 | cleanup=0、hold-release=0、bypass=0；两份源版本、四份survivor、全部备份和其他环境保留 |

## 核验、拒绝与只读恢复

新[preflight](evidence/issue307-d1/delete-confirmed-v14-execution01/preflight/checkpoint.json)核对
源194表/1879行及roles/owner/ACL与v14一致；27项reference guard资格、54项目标FK检查、
23份requirements/releases、7 pins/terminations、两份输出ready及旧NOT_SENT closure继续成立。
新Core检查点使用专属持久卷恢复，完整fingerprint一致；新原生checkpoint读取六版本，内容/hash/保护不变。
删除前20个读取在新checkpoint Core独立记账，不写源业务事实。

native IAM准备首次对disabled新账号调用AttachPolicy返回404；只读复核其disabled/empty-policy状态。
复用现有SDK的enabled→AttachPolicy→disabled顺序后关联成功；policy回读的StringEquals单值与单元素数组
仅作该字段等价比较，ARN、action、version和其余结构仍完全相同。所有失败管理调用分别保留，未重复创建用户/策略。
发送前另开启精确IAM；regular reader/producer始终原生disabled，root只进入独立admin helper。
原gateway为固定deny-all403，按冻结host verifier要求短暂运行；实际删除流量使用独立exact gateway。
初次staging sender受Postgres镜像默认匿名卷影响，不满足固定two-bind验证；两份未启动helper改名保留，
新sender以tmpfs覆盖scratch，签名host topology通过。原匿名卷未清理。

A 在 `2026-10-15T08:58:59.697914Z` 形成真实调用事实；attempt
`c163dca9-4650-4efe-bdef-e3380bdc9da0`，work `144b5fde-01f3-48a1-ba86-a41c79ebe933`。
固定sender当时将所有返回错误投影为`response_unknown=true`，并在CONFIRMED调用后统一exit93；
该原始回执保留。[DB原始观察](evidence/issue307-d1/delete-confirmed-v14-execution01/final-core-readback.json)
明确为CONFIRMED/DENIED；从其不可变proof重算匹配HTTP403，不能把exit93或错误标志视为response-loss成功验收。

真实Outbox→Redis/Asynq worker已运行；为隔离原有PENDING业务事件，host在单独transaction对非目标
outbox rows持行锁，现有dispatcher以SKIP LOCKED仅发布本work，然后ROLLBACK释放锁。
没有手工改status/claim、伪造consumption或直接注入队列；旧业务outbox各event type/status/count保持一致。
worker读完整inventory和exact HEAD，保留UNKNOWN_PRESENT，不能追加absence或启动B。
随后停止worker并关闭发送权限及IAM。经再次六版本原生内容/保护核验，复用已有受控
`ExactExecution.ReconcileDeniedAttempt`，以fresh正向HEAD为依据追加DENIED reservation release。
原DENIED、成本、旧两条NOT_SENT/releases及不确定恢复观察不改写。
existing RecoveryService再追加ALREADY_RESOLVED终态；[终态重放](evidence/issue307-d1/delete-confirmed-v14-execution01/denied-resolution/terminal-replay.json)
新增provider I/O=0。这是“拒绝且仍在”的恢复，不是缺失或删除成功。

源最终125 calls/125 observations/125 typed storage costs，另有1份真实provider invocation成本，
unaccounted=0；旧业务内容、roles/ACL及旧outbox状态不变。native管理API调用另列admin receipts，不冒充业务storage成本。
新Core dump、恢复副本、私有payload及全部helper保留。
[封存回执](evidence/issue307-d1/delete-confirmed-v14-execution01/sealed-final-state.json)确认86个5d相关容器
全部exited/created、PID0、0 host ports，native executor disabled、sender permission/recovery maintenance关闭。
原tar/dump SHA256不变。额外两份network-none离线诊断helper同样停止保留，不启动源环境。

## 根因与当前修复边界

固定MinIO binary模块为`github.com/minio/minio@v0.0.0-20251015172955-9e49d5e7a648`，源码
[`authorizeRequest`](https://github.com/minio/minio/blob/9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a/cmd/auth-handler.go#L430)
对version DELETE先做DeleteObjectVersion的deny检查，再要求DeleteObject allow。
candidate04只生成DeleteObjectVersion allow，缺少DeleteObject，因此不能授权这条实际路径。
仓库旧native验收`examples/lifecycle-d1/results/a6b30840-3e76-49ad-91d3-f0c399fcf170/delete-review.md`
已记录该产品特性，本候选遗漏了复用；此前结构检查与只读评审没有证明原生可发送性。

[隔离gateway诊断](evidence/issue307-d1/delete-confirmed-v14-execution01/gateway-offline/route-diagnostic.json)
仅把同一gateway的upstream替换为network-none命名空间里的loopback responder，使用假Authorization。
两份精确URI到达responder且204被abort；错误version返回403、upstream调用0。
这排除了所测URI matcher的离线阻断；它没有再对5d或任何真实对象发送DELETE，也不冒充原生权限成功。

当前源码修正IAM生成器：两项action同受各自exact ARN + `s3:versionid`约束；
复用已引入的`github.com/minio/pkg/v3 v3.1.3`真实policy evaluator验证两目标allow、四survivor/bare key/
其他version/GET/PUT/hold-change/bypass拒绝。无新增依赖或自研策略引擎。
sender同时从immutable observation读取certainty/outcome，只在CONFIRMED/UNKNOWN时走预期crash；
DENIED不再被错误标记为UNKNOWN/crash验收通过。
delete/recovery/storage/pilot四包测试已PASS；后续修复尚未在源环境激活。

**本execution的A“一次”已消费，禁止在旧确认下重发。** B因第一份absence前置条件失败而未启动。
后续若继续删除，需要先形成新的独立执行身份、重新冻结修复后的source/binary/profile/IAM及当前Core检查点，
并提供新的精确方案供用户确认；不得复用旧614cd9ad basis/approval、扩展旧窗口或在本回执上伪造completion。
当前仍有阻断：固定旧候选原生IAM无法完成删除；成功response-loss/absence恢复和两份scope completion均未通过。
本轮不再机械刷新只读proposal编号；#307保持OPEN，#318保持Draft。

有限[独立复评](evidence/issue307-d1/delete-confirmed-v14-review01/independent-review.json)已PASS，
该拒绝执行/封存与离线修复切片的有效问题0；独立执行4项TestReference通过，源125调用/成本、
三UNKNOWN_PRESENT、DENIED release、ALREADY_RESOLVED、六版本保留及86容器封存逐项闭合。
新81payload/index与旧255raw原字节均核对通过。sender新的SQL读回与结果分支只做静态复评，
未在5d运行修复后的sender；原生成功删除和两份completion仍BLOCKED，不因本次复评通过而解除。
