# Certified Dataset enterprise-activity Pilot 验收报告

> 状态：**FINAL — E2E1–E2E20 PASS**。E2E18 已由 #194 / CI #998 的真实 live-core browser gate 验证。
>
> **本报告不是生产上线批准。** 它只说明当前仓库的受控 enterprise-activity Reference Implementation 已经验证了 Certified Dataset 第一阶段业务闭环。

## 1. 试点范围

本次 Pilot 只验证仓库当前已经落地的 trusted server-side **DIRECT_DATA** 模式：

~~~text
enterprise.csv / lease.csv / energy.csv
  ↓
RAW DatasetVersions
  ↓
Entity Resolution
  ↓
STANDARDIZED DatasetVersion
  ↓
Native Execution
  ↓
CURATED DatasetVersion
  ↓
QualityAssessment / Quality Report
  ↓
Rights / Compliance / Contract
  ↓
EffectiveRightsSnapshot
  ↓
DatasetCertification
  ↓
Current Delivery Eligibility
  ↓
trusted principal / effective consumer
  ↓
fresh CurrentDeliveryGate
  ↓
DeliveryOperation
  ↓
DIRECT_DATA
~~~

不在本轮验收范围：

- bearer / presigned credential provider；
- provider credential revoke / containment / recovery；
- 完整企业 IAM / delegation management；
- T4/T5/T6、灾备、性能压测与生产 SLA；
- Label Studio / X-AnyLabeling；
- AI / Gold Dataset 第二阶段。

## 2. Reference fixture

固定输入：

| 数据源 | 记录数 | 用途 |
| --- | ---: | --- |
| enterprise.csv | 6 | 企业主数据 / Entity Resolution anchor |
| lease.csv | 10 | 租赁活动输入 |
| energy.csv | 14 | 能源活动输入 |
| **合计** | **30** | Pilot RAW reference records |

Entity Matching Policy：

- `park-company-match@1.0.0`
- USCC exact → AUTO_MATCH / confidence 1.0
- normalized name + address exact → AUTO_MATCH / confidence 0.98
- name similarity + legal representative → REVIEW / confidence 0.85
- otherwise UNRESOLVED

Quality RuleSet：

- `enterprise-activity-quality@2.0.0`
- 11 rules
- 6 dimensions:
  - COMPLETENESS
  - ACCURACY
  - CONSISTENCY
  - UNIQUENESS
  - TIMELINESS
  - TRACEABILITY

## 3. 核心验收结果

### 3.1 数据生产与实体解析

已验证：

- 3 个 RAW DatasetVersion 从真实 CSV fixture 创建；
- enterprise anchor 经过 Entity Resolution；
- STANDARDIZED DatasetVersion = READY；
- STANDARDIZED output 绑定唯一 `generated_by_entity_match_job_id`；
- MatchJob、input/output/source identity 可反查到同一个 SUCCEEDED job；
- 每个 candidate 都存在 frozen immutable MappingDecision；
- producer identity DB mutation 被拒绝；
- Native Execution 只产生一个 CURATED live output；
- CURATED DatasetVersion 的 `generated_by_execution_id` 指向实际 Execution；
- Execution dependency、lineage 与 output producer identity 一致。

Reference fixture 的 live-browser 流程明确触发 1 条人工审核候选。最终 STANDARDIZED output 覆盖全部 6 条 enterprise anchor records，因此：

- 标准化成功率：6 / 6 = **100%**
- 人工复核触达率：1 / 6 = **16.7%**
- 无人工复核路径：5 / 6 = **83.3%**

这些比例只描述当前固定 fixture，不是生产匹配率或 SLA 基准。

### 3.2 Quality

成功样例：

- RuleSet 共 11 条规则；
- Quality Gate = PASS，blocking FAIL = **0**；
- 六个维度全部存在；
- Quality Report 返回完整 bounded findings page；
- 每条 finding 都能解释：
  - rule
  - dimension
  - severity
  - status
  - observed facts
- Quality Evidence / Audit refs 非空。

失败样例：

- 在 DatasetVersion READY 后 +48h 重新运行真实 Quality Engine；
- CRITICAL `QA-FRESHNESS` 失败；
- targeted negative slice：CRITICAL FAIL = **1**；
- Quality Gate = FAIL；
- 使用该 immutable failed QualityAssessment 认证时，DatasetCertification = REJECTED。

历史冻结：

- 同一 RuleSetRef 后续变更版本/内容，会产生新的 QualityAssessment；
- 原 Assessment 的 rule version / source content / content SHA 保持不变。

### 3.3 Rights / Compliance / Contract

已验证：

