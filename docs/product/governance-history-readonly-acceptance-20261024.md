# 认证和发布历史只读接入

## 范围与语义

继续使用 aq1 隔离预览 4189，保留 Agent API，不加载模型、执行引擎、真实评测或交付。没有新建认证、发布、撤销、替代或演示冒充服务端记录。当前隔离库认证和发布均为 0 条。

新增页面：

- `/core/certifications?datasetId=…&versionId=…`：从 Core 资产明确版本进入，展示认证原始结论、配置快照版本、签发时间、阻断项、冻结依据和后续处置。认证历史分页保留服务端 asOf 与 anchorRevision；刷新到当前记录为显式操作。历史认证不授予当前交付资格。
- `/core/releases`：授权工作空间的发布批次历史，按后端实际支持的状态筛选，展示产品/产品版本、创建与发布时间、冻结引用和发布说明。没有发布时间不伪造为已发布；列表不包含数据集成员，不能推断与某资产版本的关系。

沿用新版统一 shell、已有 Badge/EmptyState、质量目录和服务端 transport，没有引入新的业务状态或第三方基础设施。原本地演示流程保留并明确区分连接范围。

## 发现与根因修复

1. 认证 history 和 workspace product-releases 虽有 workspace 参数及归属检查，但缺少可信 principal 门禁。现直接在两个现有处理器认证后查询，生产 API 与隔离预览均配置已有 principal resolver；未配置时 fail closed。不是仅在前端隐藏入口。
2. 这两条前端旧查询使用不带可信凭据的通用 apiGet；改为复用 server-only qualityRead transport，仅用于明确受保护的两个查询，不给其他 API 全局附加凭据。
3. 隔离预览分别注册 `RegisterHistoryReadOnly` / `RegisterReleaseHistoryReadOnly`，不挂载认证、替代、发布或资格变更命令。认证服务只为已有查询配置 repositories；不提供 transaction writer 或 evidence resolver。
4. 发布查询增加明确 workspaceId 响应范围，前端校验工作空间及每条分页数据。认证同时核对所选版本、profile workspace、历史时点与修订。失败显示 UNKNOWN，不回退到演示、最新认证或最新发布。
5. 修复空结果中指定历史 ID 没有显示“无法定位”的问题：空态仍保留，所指定认证/发布不在本页时另显示 UNKNOWN。

## 自动验证

- Go certification HTTP、readmodel HTTP、preview tests 通过；API 主程序编译检查通过。
- 新增认证/发布只读路由测试：无可信身份先拒绝，跨空间先拒绝，不触达 repository，POST 不注册。
- `test:history` 5 项：认证/撤销分别保留；跨版本、跨空间、变化时点/修订拒绝；分页锚点含零值；发布筛选与空数据；非空记录呈现及缺失指定记录。非空组件测试的 Badge 为 primitive stub，不是实际 Core 浏览器记录。
- `history-readonly-api.test.mjs` 与原只读 API 回归：当前真实隔离 API 的 401/403、明确版本、分页时点与修订、非法发布状态、认证/替代 POST 禁用及 Agent r4 / REQUESTED / dispatchEnabled=false 通过。日志 `.data/governance-history-readonly-api.log`。

## 实际浏览器验收结果

1. **PASS**：Core 资产目录 → energy_raw → 明确 v1 → 查看此版本认证历史。工作空间和版本 UUID 一致，显示服务端 asOf、历史修订 6 和“暂无认证记录”，未误称已认证。
2. **PASS**：使用该服务端时点/修订读取 offset=25 空页，再用“上一页”返回；时点与修订保持不变。指定不存在的认证时显示 UNKNOWN，不读取其他认证；刷新仍保留该引用及提示。
3. **PASS**：返回版本与质量，仍为原 energy_raw v1；再进入发布历史，连接范围保持 Core，非本地 REL 演示记录。
4. **PASS**：发布 PUBLISHED 状态筛选经 GET 提交后生效，刷新保留；当前返回 0 条。指定不存在发布时显示 UNKNOWN，不回退到最近批次。通过导航可回到正常列表。
5. **PASS**：数据库计数未变：CostEvent=7、Execution=0、源 QualityAssessment=1、修复 assignment=0、认证=0、发布=0。API 回归再次读取既有 Agent r4、原计划与 REQUESTED 意图，dispatchEnabled=false。

截图已保存并查看：`.data/governance-browser-acceptance/17-core-certifications.png`、`18-core-certification-missing.png`、`19-core-releases.png`；18、20 同名 txt 保留缺失引用可见事实。完整构建（含 26 项演示、5 项历史及既有前端测试、Next 编译和类型检查）通过，日志 `.data/governance-history-readonly-build.log`。

## 验收边界

当前没有非空 Core 认证/发布记录，不声称完成非空历史的真实浏览器数据验收。非空展示由组件和契约回归覆盖；实际浏览器检查空态、查询范围、筛选、缺失引用和回跳。没有为验收修改数据库业务历史。真实 PostgreSQL 非空并发历史测试未在本轮运行。
