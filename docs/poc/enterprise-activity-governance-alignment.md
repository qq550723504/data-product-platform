# Enterprise Activity Governance Alignment

This note records the required alignment between the native Enterprise Activity processing output and the frozen V1 Data Contract / Quality / Compliance policies.

## Required product schema

The CURATED Product Dataset emitted by the native worker must use the Data Contract field names:

- `company_id`
- `company_name`
- `period`
- `tenancy_stability`
- `rent_performance`
- `energy_stability`
- `activity_score`
- `activity_level`
- `indicator_coverage`
- `generated_at`

Legacy processing aliases (`canonical_company_id`, `target_period`) may be accepted temporarily by readers but must not be the canonical Product Dataset schema.

## Negative energy quarantine

A RAW energy record with `energy_kwh < 0` must:

1. be persisted to `execution_quarantine_record` with reason `NEGATIVE_ENERGY_KWH`;
2. be excluded from accepted energy readings used by indicators;
3. contribute to `quarantineCount`;
4. not increase the accepted-negative-energy rate.

The output DatasetVersion processing metadata must include:

- `unresolvedEntityRate`
- `acceptedNegativeEnergyRate`
- `quarantineCount`
- exact WorkflowVersion
- exact Indicator Set version
- exact Entity Matching Policy version

For a successfully completed native reference workflow where all source company records are resolved and negative energy is quarantined before indicator calculation:

```text
unresolvedEntityRate = 0
acceptedNegativeEnergyRate = 0
```

## Generated timestamp

`generated_at` is product metadata for the release candidate. It may vary between executions; the frozen DatasetVersion checksum makes the concrete generated artifact reproducible and auditable.

## Gate consequence

The real Quality and Compliance gates introduced in Sprint 3.2 must be able to run against the worker-produced CURATED DatasetVersion without compatibility mocks. This alignment must be completed before ReleaseReadiness can transition a release to `READY`.
