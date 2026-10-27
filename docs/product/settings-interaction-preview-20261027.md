# 创平数据治理平台：系统设置交互预览

2026-10-27。用户在预览聊天明确要求暂停后端对接、先展示交互流程，并将对外名称改为“创平数据治理平台”。原始用户消息已从线程`01a17c2d-af32-7861-a0e4-68b97400d953`核验。本轮仅交付独立前端演示；未完成的管理写入代码保留在工作区，不纳入本预览构建或启用。

## 实现与边界

复用已有`apps/web/components/governance-demo`中的AdminPages、WorkspacePages、WorkspaceContext、DemoProvider及会话模型；没有另建一套设置页面。原交互未丢失：问题是集成导航把`settings`送往只读`/settings`。本轮添加默认关闭的`GOVERNANCE_SETTINGS_INTERACTION_DEMO=true`，使独立预览的设置入口和菜单沿用本地hash流程，工作空间切换后仍返回设置总览。

左上角品牌、页面浏览器标题、根页面元信息及系统说明统一使用“创平数据治理平台”；业务对象、空间名称、包名、接口和数据库名称保持不变。

- 交互入口：`http://127.0.0.1:4445/#settings`，已启动并完成独立Edge验收。
- 默认4193及既有只读/Agent入口不替换、不重启、不解除写入限制。
- 仅使用当前浏览器标签页的演示数据；刷新恢复本地会话，关闭会话或明确重置后恢复样例，不修改真实配置或已有其他入口的数据。
- 独立host与middleware双重拒绝POST等写方法及`/api`、`/auth`、`/core`、`/agent`连接路径；页面数据操作仅为React状态及sessionStorage。
- 不连接数据库、真实身份目录、邮件、模型或执行服务。原只读预览及其Agent API保持原状；本入口不伪装已连接这些能力。
- 后端WIP源码已保存独立快照，未创建新的管理数据库/运行服务，未提交或发布未完成后端。

## 验收与重演

最终独立Edge验收7组PASS，其中一组循环九个设置菜单，不把九次菜单访问计为九条独立测试。覆盖用户创建/编辑/启停、取消不保存、机构维护、角色配置/访问预览、配置保存与刷新、管理审计详情、日志详情、空间创建/编辑/归档/恢复、成员增改移除、负责人保护、空间切换及原空间成员保留。小窗口1280×360和390×600的空间弹窗也通过。页面错误、后端写请求、外联均为0；直接访问连接路径与POST返回拒绝，未知/编码路径不得进入后端。

演示模型43项PASS、0失败/跳过，包含原根页面/Agent/Core路径及OIDC profile边界和新增默认关闭的交互模式路由回归。生产构建及其类型/lint检查通过。构建保留原CSS兼容警告和Windows standalone符号链接警告；本地使用显式Next宿主及既有依赖，未使用不完整的standalone产物，不把这些警告称作已修复。

首次浏览器失败保留：独立宿主拒绝了Next根页面rewrite内部请求；选择空白demo页面后，发现Next middleware又将设置重定向地址归一化为localhost。宿主现先选择demo页面并以相对地址完成设置跳转，复用同一份封闭路由表；原Host限制和后端拦截未放宽。最终测试同时核对完整127.0.0.1地址和页面标题，避免只看hash就误判成功。单测加载器也修正为相对真实middleware解析依赖、沿用项目esModuleInterop设置，没有放宽断言。

- [安全结果摘要](evidence/settings-interaction-20261027/acceptance.json)
- [用户管理截图](evidence/settings-interaction-20261027/users.png)
- [空间成员截图](evidence/settings-interaction-20261027/members.png)

重演：打开交互入口→用户管理新增/编辑/启停并刷新→组织机构新增机构→角色权限新增角色/访问效果预览→基础配置保存时区并刷新→审计日志切管理审计查看变更前后→工作空间新建、编辑、归档、恢复、进入→成员添加、调整角色、移出→切回原空间核对原成员。关闭弹窗即取消；没有账号创建、邀请邮件、真实授权或配置写入。跨空间验证仅证明本地会话数据分桶，不充当真实权限隔离证明。

自动重演：构建Web后，以`GOVERNANCE_SETTINGS_INTERACTION_DEMO=true BROWSER_IAM_MODE=off SETTINGS_ADMIN_UI_ENABLED=false`启动`node tests/browser/start-settings-interaction-preview.mjs`；在tests/browser运行`npx playwright test --config playwright.settings-interaction.config.mjs`。本机默认Edge；CI显式选择Chromium，复用已有web-console构建，不新增后端服务。新增CI结果须以本轮精确HEAD的运行记录为准，不继承a16a71aa的绿色。

构建使用Git中本轮已选定的前端变更生成独立归档，排除未完成的管理写入代码；构建目录与旧预览分离。使用已有Next与依赖，`tests/browser/start-settings-interaction-preview.mjs`仅绑定127.0.0.1:4445；不安装新基础设施、不修改原型源目录、不清理已有数据或容器。
