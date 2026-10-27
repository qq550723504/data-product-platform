# 交付记录与证据归档只读接入

## 范围与语义

沿用 aq1、独立 `agent_q2_acceptance_20261024` 数据库、SELECT-only reader 和默认只读预览。保留原 `agent_q2_test` 与 Agent API；不接模型、不加载真实交付服务/执行器，不产生下载链接或凭证。不 merge/deploy。

复用已有 readmodel、principal、分页、历史组件与发布 Trace，不引入新的 IAM、工作流、队列或持久化系统。

重要缺口：仓库没有与原型业务签收/验收通过等价的业务验收聚合或公开查询。`/core/archives` 目前只接现有 EvidenceSnapshot 的冻结档案；明确显示业务签收未接入，不能把证据快照、PUBLISHED 或 ISSUED 当作验收通过。完整业务验收 contract 为后续独立切片。

## API 与页面

- `GET /api/v1/workspaces/{workspaceId}/delivery-history` → `/core/deliveries`。
- `GET /api/v1/workspaces/{workspaceId}/evidence-archives` → `/core/archives`。
- 两者支持唯一 `id` 精确查询及 `limit/offset` 分页；拒绝重复、未知 query key 与非法引用。未找到明确对象返回空集合，不选择其他对象。
- 复用可信 principal 的 workspace/capability 检查；401/403 在访问 repository 前返回。只注册 GET，不挂载交付 Command 或 recovery service。
- count、列表、门禁/迁移子历史在同一 REPEATABLE READ / READ ONLY 事务内读取。稳定顺序为创建时间与 ID；这是当前历史分页，不是冻结整个跨请求分页窗口。
- 交付使用显式字段投影，区分 lifecycle projection、历史 gate evaluation 和 transition。不给出实时授权结论。ISSUED 不证明传输完成或业务签收。
- 不输出 credential/ref/hash、provider request key、idempotency key、principal/consumer 标识、manifest、存储地址或任意 metadata/payload。归档只返回快照 header、成员数及同 workspace 的实际 release binding；完整性结果仍复用发布 Trace。
- 精确交付 → 明确版本/质量/认证、retry_of；归档 → 明确发布 Trace → 原快照归档；刷新不改选最新。
- 新导航纳入生产与交付中心，保留原本地演示入口的数据与行为。

## 独立夹具与权限

`prepare-delivery-history.py` 是一次性、显式 ACK 的测试准备脚本，只允许已知独立数据库/reader profile，已有固定 fixture 时拒绝重跑，绝不 reset 原库。只补充一笔 `SYNTHETIC_HISTORY` 的 BLOCKED operation 和它的 INITIAL/BLOCKED evaluation、PREPARED→BLOCKED transition；不是实际调用 delivery Command 得到的业务结果。没有 provider attempt、credential 或 execution。

为新 reader 增加既有 `delivery_operation`、`delivery_gate_evaluation`、`delivery_transition` 三张表的 SELECT。没有新增 DML、序列、TEMP、函数 EXECUTE 或 schema CREATE 权限；没有 DDL/migration 变化。原库全表内容 fingerprint 在准备前后相同。新库业务行在夹具准备完成后的 API/浏览器窗口内必须不变。

## 验证与证据

- Go readmodel、HTTP 与 preview 包测试通过；HTTP 新增两条路径的无身份、外空间、错误/重复/未知 query、POST 不可达回归。
- 前端 history 回归覆盖明确记录、workspace、分页、非空数组、快照 hash/引用及 archive 返回引用校验。
- 实际独立库 API `tests/agent-task-preview/delivery-history.test.mjs` 3/3 PASS：BLOCKED gate/transition、空页保持 total、精确/不存在查询、归档引用、字段过滤、身份/空间/输入边界、POST405、Agent writeEnabled=false / dispatchEnabled=false。
- 最终隔离构建、浏览器路径与读窗口指纹结论记录在本轮交付和 PR 增量记录；不把上一提交的 CI 结果当作本轮 CI。

本地证据位于 ignored `.data/governance-delivery-*`；reader DSN/password 保持私有 profile，不进入源码或验收输出。

## 本轮最终实测

- 最终隔离 Next 构建 PASS，包含全部现有前端回归、15 项 history 测试、编译/类型检查；新增 `/core/deliveries` 与 `/core/archives` 构建成功。
- Edge 使用已存在的 4188 验收标签页：发布历史入口 → 交付列表 → 明确 BLOCKED operation → 展开 INITIAL gate 与 PREPARED→BLOCKED transition → 刷新保持 → 明确版本与质量 → 浏览器返回保持选择。
- Edge 归档 → 明确发布 Trace / INTEGRITY_OK → GET 重查仍保留 returnArchiveId → 返回同一快照 → 刷新保持。非法引用显示 UNKNOWN；不存在的交付/快照显示未找到，不保留旧主体。
- Edge 偏移25空页仍显示总数1，上一页能回到有记录首页；最终侧栏正确归属数据开发中心并高亮交付/归档入口。
- API 3/3 再次 PASS；Agent authority writeEnabled=false、dispatchEnabled=false。
- `verify-delivery-history.py` 实测 originalUnchanged=true、acceptanceReadWindowUnchanged=true；两库216张 public 表的内容指纹分别与本轮原库准备前、新库夹具准备后的基线一致。
- 最终 Execution=0、AgentTask=0、ActionRequest=0、ProviderAttempt=0、Outbox=0，仅一笔明确标记的 synthetic BLOCKED operation。reader writeTables=0、writeColumns=0、schemaCreate=false、TEMP=false。
- 截图 `.data/governance-delivery-record-edge.png`、`.data/governance-delivery-archive-edge.png` 均已保存并目视检查；构建、API 与无写入证明分别为 `.data/governance-delivery-build-final.log`、`.data/governance-delivery-api-final.log`、`.data/governance-delivery-no-write-proof.json`。

运行来源为本轮工作树源码的独立构建；Git 提交不等于重新发布。预览继续保留，原库、数据库容器、其他浏览器标签均保留。未执行真实认证/发布/交付/签收。尚不能声称业务验收对象已接入或生产就绪。
