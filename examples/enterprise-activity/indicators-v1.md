# Enterprise Activity Indicator Specification V1.0

This document is the human-readable companion to:

`industry-packs/park/indicators/enterprise-activity-v1.yaml`

The V1 indicators are deliberately transparent and rule-based so the POC can validate versioning, explainability, Quality/Compliance gates, evidence and release reproducibility.

> **Important:** these scores are POC product indicators. They are not a validated bank credit score, underwriting decision, probability-of-default model, or investment/risk recommendation. Real financial use requires domain-owner definition, real-data calibration, validation and consumer agreement.

## 1. Target period

The reference implementation calculates one output row per canonical company and target month (`YYYY-MM`). Dates are evaluated using the target period end unless a rule says otherwise.

All scores are rounded to 2 decimal places using HALF_UP after the unrounded indicator has been calculated.

## 2. `tenancy_stability`

Purpose: provide an explainable signal for how long the enterprise has been present and whether it has an active lease covering the target month.

Inputs:

- `enterprise.entry_date`
- `lease.contract_start`
- `lease.contract_end`
- `lease.lease_status`

V1 parameters:

- full-tenure reference: 36 months
- tenure weight: 70%
- current active lease weight: 30%

Calculation:

```text
tenure_months = full calendar months from entry_date to target-period end
                clamped to 0..36

tenure_score = min(100, tenure_months / 36 * 100)

current_lease_score = 100
  if at least one ACTIVE lease overlaps the target period
  else 0

tenancy_stability = 0.70 * tenure_score
                  + 0.30 * current_lease_score
```

Missing-data rule:

- missing `entry_date` → indicator is `null`
- no usable lease data → indicator is `null`

The implementation must retain the components used for explanation: `tenure_months`, `tenure_score`, whether an active lease was found, and `current_lease_score`.

## 3. `rent_performance`

Purpose: summarize payment-event performance without hiding late or unpaid events.

Inputs:

- `lease.payment_due_date`
- `lease.payment_date`

V1 window:

- trailing 12 months ending at the target-period end
- minimum 2 due events required

Event score:

| Event | Score |
| --- | ---: |
| paid on/before due date | 100 |
| 1–7 days late | 70 |
| 8–30 days late | 40 |
| more than 30 days late | 20 |
| overdue and unpaid as of target period end | 0 |

```text
rent_performance = arithmetic mean(event_scores)
```

A future-due unpaid event is not included until its due date has passed.

If fewer than 2 due events are available, the result is `null` (`INSUFFICIENT_DATA` for this component).

The explanation should retain due-event counts by classification.

## 4. `energy_stability`

Purpose: measure continuity and month-to-month stability of valid energy observations. It intentionally does not claim that rising/falling energy is good or bad for credit risk.

Inputs:

- `energy.reading_time`
- `energy.energy_kwh`

Preprocessing:

1. `energy_kwh < 0` is technically invalid for the POC and is **quarantined**, never rewritten to zero.
2. Accepted readings are summed to monthly totals per canonical company.
3. Use up to the trailing 6 months ending at the target month.

Minimum data:

- at least 3 valid monthly totals
- arithmetic mean of valid monthly totals must be greater than zero

Expected months are counted from the first observed month inside the lookback window through the target month, capped at 6. This avoids penalizing a company merely because the POC dataset starts later than the six-month window.

```text
coverage_score = valid_month_count / expected_month_count * 100

cv = population_stddev(valid_monthly_totals)
     / mean(valid_monthly_totals)

variability_score = clamp(100 * (1 - cv / 0.30), 0, 100)

energy_stability = 0.70 * variability_score
                 + 0.30 * coverage_score
```

`0.30` is a POC reference CV threshold and is deliberately stored in versioned configuration. It must be calibrated before production claims are made.

Explanation output should retain:

- expected/valid months
- coverage score
- monthly mean/stddev
- coefficient of variation
- variability score
- quarantined record count

## 5. `activity_score`

V1 requires all three component indicators:

- `tenancy_stability`
- `rent_performance`
- `energy_stability`

The initial composite uses equal weights to avoid implying an empirically validated risk weighting:

```text
activity_score = 1/3 * tenancy_stability
               + 1/3 * rent_performance
               + 1/3 * energy_stability
```

If any component is `null`:

```text
activity_score = null
activity_level = INSUFFICIENT_DATA
```

There is **no silent zero, neutral-score, median, or forward-fill imputation**.

POC activity levels:

```text
HIGH   >= 80
MEDIUM >= 60 and < 80
LOW    >= 0 and < 60
INSUFFICIENT_DATA when activity_score is null
```

These level thresholds are POC presentation semantics, not validated financial thresholds.

## 6. `indicator_coverage`

The product exposes a simple explainability/availability measure:

```text
indicator_coverage = available_required_components / 3 * 100
```

Examples:

- 3/3 available → 100
- 2/3 available → 66.67
- 1/3 available → 33.33
- 0/3 available → 0

A coverage below 100 means `activity_score` is null in V1.

## 7. Quality vs business indicator semantics

`activity_score` describes the reference product's business indicator output.

It must **not** be reused as a Data Quality score. Data fitness is independently evaluated by Quality Rules / Quality Gate.

A company can have a low `activity_score` in a perfectly high-quality Dataset, and a high calculated activity score can still be blocked from release if Data Quality fails.

## 8. Versioning and evidence

Every execution producing these indicators must retain:

- indicator set name/version
- exact input DatasetVersions
- workflow version
- entity matching policy version
- quarantined/invalid record counts
- component explanation values

A published ProductRelease freezes these references in its evidence snapshot.