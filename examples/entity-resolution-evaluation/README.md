# 实体解析参考评估

本目录是 Sprint 6.3 COMPANY 实体解析的可复现标注参考集。

- `references.csv` 包含权威参照企业。
- `sources.csv` 同时包含 `train` 与 `eval` 源记录。
- `training_labels.csv` 包含用于估计 Splink `m` 概率的正样本"源/参照"配对。
- `evaluation_labels.csv` 包含仅用于评估的正负样本真值（ground truth）。

这些数据是合成的，不包含任何真实企业或个人数据。

评估器以固定随机种子训练一个 Splink 4.0.17 参考模型，并比较两种模式：仅使用 Core 规则，以及 Core 规则 + Splink。强的确定性 AUTO_MATCH 规则仍然具有权威性；当概率化候选路径不可用或未达到 `reviewMinimum` 时，确定性 REVIEW 规则会作为回退保留。

在仓库根目录运行：

```bash
python -m pip install ./engines/splink
python engines/splink/scripts/evaluate_reference.py \
  --assert-quality \
  --model-out /tmp/park-company-v1-model.json \
  --report /tmp/entity-resolution-evaluation.json
```

报告会记录 `autoMatched`、`review`、`unresolved`、`conflicts`、`falsePositive` 与 `falseNegative`。正确的"人工复核候选"不会被计为 false negative，因为 Core 有意要求人工确认。

参考阈值仅用于验证 Park POC。在启用自动决策之前，生产阈值必须基于客户标注数据进行校准。
