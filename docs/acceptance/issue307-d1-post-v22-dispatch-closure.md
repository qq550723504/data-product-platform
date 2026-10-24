# #307 D1 旧 business dispatch 的有限闭合合同

本切片只处理已终止 v22 execution `8e41cfba-a41e-44dd-9ce8-d6365b3bb90e` 的 P2/T1/T2；不是重新授权或继续 v22。#323 Draft、不合并；#307 OPEN；DELETE、cleanup、hold-release、bypass 均为 0。P1 已有真实 ACCEPTED 和新的 absence/completion，永久排除本切片和未来删除目标。

权威输入为原 v22 冻结 candidate/bindings、原 `window01/remaining-business-regression.json`、P2 原预算及 wire 原始证据、P1 audit04 的最终封存回执，以及本合同。旧记录都是历史输入，不能代替当前 Core、进程、身份或存储检查。

| case | plan | copy | 原 dispatch | 原 attempt |
| --- | --- | --- | --- | --- |
| P2 | bce1b1b1-d6e2-44fd-8b87-171142350ab9 | 6fabf3b5-76b8-4245-8f5d-01e64f610146 | 670f5351-103f-472f-aeea-1f29213eda7c | 73d9251e-a128-47b1-8f5a-eaa35603c233 |
| T1 | 247a2ca9-d2f3-4384-bcee-b10add497175 | f48755f1-976b-4f47-98da-70ead936a4c3 | 134315c3-bad8-4fda-a927-3ef71e2cca5e | bb6ecab6-53f1-4a9c-a333-f0cf6f4edd4a |
| T2 | 531c7b96-173f-4028-be97-2de9ed6c72f2 | e09fd433-54cf-4e82-b8c4-098472505fa2 | b9ebd875-8942-4303-8e17-41612dd8100c | 933462ae-04d6-4da4-97de-067b62863e0a |

三个身份均属于 business workspace `a4aa2689-6e8f-4180-a567-dfb875407eac`、provider `cc068c8c-4f00-44cd-a825-377631253d59`、incarnation `d5f220ca-6133-42a0-a31c-1cf717d05274`。不接受输入任意 attempt/copy，也不按 latest 推断。

先运行固定 `audit-old-business-dispatches.py`：确认旧 61 CID 和 P1 各次审计新增 CID 均停止、旧 46 个控制文件及 8 个新只读 profile 仍关闭、三组旧预算和九备份不变、三个 gateway 仍 sealed。仅启动绑定的 business Core，使用 REPEATABLE READ READ ONLY 查询实际 attempts、dispatches、approvals/revocations、claims、observations、releases、absence、pins 和当前 unresolved count。比较整个 Core 的前后 relation/roles/ACL 指纹，再在 finally 停止同一 Core 并独立读回 PID0。不启动 Native、IAM helper、case、wire、sender 或队列；不签名、不写业务事实。查询结果即使无冲突也只能记为“Core 无冲突，尚未可信 NOT_SENT”。

实际闭合另需新的有限 observer 配置和独立评审，不能用只读审计结果直接写 NOT_SENT。可信宿主在签名之前逐项取得新鲜 owner/PID0/no-restart、所有旧权限关闭、完整 IAM disabled/policy 语义、sealed gateway 和精确日志/预算证据；签名绑定新 audit ID、原 execution/candidate/bindings、三个完整身份、源 Core、全部 owner/binary 和证据摘要，最多一分钟。host 私钥不挂 worker；proof 缺失、过期、范围扩张或运行中的 enable owner 均拒绝。P2 的旧 claim 错误已丢失，不能补造，也不能仅凭“没有成本/预算/wire 文件”推断 NOT_SENT。

闭合复用现有 `DispositionService.RecordProviderObservation`（`CapabilityObserveAttempt`）、事务内 fence→plan→dispatch→attempt 锁及 Audit/Evidence/Outbox。旧批准先以现有 RevokeDispositionApproval 追加撤销事实（如尚未撤销）；不修改原 approval。verifier 在同一锁内重新核对批准来源、原 attempt/dispatch/copy/plan/hash 及无任何 claim、observation、release、absence；有冲突即停，不覆盖 UNKNOWN。仅追加签名 NOT_SENT 和既有 dispatch release，不修改旧 attempt，不产生虚构 provider Cost，不释放旧 reader pins、不退预算。三项各自保留成功或失败事实；同 key replay 不追加，不能为补齐三项而回滚已经真实提交的事实。

非删除验收包括：签名/时效/身份/owner 扩张拒绝；实际 PostgreSQL 上旧 unresolved admission 阻断、可信闭合后新 admission 正例；claim/UNKNOWN/absence/未撤销批准反例；replay 与原行/预算/pins/成本保留。SDK 只用 loopback，不触及保留原生源。离线、PG、宿主与原生结果分别报告，独立评审不能替代实际执行。

三项可信闭合后，再运行完整 17 版本只读入口：六目标与十一 survivor 的实际 bytes/SHA/库存/保护，P1 absence 独立绑定，三源 Core、完整 disabled IAM、九备份、owner/topology/预算全部刷新。不得把旧 18 版本入口投影成 17 版本 PASS。全部通过后才冻结新的六目标 execution/source/binary/Core/IAM/gateway 及新空预算，更新矩阵和精确确认文本；窗口与精确授权仍须另定，不复用旧授权。
