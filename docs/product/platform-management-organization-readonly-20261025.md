# 当前目录组织的只读资料与人员查询

2026-10-25，承接PR #363的`83e27ff91c8d166a7e56ae1aa8da9b4b16f4ca6f`。完成组织资料→当前空间成员/用户→明确详情→原页和组织资料返回。唯一查询合同为[平台管理只读契约](../architecture/platform-management-readonly-v1.md#当前目录组织资料)。保持Draft，不合并、不部署。

## 交付与复用

- 复查固定shuomi-ai-commerce提交，没有找到独立组织资料Reader；复用已经接入的transport、原生权限前后确认、TLS、错误投影和成员/用户完整流程。新增最小OrganizationReader Port，沿用现有ZITADEL的官方Organization v2查询，没有新依赖、SDK、账号/组织表、迁移或组织同步服务。
- 新增`GET /api/v1/workspaces/{workspaceId}/management/organization`，组织ID来自服务器当前空间绑定。原生查询使用固定单ID过滤、limit=1，禁止全局搜索或由URL/body选择其他组织；结果必须恰一条、总数一致、ID匹配。用户的home组织和目录object.resourceOwner不用于推定企业隶属。
- 名称、目录状态、最小日期字段经核验后展示；缺失名称/日期显示未提供，未知状态或非法资料拒绝，不从工作空间配置猜测组织名。原生domain、metadata和owner等字段不投影。
- `/settings/organizations`展示身份目录组织资料，企业部门层级来源保持未配置，不生成树、不推断法定主体或负责人。目录启停状态和平台访问资格分开核验。
- 复用原成员/用户筛选、分页、详情与返回；仅增加固定`from=organization`导航来源。组合条件、第二页、原行定位、刷新、用户→成员往返保持；错误页面仍可返回组织资料。标记不进API，不选择workspace/组织/身份，不接受任意返回URL。
- 继续使用现有PLATFORM_MANAGEMENT_READ、OIDC/RP、前后无缓存身份确认和只读事务内ValidateInTx；本地或原生权限在读取中撤销均丢弃资料。切空间经原登录核验后读新绑定组织，旧成员引用拒绝。

## 预览与边界

默认新版预览<http://127.0.0.1:4193/settings/organizations>继续无需登录，真实来源未配置，不用合成组织填充。独立管理验收为`https://localhost:4300/settings/organizations`，使用原独立PG、合成OIDC及原生目录夹具。Web构建为`.next-acceptance-identity-management-organization-final-20261025`；仅在核验PID、启动时间、命令路径和监听端口后替换所属IdP/API/Web，原库、独立PG、旧构建和其他预览保留。

真实场景、组织、责任和企业部门来源仍待定；真实来源UNCONFIGURED，业务写入口、模型、真实派发关闭。目录组织不等于企业部门树、法定主体或平台空间。原OIDC/RP、Agent API及业务签收/验收契约保持；创建组织、调整部门、邀请成员和修改授权均未启用。原生POST `_search`是只读查询，不是业务写接口。

## 验证

| 项目 | 结果 |
|---|---|
| Go领域、HTTP和Native Adapter | PASS：精确组织、单ID过滤、总数与结果一致性、稀疏资料、未知状态/非法资料、权限前后撤回、拒绝跨空间/外部选择和写方法、不投影私有字段 |
| 新独立PG scratch | 从迁移安装；管理、principal、bindings及OIDC包回归PASS，未使用原库 |
| Web | 完整生产构建、类型与lint通过；356项PASS，2项已有可选PG单测SKIP，不将跳过算通过 |
| 独立Edge管理路线 | 最终43项PASS，保留原34项并新增9项组织流程；页面错误、业务写请求、外部请求均0 |
| 查询往返 | 组织→成员筛选/第二页24条/详情刷新/原行返回/清除；组织→用户第二页/详情/成员/浏览器返回/原行恢复；移动端入口及返回，PASS |
| 来源异常 | 原生org.read撤回、错组织、源故障、未知状态、额外记录、不可见组织和伪造参数全部拒绝；稀疏名称与日期保持未提供；目录停用状态不改写空间状态，PASS |
| 身份与空间 | 切换后只显示新组织，复制旧成员详情拒绝；读取中caller令牌撤销401无data，原生org.read撤销503无data；PASS |
| 最小权限和历史 | API SELECT-only、RP仅临时会话权限保持；审计事实摘要不变，execution/agent task/provider attempt/outbox均0 |
| 默认新版预览 | 未配置来源入口1项PASS，8个菜单无合成人员/表格回退或写入口 |
| 保留路线 | 更新后4193的Agent与业务签收21步PASS，无页面错误、外部请求或业务写请求 |

首轮新增Adapter正例因测试辅助服务未授予org.read而失败，已修正该测试服务的权限配置，未放宽产品校验。浏览器首轮43项已通过；截图复核后将组织页通用人员说明改为组织资料说明，使用最终独立构建再次43项通过。旧构建和第一次截图保留，最终交付不混用旧截图。

本片仅证明合成接口契约和隔离运行路径。实际企业身份源、原生API版本及服务账号权限仍未接入验收，不把fixture、配置组织引用、当前读取或测试通过当作企业事实、部门归属或后续写入授权。

## 证据与重演

[最终43项结果](platform-management-organization-readonly-20261025/browser-results.json)、[SHA256清单](platform-management-organization-readonly-20261025/manifest.json)、[组织资料](platform-management-organization-readonly-20261025/organization.png)、[成员原页与原行恢复](platform-management-organization-readonly-20261025/organization-members-return.png)、[移动端](platform-management-organization-readonly-20261025/organization-mobile.png)。捕获时的基线与未提交源摘要独立记录，源码摘要不是运行二进制认证，不沿用旧HEAD的CI。

独立环境已准备时，设置新的`MANAGEMENT_BROWSER_OUTPUT=management-readonly-organization-replay-20261025`运行`node tests/platform-management/browser.mjs`，使用独立Edge上下文。最短路线：组织机构→当前空间成员→搜索“空间甲成员”与启用状态→第二页→明确成员→刷新→返回原行→组织资料；再走用户详情与成员往返。真实部门层级需明确独立来源后另行设计，不从目录组织推定。
