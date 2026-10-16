# #307 D1：candidate05 / v15 新精确确认方案

**当前状态（2026-10-16）：新授权已收到并按全部绑定执行，两目标各一次 exact DELETE，2 absence / 1 completion、四 survivor 保留，最终只读审计通过且已封存。** 见[实际执行与当前验收矩阵](issue307-d1-confirmed-v15-execution.md)。本 execution 发送额度已消费完毕，禁止重发；#318仍OPEN / Draft、不合并。下文是授权前冻结方案及其观察时点，NOT_RECEIVED / NOT_RUN 等字段保留历史含义。
旧execution `614cd9ad-3f9d-40be-b110-3f7cf95f0790` 的A一次HTTP403额度已消费；其确认、basis、approval、dispatch、provider key及窗口不能复用。

[结构化方案](evidence/issue307-d1/delete-candidate05-preparation01/proposal.json)、[完整新确认模板](evidence/issue307-d1/delete-candidate05-preparation01/confirmation-template.txt)与[证据索引](evidence/issue307-d1/delete-candidate05-preparation01/evidence-index.json)保存精确绑定。模板仍是待确认文本，没有写入执行授权文件。新execution目前只有独立处置运行身份及准备manifest，数据库中的新disposal basis、approval、dispatch、attempt及claim均未创建。

## 精确环境与冻结候选

| 绑定 | 精确值 |
| --- | --- |
| project | `dpp-lifecycle307-5d2274ee` |
| environment | `5d2274ee-a671-4bdc-b406-22b8dad8fd61` |
| 新execution | `9ea8f3e2-c805-44b4-b133-a36f50df9d15` |
| workspace | `89eba3fd-9763-4f66-8cec-1a779104f155` |
| provider | `1980751b-c50e-4895-93a8-cb55960cd635` |
| incarnation | `be784eda-96e4-49a9-ae47-eaa3203a2dc5` |
| plan | `8f0e9340-18f0-4a73-bdd6-622196a9011a` |
| plan hash | `d7db735bf2ea7a8bc8eefca70ad4fc046be18e7e159e6a5231be3073e550bbfc` |
| inventory hash | `2b1df1ccd9e7a138bf47050cad9f031c02bd04664b64eb12aee0337ad670c9f8` |
| source commit | `dbe3fbd666146db6a010a508fa99e6ed558b6fe9` |
| Linux sender SHA256 | `4614fe539f2a41a39fa3d07bf0adf10f832fcc7dff04abf6e9f538617a21b4cd` |
| Windows attestor SHA256 | `46ea9e223f190e80be55ef21032eb94b8e64c992a70ddd9a3483460fdf06a6aa` |
| Linux recovery SHA256 | `dfb2b5e3c76164868bdb8950dbd153e5e8ebf0ddf4a446b36c92d7f1be1c10c3` |
| profile SHA256 | `7f38680f8d75c3ae2332ad3bb70f64e9923abaecc504481d3930ca73194546ca` |
| target digest | `4cbb36e5aebbc9451603efd276aa309a2c4afdefcf252107571b0f8334360a38` |
| IAM SHA256 | `77e082bcfec2a34745e24227758ce6a6af40ed1e55e834791fbd0313067de94a` |
| active gateway SHA256 | `cf528277a361b62d19b478069391fafa596833e7cc71722a9f1632dd9b80b4d6` |
| sealed gateway SHA256 | `6a08b8647cc63cb389e2a511a5ad944b82d14e1ec9721f1a5ae45cb984bed4f6` |

317项runtime LF源码与该source commit逐项绑定；Go1.26.5，CGO_ENABLED=0，amd64，`go build -trimpath`。修复同时包含IAM的两项exact action和sender immutable outcome分类，新profile只接受9ea8f3e2身份。

