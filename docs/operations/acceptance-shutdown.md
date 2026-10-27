# 验收环境收尾

本页是 `tests/platform-management/run.py` 的运行与收尾契约。适用管理只读、运行日志、主服务、兼容性、目录产品和用户产品六类合成验收。既有长期演示、真实来源、其他项目及其他会话不属于本入口。

## 正常入口

在明确获准的新独立验收环境中，设置 `MANAGEMENT_FIXTURE_ACK=ISOLATED_READONLY_MANAGEMENT`，运行：

```powershell
$env:MANAGEMENT_FIXTURE_ACK='ISOLATED_READONLY_MANAGEMENT'
python tests/platform-management/run.py --suite user-product
```

`--suite` 可选 `readonly`、`runtime`、`main-service`、`compat`、`directory`、`user-product`。本地浏览器沿用 Edge；CI 显式设置 `MANAGEMENT_BROWSER_CHANNEL=chromium`，日志验收还设置 `RUNTIME_LOG_BROWSER_CHANNEL=chromium`。原 Go、权限、浏览器、最终保留比较均在这个入口内部按原顺序执行；`readonly` 的重复只读保留探测也在停止前完成。

`bootstrap.py` 和 `start-web.py` 现在是受管理的内部阶段，直接调用会拒绝。准备或验收成功、断言失败、超时、Ctrl+C、SIGTERM 都会进入同一 `finally`。先停止本轮记录的 Web、API 和合成身份进程，再以完整 Docker ID 停止数据库容器。无 `rm`、`down`、`prune`、卷删除、重新 seed 或恢复旧会话。

已有目录、容器或端口继续拒绝覆盖。完成后数据库、卷、配置、日志、浏览器截图及原始失败证据保留，容器保持停止；不能删除 ledger 后重新 bootstrap。需要长期演示应使用它自己的明确生命周期入口，不能把验收准备阶段当常驻服务启动器。

## 本轮归属与其他会话

- 每轮分配独立 UUID；Docker context 与 daemon ID 固定。容器创建前落盘 intent，使用 Docker `--cidfile`；创建后保存完整 ID、owner label 和配置/挂载摘要，随后才启动。
- 每个常驻进程启动后立即记录 PID、原生创建时间和可执行文件身份，不再等全部 sibling 启动完才记录。Windows 复用 `deploy/trial/runtime.py` 的同一 native handle 校验与停止；Linux 使用标准库 pidfd。
- 生命周期控制文件放在工作树外的当前用户私密目录，复用已有 helper 的目录权限和原子 JSON 写入；日志、环境和凭据不写入公开回执。
- 本轮进程和容器共享非阻塞 owner lock，整个验收期间持有。另一会话不能接管活跃 run；明确恢复收尾也必须重新取锁并核对同一工作树。
- 任何 PID 复用、身份不可读、owner label、完整 ID、配置、挂载或 daemon 变化，均在发信号前拒绝。没有全机容器扫描、名称/年龄推断、父 PID 递归杀进程或“清掉所有测试容器”。
- 配置摘要仅规范化实测 Docker Desktop 在 create→start 时把 `HostConfig.OomKillDisable` 的禁用默认值 `false` 改为 `null` 的变化；`true`、缺失字段及其他字段/类型变化仍拒绝。原 retained preservation 合同不变。
- 历史环境没有新协议的 locator/control identity 时不接管。本次修改不会热替换其他 checkout 或重开已停的验收环境。

## 结果与中断恢复

`acceptance-result.json` 保存原验收结论和失败阶段；每次停止追加独立 `cleanup-<attempt UUID>.json`，保存本轮 ID、停止结果、数量及保留/零删除事实。验收 FAIL 不会因停止成功而变成 PASS；停止未确认也返回非零，不声称已收尾。已有回执不会被重写。

数据库正常退出也属于收尾成功条件；超时后异常退出或 OOM 不冒充正常停止，仍保留数据卷与原始失败回执。

正常 finally 之外，CI 有 `if: always()` 恢复步骤，仅列举该 job 的固定 run 目录并读取新 owner 记录。若解释器被强制终止或机器掉电，Python finally 无法执行；保留控制记录，随后对**明确的同一 run**执行：

```text
python tests/platform-management/stop.py --run management-user-product-20261027
```

该命令不会启动服务、重新验收或读取业务数据；缺失控制记录、活跃 owner、跨 checkout 或归属变化均拒绝。重复停止已经停止的同一资源可安全追加回执。未知创建结果且缺少 cidfile 时保留 UNKNOWN/FAIL，不能按名称寻找并停止可能属于他人的容器。

## Reuse Check 与验证范围

复用 Docker CLI、原验收步骤、现有 retained preservation 合同、`deploy/trial/runtime.py` 的 Windows 进程身份/handle，以及 `examples/temporary_containers.py` 的私密目录和原子文件写入。现有 temporary helper 回收器排除数据库/服务且会退役 helper，本入口需要的是保留数据的停止，因此不改变或滥用那个回收器。

评估了 Testcontainers 这类成熟方案；现有 Docker 和 OS 原语已满足当前明确归属与保留 invariant，没有必要引入第二套 fixture runtime，也不能由它接管旧验收会话。本次仅增加原入口的资源归属和 finally 装配，不引入调度器、后台清理 daemon、TTL janitor、Core 领域对象或 provider。

契约回归：`python -B -m unittest discover -s tests -p test_acceptance_lifecycle.py -v`；CI 在 Windows/Linux 同时执行正常与 `-O` 模式。真实进程回归只创建并停止自己生成的短命 Python 进程；Docker 归属/故障使用严格替身，不接触保留环境。原六类完整验收由原 CI 的统一入口执行，结果与收尾证据分开上传。

明确获准的新隔离 native smoke：`python -B tests/native_acceptance_lifecycle.py`。它只创建新的合成容器、卷和短命进程，分别验证成功与验收失败后的停止、卷内 proof 与证据保留、不同 owner sentinel 不变，以及原容器清单不变；全部新容器最后保持停止，不复验或启动旧环境。
