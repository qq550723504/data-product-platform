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
        total = len(dataframe)
        return FindingResult(
            ruleId=rule_id,
            status="FAIL",
            observed={"affectedCount": total, "total": total, "observedValue": 0.0, "threshold": 1.0},
        )

    if rule_type in {"unique", "duplicate_ratio"}:
        return _evaluate_uniqueness_rule(dataframe, rule)

    expectation, prepared, threshold, local_invalid = _expectation_for_rule(dataframe, rule)
    validation_batch = _batch_for_dataframe(prepared)
    try:
        validation = validation_batch.validate(expectation)
        payload = validation.to_json_dict() if hasattr(validation, "to_json_dict") else dict(validation)
    except Exception as exc:
        raise HTTPException(status_code=502, detail="GX validation failed") from exc

    result = payload.get("result") or {}
    total = len(dataframe)
    unexpected = int(result.get("unexpected_count") or 0)
    affected = max(0, unexpected + local_invalid)

    success = affected == 0
    if rule_type in {"not_null", "completeness_ratio"}:
        observed_value = 1.0 if total == 0 else max(0.0, min(1.0, 1.0 - affected / total))
        success = observed_value >= threshold
    else:
        observed_value = 1.0 if total == 0 else max(0.0, min(1.0, 1.0 - affected / total))

    observed = {
        "affectedCount": affected,
        "total": total,
        "observedValue": observed_value,
        "threshold": threshold,
    }
    return FindingResult(ruleId=rule_id, status="PASS" if success else "FAIL", observed=observed)


def _evaluate_uniqueness_rule(dataframe: pd.DataFrame, rule: dict[str, Any]) -> FindingResult:
    rule_id = str(rule["id"]).strip()
    rule_type = str(rule["type"]).strip().lower()
    target = str(rule["target"]).strip()
    threshold = _ratio_threshold(rule, 1.0 if rule_type == "unique" else 0.0)

    prepared = dataframe.copy()
    normalized = prepared[target].astype("string").str.strip()
    normalized = normalized.mask(normalized == "", pd.NA)
    prepared[target] = normalized
    non_null = int(normalized.notna().sum())

    if non_null == 0:
        status = "FAIL" if bool(rule.get("required")) else "SKIPPED"
        return FindingResult(
            ruleId=rule_id,
            status=status,
            observed={"affectedCount": 0, "total": 0, "observedValue": 0.0, "threshold": threshold},
        )

    batch = _batch_for_dataframe(prepared)
    expectation = gx.expectations.ExpectColumnUniqueValueCountToBeBetween(
        column=target,
        min_value=0,
        max_value=non_null,
    )
    try:
        validation = batch.validate(expectation)
        payload = validation.to_json_dict() if hasattr(validation, "to_json_dict") else dict(validation)
    except Exception as exc:
        raise HTTPException(status_code=502, detail="GX validation failed") from exc

    result = payload.get("result") or {}
    unique_count = result.get("observed_value")
    try:
        unique_count = int(unique_count)
    except (TypeError, ValueError) as exc:
        raise HTTPException(status_code=502, detail="GX validation returned invalid unique count") from exc
    if unique_count < 0 or unique_count > non_null:
        raise HTTPException(status_code=502, detail="GX validation returned invalid unique count")

    duplicate_count = non_null - unique_count
    unique_ratio = unique_count / non_null
    duplicate_ratio = duplicate_count / non_null
    if rule_type == "unique":
        observed_value = unique_ratio
        success = unique_ratio >= threshold
    else:
        observed_value = duplicate_ratio
        success = duplicate_ratio <= threshold

    return FindingResult(
        ruleId=rule_id,
        status="PASS" if success else "FAIL",
        observed={
            "affectedCount": duplicate_count,
            "total": non_null,
            "observedValue": observed_value,
            "threshold": threshold,
        },
    )


def _batch_for_dataframe(dataframe: pd.DataFrame) -> Any:
    context = gx.get_context(mode="ephemeral")
    source = context.data_sources.add_pandas("request")
    asset = source.add_dataframe_asset(name="dataset")
    batch_definition = asset.add_batch_definition_whole_dataframe("whole")
    return batch_definition.get_batch(batch_parameters={"dataframe": dataframe})


def _expectation_for_rule(
    dataframe: pd.DataFrame, rule: dict[str, Any]
) -> tuple[Any, pd.DataFrame, float, int]:
    rule_type = str(rule["type"]).strip().lower()
    target = str(rule["target"]).strip()
    parameters = rule.get("parameters") or {}

    if rule_type in {"not_null", "completeness_ratio"}:
        threshold = _ratio_threshold(rule, 1.0)
        prepared = dataframe.copy()
        normalized = prepared[target].astype("string").str.strip()
        normalized = normalized.mask(normalized == "", pd.NA)
        prepared[target] = normalized
        return (
            gx.expectations.ExpectColumnValuesToNotBeNull(column=target, mostly=threshold),
            prepared,
            threshold,
            0,
        )

    if rule_type == "range":
        minimum = _finite_float(parameters.get("min"))
        maximum = _finite_float(parameters.get("max"))
        allow_null = bool(parameters.get("allowNull", True))
        prepared = dataframe.copy()
        normalized = prepared[target].astype("string").str.strip()
        blank_mask = normalized == ""
        normalized = normalized.mask(blank_mask, pd.NA)
        numeric = pd.to_numeric(normalized, errors="coerce")
        invalid_numeric = int((normalized.notna() & numeric.isna()).sum())
        missing = int(blank_mask.sum())
        prepared[target] = numeric
        local_invalid = invalid_numeric + (0 if allow_null else missing)
        return (
            gx.expectations.ExpectColumnValuesToBeBetween(
                column=target,
                min_value=minimum,
                max_value=maximum,
                mostly=1.0,
            ),
            prepared,
            1.0,
            local_invalid,
        )

    if rule_type == "enum":
        values = parameters.get("values", parameters.get("allowedValues"))
        if not isinstance(values, list) or not values or any(not isinstance(item, str) for item in values):
            raise HTTPException(status_code=400, detail="enum rule values are invalid")
        allow_null = bool(parameters.get("allowNull", True))
        prepared = dataframe.copy()
        exact = prepared[target].astype("string")
        missing_mask = exact == ""
        prepared[target] = exact.mask(missing_mask, pd.NA)
        local_invalid = 0 if allow_null else int(missing_mask.sum())
        return (
            gx.expectations.ExpectColumnValuesToBeInSet(
                column=target,
                value_set=values,
                mostly=1.0,
            ),
            prepared,
            1.0,
            local_invalid,
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
