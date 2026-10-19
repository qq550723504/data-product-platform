# 固定 PostgreSQL 合成试用 — #325 RAW 与原环境续用

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

`status` 的 `facts` 来自只读 SQL transaction 与真实存储 GET/hash 校验，不把文件状态当作业务事实。`.data/trial/state.json` 是本 worktree 的 ownership registry；`initial-facts.json`、`latest-facts.json`、`before-stop-facts.json` 保存对应时点回执。registry 记录 source HEAD 与逐文件 hash、确切 binary SHA256、image ID、生产 web build、workspace、端口、容器/卷/network 身份及 secret 引用。源文件、制品、容器、配置或 daemon 漂移时拒绝继续。CLI 独占锁拒绝并发 writer。

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

中断/失败后 registry 保留 UNKNOWN，后续 start 拒绝重新初始化；先保存运行身份和日志，通过 stop 精确停止已记录服务。stop 只依赖本机 Docker daemon 与确切归属校验，不依赖构建工具、fixture 或业务健康。RUNNING 的停止前事实读取失败时，`before-stop-observation.json` 和 registry 的 `stopInspection` 明确记录 UNKNOWN，保留已有成功回执，仍停止已核验的服务；不能把这当作业务数据已验证。不存在 reset/cleanup/down/故障注入/DELETE 命令。不删除卷、对象、staging、备份、镜像标签或其他环境。STOPPED 后仍保留完整回执与资源，start 明确拒绝重建，使用上述显式续用协议。

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
