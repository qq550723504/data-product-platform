# #307/D1：schema66 历史非删除验收与下一步清单

2026-10-16 当前入口：[剩余 D1 有限设计与非删除准备矩阵](issue307-d1-remaining-contract.md)。bca29f4d 的 GitHub 同步已恢复、独立只读复评 BLOCKER=0；新 [PR #323](https://github.com/qq550723504/data-product-platform/pull/323) 保持 Draft，最新提交 CI 见 PR checkpoint，原生组合仍 NOT_RUN。#318 已 squash 合并为 `b823eb33`，matching push/main CI `38842607982` 13/13 SUCCESS；#307 仍 OPEN。5d 的单独 v15 两版本删除/恢复已完成并封存，不复用其授权。以下原阶段和[#318合并前最终评审](issue318-final-closure.md)中的 Draft/NOT_RUN 文字保留各自历史时点。

本文件保留2026-10-13原阶段。2026-10-14当前补验与矩阵见
[native completion](issue307-d1-native-completion.md)及[LC-13/5d只读核验](issue307-lc13-readonly.md)；真实删除仍待单独确认。

2026-10-13，基线 main `79d175d0c8ff7579b0466f44e9103672ac7588da`。
本轮只完成非删除验收、独立备份恢复和 exact 删除候选准备。
**完整 D1 / 实际删除竞争 NOT_RUN；人工解释 NOT_RUN / 暂缓。**
#307 和 #296 保持 OPEN；不进入 D2，不追加总体蓝图。

证据入口：[回执 hash manifest](evidence/issue307-d1/receipt-manifest.json)、
[非删除实跑目录](../../examples/lifecycle-d1/results/9b89333a-9620-4387-a520-7685641281e2/)、
[下一环境 exact 候选](../../examples/lifecycle-d1/results/9812f0fc-c05a-4bcb-87ea-a4cef9089f3f/deletion-candidate.json)。

## 源码、复用与最小修复

原 D1 writer 的 `66625210` 分支干净、最后 turn 已结束；本轮使用独立
`codex/issue307-d1-acceptance`，没有修改其 checkout、保留环境或迁移。
复用既有 lifecycle Commands、fence、typed pins、NATIVE/DirectData、MinIO
Adapter、PG/Asynq/Worker/Outbox、Cost/Audit/Evidence；未引入基础设施。

实跑发现并修复两个 required-path 问题：

- 非删除夹具硬编码 schema64 / 旧 main1892，拒绝当前新安装的 schema66。
  将非删除 reader/plane/copy helpers 统一为本轮严格 main79/schema66；不接受
  64 fallback，不升级旧库，不改 Core gate、60–66 SQL 或已消费的删除脚本。
- Docker29.8.2/API1.56 实际 inspect 在空映射时省略 Tmpfs，使 StrictMode
  预检退出；[Moby HostConfig](https://github.com/moby/moby/blob/master/api/types/container/hostconfig.go)
  将该字段标为可省略。修复 optional metadata 读取，保留实际 named-volume、
  ownership、路径、WAL/parent/nested tmpfs 拒绝规则；5 正例/19 拒绝负例通过。

修复提交 `c00ff44a`。运行二进制在提交前由 main79 + 同一组 schema66 夹具改动
构建；provision 回执记录main79，`go version -m`未记录VCS revision，
不能声称其 build metadata 是c00。
62 个实际源码文件 hash 和 binary SHA256 绑定见本轮回执。
其中60文件为Windows CRLF checkout，2文件与Git blob字节相同；另存Git blob
hash，逐文件确认差异仅为CRLF→LF，不把checkout原始hash冒充Git blob hash。

## 实际非删除结果

Run `9b89333a-9620-4387-a520-7685641281e2`，internal `10.234.80.0/24`，
无宿主端口；Core/MinIO 均使用独立 named volume，通过真实 durable preflight。

- 新装全部63个迁移文件，max66；当前原生 retention/hold、四版本原值/hash
  与盘点通过。Core 原 owner/ACL、192表/258行完整恢复相符，真实独立网络
  SCRAM 正确凭据连接、错误密码拒绝；四个 object version/size/SHA256 恢复相符。
- 真实 NATIVE processing → 新输出、独立 Quality/Certification/current rights
  → DirectData 实际129328字节；同 key 不重读，排队后 source restriction
  阻断新 worker I/O，过期后新 delivery 0 I/O。
- 实际 TCP 慢客户端不读 payload，无 writer barrier；UseUntil 自然到期，
  WriteDeadline/Flush timeout 后确认停止，追加36714字节 interrupted observation。
  原两份 ISSUED 不改写，accepted bytes 不等同客户端收到。
- 独立SQL核对40 physical calls /40 observations /40 typed invocation costs；
  5 pins /5 verified termination，active pins=0。原 hold/retention 不变，
  六个版本及所有成功/失败 helper、named volumes 保留，窗口和身份已封存。
- 新独立测试PG运行 lifecycle domain/application/pilot/HTTP/storage `go test -race`
  全通过，application215.532s；fixture vet通过。该库不与任何验收/保留Core共用。

原 schema64、Tmpfs 和备份 harness 命令失败保持 FAIL，不重标。
备份首轮 localhost 的错误密码没有被拒绝，属于验证方法错误，未计认证PASS；
改用独立网络客户端后 SCRAM 的正负例通过，未修改原PG认证规则。
缺失手工 source/accounting 回执的原 offline 命令失败也保留；补独立采集后通过。

## 必需矩阵的证明边界

| 必需用例 | 本轮已证明 | 仍需的原生闭环 |
| --- | --- | --- |
| LC-01/02 | fresh pinned read、当前restriction/expiry阻断、历史ISSUED保留 | LC-01读与实际删除竞争未跑 |
| LC-03/20 | 当前PG copy/restore/witness/restriction回归通过；历史原生报告保留原阶段 | 当前代码原生archive/restore闭环尚未重跑 |
| LC-04/15/16 | 当前PG dispatch/observation/absence/recovery/成本回归通过；四版本原生inventory通过 | 新exact DELETE、unknown/absence和receipt未跑 |
| LC-05–08 | 当前PG保全/dispatch双方锁序、shared/reference/unknown拒绝回归通过 | 原生late hold/partial delete等组合待批准执行 |
| LC-11/12 | actual processing/transfer pin、慢TCP、verified停止、反向限制0 I/O | 两种顺序的实际处置竞争未跑，不计完整PASS |
| LC-13 | [新原生补验](issue307-lc13-readonly.md)：实际GET后exit95、真实Asynq重投递、自然10分钟recovery安全FAILED，0新I/O且六pins保留 | positive same-execution resume、可信外部owner终止/自动解pin、credential containment仍未验收 |
| LC-14 | 当前PG partial/unknown per-copy事实回归 | 三copy原生混合结果未跑 |
| LC-18/19 | 独立全备份恢复隔离；PG watermark/gap/wrong-context拒绝回归 | 不是生产备份重入库或D4验收 |
| LC-21–24 | 当前PG finalize/restriction/member/authority/idempotency双方竞争与拒绝回归通过 | 不将PG模拟端口当成原生删除证明 |
| LC-28 | internal网络、0 host ports、gateway闭合及sealed deny-all | 全平台reader、自动lifecycle/replication与D2不在此片 |

因此本轮不能勾选完整D1或关闭#307。既有设计文档的NOT_RUN是其原阶段，不改历史。

## 待确认的新环境与exact版本

为后续LC-11/12竞争另建 `9812f0fc-c05a-4bcb-87ea-a4cef9089f3f`，
project `dpp-lifecycle307-9812f0fc`，internal `10.234.82.0/24`，无宿主端口。
此环境未开启业务reader/processing/delivery窗口，fixture principal disabled。
独立Core192表/258行及四个原生版本备份恢复、owner/ACL/SCRAM正负例通过。
两环境的公开备份摘要绑定 dump/tar SHA256；备份均在业务运行和候选plan之前，
不包含随后新输出或plan事实。A环境原生archive/restore未跑，与完整备份恢复分开。

Plan `756a4139-bfae-4b5e-9368-e65d28d4c877` FINALIZED，hash
`fa2b19e1d7fc12132b3f4e4b4acee09e4f5bc0f59eae87820b83fb2fff220c86`。
仅准备候选、0 storage/DELETE；approval=PENDING_USER_CONFIRMATION。

| namespace / key | exact provider version | bytes / SHA256 |
| --- | --- | --- |
| dpp-lifecycle307-online / fixture/a.csv | 0f6aa755-4b86-496b-9ee1-43d7c59a8bb3 | 232 / 722b7c8834f35892e3a3404b43ff8118b326b5c0c992207290dcc674f6b7ade0 |
| dpp-lifecycle307-quarantine / fixture/b.csv | e1611b0a-efc0-4939-abc2-30394ec2c8d2 | 129328 / b5b9321c647c6dc75e27c25beb46ecf7f4cf10f9bdd0d288887e41c343ef3524 |

批准仅用于这两个source-provider版本的有界处置竞争/unknown恢复与receipt；
仍须每次重新检查当前caller、approval、retention/hold、reference和reader pins。
任何阻断即0 DELETE。此新环境其余两个protected版本、此前封存环境六版本、
全部旧运行版本、protected objects、
数据库事实、备份、恢复副本、namespace/bucket、容器和卷均保留；cleanup=0、
无bypass、无解除hold/retention。备份含可恢复payload，明确排除在source删除receipt之外，
不声称全域销毁。已消费旧确认不得重用。

操作准入来源是[#307](https://github.com/qq550723504/data-product-platform/issues/307)：
“任何实际 provider deletion…之前，必须先提交…namespace、key + provider version…清单，收到用户明确确认才运行。”
本轮没有取得该新清单的实际删除确认。
