# 固定 PostgreSQL 合成试用 — #325 RAW 与原环境续用

## 显式业务前提准备（源代码候选，不自动执行）

装配/复用与有限评审契约见 [business prerequisites](../../docs/acceptance/issue325-business-prerequisites.md)。
此入口要求已完成原环境 activation，schema71、原五份版本与原 batch/execution 仍在。
部署、准备执行和随后宿主启动分别需要授权；普通 start/resume 不会创建业务前提。

部署时单独把 `./cmd/trial-business` 构建到原 private 目录的
`business-tools/<reviewed-head>/trial-business.exe`；不要覆盖旧工具或业务制品。
私有 `business-spec.json` 按该命令的 `spec` contract 填写：`contract` 为
`issue325/business-prerequisites-v1`，原 run/workspace/Actor/activation attempt、原
CURATED target 和五份版本（id/kind/sha256/uri/bytes/rows）；明确单 consumer、purpose
`ENTERPRISE_CREDIT_RISK_SUPPORT`、最长 24 小时的 UTC validFrom/validTo 和合成来源的
verificationReason。版本身份取自此前只读验收，不能用新版本替换。消费者是本次
合成试用的明确主体，不默认继承历史 fixture 的 LICENSED_BANK 或现实授权。

```powershell
python deploy/trial/trial.py business-plan --profile <private-spec.json> --admission <merged-head-main-ci.json> --tool <private-reviewed-trial-business.exe> --output <new-plan.json>
# 独立审查输出的 source/tool、原事实、三来源 READ scope、有限有效期、合同/Profile 和配置。
# 取得业务准备执行授权后，才运行：
python deploy/trial/trial.py prepare-business --plan <new-plan.json> --plan-sha256 <printed-plan-sha256>
```

plan 只有只读 SQL/local/native metadata 检查；不会启动服务、构建或分配 secret。
prepare 调用已有 Core Commands，失败保留 UNKNOWN 和每次调用回执；用同一计划
继续，Core 通过稳定审计 trace 恢复已提交步骤。改变同 run 的输入会冲突，不自动
续期/重置。完成重放只读既有回执。原 five versions、review、execution 不重跑。

准备成功只绑定下一次授权宿主启动的 Quality/Compliance Actor 配置及既有 remediation
plan；不自动重启。Delivery 保持关闭，不生成 delivery token。质量/合规评测、认证、
实际下载、撤权和 C1 新修复 Execution 均是后续显式动作，本入口不声称它们已通过。

## 显式同范围业务权利续期（源代码候选，不自动执行）

原准备有效期结束后，不能修改 #339 spec 或重跑 `prepare-business` 来延长旧事实。
按 [续期契约](../../docs/acceptance/issue325-business-rights-renewal.md) 生成新的私有
spec：`contract=issue325/business-rights-renewal-v1`，新的 UUID `renewalId`、原
`runId/workspaceId/actorId`、原 preparation manifest 的 `preparationSha256`，以及
精确 UTC 微秒的 `validFrom/validTo` 和 `verificationReason`。时长不得超过原窗口
或 24 小时；必须在 predecessor 到期后执行。资源、consumer、purpose、READ scope
全部从原已接纳准备继承，不接受覆盖。只复用原 ContractVersion/Profile。

```powershell
python deploy/trial/trial.py business-rights-renewal-plan --profile <new-private-renewal-spec.json> --admission <merged-head-main-ci.json> --tool <private-reviewed-trial-business.exe> --output <new-renewal-plan.json>
# 独立审查原准备/predecessor、五版本、scope、有限窗口、工具及 main CI 后，取得执行授权才运行：
python deploy/trial/trial.py renew-business-rights --plan <new-renewal-plan.json> --plan-sha256 <printed-plan-sha256>
```

