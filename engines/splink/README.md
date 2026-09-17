# Splink Candidate Engine

This directory contains the optional Python runtime used by the Core Platform to generate probabilistic COMPANY match candidates.

## Boundary

Splink does **not** own canonical entities or mappings. The service accepts one normalized source record plus the current canonical reference set and returns ranked candidate entity IDs with match probabilities.

Core remains responsible for:

- strong-key precedence
- matching policy thresholds
- AUTO_MATCH / REVIEW / UNRESOLVED decisions
- Human Review
- EntityMapping persistence
- Evidence and audit history

## Runtime version

The reference runtime pins Splink `4.0.17` and uses the DuckDB backend. The Go adapter performs a `/health` capability/version check before enabling the candidate generator.

Splink 5 development releases are intentionally not used by this reference implementation.

## Endpoints

```text
GET  /health
POST /v1/candidates
```

`POST /v1/candidates` receives a provider-neutral request from the Go Core and returns:

```json
{
  "engine": {
    "name": "SPLINK",
    "version": "4.0.17",
    "modelVersion": "1.0.0"
  },
  "candidates": [
    {
      "entityId": "...",
      "score": 0.87,
      "method": "FELLEGI_SUNTER",
      "metadata": {
        "matchWeight": 2.73,
        "matchKey": "0"
      }
    }
  ]
}
```

Provider exceptions and Python tracebacks stay inside the runtime logs; the Go adapter maps transport failures to stable platform errors.

## Configuration

```text
SPLINK_MODEL_PATH=./models/park-company-v1/model.json
SPLINK_MODEL_REF=park-company-v1
SPLINK_MODEL_VERSION=1.0.0
SPLINK_API_TOKEN=
SPLINK_MAX_REFERENCES=100000
```

The model must be a trained Splink `link_only` JSON artifact. See `models/park-company-v1/README.md` and `industry-packs/park/matching/splink-company-model-v1.yaml`.

## Run

```bash
pip install .
uvicorn app.main:app --host 0.0.0.0 --port 8091
```

or build the supplied Dockerfile.
