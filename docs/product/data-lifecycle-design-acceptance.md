# #300 设计验收、实施切片与 Merge Contract

> 状态：设计候选；runtime 用例全部 **NOT_RUN**，下面的拆分是实施建议，未创建后续 GitHub Issues。
> 日期：2026-10-11；设计核查基线 `main@b69433df4aa1b88e03fdfd6166f077c11960248d`。
> [Issue #300](https://github.com/qq550723504/data-product-platform/issues/300) / [Epic #296](https://github.com/qq550723504/data-product-platform/issues/296)。

## 1. Ownership / design preflight

本聊天“设计 #300 保留归档与销毁契约”是 #300 唯一 writer。隔离 worktree 为 `C:\Users\Henry\.codex\worktrees\issue300-lifecycle-design\data-product-platform`，branch `codex/issue300-lifecycle-design`；从 refreshed origin/main 建立，主工作区仍可供原聊天使用。

| 核查 | 2026-10-11 观测 / 决定 |
|---|---|
| AGENTS | 当前文件完整读取，含 §1A 复用、§3/4 冻结/并发、§7 actual attempt Cost、§10 当前交付、§16 pre-production、§17 收敛；本设计不撤销 |
| #296 / #297 / #300 | #296 OPEN，蓝图 Epic；#297 CLOSED，PR #301 MERGED，merge `87b80de64825f600649106449e3ec2fd60b42d1b`；#300 OPEN / docs-only、依赖 #297 已满足，可与 #298 设计并行。#296 body 的部分进度早于 #305，不据其旧 B1/B2/B3 文案重开任务 |
| main / #305 | refreshed `origin/main=b69433df4aa1b88e03fdfd6166f077c11960248d`，PR #305 MERGED 于 2026-10-11；main CI、运行/非开发者验收状态由 #298 原 owner 收尾，本设计不代报 PASS |
| 活跃聊天 | “实现并运行 #298/B1 抽取验证” `01a1287d-6ac0-72f1-a7f3-96a529fdefef` 负责 #298；“识别可并行任务”是父侧规划聊天。未新开其他任务、未恢复暂停聊天 |
| PR / worktree | 开工时唯一 open PR 是 #294 的 Draft #295，`codex/issue294-source-integrity`，保持原 owner。另有 #298 `issue298-b1` / `issue298-pgi09`、IAM/旧文档 worktrees；本任务不写这些目录，不删除/重建它们 |
| 产品与设计权威 | V2 蓝图 + #300；ADR-0014 + Capability Map + ADR-0010；各既有 Dataset/Rights/Certification/Delivery/Evidence/Annotation 专门 contract 的业务语义继续有效 |
| 写入清单 | 仅新增 lifecycle architecture、Storage Reuse Check、ADR-0015 和本文；无需改共享索引，索引修改推迟到明确 owner 协调。#298 接入代码/证据/保留环境及 #294 Annotation 文件均不写 |

跨文件发现的必要演进是 Storage exact-copy contract、当前 reader/copy fence、敏感 Evidence envelope 和 restore 台账，已在专门设计记录为待实施 gap，不以文档补出已有 runtime 能力。原 Annotation contract 要求必需 payload 可重验；本文保留 material pin，不默认可自动清理。

## 2. 按 AGENTS §17 冻结的有限 Merge Contract

**Goal：** 完整定义数据保留/归档/销毁的可实施业务和执行边界，交付决策表、负例及受控合成第一片准入；本候选只交付设计。

| Authoritative source | 唯一职责 |
|---|---|
| [V2 蓝图](full-lifecycle-governance-v2.md) + #300 | 产品范围与设计验收要求 |
| [Lifecycle Contract](../architecture/data-lifecycle.md) | 模型/Command/决策表、范围、历史、并发、外部恢复与敏感正文/备份边界 |
| [ADR-0015](../adr/0015-data-lifecycle-disposition-boundary.md) | 跨模块决定、替代方案与后果；具体规则只链接专门 contract |
| [Storage Reuse Check](../architecture/data-lifecycle-storage-reuse.md) | 仓库 reuse、exact edition/version/license、维护、原生能力和 Adapter 替换边界 |
| 本文 | 用例、实施准入、工作包、检查结果与 review convergence；不复制领域状态机 |

In-scope invariants：历史事实/hash/membership 不覆盖；当前资格与物理事实独立；可信 workspace/principal/批准；保留与保全冲突 fail closed；合法活动/shared/payload pins 不被删除；同一 fence + parent-first 与 current recheck；DB-first actual attempts/unknown/reconciliation/成本；部分与外部覆盖不夸大；敏感 proof/payload 分离先有 contract/migration；恢复不能复活禁用内容。

Non-goals：不新增 runtime/schema/migration、依赖、UI、bucket policy、定时销毁；不执行真实归档/删除/通知/crypto-erasure，不解决司法年限、完整 IAM/灾备/通用工单/合规引擎；不接管 #298/#294/#295/#47/#84/#86/#87；不把全量可靠性或生产 rollout 放成当前 docs-only blocker。

Merge blockers 仅包括：按设计会得到权限/tenant bypass、数据/credential 泄漏；历史/账务可被静默改写或丢失；authoritative contracts 明确冲突；#300 必需内容缺失/路径不可实现或文档检查失败；并发/幂等破坏已声明 invariant。未来性能、扩展 provider、全量 UI/telemetry、可选更强保证和新产品能力归 follow-up，不追求无限文档 hardening。

同根因问题横向修复 primary contract，不逐 comment 制造实体。三轮 substantive review 或 20 条 resolved comments 后执行 checkpoint：冻结 contract、闭包核查、把新意见分 BLOCKER / FOLLOW-UP / INVALID-OR-OUT-OF-SCOPE；只 blocker 重开 scope。后续发布 PR 时按 §17 在 PR 记录 exact HEAD、blocker 列表/0、follow-up；本轮未发布 PR，以下为作者本地审阅，非独立审批。

## 3. 设计交付条件映射

| #300 验收项 | 权威落点 / 本候选交付内容 |
|---|---|
| lifecycle 专门文档 + ADR；owner、状态/命令、最小模型、范围 | Lifecycle §1–4、§6；ADR-0015 |
| Policy × current rights × availability × active refs × hold × capability | Lifecycle §5 D01–D16；UNKNOWN 不放行、USE 与 disposal authority 分离 |
| 有界合成第一片 + 后续拆分 / 真数据审批 | 本文 §5–6，全部 runtime NOT_RUN |
| 保全/删除、派生共享、processing/delivery、restore、partial/unknown、tenant、敏感审计 | 本文 §4 LC-01–LC-28；Lifecycle §6–9 |
| 停止使用 / 归档 / 删除请求 / 验证范围销毁的证据与限制 | Lifecycle §2、§7；LC-01/03/04/14/15/17 |
| storage lifecycle/object lock/retention reuse + license/功能/责任比较 | Reuse Check §1–4，固定 SDK、MinIO CE、Ceph RGW 与 S3 标准，不新建基础设施 |
| 敏感冻结正文与细粒度/密钥删除先有专门模型 migration | Lifecycle §8：单一新 envelope contract、typed payload、历史验证降级、pre-production 重建/真实发现分支；D3 独立实施 |

## 4. 正负验收用例

所有用例是后续可执行 acceptance specification；本轮未对 live 系统运行。fixture 必须使用独立合成对象、固定 clock/authority/copy identity，可控制 pause barriers，不靠随机 sleep 推断竞态。每例报告 exact candidate、输入/配置/registry、DB facts、provider observations/counters、0-byte/0-call 断言及 PASS / FAIL / SKIP / NOT_RUN / UNKNOWN；SKIP 与 UNKNOWN 不计 PASS。

| ID | 设置 / 触发 | 必须观察的结果 | 层级 / 本轮 |
|---|---|---|---|
| LC-01 | 在线合成版本；合法 USE/DELIVER；有明确最小保留 | 新业务读 fresh gate 后成功，reader pin 结束前不能删除；历史认证不变 | Application + live / NOT_RUN |
| LC-02 | 到 use_until，或 revoke identity/source delegation/Rights/Certification；存储仍 ONLINE、可有 hold | 0 新 bytes/job/credential；内容保全，历史 CERTIFIED/ISSUED 保留；rights revoke 不自动准许 delete | Domain/App/PG / NOT_RUN |
| LC-03 | 相同 bytes 受控归档，目标校验通过；归档区可透明 GET | 原 identity/hash/URI/认证不改；追加 locator/availability；普通读不绕 gate；source delete 独立授权 | Adapter/App / NOT_RUN |
| LC-04 | 已盘点 exact versions/copies；期限到、无 pin/hold，有效处置批准 | 先 DB dispatch/start 后 provider；exact receipt/coverage finalize；只有声明 scope VERIFIED_REMOVED，历史内容重算不可用 | PG/real adapter / NOT_RUN |
| LC-05 | 多个 retention 起算/边界；一个未到期或起算未知；再解除一个 hold | 删除 0 调用；保守剩余限制不消失；冲突记录责任人，不自行算法律期限 | Domain/App / NOT_RUN |
| LC-06 | Hold 与 PREPARED→DISPATCHED 用双事务 barrier 竞争，hold 先提交 | 共享 fence 阻止 dispatch，0 delete；批准存在也无效；之后释放须重新 gate | Real PG + adapter / NOT_RUN |
| LC-07 | dispatch/start 先提交，删除响应丢失时到达 hold | 保全请求/冲突可追溯；阻断未 dispatch targets；对在途范围追加 unknown/reconcile/可用 stop，不能声称保全成功或全删 | PG/fault / NOT_RUN |
| LC-08 | 两个版本共享 exact copy；第一版到期，第二版保留/material pin | 0 对共享 copy 的 delete；解除第一 binding 不宣称 bytes 消失；closure/hash 不被软删 | PG/App / NOT_RUN |
| LC-09 | source 有 derived output；source 字节拟删除，但 replay/Annotation integrity 仍需它 | material pin 阻断；批准取消需求前不得删。无 material pin 时独立处理 source，derived history/lineage不级联删除 | Domain/App / NOT_RUN |
| LC-10 | 可独立使用 derived bytes；source bytes absent，后续 source rights revoke | 历史 source identity/rights facts 仍存在，derived current gate 遍历 required inputs 拒绝；历史 snapshot 不放大 current rights | App / NOT_RUN |
| LC-11 | processing 真正读取先提交 pin，再申请 delete；反向顺序也跑 | 先读：DRAINING 等完成/确认取消；先处置：queued/start/retry/recovery 0 新读；不删除活跃输入 | PG/worker/fault / NOT_RUN |
| LC-12 | direct-data 先 ISSUED 再慢 stream，过程中 delete；另在 ISSUED commit 前暂停 | transfer pin 护住 bytes；commit 前 0 bytes；结束追加 observation 不改 ISSUED；same key replay 无 payload | PG/HTTP / NOT_RUN |
| LC-13 | reader/worker 崩溃或 lease 超时；credential 尚活跃或 revoke unknown；已知 future use_until | 不把到期/进程旧心跳当 quiescence；generation fence/confirmed termination 或 expiry/containment 后才解 pin；新 issuance/replay cap 不越过 lifecycle 使用截止 | Fault/App / NOT_RUN |
| LC-14 | 三份 copies 一份删除、一份 provider timeout、一份 retention blocked | PARTIAL/UNKNOWN + exact per-copy facts；不能 COMPLETED 全覆盖或覆盖首个 unknown；历史 Cost 完整 | Adapter/PG / NOT_RUN |
| LC-15 | DELETE 返回 200、bare-key HEAD 404；仍有 noncurrent version/replica | 不标 VERIFIED；无 LIST 权限/403 与 absent 区分；缺副本 inventory fail closed | Real adapter / NOT_RUN |
| LC-16 | delete 返回前 crash、worker 重复入队、实际 lookup 后受控 retry | 恢复原 operation/exact version；新真实调用独立 attempt/cost，same observation replay 无新增 invocation；无第二调度 | Worker/PG/fault / NOT_RUN |
| LC-17 | 已交付外部下载；平台通知计划、自报 ACK，但无删除验证 | 仅记录通知/ACK 范围；外部 bytes UNKNOWN，不把 credential revoke 或 ACK 计 verified destruction | Domain/App / NOT_RUN |
| LC-18 | 老备份含后来禁用/已销毁对象及旧 rights/queue/cache | 隔离恢复，重放独立最新台账/watermark，禁止对象不开放；0 新 processing/delivery；记录恢复出的 retained copy | Restore/PG / NOT_RUN |
| LC-19 | 最新处置台账丢失/重放有 gap；或 restored hash 错配 | 拒绝开放与接纳；不能凭备份旧 CERTIFIED 继续服务；UNKNOWN 不是 PASS | Restore/App / NOT_RUN |
| LC-20 | 允许恢复同 bytes 归档内容，当前 rights/profile disposition 仍有效 | 新 copy/hash 验证 + fenced admission 后 ONLINE；每次 use fresh gate；纯迁移不强造新版本/认证 | App/adapter / NOT_RUN |
| LC-21 | copy/restore/output finalize 与 delete reservation 竞争 | pending copy 在闭包内；restriction 后不能接纳 ONLINE/READY 逃逸；quarantine 也在 inventory、成本留存 | PG/worker / NOT_RUN |
| LC-22 | 跨 workspace plan/copy/policy/批准或任意 body consumer/root credential | 拒绝，0 provider call/bytes；不泄露其他 tenant binding/hold；未来共享 resolver 仅出阻断，不出租户详情 | Domain/HTTP/adapter / NOT_RUN |
| LC-23 | plan/coverage finalize 与 late member writer 竞争 | 同 parent-first lock；只一个完整 immutable set，旧 txn 不越过冻结；FINALIZED members 变更全拒绝 | Real PG / NOT_RUN |
| LC-24 | 同幂等 key 修改 targets/action/approval；过期批准重试 | fingerprint 冲突/当前 authority 拒绝；同键同语义一个历史事件，不能用旧 approval 再 dispatch | App/PG / NOT_RUN |
| LC-25 | 敏感内容内嵌现有 Evidence/Audit/manifest；要求删除正文 | 当前 mode UNSUPPORTED，guards 仍拒绝修改；先隔离权限/专门 migration，不追加“已擦除”伪事实 | PG/security / NOT_RUN |
| LC-26 | 新专门 envelope/payload 演进后删除可处置正文；请求旧原文重算 | root/membership 不改；ENVELOPE_VERIFIED 与 CONTENT_UNAVAILABLE 明确分开；Audit/Outbox/错误/CI 无正文再复制 | PG/contract / NOT_RUN |
| LC-27 | body 明文/低熵 hash/shared key、多备份；只删除 KMS alias | 不宣称匿名或 crypto-erased；正文 copy/密钥范围未证明即 BLOCKED/UNKNOWN，最小 proof 也要保留依据 | Security/adapter / NOT_RUN |
| LC-28 | 缓存/索引陈旧、provider 自动 lifecycle/replication 绕 Core | 闭合受管访问、generation invalidation；无法证明旁路已关闭则该模式 unsupported；不得开启 bucket 自动删除 | Adapter/live / NOT_RUN |

独立验证优先级：Domain → Application → Adapter contract → 真实 PostgreSQL 锁/约束 → worker/HTTP/垂直切片 → 操作者路径。fixture/fake 不代替真实存储/PG concurrency，旧 #298 smoke 不代替这些用例；非开发者解释验收单列。

## 5. 首个实现切片 D1（建议，尚未授权实施）

D1 的有限业务结果：授权操作者对一个登记的**受控合成版本/内容对象**申请停止新使用、等 reader 排空、进行明确复制归档/受控恢复或 exact-copy 删除，并看到范围化 receipt；hold/共享引用/当前 authority 拒绝与 unknown 恢复能够解释。它验证契约，不接管全仓生命周期。

输入固定一个 workspace，两个已登记合成 namespace，少量小文件与已知 provider version IDs；人工固定短 policy/hold、一个合法 reader 和一个同 workspace shared-binding 负例。禁止用户任意 URI/key/prefix、裸 SQL、root、bypass、公开 provider endpoint。first action target registry 只能包含该切片 fixture，既有 #298/#294/历史 Pilot 对象不在 allowlist；不把新增 generic PATCH 当实现。

实施须交付 Domain rules、专门 migration/repository、显式受控 Command/必要 HTTP、当前可信 principal/批准、workspace fence/parent-first、读/复制 pending pin、immutable plan/coverage/attempt/observation、Domain Event/Audit/Evidence/Outbox 与 typed CostAllocation、existing worker 对账、adapter contract/真实 PG tests。复用成熟 storage 原语、已有调度，不建新的 scheduler、policy DSL、审批 SaaS 或存储 engine。

第一片只准通过 fixture 的封闭数据面路径使用已有 processing/delivery gate；要让任何现有读取路径支持 fixture，必须在同片接入 pin/current recheck 或显式禁用该入口，不能仍有未纳管的直读后宣称安全。D1 可以使用受控复制形成归档 copy，不承诺真正冷 tier 或 provider 灾备删除。敏感正文、Annotation sealed payload、备份介质和外部 recipient copy 不纳入删除支持，但 inventory/report 中必须明确排除。

D1 required acceptance：LC-01–08、LC-11–16、LC-21–24、LC-28；LC-03/20 为 scoped copy/restore；没有范围内备份即可不支持其实际清除，但须执行 LC-18/19 的隔离 replay/未知台账拒绝用例，防止试验 restore 绕 gate。real provider retention/hold + exact delete 不能以 fake 替代。输出 exact candidate、provider binary/image digest/SDK、fixtures、DB/对象请求 counters、故障矩阵、coverage receipt；操作者能区别“已限制访问”“已归档”“已请求”“范围内已验证”“外部未确认”。

D1 不在本轮实现；创建隔离存储、运行 destructive synthetic fixture、增加代码/迁移及其 PR 是独立获准任务。首次 destructive 运行前由用户批准具体 fixture/project/namespace/版本清单和 cleanup，不推导为可清现存 demo 数据。不能对 #298 的 retained volumes 执行 down -v 或修改现有 delete-deny。

## 6. 后续工作包与 dependency order（未创建 Issues）

| 建议包 | 前置 / 单 writer 写域 | 有限结果 / 专属验收 |
|---|---|---|
| D1 受控合成 operation + 存储 contract | #300 设计收口、环境和操作授权；Lifecycle/Storage/fixture paths | §5 的封闭 vertical slice，先证明安全/恢复，不宣传生产归档 |
| D2 现有 processing/delivery/共享 content 接入 | D1；与原 Domain owner 串行协调，先核对 active worktrees/最新 main/锁序 | 完整真实读取入口、closure/inventory、pending copies、capability containment。LC-09–13/21/22/28；跨 workspace 共享另加 trusted global-parent contract，不在首片偷加 |
| D3 Evidence/Audit 敏感 envelope + 内容处置 | #300 §8 专门 contract、Evidence/Audit owner、独立 migration/环境授权；可与 D2 设计协调但不重叠写入 | strict 新格式、typed payload、所有事件/诊断副本最小化、body erase/integrity reporting；LC-25–27。现有内嵌正文不在 D1 可删范围 |
| D4 备份恢复与受控重新入库 | D1 + D2 实际纳管路径；D3 涉及敏感台账时先完成其最小化 | independent watermark/ledger replay、隔离开放、cache/index rebuild、备份范围报告；LC-18–21/28。不一次建设完整灾备平台 |
| D5 Annotation/隔离 payload、外部 engine copies | D2/D3；#294/#295 的来源接纳收尾与跨仓 authority 满足后，由原 Annotation owner协调 | 尊重 snapshot/payload/material pins，专门合同批准失去可重验性的范围；LC-09/14/25/26，不恢复暂停联调 |
| D6 多副本/tiering/外部接收方与真实 rollout | 上述实际支持片通过；provider/备份/接收方 authority、维护与预算另准入 | 每 provider scope 的 proof/未知报告、组织 policy/hold/审批；LC-14–17/27/28。不保证不可控二次传播消失 |

不创建这些远端 Issues、不向外部发通知、不启动其他聊天。实现时先检查已有 open Issue/Port 能否承接，避免平行立项；每个 substantial PR 重新冻结自己的有限 contract，不把所有后续都塞入 D1。

真实数据 / 生产门槛：组织已批准 retention/basis/起算/hold 和处置责任；完整受控数据面及可信 principal/delegation；inventory/material pins/备份/外部范围；exact provider build/license/SBOM/维护与 capability 验证；敏感 Evidence/Audit 专门方案；试运行 dry-run 报告与 exact targets/irreversible impact 批准；执行、部署、付费服务与通知各有明确授权。未知任何 required 边界都 fail closed，不因设计/CI PASS 自动开放。

## 7. 作者 closure audit 与检查记录

本候选自审的有限 closure：

| 检查主题 | 结论 / 范围 |
|---|---|
| source-of-truth / ownership | 产品、ADR、专门 lifecycle、reuse、验收职责独立；原 Rights/Delivery/Annotation contract 不被替换；仅四个新文档 |
| 状态/Command | 请求/批准/dispatch 与验证完成分开；UNKNOWN/PARTIAL、hold late conflict、active reader 与 current projection 明确；没有新增运行枚举 |
| 历史不可变/敏感证明 | 不覆盖 hash/membership/认证/发布；正文演进先有 migration；旧内嵌正文仍不支持；hash/最小 proof 不自动匿名或永久 |
| 幂等/成本 | operation replay 与 physical attempt 分开；start/首次 unknown/后续 resolution append-only；实际查证/失败/重试留成本 |
| 并发/线性化 | 共用 fence、parent-first、durable pin、copy reservation；current recheck；late hold 的外部不可逆缺口不作虚假承诺 |
| 安全/恢复 | 可信 workspace/authority 与独立 disposal basis；备份最新台账失联拒绝开放；无法覆盖的外部副本/介质明确 |
| required 实现路径 | 成熟 provider 原语可通过 Adapter 验证；缺能力 mode unsupported；D1 有封闭 fixture/准入与测试，不要求生产全域销毁 |

本地文档检查：UTF-8/相对文件链接、内部章节锚点、Markdown 表格结构、D01–D16 和 LC-01–LC-28 完整/唯一、changed-path scope、`git diff --check`。2026-10-11 静态检查 PASS：四个新 Markdown、42 个本地相对链接、17 个表格块、16 条决策与 28 个验收用例；相对链接没有显式 `#anchor`，章节引用另按 primary contract 自审。正式结果以最终本地候选检查为准。官方源版本/许可证/维护状态已在线核对，provider runtime 能力仍 NOT_RUN。

没有运行 Go/数据库/浏览器/生命周期删除测试，docs-only 不计功能 PASS。没有远端 candidate CI、独立 reviewer approval、PR 发布、merge/main CI、部署或 Issue 关闭记录。本候选作者 closure 未发现本范围 blocker；这不是后续代码/真实处置准入批准。