续期仅追加 Authorization、三份声明/核验/绑定及各自的成本、证据和审计，不更新旧
有效期、原 spec 或准备 claim。每个 predecessor 仅允许一个 successor。UNKNOWN
保留旧失败记录，必须沿用同一 UUID/窗口/计划恢复；窗口过期后保持 fail closed，
不自动分配新窗口。完成重放只返回历史回执，不激活已撤销或过期权利。
新旧私有 manifest 都保留，仅推进显式 latest renewal binding。
计划不构建工具、不写锁/registry/journal、不访问对象；`--output` 只写指定的新计划。
执行不会启停服务、续发 STS、改变策略、seed、重跑审核/加工或执行评测/认证/交付。

仅支持 Windows 宿主 + 本机 Docker Linux / Compose v2、Go（仓库 go.mod）、Node 22+、Python 3.11+、PowerShell 7、Git。首次构建需要访问已固定的容器镜像、npm、Go modules、MinIO 官方源码及 Maven JDBC。不是生产模板；单操作人、合成数据、loopback，无付费 Provider。现有 CSV demo 仍按原文档使用。

在本候选的完整独立 worktree 根目录打开 PowerShell：

已有 `.data/trial/state.json` 的原环境使用下节 `resume`；首次 `start` 的初始化步骤不适用于保留环境。

```powershell
python deploy/trial/trial.py preflight
python deploy/trial/trial.py start
```

首次 start 编译本次 API / Worker / prerequisites binary、官方固定版本 MinIO 和生产 Next，准备全新随机身份、专属 network / named volumes、只读来源账号及三个空 RAW Dataset。不自动登记 batch、抽取、接纳版本、审核、加工、认证或下载。控制台打印实际 `web` 地址；打开其 `/batches`，按下面三步操作：

1. 保留默认批次名或填写仅含字母/数字/下划线/连字符的名称，点击“登记来源并准备批次”。
2. 在详情点“启动抽取”；用“核对原运行”读取真实 Hop 运行直到 SUCCEEDED。UNKNOWN 时保留 Operation / Attempt，核对原运行，不能盲目再建批次冒充恢复。
3. 点“原子接纳三份 RAW”。应显示 enterprise / lease / energy 的 6 / 10 / 14 行、三个明确版本、manifest hash 和逐份 SHA256。可点击版本检查事实。第一阶段不开放该批次后续加工入口。

```powershell
# 同一运行：读取事实，不重新 seed / 新建 workspace / 重发业务命令
python deploy/trial/trial.py start
python deploy/trial/trial.py status
# 仅停止本次确切持有的服务，保留全部数据
python deploy/trial/trial.py stop
python deploy/trial/trial.py status
```

`status` 的 `facts` 来自只读 SQL transaction 与真实存储 GET/hash 校验，不把文件状态当作业务事实。`.data/trial/state.json` 是本 worktree 的 ownership registry；`initial-facts.json`、`latest-facts.json` 保存当前观察。每次 stop 的不可变清单与观察写入 `stops/<attempt>/facts.json`、`observation.json`，`stopInspection` 绑定其路径和 SHA256；历史 root `before-stop-facts.json` 不再覆盖。registry 记录 source HEAD 与逐文件 hash、确切 binary SHA256、image ID、生产 web build、workspace、端口、容器/卷/network 身份及 secret 引用。源文件、制品、容器、配置或 daemon 漂移时拒绝继续。CLI 独占锁拒绝并发 writer。

## 原环境 restart/resume

保留同一 worktree、registry 和私有目录。先提交本候选控制器源码；普通启动不能代替显式续用：

```powershell
# STOPPED → 原容器/卷/端口和原 API/Worker/生产 Web，不初始化数据
python deploy/trial/trial.py resume
python deploy/trial/trial.py status
# RUNNING 的重复 resume/start 只读当前事实
python deploy/trial/trial.py resume
# 已知 RUNNING 的受控 stop → resume
python deploy/trial/trial.py restart
```

`resume` 核对原 Git/source manifest、全部原制品/配置、确切 daemon/container/volume/network、旧进程退出和原端口可用。仅允许已提交的 `deploy/trial/**` 与 credentials-only helper 变更，并追加 controller binding；原业务源码或制品变更、端口冲突和 UNKNOWN 均拒绝，不自动选新端口或重建环境。本切片恢复原业务制品，不执行 migration、prepare、seed、抽取、审核、加工或交付。

