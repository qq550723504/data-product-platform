from __future__ import annotations

import importlib.util
import sys
from pathlib import Path

import pytest

MODULE_PATH = Path(__file__).resolve().parents[1] / "scripts" / "evaluate_reference.py"
_spec = importlib.util.spec_from_file_location("evaluate_reference", MODULE_PATH)
assert _spec is not None and _spec.loader is not None
evaluate_reference = importlib.util.module_from_spec(_spec)
sys.modules.setdefault("evaluate_reference", evaluate_reference)
_spec.loader.exec_module(evaluate_reference)

validate_evaluation_labels = evaluate_reference.validate_evaluation_labels


def _row(source_id: str, expected: str, label: str) -> dict[str, str]:
    return {"source_unique_id": source_id, "expected_reference_id": expected, "label": label}


def test_valid_labels_are_indexed_by_source_id() -> None:
    labels = validate_evaluation_labels(
        [
            _row("E001", "R007", "MATCH"),
            _row("E002", "", "NO_MATCH"),
        ]
    )
    assert set(labels) == {"E001", "E002"}
    assert labels["E001"]["expected_reference_id"] == "R007"


def test_duplicate_source_id_rows_are_rejected() -> None:
    rows = [
        _row("E001", "R007", "MATCH"),
        _row("E001", "", "NO_MATCH"),
    ]
    with pytest.raises(SystemExit, match="duplicate source_unique_id rows: E001"):
        validate_evaluation_labels(rows)


def test_match_label_requires_a_reference() -> None:
    with pytest.raises(SystemExit, match="E001 is MATCH but expected_reference_id is empty"):
        validate_evaluation_labels([_row("E001", "", "MATCH")])


def test_no_match_label_rejects_a_reference() -> None:
    with pytest.raises(SystemExit, match="E002 is NO_MATCH but expected_reference_id is 'R008'"):
        validate_evaluation_labels([_row("E002", "R008", "NO_MATCH")])


def test_unknown_label_is_rejected() -> None:
    with pytest.raises(SystemExit, match="E003 has unknown label 'MAYBE'"):
        validate_evaluation_labels([_row("E003", "R009", "MAYBE")])


def test_missing_source_id_is_rejected() -> None:
    with pytest.raises(SystemExit, match="require source_unique_id"):
        validate_evaluation_labels([_row("", "R010", "MATCH")])
