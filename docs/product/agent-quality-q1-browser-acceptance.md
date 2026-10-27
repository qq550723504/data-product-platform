# Q1 隔离浏览器验收记录

2026-10-23，分支 `codex/agent-quality-readonly`。基线 `f7a90022aad5da4d67ea594ccb86f8b9925a3840`。

**结果：14 项通过。** 浏览器操作使用 Codex 的 CUA 浏览器接口，未运行独立 Playwright 浏览器进程。预览使用真实的 `apps/web` 生产构建与合成 Core 测试夹具，不是 Q7 全流程 live-core 验收。

## 运行边界

- Next standalone 预览：`http://127.0.0.1:4186/agent`。
- 合成测试夹具：`http://127.0.0.1:4459`。业务端点仅接受 GET；POST 探针返回 405。带专用测试 token 的 `__control` 仅切换合成场景，不执行真实业务命令。
- 全部 `POC_ENABLE_*_ACTIONS` 关闭；脚本只设置子进程配置并恢复调用终端环境。无真实数据库、对象存储、queue、worker、引擎或模型连接。
- 两个监听地址均验证为 `127.0.0.1`。原有 4178 原型及保留运行环境未修改。

## 实际观察

| 编号 | 用例 | 结果 |
|---|---|---|
| B01 | 工作台 → 数据集 → 明确版本 → 旧评测 | 旧评测 `cccccccc-cccc-4ccc-8ccc-cccccccccccc` 精确显示，证据引用为 1；未默认切换最新 |
| B02 | 26 条评测历史的第二页 | 正确显示第 26 条，保留所选旧报告 |
| B03 | 26 条发现项的第二页 | 仅显示 `FIXTURE-26`；历史页、数据版本、评测身份保持不变 |
| B04 | 尝试核实、上一页与刷新 | GET 查询显示 SUCCEEDED，关联原评测；历史/发现项 offset=25 保留；前页和刷新仍是同一旧评测 |
| B05 | 历史、尝试及 latest 报告同时失败 | 相应区域 UNKNOWN；明确选择的旧报告与证据独立可读 |
| B06 | 旧报告返回 403 | 报告局部 UNKNOWN，已知来源及评测列表保留；不暴露原始错误体 |
| B07 | 报告 HTTP=200 但版本不匹配 | 前端仍拒绝将它显示为已确认报告，不展示其证据 |
| B08 | 来源上下文返回 503 | 页面 UNKNOWN；后续新增 Quality 读取为 0；无匿名重试 |
| B09 | 重复/无效版本引用 | 显示“上下文引用无效”，不选择其他版本替代 |
| B10 | 未找到的尝试 | 尝试区域 UNKNOWN；旧报告保留，没有触发新评测或重试 |
| B11 | 折叠导航及键盘焦点 | 导航具备可访问名称与提示；Tab 进入核实按钮，显示 3px solid 焦点框 |
| B12 | 1365 / 768 / 390px | 页面 scrollWidth 等于 clientWidth，无页面级横向溢出；测试后已恢复默认视口 |
| B13 | 浏览器网络与凭据 | 新采集的完整窗口含 14 个请求，全部 GET、仅访问本机 Next；URL、请求头和 DOM 均无服务 token |
| B14 | standalone 启动复验 | 页面、静态资源与旧报告正常；无 HTTP 错误或异常加载失败。另记录到 10 个已取消的其他导航 RSC 预取，不计作当前页面资源失败 |

完整业务测试阶段的夹具日志共 51 次业务请求，全部 GET；deliveryCount=0、retryCreated=false、releaseCreated=false。拒写探针状态为 405。测试场景切换属于夹具控制，不是业务写入。

最初跨全部场景的浏览器网络缓冲已被截断，因此没有用它证明完整网络行为；保留该原始摘要，并重新采集未截断的窗口。所有场景的只读判断同时使用完整的夹具业务请求日志。

## 修复与回归

- 修复窄屏折叠菜单只剩图标时缺少无障碍名称的问题；补充 `aria-label` 与标题提示。
- 修复按尝试核实时丢失历史/发现项分页位置的问题；GET 表单保留这两个分页值。
- 启动器使用明确的单个 Node 可执行路径；停止时验证工作区、PID、Node 进程启动时间和监听端口，避免误停其他会话。时间比较使用 DateTimeOffset，避免 JSON 自动日期转换造成重复时区偏移。
- 预览按项目 `output: standalone` 启动并包含静态资源，复验时无启动警告。
- `npm run build` 通过，包含新增 Agent 回归与既有模块测试、Next 编译、类型及 lint；fixture 的 18 个 Node 用例通过。

## 本地证据

证据保存在工作区的 `.data/agent-q1-preview/evidence/`，未纳入 Git：

- `browser-cases.json`：14 项实际验收观察。
- `core-readonly-summary.json`：51 次完整业务请求的路由计数与拒写结果，不含认证请求头。
- `browser-network-summary.json`：未截断的 14 请求网络窗口。
- `browser-network-initial-truncated.json`：初始被截断窗口的摘要，未用作完整性依据。
- `standalone-smoke.json`：最终启动复验及已取消预取的分类。
- `final-runtime-summary.json`：最终 standalone 进程中的 3 次业务请求均为 GET，拒写探针仍为 405。
- `quality-context-final.jpg`、`quality-context-desktop.jpg`、`quality-context-mobile.jpg`、`history-failure.jpg`、`report-denied.jpg`：实际页面截图。

## 继续预览

在仓库根目录 PowerShell 中启动或停止：

```powershell
& './tests/browser/start-agent-q1-preview.ps1'
& './tests/browser/start-agent-q1-preview.ps1' -Stop
```

启动前需有当前的 `apps/web/.next` standalone 构建；端口被占用时脚本会报错退出，不抢占端口或终止其他进程。当前预览已保持运行，PID 与日志位于 `.data/agent-q1-preview/`。脚本内凭据为公开的合成 fixture 凭据，不是任何真实服务密钥。

Q2 及后续委托、计划冻结、动作受理、真实执行和模型能力仍未实现；这些不属于本次浏览器验收结论。
