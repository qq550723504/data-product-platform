# #307 D1 candidate17/v27 有限准备契约

本切片将已独立通过的索引修复接入新候选，不复活已失败的 v26。唯一新 execution 为 `6f40bca9-cfe2-4362-ad2b-4e38bbbd8453`，固定窗口为 2026-10-21 新加坡 14:00–14:40（UTC 06:00≤now<06:40），只读恢复截止 08:40Z。当前无删除授权、无调度；#332 Draft、不合并，#307 OPEN。

## 唯一契约及复用

领域语义仍由 `docs/architecture/data-lifecycle.md`、ADR-0015 和既有六 slot runner 拥有。新代接线复用现有 MinIO native SDK、官方 policy ParseConfig/Equals、Transactional Outbox→Asynq、原生 archive/restore、固定完整入口和已经受审的封存工具；没有新队列、policy engine 或 storage 实现。stdlib JSON/hash/path 只用于 fixture 证据契约。

编译期 execution/profile/START/END 与生成器共享同一 current contract。六 case 的 CandidateUseUntil=END，六 recovery=END+2h，源业务不 Renew、不重做 Processing/DirectData、P1 永久排除。发送顺序仍为 P2→T1→T2→H2A→M1A→M1B；每份最多一次、全局最多六次，UNKNOWN 不重发。

## 必需不变量

- `retained_v26.py` 固定验证旧工作树实际 76 项批准引用、原用户原文/envelope、失败 terminal、25 项私有 seal-history、86 owner 配置/PID0、三预算归属/完整挂载图/原空状态、九备份和实际 sealed gateway。三源及更早 359 CID 由既有 retained gate 保留；合计旧 445 CID。合法撤销只允许 Base.Enabled 或 Enabled 的 true→false，原字节、撤销记录、disabled digest 与全部其他字段必须一致。旧整体 IAM UNCONFIRMED 历史不改写；新 native 入口另取得当前 fresh disabled 证明。
- 固定 Go IAM 集合新增实际旧 v26 六身份；当前新六身份独立。合计预期 29 个已知 disabled，其中 28 sole policy/no groups、1 固定 partial NONE/no groups；另 9 个未创建名称必须 fresh NoSuchUser。只有本新 execution 的限定身份可用于后续另获授权的启用。
- 当前新准备索引由 writer 直接输出 files-map-v1 的 metadata 映射（status/count/sha256/bytes），freeze 与完整入口复用 `evidence_indexes.py` 严格核验。旧真实 digest-map 历史由其精确旧契约核对，不做格式猜测、转换后冒充原字节或回退。
- source 独立复评、原生非删除准备先完成。冻结 preparation audit plan 绑定真实 candidate SHA、source review 索引/报告及新准备索引。未来人类授权、调度尚不存在，不能伪造其索引；取得之后另按批准 manifest 冻结对应 phase plan，准备快照保持原字节。
- 所有原环境、失败、UNKNOWN、真实 attempt/成本、pins、新旧预算、备份保留。新 owner/卷/网络/预算独立，发送 permission 始终 false。只在隔离 restore 受限 inspector 上执行非删除业务重跑，关闭后 fresh disabled 读回；源 Core fingerprints/roles/ACL 前后一致。

## 验收与独立评审

先完成 normal/-O 全部 Python 回归、Go 受影响的 pilot/reader/checker/operator/root-owner contract tests，再实际执行冷态 retained gate。源码独立复评无有效阻断后才分配新停止资源并执行一次非删除准备、九新备份/独立 restore/rehearsal、freeze、固定完整入口的 17 版本 fresh 核验。最后独立复评冻结 packet、完整入口回执和精确确认文本。模拟测试、实际 provider 只读、PR CI、人类授权分别留证。

只有安全/授权绕过、历史改写/丢失、契约冲突、当前必需路径或 CI 失败、并发/幂等破坏构成本切片阻断。通用 fixture-generation 管理器、未来环境清理、额外 observability、真实发送和完整 D1 业务完成均不在本轮准备实现内。当前逐代固定 fixture 历史不能继续演变为产品兼容层；本轮只解释真实 v26，通用代际重构留 #307 后续，不借它扩大当前数据面。

冻结后输出完整 candidate17/v27 确认文本和验收矩阵。没有新的精确人类授权前不得执行真实 DELETE；cleanup=0、hold-release=0、bypass=0。
