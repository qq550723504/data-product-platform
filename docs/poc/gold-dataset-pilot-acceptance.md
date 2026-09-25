# Gold Dataset Pilot 验收报告

Issue: #208  
Parent: #203  
Status: **AUTOMATED ACCEPTANCE PASS / EXTERNAL EXPLANATION TEST PENDING**

> 本报告只记录已由仓库 CI、browser、live-core 和 PostgreSQL integration 证明的事实。人工 External Explanation Test 尚未执行，因此本 Pilot 暂不能标记为最终通过，也不得描述为 production-ready。

## 1. Pilot 目标

验证第二阶段 Gold Dataset 纵向闭环能够在真实 disposable 环境中运行，而不是只证明 domain/unit/API 存在。

Reference chain:

```text
Certified DatasetVersion
 -> Annotation Campaign
 -> real Label Studio tasks
 -> real annotation
 -> Core reconcile
 -> trusted human review
 -> FINALIZED AnnotationSnapshot
 -> Gold DatasetVersion
 -> formal Gold QualityAssessment
 -> Gold Certification
 -> CurrentDeliveryGate
 -> DIRECT_DATA
```

## 2. 已验证运行环境

live-core required gate 已实际运行：

- Core API / Worker
- PostgreSQL
- Redis / asynq
- MinIO
- Web console
- Label Studio Community 1.23.0（仓库固定 digest）
- Chromium / Playwright

Label Studio 使用 disposable test runtime 与临时 API token，不依赖 provider mock。

## 3. 自动化验收结果

### 3.1 Gold Dataset UI / explainability — PASS

PR #231 / #238 / #239 已验证 DatasetVersion 详情可解释：

- input / output DatasetVersion
- GoldProductionBinding ID / root
- Annotation Campaign
- task counts / status
- schema / taxonomy version/hash
- reviewer decision / reviewer / reason
- selected immutable AnnotationResult
- FINALIZED AnnotationSnapshot ID / root
- formal Gold QualityAssessment
- Gold Certification history
- producer Execution / lineage
- Current Delivery Eligibility
- bounded Cost / Evidence / Audit refs

UI 只读，不使用 Label Studio current state 替代 Core frozen facts。

### 3.2 Real Label Studio -> Core AnnotationResult — PASS

PR #232 / #234 已验证：

- real Label Studio project creation
- real task import
- real annotation creation
- EngineRunner / EngineResultReconciler
- provider result -> immutable Core AnnotationResult
- provider binding / external task / external annotation provenance
- normalized author mapping
- task -> REVIEWABLE
- trusted RequestPrincipal review boundary
- forged X-Actor-ID 不可覆盖 trusted reviewer
- ACCEPT review decision
- FINALIZED integrity-valid AnnotationSnapshot

AnnotationResult 与 ReviewDecision 不通过测试 SQL 直接插入。

### 3.3 FINALIZED Snapshot -> real Gold worker / MinIO — PASS

PR #235 已验证：

- CreateBuild idempotency replay -> same Execution
- ExecutionQueued outbox
- actual platform-worker process
- outbox dispatcher -> Redis/asynq -> workflow queue handler
- GOLD_DATASET_BUILDER_V1
- READY Gold DatasetVersion
- real MinIO output object
- persisted checksum == SHA-256(actual output bytes)
- Gold label materialization
- exact DERIVED_FROM lineage
- FINALIZED GoldProductionBinding
- binding matches exact snapshot / output / checksum
- formal Gold QualityAssessment on exact output
- same assessmentAttemptId -> same immutable assessment

### 3.4 Rights / Certification / DIRECT_DATA — PASS

PR #236 已验证：

- required rights closure includes:
  - DATASET_VERSION
  - ANNOTATION_CONTRIBUTION_RESOURCE
- both required resources can receive VERIFIED USE rights
- EffectiveRightsSnapshot -> USE ALLOWED
- canonical Gold CertificationProfile
- exact quality + rights + evidence -> immutable CERTIFIED DatasetCertification
- actual DirectDataService -> ISSUED
- delivery operation references exact Gold Certification
- returned DatasetVersion is the exact Gold output

### 3.5 Fresh re-gate / negative controls — PASS

已自动验证：

- same DIRECT_DATA idempotency key does not duplicate operation/cost
- replay requires a new physical delivery attempt
- annotation contribution rights revoked after certification:
  - historical certification remains CERTIFIED
  - new DIRECT_DATA -> BLOCKED / CURRENT_ENTITLEMENT_BLOCKED
