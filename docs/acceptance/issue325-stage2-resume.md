# #325 第二阶段首项：原环境 restart/resume

2026-10-19。仅覆盖用户明确要求的原环境续用；完整 B/C、整改、认证、DIRECT_DATA 及撤权业务负例仍为 NOT_RUN。本切片的 [有限 contract / reuse / write set](issue325-stage2-resume-checkpoint.md) 与 [操作步骤](../../deploy/trial/README.md) 分别维护范围和普通操作入口。Draft PR #327，不自动合并或关闭 #325。

## 运行与制品身份

- 基线 main / #326 merge：`1d1e8efd8217854cc3b09010f7e47b183fa824d6`。
- 实跑续用控制器：`23d54259c8bc24619f1667069d18307596d9f5cc`。后续仅增补本报告和 `docs/acceptance/evidence/issue325-stage2-resume/`，不修改该控制器的 source fingerprints。
- 原业务制品源码仍为 `1687a6ef4bd5bd998696f95255a9e3885607a416`；原 API、Worker、trial-prepare、Node 和实际 standalone Web hashes 均保留。这里的 PASS 是当前续用控制器恢复原制品，不声称旧业务二进制是新构建。
- Run：`1b7e1949e8224ad8b870d28b96a3c6c2`；workspace：`e7469d12-05bc-4427-b79a-356a49cfd13c`。Actor、6 个容器 ID、6 个卷、network、image 和全部端口均沿用。
- 原 Operation `31a4bee4-e24f-405e-b53d-53c7807ac797`，业务 Attempt `600d0205-4372-48d0-a884-5e5ffed1fdb1`，manifest `99106c442732f8aec9ce270e971c4774a125c628a4e28883ff75ae47c8001c55`；原三个 v1 READY DatasetVersion 均未更换。
- 本次结束状态为 RUNNING，实际页面 `http://127.0.0.1:60087/batches/31a4bee4-e24f-405e-b53d-53c7807ac797`。这是本机有限试用入口，不是生产部署。

## 当前实际验证

汇总证明：[continuity.json](evidence/issue325-stage2-resume/continuity.json)。

| 检查 | 结果与边界 |
|---|---|
| 原状态与源码 | PASS：原 registry 的 895 个 source fingerprints 与其原 Git commit 完全一致；实际原状态的完整 bytes/hash 保存在不可覆盖的 `resume-origin.json`。原业务源码变化不允许被 controller binding 接纳 |
| 首次实际 resume | PASS：从原 STOPPED 恢复同一 6 容器；在启动宿主服务前后比较完整只读 SQL/native 对象事实，全部相同；无 prepare、migration、seed、容器重建或镜像拉取 |
| 重复 resume / start / status | PASS：完整事实、进程 identity、private hashes、resume history 都相同，只有一次实际 resume attempt；未续发第二份 STS，未增加业务事实 |
| 实际 restart | PASS：精确停止后再恢复原容器/卷/端口；宿主进程为新的 creation identities，原 run/workspace/业务事实不变。第二次 resume attempt `4e4de480e7aa4695a3b3bdcfa463bec5`；第一次为 `b5e99ca9a32d458b8c21d6effbf2af9a`。这两个 identity 只标识宿主续用，不是新业务抽取 |
| 原数据、配置、凭据、日志 | PASS：原 16 个文件 hashes 完全不变；包括旧配置、旧凭据与第一阶段日志。新 credential/profile/helper/log 写入各自私有 resume attempt 目录；原 6 个 RAW 原生文件 bytes/hash 完全不变 |
| 来源数据 | PASS：真实 PostgreSQL SELECT 三张来源表，enterprise/lease/energy 仍为 6/10/14 行，逐列逐值与原冻结 fixture 完全一致 |
| 事实与历史 | PASS：仍为 3 Resource、3 Dataset、1 ingestion operation、3 versions、23 Audit、15 Evidence、6 Cost；workflow/execution/quality/certification/delivery 均未新增。完整 operation/receipt/version/binding 和对象 bytes/hash 均与停止前相同 |
| 同范围凭据实际可用 | PASS：两次真实 native STS AssumeRole 分别保留 start/result observation；每次使用新有限 Core session 对三 RAW 实际 GET/hash 后才启动宿主服务。独立 [native oracle](evidence/issue325-stage2-resume/native-scope-oracle.go) 再读取三对象并验证跨 workspace 原生访问为 HTTP 403 / AccessDenied，见 [native-scope.json](evidence/issue325-stage2-resume/native-scope.json) |
| 真实 Edge | PASS：首次 resume 和 restart 后均读取同一 ACCEPTED 批次，原 Operation/Attempt/manifest 和三份原版本可见，行数仍为 6/10/14。只读浏览，没有点击登记、dispatch、accept、审核或加工动作 |
| 密钥隔离 | PASS：[secret-proof](evidence/issue325-stage2-resume/secret-proof.json) 扫描 2270 个表面 / 281246652 bytes，覆盖旧业务与新 helper binaries、原生产 Web、原与新增日志、HTML、全部本机回执、容器日志和 Core Audit/Evidence/Outbox，已知旧/新密钥及 Hop Basic 编码匹配为 0。不是一般 DLP 证明 |
| 本地与 CI | PASS：28 个 Python lifecycle/resume regressions；Go helper 的 SDK policy、same-attempt no-reissue、expiry、redirect、fresh-session GET/hash、foreign context tests 及 vet；diff check。当前 HEAD CI 和独立评审单列在 PR #327，不移用旧 HEAD 或 #326 的 CI |

