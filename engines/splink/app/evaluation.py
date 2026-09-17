from __future__ import annotations

import argparse
import hashlib
import json
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any

import pandas as pd
from splink import DuckDBAPI, Linker

AUTO_MATCH = "AUTO_MATCH"
REVIEW = "REVIEW"
UNRESOLVED = "UNRESOLVED"


@dataclass(frozen=True)
class Decision:
    source_id: str
    expected_reference_id: str
    decision: str
    predicted_reference_id: str
    method: str
    score: float


def _text(value: Any) -> str:
    if value is None:
        return ""
    return str(value).strip()


def _levenshtein_similarity(left: str, right: str) -> float:
    a = list(left)
    b = list(right)
    max_len = max(len(a), len(b))
    if max_len == 0:
        return 1.0
    previous = list(range(len(b) + 1))
    for i, ca in enumerate(a, start=1):
        current = [i]
        for j, cb in enumerate(b, start=1):
            cost = 0 if ca == cb else 1
            current.append(min(previous[j] + 1, current[j - 1] + 1, previous[j - 1] + cost))
        previous = current
    return 1.0 - previous[-1] / max_len


def deterministic_decision(source: dict[str, str], references: list[dict[str, str]]) -> Decision:
    source_id = source["unique_id"]
    expected = source.get("expected_reference_id", "")
    uscc = source.get("unified_social_credit_code", "")
    if uscc:
        for reference in references:
            if reference.get("unified_social_credit_code", "") == uscc:
                return Decision(source_id, expected, AUTO_MATCH, reference["unique_id"], "USCC_EXACT", 1.0)

    name = source.get("normalized_company_name", "")
    address = source.get("normalized_registered_address", "")
    if name and address:
        for reference in references:
            if (
                reference.get("normalized_company_name", "") == name
                and reference.get("normalized_registered_address", "") == address
            ):
                return Decision(source_id, expected, AUTO_MATCH, reference["unique_id"], "NAME_ADDRESS_EXACT", 0.98)

    legal = source.get("legal_representative", "")
    if name and legal:
        best_id = ""
        best_similarity = 0.0
        for reference in references:
            if reference.get("legal_representative", "") != legal:
                continue
            similarity = _levenshtein_similarity(name, reference.get("normalized_company_name", ""))
            if similarity >= 0.88 and similarity > best_similarity:
                best_similarity = similarity
                best_id = reference["unique_id"]
        if best_id:
            return Decision(
                source_id,
                expected,
                REVIEW,
                best_id,
                "NAME_SIMILAR_LEGAL_EXACT",
                min(0.85, best_similarity),
            )

    return Decision(source_id, expected, UNRESOLVED, "", "DEFAULT", 0.0)


def _reference_id(prediction: dict[str, Any], source_id: str) -> str:
    left = _text(prediction.get("unique_id_l"))
    right = _text(prediction.get("unique_id_r"))
    left_source = _text(prediction.get("source_dataset_l"))
    right_source = _text(prediction.get("source_dataset_r"))
    if left_source == "reference":
        return left
    if right_source == "reference":
        return right
    if left == source_id:
        return right
    if right == source_id:
        return left
    return ""


def splink_decision(
    source: dict[str, str],
    references_df: pd.DataFrame,
    model_path: Path,
    auto_threshold: float,
    review_threshold: float,
) -> Decision:
    source_id = source["unique_id"]
    expected = source.get("expected_reference_id", "")
    source_frame = pd.DataFrame(
        [
            {
                "unique_id": source_id,
                "normalized_company_name": source.get("normalized_company_name", ""),
                "normalized_registered_address": source.get("normalized_registered_address", ""),
                "legal_representative": source.get("legal_representative", ""),
                "unified_social_credit_code": source.get("unified_social_credit_code", ""),
            }
        ]
    )
    linker = Linker(
        [source_frame, references_df],
        settings=str(model_path),
        db_api=DuckDBAPI(),
        input_table_aliases=["source", "reference"],
        set_up_basic_logging=False,
    )
    rows = linker.inference.predict(threshold_match_probability=0.0).as_record_dict()
    best_id = ""
    best_score = 0.0
    for row in rows:
        entity_id = _reference_id(row, source_id)
        if not entity_id:
            continue
        score = float(row.get("match_probability") or 0.0)
        if score > best_score or (score == best_score and entity_id < best_id):
            best_id = entity_id
            best_score = score

    if not best_id:
        return Decision(source_id, expected, UNRESOLVED, "", "SPLINK", 0.0)
    if best_score >= auto_threshold:
        decision = AUTO_MATCH
    elif best_score >= review_threshold:
        decision = REVIEW
    else:
        decision = UNRESOLVED
        best_id = ""
    return Decision(source_id, expected, decision, best_id, "SPLINK_FELLEGI_SUNTER", best_score)


