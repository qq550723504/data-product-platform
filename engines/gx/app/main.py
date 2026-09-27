from __future__ import annotations

import math
import os
import time
from importlib.metadata import version
from typing import Any

import great_expectations as gx
import pandas as pd
import yaml
from fastapi import Depends, FastAPI, Header, HTTPException
from pydantic import BaseModel, Field

ENGINE_NAME = "gx-core"
ENGINE_VERSION = version("great-expectations")
API_TOKEN = os.getenv("GX_ENGINE_API_TOKEN", "").strip()
MAX_ROWS = int(os.getenv("GX_ENGINE_MAX_ROWS", "100000"))
SUPPORTED_RULE_TYPES = {
    "not_null",
    "completeness_ratio",
    "unique",
    "duplicate_ratio",
    "range",
    "enum",
}

app = FastAPI(title="Data Product Platform GX Quality Engine", version="0.1.0")


class EvaluateRequest(BaseModel):
    attemptId: str = Field(min_length=1)
    datasetVersionId: str = Field(min_length=1)
    ruleSetRef: str = Field(min_length=1)
    ruleSetContent: str = Field(min_length=1)
    headers: list[str]
    rows: list[dict[str, str]]


class FindingResult(BaseModel):
    ruleId: str
    status: str
    observed: dict[str, Any] = Field(default_factory=dict)


class EngineInfo(BaseModel):
    name: str
    version: str
    capabilities: list[str]


class ExecutionInfo(BaseModel):
    ref: str
    durationMillis: int


class EvaluateResponse(BaseModel):
    engine: EngineInfo
    findings: list[FindingResult]
    diagnosticsRef: str = ""
    execution: ExecutionInfo


def authorize(authorization: str | None = Header(default=None)) -> None:
    if not API_TOKEN:
        return
    if authorization != f"Bearer {API_TOKEN}":
        raise HTTPException(status_code=401, detail="unauthorized")


@app.get("/health")
def health(_: None = Depends(authorize)) -> dict[str, Any]:
    return {
        "status": "ok",
        "engineName": ENGINE_NAME,
        "engineVersion": ENGINE_VERSION,
        "capabilities": sorted(SUPPORTED_RULE_TYPES),
    }


@app.post("/v1/evaluate", response_model=EvaluateResponse)
def evaluate(request: EvaluateRequest, _: None = Depends(authorize)) -> EvaluateResponse:
    started = time.monotonic()
    if len(request.rows) > MAX_ROWS:
        raise HTTPException(status_code=413, detail="dataset exceeds configured row limit")
    if not request.headers:
        raise HTTPException(status_code=400, detail="headers are required")

    policy = _load_policy(request.ruleSetContent)
    rules = policy.get("spec", {}).get("rules", [])
    unsupported = sorted({
        str(rule.get("type", "")).strip().lower()
        for rule in rules
        if str(rule.get("type", "")).strip().lower() not in SUPPORTED_RULE_TYPES
    })
    if unsupported:
        raise HTTPException(status_code=409, detail="rule set contains unsupported GX rule types")

    dataframe = pd.DataFrame(request.rows, columns=request.headers)
    dataframe = dataframe.replace(r"^\s*$", pd.NA, regex=True)

    context = gx.get_context(mode="ephemeral")
    source = context.data_sources.add_pandas("request")
    asset = source.add_dataframe_asset(name="dataset")
    batch_definition = asset.add_batch_definition_whole_dataframe("whole")
    batch = batch_definition.get_batch(batch_parameters={"dataframe": dataframe})

    findings = [_evaluate_rule(batch, dataframe, rule) for rule in rules]
    elapsed_ms = max(0, int((time.monotonic() - started) * 1000))
    return EvaluateResponse(
        engine=EngineInfo(
            name=ENGINE_NAME,
            version=ENGINE_VERSION,
            capabilities=sorted(SUPPORTED_RULE_TYPES),
        ),
        findings=findings,
        execution=ExecutionInfo(ref=request.attemptId, durationMillis=elapsed_ms),
    )


def _load_policy(content: str) -> dict[str, Any]:
    try:
        policy = yaml.safe_load(content)
    except yaml.YAMLError as exc:
        raise HTTPException(status_code=400, detail="invalid quality rule set") from exc
    if not isinstance(policy, dict) or policy.get("kind") != "QualityRuleSet":
        raise HTTPException(status_code=400, detail="invalid quality rule set")
    rules = policy.get("spec", {}).get("rules")
    if not isinstance(rules, list) or not rules:
        raise HTTPException(status_code=400, detail="quality rule set has no rules")
    return policy


