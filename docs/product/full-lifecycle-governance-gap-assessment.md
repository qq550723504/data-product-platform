# 全生命周期数据治理：能力差距评估

> 核查日期：2026-10-10；固定基线：`main@010a80c30b5a74a638faaaa27ebc7206634606f4`。
> 配套：[产品蓝图 V2](full-lifecycle-governance-v2.md)、[ADR-0014](../adr/0014-full-lifecycle-governance-boundary.md)；路线 #296。
> 本文记录源码/文档与 Issue/PR 核查，不是本轮重新运行所有 Pilot 的验收报告。

## 1. 判读方法

- **受控 Pilot 已记录验收**：既有报告明确记录通过；不能推广为生产就绪。
- **有限实现 / 参考适配**：存在代码、接口或参考工作流，但未证明当前目标的通用运行能力。
- **架构候选**：职责已定义，产品/接口/部署尚未落实。
- **目标缺口 / 未验证**：本轮查阅的来源没有覆盖对应完整闭环；不是对全仓代码不存在的数学证明。
- **分支实现**：未合并 PR 的实现和证据不计入 main；任务正文中的 PASS 也不等于本轮独立复跑。

不提供完成百分比：不同能力的工作量、覆盖范围和安全门槛不可相加。表中来源全部对应固定 main；任务状态另以查询时点为准。

## 2. 能力矩阵

| 能力 | 当前证据与判定 | 目标差距 | 归属 / 下一步 |
|---|---|---|---|
| 产品主链 | 已有 DataResource→DatasetVersion→加工→质量/Rights→认证→交付的受控 Pilot [E1][E2] | 不能据此声称跨数据源、持续运营与销毁均可用 | 复用，A 只扩长期定位 |
| 发现/目录/元数据 | ADR-0002 与 MetadataEngine/ResourceBinding 已定义治理投影 [E3] | 需验证用户自己的来源发现、稳定绑定、责任/术语/标签呈现、陈旧状态与权限一致性 | B 单来源最小切片；通用目录不重造 |
| 分类分级/标准 | Industry Pack/元数据边界已有，未见统一闭环验收 [E1][E3][E4] | 字段写入 owner、批准版本、分类与保护等级、进入业务 gate 的接纳方式未形成此目标下的完整契约 | B 最小责任与分类绑定，C 变化处理；全面设计器后续 |
| CSV/File 接入 | 已有 /ingest，512 KiB/1000 rows 等窄界面约束，直接复用 Core commands [E5] | 不是通用 ingestion Port，不支持任意来源与清洗配置 | B 不复制 CSV handler 建各类 connector |
| 数据库/API/CDC | ADR-0011、#164/#165 只完成职责和候选边界 [E6] | 无首个真实异构来源纵向运行证明；SeaTunnel 未选定默认实现 | B / #298；CDC 与其版本 cut 后续 |
| 清洗/标准化 | 企业名/地址/信用代码规范化、日期解析、负能耗隔离等园区实现 [E7][E8] | 固定场景能力，不是通用清洗工作台；缺规则选择—前后差异—修复复检体验 | B 固定模板，C 异常闭环 |
| 外部加工 | ManagedProcessingEngine 生命周期接口，Hop 月度能耗 SUM 参考工作流 [E9][E10] | 有适配不等于默认部署或通用加工产品；demo 的 HOP_ENABLED=false [E11] | 优先复用，不强行改成 dbt |
| 质量/认证 | 冻结评测、规则证据、Rights 与认证及受控交付已有 Pilot 证据 [E2] | 需覆盖新来源输出；live source profiler 不等于版本化质量证明；持续分派/复检未在该 Pilot 内验证 | B/C，不重建 QualityAssessment |
| 实体/Gold | Rules/实体映射及官方 CE→Core Review→Snapshot→Gold 的受控链已有记录 [E1][E12] | 不能将 Entity 等同完整 MDM，也不能将受控 Gold 等同全模态/生产标注平台 | 既有能力复用；#294/#295 独立收尾 |
| 数据安全与使用授权 | Core Rights/CurrentDeliveryGate 及 trusted DIRECT_DATA 已验证；CSV demo actor 不是认证 [E2][E5] | 最小可信身份必须覆盖新连接/读取/处理路径；目录 RBAC 不是全数据面授权；生产级 IAM/脱敏体系不能从现有 gate 推导 | B 受控边界；真实数据前专项准入 |
| 数据服务与使用观察 | 当前受控 DIRECT_DATA；外部发布与 runtime/usage 仍由 #84/#86/#87 跟踪 | 不代表已具备通用 API 数据服务、外部使用审计或全链路撤回副本 | B 复用；外部能力不重复立项 |
| 持续治理 | 有 Worker/Outbox/Reconciliation 和质量事实基础 [E1][E4] | 缺少统一“检测→分派→修复→复检”及有限时效/schema 变化场景验收 | C / #299 |
| 保留/归档/销毁 | 已有历史不可变及生命周期约束 [E13]；本轮检索未找到对应完整处置契约与运行验收 | 内容可用性、保全、派生/共享副本、备份、Evidence 敏感内容、恢复/删除并发须设计 | D-design / #300；runtime 尚未立项实施 |
| 用户可操作与生产条件 | 本地演示、Certified/Gold 浏览器证据存在；当前仍非 production-ready [E1][E2][E12] | 非开发者在新来源上的独立操作与解释、最小部署安全及生命周期处置不能由旧测试替代 | B/C 单独验收；生产按场景批准 |

