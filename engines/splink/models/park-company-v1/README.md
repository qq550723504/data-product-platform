# Park COMPANY Splink model artifact

The runtime expects a **trained Splink 4 model** at:

```text
models/park-company-v1/model.json
```

The model file is a generated statistical artifact, not Core domain state. It must use:

- `link_type = link_only`
- Splink `4.0.17`
- model ref `park-company-v1`
- model version `1.0.0`

The model is intentionally not hand-authored in Sprint 6.2. Sprint 6.3 owns the committed labelled evaluation set, training/evaluation procedure, and the decision about whether a trained reference artifact should be checked into the repository or produced by CI/release tooling.

The service fails fast when the configured model file is absent or is not a `link_only` model. This prevents silently falling back to an untrained or incompatible statistical model.
