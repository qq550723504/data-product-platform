# 全站新版界面与演示边界（2026-10-23）

> 历史批次记录：下文的全路径重写与4189入口描述当时的迁移阶段，已被后续根路径重写方案取代。当前仅根演示页重写到 `/demo`，Core/Agent 深链及其服务端读取/Actions 保留；精确路径与只读边界以 [merge contract](governance-preview-merge-contract.md)、当前 middleware 和[领导演示交付指南](leader-demo-closeout-20261025.md)为准。原批次证据继续保留，不能当作当前部署或业务写入授权。

## 目标与唯一入口

在隔离 worktree aq1 中统一前端。预览 http://127.0.0.1:4189/；不是另起一份 Vite 站点，也不是 iframe 包装。ConsoleShell 只加载同一个 Governance App；Agent 的工作台、任务详情、修复绑定和质量上下文仍由原 Next 页面及既有控制器提供。

新版外观来源：C:/Users/Henry/.codex/visualizations/2026/10/22/01a163ef-ed6d-7d53-bb1d-9d059057244c/governance-prototype/src。复用完整页面/上下文/模型，集中放在 apps/web/components/governance-demo；原型不修改。复用 React、Phosphor 与已有 Next，无新增组件库、IAM、队列或编排运行时。

## 导航与页面范围

顶部仍是工作台、数据开发中心、数据管理中心；系统设置是独立全局入口，不变成第四个业务中心。每个中心使用自己的左侧菜单。

| 范围 | 页面 |
| --- | --- |
| 工作台 | 治理总览、Agent、架构搭建流程、数据开发流程、沉淀提升流程、我的工作；个人资料、消息 |
| 开发中心 | 开发概览、需求/版本化方案、汇聚、处理、发布与交付演示、交付工具箱、技术支持；三类报告、去向、接口配置 |
| 管理中心 | 管理概览、调研规划、系统架构、数据模型、质量、元数据/血缘、安全/权利/合约、资产、标准、知识；版本、认证、发布资格等详情 |
| 系统设置 | 设置总览、空间管理、空间成员、用户、组织、角色/访问预览、业务/管理审计、运行日志、基础配置 |
| 原业务入口补齐 | 来源登记、汇聚批次、实体人工审核；复用同一演示上下文及活动记录 |

