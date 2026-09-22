# ADR-0011：Data Ingestion 作为外部通用能力，Core 只拥有接入验收与数据产品语义

- 状态：已接受
- 日期：2026-09-22

## 背景

Data Product Platform 的产品链路从“数据接入”开始，但当前架构只有一条 CSV 界面化接入切片：

~~~text
CSV
→ DataResource
→ RAW DatasetVersion
→ Entity Resolution / Processing
→ Quality / Rights / Certification
~~~

与此同时：

- OpenMetadata 已被定义为 Metadata / Governance Projection；
- Processing、Entity Resolution、Quality、Annotation 已有明确 Engine / Adapter 边界；
- `docs/architecture/capability-map.md` 尚未定义通用 Data Ingestion / Integration capability；
- 如果继续沿 CSV 路径逐个加入 MySQL、PostgreSQL、Oracle、Kafka、API、CDC 等实现，会在 Core 内重复建设 connector、checkpoint、增量游标、CDC runtime、技术重试和同步运维能力。

因此需要先定义长期边界，再决定具体 OSS，而不是先引入某个产品后反推其架构位置。

## 决策

将 **Data Ingestion / Integration** 定义为：

> **External commodity capability + Core platform acceptance boundary**

总体关系：

~~~text
External Data Sources
        │
        ├── Metadata Ingestion
        │        ↓
        │   OpenMetadata
        │   metadata / lineage / usage / governance projection
        │
        └── Data Ingestion / Integration
                 │
                 ├── current implementation: CSV / File path directly invokes existing Core commands
                 └── future heterogeneous-source boundary:
                        provider-neutral Ingestion Adapter / Port
                                  │
                                  ▼
                        replaceable external ingestion engine
                                  │
                                  ▼
          Core acceptance boundary
                 │
                 ▼
       immutable RAW DatasetVersion
~~~

### 1. Metadata Ingestion 与 Data Ingestion 分离

OpenMetadata 的 connector/ingestion 用于发现和投影 metadata，不承担把真实业务数据内容持续搬入 DatasetVersion 的职责。

~~~text
Metadata Ingestion
= 知道数据是什么、在哪里、结构/血缘/使用情况如何

Data Ingestion
= 将实际数据内容通过 snapshot / incremental / CDC 等方式带入受控生产边界
~~~

两者可以绑定到同一个 DataResource，但不能混成同一 System of Record。

### 2. 外部 ingestion capability 拥有技术执行细节

成熟 ingestion engine / connector system 可以负责：

- source connectivity / protocol / driver；
- technical schema discovery；
- file/database/API/message-system connectors；
- batch snapshot；
- incremental sync；
- CDC；
- checkpoint / offset / partition；
- engine-internal retry / recovery；
- connector/runtime metrics 与技术日志。

这些能力默认不在 Core 中重新实现，也不把 provider-specific checkpoint/job schema 复制成 Core Domain。

### 3. Core 拥有接入结果的业务语义

Core 继续拥有：

- DataResource；
- Dataset / DatasetVersion；
- workspace/source ownership/binding；
- 被平台接纳的 source provenance；
- 不可变 content identity / checksum / manifest identity；
- Dataset lineage；
- platform Evidence / Audit / Cost；
- Rights / Effective Rights；
- QualityAssessment；
- DatasetCertification 与 Delivery gate。

**外部 ingestion job 成功不是 Core 业务成功。**

只有 Core 对接入结果完成必要验证并接受后，才能建立/完成对应 RAW DatasetVersion 事实。

### 4. 当前 CSV 是 Native Adapter，不是通用模型

现有 `/ingest` CSV 路径继续保留，它代表当前最小 File ingestion slice，**但当前并不存在 `DataIngestionProvider` / 通用 ingestion Port**；CSV 页面仍直接调用现有 Resource / Dataset / UploadVersion 等 Core commands。

它的 CSV 格式、浏览器预览、512 KiB / 1000 rows 等 POC 约束，不上升为通用 Data Ingestion contract。未来 provider-neutral ingestion Port 只在首个真实异构来源 vertical slice 中按实际需求落地。

未来数据库、API、对象存储、消息系统或 CDC 接入不得通过复制 CSV handler 的方式逐源扩张。

### 5. 首个真实异构接入再定义最小 Port

本 ADR 不提前创建大型 `ingestion` Domain，也不引入：

- IngestionJob/Connector/Pipeline/Schedule 等完整平台模型；
- 第二套 scheduler；
- 第二套 generic workflow runtime；
- provider-specific offset/checkpoint tables。

当首个真实 database/API/CDC vertical slice 出现时，再定义最小 provider-neutral Port / Adapter contract。

该 contract 应只暴露 Core 真正需要的稳定边界，例如：

- Core 先持久化的 stable provider_request_key / invocation request identity；
- 每一次真实 provider invocation 在调用前持久化的 physical attempt identity；
- provider/external execution reference（允许在调用后才能获得）；
- source identity/binding；
- output artifact or manifest reference；
- snapshot/window/cut identity（如适用）；
- content/schema fingerprint（如适用）；
- provider-neutral status / metrics / diagnostics classification；
- append-only provider attempt outcome / observation / reconciliation evidence。

