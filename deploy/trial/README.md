# 固定 PostgreSQL 合成试用 — #325 第一阶段

仅支持 Windows 宿主 + 本机 Docker Linux / Compose v2、Go（仓库 go.mod）、Node 22+、Python 3.11+、PowerShell 7、Git。首次构建需要访问已固定的容器镜像、npm、Go modules、MinIO 官方源码及 Maven JDBC。不是生产模板；单操作人、合成数据、loopback，无付费 Provider。现有 CSV demo 仍按原文档使用。

在本候选的完整独立 worktree 根目录打开 PowerShell：

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

密钥为每次运行随机生成，保存在 `%LOCALAPPDATA%/Codex/trials/dpp-trial-<runId>`，ACL 仅当前用户和 SYSTEM；不在参数、构建源、HTML 或公共日志内。不要提交/分享私有目录、环境变量或原始 Docker inspect。控制台为正常生产 Next，禁止改成 development server。`last-command-error.log` 是已脱敏的失败诊断；运行日志也保留在私有目录。所有宿主监听均绑定 127.0.0.1；Core API 仍为本机受控试用边界，不能当作生产 IAM。

中断/失败后 registry 保留 UNKNOWN，后续 start 拒绝重新初始化；先保存运行身份和日志，通过 stop 精确停止已记录服务。不存在 reset/cleanup/down/故障注入/DELETE 命令。不删除卷、对象、staging、备份、镜像标签或其他环境。STOPPED 后仍保留完整回执与资源；停止后的 restart/resume 是 PR 2 的独立验收范围，当前 start 会明确拒绝。恢复不能靠新 workspace 或重新 seed 冒充。原生 Core STS 凭据有有限有效期，超期需要后续明确续用协议，不能放宽鉴权。

OpenMetadata 为 NOT_ENABLED；完整 B/C 闭环、真实下载与撤权负例属于 PR 2。人工解释 deferred / NOT_RUN。当前候选的实际验收见 `docs/acceptance/issue325-stage1.md`；历史 B/C 报告不构成本候选 PASS。精确 merge SHA 的 main CI、生产准入均单列。