- rights restored:
  - new DIRECT_DATA -> ISSUED
- Gold DatasetVersion invalidated:
  - new DIRECT_DATA -> BLOCKED / DATASET_VERSION_INVALID
  - historical certification remains CERTIFIED
- stale review / unauthenticated / cross-workspace / forged actor fail closed
- snapshot integrity/tamper checks fail closed
- Gold quality blocking failure -> REJECTED Gold Certification
- schema/snapshot/production-proof mismatch fail closed
- provider duplicate/replay paths converge without duplicate Core result/output facts

## 4. Cost / Evidence / Audit Trace — PASS

PR #239 已补齐有界 trace summary。

可按 phase 查询和展示：

- ANNOTATION_ENGINE
- HUMAN_REVIEW
- SNAPSHOT
- GOLD_BUILD
- GOLD_QUALITY
- GOLD_CERTIFICATION
- DIRECT_DATA

Cost 输出只包含 typed subject / quantity / unit / pricing / timestamp；  
Evidence 输出只包含 immutable relation/hash refs；  
Audit 输出 actor/action/object/trace/timestamp。

不会在 explainability 页面无界加载大 metadata。

## 5. 自动化完成定义对照

| # | #208 完成定义 | 状态 | 证据 |
|---|---|---|---|
| 1 | real browser E2E PASS | PASS | browser + live-core required CI |
| 2 | live-core required gate PASS | PASS | #232/#234/#235/#236 live-core |
| 3 | Gold bytes/checksum 与 persisted facts 一致 | PASS | #235 |
| 4 | UI 可追到 annotation/review/source/quality/rights/certification | PASS | #231/#238/#239 |
| 5 | DIRECT_DATA fresh re-gate | PASS | #230/#236 |
| 6 | Cost/Evidence/Audit 按业务阶段回溯 | PASS | #239 |
| 7 | External Explanation Test PASS | **PENDING** | 需非实现人员人工执行 |
| 8 | 输出最终 Pilot 验收报告 | DRAFT | 本文档；待人工测试后定稿 |
| 9 | 列出未验证边界并禁止 production-ready 描述 | PASS | 见第 7 节 |

## 6. External Explanation Test — PENDING

人工测试必须由至少 1 名未参与该 Gold 链路实现的人执行。

受试者只能使用：

- Web UI
- Evidence / Trace 页面
- 产品公开的 ID、hash、history、gate 信息

禁止使用：

- 源代码
- SQL
- ADR / 内部设计文档
- 开发者口头解释

### 必答问题

1. 这份 Gold 数据来自哪些输入数据与版本？
2. 经历了哪些实际生产步骤？
3. 哪些决定由自动引擎产生？
4. 哪些决定由人工完成，谁做的、理由是什么？
5. 谁完成 Annotation，使用哪个 schema/taxonomy？
6. 谁完成 Review，最终选择哪个 immutable result？
7. Gold Quality 为什么 PASS / FAIL？
8. 为什么获得 Gold Certification？
9. 当前为什么允许或拒绝交付？
10. Campaign / Review / Build / Certification / Delivery 分别有哪些可追溯 Cost / Evidence / Audit？

判定规则见：`docs/poc/gold-external-explanation-test.md`。

## 7. 明确未验证边界

本 Pilot **不证明 production-ready**。以下仍不在第二阶段 Pilot 的已验证范围：

- production IAM / enterprise SSO / organization-wide RBAC
- 大规模并发 annotation / review workforce
- Label Studio HA / multi-instance / backup / disaster recovery
- 大数据集吞吐、容量、性能 SLO
- 多地区 / 多租户生产隔离
- provider 长时间不可用与复杂人工恢复流程
- production secrets / credential lifecycle
- 全行业 schema/taxonomy 通用性
- 真实客户数据合规与法律结论
- 长周期成本模型准确性

## 8. 当前结论

截至本报告生成时：

- **自动化 Gold vertical acceptance：PASS**
- **Explainability / trace automation：PASS**
- **External Explanation Test：PENDING**
- **最终 #208 / #203 closure：BLOCKED BY HUMAN EXPLANATION TEST ONLY**

人工测试完成并记录 PASS 后，才可以：

1. 将本文档状态改为 FINAL / PASS；
2. 关闭 #208；
3. 更新并关闭 #203 Epic。