def evaluate_mode(decisions: list[Decision]) -> dict[str, Any]:
    tp = fp = fn = tn = 0
    review_correct = review_incorrect = 0
    candidate_correct = 0
    positives = sum(1 for item in decisions if item.expected_reference_id)
    false_positives: list[str] = []
    false_negatives: list[str] = []

    for item in decisions:
        expected = item.expected_reference_id
        predicted_auto = item.predicted_reference_id if item.decision == AUTO_MATCH else ""
        if expected:
            if predicted_auto == expected:
                tp += 1
            else:
                fn += 1
                false_negatives.append(item.source_id)
        elif predicted_auto:
            fp += 1
            false_positives.append(item.source_id)
        else:
            tn += 1

        if item.decision == REVIEW and item.predicted_reference_id:
            if item.predicted_reference_id == expected and expected:
                review_correct += 1
            else:
                review_incorrect += 1
        if item.decision in (AUTO_MATCH, REVIEW) and item.predicted_reference_id == expected and expected:
            candidate_correct += 1

    precision = tp / (tp + fp) if tp + fp else 1.0
    recall = tp / (tp + fn) if tp + fn else 1.0
    candidate_recall = candidate_correct / positives if positives else 1.0
    return {
        "total": len(decisions),
        "positives": positives,
        "truePositives": tp,
        "falsePositives": fp,
        "falseNegatives": fn,
        "trueNegatives": tn,
        "autoPrecision": round(precision, 6),
        "autoRecall": round(recall, 6),
        "candidateRecall": round(candidate_recall, 6),
        "reviewCorrect": review_correct,
        "reviewIncorrect": review_incorrect,
        "unresolved": sum(1 for item in decisions if item.decision == UNRESOLVED),
        "falsePositiveCases": false_positives,
        "falseNegativeCases": false_negatives,
    }


def evaluate(model_dir: Path, auto_threshold: float, review_threshold: float) -> dict[str, Any]:
    references_df = pd.read_csv(model_dir / "references.csv", keep_default_na=False, dtype=str)
    sources_df = pd.read_csv(model_dir / "evaluation-sources.csv", keep_default_na=False, dtype=str)
    model_path = model_dir / "model.json"
    if not model_path.exists():
        raise FileNotFoundError(f"trained model not found: {model_path}")

    reference_fields = [
        "unique_id",
        "normalized_company_name",
        "normalized_registered_address",
        "legal_representative",
        "unified_social_credit_code",
    ]
    references_for_model = references_df[reference_fields].copy()
    references = references_for_model.to_dict(orient="records")
    sources = sources_df.to_dict(orient="records")

    rule_only: list[Decision] = []
    rule_plus_splink: list[Decision] = []
    for source in sources:
        deterministic = deterministic_decision(source, references)
        rule_only.append(deterministic)
        if deterministic.decision != UNRESOLVED:
            rule_plus_splink.append(deterministic)
            continue
        rule_plus_splink.append(
            splink_decision(source, references_for_model, model_path, auto_threshold, review_threshold)
        )

    model_sha256 = hashlib.sha256(model_path.read_bytes()).hexdigest()
    return {
        "evaluationVersion": "1.0.0",
        "engine": {"name": "SPLINK", "version": "4.0.17", "modelRef": "park-company-v1", "modelVersion": "1.0.0"},
        "policy": {
            "name": "park-company-match",
            "version": "1.0.0",
            "autoMatchMinimum": auto_threshold,
            "reviewMinimum": review_threshold,
        },
        "modelSha256": model_sha256,
        "ruleOnly": {
            "metrics": evaluate_mode(rule_only),
            "cases": [asdict(item) for item in rule_only],
        },
        "rulePlusSplink": {
            "metrics": evaluate_mode(rule_plus_splink),
            "cases": [asdict(item) for item in rule_plus_splink],
        },
    }


def _print_summary(report: dict[str, Any]) -> None:
    print("mode,auto_precision,auto_recall,candidate_recall,false_positives,false_negatives,review_correct,review_incorrect,unresolved")
    for key in ("ruleOnly", "rulePlusSplink"):
        metrics = report[key]["metrics"]
        print(
            ",".join(
                [
                    key,
                    str(metrics["autoPrecision"]),
                    str(metrics["autoRecall"]),
                    str(metrics["candidateRecall"]),
                    str(metrics["falsePositives"]),
                    str(metrics["falseNegatives"]),
                    str(metrics["reviewCorrect"]),
                    str(metrics["reviewIncorrect"]),
                    str(metrics["unresolved"]),
                ]
            )
        )


def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate Core deterministic matching against rule + Splink")
    parser.add_argument(
        "--model-dir",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "models" / "park-company-v1",
    )
    parser.add_argument("--auto-threshold", type=float, default=0.95)
    parser.add_argument("--review-threshold", type=float, default=0.75)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--assert-non-regression", action="store_true")
    args = parser.parse_args()

    if not 0 < args.review_threshold <= args.auto_threshold <= 1:
        raise SystemExit("thresholds must satisfy 0 < review <= auto <= 1")

    report = evaluate(args.model_dir.resolve(), args.auto_threshold, args.review_threshold)
    _print_summary(report)
    if args.output:
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

    if args.assert_non_regression:
        baseline = report["ruleOnly"]["metrics"]
        enhanced = report["rulePlusSplink"]["metrics"]
        if enhanced["falsePositives"] > baseline["falsePositives"]:
            raise SystemExit("rule+Splink increased automatic false positives")
        if enhanced["candidateRecall"] < baseline["candidateRecall"]:
            raise SystemExit("rule+Splink reduced candidate recall")
        if enhanced["candidateRecall"] == baseline["candidateRecall"]:
            raise SystemExit("reference evaluation shows no candidate-recall improvement from Splink")


if __name__ == "__main__":
    main()
