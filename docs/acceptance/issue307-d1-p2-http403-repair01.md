# #307 D1：P2 HTTP403 根因与非删除修复验收

本轮只修复、验证和评审；不创建可发送候选、不安排窗口、不复用旧授权。#332 保持 OPEN / Draft / unmerged，#307 保持 OPEN。

## 有限验收范围与根因

candidate18/v28 execution `4fd5a7e7-779e-41d7-a35a-3bab7dd615b5` 已以 `FAILED_FIRST_P2_CONFIRMED_DENIED_SEALED_NO_CONFIRMED_DELETION` 终结。P2 消耗 sender、wire 两条预算，真实 HTTP403 / CONFIRMED DENIED；其余五份未发送。旧记录、预算和 pins 原样保留。

**可复现的根因是策略生成器的动作契约回归。** `pilot/remaining_policies.go` 只生成 `s3:DeleteObjectVersion`，而实际固定 MinIO 的 DeleteObjectHandler 先以 `s3:DeleteObject` 授权，即使请求包含 versionId。Version 动作检查不能替代入口动作。仓库旧 `remaining_verify.go` 已同时授予两者；这次复用该契约，不引入新策略引擎或依赖。[实际 MinIO handler](https://github.com/minio/minio/blob/9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a/cmd/object-handlers.go)、[实际 auth handler](https://github.com/minio/minio/blob/9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a/cmd/auth-handler.go)。

实际 image 为 `sha256:72476ee7f52a538e561627f7d8746e20c15d50c13e8bcdc58b22fa65cff94df6`；从停止的旧 CID 只读复制 binary，`go version -m` 确认 module `v0.0.0-20251015172955-9e49d5e7a648`。早期按202504版本假设的 native01 在模块门禁即失败，未分配或启动资源，原失败保留。正式证明使用实际 module；不以早期版本替代。

原 P2 响应正文已被旧 transport 丢弃，**历史具体 XML Code 仍 UNKNOWN**。本轮确认同镜像中这处策略缺项足以产生403，并隔离排除正确 Host 转发的签名问题；不能把新复现的 AccessDenied 回填旧事实。

## 修复与证据边界

- 新策略的两个删除动作共用同一个 exact namespace/key/version 条件。裸 key、错误版本/namespace/key、11份 survivor 均继续拒绝。实际历史 retained policy 仍按原 Version-only 语义核对，不把新修复复制到旧 disabled IAM。
- storage transport 保留 HTTP状态、闭合词表错误码、bounded body SHA256、完整/截断状态；不保存 Message、Resource、CanonicalRequest、Authorization 或任意未知 Code。新 Core observation 的 proof hash 绑定诊断内容，typed failure 经 SDK/adapter 到 sender 回执保留。
- wire 对上游非204响应追加签名回执，与本地 admission/target/attempt/budget 拒绝的签名回执区分。预算先于上游发送；任何拒绝、留证失败或 UNKNOWN 均不自动重发。原已签事实和旧 proof hash 不改写。
- 修复源码仍带旧 c18 的编译常量，仅是修复分支，**不是可执行的新候选**。不得运行它更新旧源 IAM，或用旧 execution、身份、预算和授权发送。新代接线、冻结、窗口及精确授权属于后续工作。

## 验收矩阵

| 项目 | 实际结果与限制 |
| --- | --- |
| 策略与范围回归 | 三组策略使用现有官方 policy evaluator 验证两动作及 exact 条件；裸 key、错版本、foreign key 拒绝；retained 策略没有扩大。 |
| Go 回归 | lifecycle 各包、storage、remaining sender/wire、reader、IAM owner 相关测试通过。包括403 typed diagnostic、反射 secret/重复Code/错误root/截断/尾部错误、observation写失败保留、预算阻止重发、签名 wire 本地与上游拒绝回执。未配置的真实 PostgreSQL 集成不是本轮新 PASS。 |
| 实际原生权限检查 | native03：118个用例、118次非变更 HTTP DELETE 权限探针；130次两跳 transport roundtrips 包含12次 relay前后两跳，不等于130次 native DELETE。六目标在旧策略下 AccessDenied；新策略 exact+relay 均在授权后短路为 HTTP400 / ReplicationPermissionCheck。 |
| 原生拒绝反例 | 11份 survivor exact、六目标的 foreign version、bare key、foreign key、foreign bucket 继续403 AccessDenied。错误 secret 与篡改 Host 均403 SignatureDoesNotMatch。缺失专用检查 header 在进程内被阻止，0出站。 |
| 原生非删除保证 | 所有探针构造后、签名前加入 `X-Minio-Source-Replication-Check: true`，transport再次要求该header已签名；固定官方 server 在任何 delOpts / DeleteObject 前短路。没有无header成功对照。三组17份新合成版本完整库存、版本号、bytes/SHA不变，无delete marker；ordinary DELETE=0，实际对象删除=0。 |
| 成本与请求留证 | native03 共205对合成HTTP transport尝试的start/observation，包含12次loopback wire前端与193次native后端尝试；后端中192次有HTTP响应，1次为启动期ready轮询连接失败。118次非变更权限探针均收到native响应；其余75次为健康检查/新合成版本/新IAM配置与读回尝试。原始quantity与`synthetic_native_http_request`标签保留为本合成HTTP栈的尝试口径，不把205全称为physical provider已收到的请求或删除次数；连接失败不补推已调用provider。amount=NOT_ASSESSED。12份实际 wire 签名响应样本保留并绑定SHA；本轮原生测试临时公钥未另存，因此独立验签通过的是unit实际路径，不声称样本可离线重验签。 |
| 原环境与失败保留 | native03 前后715个旧已知CID停止/PID0/noports/no-restart；原window全部文件及三份新预算archive SHA不变，其中P2 sender+wire已用记录保留。旧源启动、旧授权复用、旧预算写入、pins释放均0。native01/02/03历史回执各自保留，native02/03新隔离CID均停止，未清理卷/数据。 |
| 独立评审 | 最终结果见 `p2-http403-repair01/independent-review01.json`；仅此有限修复与非删除验收，不代表候选冻结或完整D1已通过。 |
| 剩余六份 | 全部真实执行 NOT_RUN；旧 P2 DENIED 不被改为 NOT_SENT/ACCEPTED。后续还需按实际 Core 事实处理本次 P2/T1/T2 保留 dispatch，准备新 execution/独立预算和新冻结候选，再重新精确授权；本轮不释放旧 pins。 |
| 整体D1 | NOT_ACCEPTED；PR CI、真实删除、Core业务恢复、H0/H1/H2/M1有限场景及整体验收分别记录，不互相替代。 |

权威修复证据目录：[p2-http403-repair01](evidence/issue307-d1/p2-http403-repair01/)。历史失败原字节见 [v28-window01](issue307-d1-v28-window01.md) 及其原始执行目录。新 native 只使用隔离新卷、无端口网络、独立合成凭据，不挂旧数据卷、预算或私钥。

本切片 merge contract：修复实际 native 授权动作契约；保持 exact 范围、无重发和历史事实；诊断可解释且不泄漏 secret；真实原生非变更成功/拒绝路径可复现。新候选、窗口、发送授权、源执行和合并均不在本轮交付中。
