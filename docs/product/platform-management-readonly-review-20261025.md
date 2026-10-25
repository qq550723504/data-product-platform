# 系统设置只读接入复审

本页保留`f99889f8`阶段的复审与14项验收证据；后续角色→成员→详情的页面联动、当前构建和18项验收见[实施记录](platform-management-role-member-flow-20261025.md)。

2026-10-25，审查起点为PR #363的`1dfa7c6fe00e9b6a7cfda536b43be378ca985de3`。范围是本轮成员目录、应用角色权限与审计查询，以及其身份、空间、撤权和分页边界；不是对整个PR历史增量重新作完整审查。[平台管理只读契约](../architecture/platform-management-readonly-v1.md)继续是唯一查询合同。

## 发现与修复

| 级别 | 复现的问题 | 根因及修复 |
|---|---|---|
| P1 | 读取中途撤销访问令牌或改变身份版本，直接管理API仍返回200及成员资料 | 原先仅首次调用OIDC resolver，结束时ValidateInTx只能检查本地授权。结果公开前再次使用既有无缓存resolver，核对原actor、issuer/subject、workspace与revision，再校验本地授权。身份变化丢弃全部结果，不用新资格追认旧读取；更短有效期只收窄。成员、角色和审计共用此边界 |
| P2 | 目录构造器接受关闭证书验证的TLS配置及任意RoundTripper | 沿用已有OIDC适配器的标准transport校验/克隆方式，拒绝InsecureSkipVerify、自定义TLS拨号、错主机ServerName和未知传输，限制响应头；调用者后续修改原transport不影响已构造的连接。没有新身份栈或系统级TLS例外 |
| P2 | 前端将toString、constructor、__proto__误识别为合法权限 | 原先查普通对象属性，包含Object继承属性。改为显式自有键白名单；未知角色仍不获得权限，未增加管理写权限。此问题属于响应校验缺口，不能声称这些字符串曾获得后端授权 |

三组新回归在修复前均失败，修复后通过。另修正原取消测试，让它真正到达目录响应体的取消点，而非在前置权限探针就失败；测试故障注入留在同包测试内，不再通过产品构造参数绕过TLS校验。

审查同时收窄一处契约过度表述：远端offset目录接口没有共同快照或修订号，只能拒绝已检测到的数量变化、短页、重复/乱序、错空间或筛选不符。相同总数下的并发成员替换不保证可检测；目录是查询视图，不是冻结事实或后续业务写入依据。本片不另造目录快照系统。

## 验证结果

- 全新`management_review_regression_20261025`空白测试库迁移至77，管理查询、现有principal/fence及trial-business PostgreSQL回归通过。新增同一微秒58条审计记录的25/25/8分页，验证不重复、不遗漏、工作空间过滤和跨调用者/跨空间游标拒绝。没有修改原库或既有管理验收库的审计历史。
- 独立Edge管理路线14项通过，复验成员完整筛选、详情/刷新、角色投影、审计分页、多人和跨空间、撤权及旧revision不复活。新增受控暂停目录读取、撤销合成令牌后释放的直接API用例：401且无data，随后页面不再显示成员；重新登录才恢复。浏览器无页面错误、外部请求或业务写请求。
- 新版独立Web构建通过，349项通过、2项已有可选PG单测跳过；真实PG身份与会话另由上述独立浏览器覆盖，跳过项不计通过。现有CSS编译提示不属于本轮改动。
- 默认4193系统设置8个未配置菜单的浏览器用例通过，没有合成人员回退或写入口。Agent与业务签收的既有21步重演归本地结果记录；本轮未修改Agent API或领域路径。

新直接API用例第一次沿用了浏览器Cookie，被现有Cookie拒绝边界提前拦截，未到达暂停点。已改用独立无Cookie测试上下文并通过；首次失败保留在`.data/management-readonly-review-20261025/`，不把该失败当产品撤权验证成功。

本地最终管理结果为`.data/management-readonly-review-verified-20261025/browser-results.json`，非秘密副本见[14项验收结果](platform-management-readonly-review-20261025/browser-results.json)及[结果清单](platform-management-readonly-review-20261025/manifest.json)。旧13项结果、旧截图及本轮较早结果保持历史状态。CI必须以本轮修复后的精确HEAD为准，不能沿用审查起点的16项绿色结果。

## 运行及范围

4193和独立管理4300使用`.next-acceptance-identity-management-review-20261025`；仅核验PID、启动时间、命令路径和监听端口后更新所属的预览进程，保留旧构建、旧归属记录和独立PG。来源仍为UNCONFIGURED，4300只有合成夹具。现有OIDC/RP、原库、原项目工作区与Agent API保留。

重演管理路线可设置一个新的`MANAGEMENT_BROWSER_OUTPUT=management-readonly-review-replay-20261025`后运行`node tests/platform-management/browser.mjs`，输出目录限独立验收命名，避免覆盖旧证据。受控暂停、合成令牌和撤销只在测试fixture中，通过私有测试控制启用，不是新增产品API。

未开放邀请、账号/组织/角色修改、业务签收写入、模型或真实执行。企业自定义角色、真实来源接通与一致目录快照均不算已实现。当前已复现问题均已修复；上述远端分页一致性限制明确保留。PR继续Draft，不合并、不部署。
