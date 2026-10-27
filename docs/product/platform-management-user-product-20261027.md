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

仅在新的验收执行器首次建立该固定环境时，设置 `MANAGEMENT_FIXTURE_ACK=ISOLATED_READONLY_MANAGEMENT`，运行 `python tests/platform-management/run.py --suite user-product`。原 Go/PG、产品配置合同、独立 Web、浏览器和最终保留比较保持执行；入口在成功、失败或中断后停止本轮资源并保留数据与证据。已有环境禁止覆盖、延长凭据或重置会话。本地默认 Edge，CI 显式 Chromium；详见[验收收尾契约](../operations/acceptance-shutdown.md)。

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


## 本轮复审与补修

复审基线为 `3c1fa691aeb9ee6b138057d9b0998399f189d346`，只核对用户资料的关闭、身份/空间边界和列表往返，不启用真实来源或扩大业务范围。

| 问题 | 已复现的影响 | 根因修复 |
|---|---|---|
| RP登录期限被异步确认延后 | SDK寿命快照在确认后才换算成绝对时间；耗尽的登录仍可能保存 | 确认前冻结原绝对期限；确认后到期拒绝，不续期 |
| RP读取中到期或注销 | current、管理client构造、请求前及返回前，复核完成后可能接受已到期/被注销的本地会话 | 复核后重读同一加密会话并核对两项期限；无新会话/令牌回退 |
| 有效会话的浏览器后退丢失焦点 | 筛选和第二页仍在，键盘焦点离开原行 | 复用原显式返回的hash及原生replaceState标记离开行，重新授权渲染后仅聚焦现有TR |

复用决定：使用现有SessionStore查询、成熟OIDC/RP和ManagementRowFocus，不引入新登录系统、缓存、依赖、表、队列、业务API或权限。导航只保存URL行片段，保持原history.state，无账号资料或token进入WebStorage/历史状态。默认关闭及IdentityLinks不装配的产品代码不变；原库、Agent API及默认预览不改动。

回归在补修前已记录失败：期限的6个子场景、注销的4个子场景，以及浏览器有效后退焦点失败。最终回归和保留结果见新增review证据；先前20项与旧CI属于首片历史结果，本轮CI须按最新HEAD单独核对并在PR评论记录。


复审最终本地验证：

- [SDK/TLS回归](platform-management-user-product-20261027/review-unit-after.json)：47个测试事件，45 PASS / 2可选PG SKIP / 0 FAIL；[到期补修前失败](platform-management-user-product-20261027/review-unit-before-expiry.json)、[注销补修前失败](platform-management-user-product-20261027/review-unit-before-logout.json)保留，含父测试状态，不把父/子失败当作不同缺陷。
- [独立PG/Go](platform-management-user-product-20261027/review-go-regression.json)：293个pass事件，0 fail/skip；默认关闭、逐空间配置与IdentityLinks不装配保持。
- 最终新版生产构建与lint/typecheck成功；[日志前端单元](platform-management-user-product-20261027/review-runtime-unit-after.json)12项PASS。仅更换本轮经过PID/创建时间/绝对入口/监听核验的4420隔离Web，旧构建/进程账册保留，API、IdP及库不替换。
- [Edge](platform-management-user-product-20261027/review-browser-edge.json)、[Chromium](platform-management-user-product-20261027/review-browser-chromium.json)各29项PASS；新增有效后退/前进、无效片段、正常来源与来源失败两种情况下的5类权限失效，以及Web读取中撤销/到期。页面错误、业务写请求、外联均0，业务事实摘要不变。[补修前焦点失败](platform-management-user-product-20261027/review-browser-focus-before.json)保留。
- [最终容器保留](platform-management-user-product-20261027/review-retained-final.json)PASS，证明列出的旧容器元数据相同；历史original别名前后均不存在，不冒充原库内容验证。原库不运行SQL/DDL，真实来源/身份映射仍未配置，默认预览未接入，业务写入口/模型/真实派发关闭，Draft不合并。

本轮代码的全量CI及跨模块共享组件回归必须以最新HEAD为准，在PR收口评论登记，不继承首片3c1fa691的绿色。风险限制：这里只证明隔离合成身份与资料查询边界，不证明企业真实用户、组织、权限来源或正式签收。


## 系统设置产品入口整体验收

