import app.main as runtime
from fastapi.testclient import TestClient

from app.main import ENGINE_NAME, ENGINE_VERSION, app

client = TestClient(app)


def _policy() -> str:
    return """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: gx-contract
  version: 1.0.0
spec:
  rules:
    - id: NOT_NULL
      type: not_null
      target: id
      threshold: 1
      required: true
    - id: COMPLETE
      type: completeness_ratio
      target: id
      threshold: 0.5
      required: true
    - id: UNIQUE
      type: unique
      target: id
      threshold: 0.6
      required: true
    - id: DUPLICATE_RATIO
      type: duplicate_ratio
      target: id
      threshold: 0.4
      required: true
    - id: RANGE
      type: range
      target: score
      parameters:
        min: 0
        max: 100
        allowNull: false
      required: true
    - id: ENUM
      type: enum
      target: level
      parameters:
        values: [HIGH, LOW]
        allowNull: false
      required: true
"""


def test_health_reports_pinned_gx_capabilities() -> None:
    response = client.get("/health")
    assert response.status_code == 200
    body = response.json()
    assert body["engineName"] == ENGINE_NAME
    assert body["engineVersion"] == ENGINE_VERSION
    assert set(body["capabilities"]) == {
        "not_null",
        "completeness_ratio",
        "unique",
        "duplicate_ratio",
        "range",
        "enum",
    }


def test_evaluate_returns_bounded_rule_observations() -> None:
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/gx-contract.yaml",
            "ruleSetContent": _policy(),
            "headers": ["id", "score", "level"],
            "rows": [
                {"id": "A", "score": "10", "level": "HIGH"},
                {"id": " A ", "score": "not-a-number", "level": " HIGH "},
                {"id": "B", "score": "50", "level": "LOW"},
            ],
        },
    )
    assert response.status_code == 200, response.text
    body = response.json()
    assert body["engine"]["name"] == ENGINE_NAME
    assert body["execution"]["ref"] == "11111111-1111-4111-8111-111111111111"
    findings = {item["ruleId"]: item for item in body["findings"]}
    assert set(findings) == {"NOT_NULL", "COMPLETE", "UNIQUE", "DUPLICATE_RATIO", "RANGE", "ENUM"}
    assert findings["NOT_NULL"]["status"] == "PASS"
    assert findings["COMPLETE"]["status"] == "PASS"
    # Native trims identity values before uniqueness, so A and " A " are one distinct value.
    assert findings["UNIQUE"]["status"] == "PASS"
    assert findings["DUPLICATE_RATIO"]["status"] == "PASS"
    # Nonnumeric input is not an allowed null even when range.allowNull=true.
    assert findings["RANGE"]["status"] == "FAIL"
    # Enum uses the exact raw categorical value; surrounding spaces are data, not normalization.
    assert findings["ENUM"]["status"] == "FAIL"
    for item in findings.values():
        assert set(item["observed"]).issubset({"affectedCount", "total", "observedValue", "threshold"})


def test_unsupported_rule_fails_before_validation() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: unsupported
  version: 1
spec:
  rules:
    - id: REGEX
      type: regex
      target: id
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/unsupported.yaml",
            "ruleSetContent": policy,
            "headers": ["id"],
            "rows": [{"id": "A"}],
        },
    )
    assert response.status_code == 409


def test_health_remains_public_when_evaluation_token_is_enabled() -> None:
    original = runtime.API_TOKEN
    runtime.API_TOKEN = "secret-token"
    try:
        assert client.get("/health").status_code == 200
        assert client.get("/ready").status_code == 401
        response = client.post(
            "/v1/evaluate",
            json={
                "attemptId": "11111111-1111-4111-8111-111111111111",
                "datasetVersionId": "22222222-2222-4222-8222-222222222222",
                "ruleSetRef": "quality/gx-contract.yaml",
                "ruleSetContent": _policy(),
                "headers": ["id", "score", "level"],
                "rows": [{"id": "A", "score": "10", "level": "HIGH"}],
            },
        )
        assert response.status_code == 401
    finally:
        runtime.API_TOKEN = original