## 3. 近期任务关系与真实进度

核查时 main 未包含 #295；该 PR 为 draft/open，HEAD 为 `433d5733cca384c2f59ebbda04f4122d54a0b1f0`。其描述记录精确 HEAD CI 通过，但 LS #76 新候选及浏览器 Submission→Core→Gold 跨仓运行仍标记 NOT_RUN。此处只转述该 PR 的状态，不重审代码、不改变 owner，不把分支成果归为 main 已完成。[E14]

#164/#165 已定义数据接入边界，不需要再开同一架构任务；#298 负责它之后的首条实现切片。#47/#84/#86/#87 保留外部发布与使用事件归属。新的路线不是重开旧 Certified/Gold Epic。

| 新任务 | 目的 | 当前本轮交付 |
|---|---|---|
| #296 | 全生命周期方向与首批工作编排 | 创建 Epic，不代表整体完成 |
| #297 | 蓝图、差距、ADR 与入口同步 | 本文档批次；需按 PR 状态判断是否合并 |
| #298 | 单数据库源治理/生产/交付 | 已立项待执行；先做 B0 复用/选型及有限契约 |
| #299 | 质量修复与持续治理 | 已立项，依赖 #298 |
| #300 | 保留/归档/销毁设计 | 已立项，可在 #297 合并后并行设计；不执行删除 |

## 4. 应优先消除的三类风险

**范围风险：**“全生命周期”是长期方向。先用同一来源证明可操作的闭环，不以接入更多组件或添加更多导航衡量进度。

**权威风险：** OpenMetadata 的 current labels/technical lineage 与 Core 的冻结输入、质量、Rights、认证和交付不是同一事实。目录同步失败不允许放宽 gate；治理观察可以触发复核，不可悄悄重写历史。

**终止风险：** 数据内容到期、停止使用、实际归档/删除和审计保留不可混为一个 status。尤其旧 Evidence 可能含正文，不能只删主 CSV 就宣称完成合法处置，也不能在本轮破坏 immutable guards。

## 5. 核查限制

本轮读取了当前愿景、能力图、系统架构、ADR、AGENTS 及任务状态，并结合本次对话在相同 main SHA 下核对过的具体代码/工作流。未对全部运行端点、安全控制、全部迁移或所有第三方部署重新执行测试。负面结论限于列出的证据及检索范围。

表中历史 PASS 属于原报告，不是本轮新测试。后续开工应重新核对 main 与 #294/#295；不得照抄本文静态任务状态。官方组件资料只用于判断职责，不证明已选择的 edition/license、API/connector 或部署可行性。

## 证据索引（固定基线）

[E1]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/docs/product/product-vision.md
[E2]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/docs/product/certified-dataset-pilot-acceptance.md
[E3]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/docs/adr/0002-openmetadata-governance-projection.md
[E4]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/docs/architecture/capability-map.md
[E5]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/docs/poc/csv-ingestion-v1.md
[E6]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/docs/adr/0011-data-ingestion-capability.md
[E7]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/apps/platform/internal/entity/matching/normalize.go
[E8]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/apps/platform/internal/workflow/native/engine.go
[E9]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/apps/platform/internal/workflow/application/processing_engine.go
[E10]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/examples/enterprise-activity/workflow/energy-monthly-hop-v1.yaml
[E11]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/deploy/demo/compose.yml
[E12]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/docs/poc/gold-dataset-pilot-acceptance.md
[E13]: https://github.com/qq550723504/data-product-platform/blob/010a80c30b5a74a638faaaa27ebc7206634606f4/AGENTS.md
[E14]: https://github.com/qq550723504/data-product-platform/pull/295