- 3 个 lineage source resource 都有真实 RightsDeclaration / Authorization provenance；
- EffectiveRightsSnapshot finalized；
- READ action 当前允许；
- Compliance Gate = PASS；
- Data Contract `DP-ENTERPRISE-ACTIVITY` 已发布并参与认证；
- required Rights / Compliance / Contract 任一缺失时，Certification 独立 REJECTED；
- Authorization revoke 后：
  - historical Certification 不改写；
  - CurrentCertificationGate 仍可解释历史认证；
  - CurrentEntitlementGate BLOCKED；
  - replacement delivery fresh re-gate → BLOCKED / 0 payload；
- consumer / purpose / action / scope / provenance context mismatch fail closed。

### 3.4 Certification

成功样例：

- explicit CertificationProfile；
- DatasetCertification = CERTIFIED；
- 绑定 frozen：
  - QualityAssessment
  - RightsSnapshot
  - EffectiveRightsSnapshot
  - ComplianceResult
  - ContractVersion
  - Traceability Evidence
  - EvidenceSnapshot
  - CertificationProfile snapshot

已验证历史语义：

- 新 DatasetVersion 不继承旧 QualityAssessment / Certification；
- Certification revoke/supersede 不改写旧历史事实；
- ProfileRef 新 V2 不改写 Profile V1；
- 历史 Certification 继续引用原 QualityAssessment + Profile V1 frozen snapshot；
- DatasetVersion INVALID 后 historical Certification 仍可查，但 CurrentDeliveryGate BLOCKED。

### 3.5 Trusted DIRECT_DATA

已验证：

- StaticPrincipalResolver 作为受控试点 trust boundary；
- principal A 不能在 body 中冒充 consumer B：
  - HTTP 403 `CONSUMER_PRINCIPAL_MISMATCH`
  - 请求在 DirectDataService 前被拒绝
  - 不产生 DeliveryOperation；
- 正确 consumer：
  - HTTP 200
  - 返回真实 CURATED immutable bytes
  - persisted DeliveryOperation 保存 trusted `principal_ref` 与 `effective_consumer_ref`；
- delivery command 不信任 eligibility preflight，会 fresh re-gate；
- ALLOWED path：
  - DeliveryOperation 先在事务中提交为 ISSUED
  - ObjectStore.Get / 第一 response byte 只能发生在 commit 后；
- BLOCKED path：
  - persisted BLOCKED operation
  - ObjectStore 不打开
  - 0 dataset bytes；
- response loss window：
  - ISSUED 已提交、第一字节前失败，原 operation 保持单一 ISSUED 历史；
  - same idempotency key 不重放 payload；
  - replacement 必须 new key + retry_of；
  - replacement fresh re-gate；
- delivery fence 双事务测试：
  - revoke-first → delivery BLOCKED；
  - finalize-first → delivery ISSUED，revoke 必须等 fence；
  - PostgreSQL lock timeout 证明唯一线性顺序，无 lock cycle。

## 4. Frozen evidence tamper

Certification 实际引用的 snapshots 已验证：

### EvidenceSnapshot

FINALIZED 后尝试：

- INSERT membership
- UPDATE membership
- DELETE membership

均由 PostgreSQL 拒绝。

重新读取：

- root hash 不变
- manifest 不变
- membership 不变
- IntegrityValid=true

### RightsSnapshot

对以下 frozen membership：

- authorization
- declaration
- provenance binding

INSERT / UPDATE / DELETE 均由 PostgreSQL 拒绝。

重新读取：

- root hash 不变
- manifest 不变
- declaration IDs 不变
- binding IDs 不变
- Certification 历史仍引用原 snapshot。

## 5. Cost / Audit / Evidence

Pilot 不在测试末尾反推“伪成本”；成本来自真实 activity / attempt。

### Quality

- `QUALITY_ENGINE_INVOCATION`
- 1 次 successful assessment physical invocation
- typed 回溯到 QualityAssessment
- same fact 的历史解释有 Audit + Evidence

### Rights

- 3 次 `RIGHTS_DECLARATION_VERIFICATION`
  - 对应 enterprise / lease / energy 三个 lineage resources
- 1 次 `EFFECTIVE_RIGHTS_COMPUTE`
- allocation 绑定 finalized EffectiveRightsSnapshot
- Audit + Evidence 可追溯

### Certification

- 1 次 `CERTIFICATION_EVALUATION`
- typed 到 DatasetCertification
- same-key replay 返回同一事实，cost count 保持 1
- Audit + Evidence 可追溯

### DIRECT_DATA

- 每个新 DeliveryOperation 写 1 次 `DIRECT_DATA_DELIVERY_ATTEMPT`
- activity identity = DeliveryOperation ID
- quantity = 1 attempt
- pricing mode = ACTUAL
- same-key replay 不新增 operation，不重复成本
- ISSUED / BLOCKED attempt 都可审计
- Audit + Evidence 可按 DeliveryOperation 追溯

## 6. 失败路径矩阵

