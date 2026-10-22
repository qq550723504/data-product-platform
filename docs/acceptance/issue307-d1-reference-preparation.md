# #307 D1：六版本原生恢复与服务端非删除准备

2026-10-14，本阶段 **PASS_NATIVE_NON_DELETE_PREPARATION**。#318 保持 Draft；
真实 DELETE、hold release、retention bypass、cleanup 均为 0，未收到删除确认。
当前精确确认范围见[删除方案](issue307-d1-delete-proposal.md)。

## 当前源码与边界

原生执行绑定 source `4af0ee88db92dfbe7aad029b22916bb046155e27`、
Linux binary `e1d9d35e48210ffe4400edb3476bdc8a51cb71cab9bd05686aedbc791a0a5b30`，
所列97个源码/脚本/schema文件及binary hash见[build-source02](evidence/issue307-d1/reference-disposal-preparation01/build-source02.json)；全仓库版本由source HEAD绑定。
原 source Core、MinIO、gateway 容器、镜像、named volumes 和网络身份保持原值。
本阶段复用既有 lifecycle fence/Commands/journal、Cost/Evidence/Audit/Outbox、
MinIO SDK/原生存储和 PostgreSQL dump/restore，不新增 runtime 或通用删除 API。
migration67 负责 reference-demand，新增68只允许专用 checkpoint read 的 typed storage call；
它没有伪造 reader pin、复制业务 admission 或 processing output intent。down 拒绝破坏历史。

## 原生 no-send 事务

[事务回执](evidence/issue307-d1/reference-disposal-preparation01/no-send02/no-send-transaction.json)
绑定原两份 trial attempts。可信 Windows host 在事务前重新执行固定 canonical verifier，
核对原 executor 的结束状态、入口/binary/mount、root-free 环境和不授予 DELETE 的 native policy；
一次性 Ed25519 signer 不持久化、不挂载。服务端只接收其 public key 和 signed receipt，
在共享 fence、plan/dispatch/attempt 锁内重验签名、scope、freshness、原批准和无冲突当前 DB。
事务中不调用 Docker、HTTP 或 provider。

两条 observation、两条 reservation release 与 Event/Audit/Evidence/Outbox **一次原子提交**。
[同 proof 重放](evidence/issue307-d1/reference-disposal-preparation01/no-send-replay/no-send-transaction.json)
返回 Replayed=true，不增加事实或虚构 invocation cost。
[实际持久化验签/DB闭合](evidence/issue307-d1/reference-disposal-preparation01/no-send-cryptographic-db-closure.json)
公开验证用public key，离线Ed25519验签成功，signed body hash与两条DB observation proof hash相同。
原 starts/approval/basis 不覆盖，
旧 trial attempts 永远不发送。当前 send claims、absence receipts、completion 均为0。

## 六版本备份与独立原生恢复

[源 before](evidence/issue307-d1/reference-disposal-preparation01/native-before/native-six-version.json)、
[独立 restore](evidence/issue307-d1/reference-disposal-preparation01/native-restored/native-six-version.json)、
[源 after](evidence/issue307-d1/reference-disposal-preparation01/native-after/native-six-version.json)
均原生读取六份 exact version，校验 bytes/SHA256，并读取 native hold/retention 与两份完整 namespace inventory。
每次执行20个 physical calls：6 exact GET、12 protection reads、2 version listings。
源 Core 独立记录40个新增调用，累计92 calls/observations/typed costs；恢复 Core 独立记录20个调用，
不进入源 ledger。payload/base64、完整 tar、provider 内部/IAM 文件均私有保留。

[snapshot](evidence/issue307-d1/reference-disposal-preparation01/native-snapshot.json)
复用 GNU tar，将源 object volume 只读挂载到 network=none helper；源服务未停止或重启。
两份封存 synthetic bucket 对象树的 before/after SHA256 相等，随后独立 MinIO 原生恢复核对全部六版本。
这只证明本封存 fixture 的六版本恢复，不声称任意活动生产 MinIO 的一致热备份。
[closure](evidence/issue307-d1/reference-disposal-preparation01/native-restore-closure.json)
确认 identity/bytes/hash/hold/retention 全等，backup payload 不作为 source absence。

| 保留副本 | 精确身份 / 状态 |
| --- | --- |
| Object container/volume | `dpp-lifecycle307-5d2274ee-predelete-objects`；container `066eadc202b07816f4736578f1211ac84e9257ffc92db17b5116fdeb2d510924`；image `sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6`；STOPPED_RETAINED |
| 最新 Core container/volume | `dpp-lifecycle307-5d2274ee-predelete-postgres-02`；container `1976c314879a819f548c22e621a81277e43c80fa6e3de7969eadc4f169a19332`；image `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`；STOPPED_RETAINED |
| 最新 Core network | `dpp-lifecycle307-5d2274ee-predelete-private-02`，internal `10.234.153.0/24`，ID `db584ee347fa8a7946c82a73e2adfa62ad313b6153dbe8e2de870e4817797a24`；无 host ports |