def _evaluate_rule(batch: Any, dataframe: pd.DataFrame, rule: dict[str, Any]) -> FindingResult:
    rule_id = str(rule.get("id", "")).strip()
    rule_type = str(rule.get("type", "")).strip().lower()
    target = str(rule.get("target", "")).strip()
    if not rule_id or rule_type not in SUPPORTED_RULE_TYPES or not target:
        raise HTTPException(status_code=400, detail="invalid supported quality rule")
    if target not in dataframe.columns:
        return FindingResult(
            ruleId=rule_id,
            status="FAIL",
            observed={"affectedCount": len(dataframe), "total": len(dataframe), "observedValue": 0.0},
        )

    expectation, prepared, threshold = _expectation_for_rule(dataframe, rule)
    validation_batch = batch
    if prepared is not dataframe:
        context = gx.get_context(mode="ephemeral")
        source = context.data_sources.add_pandas("request")
        asset = source.add_dataframe_asset(name="dataset")
        batch_definition = asset.add_batch_definition_whole_dataframe("whole")
        validation_batch = batch_definition.get_batch(batch_parameters={"dataframe": prepared})

    try:
        validation = validation_batch.validate(expectation)
        payload = validation.to_json_dict() if hasattr(validation, "to_json_dict") else dict(validation)
    except Exception as exc:
        raise HTTPException(status_code=502, detail="GX validation failed") from exc

    result = payload.get("result") or {}
    success = bool(payload.get("success"))
    total = int(result.get("element_count") or len(prepared))
    unexpected = int(result.get("unexpected_count") or 0)
    missing = int(result.get("missing_count") or 0)

    parameters = rule.get("parameters") or {}
    allow_null = bool(parameters.get("allowNull", True))
    if rule_type in {"range", "enum"} and not allow_null and missing > 0:
        success = False
        unexpected += missing

    observed_value = _observed_ratio(total, unexpected, rule_type)
    observed = {
        "affectedCount": max(0, unexpected),
        "total": max(0, total),
        "observedValue": observed_value,
        "threshold": threshold,
    }
    return FindingResult(ruleId=rule_id, status="PASS" if success else "FAIL", observed=observed)


def _expectation_for_rule(
    dataframe: pd.DataFrame, rule: dict[str, Any]
) -> tuple[Any, pd.DataFrame, float]:
    rule_type = str(rule["type"]).strip().lower()
    target = str(rule["target"]).strip()
    parameters = rule.get("parameters") or {}

    if rule_type in {"not_null", "completeness_ratio"}:
        threshold = _ratio_threshold(rule, 1.0)
        return (
            gx.expectations.ExpectColumnValuesToNotBeNull(column=target, mostly=threshold),
            dataframe,
            threshold,
        )

    if rule_type == "unique":
        threshold = _ratio_threshold(rule, 1.0)
        return (
            gx.expectations.ExpectColumnValuesToBeUnique(column=target, mostly=threshold),
            dataframe,
            threshold,
        )

    if rule_type == "duplicate_ratio":
        max_duplicate_ratio = _ratio_threshold(rule, 0.0)
        mostly = max(0.0, min(1.0, 1.0 - max_duplicate_ratio))
        return (
            gx.expectations.ExpectColumnValuesToBeUnique(column=target, mostly=mostly),
            dataframe,
            max_duplicate_ratio,
        )

    if rule_type == "range":
        minimum = _finite_float(parameters.get("min"))
        maximum = _finite_float(parameters.get("max"))
        prepared = dataframe.copy()
        prepared[target] = pd.to_numeric(prepared[target], errors="coerce")
        return (
            gx.expectations.ExpectColumnValuesToBeBetween(
                column=target,
                min_value=minimum,
                max_value=maximum,
                mostly=1.0,
            ),
            prepared,
            1.0,
        )

    if rule_type == "enum":
        values = parameters.get("values", parameters.get("allowedValues"))
        if not isinstance(values, list) or not values or any(not isinstance(item, str) for item in values):
            raise HTTPException(status_code=400, detail="enum rule values are invalid")
        return (
            gx.expectations.ExpectColumnValuesToBeInSet(
                column=target,
                value_set=values,
                mostly=1.0,
            ),
            dataframe,
            1.0,
        )

    raise HTTPException(status_code=409, detail="unsupported GX rule type")


def _ratio_threshold(rule: dict[str, Any], fallback: float) -> float:
    raw = rule.get("threshold")
    if raw is None:
        raw = (rule.get("parameters") or {}).get("threshold", fallback)
    value = _finite_float(raw)
    if value < 0 or value > 1:
        raise HTTPException(status_code=400, detail="ratio threshold is out of range")
    return value


def _finite_float(value: Any) -> float:
    try:
        number = float(value)
    except (TypeError, ValueError) as exc:
        raise HTTPException(status_code=400, detail="numeric rule parameter is invalid") from exc
    if not math.isfinite(number):
        raise HTTPException(status_code=400, detail="numeric rule parameter is invalid")
    return number


def _observed_ratio(total: int, unexpected: int, rule_type: str) -> float:
    if total <= 0:
        return 1.0
    unexpected_ratio = max(0.0, min(1.0, unexpected / total))
    if rule_type == "duplicate_ratio":
        return unexpected_ratio
    return 1.0 - unexpected_ratio