Windows 的已退出进程对象可能因观察者仍持有 handle 而暂时存在。显式允许旧进程缺席的 stop/resume 核验使用原生 process-object signaled 状态证明退出；不能要求已退出对象仍能返回可执行文件名。存活进程仍核验 PID、creation identity、完整 executable；退出探测失败或未知均拒绝，不据此停止外来进程。

原状态保存在不可覆盖的 `.data/trial/resume-origin.json`，每次续用的开始/完成/失败、前后完整事实与 native STS observation 保存在 `.data/trial/resumes/<attempt>`；同一 run/workspace 下的 resume attempt 只标识本次宿主服务启动。私有旧配置和凭据保留；既有 SDK 为同一 workspace 续发相同 scope 的有限 Core STS，写新私有凭据/profile，启动旧业务制品前以新凭据实际 GET/hash 原对象。停止前与启动后 SQL/native 事实必须一致。运行期 STS 到期时 status 拒绝报告就绪，使用显式 restart 续用，不放宽鉴权。

`sourceHead` 仍指原业务制品源码，`controllerHead` 单列本次续用控制器，不能把旧业务二进制冒充新构建。`PR2.restartResume` 与 `PR2.businessDelivery` 分列。已登记续用阶段的执行失败记录 UNKNOWN，保留容器/卷/数据/receipt，可显式 stop 核对过的 owner；不盲目再次 resume。首次建立 journal、尚未保存 RESUMING 时若中断，可能保留 STOPPED 与部分 journal；后续拒绝自动采纳或覆盖该 journal，需保留现场诊断。该首次建立窗口的显式恢复属于 #325 后续，不影响已绑定原环境的正常续用。具体 write set 和验收边界见 [resume checkpoint](../../docs/acceptance/issue325-stage2-resume-checkpoint.md)。

密钥为每次运行随机生成，保存在 `%LOCALAPPDATA%/Codex/trials/dpp-trial-<runId>`，ACL 仅当前用户和 SYSTEM；不在参数、构建源、HTML 或公共日志内。不要提交/分享私有目录、环境变量或原始 Docker inspect。控制台为正常生产 Next，禁止改成 development server。每次命令失败独立创建已脱敏的私有 `command-error-<id>.log`，错误提示只引用该文件；已有 `last-command-error.log` 和此前诊断均保留，不覆盖。运行日志也保留在私有目录。所有宿主监听均绑定 127.0.0.1；Core API 仍为本机受控试用边界，不能当作生产 IAM。

中断/失败后 registry 保留 UNKNOWN，后续 start 拒绝重新初始化；先保存运行身份和日志，通过 stop 精确停止已记录服务。stop 只依赖本机 Docker daemon 与确切归属校验，不依赖构建工具、fixture 或业务健康。RUNNING 的停止前事实读取失败时，本次 `stops/<attempt>/observation.json` 和 registry 的 `stopInspection` 明确记录 UNKNOWN，保留已有成功回执，仍停止已核验的服务；不能把这当作业务数据已验证。不存在 reset/cleanup/down/故障注入/DELETE 命令。不删除卷、对象、staging、备份、镜像标签或其他环境。STOPPED 后仍保留完整回执与资源，start 明确拒绝重建，使用上述显式续用协议。

已接纳 bootstrap 的原环境如果曾被旧 stop 覆盖历史 `before-stop-facts.json`，须先审查只读 `stop-recovery-plan --facts <现存 exact-hash 历史副本>`，再单独授权 `stop-recover --plan <审查计划> --plan-sha256 <SHA256>`。恢复保留本次完整清单，恢复历史原字节并绑定新 stop receipt，不改旧 manifest、不启停服务、不续发凭据或访问对象；普通 resume 不自动修复。详见 [stop receipts/recovery contract](../../docs/acceptance/issue325-stop-receipts.md)。

## 显式业务制品升级与原 RAW 加工

用户授权业务升级后，在同一 worktree 提交升级源码，再执行：