源Core容器ID `c85b9ff408ed110f531ac79d27a3b99f66242d12766e1674e3de78067d140bd0`；源MinIO `0a344aa06c7bc9c59e5b5ffe8d8d5584e22abf1daeb8b28c549a9735f5b6bf65`；原封存gateway `ba90451138af2c980f2ee77b58bfe2908ef67e06b1e03c96fd1a4e0836400af4`。internal网络ID `33f303a57406f05c4e78b3cdb1355059a8f55f552e438162edc5bff129f4c02c`，无host ports。
PostgreSQL image `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`；MinIO image `sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6`。新gateway计划名`dpp-lifecycle307-5d2274ee-reference-delete-gateway-02`，尚未创建/启动；sender前缀`dpp-lifecycle307-5d2274ee-reference-send-02-`。
新native账号`refdelete9ea8f3e2`未创建；旧`refdelete614cd9ad`、reader和producer均原生disabled，inspector enabled。5次GET管理RPC独立记账，不冒充业务storage cost。

## 两项目标及四份保留版本

原生保护观察时间：**2026-10-15 18:11:24 +08:00**。六份完整identity、bytes、SHA256、hold及retention与旧封存基线相同。此快照不替代未来逐项fresh gate。

| 范围 | namespace / key | 精确version | bytes | legal hold / retention |
| --- | --- | --- | --- | --- |
| 目标A，先执行 | `dpp-lifecycle307-online/fixture/a.csv` | `312365a9-f984-43de-b9d5-7e26009ccd82` | 232 | OFF / NONE |
| 目标B，等待A absence | `dpp-lifecycle307-quarantine/fixture/b.csv` | `846d0c7c-289d-4667-bc7c-aa04e44357d4` | 129328 | OFF / NONE |
| 保留 | `dpp-lifecycle307-online/fixture/b.csv` | `159e90c3-adb6-4ea3-b2e8-2804e578e86f` | 129328 | ON / NONE |
| 保留 | `dpp-lifecycle307-online/fixture/processed.csv` | `7c8ad93b-31ee-445c-9402-816dda0410ec` | 232 | OFF / NONE |
| 保留 | `dpp-lifecycle307-online/fixture/standardized.csv` | `2139f293-87f4-4d49-a9f0-941b22275093` | 494 | OFF / NONE |
| 保留 | `dpp-lifecycle307-quarantine/fixture/a.csv` | `5a7ce790-9093-4569-a8fd-0726009a5eb4` | 232 | OFF / GOVERNANCE，retain-until `2026-10-15T00:58:47Z`（已到期，仍保留） |

四份survivor始终在删除范围外；保护到期或未设hold不改变保留要求。不允许bare-key DELETE、其他version、delete marker、hold-release或governance bypass。IAM的`DeleteObject`及`DeleteObjectVersion`同受各自exact ARN + `s3:versionid`约束，复用现有MinIO OSS policy evaluator核验范围。

## 新Core检查点与当前验收矩阵

检查点`dpp-lifecycle307-5d2274ee-delete-checkpoint-core-02`，ID `85567273b268e66511ab58d6c03a430788b3f93ceb7346a52cf44eaa82939ab6`，专属卷`dpp-lifecycle307-5d2274ee-delete-checkpoint-core-02-data`。
dump SHA256 `4457301bc212eff3d5010b61a907948c48c330aae70d614c0530f685386f58d7`；globals SHA256 `bb538f66ee5ee60fd2132cc92cdfd61b7051ccec9da1c0533c3269d17e0fd665`。
原始dump保留源125次调用的基线；恢复clone随后追加本轮20次只读调用及成本，达到145。DSN隔离使用严格源身份检查和显式构造clone地址；独立评审发现的字符串替换no-op缺口已关闭，大小写反例正确转向clone、缺省端口等越界输入拒绝。

