# #307 封闭 native reader 验收

代码候选：`283eba050c96ac4aa70990339e3fa25dea6068e1`；main 基线：`1892cd76c4ee578896ddfb3c6f4a7252bfacc51b`。本轮只做合成 exact-version GET 和 metadata 复核，DELETE / COPY / 解除保护 / bypass / 对象或 volume 清理均为 **0**。没有 push、PR、合并或 main CI；#307 保持 OPEN。

Native binary 从当前已提交代码构建；[逐文件 SHA256 与已提交 Git blob 绑定](reader-source-binding.json)证明实际验收的七个实现/SQL文件与上述代码候选完全一致。没有把旧 HEAD、旧环境或 PR CI 当作当前验收。

## 实际结果

- 三次 native GET：进程读取后退出95、一次 HTTP 完整读取、一次 HTTP 输出取消。每次都是注册 copy 的 exact key/version；先提交 durable pin，再在共享 fence 下重核当前 caller、restriction、window、reservation 与 pin generation，持久化 physical start 后才发 GET。实际27 bytes/SHA256与冻结 DatasetVersion 匹配。
- 请求取消时，阻塞中的实际输出 owner 尚未返回，active pins 仍为2；owner返回后只追加该次 `CANCELLATION_CONFIRMED`，active pins减为1。正常读取另有一份 `COMPLETED`。崩溃进程的 pin 没有 termination，保留为未确认；lease/进程退出不是排空证明。HTTP writer设置pin/use_until deadline，并同步完成实际Flush后才返回给termination流程；Write仅写入缓冲不能释放pin。
- 完整读取 replay 与崩溃 replay 都返回409，0新 native I/O、0内容 bytes。错 token、foreign copy、已限制B、任意URI字段、legacy download、production delivery route均拒绝。A在读取途中追加限制后，新读403且0新 I/O。
- 全阶段31 physical calls / 31 observations / 31 typed CostAllocations一一对应；其中READ_PINNED为3/3/3。Reader admitted3、terminated2、call started31/observed31各自与 Audit/Evidence/Outbox数量一致。管理员IAM创建/禁用另有基础设施 receipts，不冒充业务CostEvent。

证据：[native HTTP结果](reader-acceptance.json)、[仍在输出](reader-output-active.json)、[取消尚未停止](reader-cancel-before-stop.json)、[崩溃 owner](reader-crash.json)、[数据库不可变事实](reader-durable-facts.json)、[Audit/Evidence](reader-audit-evidence.json)、[独立离线核对](reader-independent-validation-final.json)。

## 环境及保留清单

环境 **dpp-lifecycle307-34696798**；run `34696798-06e3-42de-8b31-6b237c6e9c22`，workspace `8d26375d-fd97-487b-a3fe-68c70821865c`。独立 internal network `10.234.31.0/24`；PG、MinIO、gateway均无宿主端口。HTTP仅在one-shot helper内短暂监听loopback，未部署平台API或processing/delivery路径。

| namespace | exact key | exact version | 原保护 / 当前结果 |
|---|---|---|---|
| dpp-lifecycle307-online | fixture/a.csv | 39ef4e93-485d-40ee-b305-7d2ba2753cbb | NONE/OFF；bytes已读验；当前use已限制、崩溃pin保留 |
| dpp-lifecycle307-online | fixture/b.csv | 7d49c000-c013-4b1a-ac50-5a816254feba | LegalHold ON保持原样；当前use阻断，0 payload GET |
| dpp-lifecycle307-quarantine | fixture/a.csv | 2c583862-3d9f-4ae3-bb35-df87b8ae9728 | GOVERNANCE及retain_until保持原样；0 payload GET |
| dpp-lifecycle307-quarantine | fixture/b.csv | bc942d0f-314e-4ced-bd41-fc669b1b740b | NONE/OFF保持原样；当前use阻断，0 payload GET |

这张表是保留/验收清单，**不是新增删除批准**。本环境没有处置approval/dispatch/provider attempt。未确认崩溃pin仍阻断排空；不能通过到期、重启或伪造停止receipt释放。

[原始manifest](manifest.json)与[最终exact HEAD/保护/完整版本库存](reader-originals-final.json)一致，四版本全部保留。新10个容器（3服务、7退出helpers）和两个named volumes全保留。此前63个容器的ID/image/state/mount及17个volumes identity均保持原样，旧环境没有storage I/O：[before](preservation-before.json)、[after](reader-preservation-after.json)、[新容器](reader-final-containers.json)。

本轮使用第一次reader准备时重建并固定的fixture image。相同MinIO binary SHA256为 `fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8`，固定Alpine基础与fixture image `sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6`。第一次镜像恢复的只读二进制提取与失败准备目录保留，见前一环境报告；此轮不再次读取旧容器。

[Reader原生IAM](reader-iam-policy.json)只授予四个固定key各自 exact version 的 `GetObjectVersion`；gateway payload仅放行online/A的该version和独立reader身份。Reader mount没有root credentials。验收后native reader身份禁用、两个reader窗口关闭，gateway payload/admin/DELETE/COPY全部拒绝，metadata只保留inspection身份：[封存证明](reader-seal.json)。

## 复用、验证与边界

复用已有 Occupancy/Copy fixture policy、PostgreSQL transaction/fence、StaticResolver、MinIO OSS signer、single-send storage transport、physical-call journal、CostAllocation与Audit/Evidence/Outbox。HTTP使用Go标准库；没有第二套存储、durable workflow、IAM、queue或通用传输runtime。新增能力仅是平台pin/实际read-owner-stop语义；相较另引Temporal/通用streaming sidecar，现有infra已满足这个有限合成切片。128KiB以内缓冲只用于fixture；没有新造resumable upload或生产数据下载系统。

未发布migration仍为60–64；当前64补充typed `reader_pin_id` FK、active owner guard和one-pin/one-native-call唯一约束，没有占用C2的65或共享routing注册文件。新保留数据库 `upgrade_main59_reader64` / `fresh_reader64`均通过；max64/count61（原历史编号存在gap），旧接受数据库未升级：[迁移证明](reader-migration-proofs.json)。Native fresh64与上述SQL SHA256一致。

| 检查 | 结果 / 边界 |
|---|---|
| 既有迁移/COPY/worker真实PostgreSQL回归 | PASS；application149.588s（flush修正前，相关COPY/worker路径未变） |
| 最终reader/caller/flush/HTTP/Adapter真实PG专项 | PASS；与最终race检查同一轮，包含新增buffered-output回归 |
| reader及相关HTTP/Adapter `-race` | PASS；application21.896s |
| 全Go模块 | PASS；86包ok、45无测试；未配PG的套件不冒充真实PG验收 |
| scoped `go vet`、diff check | PASS |
| 独立证据、credential scan、native source hashes | PASS |
| LC-01/02/13 | 当前fixture reader范围PASS；不是全平台读取承诺 |
| LC-11实际processing接入、LC-12生产direct delivery共事务pin | NOT_RUN |
| LC-28全平台读取closure、完整D1、生产Rights/Delivery/灾备 | NOT_RUN |

完整日志见 [检查汇总](reader-test-results.json)及同目录tests.log。当前平台通用 `storage.Get` 和现有 processing/delivery Commands尚未接lifecycle pin；本封闭环境显式没有这些入口。这是后续真实接入必须解决的架构边界，不能在SDK内偷偷放行URI、用fixture policy替代CurrentDeliveryGate，或据此关闭#307。任何新删除仍须先提交新环境/exact key/version清单、得到用户确认并重新通过当前门禁。
