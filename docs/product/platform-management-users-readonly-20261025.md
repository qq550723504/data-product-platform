# 用户目录、账号资料与空间成员关系

2026-10-25。承接PR #363的`d2372f0680152b1fe1c0ef72fa76413f0aa10c97`，完成当前授权范围内的用户目录→明确用户详情→成员/角色查询→返回原详情及列表。查询合同唯一归属[平台管理只读契约](../architecture/platform-management-readonly-v1.md#用户目录与身份关联)。保持Draft，不合并、不部署。

## 交付与复用

- `/settings/users`接入现有管理RP和只读GET。复用shuomi-ai-commerce固定版本`e29058c56df4294e1c612b3a705ab767bc333188`的ReadHuman读侧、既有完整成员分页、literal搜索、权限前后确认、Casbin角色映射及平台bindings.Store；不导入写侧，不自建账号库或第二套IAM。
- 列表只显示当前组织/应用/工作空间可见的用户引用，按原生ID去重后搜索分页，25条一页。没有逐人资料请求或全局人员搜索。
- 详情单独读取原生账号资料。账号状态、成员关系状态、应用映射权限分别展示，不能互相推定。只投影最小资料，不返回邮箱、电话等原始字段。
- 身份关联必须来自服务器显式可信引用，再核对当前空间的actor/binding/grant。没有来源显示“未关联 / 关联来源未配置”；没有当前匹配显示“未关联”；不按登录名、邮箱或原生ID猜测平台身份。关联本身不授予权限。
- 列表返回保留搜索条件、页码并定位原行；详情刷新、成员/角色查询后的浏览器返回重新核验。不可见详情与越界分页有明确恢复入口，不自动选其他人。

## 运行和权限边界

默认新版预览仍为<http://127.0.0.1:4193/>，无需登录，系统设置真实来源继续未配置，不展示合成人员代替真实查询。独立管理验收为`https://localhost:4300/settings/users`，使用合成OIDC/原生目录、独立PG及已有最小权限角色。两者构建为`.next-acceptance-identity-management-users-final-20261025`。

仅在核验PID、启动时间、命令路径和监听端口后替换所属IdP/API/Web进程；独立PG、原库、旧构建与其他预览保留。独立库只增加一名明确合成人员关联，具有验收READ、没有管理READ；fixture控制只用于故障验收，不是产品写接口。原OIDC/RP身份协议、Agent API、业务依据契约和业务写入开关保持原有边界。真实来源UNCONFIGURED，业务写入口、模型与真实执行关闭。生产人员来源、身份映射owner及接入配置均未确认。

## 验证和已知限制

| 验证 | 结果 |
|---|---|
| Go领域、HTTP和原生Adapter | PASS：分页去重、全量搜索、作用域、精确资料、Human类型、原生账号状态、权限撤回、错误无部分数据和无私有字段 |
| 新独立PG scratch | 从迁移安装；管理查询、principal、binding、OIDC包PASS。显式native→pairwise subject→canonical actor关联、错空间、actor/binding/grant撤销与到期均覆盖；原库未使用 |
| Web | 完整生产构建和类型/lint通过，352项回归PASS，2项已有可选PG单测SKIP；另一次IAM回归25PASS/2SKIP，不把跳过算通过 |
| 独立Edge管理路线 | 26项PASS；既有成员、角色、审计18项路径保留，并增加8项用户流程；页面错误、外部请求、业务写请求均0 |
| 用户流程 | 25/25/8分页、全目录搜索、第二页详情/刷新/原行恢复、成员/角色往返、缺失和越界恢复PASS |
| 状态与身份 | 账号启用但成员停用、账号停用但成员启用、可信显式关联、无映射、来源未配置、被查询actor撤销后不泄漏关联PASS |
| 失效与隔离 | 切空间后只见新目录、复制旧用户拒绝；原生权限撤回/错owner/未知状态/源故障均丢弃；读取中撤销caller令牌或移除成员关系均无详情数据PASS |
| 最小权限与历史 | API SELECT-only，RP仅临时会话权限，原业务审计摘要不变；execution/agent task/provider attempt/outbox均0 |
| 默认新版预览 | 未配置来源入口1项PASS；8个菜单无假人员回退及写入口 |
| 保留路线 | 4193的Agent与业务签收21步PASS，无页面错误、外部请求或业务写请求 |

第一次运行发现返回链接的装饰箭头进入可访问名称，已修正并重新构建。第二次为测试手写URL参数顺序与固定导航序列不一致，已调整测试入口，未放宽条件或页码断言。两次失败记录保留，最终26项使用仓库正式用例通过。默认未配置入口最初误用非项目Playwright CLI导致测试收集失败，随后使用项目已锁定CLI完成正式1项验收；该失败不计为产品验收或通过。

目录仍是有界当前查询视图：原生offset读取没有共同快照/修订号，不能证明同总数并发替换的冻结快照或跨系统原子撤销。当前详情仅支持资源归属选定组织的Human账号；跨组织guest、机器账号、企业全局用户生命周期和真实映射来源不在本片。关联和角色投影不能用作业务写入准入或历史事实。

## 证据与重演

[26项Edge结果](platform-management-users-readonly-20261025/browser-results.json)、[摘要清单](platform-management-users-readonly-20261025/manifest.json)、[用户列表](platform-management-users-readonly-20261025/users.png)、[用户详情与显式依据](platform-management-users-readonly-20261025/user-detail.png)、[返回原页与原行](platform-management-users-readonly-20261025/users-return.png)、[移动端](platform-management-users-readonly-20261025/mobile.png)。证据捕获时HEAD为上方基线并带本轮未提交源变更，见manifest的source hashes；不把基线CI称为本轮提交CI，也不将源码摘要视为运行二进制认证。

独立环境已准备时设置新的`MANAGEMENT_BROWSER_OUTPUT=management-readonly-users-replay-20261025`，执行`node tests/platform-management/browser.mjs`，默认使用独立Edge。最短路线：用户目录→搜索“空间甲成员”→第二页→查看用户→刷新→返回用户目录，核对页码和原行；明确打开`person-a-7`与`person-a-5`核对账号/成员状态差异，`person-a-2`核对显式平台关联。真实来源启用需要独立确认，不从合成样例推定。
