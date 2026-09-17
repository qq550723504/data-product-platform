# POC ProductRelease UI

本次增量面向企业活跃度（Enterprise Activity）POC，新增了运维人员可用的数据产品详情页、ProductRelease 就绪状态展示，以及带守卫的发布动作。

## Core 始终是权威

浏览器从不计算或修改 release 状态。它读取 Core 的 `ProductRelease` 与 `ReadinessResult`，独立展示每一个门禁，并且仅在 Core 已经报告以下条件时才调用既有的发布命令：

- `ProductRelease.status = READY`
- `ReadinessResult.overall = READY`
- 每一项就绪检查均为 `PASS`

UI 展示的八项检查为：

1. production
2. dataset
3. rights
4. quality
5. compliance
6. contract
7. evidence
8. delivery

Release 不会被表示为一个不透明的整体质量/就绪得分。阻塞项代码（blocker codes）与 Core 可提供的就绪详情始终对运维人员可见。

## 作用域保护

产品导航从 workspace 作用域的核心读模型开始。在执行任何发布写操作之前，服务端 action 会校验：

- 该产品确实由所配置的 workspace 返回；
- 该 release 确实出现在该产品 workspace 作用域的 release 历史中；
- 全局不可变 release 详情仍引用同一个产品；
- 就绪响应仍引用同一个 release。

这些检查用于防止 POC UI 意外提交任意的 product/release UUID。它们是控制台的纵深防御，并不能替代 API 的身份认证与授权。

## 发布配置

Release 写操作默认禁用：

```text
POC_ENABLE_RELEASE_ACTIONS=false
POC_RELEASE_ACTOR_ID=
```

仅用于可信的本地/私有 POC 时：

```text
POC_ENABLE_RELEASE_ACTIONS=true
POC_RELEASE_ACTOR_ID=<operator UUID>
```

`POC_RELEASE_ACTOR_ID` 在服务端读取。浏览器无法提供或覆盖 `X-Actor-ID`。

功能开关与配置的 UUID 不是 IAM。在多用户或对外部署之前，actor 身份必须来自经校验的会话，并且 Core 必须强制校验 workspace 归属与命令权限。

## 幂等性与结果不明确的处理

每一次显式发布提交都会由服务端生成一个 `Idempotency-Key`，并调用：

```text
POST /api/v1/product-releases/{releaseId}/publish
```

UI 绝不会自动重试发布。如果 POST 超时，或返回的载荷无法被校验，则该结果被视为未知：会告知运维人员先重新加载并检查 Core 状态，然后再尝试其他命令。

诸如 `PRODUCT_RELEASE_NOT_READY` 或 `IDEMPOTENCY_KEY_CONFLICT` 的 Core `409` 会被展示为状态冲突，同样要求刷新/重新检查。

## 本次增量不做的事

本 UI 不创建也不修改：

- Data Contract 版本
- Rights 快照
- Quality 结果
- Compliance 结果
- ProductVersion 定义
- DatasetVersion

这些仍属于各自独立的核心工作流。产品详情页会暴露冻结点所引用的 ID 与交付资产，以便理解每一项就绪门禁为何通过或被阻止。

Release → DatasetVersion → Execution → 来源/证据的追溯导航，以及最终的实况浏览器 POC 冒烟测试，仍属于父 issue #75 的一部分。