Cost 数量未增加只说明本切片未新增 Core 业务活动；原 UNKNOWN/intent/actual Cost 历史保留。这里明确发生了两次基础设施 native STS invocation 和多次真实 native GET/HEAD/health/SQL 读取，不宣称零 Provider I/O，也不把逻辑对象检查数量当作底层 HTTP 请求计数。

## 失败边界与交接

现有 source identity 将 controller 与业务源码混在一起，不能通过改写旧 manifest 绕过 drift。当前方案保留原状态，单列 committed controller binding，且只接纳控制器与 credentials-only helper 的受限变化。原制品、业务源码、配置、资源 identity 漂移，以及端口冲突、UNKNOWN、活跃旧进程、前后事实不一致均 fail closed。过期 session 不能报告 RUNNING 就绪，使用显式 restart 走同范围续用。

原运行文件仍在同一 worktree 的 `.data/trial`；原状态在 `resume-origin.json`，续用前基线在 `resume-baseline.json`，两次不可覆盖的 before-state/start/STS/scoped-read/完成记录在 `resumes/<attempt>/`。私有文件仍受原用户/SYSTEM ACL 保护。不要删除、改写或提交私有配置、凭据、环境转储；不要以另一个 workspace、重新 seed 或旧 B/C Core 替代续用。

后续 B/C 若需要更换业务制品，必须新增显式升级绑定和对应验收；本切片不会放宽这项边界。OpenMetadata 为 NOT_ENABLED，人工解释 deferred / NOT_RUN；#296/#298/#299/#307 未关闭，其运行环境和 owner 不变。未进行 Provider DELETE、hold-release、cleanup、down/prune、外部发布或生产部署。

## 独立评审收敛（2026-10-19）

独立 reviewer 在 `b0a82a8b...` 发现：新续用路径的命令非零退出会经既有错误处理覆盖固定 `last-command-error.log`；原私有目录确实已有该文件，属于须保留的历史。现改为每次 exclusive-create 独立的私有 `command-error-<id>.log`，保持脱敏，仅返回文件引用，旧固定日志和此前诊断不再覆盖。

新增回归覆盖旧日志与两次新失败诊断分别保留、每份诊断脱敏，以及文件名碰撞拒绝覆盖。修复后的 30 项 Python 测试与 Go helper 单元测试通过；最终 reviewed SHA、当前 HEAD CI 和剩余 blockers 单列于 PR #327，不移用修复前 CI。

本轮只读现场复核：registry SHA256 仍为 `af3f7d401f5d96574074030dbd0b89bb992c33027ec8347b39d4e6b1dcec424b`，run/workspace/Actor、进程 creation identities、容器、制品与全部 private hashes 保持，完整 SQL/native facts 与最后 resume receipt 相同；旧诊断 hash 为 `7c926b87e40b26ec6ffb4c78f403571ac977ece539b51f52b4a58a6943d143ea`。本轮没有启停环境、续发 STS、重新 seed 或执行业务命令。现场运行绑定仍是 `23d54259...`；本次错误分支修复通过隔离临时目录验证，不声称已在原环境触发命令失败或部署新控制器。后续须使用显式 stop→resume 接纳已提交的新控制器，不能修改原 registry 绕过 source guard。

首次 journal 建立中断的保留/拒绝边界见 checkpoint，显式恢复列为 #325 非阻塞 follow-up；完整 B/C 与交付验收仍为 NOT_RUN。
