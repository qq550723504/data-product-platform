# Entity Resolution Reference Evaluation

This directory is the reproducible labelled reference set for Sprint 6.3 COMPANY entity resolution.

- `references.csv` contains canonical reference companies.
- `sources.csv` contains both `train` and `eval` source records.
- `training_labels.csv` contains positive source/reference pairs used to estimate Splink `m` probabilities.
- `evaluation_labels.csv` contains positive and negative ground truth used only for evaluation.

The data is synthetic and contains no real company or personal data.

The evaluator trains a Splink 4.0.17 reference model with a fixed seed and compares two modes: Core rules only, and Core rules plus Splink. Strong deterministic AUTO_MATCH rules remain authoritative; deterministic REVIEW rules are retained as a fallback when the probabilistic candidate path is unavailable or does not reach `reviewMinimum`.

Run from the repository root:

```bash
python -m pip install ./engines/splink
python engines/splink/scripts/evaluate_reference.py \
  --assert-quality \
  --model-out /tmp/park-company-v1-model.json \
  --report /tmp/entity-resolution-evaluation.json
```

The report records `autoMatched`, `review`, `unresolved`, `conflicts`, `falsePositive`, and `falseNegative`. A correct Human Review candidate is not counted as a false negative because Core intentionally requires human confirmation.

Reference thresholds validate the Park POC only. Production thresholds must be calibrated against customer-labelled data before enabling automatic decisions.
