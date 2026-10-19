# #325 / #327 原环境续用：Windows 已退出进程对象修复

2026-10-19。用户授权在原环境执行一次 stop→resume，接纳 #327 已合并控制器并续发同范围 Core STS，保留 run/workspace/Actor/数据，不重新 seed。

## 有限 contract 与复用

Goal：让确已退出、但 process object 因观察者引用而仍存在的 Windows 旧进程通过原环境 resume 预检；保留全部 live-process ownership 与 source/artifact/data guards。

权威实现仍为 `deploy/trial/runtime.py` 的 process identity helper 与 `deploy/trial/resume.py` 的原续用协议，普通入口仍为 README。复用 Windows `WaitForSingleObject` / process-object signaled 状态；只为允许旧进程缺席的核验句柄增加 SYNCHRONIZE 权限。不新增 supervisor、进程管理服务、PID registry 或恢复协议，不改 Core、migration、worker、STS policy 或 seed 路径。成熟原生 OS 能力已满足需求，无须引入第二套运行基础设施。

Invariants：只有原生 process object 已 signaled 才可按缺席处理；存活进程继续校验 PID/creation identity/executable，identity 不符不得停止；等待失败或非预期状态 fail closed；handle 始终关闭。正常 RUNNING status 不把已退出进程当健康。仅改 `deploy/trial/**` 与本主题文档；其他 owner 和环境只读。

Non-goals：不自动关闭其他进程持有的 handle，不删除旧 PID/registry/journal，不重建原环境；不扩大到 B/C、业务制品升级、认证或交付。首次 journal 中断恢复仍是 #325 后续。Blockers 限于上述退出/ownership/历史保留 invariant、当前必需路径和 CI。

## 当前现场与根因

- #327 merge `3b9f6367f380bcb4b0122da5e7034420b7cc33c9`，main CI `39200441245` 成功。原 worktree 固定至该提交后，原服务 stop 成功，数据回执与停止前完整 SQL/native facts 相同。
- 预检拒绝于 `process_identity(..., allow_missing=True)` 的 executable 查询。三个原 PID 的 creation identity 都匹配；Windows `GetExitCodeProcess` 返回 0，`WaitForSingleObject(handle, 0)` 返回 WAIT_OBJECT_0；进程确已退出。`QueryFullProcessImageNameW` 对这些退出对象返回 error 31。
- 根因是 helper 仅把 OpenProcess 的 missing PID 当缺席，却对仍被引用的退出对象继续要求 executable；这与 Windows 的 process-object 生命周期不符。不能靠删 registry、换 PID 或重试启动解决。
- 原环境保持 STOPPED：run `1b7e1949e8224ad8b870d28b96a3c6c2`、workspace `e7469d12-05bc-4427-b79a-356a49cfd13c`、Actor 和全部容器/卷/制品/数据保留。未创建新的 resume attempt，历史仍为 2，未续发 STS、未启动容器或新宿主、未 seed。

修复仅在允许缺席时、读取进程信息前核验原生退出信号。真实 Windows 回归保留一个已退出测试进程的 handle，证明可按缺席处理；另用真实存活子进程证明错误 creation identity 仍拒绝终止；未知等待状态在读取 identity 或 TerminateProcess 前拒绝。

修复候选的精确 SHA、独立复核和当前 HEAD CI 单列于 PR。原环境实际 resume / fresh STS / scoped reads / 续用后完整事实校验仍为 NOT_RUN，须在接纳修复控制器后执行；不能把本次隔离测试冒充原环境续用成功。