```powershell
python deploy/trial/trial.py upgrade
python deploy/trial/trial.py status
```

`upgrade` 要求原环境已知 RUNNING。先构建独立路径的 API/worker/生产 Web，
不覆盖旧 binary 或运行中的 `.next`；再使用既有 CreateDataset / CreateWorkflowVersion
命令登记两个输出 Dataset 和冻结工作流，不 seed、不 migration、不重建或重抽 RAW。
原 source HEAD、原制品字段与 `resume-origin.json` 保留；`businessHead` 和独立 hash
manifest 明确新制品版本。升级 journal 位于 `.data/trial/upgrades/<attempt>`，私有
制品位于原 private 目录下 `upgrades/<attempt>`，所有开始/完成/失败回执保留。
新人工决策 capability 仍绑定同一 Actor/workspace/subject，仅用于本机合成试用。

固定工作流不属于原 runtime source manifest；新升级准入单独记录其路径、规范化
SHA256 与旧 controller HEAD。工作文件必须与目标 commit 一致，目标定义还必须与旧
controller commit 一致；把越界工作流修改提交到 Git 也不能获准接纳。此指纹只追加
到新升级 binding/manifest，不改写已有真实升级制品、旧 manifest 或 origin。

切换时受控停止同一 owner，复用原容器、卷、端口和同范围有限 Core STS 后启动新制品。
完整旧 SQL 事实、来源每一行、原批次/receipt/RAW hash 必须保持。重复 upgrade 只读状态，
不会再次创建工作流、输出或业务运行。普通 resume/restart 只恢复明确绑定的制品；未接纳
的业务源码变化继续拒绝。UNKNOWN 保留现场，禁止盲目重新升级或修改旧回执。

回到原 `/batches/<operation>`：点“开始实体解析”，进入 `/reviews` 核对来源、策略、
冻结候选后填写理由确认或拒绝；任务 SUCCEEDED 后回原批次点“运行既有加工模板”。
该操作使用冻结 WorkflowVersion、原三份 RAW 和审核后 STANDARDIZED 版本，稳定的
batch idempotency key 防止顺序重放重复加工。执行详情展示冻结依赖、产物、隔离和成本。
不要创建替代批次；审核不由 launcher 自动作出，certification/delivery 也不由加工代替。
具体边界见 [business checkpoint](../../docs/acceptance/issue325-stage2-business-checkpoint.md)。

OpenMetadata 为 NOT_ENABLED；完整 B/C 闭环、真实下载与撤权负例属于 PR 2。人工解释 deferred / NOT_RUN。第一阶段实际验收见 `docs/acceptance/issue325-stage1.md`；历史 B/C 报告不构成本候选 PASS。精确 merge SHA 的 main CI、生产准入均单列。

## 已有五份版本的显式激活入口

设计和有限合同见 [explicit activation](../../docs/acceptance/issue325-explicit-activation.md)。
该入口适用于已保留三份 RAW、两份加工版本均 READY 的同一试用环境。
先单独完成账务 bootstrap（原库68→69→70→71），再取得 Core 接纳的 custody，
最后生成新 activation-plan。本次源代码实现未在原环境部署、迁移、取证、续发、复制或激活。

准备 admission JSON：`{"head":"包含本控制器的实际合并SHA","ciRunId":该SHA的push/main运行ID}`。
只读计划会通过公开 GitHub API 核实实际 SHA、main ancestry 和已完成的 required CI。
源码以 #335 为严格基线，只接纳已提交控制器变化；API 不可用则拒绝，不操作原服务。

