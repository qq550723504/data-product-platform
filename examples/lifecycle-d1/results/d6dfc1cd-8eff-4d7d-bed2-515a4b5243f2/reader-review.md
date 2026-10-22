# #307 封闭 native reader 验收

本页是第一次候选的保留记录。HTTP缓冲/flush边界后来发现不足，已修正并在[另一个新环境重验](../34696798-06e3-42de-8b31-6b237c6e9c22/reader-review.md)；当前输出闭合结论见新报告，末尾保留更正说明。

代码候选：`b5d05bd786e8585427a6be78883636c75aa250c4`；main 基线：`1892cd76c4ee578896ddfb3c6f4a7252bfacc51b`。本轮只做合成 exact-version GET 和 metadata 复核，DELETE / COPY / 解除保护 / bypass / 对象或 volume 清理均为 **0**。没有 push、PR、合并或 main CI；#307 保持 OPEN。

Native binary 在提交前从 working tree 构建；[逐文件 SHA256 与已提交 Git blob 绑定](reader-source-binding.json)证明实际验收的七个实现/SQL文件与上述代码候选完全一致。没有把旧 HEAD、旧环境或 PR CI 当作当前验收。

## 实际结果

- 三次 native GET：进程读取后退出95、一次 HTTP 完整读取、一次 HTTP 输出取消。每次都是注册 copy 的 exact key/version；先提交 durable pin，再在共享 fence 下重核当前 caller、restriction、window、reservation 与 pin generation，持久化 physical start 后才发 GET。实际27 bytes/SHA256与冻结 DatasetVersion 匹配。
- 请求取消时，阻塞中的实际输出 owner 尚未返回，active pins 仍为2；owner返回后只追加该次 `CANCELLATION_CONFIRMED`，active pins减为1。正常读取另有一份 `COMPLETED`。崩溃进程的 pin 没有 termination，保留为未确认；lease/进程退出不是排空证明。
- 完整读取 replay 与崩溃 replay 都返回409，0新 native I/O、0内容 bytes。错 token、foreign copy、已限制B、任意URI字段、legacy download、production delivery route均拒绝。A在读取途中追加限制后，新读403且0新 I/O。
- 全阶段31 physical calls / 31 observations / 31 typed CostAllocations一一对应；其中READ_PINNED为3/3/3。Reader admitted3、terminated2、call started31/observed31各自与 Audit/Evidence/Outbox数量一致。管理员IAM创建/禁用另有基础设施 receipts，不冒充业务CostEvent。

证据：[native HTTP结果](reader-acceptance.json)、[仍在输出](reader-output-active.json)、[取消尚未停止](reader-cancel-before-stop.json)、[崩溃 owner](reader-crash.json)、[数据库不可变事实](reader-durable-facts.json)、[Audit/Evidence](reader-audit-evidence.json)、[独立离线核对](reader-independent-validation.json)。

## 环境及保留清单

环境 **dpp-lifecycle307-d6dfc1cd**；run `d6dfc1cd-8eff-4d7d-bed2-515a4b5243f2`，workspace `fe565115-71dd-48f7-a0cc-8fdaf6e8b11e`。独立 internal network `10.234.30.0/24`；PG、MinIO、gateway均无宿主端口。HTTP仅在one-shot helper内短暂监听loopback，未部署平台API或processing/delivery路径。

| namespace | exact key | exact version | 原保护 / 当前结果 |
|---|---|---|---|
| dpp-lifecycle307-online | fixture/a.csv | 7ced7b6f-daf5-4020-bebd-b20d53d1be3c | NONE/OFF；bytes已读验；当前use已限制、崩溃pin保留 |
| dpp-lifecycle307-online | fixture/b.csv | bdd4b29a-e534-42dd-8377-9bdea97fb0ed | LegalHold ON保持原样；当前use阻断，0 payload GET |
| dpp-lifecycle307-quarantine | fixture/a.csv | 2f8c5688-0013-4be3-9a1d-0fb2dc78690e | GOVERNANCE及retain_until保持原样；0 payload GET |
| dpp-lifecycle307-quarantine | fixture/b.csv | e3a5b758-c525-4004-85f2-b9089ce6cd78 | NONE/OFF保持原样；当前use阻断，0 payload GET |

这张表是保留/验收清单，**不是新增删除批准**。本环境没有处置approval/dispatch/provider attempt。未确认崩溃pin仍阻断排空；不能通过到期、重启或伪造停止receipt释放。

