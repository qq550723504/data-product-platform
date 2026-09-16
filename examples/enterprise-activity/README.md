# Enterprise Activity Reference Implementation V1.0

## 1. Purpose

Reference Use Case: 企业融资风险辅助。

Reference Data Product: **企业经营活跃度 V1.0**。

目标不是构建银行授信模型，而是用一个可解释场景验证完整 Data Product 生产链。

## 2. Input Sources

### enterprise.csv

Planned fields:

```text
source_company_id
company_name
unified_social_credit_code
legal_representative
registered_address
entry_date
company_status
```

### lease.csv

Planned fields:

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

### energy.csv

Planned fields:

```text
meter_id
source_company_id
company_name
reading_time
energy_kwh
```

POC sample data should intentionally contain dirty cases such as:

- inconsistent company names
- missing unified social credit codes
- source-system IDs that do not match across files
- duplicate rows
- inconsistent dates
- negative/invalid energy records
- late rent payments
- company aliases or renamed companies

## 3. Company Entity Resolution V1

Canonical output key:

```text
canonical_company_id = COMPANY-xxxxxx
```

Initial rules:

1. Unified social credit code exact match → AUTO_MATCH, confidence 1.0.
2. Normalized company name exact + normalized address exact → high-confidence match.
3. Similar company name + same legal representative → REVIEW candidate.
4. Otherwise → UNRESOLVED.

Every non-trivial mapping records:

- source system / source key
- canonical company
- match method
- policy version
- confidence
- review status
- reviewer / reason where applicable
- evidence reference

## 4. Initial Indicators

The first version remains explainable and rule-based.

### tenancy_stability

Represents enterprise tenancy/occupancy continuity using available entry and contract history.

### rent_performance

Represents lease payment performance based on expected vs on-time payment events.

### energy_stability

Represents stability/continuity of recent monthly energy usage. Technical data errors must be distinguished from meaningful business changes where possible.

### activity_score

Composite score built from the three indicators above. POC weights are configuration parameters and must not be presented as a validated financial risk model.

## 5. Product Dataset

Planned output schema:

```text
company_id
period
tenancy_stability
rent_performance
energy_stability
activity_score
activity_level
generated_at
```

Raw personal contact information must not appear in the Product Dataset.

## 6. Initial Quality Rules

Candidate rules:

- `company_id` completeness >= 99.9%
- canonical company unique mapping rate >= 99%
- unresolved entity rate <= 0.5%
- `energy_kwh >= 0` for accepted records
- `activity_score` in [0, 100]
- period must be present
- Product Dataset freshness <= 24h for the reference contract

Thresholds are POC targets and must be calibrated with real data before production claims are made.

## 7. Initial Compliance Rules

Reference Product Dataset follows minimum-necessary-data principle.

Example handling:

```text
contact_name → REMOVE
mobile       → REMOVE
email        → REMOVE
company_id   → KEEP
activity indicators → KEEP
```

## 8. Data Contract Draft

Consumer types:

- licensed bank
- guarantee institution

Purpose:

- enterprise credit-risk support / operating-status reference

Delivery:

- API allowed
- raw source export forbidden

Usage:

- redistribution forbidden
- marketing use forbidden

Freshness:

- target T+1 / max delay 24h for the POC

## 9. Production Flow

```text
enterprise / lease / energy RAW datasets
→ standardization
→ Company Entity Resolution
→ standardized datasets
→ aggregation / join
→ indicator calculation
→ CURATED activity dataset
→ Compliance Gate
→ Quality Gate
→ Data Contract
→ Data Product Version 1.0
→ Product Release
→ EvidenceSnapshot + Cost Events
```

## 10. Acceptance Questions

The finished reference implementation must answer:

1. Which raw records produced a company's result?
2. Why were source company records matched together?
3. Which entity policy and workflow version were used?
4. Why did Quality and Compliance pass/fail?
5. Which exact DatasetVersion was released?
6. What Evidence supports the Release?
7. What production costs were recorded?