```powershell
python -B deploy/trial/trial.py bootstrap-plan --facts <原五版本回执> --admission <admission路径> --output <新bootstrap计划>
# 单独授权原环境账务 bootstrap 后：只启用原 Core，保留备份与逻辑事实，迁移到71，再停止 Core。
python -B deploy/trial/trial.py bootstrap --plan <bootstrap计划> --plan-sha256 <计划digest>

# 取得 bootstrap PASS 后，按 operator-custody contract 准备新的私有 profile 和空输出目录。
python -B deploy/trial/trial.py custody-plan --profile <私有operator-profile> --admission <admission路径> --output <新custody计划>
# 单独授权两份原对象的有限读取后：不续发 STS，不调用通用健康检查。
python -B deploy/trial/trial.py custody --plan <custody计划> --plan-sha256 <计划digest>

python -B deploy/trial/trial.py activation-plan --facts <原五版本回执> --custody <已接纳custody.json> --output <新activation计划>
# 另行授权同策略 STS、受保护复制与激活后：此阶段要求71，不再迁库。
python -B deploy/trial/trial.py activate --plan <activation计划> --plan-sha256 <计划digest>
```

operator profile 的格式见 [operator custody](../../docs/architecture/operator-custody.md)。
其中 planSha256 使用已接纳 bootstrap 的计划 digest，providerWitness 使用 bootstrap-plan
输出的同名字段；credentialFile 引用原 config.json，hash 来自原文件，不能改写原配置。
两条 version/URI/hash/size 必须精确匹配原 STANDARDIZED/CURATED；不创建新版本。
输出目录须已存在、为空且保持原私有 ACL。profile 的有效期须在一小时内。

每阶段均保留旧业务 binding、origin、凭据、制品、数据和所有回执。
activation-plan 要求 Core 的 ACCEPTED custody 观察，执行激活时会在签发或复制前重新查询。
bootstrap 后，普通 resume 必须等到显式 activation binding 才能启动宿主。
失败、UNKNOWN、孤立 journal 均拒绝重试；69/70 中断不能冒充新的68检查点。
完成阶段的 replay 仅读核验。精确 owner stop 可用于止损，不清除未完成 journal。
plan 输出、源码合并与 CI 都不授予原环境执行权。

已完成激活的同一环境续用见 [protected continuation](../../docs/acceptance/issue325-protected-resume.md)。
部署并单独授权后，stop/status 通过 SQL 和原生元数据核对原五份版本；resume 必须
持有成功的五版本 before-stop 基线，拒绝旧三 RAW 回执或 UNKNOWN 检查点。
resume 只续发一次原策略 Core STS，并复用受保护读取核对五份内容后才启动原宿主。
现有 SDK 每份执行 HEAD→GET，十次物理调用分别产生新成本和证据；原业务事实保持
精确一致，新调用账务按封闭关联核对，不再把合法新增账务误判为历史数据变化。
失败保留现场和已发生成本，不自动重试、复制或重跑业务。实现和 CI 不代表已执行原环境续用。

## 副作用前 UNKNOWN 的显式恢复

仅接纳已部署 #336 的同一 bootstrap attempt：原 build/private-admission
已完成、失败为 Refused UNKNOWN，且公共/私有成员完全匹配规定检查点。
宿主/Core/SQL/备份/迁移动作标记、额外文件、身份或输入漂移、孤立 recovery
均拒绝。普通 resume/restart 和重新 bootstrap 不会自动恢复它。

```powershell
python -B deploy/trial/trial.py bootstrap-recovery-plan --admission <新控制器的实际合并SHA及main-CI admission> --output <新恢复计划>
# 单独审查恢复计划并授权执行后，才允许以下命令：
python -B deploy/trial/trial.py bootstrap-recover --plan <恢复计划> --plan-sha256 <恢复计划digest>
```

恢复在同一 attempt 的 recoveries 子目录中构建新工具并追加日志，重新核验
private ACL、source/main-CI、原五版本和原生身份后，复用账务 bootstrap
执行顺序。旧 failed.json、before-state、plan 和原工具均保留；成功追加
resolution 与新的 accepted binding。custody profile 的 planSha256 使用命令
返回的 acceptedBootstrapPlanSha256（accepted manifest 的 plan digest），
不使用旧 attempt projection 中的原始 planSha256。

完成 replay 仅核验已接纳 binding；恢复中断或 allocation orphan 继续 UNKNOWN，
不允许第二次恢复。该源代码实现不代表原环境已经部署或执行恢复。