| 验收 | 当前结果 |
| --- | --- |
| 源Core及恢复 | PASS：194表/2361行逐表fingerprint、role/ACL一致，源业务mutation0 |
| 旧执行历史 | PASS：两NOT_SENT、一次CONFIRMED/DENIED及verified release、三UNKNOWN_PRESENT、terminal ALREADY_RESOLVED、一claim及全部成本原样保留 |
| sender前置SQL | PASS：实际READ ONLY执行同一SQL得`t\|t\|f\|f`；旧trials/denial闭合，新A attempt0、A absence0；仍禁止先发送B |
| references / owners | PASS：27 qualified guards、54 target-FK核验，23 requirements/releases、7 pins/terminations；active pins0、未释放requirements0 |
| pending copy / output | PASS：copy operations0，两个output intents均ready |
| 六版本/保护/库存 | PASS：新candidate实际运行既有native reader，6版本/2完整inventory一致，无额外version或delete marker |
| 调用与成本隔离 | PASS：源calls/observations/typed costs仍125，clone145；20新ID逐项对应observation、typed allocation及quantity1 invocation cost |
| IAM | PASS_CLOSED：reader/producer/旧executor disabled，inspector enabled，新executor NOT_PROVISIONED；5GET、0mutation |
| 非删除入口 | PASS：network-none render/dry-run；两个完整send入口因disabled退出1，旧profile退出1；无新确认文件/发送权限 |
| 自动化 | PASS：四Go包、6 TestReference；准备正常/-O各4，checker正常/-O各7、22项内层反例及CLI；旧344raw/317runtime严格核验 |
| 资源与备份 | PASS：94个5d容器停止/未启动，PID0/noports，own preparation关闭，旧private seals及全部dump/tar原字节保留 |
| 新授权/真实删除 | NOT_RECEIVED / NOT_RUN：新basis/approval/dispatch/claim均0，权限关闭，active gateway未创建 |
| 原生成功路径 | NOT_ACCEPTED：新IAM真实DELETE、成功response-loss/absence恢复及两目标completion待新确认后验收 |
| 独立评审 | PASS，当前切片blocker0；独立正常/-O各11项、Go6、CLI及22反例通过；DSN/scope/render绑定缺口均关闭 |

[独立复评回执](evidence/issue307-d1/delete-candidate05-review01/independent-review.json)核对新35payload/index、旧344raw及317项冻结源码。
reviewer未访问Docker/provider/DB或私密凭据，未重跑native/operator；本PASS不构成删除授权。
新文档收口HEAD的PR CI另在[执行讨论回执](https://github.com/qq550723504/data-product-platform/pull/318#issuecomment-6188927216)记录，不冒充main CI或原生成功删除。

## 新确认后才可执行的步骤

1. 取得[完整新确认模板](evidence/issue307-d1/delete-candidate05-preparation01/confirmation-template.txt)的精确确认，核对source/binary/profile/IAM/gateway/checkpoint绑定。任何变化先停止、更新方案；不沿用旧614cd9ad确认或延长旧窗口。
2. 重核源Core、roles/ACL、六版本库存/保护、owner/reference/pin/pending-copy/ledger及旧release/terminal。当前只读快照不构成未来发送授权。
3. 在新有限窗口中准备新IAM并权威回读；按已验证SDK顺序关联精确policy后disabled，实际发送前才有限启用。旧executor始终disabled；不释放hold、不使用bypass。
4. 通过既有server Commands创建新basis/approval。新-02 gateway/sender使用冻结binary、root-free two-bind mounts和tmpfs scratch，取得fresh签名host topology；每次dispatch/claim重走共享fence下完整gate。
5. A最多一次；仅CONFIRMED/UNKNOWN进入预期response-loss crash。DENIED/FAILED/已确认返回单独判定，不能冒充成功；UNKNOWN只读恢复、绝不重发，A exact absence前B不启动。
6. B同样最多一次；总计最多两次、窗口不超过40分钟。scope completion必须由真实absence和既有受控事务产生，不能用HEAD200、HTTP403或退出码替代。
7. 核对四survivor、两目标absence、全部调用/成本/历史及原备份；关闭新IAM/发送/恢复权限、封存gateway并停止资源。任何阻断立即停发，保留诊断、不cleanup。本方案不授权合并。