def test_required_rule_fails_on_empty_dataset_and_optional_missing_target_skips() -> None:
    required_policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: empty-required
  version: 1
spec:
  rules:
    - id: REQUIRED
      type: not_null
      target: id
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/empty-required.yaml",
            "ruleSetContent": required_policy,
            "headers": ["id"],
            "rows": [],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["status"] == "FAIL"

    optional_policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: missing-optional
  version: 1
spec:
  rules:
    - id: OPTIONAL
      type: enum
      target: missing_column
      parameters:
        values: [A, B]
      required: false
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/missing-optional.yaml",
            "ruleSetContent": optional_policy,
            "headers": ["id"],
            "rows": [{"id": "A"}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["status"] == "SKIPPED"


def test_range_preserves_exact_decimal_boundary() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: exact-range
  version: 1
spec:
  rules:
    - id: EXACT
      type: range
      target: score
      parameters:
        min: 0
        max: 0.1
        allowNull: true
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/exact-range.yaml",
            "ruleSetContent": policy,
            "headers": ["score"],
            "rows": [{"score": "0.10000000000000001"}],
        },
    )
    assert response.status_code == 200, response.text
    finding = response.json()["findings"][0]
    assert finding["status"] == "FAIL"
    assert finding["observed"]["affectedCount"] == 1


def test_ratio_threshold_preserves_exact_decimal_semantics() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: exact-ratio
  version: 1
