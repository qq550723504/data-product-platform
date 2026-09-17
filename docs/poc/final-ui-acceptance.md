# 企业活跃度 POC UI —— 最终验收对照表

本文档将最小化的 POC 控制台映射到 issue #13 与父级 UI 里程碑 #75。它刻意验证核心产品模型，而不是把 OpenMetadata、Apache Hop 或 Splink 暴露为顶层产品概念。

## 操作员路由对照表

| #13 要求 | POC 路由 | Core / 读模型来源 | 自动化覆盖 |
| --- | --- | --- | --- |
| 工作台摘要 | `/` | workspace 工作台读模型 | Next 生产构建 + 路由冒烟测试 |
| 数据资源详情 | `/resources`、`/resources/{id}` | workspace Data Resource 读模型 | Next 生产构建 + 路由冒烟测试；Go 读模型测试 |
| Dataset / 不可变 DatasetVersion | `/datasets`、`/datasets/{id}` | workspace Dataset/DatasetVersion 读模型 | Next 生产构建 + 路由冒烟测试；Core DatasetVersion 测试 |
| 实体复核 | `/reviews` | workspace 待处理复核读模型 + 既有 confirm/reject 命令 | 24 个复核命令回归测试 + Next 构建 |
| Workflow / Execution | `/production`、`/production/{id}` | workspace execution 读模型 + 不可变 Execution 详情 | Next 生产构建 + 路由冒烟测试；Core workflow 集成测试 |
| 数据产品 / 就绪状态 | `/products`、`/products/{id}` | workspace 产品/release + Core ProductVersion/Release/Readiness | release 命令回归测试 + Next 构建 |
| Release 证据追溯 | `/products/{productId}/releases/{releaseId}` | 带作用域的产品/release 发现 + Core ProductRelease 可追溯性 | trace 作用域回归测试 + Next 构建/路由冒烟测试；Core 可追溯性测试 |
| 证据中心 | `/evidence` | workspace 产品/release；链接到 release 追溯 | Next 生产构建 + 路由冒烟测试 |

## 验收标准

### 无需直接操纵数据库的参考流程

UI 是建立在既有 Core 命令/读模型之上的操作员界面。复核确认/拒绝与 ProductRelease 发布均使用 server action；两者都不要求运维人员执行 SQL 或向核心 API 粘贴命令。

当前参考 POC 仍然需要通过 fixture/bootstrap 准备来创建底层的 DataResource、Dataset、Workflow、Contract、Rights、Quality、Compliance、ProductVersion 与 Release 对象。该 bootstrap 属于测试/POC 搭建，不是操作员 UI 的要求。

### 实体复核溯源

复核队列展示：

- 源值/原始值
- 归一化值
- 候选实体
- 匹配方法
- 匹配规则
- 置信度
- 存在时的引擎/模型溯源信息
- 策略版本

人工确认/拒绝必须填写原因，并使用服务端推导的 reviewer UUID。写操作默认禁用，且结果不明确时不会自动重试。

### Dataset 语义

Dataset 页面将逻辑 Dataset 类型（`RAW`、`STANDARDIZED`、`CURATED`、`PRODUCT`）与不可变 DatasetVersion 记录分别展示。历史版本仍然可以独立寻址。

### Release 就绪状态

产品页面独立展示以下门禁：

- production
- dataset
- rights
- quality
- compliance
- contract
- evidence
- delivery

UI 绝不会用一个单一的就绪得分替代它们。只有当 Core 报告 `Release.status=READY`、整体就绪为 `READY`、且每一项门禁均为 `PASS` 时，发布才被启用。

### 证据追溯

Release 追溯导航遵循：

```text
Workspace Data Product
  → ProductRelease
    → EvidenceSnapshot
    → DatasetVersion lineage
      → producing Execution
      → source Data Resource
    → EntityMatchJob / EntityMapping
    → Evidence
    → CostEvent
    → AuditEvent
```

只有在当前 workspace/product 下发现该 release 之后，才会执行全局可追溯性查询。渲染前会再次校验返回的 release/product/携带 workspace 的事实。

### 外部引擎只是实现细节

OpenMetadata、Hop 与 Splink 只能出现在其运行时/溯源信息相关的位置（例如 Execution 上的 `engineType`，或 EntityMapping 上的模型溯源）。它们不是一级导航概念，也不拥有核心状态。

## CI 验收层次

必需的 CI 门禁组合了互补的检查：

1. **Go platform CI** —— 迁移、`go test ./...`、API/worker/migrate 构建。其中包含 Core 中已有的参考垂直切片与可追溯性集成测试。
2. **Web 命令/作用域回归** —— 实体复核、release 发布与 release 追溯作用域测试。
3. **Next 生产构建** —— 校验所有 server/client 路由模块与类型集成。
4. **生产路由冒烟测试** —— 启动构建后的 Next 服务，对主要 POC 路由执行 HTTP GET，包括动态 product 与 release 追溯路由。本次冒烟测试中 workspace 配置刻意留空，以便在不需第二套实时 Core 栈的情况下走安全的 `SetupRequired` 路径。
5. **Hop/Splink CI** —— 既有的运行时/参考检查仍然强制执行，不会因 UI 变更而被绕过。

这些层次校验代码、API 契约、可构建性、路由注册与参考 Core 行为。它们不能替代针对具体部署环境的 IAM/安全测试。

## 最终实况演示检查清单

针对实际的 POC 演示环境，请配置真实的 `POC_WORKSPACE_ID`，并在服务端提供操作员身份之前保持写开关关闭。然后按顺序验证：

1. 工作台显示参考 workspace。
2. 依次打开每个输入 Data Resource 与 RAW DatasetVersion。
3. 打开实体复核，检查溯源信息，并带着原因完成所有待处理复核。
4. 打开由此产生的 STANDARDIZED/CURATED/PRODUCT DatasetVersion 以及产出它们的 Execution。
5. 打开数据产品，检查全部八个 Release Readiness 门禁。
6. 如果该 Release 在 Core 中为 `READY`，使用带守卫的 server action 将其发布。
7. 打开 Release 证据追溯，核实 EvidenceSnapshot 完整性、DatasetVersion 血缘、Execution 溯源、实体映射、Evidence、Cost 与 Audit。
8. 从 `/evidence` 导航到同一个 Release，以证明运维人员无需粘贴任意 UUID。

完成本检查清单，并配合全绿的必需 CI，即满足 #13/#75 的最小 POC UI 验收。生产 IAM 与对外部署加固仍不在本 POC 范围内。
