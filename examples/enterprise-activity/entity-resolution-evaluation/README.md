# Enterprise Activity Entity Resolution Evaluation

This fixture is the reproducible Sprint 6.3 reference set for COMPANY entity resolution.

## Files

- `canonical-companies.csv` — canonical Core COMPANY references.
- `query-companies.csv` — source records that exercise strong-key, deterministic fuzzy, probabilistic, and negative cases.
- `labels.csv` — clerical ground truth. `MATCH` rows name the expected canonical entity; `NO_MATCH` rows must not be auto-matched.

The fixture intentionally contains both positive and negative examples. It is synthetic and contains no real personal or enterprise data.

## Reproduce

From the repository root, with the optional Splink runtime dependencies installed:

```bash
python -m pip install ./engines/splink
python engines/splink/scripts/evaluate_reference.py \
  --assert-quality \
  --model-out /tmp/park-company-v1-model.json \
  --report /tmp/entity-resolution-evaluation.json
```

The evaluator reads the versioned Park matching policy and Splink model specification, trains a deterministic reference model with a fixed random seed, then compares:

1. `RULE_ONLY` — Core strong-key and deterministic matching only.
2. `RULE_PLUS_SPLINK` — the same Core precedence followed by Splink candidate generation and the policy's `autoMatchMinimum` / `reviewMinimum` thresholds.

## Metrics

- `autoMatched`: Core accepted a mapping without Human Review.
- `review`: a candidate requires Human Review.
- `unresolved`: no candidate reached the review threshold.
- `conflicts`: an AUTO_MATCH or REVIEW candidate points to a different entity than the label.
- `falsePositive`: an automatic match is wrong or matches a `NO_MATCH` label.
- `falseNegative`: a labelled positive remains unresolved. A correct REVIEW candidate is not counted as a false negative because Core deliberately requires human confirmation.

CI fails if rule+Splink introduces additional automatic false positives, increases false negatives, fails to exercise the Splink path, or bypasses exact-USCC precedence.

The reference thresholds are test evidence for the Park POC only; customer-specific production thresholds must be calibrated on customer-labelled data.
