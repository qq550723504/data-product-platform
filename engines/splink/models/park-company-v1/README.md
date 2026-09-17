# Park COMPANY Splink 模型产物

运行时期望在以下位置存在一个**已训练的 Splink 4 模型**：

```text
models/park-company-v1/model.json
```

模型文件是生成的统计产物，不是核心领域状态。它必须使用：

- `link_type = link_only`
- Splink `4.0.17`
- 模型 ref `park-company-v1`
- 模型版本 `1.0.0`

在 Sprint 6.2 中，该模型刻意不由人工手写。Sprint 6.3 负责已提交的标注评估集、训练/评估流程，以及关于"训练得到的参考产物应签入仓库还是由 CI/发布工具生成"的决策。

当配置的模型文件缺失或不是 `link_only` 模型时，服务会快速失败。这可防止静默回退到未训练或不兼容的统计模型。