| 场景 | 结果 |
| --- | --- |
| CRITICAL Quality FAIL | Certification REJECTED |
| required Rights missing | Certification REJECTED |
| required Compliance missing | Certification REJECTED |
| required Contract missing | Certification REJECTED |
| 新 DatasetVersion 未重新认证 | CERTIFICATION_NOT_CURRENT |
| Profile action mismatch | CERTIFICATION_ACTION_NOT_COVERED |
| Authorization / provenance consumer mismatch | CURRENT_ENTITLEMENT_BLOCKED |
| principal 冒充 consumer | HTTP 403 / 0 operation |
| Authorization revoke after preflight | replacement delivery BLOCKED / 0 bytes |
| DatasetVersion INVALID | CurrentDeliveryGate BLOCKED |
| Certification revoked/superseded | CurrentCertificationGate BLOCKED |
| same delivery idempotency key replay | payload 不重放 |
| ISSUED 后第一字节前 response loss | explicit replacement required |
| finalized Evidence/Rights membership tamper | PostgreSQL拒绝 |

## 7. Browser / UI

E2E18 final live-core Certified Dataset browser gate：**PASS — #194 / CI #998**。

真实环境：

- Core API / Worker；
- PostgreSQL / Redis / MinIO；
- standalone Next console；
- Chromium / Playwright。

目标页面：

`/datasets/{datasetId}/versions/{versionId}`

已验证：

- DatasetVersion full ID / persisted checksum；
- read model 暴露 immutable `generatedByExecutionId`，页面链接到 exact producing Execution；
- Quality Assessment / Quality Report；
- 预期维度来自独立 domain contract `qualitydomain.QualityDimensions`，六个维度逐行唯一匹配且全部 PASS；
- CERTIFIED history；
- EffectiveRights snapshot ID / hash；
- Certification EvidenceSnapshot / Certification ID；
- Current Delivery Eligibility：
  - DatasetVersion usability = ALLOWED
  - Current Certification = ALLOWED
  - Current Entitlement = ALLOWED
  - 预检区域无 BLOCKED；
- browser 前置 Go acceptance 证明：
  - 同一 DatasetVersion 在验收时可通过 ProductRelease trace 追溯；该断言是 point-in-time trace verification，不表示 `product_release_dataset` membership 已具备数据库级不可变性；
  - 真实 MinIO bytes 的 SHA256 与 persisted DatasetVersion checksum 一致；
  - Certification 指向同一 DatasetVersion / QualityAssessment / EffectiveRightsSnapshot / EvidenceSnapshot；
  - Certification EvidenceSnapshot integrity valid。

CI #998 同时通过：go-platform、web-console、browser-contracts、demo lifecycle、live-core、required。

## 8. 当前已知限制

这些限制不属于本次 Pilot 失败：

1. **不是生产 IAM**
   - 当前 HTTP delivery 使用受控 static principal resolver；
   - 未验证企业 IdP、delegation management、组织级 provisioning。

2. **只验证 DIRECT_DATA**
   - bearer / presigned credential provider 未进入本阶段；
   - provider revoke / containment / recovery / expiry-cap 未进入本阶段。

3. **不是性能/SLA 验收**
   - 没有生产规模负载、容量、灾备、RTO/RPO、p95/p99 SLA 结论。

4. **Reference fixture，不是生产数据质量基准**
   - 30 条固定 synthetic records 用于闭环验证；
   - 16.7% 人工复核触达率只属于该 fixture。

5. **仍有非 Pilot blocker 的 Core debt**
   - #99 ProductRelease / ProductVersion 其余冻结项，包括 `product_release_dataset` membership 的 DB-level freeze；
   - #100 Metadata stale projection / Hop unknown submit / manual-upload idempotency 等；
   - 只有后续真实场景复现时才升级优先级。

6. **UI pagination follow-up**
   - #161 DatasetCertification history pagination 为 P2，不阻塞本 Pilot。

## 9. 第一阶段结论

最终状态：

- E2E1–E2E17：PASS
- E2E18：PASS — #194 / CI #998
- E2E19：PASS — Pilot slices 与最终 #194 required gate 全绿
- E2E20：PASS — 本报告

因此可判定：

> Certified Dataset 第一阶段的“生产 → 质量 → 权利 → 认证 → 当前资格 → trusted DIRECT_DATA → UI/trace”受控闭环完成。

该结论用于关闭 #136 与 #129 的**第一阶段 MVP/Pilot**，不得解释为生产上线批准，也不代表生产 IAM、provider credential、性能/SLA 或灾备验收通过。

## 10. 后续决策入口

Pilot 完成后，不自动扩平台底座。下一阶段从真实产品优先级中选择：

- AI / Gold Dataset；
- Delivery Hardening / external provider；
- Core reliability debt；
- Pilot 客户/行业反馈驱动的业务增强。

在做出下一阶段选择前，保持当前能力边界，不提前引入新的基础设施层。
