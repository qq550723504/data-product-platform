# 标准与验收责任只读接入验收

## 范围

第 0 切片在 `e448fa8f` 基线上新增标准来源、合约版本及责任准备情况的只读查询，页面为 `/core/acceptance-context`。本文拥有已实现的读取行为；[后端契约草案](../architecture/business-acceptance-backend-contract-v1.md) 继续拥有后续业务写入的目标不变量。

Core 尚无 BusinessAcceptanceCase 或权威业务责任绑定。接口明确返回六类责任 `UNASSIGNED`、接收方代表 `UNVERIFIED` 并阻断正式办理；这不是查到了实际人员，也不是以读取身份授予验收权限。标准格式可核验不能证明双方已确认或该合约适用于这笔交付。签收、标准确认、验收、归档和处置的后端写接口仍未接入。

## 接口与来源

| GET 接口 | 边界 |
|---|---|
| `/api/v1/workspaces/{workspaceId}/contract-versions?limit=25&offset=0` | 同空间版本列表；稳定分页，不选 latest |
| `/api/v1/contract-versions/{versionId}?workspaceId={workspaceId}` | 原详情入口现在要求明确空间及可信身份，返回安全投影 |
| `/api/v1/workspaces/{workspaceId}/acceptance-context?deliveryOperationId=…&datasetVersionId=…&contractVersionId=…` | 必须配对明确交付/版本，可选精确合约；显示事实及责任缺口 |

所有查询在 repository 前校验 trusted Principal、workspace 与已有读取能力。任意 actor/consumer 参数和 X-Actor-ID 均不授予权限。未认证 401，越空间 403，空间内缺失/错配 404，非法/重复/未知参数 400。响应 `private, no-store`；列表、总数和上下文在同一 READ ONLY / REPEATABLE READ transaction 读取。

查询 JOIN 合约归属空间及交付成果的 dataset 空间，移除了原 unscoped HTTP GET。内部现有 Command 使用的 repository 方法不成为公开查询捷径。预览只注册 GET，不注册原 create/publish Command。

响应不包含原始 document、sourceRef、创建人/发布人、provider key、credential 或存储位置。前端再校验作用域、明确对象、完整标准集、分页和责任集合。列表、详情与上下文独立读取，失败区域显示 UNKNOWN，已知兄弟区域保留，不回退演示数据。

标准来源为明确版本的 `spec.acceptanceCriteria`：

```json
{"version":"1","criteria":[{"id":"C1","clause":"3.1","text":"核对明确成果范围","required":true,"method":"MANUAL_CHECK","evidenceCategory":"RECEIPT"}]}
```

验证要求 1–100 项、唯一 ID、明确 required、至少一个必需项、非空条款/内容，及 MANUAL_CHECK/EVIDENCE_ASSERTION。缺格式为 UNSUPPORTED_SOURCE，字段错误为 INVALID_SOURCE，未支持 method 为 UNSUPPORTED_CRITERION；失败不输出可供办理的部分标准。成功按 ID 排序并计算版本化标准集 hash；它与已有 sourceSha256 分别表示标准集和来源内容。DRAFT/DEPRECATED 可供核对，不成为已发布来源。

复用现有 ContractVersion、Principal Resolver、PostgreSQL、标准库 JSON/哈希、Core 壳与 transport；不新增 IAM、标准库产品、审批引擎、queue 或 SDK，不新增 migration，不改写历史版本。业务责任源和正式标准确认属于后续切片，当前不推断。

## 页面与验收

侧栏新增“标准与验收责任”，交付记录提供带明确交付/版本的入口。页面按目标、责任与阻断、明确标准、版本列表排列；选择、分页和刷新保留上下文，精确历史回跳不改查其他对象。`/#business-*` 的演示状态和工作空间隔离保持原样。

| 验证 | 结果 |
|---|---|
| Go Domain/HTTP/预览及 API 编译 | PASS；鉴权、越界、非法/重复查询和 POST 拒绝 |
| 真实 PostgreSQL | PASS；独立库随机两个空间，列表限定、外空间明确 ID 不可见，末页保留总数 |
| 前端历史/来源校验 | 18 PASS，含新增 3 项 |
| 既有 HTTP 浏览器夹具 | 19 PASS |
| Lint、隔离完整构建及现有离线回归 | PASS |
| 完整浏览器契约 | 118 PASS，含新增 4 项 |
| Edge 和真实只读 API | PASS；菜单、明确标准选择、刷新、原交付回跳、错配版本 UNKNOWN、缺结构化标准阻断 |
| 双库读取窗口 | PASS；各 216 表，窗口前后计数与摘要一致，原库从准备前起保持一致 |

证据在忽略的 `.data/acceptance-s0-build.log`、`acceptance-s0-postgres.log`、`acceptance-s0-api-tests.log`、`acceptance-s0-browser-all.log`、`acceptance-s0-read-before.json`/`read-after.json` 及 Edge 截图。构建 `.next-acceptance-s0-20261024065922`，buildId `s1r0gEyeNjkYv6u6ahspR`。

首轮浏览器 14 PASS/1 FAIL：测试把侧栏“验收归档”误认作业务写入按钮。根据实际失败快照限定 main 内容区，保留全部请求须为 GET、私密字段不得进入 HTML 的断言；全套实际重跑 118 PASS。失败 trace/截图保留在 `.data/acceptance-s0-browser-failure1`，原日志保留在 `acceptance-s0-browser-failed1.log`。

夹具首轮因 Windows 管道编码被 PostgreSQL 拒绝，事务未提交；脚本显式 UTF-8 后才完成。首轮构建发现闭包可选字段的类型错误，修复后完整重建，原日志保留在 `.data/acceptance-s0-build-failed-types.log`。这些失败没有被当作原始成功。

## 运行保留

准备仅向独立库 `agent_q2_acceptance_20261024` 追加明确 synthetic 的标准夹具和隔离测试事实，对现有 reader 增加 data_contract/contract_version 两项 SELECT。无 DDL、表/列 DML 授权、schema CREATE 或 TEMP；没有重跑旧 prepare/seed，没有写入原库 `agent_q2_test`。

PostgreSQL、Caddy、原库和旧 API/UI 可回退文件保留。校验 PID、创建时间、可执行路径和 listener 后，只替换所属预览 API/UI；Agent URL 与行为保留，writeEnabled=false、dispatchEnabled=false。Execution、Agent 任务、执行请求、provider attempt、outbox 均为 0。UI 4188、网关 4189、API 18497。

本地验收不代表 PR CI、main CI、生产部署或真实业务签收通过。PR #363 保持 Draft、不合并；新增提交 CI 单独按精确 HEAD 确认。