spec:
  rules:
    - id: COMPLETE
      type: completeness_ratio
      target: id
      threshold: 0.50000000000000001
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/exact-ratio.yaml",
            "ruleSetContent": policy,
            "headers": ["id"],
            "rows": [{"id": "A"}, {"id": ""}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["status"] == "FAIL"


def test_enum_policy_values_are_trimmed_but_cells_remain_exact() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: enum-trim
  version: 1
spec:
  rules:
    - id: ENUM
      type: enum
      target: level
      parameters:
        values: [" HIGH "]
        allowNull: false
      required: true
"""
    exact = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/enum-trim.yaml",
            "ruleSetContent": policy,
            "headers": ["level"],
            "rows": [{"level": "HIGH"}],
        },
    )
    assert exact.status_code == 200, exact.text
    assert exact.json()["findings"][0]["status"] == "PASS"

    padded = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111112",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/enum-trim.yaml",
            "ruleSetContent": policy,
            "headers": ["level"],
            "rows": [{"level": " HIGH "}],
        },
    )
    assert padded.status_code == 200, padded.text
    assert padded.json()["findings"][0]["status"] == "FAIL"


def test_range_rejects_non_core_numeric_spelling() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: numeric-spelling
  version: 1
spec:
  rules:
    - id: RANGE
      type: range
      target: score
      parameters:
        min: 0
        max: 2000
        allowNull: true
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/numeric-spelling.yaml",
            "ruleSetContent": policy,
            "headers": ["score"],
            "rows": [{"score": "1_000"}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["status"] == "FAIL"


def test_core_yaml_dialect_keeps_on_off_yes_no_as_strings() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: yaml-dialect
  version: 1
spec:
  rules:
    - id: ENUM
      type: enum
      target: state
      parameters:
        values: [ON, OFF, YES, NO]
        allowNull: false
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/yaml-dialect.yaml",
            "ruleSetContent": policy,
            "headers": ["state"],
            "rows": [{"state": "ON"}, {"state": "OFF"}, {"state": "YES"}, {"state": "NO"}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["status"] == "PASS"


def test_core_yaml_dialect_keeps_timestamp_looking_enum_as_string() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: timestamp-enum
  version: 1
spec:
  rules:
    - id: ENUM
      type: enum
      target: day
      parameters:
        values: [2026-01-01]
        allowNull: false
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/timestamp-enum.yaml",
            "ruleSetContent": policy,
            "headers": ["day"],
            "rows": [{"day": "2026-01-01"}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["status"] == "PASS"


def test_allow_null_string_false_matches_core_semantics() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: quoted-bool
  version: 1
spec:
  rules:
    - id: RANGE
      type: range
      target: score
      parameters:
        min: 0
        max: 1
        allowNull: "false"
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/quoted-bool.yaml",
            "ruleSetContent": policy,
            "headers": ["score"],
            "rows": [{"score": ""}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["status"] == "FAIL"


def test_rule_id_is_preserved_exactly() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: exact-id
  version: 1
spec:
  rules:
    - id: " RULE WITH SPACE "
      type: not_null
      target: id
      threshold: 1
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/exact-id.yaml",
            "ruleSetContent": policy,
            "headers": ["id"],
            "rows": [{"id": "A"}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["ruleId"] == " RULE WITH SPACE "


def test_target_is_preserved_exactly() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: exact-target
  version: 1
spec:
  rules:
    - id: TARGET
      type: not_null
      target: " id "
      threshold: 1
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/exact-target.yaml",
            "ruleSetContent": policy,
            "headers": ["id"],
            "rows": [{"id": "A"}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["status"] == "FAIL"


def test_allow_null_accepts_all_core_string_boolean_spellings() -> None:
    for raw, expected in [("t", "PASS"), ("1", "PASS"), ("f", "FAIL"), ("0", "FAIL")]:
        policy = f"""apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: bool-{raw}
  version: 1
spec:
  rules:
    - id: RANGE
      type: range
      target: score
      parameters:
        min: 0
        max: 1
        allowNull: "{raw}"
      required: true
"""
        response = client.post(
            "/v1/evaluate",
            json={
                "attemptId": "11111111-1111-4111-8111-111111111111",
                "datasetVersionId": "22222222-2222-4222-8222-222222222222",
                "ruleSetRef": f"quality/bool-{raw}.yaml",
                "ruleSetContent": policy,
                "headers": ["score"],
                "rows": [{"score": ""}],
            },
        )
        assert response.status_code == 200, response.text
        assert response.json()["findings"][0]["status"] == expected


def test_uniqueness_rule_id_is_preserved_exactly() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: exact-unique-id
  version: 1
spec:
  rules:
    - id: " UNIQUE WITH SPACE "
      type: unique
      target: id
      threshold: 1
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/exact-unique-id.yaml",
            "ruleSetContent": policy,
            "headers": ["id"],
            "rows": [{"id": "A"}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["ruleId"] == " UNIQUE WITH SPACE "


def test_duplicate_headers_follow_collapsed_row_map_semantics() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: duplicate-headers
  version: 1
spec:
  rules:
    - id: DUP-HEADER
      type: not_null
      target: id
      threshold: 1
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/duplicate-headers.yaml",
            "ruleSetContent": policy,
            "headers": ["id", "id"],
            "rows": [{"id": "last-value"}],
        },
    )
    assert response.status_code == 200, response.text
    assert response.json()["findings"][0]["status"] == "PASS"


def test_empty_required_rule_reports_zero_affected_rows() -> None:
    policy = """apiVersion: dataprod.platform/v1alpha1
kind: QualityRuleSet
metadata:
  name: empty-observation
  version: 1
spec:
  rules:
    - id: EMPTY
      type: not_null
      target: id
      threshold: 1
      required: true
"""
    response = client.post(
        "/v1/evaluate",
        json={
            "attemptId": "11111111-1111-4111-8111-111111111111",
            "datasetVersionId": "22222222-2222-4222-8222-222222222222",
            "ruleSetRef": "quality/empty-observation.yaml",
            "ruleSetContent": policy,
            "headers": ["id"],
            "rows": [],
        },
    )
    assert response.status_code == 200, response.text
    finding = response.json()["findings"][0]
    assert finding["status"] == "FAIL"
    assert finding["observed"]["affectedCount"] == 0
    assert finding["observed"]["total"] == 0
