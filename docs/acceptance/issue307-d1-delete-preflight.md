# #307 D1：5d2274ee 非删除准备阶段

2026-10-14，接续[精确删除方案](issue307-d1-delete-proposal.md)。
本文记录历史候选/Core generation01阶段 **PASS_READ_ONLY_PREPARATION / PARTIAL_PREDELETE_READINESS**。
当前原生六版本恢复、server no-send已完成，见[新非删除阶段](issue307-d1-reference-preparation.md)；
以下NOT_RUN/候选未应用/52calls/1377rows均保留其原时点语义。
#318 保持 Draft，未收到新的 exact-list 删除确认，真实 DELETE / provider I/O=0。
没有创建 enabled delete permission、没有追加 NOT_SENT 或释放原 reservation。

## 固定 profile 与 no-send 候选证明

[prepare-reference-delete.py](../../examples/lifecycle-d1/prepare-reference-delete.py)
只支持只读 preparation，不存在 execute/provision/enable/release CLI。
它固定 project/run/workspace/provider/incarnation、六份 registry identity、两份目标、
plan/hash 及未来 execution stage614cd9ad；生成的 profile permission_enabled=false。
它不修改旧已消费 driver，也不提供通用 key/URI/prefix 或实际删除入口。

最新回执为
[02 preflight profile](evidence/issue307-d1/delete-preflight-5d2274ee-02/preflight-profile.json)、
[02 no-send candidate](evidence/issue307-d1/delete-preflight-5d2274ee-02/trial-no-send-candidate.json)、
[02 current READ ONLY Core](evidence/issue307-d1/delete-preflight-5d2274ee-02/current-core-readback.json)。
最初候选阶段也保留；在后续 source audit 中，manifest/SQL 的信任根被收紧为
固定 fc009d88 / 917465d8 canonical Git blobs，不能用工作文件中的 manifest entries
或改写 SQL 重新定义证明。02 是完成此修正后的重新只读执行，没有覆盖 01。

正向依据：原 helper ID/image/入口、开始结束时间、未重启且 exit0，
root-free / readonly 精确 mount、无 ambient credential，固定 entry script 和 reader binary，
48 项 frozen native receipts、685 项 historical source blobs，以及不授予 DELETE 的
reader/producer policies。当前 SQL 重新核对原两份 plan/dispatch/attempt、无 claim/observation、
7 pins/7 stops、active0、pending-copy/output closure。不是仅凭“没有 send claim”作判断。

候选状态 **VERIFIED_CANDIDATE_NOT_APPLIED**：可信 server verifier 仍须在现有
fence/attempt locks 下重新核对当前 no-send、原 executor 及 exact attempt scope，再调用
`RecordProviderObservation` 追加 NOT_SENT/release/Event/Audit/Evidence/Outbox。
此 transaction/composition bridge 和真实发送 profile 尚未实现/验收；不能把 JSON 候选直接
插入数据库、当作当前 disposal approval，或复用旧试验 attempt。

[安全测试](../../examples/lifecycle-d1/test_reference_delete_preflight.py) 的六个 test methods
检查 known closed trial、changed/foreign/duplicate identity、保护/权限扩张、运行/重启/不同入口、
root/额外 mount/ambient credentials、changed approval/attempt、claim/observation、active pin、
pending output、改写 manifest/SQL，以及 CLI execute/confirmation/release 拒绝。
普通与 `python -O` 模式均通过；已接入既有 demo-lifecycle required CI 的纯本地安全检查，
测试不调用 provider、Docker 或真实 PostgreSQL，不冒充 native/PG 并发证明。

## Core checkpoint 与独立恢复

[checkpoint-reference-core.ps1](../../examples/lifecycle-d1/checkpoint-reference-core.ps1)
复用 PostgreSQL pg_dump/pg_dumpall/pg_restore 和现有
`Assert-RetainedServiceStorage`。只对源 Core 做 SELECT/export；不停止、重启或重初始化源 Core。
backup/role dump/密码/原诊断全部位于私有目录，不进入 Git。

[Core checkpoint](evidence/issue307-d1/delete-preflight-5d2274ee/core-checkpoint.json)：
194 个 public base relations、1377 rows，独立 restore 的逐表 SHA256 fingerprint 与源完全一致，
源在 export 前后 fingerprint 不变。fixture role/SCRAM verifier 与 object owner/ACL 一致。
[真实远程 SCRAM 验证](evidence/issue307-d1/delete-preflight-5d2274ee/core-checkpoint-auth.json)
通过合法密码连接与错误密码拒绝两个真实容器路径；不是仅比较 hash。

