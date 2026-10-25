# #307 D1：路径边界与未发送 dispatch 收敛

本切片只修复固定完整入口与签名器共享的 Docker 宿主路径边界，完成隔离非删除验收，并可信闭合失败 v27 的 P2/T1/T2。没有新删除候选、新发送 execution、新窗口或调度；旧删除授权不复用。#332 保持 Draft、不合并；#307 保持 OPEN。

## 有限交付契约

- 权威路径转换：`examples/lifecycle-d1/docker_host_paths.py`。完整预检经现有 `reconcile-p1-post-v22.py`、拓扑签名器共同复用，所有 bind source 均经过同一严格边界。
- 闭合事实：`post_v27_no_send.go` 固定失败 execution `6f40bca9-cfe2-4362-ad2b-4e38bbbd8453` 的三个精确 attempt/dispatch/approval。只有对应 v22 精确 revoked、NOT_SENT、released、零 claim 的前序可排除；UNKNOWN、其他 attempt/dispatch、现存 absence 均阻断。
- 使用现有 RevokeApproval / RecordProviderObservation Commands、workspace fence、Audit/Evidence/Outbox，不新增通用 workflow、queue、IAM 或存储实现。路径使用标准库 pathlib；签名继续使用既有 cryptography；备份恢复使用既有 pg_dump/pg_restore。
- 不新增/重跑 Processing、DirectData、Rights、发送或清理；不释放 pins，不删除/退款/重建预算，不删除数据版本、备份或失败证据。
- 当前阻断只限：必需路径不能执行、权限扩大、历史/成本/预算事实丢失、并发或幂等失效。新的产品能力和通用运维平台不纳入本切片。

## 验收矩阵

| 项目 | 本轮结果 | 证据 / 边界 |
|---|---|---|
| Windows / Docker Desktop bind source | PASS | 统一绝对路径及类型检查；四处实际签名器文件读取共同修复 |
| 签名器完整 main 成功 / 拒绝 | PASS | wire、P2、recover-P2 × native/Desktop 路径；22 类拒绝反例；真实 Ed25519 签名验签，Docker/SQL 在该单测中明确为模型 |
| 固定完整预检回归、旧维护封存回归 | PASS | `host-path-repair01/regression03`：normal / `-O` 各 39 方法；之后 canonical SQL 修复另见真实副本验收 |
| 真实 Docker 隔离组合 | PASS | `isolated04`：实际挂载/预算图/PG只读 receipt、三阶段签名验签、目录扩张拒绝、无授权 operator 在 provider 前拒绝；28 CID 全停 |
| 手写 TEMP 表的 69 分支 | 有限模型证据 | 初次通过掩盖了不存在的 observation 字段，不能证明真实 schema 或完整 Command 可执行；保留原回执 |
| 真实 schema 回归 | PASS | `restored-core02` 的 public-schema 测试不建 TEMP、不造列；实际 source consistent dump 恢复完整约束/触发器后验证 |
| 真实恢复副本完整 Command / 同键重放 | PASS | 复用 P2 已撤销事实；新增 2 revoke、3 NOT_SENT、3 release、5 idempotency、8 组 Audit/Evidence/Outbox；三 replay ID 稳定且累计事实不增，旧行多重集保留 |
| 旧 v27 源 Core 三 dispatch 可信闭合 | PASS | `post-v27-business-closure03`：三个精确 attempt 均追加 NOT_SENT + exact release，invoked_at=NULL、claim=0、每份 unresolved=0；2/3/3/5/8 增量与显式部分基线一致 |
| 当前 17 版本全量 source native 核验 | NOT_RUN | v27 窗口的历史完整入口 PASS 原样保留；本切片不把隔离组合或 Core 闭合当新鲜对象库存证明 |
| 六份真实删除 | NOT_RUN | P2/T1/T2/H2A/M1A/M1B 均未在本轮发送；P1 永久排除，11 survivor / 其他环境 / 全部备份保留 |
| 新候选 / 窗口 / 精确删除方案 | NOT_PREPARED | 本轮先完成收敛；没有继续滚动候选或索取窗口授权 |

## 原失败与修复后的真实边界

v27 原终态仍为 `FAILED_TOPOLOGY_ATTESTATION_SEALED_NO_DELETE`，原 terminal SHA256 为 `a98a6016e64c71e4f883ac4dbeff975f794f0135d6aa9a80cc946a4e51a45672`，没有补写为成功。

隔离演练01/02/03分别保留子网池耗尽、PG 初始化临时就绪、演练 initializer 未按真实角色配置 tmpfs 的失败。后续通过明确不相交 internal 子网、正式 TCP ready、实际 initializer descriptor 验收；未清理任何已有网络/卷。旧维护01在资源分配前遇 Windows argv 长度限制，修复为每批64完整CID并校验完整并集。

维护02之前又发现只计成功 IAM 请求会遗漏一条真实 NoSuchUser。门禁现要求13用户、13 policy、1未创建身份共27请求，区分该身份的404与正常200，且以冻结 root owner 的官方 `XMinioAdminNoSuchUser` 验证和 exit0 作为完整证明。旧误签评审的原字节、撤销与后续评审分开保留。

维护02实际运行仍失败：verifier 使用了不存在的 `observation.dispatch_id/copy_id`。只提交 P2 的一条 approval revocation 和对应事务事实，没有 NOT_SENT/release；随后 observer/source 全部封存，旧行/18预算/531配置与备份文件不变。修复沿既有 observation→attempt 关系查询，不添加冗余列或兼容 fallback。真实恢复副本完整 Command 是解除此阻断的依据，不能用 TEMP 模型替代。

维护03只接受新的独立 manifest 与显式部分基线：P2 已撤销，T1/T2 未撤销。其运行二进制必须与恢复副本验收的 `547fc639550ed881f5015dc9b2ca4b5ffe824e11f040f682d545d4c2e4577be4` 完全一致；源 Core 开启后必须等于维护02封存的完整 fingerprint。实际新增计数必须精确为 2/3/3/5/8，不重试维护02，也不抹去那条已提交撤销。

恢复副本签名使用显式测试 oracle / 测试密钥，具有原历史对象身份但不构成真实宿主终止证明、用户删除授权或 source native 验收。它与维护03实际物理停机、新鲜 disabled IAM、签名及 Core 回执严格区分。

完整 D1 尚未验收。未来准备六份新方案之前仍须重新做17版本、保护状态及全部执行前条件核验，并获得新的精确授权；本轮不预约时间。

最终物理读回见 `host-path-repair01/final-physical-and-Core-readback.json`：620 个本轮及原有已知 CID 均 stopped/PID0/noports/norestart，旧18预算、531配置/备份文件和旧维护02的91份文件指纹不变。维护02/03两次实际 root 只读核验共54个技术 GET，start/observation 分别保留；amount=NOT_ASSESSED。`new_provider_Cost=0` 仅指 NOT_SENT 闭合不产生内容删除 provider invocation 成本，不能用于否认这些真实管理请求。
