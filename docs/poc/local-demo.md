# 可重复启动的本地 POC 演示

这是**合成数据、单操作人、仅本机访问**的演示，不是商业部署模板。真实运行 Core API、原生 Worker、PostgreSQL、Redis、MinIO 和 Next standalone；不包含 Hop、Splink、OpenMetadata 外部引擎实跑，也没有生产登录权限系统。

## 启动

从包含本文件的完整候选分支根目录执行。主机需要 Node.js 22、Docker（Linux containers）和 Compose v2；不需要在主机安装 Go、PostgreSQL、Redis 或 MinIO。Linux 为 CI 验证平台，Windows Docker Desktop / WSL 与 macOS 尚需本机验证。

```sh
node deploy/demo/demo.mjs doctor
node deploy/demo/demo.mjs up
```

打开 `http://127.0.0.1:3180/reviews`。首次启动自动构建镜像、等待依赖健康、执行迁移，并调用真实 Core 应用命令初始化参考 CSV 和待审核任务。它不会替操作人确认候选，也不会替操作人发布数据产品。

只有控制台发布主机端口，且固定绑定 `127.0.0.1:3180`。数据库、对象存储、Redis、Core API 没有主机端口。启动器拒绝 SSH/TCP 远程 Docker context，不继承仓库 `.env` 或 `COMPOSE_*` 设置。不要改成公网监听，也不要导入客户数据。服务间演示凭据是公开测试值。

## 一次完整演示

**1. 审核。** 在实体审核页面比较原始记录与候选，填写自己的审核理由并确认参考候选。理由会写入真实审核证据和审计。拒绝是真实拒绝，不会被脚本偷偷改回确认；拒绝导致流程不能继续时，保留现场排查，或明确清空这一份合成演示后重新开始。

**2. 加工与发布准备。** 审核完成后执行：

```sh
node deploy/demo/demo.mjs advance
node deploy/demo/demo.mjs status
```

`advance` 先确认实体解析成功，才通过真实 Redis 队列调度原生 Worker，计算参考指标、运行质量与合规检查、建立演示权利及数据契约，并由 Core 校验 Release。输出中会给出产品与追溯地址。样本业务周期固定为 `2025-03`；它不是当前真实企业数据。

页面中有 `R-DEMO-READY` 和 `R-DEMO-BLOCKED` 两个版本。前者经过完整 Core 校验；后者故意没有完成治理准备，以展示阻止发布的状态。演示权利有效期为初始化加工时起七天，时间见 `rightsExpireAt`。过期后不篡改旧授权来继续发布。

**3. 发布。** 在产品详情中检查八项 Gate，手动点击可发布版本的“发布 Release”。观察被阻塞版本按钮仍不可用。

**4. 验证。**

```sh
node deploy/demo/demo.mjs verify
```

只读验证查询 PostgreSQL 和真实对象存储，检查已发布状态、证据快照完整性、五个数据版本的字节校验和、审核理由、成本、发布审计及唯一发布事件。发布前运行会报错，不会自动发布来满足验证。查看 JSON 输出中的 `snapshotId` 和 `rootHash`，再进入证据追溯页面核对。

## 停止、继续与清空

```sh
# 停止容器，保留数据库、对象存储和演示清单
node deploy/demo/demo.mjs down

# 从相同目录再次启动，读取已有样本，不重新初始化或覆盖历史
node deploy/demo/demo.mjs up
node deploy/demo/demo.mjs verify
```

重复 `up` 不创建新工作区或样本；成功完成加工后的重复 `advance` 只读取状态，不重复投递任务或创建 Release。数据库业务状态仍是事实来源；`manifest.json` 仅保存演示导航 ID 和初始化阶段，不授予权限、不替代业务状态。

清空必须使用完整确认词：

```sh
node deploy/demo/demo.mjs reset --confirm=DELETE_DEMO_DATA
```

这只删除由当前仓库实际路径派生的 Compose 项目及其四个演示数据卷，不调用 `docker system prune`，不删除镜像、其他项目或现有客户数据库。移动仓库目录会改变项目名；应先在原目录停止/清空原演示，避免遗留卷和端口占用。

## 失败与恢复

`node deploy/demo/demo.mjs logs` 显示服务日志。3180 已被占用时启动会失败，而不是接管已有服务。`status` 是 JSON，列出实际任务状态和业务对象数量。

初始化/加工持有 PostgreSQL advisory lock，防止两个启动器重复工作。多步初始化**不是一个跨服务原子事务**：失败或中断后保留 `INITIALIZING` / `ADVANCING` 阶段并拒绝盲目重试，避免复制不确定结果。检查日志后，显式重置这一份合成演示。不能声称它已经实现断点恢复或灾难恢复。

演示清单保存在专用数据卷，Core 不读取它作权限判断。原生 Worker、业务状态迁移、不可变历史和审核/发布命令不因演示便利而放宽。

## Dead-letter 运维重放

Core 镜像包含显式的 `outbox-replay` operator tool；Compose 通过 `tools` profile 暴露它。它只允许重放已经进入 `DEAD_LETTER` 的原事件，要求明确的 operator actor、稳定幂等键与理由，并在同一事务写入 immutable replay fact 和 Audit。不要用 SQL 手工修改 `outbox_event.status`。

在与当前 Compose 项目相同的环境中执行：

```sh
docker compose -f deploy/demo/compose.yml --profile tools run --rm outbox-replay \
  -event-id=EVENT_UUID \
  -actor-id=OPERATOR_ACTOR_UUID \
  -idempotency-key=INCIDENT_OR_REQUEST_KEY \
  -reason='dependency recovered after incident' \
  -trace-id=OPTIONAL_INCIDENT_TRACE
```

该工具复用容器内的 `POSTGRES_DSN`，不会生成新的业务事件；原 event ID、payload、routing obligation 和已经成功的 handler acknowledgement 都保持不变。重放后 dispatcher 只继续未确认的 handler。重复使用同一幂等键和相同语义返回原 replay fact；同键不同语义会显式冲突。

这只是受控恢复入口，不是完整运维后台、自动无限重放或双人审批系统；生产环境应由既有部署/IAM 流程限制谁能启动该 operator tool。

## 云主机上的私人演示

在自己的云主机本地 Docker daemon 上运行相同命令，保留所有 loopback 绑定。通过 SSH 本地端口转发访问，不开放安全组中的 3180、5432、6379、9000 或 8080。示例：

```sh
ssh -N -L 3180:127.0.0.1:3180 USER@YOUR_SERVER
```

然后在本机访问 `http://127.0.0.1:3180`。本交付不创建服务器、不修改安全组、不完成公网部署或多用户认证。

## 可验证的交付

`demo-lifecycle` CI 构建这里的实际镜像，执行人工操作的 Chromium 模拟，再验证重复 up、重复 advance、未确认 reset、down/up 保留同一证据快照和审核理由，以及只有本机控制台端口被发布。日志、截图、浏览器 trace 与 `verification.json` 存为七天 CI artifact。只有 `verified: true` 的成功运行才是验收证据，脚本存在本身不是。

前置合并收敛记录见 `docs/poc/pr-consolidation.md`。既有真实 Core 验收和模拟接口浏览器测试仍独立保留，不能把一个层级的成功替代另一个。

实施参考：Docker 官方 Compose startup-order 文档的健康依赖机制；Next.js 15 官方 output 文档的 standalone 与静态资源复制说明。此演示沿用此前验证的历史 MinIO 镜像，它不是当前生产安全基线推荐。