| 新保留资源 | 精确 identity / 状态 |
| --- | --- |
| Core replica/container/volume | `dpp-lifecycle307-5d2274ee-predelete-postgres`；container `b57c6b44e63d6ccb797b2f8f82b885d8dc5601307bc0984f82306e08b79267b6`；专属同名 named volume；STOPPED_RETAINED |
| Replica image | `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea` |
| 网络 | `dpp-lifecycle307-5d2274ee-predelete-private`，internal `10.234.152.0/24`，ID `c3fdcbaf092e77b790aed53de1dec98deb2911987e21d86b812dc6f86067cc10`；无 host ports |
| Auth positive helper | `dpp-lifecycle307-5d2274ee-predelete-core-auth-positive`，exit0，保留 |
| Auth negative helper | `dpp-lifecycle307-5d2274ee-predelete-core-auth-negative`，exit2 为预期 password rejection，保留 |
| 私有有效检查点 | 原 private root 的 `predelete-core-checkpoint-03`；dump 和 credentials 保留 |

首次 dump/restore 前未额外捕获脚本 hash，此限制明确保留。
其后最终代码执行
[独立 READ ONLY 复核](evidence/issue307-d1/delete-preflight-5d2274ee/core-checkpoint-final-readback.json)，
再次证明当前源/replica 194/1377 全部 fingerprints 匹配原 checkpoint，原 dump SHA256 不变；
该阶段记录实际 working script SHA256 与运行中未变。不能把最后 verification 的 source hash
冒充最初 dump/restore 也从该最终源码重跑过。

三项 source 服务、两份 source named volumes 和原 windows 均保持原状态；
source provider calls/observations/typed costs 仍52/52/52，原 trial observations/releases=0。
所有新旧 resources/private directories 保留；cleanup=0，没有恢复副本的业务 admission。

## 保留失败与下一项实际条件

[失败记录](evidence/issue307-d1/delete-preflight-5d2274ee/core-checkpoint-preparation-failures.json)：
PowerShell native stdout redirection 错误、同目录 replay 拒绝、Windows SQL 参数长度上限。
前两次只产生空私有目录，没有 dump/credential/service/volume/network 或 source mutation。
修正为显式 redirected ProcessStartInfo + SQL stdin 后，用明确 attempt03 完成 checkpoint；
前面的空目录与 FAIL/BLOCKED 回执保留，未清理或覆盖。

**Native 六份对象备份/恢复仍 NOT_RUN**；本 Core checkpoint 不冒充完整 pre-delete backup，
也不恢复原 LOST C2 Core。原 sealed MinIO 未调用，retention/hold 不改。
下一项必需条件仍包括：六份 exact object native backup/restore、服务端 no-send verifier/
transaction bridge、新短期 human-confirmed disposal basis/approval 与实际发送/recovery composition，
以及每次 dispatch/send 的 fresh current gate。任一未知或缺失都0 DELETE。
完整 D1 / native pre-ISSUED pause / 剩余 worker faults / archive-restore rerun 仍不计接受，
#307 不自动关闭，#318 不 Ready/merge/deploy。

## 独立增量评审与证据绑定

[独立评审原文](evidence/issue307-d1/delete-preflight-5d2274ee-02/preflight-independent-review.json)
固定 parent917465d8 下的18份 staged Git blobs；未发现本非删除准备阶段的可验证 BLOCKER。
reviewer 只审查源码和保存证据，没有运行测试、SQL/runtime/provider，
没有独立重证 live restore/SCRAM。native 对象备份、no-send transaction bridge 与发送 composition
仍是未来真实删除的必需条件，未被本评审降级或计为完成。

[新阶段 manifest](evidence/issue307-d1/delete-preflight-5d2274ee-02/preparation-manifest.json)
独立绑定本次提交的源码/文档/回执；原108/48项 native manifest 与原 proposal manifest 保留。
[最终私有值检查](evidence/issue307-d1/delete-preflight-5d2274ee-02/final-known-secret-scan.json)
覆盖 manifest 生成前的 staged 公共文件，只保存计数和结果，不含任何私有值。