middleware 将非 Agent UI 路径重写到无业务 loader 的 /demo。/api、静态文件及 /agent/** 不受重写。旧列表入口映射到对应演示模块；旧 Core 对象详情 URL 显示明确说明，不能把真实对象 ID 偷换成合成记录。旧业务 Server Components/Actions 保留供未来逐模块接入，当前演示入口不运行它们。

## 状态与交互

非 Agent 使用同一套按 workspace ID 分区的演示流水线；需求确认、方案修订、任务、逐次运行、报告、评测、认证、发布/交付/验收演示保留关联编号与历史。新来源登记只登记意图，不冒充真实 DataResource/RAW 接纳；汇聚批次读取流水线现有运行；实体决定追加历史，不改变真实映射或版本。

平台用户、角色、基础配置和管理审计改用全局演示状态，避免切换业务空间后产生第二套账号；空间成员/业务数据仍按空间区分。

合成数据存放当前标签页的 sessionStorage，命名空间 dpp:governance-demo:*，用于跨 Next 路径和刷新恢复。页面明确可手动重置；重置只移除 registry/data 两个键，不清除 Agent 的待核实请求、不删除服务端记录。没有持久化后端承诺。浏览器禁用存储时回退内存，不影响 Core。

原型中的模拟 Agent 委托/执行控制器已从迁移目录移除，避免并行事实来源。演示上下文侧栏只展示明确合成版本；进入 Core Agent 时不提交这些假 ID。Agent 不使用演示空间切换器，单独显示 Core 试点范围，原确认/幂等/CAS/意图/刷新恢复契约不变。

## 验收记录

待最终浏览器与构建验收后追加。没有调用模型、真实引擎、派发、发布或交付；没有合并或部署。

### 最终结果

- PASS：完整 Next build（包含所有已有前端回归）；新增/复用的演示领域与 session 隔离测试 19 项通过。App.d.ts 是 JS 页面与 TS shell 的边界，不把未类型化原型声明为全量 TypeScript 校验。
- PASS：31 个非 Agent 主菜单逐项打开，标题与页面内容正常；证据 .data/agent-q2-preview/evidence/site-menu-acceptance.json。另验证实体审核理由门禁、追加决定及刷新恢复；来源登记草稿/核对；批次指向明确运行报告。
- PASS：真实浏览器创建 REQ-0002 → 确认 → PLAN-0002/r1 → TASK-0003 → RUN-0002 → VER-0002 与本次报告，关联编号一致。此路径在本轮首次迁移构建中验收，后续 session/边界改动未改动 pipeline 控制器。截图 site-demo-report.jpg。
- PASS：设置保存追加管理审计；切换工作空间后全局 UTC 配置保持；进入 Core Agent 后返回，演示配置保持；取消重置无清空。Session 单测验证真正重置也不会删除 Core pending 键。
- PASS：Agent API 4 组回归；最终重启后浏览器读取既有 ef6e94ef-23fb-4afe-8296-b9960ce86127，V2 08f396bb-bcf8-4b57-bb3a-e42dc606179c、独立确认 1 条、意图 787a579f-3936-4bc4-ad34-b4d7e66e9cf9 保持未派发。
- PASS：原 /resources 入口落到新版资产页；旧 /datasets/{CoreID} 不伪装为合成详情，显示明确说明与返回入口。非 Agent 目录没有 fetch/XMLHttpRequest/WebSocket/sendBeacon/API client 调用。
- PASS：390px 窄屏配置页无页面横向溢出，顶部中心与横向侧菜单可达；临时 viewport 覆盖已清除。最终桌面首页/系统设置已截图目视检查。
- 初次 API 回归在 Next 尚未监听时出现 ECONNREFUSED/502，保留 governance-modern-api-startup-failure.log；确认 HTTP 已就绪后完整回归通过。启动脚本现等待受认证的只读 HTTP 200 才报告 ready，不重启或重试业务命令。
- 隔离库前后均为 CostEvent=7、Execution=0、来源 Quality=1、修复 assignment=0。7 条成本和 1 次质量来自原合成夹具，未增加真实执行或模型调用。原主 checkout 保留原两份未跟踪设计文档。

本地证据：.data/governance-modern-build.log、.data/governance-modern-api.log；截图 evidence/site-modern-home.jpg、site-modern-settings.jpg、site-agent-preserved.jpg、site-demo-report.jpg。当前预览仍运行；尚未合并或部署。没有逐一穷举所有报表导出或治理分支；本轮覆盖菜单、核心需求切片、补齐入口、状态恢复和现有 Agent 接口回归。


## 2026-10-23 全站补齐复核

2026-10-24：[认证和发布历史只读接入](governance-history-readonly-acceptance-20261024.md) 已完成新版页面、可信 principal 门禁、只读路由和 Edge 空态/筛选/回跳验收；没有创建真实认证或发布。当前库缺少非空历史，实际浏览器非空记录覆盖不在本轮通过声明中。

后端逐项接入已启动：映射与首个只读切片见 [后端接入清单](governance-backend-integration-map.md)。`/core/assets` 已接隔离 Core 的目录、明确版本、历史评测和证据查询，与原演示状态分开；Agent 原 API 保留。

后续认证、发布、申请审批、交付、验收退回重提及归档的实际 Edge 验收见 [交付与归档验收](governance-delivery-acceptance-20261023.md)。已走通闭环并验证认证撤销阻断新交付、历史保留及刷新恢复；修复调用情境选择框的可访问名称，完整构建与浏览器复验通过。Agent API 与未派发意图保留，无模型或真实执行。

见 [新版全站补齐检查](governance-modern-ui-audit-20261023.md)。已补标准/模型带入、同源质量监控、绑定评测的修复任务和历史消息定位。32 个一级菜单、53 个导航标识通过源码及组件渲染检查。后续已接手用户新打开的正常 Edge 页，实际验收标准检验、模型引用、质量整改及历史消息回跳，均通过；包含失败、待核实后核实、新版本独立复评、取消确认、防重复和刷新恢复。Agent 既有 r4 计划与未派发意图通过只读浏览器核对，Core 计数保持不变。完整证据与未覆盖分支见上述报告末节。
