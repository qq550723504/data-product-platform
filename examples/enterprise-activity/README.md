# Enterprise Activity Reference Implementation V1.0

## 1. Purpose

Reference Use Case: **企业融资风险辅助**。

Reference Data Product: **企业经营活跃度 V1.0**。

目标不是构建银行授信模型，而是用一个可解释、可版本化的场景验证完整 Data Product 生产链：

```text
Raw Data
→ Data Resource
→ DatasetVersion
→ Entity Resolution
→ Processing
→ Compliance Gate
→ Quality Gate
→ Data Contract
→ Data Product Version
→ Product Release
→ Evidence + Cost
```

> V1 的指标、权重和等级阈值仅用于 POC 语义验证，不是经过真实金融数据验证的信用评分、授信决策或违约概率模型。

## 2. Canonical specifications

实现时以下文件是 V1 的规范来源：

- Company matching policy: `industry-packs/park/matching/company-match-policy-v1.yaml`
- Indicator set: `industry-packs/park/indicators/enterprise-activity-v1.yaml`
- Human-readable indicator formulas: `examples/enterprise-activity/indicators-v1.md`
- Production workflow: `examples/enterprise-activity/workflow/workflow-v1.yaml`
- Data Contract: `examples/enterprise-activity/contract/data-contract-v1.yaml`
- Park quality rules: `industry-packs/park/quality/`
- Park compliance rules: `industry-packs/park/compliance/`

若代码中的常量、缺失值处理或公式与上述版本化规范冲突，应修改代码而不是静默修改产品语义。

## 3. Input Sources

### `enterprise.csv`

```text
source_company_id
company_name
unified_social_credit_code
legal_representative
registered_address
entry_date
company_status
contact_name
mobile
email
```

### `lease.csv`

```text
lease_id
source_company_id
company_name
contract_start
contract_end
rent_amount
payment_due_date
payment_date
lease_status
```

### `energy.csv`

```text
meter_id
source_company_id
company_name
reading_time
energy_kwh
```

POC fixtures intentionally contain dirty cases such as:

- inconsistent company names
- missing unified social credit codes
- source-system IDs that do not match across files
- inconsistent dates
- invalid/negative energy records
- late or unpaid rent events
- aliases / shortened company names

These records exist to exercise standardization, entity matching, quarantine, Quality and Evidence behavior.

## 4. Company Entity Resolution V1

Canonical output key:

```text
canonical_company_id = COMPANY-xxxxxx
```

Initial policy:

1. Unified social credit code exact match → `AUTO_MATCH`, confidence 1.0.
2. Normalized company name exact + normalized address exact → high-confidence match.
3. Similar company name + same legal representative → `REVIEW` candidate.
4. Otherwise → `UNRESOLVED` until a later engine/manual decision resolves it.

Every non-trivial mapping records source identity, canonical entity, match method, policy version, confidence, review status and evidence. RAW data is never rewritten to force the match.

## 5. Indicator semantics

The V1 product contains four business indicators:

- `tenancy_stability`
- `rent_performance`
- `energy_stability`
- `activity_score`

Exact formulas and parameters are frozen in the V1 indicator spec. Important semantics:

- component scores may be `null` when minimum observations are not satisfied;
- no missing component is silently imputed as zero or a neutral value;
- `activity_score` requires all three component indicators in V1;
- when a required component is unavailable, `activity_score=null` and `activity_level=INSUFFICIENT_DATA`;
- `indicator_coverage` reports available required components / 3 × 100;
- negative energy observations are quarantined, not rewritten as zero.

## 6. Product Dataset

V1 output schema:

```text
company_id
period
tenancy_stability          nullable
rent_performance            nullable
energy_stability            nullable
activity_score              nullable
activity_level              non-null
indicator_coverage          non-null
generated_at                non-null
```

The Product Dataset must not expose unnecessary raw personal/contact or source-level fields such as `contact_name`, `mobile`, `email`, `legal_representative`, `registered_address`, raw `rent_amount` or `meter_id`.

## 7. Quality and Compliance

Quality and business activity score are separate concepts. A Dataset can contain a low-activity company while still having excellent data quality.

The Quality Gate evaluates data fitness (identity completeness, mapping rate, score range, freshness, etc.). The Compliance Gate evaluates permitted output and minimum-necessary-data handling.

A blocking Quality/Compliance decision prevents Product Release readiness; it does not modify historical DatasetVersions in place.

## 8. Data Contract

The reference contract currently defines:

- consumers: licensed banks and guarantee institutions;
- purpose: enterprise operating-status / credit-risk support;
- freshness target: daily, max delay 24h;
- delivery: API allowed, dataset/raw-source export disabled;
- redistribution and marketing use forbidden;
- V1 output schema and missing-data semantics;
- product is not to be used as the sole underwriting decision.

## 9. Production Flow

```text
enterprise / lease / energy RAW DatasetVersions
→ standardization
→ Company Entity Resolution + Human Review
→ STANDARDIZED DatasetVersions
→ energy monthly aggregation
→ V1 indicator calculation
→ CURATED activity DatasetVersion
→ Compliance Gate
→ Quality Gate
→ published Data Contract V1
→ Data Product Version 1.0.0
→ Product Release
→ EvidenceSnapshot + Cost Events
```

OpenMetadata, Apache Hop and Splink are deliberately not required for the Core POC. They are later adapters used to validate the Engine SPI architecture.

## 10. Acceptance Questions

The finished reference implementation must answer:

1. Which RAW records and DatasetVersions produced a company's result?
2. Why were source company records matched to the same canonical Entity?
3. Which entity policy, indicator set and workflow versions were used?
4. Which technical records were quarantined and why?
5. Why did Quality and Compliance pass/fail?
6. Which exact DatasetVersion and Data Contract were frozen in the ProductRelease?
7. What Evidence supports the Release?
8. What Cost Events were recorded?

## 11. POC backlog

The Core POC is tracked under GitHub Epic `#1`. The reference semantics are frozen through the spec-first work in `#12` and related implementation issues.