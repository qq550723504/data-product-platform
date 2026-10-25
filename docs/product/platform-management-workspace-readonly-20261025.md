# 当前工作空间的只读管理入口

本页保留该轮34项验收与旧构建。当前预览及新增目录组织资料见[组织只读交付](platform-management-organization-readonly-20261025.md)，不改写下述历史证据。

2026-10-25，承接PR #363的`e51670d1787b987217c0dd3020b9dfc25330f0fa`。本轮完成当前空间概览→成员/用户/角色/审计查询→原列表或空间返回。权威查询合同为[平台管理只读契约](../architecture/platform-management-readonly-v1.md#当前工作空间概览)，保持Draft、不合并、不部署。

## 已完成与复用

- `/settings/workspaces`读取新增的`GET /api/v1/workspaces/{workspaceId}/management/workspace`，复用原OIDC/RP、当前workspace grant、无缓存前后身份核验和PG只读事务内ValidateInTx，不新增账号/工作空间表、迁移、依赖或通用IAM。
- 修正空间概览与目录配置的职责耦合：空间概览独立核验身份与空间资格，人员目录未配置时仍可展示已验证的当前范围。其余原有查询路径要求保持。身份来源必须由服务器显式配置，与目录来源分开。
- 空间名称继续来自当前应用配置，页面明确说明来源；正式空间档案尚未接入，负责人和配额留空。没有把原生组织、人员归属或权限枚举当作空间档案或生命周期事实。
- 四个查询入口复用现有列表与详情。`from=workspace`仅作为固定页面导航标记，在搜索、筛选、分页、清除条件、详情、角色相关成员和审计游标往返时保留；不会进入API参数，不接受任意返回URL或客户端指定空间。返回空间时重查当前会话范围。
- 接入配置不冒充健康检查：概览不请求原生目录、账号或审计列表，目录状态为“已配置/未配置”，可用性始终未检查。点击后按原查询独立核验，故障显示不可用，并保留返回空间入口。
- 切换空间继续使用现有身份页面及重新登录；配置中的候选名称不证明当前人有权进入。旧空间详情和游标仍受原边界约束，撤权、到期和旧revision拒绝，不从旧页面恢复访问。

## 预览与未开放事项

默认新版预览<http://127.0.0.1:4193/settings/workspaces>无需登录，真实来源仍未配置，不以合成人员填充。独立管理验收页面为`https://localhost:4300/settings/workspaces`，使用原独立PG和合成OIDC/目录；两处Web构建为`.next-acceptance-identity-management-workspace-final-20261025`。仅在核验PID、启动时间、命令路径及监听端口后更换所属预览进程；旧构建和证据保留。

原库、原OIDC/RP协议与Agent API保持。真实业务场景、组织、责任与来源仍待定；真实来源UNCONFIGURED，业务写入口、模型和真实派发关闭。没有新建空间、邀请成员、修改授权、设置配额或归档入口。企业组织树、正式空间档案与生命周期需要后续独立契约和授权，本轮不实现。

## 验证

| 项目 | 结果 |
|---|---|
| Go查询及HTTP | PASS：无目录配置仍可读合法空间概览；有配置也不读取原生目录；无参GET、作用域、来源、撤权、旧revision、过期及写方法拒绝 |
| 新独立PG scratch | 从迁移安装，管理/身份/绑定/OIDC回归PASS；真实ValidateInTx验证有效期受grant约束、撤权拒绝、恢复不复活旧revision、更新后的身份可读及grant过期拒绝；原库未使用 |
| Web | 完整生产构建、类型与lint通过；354项PASS，2项已有可选PG单测SKIP，不把跳过计为通过 |
| 独立Edge管理路线 | 34项PASS：保留26项并新增8项空间流程；页面错误、业务写请求、外部请求均0 |
| 空间导航 | 成员组合筛选/第二页/详情刷新/原行返回；用户详情→角色→相关成员→空间；审计游标/最新/刷新；均保留固定来源并PASS |
| 来源与权限边界 | 概览不发原生目录请求；目录故障仍可核对配置但查询明确失败；跨空间复制旧用户拒绝、返回只见新空间、伪造来源/空间参数拒绝；PASS |
| 撤权与到期 | 当前grant撤销后前进/后退与刷新均无概览；恢复grant旧会话仍拒绝；新登录恢复；合成令牌到期后刷新无概览、须重新登录；PASS |
| 最小权限与历史 | 原API SELECT-only、RP仅临时会话权限保持，审计事实摘要不变，execution/agent task/provider attempt/outbox均0 |
| 默认新版预览 | 未配置来源入口1项PASS，8个菜单没有假人员/表格回退或写入口 |
| 保留路线 | 更新后4193的Agent与业务签收21步PASS，无页面错误、业务写请求、外部请求 |

首次浏览器停止于测试把否定说明句中的“连接正常”误判为健康状态。最终文案简化为“进入查询时重新核验资料可用性”，并将用例改为检查不存在该独立状态标签；重新构建后34项通过。首次失败单独保留，不计为通过。未放宽身份、空间、分页或往返断言。

## 证据与重演

[34项Edge结果](platform-management-workspace-readonly-20261025/browser-results.json)、[摘要清单](platform-management-workspace-readonly-20261025/manifest.json)、[空间概览](platform-management-workspace-readonly-20261025/workspace.png)、[撤权拒绝](platform-management-workspace-readonly-20261025/workspace-revoked.png)、[移动端](platform-management-workspace-readonly-20261025/workspace-mobile.png)。捕获基线和未提交源摘要分别保留；源摘要不是运行构建的身份认证，也不沿用上一提交CI。

独立环境已准备时，用新的`MANAGEMENT_BROWSER_OUTPUT=management-readonly-workspace-replay-20261025`运行`node tests/platform-management/browser.mjs`，使用独立Edge上下文。最短路径：工作空间→空间成员→搜索和状态筛选→第二页→明确详情→刷新→返回原行→返回工作空间；再走用户、角色和审计入口。默认4193继续使用原启动脚本，企业真实接入不能从合成验收推定。
