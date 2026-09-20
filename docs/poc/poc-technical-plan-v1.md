> **状态：COMPLETED（历史 POC 基线）**
>
> 本文件记录核心 POC 当时的目标、Sprint 和成功标准，不因后续产品演进而重写。
> 当前项目已进入 #129 Certified Dataset 受控试点。
> 后续路线见 docs/product/certified-dataset-pilot.md、docs/architecture/certified-dataset.md、docs/architecture/data-rights-provenance.md。

# Data Product Platform POC 技术计划 V1.0

## 1. POC 目标

验证核心平台能够从原始数据生产出一个不可变、可解释、可追溯的 Data Product Release。

当系统能够回答以下问题时，POC 即视为成功：

1. 这个 Release 由哪些源数据生产而来？
2. 为什么这些源记录被解析为同一个权威实体（canonical entity）？
3. 使用了哪些 workflow / indicator / contract / rules / rights 版本？
4. 做出了哪些质量与合规决策？
5. 发生了哪些 Cost Event？
6. 有哪些 Evidence 支撑该 Release？

主 Epic：GitHub issue `#1`。

## 2. 参考场景

- Use Case：企业融资风险辅助
- Data Product：企业经营活跃度 V1.0
- 数据源：enterprise、lease、energy
- 首个 POC 排除：人脸、详细门禁记录、停车/视频数据

权威的参考实现规格（Canonical Reference Implementation specs）：

- `examples/enterprise-activity/README.md`
- `examples/enterprise-activity/indicators-v1.md`
- `examples/enterprise-activity/test-vectors-v1.yaml`
- `examples/enterprise-activity/workflow/workflow-v1.yaml`
- `examples/enterprise-activity/contract/data-contract-v1.yaml`
- `industry-packs/park/matching/company-match-policy-v1.yaml`
- `industry-packs/park/indicators/enterprise-activity-v1.yaml`
- `industry-packs/park/quality/enterprise-activity-quality-v1.yaml`
- `industry-packs/park/compliance/enterprise-activity-compliance-v1.yaml`

V1 指标语义是用于 POC 的可解释规则，而不是经过验证的信用/授信模型。

## 3. 技术基线

POC 运行时：

- Web：React / Next.js
- API：Go
- Worker：Go
- PostgreSQL
- Redis
- MinIO/S3 兼容存储

适配器在核心垂直切片稳定之后再引入。

## 4. Sprint 计划

### Sprint 0 —— 基础设施（`#2`）

交付：

- 仓库骨架
- API/worker 入口
- 迁移（migrations）
- PostgreSQL 连接
- Redis 队列 / outbox worker
- 对象存储抽象
- 事务管理器
- 错误模型
- 审计基础
- 结构化日志 / trace ID

验收：

- API 可启动
- worker 可启动
- 迁移可执行
- 对象上传/下载可用
- outbox 事件能被消费

### Sprint 1 —— Resource / Dataset / Entity / Evidence（`#3`、`#4`、`#8` 基线）

交付：

- DataResource
- Dataset / 不可变 DatasetVersion
- EntityType / Entity / EntityMapping
- CSV 上传
- RAW Dataset V1
- 企业标准化
- 精确/人工实体匹配
- STANDARDIZED Dataset V1
- Evidence / Audit / 基线 CostEvent

验收：

系统能够将 Standardized V1 追溯到源 CSV，并解释每一条经过复核的企业映射。

### Sprint 2 —— Workflow / Product / Release（`#5`、`#7`）

交付：

- Workflow / WorkflowVersion
- Task / Execution
- 依据冻结规格实现 V1 指标计算
- CURATED Dataset
- DataProduct / ProductVersion
- ProductRelease / ReleaseReadiness

本 Sprint 早期 Rights/Quality/Compliance 可以使用 mock provider，但必须使用真实的状态机。

验收：

一个 Product Release V1.0 可以被校验并发布，之后即变为不可变。

### Sprint 3 —— 真实门禁 / Contract / Cost（`#6`、`#12`、`#14`、`#16`、`#26`、`#27`）

交付：

- Authorization 基础
- 原生 Quality Engine
- 原生 Compliance 规则
- DataContract / ContractVersion
- CostEvent
- EvidenceSnapshot
- 确定性公式测试向量（test vectors）
- 完整发布路径集成测试

验收：

当 rights/quality/compliance/contract 就绪检查失败时，Release 发布被阻止；指标行为对已提交的 fixtures 具有确定性。

### POC UI（`#13`）

仅构建运营/演示该垂直切片所需的页面；不要将 OpenMetadata/Hop/Splink 暴露为顶层产品概念。

### Sprint 4 —— OpenMetadata 适配器（`#9`）

交付：

- MetadataEngine 接口
- OpenMetadata 适配器
- ResourceBinding
- 产品治理投影

验收：

OpenMetadata 故障不会使核心 Product Release 失效；同步操作异步重试。

### Sprint 5 —— Apache Hop 适配器（`#10`）

交付：

- ProcessingEngine 接口
- Hop 适配器
- 执行状态对账（reconciliation）

验收：

可以在不改变 Dataset/Product/Release 语义的前提下，将某个原生处理任务切换为 Hop。

### Sprint 6 —— Splink 适配器（`#11`）

交付：

- EntityResolutionEngine 接口
- Splink 候选生成
- 置信度阈值
- 复核队列集成

验收：

实体解析指标能够报告精确/概率/人工/未解析的结果，并能比较策略版本。

## 5. POC UI

一级导航：

- 工作台
- 场景
- 数据资源
- 数据集
- 数据生产
- 实体中心
- 数据产品
- 证据中心

Rights / Quality / Compliance / Contract / Cost 在 POC 阶段以详情页上下文标签的形式出现。

## 6. 第一条垂直切片

```text
enterprise.csv
→ DataResource
→ RAW Dataset V1
→ 企业标准化
→ Entity Mapping / 人工复核
→ STANDARDIZED Dataset V1
→ Evidence
```

该切片不依赖 OpenMetadata、Hop 或 Splink。

## 7. 第二条垂直切片

```text
enterprise + lease + energy 标准化数据集
→ Workflow
→ V1 指标
→ CURATED Dataset
→ Quality / Compliance
→ Data Contract
→ Data Product 1.0
→ Product Release
→ EvidenceSnapshot + Cost
```

## 8. 最终演示

演示还必须模拟至少一种失败情形（例如授权过期），并展示影响分析 / 就绪检查失败 / 健康度降级或暂停。

## 9. POC 非目标

在核心 POC 期间，除非经明确批准，不得实现：

- 完整 Billing / Settlement
- 完整会计/ERP
- 区块链平台
- 复杂的微服务基础设施
- 黑盒金融风险评分
- 高风险个人监控数据集
- 参考垂直切片不需要的、宽泛的通用平台功能

## 10. 执行规则

通用持久化/基础设施工作可以在参考语义逐步细化的同时推进，但指标/工作流实现必须符合已提交的 V1 规格。当规格未作规定时，AI Coding Agent 不得自行发明评分公式、缺失值填充行为、状态迁移或引擎专属领域字段。
