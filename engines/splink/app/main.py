from __future__ import annotations

import json
import os
from importlib.metadata import version
from pathlib import Path
from typing import Any

import pandas as pd
from fastapi import Depends, FastAPI, Header, HTTPException
from pydantic import BaseModel, Field
from splink import DuckDBAPI, Linker

ENGINE_NAME = "SPLINK"
ENGINE_VERSION = version("splink")
MODEL_REF = os.getenv("SPLINK_MODEL_REF", "park-company-v1")
MODEL_VERSION = os.getenv("SPLINK_MODEL_VERSION", "1.0.0")
MODEL_PATH = Path(os.getenv("SPLINK_MODEL_PATH", "./models/park-company-v1/model.json"))
API_TOKEN = os.getenv("SPLINK_API_TOKEN", "").strip()
MAX_REFERENCES = int(os.getenv("SPLINK_MAX_REFERENCES", "100000"))

app = FastAPI(title="Data Product Platform Splink Engine", version="0.1.0")


class MatchRecord(BaseModel):
    id: str = Field(min_length=1)
    name: str = ""
    fields: dict[str, str] = Field(default_factory=dict)


class ReferenceRecord(BaseModel):
    entityId: str = Field(min_length=1)
    name: str = ""
    fields: dict[str, str] = Field(default_factory=dict)


class CandidateRequest(BaseModel):
    entityType: str
    source: MatchRecord
    references: list[ReferenceRecord]
    policyRef: str = ""
    policyVersion: str = ""
    modelRef: str
    modelVersion: str


class Candidate(BaseModel):
    entityId: str
    score: float
    method: str = "FELLEGI_SUNTER"
    metadata: dict[str, Any] = Field(default_factory=dict)


class EngineInfo(BaseModel):
    name: str
    version: str
    modelVersion: str


class CandidateResponse(BaseModel):
    engine: EngineInfo
    candidates: list[Candidate]


def authorize(authorization: str | None = Header(default=None)) -> None:
    if not API_TOKEN:
        return
    if authorization != f"Bearer {API_TOKEN}":
        raise HTTPException(status_code=401, detail="unauthorized")


def _validate_model_contract() -> None:
    if not MODEL_PATH.exists():
        raise RuntimeError(f"Splink model not found: {MODEL_PATH}")
    try:
        model = json.loads(MODEL_PATH.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise RuntimeError(f"Splink model cannot be loaded: {MODEL_PATH}") from exc
    if model.get("link_type") != "link_only":
        raise RuntimeError("Splink service model must use link_type=link_only")


@app.on_event("startup")
def startup() -> None:
    _validate_model_contract()


@app.get("/health")
def health(_: None = Depends(authorize)) -> dict[str, str]:
    return {
        "status": "ok",
        "engineName": ENGINE_NAME,
        "engineVersion": ENGINE_VERSION,
        "modelRef": MODEL_REF,
        "modelVersion": MODEL_VERSION,
    }


@app.post("/v1/candidates", response_model=CandidateResponse)
def candidates(request: CandidateRequest, _: None = Depends(authorize)) -> CandidateResponse:
    if request.entityType.upper() != "COMPANY":
        raise HTTPException(status_code=400, detail="unsupported entity type")
    if request.modelRef != MODEL_REF or request.modelVersion != MODEL_VERSION:
        raise HTTPException(status_code=409, detail="model binding mismatch")
    if len(request.references) > MAX_REFERENCES:
        raise HTTPException(status_code=413, detail="reference set exceeds configured limit")
    if not request.references:
        return _response([])

    source_df = pd.DataFrame([_source_row(request.source)])
    reference_df = pd.DataFrame([_reference_row(record) for record in request.references])

    # The saved model owns statistical parameters, blocking rules, and comparisons.
    # This service only supplies the request-scoped source/reference records.
    linker = Linker(
        [source_df, reference_df],
        settings=str(MODEL_PATH),
        db_api=DuckDBAPI(),
        input_table_aliases=["source", "reference"],
    )
    predictions = linker.inference.predict(threshold_match_probability=0.0)
    rows = predictions.as_record_dict()

    output: list[Candidate] = []
    for row in rows:
        entity_id = _reference_id(row, request.source.id)
        if not entity_id:
            continue
        probability = float(row.get("match_probability") or 0.0)
        output.append(
            Candidate(
                entityId=entity_id,
                score=max(0.0, min(1.0, probability)),
                metadata={
                    "matchWeight": row.get("match_weight"),
                    "matchKey": row.get("match_key"),
                },
            )
        )
    output.sort(key=lambda candidate: (-candidate.score, candidate.entityId))
    return _response(output)


def _source_row(record: MatchRecord) -> dict[str, str]:
    result = {"unique_id": record.id}
    result.update(record.fields)
    result.setdefault("company_name", record.name)
    return result


def _reference_row(record: ReferenceRecord) -> dict[str, str]:
    result = {"unique_id": record.entityId}
    result.update(record.fields)
    result.setdefault("company_name", record.name)
    return result


def _reference_id(prediction: dict[str, Any], source_id: str) -> str:
    left = str(prediction.get("unique_id_l", ""))
    right = str(prediction.get("unique_id_r", ""))
    left_source = str(prediction.get("source_dataset_l", ""))
    right_source = str(prediction.get("source_dataset_r", ""))

    if left_source == "reference":
        return left
    if right_source == "reference":
        return right
    if left == source_id:
        return right
    if right == source_id:
        return left
    return ""


def _response(items: list[Candidate]) -> CandidateResponse:
    return CandidateResponse(
        engine=EngineInfo(
            name=ENGINE_NAME,
            version=ENGINE_VERSION,
            modelVersion=MODEL_VERSION,
        ),
        candidates=items,
    )