范围冻结于 `10f99104daf049c4241312e1d32cd7b36e683aa2`：复用既有实际cmd/api产品组合与新版Web，在同一独立合成宿主核对九个菜单及跨页流程；不新增菜单、API、权限或业务写能力。[能力清单](leader-demo-capabilities-20261025.md#系统设置当前能力)统一区分“已实现”“已装配/合成见证”“真实来源未配置”。来源与实际责任仍由原清单拥有。

本轮发现并先复现：配置→角色→成员→账号→查看角色权限，丢失from入口。账号页现在为次级角色/其他成员链接复用memberContext中的来源；主返回仍恢复原筛选/页码/焦点，来源只作导航不进入API。工作空间、组织和配置三条真实页面入口分别覆盖，不用手写目标URL代替整条路线。

独立环境沿用固定4420/18620产品宿主，API/IdP/库不替换。增加D仅为合成无来源空间：CI新库prepare直接种入；本地既有独立库仅增加both合成主体对D的管理读资格，复用该主体原有有效窗口，未延长任何已有资格或凭据，其他grant不变。产品私有配置无D目录/审计绑定，IdentityLinks仍不装配，D不会继承A/B默认来源。该测试资格不是selected native user→平台人员映射，更不代表真实企业授权。

覆盖计划：总览及所有侧栏菜单、配置/空间/组织到角色/成员/账号并回跳、实际审计分页与跨调用者游标、D全部来源关闭、目录故障但独立审计可查、全菜单资格撤回/到期、原29项用户资料边界。运行日志只检验实际产品未接入状态；其记录查询20项仍由独立日志宿主证明，不在产品中暗接合成Reader。默认4193及原库不变，不因本轮UI/CI接纳来源或开放写入。


整体验收最终本地结果：

| 项目 | 本轮证据 |
|---|---|
| 同一产品入口全流程 | [Edge 37组](platform-management-user-product-20261027/whole-browser-edge.json)、[Chromium 37组](platform-management-user-product-20261027/whole-browser-chromium.json)全部PASS；其中一组循环九个菜单，不冒充九条独立业务验收。原29组保留，新增8组整体流程 |
| 边界与事实 | 页面错误/业务写请求/外联均0；审计及execution/agent/provider/outbox等业务表数量与内容摘要不变；所需测试资格只在独立合成库准备 |
| 权限与查询基础 | [Go/独立PG](platform-management-user-product-20261027/whole-go-regression.json)293个pass事件、0 fail/skip；[SDK/TLS](platform-management-user-product-20261027/whole-unit-results.json)45 PASS/2可选PG SKIP，生产构建、类型及lint成功 |
| 回跳修复 | [修复前失败](platform-management-user-product-20261027/whole-origin-before.json)保留；配置/空间/组织从真实页面入口到账号、角色并返回均通过，原筛选/页码/焦点回归保持 |
| 来源与能力状态 | [配置页面](platform-management-user-product-20261027/whole-configuration.png)、[无来源空间](platform-management-user-product-20261027/whole-unconfigured.png)、[实际审计](platform-management-user-product-20261027/whole-audit.png)；Config只声明入口、空间档案仍未接入、身份映射未装配、日志产品来源未配置 |
| 保留检查 | [旧容器元数据](platform-management-user-product-20261027/whole-retained-final.json)PASS；历史original别名前后均不存在，不冒充原库内容验证；默认4193与原库SQL/DDL未操作 |

修正既有能力清单中“日志权限/API/Reader均未实现”的过时描述，改为：已有源码/独立合成证据与产品配置、真实接入分别列示。没有将Loki候选、合成空间/账号或材料表中的待定项改为真实接纳。最新HEAD的CI及本轮37组产物仍须单独核对，在PR收口评论记录；不继承10f99104绿色。PR保持Draft，业务写入口/模型/真实派发关闭。


### 本轮CI离线构建诊断补齐

系统设置HEAD `2b0eb7c7` 的实际产品专项在CI已通过293/15/37；另一个[demo作业121977689487](https://github.com/qq550723504/data-product-platform/actions/runs/40439945319/job/121977689487)在离线检查器Go构建阶段失败，原测试捕获stderr却未输出，0个完整入口用例执行。后续产物上传也超时；这些信号不能证明旧构建失败的具体原因，保留UNKNOWN，不把旧失败改为PASS，也未原地重跑旧作业。

独立CI维护补修只为demo作业显式使用与主CI一致的Go1.26.10及现有go.sum缓存，避免依赖runner默认Go；离线测试在构建失败时继续抛错，新增固定类别、退出码、stdout/stderr字节数及SHA256诊断，绝不公开原始输出、代理URL或环境。类别只作定位线索，不裁定根因；未知错误仍失败，不新增编译重试或放宽权限。测试临时目录在setup失败时也由已有TemporaryDirectory机制清理，冻结候选和provider逻辑不改。

普通/优化Python各7项PASS，包含真实编译检查器的原5项完整入口回归，以及2项诊断不泄漏/未知失败阻断/不重试反例。Docker为记录替身、策略读取为文件，provider/原库I/O均0。[安全摘要](platform-management-user-product-20261027/whole-ci-build-diagnostic.json)保留。此维护不修改系统设置代码、来源、API、OIDC/RP、真实运行或业务写入口；新HEAD全量CI另行核对，旧2b0eb7c7失败仍保留。

### 身份验收作业时限修正

后续HEAD `321764c1` 的demo作业和37组产品专项已通过；但[身份验收作业121981818235](https://github.com/qq550723504/data-product-platform/actions/runs/40441064492/job/121981818235)被GitHub明确判定超过20分钟上限，目录产品入口准备被取消、24组浏览器用例未执行，不能声称该HEAD全量通过。该作业依赖安装约7分8秒，前置多套隔离API/Web及浏览器验收均成功，至最后目录准备时耗尽总预算。

本次仅将该作业总时限20→30分钟，全部步骤、断言、权限、来源默认值和产物保留不变，不增加自动重试，也不原地重跑已取消作业。[原始取消的安全摘要](platform-management-user-product-20261027/whole-ci-timeout.json)保留；下一HEAD须重新完成全部CI和目录浏览器产物核对后，才能在PR收口评论记录完成。
