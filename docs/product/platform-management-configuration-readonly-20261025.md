# 当前空间管理入口的公开配置

2026-10-25，承接PR #363的`be449a1ac3b534f96c51bf67e20dc7d0d5e26fe4`。本片补齐基础配置的只读概览，以及配置→查询→明确详情→返回原列表/配置的完整路径。权威查询合同为[平台管理只读契约](../architecture/platform-management-readonly-v1.md#当前管理入口的公开配置)，不另建平行配置体系。保持Draft，不合并、不部署。

## 交付与复用

- 复用已经接入的OIDC/RP、当前workspace grant、只读ValidateInTx、DirectoryBinding、workspaceOverview和现有六项管理查询。仅增加最小公开投影；没有新增依赖、配置中心、环境查看器、配置表、迁移或新身份系统。无需为当前公开查询引入第二套OSS配置服务。
- `GET /api/v1/workspaces/{workspaceId}/management/configuration`不接受参数，身份前后无缓存确认、空间和revision比较、事务门禁保持。它只说明当前空间的管理查询接口，不声称全站写入、模型或执行配置。
- 仅投影范围、查询方式、目录来源状态、固定NOT_CHECKED、六个已配置入口和应用角色目录来源。没有原生目录/用户/审计调用，不把配置存在当连接成功或后续操作授权。
- DTO不包含原始或掩码连接、URL、环境键值、native组织/project引用、路径、credential、token、密钥存在状态或部分内容。前端严格核验字段集合、当前身份/空间/revision、scope/mode和writeEnabled=false，额外字段拒绝。
- 新版`/settings/configuration`沿用现有页面壳和业务用语，展示登录/会话范围和查询入口，无修改、保存、重载或启用按钮。`from=configuration`为闭集导航标记，只保留在Web侧，不进入API、不选身份/空间、不接受任意返回URL。
- 工作空间/组织入口及下游成员、用户、角色和审计查询保持配置来源；筛选、页码、详情刷新、原行定位、错误返回和移动端均走现有流程。原工作空间和组织来源路径保持。

## 预览与边界

默认新版预览为<http://127.0.0.1:4193/settings/configuration>，继续无需登录并显示真实来源未配置，不用夹具填充配置。独立管理验收为`https://localhost:4300/settings/configuration`，使用已有独立PG、真实RP SDK及合成OIDC/目录。构建目录为`.next-acceptance-identity-management-configuration-20261025`。替换进程前核验PID、启动时间、命令路径及监听端口，只更新所属API/Web和默认独立预览；原IdP、原库、独立PG、旧构建和其他预览保留。

真实来源继续UNCONFIGURED，实际业务对象、企业目录、部门层级和业务来源仍未确定。原OIDC/RP与Agent API保持，业务写入口、模型及真实执行关闭。配置页自身是当前管理查询概览，不提供或证明全局管理控制面。运行日志、企业部门层级、空间生命周期和管理配置写入继续未接入。

## 验证

| 项目 | 结果 |
|---|---|
| Go HTTP/投影与已有Native Adapter | PASS：配置不调用原生来源、不泄漏环境/native canary，跨空间、身份变化/撤销、门禁撤权/到期、未知身份源、密钥选择参数及写方法拒绝 |
| 独立PG scratch | 新建专用数据库，从既有77个迁移安装；management/principal/bindings/OIDC回归PASS，不修改原库或原验收身份 |
| Web | 生产构建、类型与lint通过；358项PASS，2项原有可选PG单测SKIP。已有CSS构建警告不算失败，也不宣称零警告 |
| 独立Edge管理路线 | 51项PASS，保留原43项并新增8组配置流程；页面错误、业务写请求、外部请求均0 |
| 公开配置与来源故障 | 页面刷新及API配置查询不调用成员/用户/组织来源；已配置来源发生故障仍为NOT_CHECKED，进入原查询正确失败；无secret或native引用投影 |
| 导航往返 | 六个固定入口刷新返回；配置→工作空间→筛选/第二页24条/成员详情/原行；配置→组织→用户第二页/详情/成员/历史返回/原行；PASS |
| 身份边界 | 跨空间切换只保留当前上下文，旧成员引用拒绝；撤权后前进/后退/刷新拒绝，恢复资格不复活旧会话；到期后不显示旧配置，PASS |
| 移动端 | 390px页面无横向溢出，查询表格可内部滚动，角色入口及返回可达，PASS |
| 最小权限和历史 | API SELECT-only、RP仅临时会话权限；审计事实摘要不变，execution/agent task/provider attempt/outbox为0 |
| 默认新版预览 | 1项PASS，8个菜单保持真实来源未配置，无合成人员/表格回退和写入口 |
| Agent与业务签收 | 更新后默认预览21步PASS，未发现页面错误、外部请求或业务写请求 |

本片仅证明合成接口与隔离用户路径，不证明真实企业接入、当前企业权限或任何写入启用条件。PR CI结果按最新提交另行报告，不将旧HEAD的16项成功或本地截图当作新提交的CI。

## 证据与重演

[51项结果](platform-management-configuration-readonly-20261025/browser-results.json)、[Agent与业务签收21步](platform-management-configuration-readonly-20261025/retained-routes.json)、[SHA256清单](platform-management-configuration-readonly-20261025/manifest.json)、[基础配置](platform-management-configuration-readonly-20261025/configuration.png)、[撤权拒绝](platform-management-configuration-readonly-20261025/configuration-revoked.png)、[移动端](platform-management-configuration-readonly-20261025/configuration-mobile.png)。清单分开记录捕获时工作树基线、源码摘要、构建ID和API二进制摘要；不把工作树或截图摘要宣称为运行提交认证。

隔离环境已经准备时，用新且未存在的`MANAGEMENT_BROWSER_OUTPUT=management-readonly-configuration-replay-20261025`运行`node tests/platform-management/browser.mjs`，默认使用独立Edge上下文。最短路径：基础配置→工作空间概览→空间成员→姓名和启用状态筛选→第二页→成员详情→刷新→返回原行→基础配置；再走目录组织→用户目录→第二页→详情→成员→浏览器返回→原行→基础配置。来源故障和资格撤回均必须有明确拒绝，不使用旧结果补成功。