最新[Core checkpoint](evidence/issue307-d1/delete-preflight-5d2274ee-generation02/core-checkpoint.json)
覆盖 no-send/新增成本/schema68 后的194表、1879行；源 before/after 和独立 restore 全部 fingerprints 相同，
roles/SCRAM verifier、owner/ACL 一致。[真实 SCRAM 正反验证](evidence/issue307-d1/delete-preflight-5d2274ee-generation02/core-checkpoint-auth.json)
使用独立 suffix02 helpers，合法密码成功、错误密码拒绝；旧 helpers 保留。
[final readback](evidence/issue307-d1/delete-preflight-5d2274ee-generation02/core-checkpoint-final-readback.json)
确认实际 dump/restore 与最终复核脚本均为 SHA256
`0524d81b2a7e57ddcd79874c2e108a4850c2a693446c96f20314412a0a80e169`，运行中未变。

generation01 Core replica 曾作为隔离恢复读取的 maintenance journal，添加schema68与20个恢复调用，
其旧1377行全库 fingerprint 属于历史检查点；没有覆盖旧回执。
当前完整 Core 检查点为generation02。
[业务 fingerprint closure](evidence/issue307-d1/reference-disposal-preparation01/business-fingerprint-closure.json)
证明仅13个维护/history/成本表变化，其他181表保持原 fingerprint。
原 LOST C2 Core 仍为 **LOST/UNAVAILABLE/NOT_RESTORED**。

## 受限发送组合与当前未执行项

[关闭的 profile](evidence/issue307-d1/reference-disposal-preparation01/render02/bounded-profile.json)
固定环境、execution614cd9ad、两项目标、六份 inventory、源码/binary 和 IAM/gateway hashes。
新 sender 要求另行明确的 exact-list 确认、server-owned permission、≤40分钟窗口、
签名 fresh host topology（精确源容器/DSN、新 gateway、root-free executor 和 binary）；
不得复用旧 trial。A 的 absence receipt 完成后才允许B；present inventory按可信receipt从6→5→4闭合。
每次 dispatch 与 ClaimExactSend 都重新 gate；未知结果只走现有只读 recovery，不重发。

[IAM JSON](evidence/issue307-d1/reference-disposal-preparation01/render02/delete-executor-policy.json)
仅两条 exact key ARN + `s3:versionid` 条件的 `s3:DeleteObjectVersion`。
实际 Caddy文件私有；[network=none 原生语法验证](evidence/issue307-d1/reference-disposal-preparation01/caddy-validation.json)
与[新渲染相同字节绑定](evidence/issue307-d1/reference-disposal-preparation01/caddy-validation02-binding.json)
均通过。[新 binary 禁用演练](evidence/issue307-d1/reference-disposal-preparation01/rehearse02/send-disabled.json)
在 DB/provider 之前拒绝发送。source gateway 未改，新 maintenance permissions 已
[封存](evidence/issue307-d1/reference-disposal-preparation01/sealed.json)。

**Native IAM provision、新 active gateway、enabled send permission、新 disposal basis/approval、
真实 send/response loss/crash、当前环境的 Outbox→Asynq recovery/absence/completion 仍 NOT_RUN。**
这些在另行删除确认后按方案执行；现有队列/worker受复用，未用旧其他run driver代替当前范围。
源码/非删除验证不等于 native IAM 授权或完整 D1 验收。

## 保留失败与评审

[初始 build 诊断](evidence/issue307-d1/reference-disposal-preparation01/initial-build-diagnostic.json)
记录存储断言 PowerShell数组拼接错误，发生在资源创建前；括号修复后新 build通过。
[首次 no-send 失败](evidence/issue307-d1/reference-disposal-preparation01/no-send-initial-failure.json)
因 RawMessage 被JSON缩进重排后验签拒绝，原子事务完整回滚。
signed Body现以byte array/base64保持原始字节，no-send/topology两条持久化roundtrip回归通过，
明确新bin02/proof/no-send02成功；旧二进制、失败helper/proof/日志均保留。
独立评审还发现render私有输出路径与严格config路径不匹配；已改为私有挂载/run/output，
只公开两个JSON，补上普通/优化Python回归。19项Python tests在两模式均通过，go vet通过。

[当前源码验证](evidence/issue307-d1/reference-disposal-preparation01/source-validation.json)：
source4af0ee88 在全新已迁移schema68的独立测试库执行
`go test -race -count=1 -p 1 -json ./...`，89测试包、1117项test/subtest通过，0失败、0race报告；
14项环境条件跳过逐项列在回执，不视为原生外部服务验收。
[首次全模块诊断](evidence/issue307-d1/reference-disposal-preparation01/full-race-initial-diagnostic.json)
保留：共享schema测试最初指向空postgres库而失败，独立lifecycle fixture已自行迁移并通过；
修正为新的已迁移数据库后完整重跑成功，未使用源验收Core。

独立评审的源码与原生回执范围在本阶段manifest中分别绑定；reviewer只读检查，
未独立执行runtime或测试。完整D1、native pre-ISSUED pause、剩余worker fault combinations、
archive/restore业务rerun仍未验收，#307继续OPEN，#318不Ready/merge/deploy。
