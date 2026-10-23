# 质量成本、证据与审计明细只读接入

## 本轮完成

1. Core 资产明确版本 → 原历史评测报告，新增该评测的成本记录表；复用已有 authenticated `/api/v1/quality-assessments/{id}?workspaceId=…` 和 CostAllocation 查询，不新增账务系统或通用成本接口。
2. 原报告证据引用可展开查看类型、关联关系、来源主体、摘要及记录时间；审计引用可展开查看动作、主体、办理类型及时间。复用已有 bounded report projection，不展示 metadata、storage URI、凭据或下载入口。
3. 成本投影核对 workspace、DatasetVersion、QualityAssessment；缺少 costEvents、重复成本 ID、错主体或非法字段均为 UNKNOWN，不当成零成本。金额保持十进制字符串；未定价不当免费，不合计不同币种。不展示成本 metadata。
4. 成本与质量报告独立读取；成本未知时已知报告、证据仍可展示。报告新增字段也增加运行时校验，审计必须指向同一评测。
5. 隔离预览为已有 quality handler 注入现有 Cost QueryRepository，仍只注册 GET，无评测执行器、模型、worker 或新 Command。

## 验证

- 完整前端 build PASS：含 6 项历史/成本契约和呈现测试、26 项演示回归、已有 Agent/质量等回归、Next 编译与类型检查。日志 `.data/governance-cost-evidence-build.log`。
- Go quality HTTP 与 preview 编译回归 PASS。
- 当前 API 的只读范围/身份检查、历史查询及原 Agent r4、REQUESTED、dispatchEnabled=false 回归 PASS；新增 live assessment cost scope 检查。日志 `.data/governance-cost-evidence-api.log`。
- Edge 实走资产 → energy_raw v1 → 明确评测 d3667496-f421-497c-be69-e5dec6bb569b，读取旧成本 94185883-5c11-40cd-9b28-689138f67f1d：QUALITY_ENGINE_INVOCATION，1 assessment，金额尚未确定。展示的是旧合成夹具活动，不是本轮调用。
- Edge 展开 1 条证据和 1 条审计，来源/审计主体指向同一评测。刷新后版本、评测、成本 ID 保持一致。截图已保存并查看：`.data/governance-browser-acceptance/21-core-cost-evidence.png`，同名 txt 保存可见事实。
- 数据库前后仍为 CostEvent=7、Execution=0、源质量评测=1、修复 assignment=0、认证=0、发布=0。本轮没有创建或派发业务动作。

## 后续边界

全局成本菜单仍为演示，本轮只接明确质量评测关联成本。发布历史仍缺少非空服务端夹具，未声称已接通其全量证据链。

现有通用 traceability handler 的对象查询缺少本轮要求的可信 principal/workspace 边界，且 release trace 返回较宽内容。本轮没有挂载或调用它；后续需先收敛主体归属、最小字段投影与分页，再接发布证据链，不能直接代理现有无范围 Trace URL。

没有新增数据库迁移或第三方依赖，没有改写历史；未完成全站成本汇总、真实结算、证据下载及完整无障碍验收。成本失败隔离由契约测试覆盖，没有为浏览器故障注入中断 Core 服务。
