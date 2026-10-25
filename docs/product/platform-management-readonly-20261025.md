# 新版系统设置：管理查询接入记录

2026-10-25。本增量复用 shuomi-ai-commerce 固定来源 `e29058c56df4294e1c612b3a705ab767bc333188` 的目录适配器、筛选分页合同、角色权限投影方式和审计查询模式。查询与身份合同唯一入口为[平台管理只读契约](../architecture/platform-management-readonly-v1.md)。PR #363 保持 Draft，不合并。

后续身份/TLS/权限白名单修复、当前构建与14项管理验收归[本轮复审记录](platform-management-readonly-review-20261025.md)。下方13项结果和截图属于首轮接入证据，保持其历史范围。

## 可见结果

- 新版预览 <http://127.0.0.1:4193/settings> 保留无需登录体验，显示“真实来源未配置”。导航接入新的设置查询页，不把原本地设置样例当真实管理后台。
- 独立验收页面 <https://localhost:4300/settings> 使用现有 OIDC/RP SDK和加密 PG 会话，增加独立的管理查询 profile。展示两个合成空间的成员、角色权限和58条审计记录；没有连接企业人员目录、原库或电商项目运行环境。
- 成员支持全目录筛选后分页、启停状态筛选、明确成员详情和刷新恢复；应用角色仅展示明确配置的权限；审计按原始引用翻页追溯，不返回metadata或前后状态载荷。
- 用户目录、组织机构、真实工作空间生命周期、运行日志与基础配置尚未接入。没有新增账号创建、邀请、授权修改、配置保存、业务签收或执行入口。

## 复用及实际补齐

| 范围 | 本次实际处理 |
|---|---|
| 成员目录 | 定向抽取来源的Client/Query/Permission与对应测试，通过Port映射服务器配置的空间→组织/项目，保留完整分页与前后原生权限探针；HTTPS和过程CA沿用本平台边界 |
| 角色权限 | Casbin v2.128.0复用来源模型；治理平台管理查询目录替换电商模块。应用角色定义不冒充企业自定义角色，也不替代Core资格与业务门禁 |
| 审计 | 借鉴来源的scope-bound cursor、稳定排序、allowlist投影和失败语义；直接查询本平台AuditEvent，不迁移电商审计owner/资源账本 |
| 身份与会话 | 继续使用已有openid-client/Go OIDC、canonical actor、workspace grant和ValidateInTx；业务验收与管理profile分别投影能力，旧会话不能跨profile复用 |
| 页面 | 沿用新版布局与样式，服务端读取保护人员和审计数据；查询导航使用完整请求，避免恢复客户端路由缓存中的旧记录。没有引入Auth.js或第二套前端身份/查询缓存体系 |

新增migration77只增加独立查询能力枚举；仅应用到全新`management_readonly_20261025`验收库，原库未迁移。API角色仅SELECT身份映射与审计4表，RP角色只读写两张临时会话表。验收库里的准备与撤权由测试控制执行，不是产品写API。

## 验证与局限

| 验证 | 本地结果 |
|---|---|
| 目标Go包及来源目录回归 | PASS；包含跨组织/项目、完整筛选、取消、读后权限撤销和无权限不得冒充空目录 |
| 完整Web构建与回归 | PASS；348项通过，2项已有可选PG单测在本次普通构建中跳过。独立浏览器实际使用PG会话和身份校验，不把跳过项算通过 |
| 独立Edge管理路线 | 13项PASS：逐用户会话、最小数据库权限、成员25/25/8、筛选和精确详情、角色查询、审计25/25/8、跨空间/跨调用者游标拒绝、撤权、恢复不复活、目录权限失效、来源错空间、写入口拒绝、移动端及查询前后审计事实一致 |
| 默认预览 | 未配置页面8个菜单的1项浏览器用例PASS，无管理API调用、假人员或写按钮 |
| 既有路径 | 弹窗5项PASS；Agent及业务签收21步PASS，无页面错误、外部请求或业务写请求 |
| 来源、业务与运行边界 | 真实来源UNCONFIGURED，业务写入口关闭，原库与AgentAPI保留，模型/真实执行NOT_RUN |

合成OIDC与原生目录HTTP夹具用于协议和边界验收，不是企业ZITADEL/Keycloak的真实目录见证。未实施企业账号、部门组织树、企业自定义角色管理、生产部署或真实数据访问；当前没有将测试fixture安装成真实身份服务。

首次隔离夹具暴露Windows ESM路径和OAuth Basic凭据表单编码问题，修复了测试夹具，保留失败记录；没有放宽现有RP验证。截图检查另修复仅管理profile所需的单一品牌静态资源路径，以及成员角色主文案显示编码的问题。

首轮CI发现当前构建的`trial-business`仍精确要求schema76，而新增DDL为77。已将当前命令的精确版本要求同步至77，并补充当前命令与最新migration一致性回归；未改成接受任意更高版本。独立测试容器内另建`management_schema77_regression_20261025`空库，完整迁移与`go test -count=1 ./cmd/trial-business`均PASS。旧冻结二进制、原库和原有运行实例未升级。

## 证据与重演

本地原始结果为`.data/management-readonly-20261025/browser-results.json`及截图；保留的非秘密副本见[验收结果](platform-management-readonly-20261025/browser-results.json)、[截图和来源摘要清单](platform-management-readonly-20261025/manifest.json)。截图来自合成数据，不能证明真实来源已配置。旧候选与旧截图保持历史状态。

现有独立环境可复验 `node tests/platform-management/browser.mjs`；它只操作该明确测试库和合成身份。全新隔离准备入口为 `MANAGEMENT_FIXTURE_ACK=ISOLATED_READONLY_MANAGEMENT python tests/platform-management/bootstrap.py`，然后同一ack运行 `start-web.py --build`。已存在目录、容器或端口会拒绝重复创建，不覆盖或重建现有资源。Node/Go只在各进程信任测试CA，不改系统信任库。

CI已在既有身份验收工作流加入全新管理库、独立构建与Chromium路线；CI结果以本增量精确HEAD为准，不能继承旧冻结候选绿色。独立4300用于本片验收，4193继续展示真实来源未配置状态。