[原始manifest](manifest.json)与[最终exact HEAD/保护/完整版本库存](reader-originals-final.json)一致，四版本全部保留。新10个容器（3服务、7退出helpers）和两个named volumes全保留。此前53个容器的ID/image/state/mount及15个volumes identity均保持原样，旧环境没有storage I/O：[before](preservation-before.json)、[after](reader-preservation-after.json)、[新容器](reader-final-containers.json)。

旧受验MinIO image ID在本地image store不可创建，准备在创建新资源前拒绝。随后只读提取保留容器中的固定 `/usr/local/bin/minio`，SHA256仍为 `fb741388b404ec67c4f3777b812d6174ba5562532acb17fb1ddfba9a4e6249b8`，从固定Alpine digest建立新fixture image `sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6`。旧容器未重启/重建，二进制提取不访问MinIO对象volume；失败的私有准备目录和构建文件也保留。环境记录新image，未沿用旧image验收身份。

[Reader原生IAM](reader-iam-policy.json)只授予四个固定key各自 exact version 的 `GetObjectVersion`；gateway payload仅放行online/A的该version和独立reader身份。Reader mount没有root credentials。验收后native reader身份禁用、两个reader窗口关闭，gateway payload/admin/DELETE/COPY全部拒绝，metadata只保留inspection身份：[封存证明](reader-seal.json)。

## 复用、验证与边界

复用已有 Occupancy/Copy fixture policy、PostgreSQL transaction/fence、StaticResolver、MinIO OSS signer、single-send storage transport、physical-call journal、CostAllocation与Audit/Evidence/Outbox。HTTP使用Go标准库；没有第二套存储、durable workflow、IAM、queue或通用传输runtime。新增能力仅是平台pin/实际read-owner-stop语义；相较另引Temporal/通用streaming sidecar，现有infra已满足这个有限合成切片。128KiB以内缓冲只用于fixture；没有新造resumable upload或生产数据下载系统。

未发布migration仍为60–64；当前64补充typed `reader_pin_id` FK、active owner guard和one-pin/one-native-call唯一约束，没有占用C2的65或共享routing注册文件。新保留数据库 `upgrade_main59_reader64` / `fresh_reader64`均通过；max64/count61（原历史编号存在gap），旧接受数据库未升级：[迁移证明](reader-migration-proofs.json)。Native fresh64与上述SQL SHA256一致。

| 检查 | 结果 / 边界 |
|---|---|
| 生命周期真实PostgreSQL回归 + storage/pilot/HTTP | PASS；application149.588s，scratch测试库与所有native环境分离 |
| 最终reader/current caller/HTTP/Adapter专项 | PASS；application14.473s |
| reader及相关HTTP/Adapter `-race` | PASS；application17.331s |
| 全Go模块 | PASS；86包ok、45无测试；未配PG的套件不冒充真实PG验收 |
| scoped `go vet`、diff check | PASS |
| 独立证据、credential scan、native source hashes | PASS |
| LC-01/02/13 | 当前fixture reader范围PASS；不是全平台读取承诺 |
| LC-11实际processing接入、LC-12生产direct delivery共事务pin | NOT_RUN |
| LC-28全平台读取closure、完整D1、生产Rights/Delivery/灾备 | NOT_RUN |

完整日志见 [检查汇总](reader-test-results.json)及同目录四份tests.log。当前平台通用 `storage.Get` 和现有 processing/delivery Commands尚未接lifecycle pin；本封闭环境显式没有这些入口。这是后续真实接入必须解决的架构边界，不能在SDK内偷偷放行URI、用fixture policy替代CurrentDeliveryGate，或据此关闭#307。任何新删除仍须先提交新环境/exact key/version清单、得到用户确认并重新通过当前门禁。


## 后续HTTP输出边界更正

这份b5d05bd7候选记录只证明Write owner返回前pin未释放；后来核对发现net/http.Write可能只写缓冲，未同步flush时不能据此证明HTTP输出排空。没有据此删除对象。已在代码283eba050c96ac4aa70990339e3fa25dea6068e1加上owned write deadline和同步Flush，另建[新隔离环境的最终重验](../34696798-06e3-42de-8b31-6b237c6e9c22/reader-review.md)，并保留本环境四对象、保护、pin、volumes及原始证据。当前HTTP闭合结论以新报告为准。
