# 临时测试容器回收

本页约束 `examples/temporary_containers.py` 与 PowerShell adapter。Docker 是唯一 runtime；不新增队列、调度服务或全局清理守护进程。

## 归属与完成边界

服务、验收数据库、对象存储、Redis、备份、恢复副本与其他会话的容器不进入临时 helper 回收范围。停止状态、退出码 0 或年龄本身不证明测试已完成。

新的一次性 helper 由 `New-DppTemporaryContainer` / `TemporaryContainers.create` 创建。每次创建使用独立 owner nonce，记录 immutable Docker ID、run UUID 与 `org.dpp.temporary.*` labels。调用者在业务输出、provenance/exit receipt 已持久化后，显式调用 `Complete-DppTemporaryContainer` / `complete`，确认自己创建的 helper 已完成。

只接受预期退出码、只读 rootfs、无持久 volume mount、无端口发布、无 restart policy 的 helper。不接管无标记的旧容器，不依据名称、镜像、TTL 或全机扫描自动删除。预期非零退出必须由测试明确传入；意外失败、OOM、running、created 和未知结果全部保留。

完成后将完整 inspect 元数据、带时间戳的 stdout/stderr 与 SHA-256 清单落到工作树外的私有用户目录。Windows 复用 fixture preparation 的完整 DACL 设置方式，仅保留当前用户与 SYSTEM，复用目录的其他显式授权也必须移除；POSIX 使用 0700 目录。日志可能包含 credential，不能提交或作为公开验收 evidence。公开 cleanup receipt 只含容器 ID、移除结果和删除资源计数。

核对归档、重新 inspect 确认 identity/state 不变后执行 `docker container rm <full-id>`。不传 `--force`、`--volumes`，不删除 volume、bind 文件、镜像、network 或对象数据。原始 provenance 中的 helper ID 是历史 identity；完成后不再要求该 Docker 对象常驻，私有归档与追加的 cleanup receipt 解释其退役。不要改写已封存的旧验收结果。

## 调用与失败恢复

```powershell
. ./examples/temporary-containers.ps1
$run = [guid]::NewGuid().ToString()
$helper = New-DppTemporaryContainer -RunID $run -DockerArguments @(
  '--pull','never','--read-only','--network','none',
  'alpine:3.22','sh','-c','echo synthetic-check'
)
docker start $helper.id
docker wait $helper.id
# 先验证业务断言、写入输出与 exit/provenance receipt，再确认完成。
Complete-DppTemporaryContainer -Handle $helper
```

默认私有 registry：Windows `%LOCALAPPDATA%/Codex/docker-helper-archives`；POSIX `~/.local/state/dpp/docker-helper-archives`。每个 owner 只有一个 writer；完成后不允许重启或复用该 helper。

归档或移除失败不会重跑业务。可在修复 Docker/磁盘问题后重试**已确认完成**的同一 run：

```text
python examples/temporary_containers.py collect --run-id <exact-run-uuid>
```

collect 只读私有 registry 的 COMPLETED tickets，不枚举全机容器。CREATED/CREATE_PENDING、故障未确认、显式 RETAINED 和其他 run 的 tickets 不被接管。没有完成 receipt 的进程崩溃残留必须由 owner 审核后单独退役；不能把超时当完成。

PowerShell `-Retain` / Python `retain=True` 可显式保留某个 helper；不会进入 collect。老脚本或其他仍在执行的 checkout 不会被本变更热替换。更新到本版本或显式接入 adapter 后，新 helper 才使用该机制。

已接线：fixture preparation、reader、plane、copy-recovery 的同步单步 helper。异步 `StartOnly` 和有已消费人工批准的历史 destructive driver 不自动改变其封存契约；owner 应在确认所有依赖结束后使用同一 API，而不是全局 prune。

## 验证与复用决策

复用已有 Docker CLI、receipt/file evidence，以及测试入口自身的完成判定；现有 ingestion `CleanupReceipts` 仍负责它的多组件清理结果。Docker 原生 `--rm` 无法满足当前脚本先 inspect、落盘退出码与日志再移除的顺序，故仅增加薄的归档 adapter。Testcontainers/Ryuk 适用于其创建的资源，不能接管这些既有 PowerShell helper；无需第二套 runtime 或 TTL janitor。

本地无 Docker 单测：`python examples/temporary_containers_test.py`。Windows 权限回归：`pwsh -File examples/temporary-containers-acl-test.ps1`，验证已有目录上的额外显式授权及已有诊断文件的继承授权被移除；CI 的 Linux/Windows matrix 均执行契约单测，Windows 另执行真实 ACL 回归。原生 Docker 验证：`pwsh -File examples/temporary-containers-native-test.ps1`，仅创建新的隔离合成 helper，验证失败保留、预期非零、双日志归档与 bind 数据保留。