如果 Core 主动触发外部 ingestion，必须遵循 DB-first crash-safe 调用语义：

1. 调用 provider 前先持久化 stable provider_request_key 与 physical attempt identity/start fact；
2. provider API 必须支持相同 request identity 的 idempotent replay/lookup，或 Adapter 提供等价 same-operation recovery；若 provider 无法做到，必须显式设计不会盲目重复启动同步的 recovery contract；
3. success / explicit failure / timeout / unknown 只能追加 outcome/observation，不能覆盖 start fact；
4. response 丢失时通过同一 request identity 查询/恢复原 operation，不得直接创建第二个同步任务；
5. reconciliation 若真实调用 provider API，则使用新的 physical attempt identity；如该调用可能计费，按现有 CostEvent attempt 规则独立记录；
6. external execution reference 只是 provider 返回后的外部标识，不能替代调用前已经持久化的 request/attempt identity。

具体字段由真实需求驱动，禁止为未来假设提前建模。

### 6. CDC 与不可变 DatasetVersion 的关系

持续 CDC runtime 状态不等于 DatasetVersion。

~~~text
CDC stream
   ↓
provider checkpoint / offset
   ↓
explicit snapshot/window/cut
   ↓
accepted immutable RAW DatasetVersion
   ↓
Quality / Rights / Certification
~~~

如果数据需要进入质量评测、认证或交付链，必须形成明确的不可变版本边界。

禁止：

- 让一个已认证 DatasetVersion 指向持续原地变化的 live table/object；
- 每条 CDC event 都机械创建一个 DatasetVersion；
- 把 engine checkpoint 当作平台 DatasetVersion identity。

checkpoint/offset 默认由 ingestion provider 管理；只有当它是解释某次版本边界所必需的 provenance 时，Core 才保存 provider-neutral reference/evidence。

### 7. 调度与恢复责任只能有一个 owner

专业 ingestion system 可以拥有自己的 scheduling/retry/recovery。

Core 可以：

- 按业务 Command 触发外部同步；
- 保存必要的 external reference；
- 观察结果；
- reconciliation 不确定 outcome；
- 对接纳结果建立平台事实。

但同一链路不得同时由 Core scheduler 和 external ingestion scheduler 各自独立重试/推进，避免双运行时产生重复同步和不一致状态。

## Build-vs-Buy / Reuse Check

当前仓库已有：

- Native CSV ingestion slice；
- queue / worker / reconciliation；
- remote Processing Engine Adapter 模式；
- Object Storage；
- DatasetVersion immutable semantics。

这些能力不足以构成成熟 database/API/CDC connector platform，但也不应被丢弃或平行复制。

首个真实异构接入前，至少比较适合该场景的成熟方案，例如：

- Apache SeaTunnel；
- Apache InLong；
- Airbyte；
- Apache NiFi；
- Debezium；
- Apache Flink CDC。

本 ADR **不选择默认产品**。不同工具覆盖层级不同，选型必须基于真实 source type、batch/CDC、self-host、license、运维和 UI/API 要求完成新的 Build-vs-Buy / Reuse Check。

第三方集成继续遵循 ADR-0010：

~~~text
Native API / Standard Protocol
→ Plugin / Extension
→ Adapter
→ Sidecar
→ Fork
~~~

Core Domain 不得出现某个 ingestion 产品专属类型。

## 后果

### 正面

- CSV 不再隐式成为未来所有接入方式的架构模板；
- 避免在 Core 重复开发数据库 connector、CDC 和 checkpoint runtime；
- OpenMetadata Metadata Ingestion 与真实 Data Ingestion 职责清晰；
- 可以按场景替换 ingestion provider；
- DatasetVersion / Rights / Quality / Certification 的核心语义不被外部同步产品接管。

### 代价

- 真实异构接入时需要定义最小 Port / Adapter contract；
- 需要设计 provider output 到 immutable DatasetVersion 的 acceptance protocol；
- CDC 场景必须显式定义版本 cut/window 语义；
- 需要避免 Core 与外部 provider 双重调度。

## 非目标

本 ADR 不：

- 引入或部署 SeaTunnel / InLong / Airbyte / NiFi / Debezium / Flink CDC；
- 新增数据库 migration；
- 新增通用 Connector UI；
- 新增 CDC implementation；
- 新建完整 IngestionJob Domain；
- 修改现有 CSV POC 行为；
- 改变 OpenMetadata 仅作为 Governance Projection 的既有决策。

## 关联

- ADR-0002 OpenMetadata Governance Projection
- ADR-0004 Engine SPI
- ADR-0010 Commodity Capability Reuse
- `docs/architecture/capability-map.md`
- `docs/architecture/system-architecture.md`
- `docs/poc/csv-ingestion-v1.md`
- Issue #164
