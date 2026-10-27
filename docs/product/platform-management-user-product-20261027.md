# 用户资料产品接入与隔离验收

2026-10-27。范围：为实际 cmd/api 的 QUERY_ONLY 产品组合增加逐空间默认关闭的用户资料开关，复用现有只读查询，打通成员→账号详情→原列表。真实来源、平台身份映射仍 UNCONFIGURED，默认预览未切换。PR #363 保持 Draft、不合并；原库、已有 OIDC/RP 与 Agent API 保留，业务写入、模型和真实执行关闭。

## 唯一合同与复用决定

[装配合同第11节](../architecture/platform-management-main-service-readonly-v1.md#11-用户资料的产品级只读装配)拥有私有配置和依赖；[管理查询契约](../architecture/platform-management-readonly-v1.md)拥有身份、空间、公开API及返回规则；[来源责任清单](platform-management-source-admission-20261026.md)拥有真实材料和接纳，不从合成样例填写真实字段。本页只记录实现与验证。

Build-vs-Buy / Reuse Check：仓库已有 UserReader、QueryUsers、ZITADEL原生查询Adapter、Casbin角色Catalog、OIDC/RP及PostgresGate，满足本轮范围。继续复用源项目的查询模式和既有Ports；成熟标准方案是 ZITADEL 原生只读 API 与 OIDC。无需引入第二套IAM/权限引擎、同步器或采集服务；新增部分仅为平台逐空间装配语义与页面导航，不另造用户系统，不新增依赖、migration或业务Command。平台身份映射不装配，不用邮箱、登录名或同名组织推断。

## 页面与关闭边界

- 成员列表保留现有“查看成员”，增加“查看账号”；成员详情可查看账号资料。账号页返回原成员列表，恢复搜索、角色、状态、页码、固定来源入口及原行焦点；账号页中的对应成员也回原筛选详情。
- 用户目录本身保留搜索、25项分页及详情往返；共享焦点组件移到实际包含用户行的列表，修复原先挂在详情中导致的返回焦点缺失。身份未配置只显示“关联来源未配置”，避免重复标签拼接。
- memberContext 仅为页面导航，无空间/身份/URL参数或缓存账号；解析拒绝歧义、嵌套和不属于当前账号的成员关系。API query whitelist 不透传它。刷新与浏览器历史仍完整GET重新核验。
- userProfiles 省略/false不装配Users，公开 users=false，HTTP列表与详情均503且不触源；显式true复用当前空间同一原生Client。IdentityLinks始终为空。CONFIGURED仅为配置声明，availability仍NOT_CHECKED，真实来源未接纳。
- 用户API读取前后仍核验可信caller、workspace、revision、管理读资格与期限，并核验原生用户/成员权限和匹配组织。来源失败也先执行后置身份门禁，不泄漏资料或内部错误。读取中成员移除、撤权或到期不返回旧资料。

## 独立环境与重演

使用全新 `.data/management-user-product-20261027`、PostgreSQL `dpp-management-user-product-20261027` / `management_user_product_20261027`，仅隔离合成源。固定loopback端口：PG25620、实际产品API18620、合成OIDC/目录19220、新版Web4420。产品进程只使用私有 PLATFORM_MANAGEMENT_PROFILE，无夹具路由；测试控制端只在tests合成来源。API最小SELECT权限，RP仅技术会话写权限。

仅在新的验收执行器首次建立该固定环境（已有环境禁止覆盖）时：设置 MANAGEMENT_FIXTURE_ACK=ISOLATED_READONLY_MANAGEMENT 和 MANAGEMENT_USER_PRODUCT_FIXTURE_ACK=ISOLATED_USER_PRODUCT_READONLY，依次运行 tests/platform-management/bootstrap.py、test-main-service.py、directory-product-contract.py；设置独立 MANAGEMENT_WEB_DIST 后运行 start-web.py --build，再运行 user-product-browser.mjs 和 verify-main-service.py。浏览器本地默认Edge，CI显式 MANAGEMENT_BROWSER_CHANNEL=chromium。每次证据目录须全新，不延长旧凭据或重置旧会话。

浏览器重演：合成查询人员A→空间成员→搜索“空间甲成员”/管理查询员/启用→第二页→查看账号→刷新→返回成员列表；核对条件、页码、行与焦点。分别从工作空间/组织/配置入口重复，核对对应成员详情及固定返回。双空间人员在B打开A复制链接应无资料；空间C目录可读但用户资料关闭。账号person-a-5停用不等于成员停用；person-a-7账号启用不等于成员启用；person-a-2有合成同人候选仍不得关联平台人员。

## 验证记录

| 验证 | 结果与证据 |
|---|---|
| Go与真实隔离PG | 6个包293个pass事件（含子用例），0 fail/skip；[完整结果](platform-management-user-product-20261027/go-regression.json) |
| 实际产品入口 | 15项PASS，含错误开关类型与关闭目录的实际子进程；[结果](platform-management-user-product-20261027/product-contract-results.json) |
| 最终新版构建 | Next生产编译、类型和lint成功；已有CSS提示不宣称清零 |
| 用户资料浏览器 | 最终Edge和Chromium各20项PASS；[Edge](platform-management-user-product-20261027/browser-edge-results.json)、[Chromium](platform-management-user-product-20261027/browser-chromium-results.json)，page errors、业务写请求、外联均0 |
| Web身份/导航单测 | 33 PASS / 2可选PG SKIP，包含导航解析、来源标记、固定返回、DTO成员对应；不把skip算PASS |
| 容器保留 | 普通和优化Python各13项PASS；原库及先前管理/日志/目录容器元数据最终比较PASS，[安全报告](platform-management-user-product-20261027/retained-final-comparison.json) |

[账号详情](platform-management-user-product-20261027/account-detail.png)、[成员返回](platform-management-user-product-20261027/member-return.png)、[用户返回](platform-management-user-product-20261027/user-return.png)、[身份未配置](platform-management-user-product-20261027/identity-unconfigured.png)、[移动端](platform-management-user-product-20261027/mobile.png)。导出仅安全JSON、截图与内容清单，不含私有profile、DSN、密钥、token或原始日志。CI状态必须按本轮最终HEAD另查，在PR闭环评论记录，不继承前轮绿色；合成PASS不等于真实来源接纳或写入授权。

本地首次测试发现验收脚本按query排列顺序比较URL，以及精确按钮名漏计箭头；两项均为断言问题，改为完整参数集合比较和明确链接定位，保留首次失败JSON。一次Go/PG运行因新环境未纳入既有固定目标识别而跳过1项；已修正识别，最终必须0 skip。构建准备期间一份临时候选审计展示角色多带users.read，正式bootstrap已修正为仅audit.read；复核角色配置并更换经PID/创建时间/可执行文件/监听核实的本轮独立API后重验，两版私有配置和原证据均保留。原生角色展示不生成caller资格，产品业务权限未扩大。保留报告一次使用的文件名不合闭集规则，脚本拒绝执行，改用合法新证据名后完成，不放宽检查。旧readiness单测本地输出目录已存在，拒绝覆盖，未计作本轮PASS；共享导航未修改，其CI仍在全量回归中运行。

后续：真实组织/目录版本、人员、来源责任、受控凭据及运行授权仍待定。真实读取、平台身份关联、用户/角色管理写入、同进程全站组合、模型和执行均不由本轮自动开放。
