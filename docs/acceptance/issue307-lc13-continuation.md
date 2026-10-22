# #307 LC-13：可信停止、新 Retry 与 credential containment

当前PR收口见[#318最终评审及合并条件](issue318-final-closure.md)。以下为2026-10-14三份LC-13环境的非删除验收时点；5d的单独v15授权执行已完成并封存，其当前事实见收口入口，旧“待确认”不再描述整个PR状态。

2026-10-14；[有限contract](issue307-lc13-resume-contract.md)。#318保持Draft。
0 merge、0 object DELETE、0 hold/retention release/bypass、0 cleanup。
用户明确选择新Retry Execution；原FAILED、pins、native calls、成本和观察保留。
原04a6六个未知pins、5d六版本及备份/恢复副本/封存窗口未改动。

## 非删除验收矩阵

| 场景 | 结果与边界 | 证据 |
| --- | --- | --- |
| native processing crash | f3及升级后完整binary的812均实际GET→exit95，旧执行拥有三个输入pins，无output intent。 | [812 crash](evidence/issue307-d1/lc13-continuation01/processing-current/lc13-crash-278d115a-4f1d-4fc7-ba72-893e78c6f9f4.json)、[来源](evidence/issue307-d1/lc13-continuation01/processing-current/environment.json) |
| trusted owner stop | host签名绑定helper/image/binary/runtime closure/full pin hash；旧mount永久false、旧native identities disabled；固定旧helper restart exit1，0新calls。事务内fresh authority/full membership/generation/output checks；三个停止事实原子追加，replay稳定。 | [stop](evidence/issue307-d1/lc13-continuation01/processing-current/resume-stop-verified.json)、[closure](evidence/issue307-d1/lc13-continuation01/processing-current/owner-runtime-closure.json) |
| 成功新Retry | 812原`278d115a…`保持FAILED；新`5043802d…` RetryOf指向原执行，实际Redis/Asynq worker、fresh pins、exact-version GET与native processing PUT→SUCCEEDED/output `c454f64e…`。没有同Execution新generation。 | [success](evidence/issue307-d1/lc13-continuation01/processing-current/resume-native-success.json) |
| native credential / replay | 7cd两次MinIO service-account签发，actual expiry/exact policy；same-key recover/replay同一访问能力。platform redemption再次认证/fresh gate/DirectData ISSUED+pin+真实GET：200；错误token403。原生S3 secret不返回consumer。 | [issue/redemption](evidence/issue307-d1/lc13-continuation01/credential/credential-issue.json) |
| future expiry cap | 第二请求`14:34:13.130668Z`，实际native expiry `14:29:13Z`，受本fixture实际use_until、rights、固定caller窗口约束；不声称通用delegation chain已实现。 | 同一issue回执 |
| native自然到期 | 第一capability实际`14:00:42Z`到期；host自然等待后nativeGET403，material demand仍1。未凭到期解占用，未改时钟/persisted deadline。 | [expiry](evidence/issue307-d1/lc13-continuation01/credential/credential-expiry.json) |
| unknown→confirmed revoke | native disable实际204后丢响应，0credential输出、demand保持；追加replay CONTAINMENT_PENDING Event/Audit/Evidence/Outbox；后续真实确认与GET403。两条原ISSUED保留。含早期policy比较失败共3份pending/Audit/Evidence/Outbox，按replay identity一一匹配。 | [unknown](evidence/issue307-d1/lc13-continuation01/credential/credential-contain.json)、[confirmed](evidence/issue307-d1/lc13-continuation01/credential/credential-confirm.json)、[READ ONLY history](evidence/issue307-d1/lc13-continuation01/credential/containment-history-readback.json) |
| signed reference release | payload/issuer/root owners退出、配置false、parent native disabled、nativeGET403；签名绑定原op/full context/hash/expiry/exact copy及typed denial call。事务内无SDK I/O，2份DELIVERY release及replay稳定。 | [release](evidence/issue307-d1/lc13-continuation01/credential/credential-release.json) |
| 实际失败恢复 | f3四次400/500/hash mismatch/unknown revoke等start/观察/成本保留；root-only有限forensic按原stable key lookup/contain，4原op→BLOCKED，4次GET403，追加4份signed release，无历史覆盖。 | [forensic](evidence/issue307-d1/lc13-continuation01/credential-forensic.json)、[release](evidence/issue307-d1/lc13-continuation01/credential-release.json) |
| 拒绝/原子性 | signed proof缺失/篡改/foreign/open/stale/partial拒绝；真实PG partial pins、wrong generation、第二proof失败整批rollback、replay。MinIO OSS识别policy规范化等价，扩大version/resource/action拒绝。 | [回归](evidence/issue307-d1/lc13-continuation01/verification.json) |

## 环境、账务与封存

| Run / workspace | storage calls/observations/typed costs | provider attempts/observations/typed costs | 状态 |
| --- | ---: | ---: | --- |
| f3b0c9cd-ee53-42d5-a5e4-48c9ede5e4c3 / 43818661-5a41-4bbf-ab86-341f0c442c3f | 23/23/23 | 9/9/9 | 原FAILED+新SUCCEEDED；4failed credentials BLOCKED；6pins/6stops |
| 7cd53b0e-44d9-4f3a-9eb7-041f496ff277 / 0cbfdbc2-a5a5-45c2-bb3e-7387ad2e55f4 | 24/24/24 | 12/12/12 | 2原ISSUED、最新replay BLOCKED；2pins/2stops；准备用QUEUED execution未运行 |
| 8120904f-13d9-42f3-ab1f-66eb1a37b2b0 / 3e43d757-29dd-46aa-bac6-6e172438a4c8 | 19/19/19 | 0 | 升级后完整binary新Retry成功；6pins/6stops |

每次实际storage call/provider attempt恰一次typed allocation，unaccounted=0。
三环境active pins、unreleased credential demands、send claim/absence/completion/DELETE均0。
IAM provisioning/sealing管理RPC独立回执与业务attempt分别计数，不冒充delivery成本。
PG/MinIO/Redis named volumes全部保留，internal subnet159/160/161、0 host ports。
17个已创建角色配置含未用别名全false；reader/producer/issuer native disabled。
closure snapshot覆盖签名前23/15 helpers；随后新建的成功release observer另列最终清单，未回写旧snapshot。
最终清单含12个services、51个退出helpers、17个封存角色；两份network=none D1候选helper另列。
[末阶段索引](evidence/issue307-d1/lc13-continuation01/evidence-index-02.json)包含最后生成的metadata，
原498文件索引保留。[源码绑定](evidence/issue307-d1/lc13-continuation01/runtime-source-binding.json)
以实际native binary及runtime文件hash核对source055aa1df；原44d5 dirty-tree构建baseline保留，
不把旧baseline HEAD写成当时尚不存在的新commit。
最终索引517项由主线程逐项验证；独立review只覆盖其明确列出的源码/阶段索引/签名/账务回执，
没有独立重演SQL、provider、测试或最后新生成的索引。

## 回归、评审与未验收边界

六包真实PG `-race -count=1 -p 1`：235项测试/子测试PASS，0fail/0race；相关vet PASS。
`TestIngestionStorageIsolation`因未设置独立ingestion环境跳过1项，不计入native/235项PASS。
Python30项正常及`-O`均PASS。SDK升级后native GET/PUT在812完整重跑。
SDK更新后的delete/archive命令另6项非删除回归PASS；D1当前候选已在network=none验证
render和disabled-before-DB/provider gate，无provider I/O或批准。
MinIO policy OSS（AGPL-3.0）和必要SDK/transitives升级见Reuse Check；无新平行基础设施/schema。
独立review只读源码/签名/closure离线核对，未代跑SQL/provider/tests；关闭原mount重开、陈旧proof刷新、
90s helper等待15min和processing-only遗留root别名根因，收敛checkpoint无当前有效BLOCKER。
另关闭D1旧来源确认模板冲突、IAM原始字节及Windows checkout保存问题；
[独立收敛回执](evidence/issue307-d1/lc13-continuation01/convergence-review.json)保留实际核对范围。
当前HEAD PR CI另列最终checkpoint，本地/native证据不冒充main CI/merge/deployment。

同Execution新generation、通用external engine owner stop、生产/多租户IAM或credential provider、
完整expiry delegation chain、真实DELETE fault/race及完整D1/D2仍未验收。
Native管理Resource=*仅限独立synthetic inventory/fixed composition。本结果不授予生产启用。
5d精确两version删除[方案仍待确认](issue307-d1-delete-proposal.md)，本轮三run全部排除。
