#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
import tempfile
import unicodedata
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

import pandas as pd
import splink.comparison_library as cl
import yaml
from splink import DuckDBAPI, Linker, SettingsCreator, block_on

ROOT = Path(__file__).resolve().parents[3]
FIXTURE_ROOT = ROOT / "examples" / "entity-resolution-evaluation"
POLICY_PATH = ROOT / "industry-packs" / "park" / "matching" / "company-match-policy-v1.yaml"
MODEL_SPEC_PATH = ROOT / "industry-packs" / "park" / "matching" / "splink-company-model-v1.yaml"


@dataclass(frozen=True)
class Decision:
    decision: str
    entity_id: str = ""
    confidence: float = 0.0
    method: str = ""
    engine: str = "RULES"


def read_csv(path: Path) -> list[dict[str, str]]:
    with path.open(newline="", encoding="utf-8-sig") as handle:
        return [{key: (value or "").strip() for key, value in row.items()} for row in csv.DictReader(handle)]


def load_yaml(path: Path) -> dict[str, Any]:
    with path.open(encoding="utf-8") as handle:
        value = yaml.safe_load(handle)
    if not isinstance(value, dict):
        raise ValueError(f"{path} must contain a YAML object")
    return value


def normalize_width(value: str) -> str:
    return unicodedata.normalize("NFKC", value)


def normalize_name(value: str, policy: dict[str, Any]) -> str:
    cfg = policy["spec"]["normalization"]["companyName"]
    if cfg.get("normalizeFullWidthHalfWidth"):
        value = normalize_width(value)
    if cfg.get("trim"):
        value = value.strip()
    if cfg.get("normalizeWhitespace"):
        value = "".join(value.split())
    for alias in cfg.get("aliases", []):
        suffix = str(alias.get("suffix", ""))
        canonical = str(alias.get("canonical", ""))
        if suffix and value.endswith(suffix):
            value = value[: -len(suffix)] + canonical
    return value


def normalize_address(value: str, policy: dict[str, Any]) -> str:
    cfg = policy["spec"]["normalization"]["address"]
    if cfg.get("trim"):
        value = value.strip()
    if cfg.get("normalizeWhitespace"):
        value = "".join(value.split())
    return value


def normalize_uscc(value: str, policy: dict[str, Any]) -> str | None:
    cfg = policy["spec"]["normalization"]["unifiedSocialCreditCode"]
    if cfg.get("trim"):
        value = value.strip()
    if cfg.get("uppercase"):
        value = value.upper()
    return value or None


def normalize_reference(row: dict[str, str], policy: dict[str, Any]) -> dict[str, Any]:
    return {
        "unique_id": row["unique_id"],
        "normalized_company_name": normalize_name(row["normalized_company_name"], policy),
        "normalized_registered_address": normalize_address(row["normalized_registered_address"], policy),
        "legal_representative": row["legal_representative"].strip() or None,
        "unified_social_credit_code": normalize_uscc(row["unified_social_credit_code"], policy),
    }


def normalize_source(row: dict[str, str], policy: dict[str, Any]) -> dict[str, Any]:
    return {
        "unique_id": row["unique_id"],
        "normalized_company_name": normalize_name(row["normalized_company_name"], policy),
        "normalized_registered_address": normalize_address(row["normalized_registered_address"], policy),
        "legal_representative": row["legal_representative"].strip() or None,
        "unified_social_credit_code": normalize_uscc(row["unified_social_credit_code"], policy),
    }


def settings_for(reference_count: int) -> SettingsCreator:
    return SettingsCreator(
        link_type="link_only",
        probability_two_random_records_match=1.0 / max(reference_count, 1),
        blocking_rules_to_generate_predictions=[
            block_on("legal_representative"),
            block_on("normalized_registered_address"),
            'substr(l."normalized_company_name", 1, 4) = substr(r."normalized_company_name", 1, 4)',
        ],
        comparisons=[
            cl.NameComparison(
                "normalized_company_name",
                jaro_winkler_thresholds=[0.96, 0.90, 0.80],
            ),
            cl.JaroWinklerAtThresholds(
                "normalized_registered_address",
                [0.97, 0.90, 0.80],
            ),
            cl.ExactMatch("legal_representative"),
            cl.ExactMatch("unified_social_credit_code"),
        ],
        retain_intermediate_calculation_columns=True,
    )


def train_model(
    training_sources: list[dict[str, Any]],
    references: list[dict[str, Any]],
    training_labels: list[dict[str, str]],
    output_path: Path,
) -> None:
    linker = Linker(
        [pd.DataFrame(training_sources), pd.DataFrame(references)],
        settings_for(len(references)),
        input_table_aliases=["source", "reference"],
        db_api=DuckDBAPI(),
    )
    linker.training.estimate_u_using_random_sampling(max_pairs=10000, seed=42)
    labels_table = linker.table_management.register_labels_table(pd.DataFrame(training_labels), overwrite=True)
    linker.training.estimate_m_from_pairwise_labels(labels_table)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    linker.misc.save_model_to_json(str(output_path), overwrite=True)


def prediction_scores(
    queries: list[dict[str, Any]],
    references: list[dict[str, Any]],
    model_path: Path,
) -> dict[str, list[tuple[str, float]]]:
    linker = Linker(
        [pd.DataFrame(queries), pd.DataFrame(references)],
        settings=str(model_path),
        input_table_aliases=["source", "reference"],
        db_api=DuckDBAPI(),
    )
    rows = linker.inference.predict(threshold_match_probability=0.0).as_record_dict()
    result: dict[str, list[tuple[str, float]]] = {}
    for row in rows:
        left_id = str(row.get("unique_id_l", ""))
        right_id = str(row.get("unique_id_r", ""))
        left_source = str(row.get("source_dataset_l", ""))
        right_source = str(row.get("source_dataset_r", ""))
        if left_source == "source" and right_source == "reference":
            source_id, reference_id = left_id, right_id
        elif left_source == "reference" and right_source == "source":
            source_id, reference_id = right_id, left_id
        else:
            continue
        score = float(row.get("match_probability") or 0.0)
        result.setdefault(source_id, []).append((reference_id, score))
    for candidates in result.values():
        candidates.sort(key=lambda item: (-item[1], item[0]))
    return result


def similarity(a: str, b: str) -> float:
    if not a and not b:
        return 1.0
    if not a or not b:
        return 0.0
    previous = list(range(len(b) + 1))
    for i, char_a in enumerate(a, start=1):
        current = [i]
        for j, char_b in enumerate(b, start=1):
            cost = 0 if char_a == char_b else 1
            current.append(min(previous[j] + 1, current[j - 1] + 1, previous[j - 1] + cost))
        previous = current
    return 1.0 - previous[-1] / max(len(a), len(b))


def deterministic_paths(
    query: dict[str, Any],
    references: list[dict[str, Any]],
) -> tuple[Decision | None, Decision | None]:
    uscc = query.get("unified_social_credit_code")
    if uscc:
        for ref in references:
            if ref.get("unified_social_credit_code") == uscc:
                return Decision("AUTO_MATCH", str(ref["unique_id"]), 1.0, "USCC_EXACT"), None

    for ref in references:
        if (
            query["normalized_company_name"] == ref["normalized_company_name"]
            and query["normalized_registered_address"] == ref["normalized_registered_address"]
        ):
            return Decision("AUTO_MATCH", str(ref["unique_id"]), 0.98, "NAME_ADDRESS_EXACT"), None

    review: Decision | None = None
    legal = query.get("legal_representative")
    if legal:
        best: tuple[dict[str, Any], float] | None = None
        for ref in references:
            if legal != ref.get("legal_representative"):
                continue
            score = similarity(str(query["normalized_company_name"]), str(ref["normalized_company_name"]))
            if score >= 0.88 and (best is None or score > best[1]):
                best = (ref, score)
        if best is not None:
            review = Decision(
                "REVIEW",
                str(best[0]["unique_id"]),
                min(0.85, best[1]),
                "NAME_SIMILAR_LEGAL_EXACT",
            )
    return None, review


def rule_only_decision(query: dict[str, Any], references: list[dict[str, Any]]) -> Decision:
    automatic, review = deterministic_paths(query, references)
    return automatic or review or Decision("UNRESOLVED", method="DEFAULT")


def combined_decision(
    query: dict[str, Any],
    references: list[dict[str, Any]],
    scores: dict[str, list[tuple[str, float]]],
    auto_threshold: float,
    review_threshold: float,
) -> Decision:
    automatic, review_fallback = deterministic_paths(query, references)
    if automatic is not None:
        return automatic

    candidates = scores.get(str(query["unique_id"]), [])
    if candidates:
        reference_id, score = candidates[0]
        if score >= auto_threshold:
            return Decision("AUTO_MATCH", reference_id, score, "FELLEGI_SUNTER", "SPLINK")
        if score >= review_threshold:
            return Decision("REVIEW", reference_id, score, "FELLEGI_SUNTER", "SPLINK")
        if review_fallback is None:
            return Decision("UNRESOLVED", confidence=score, method="FELLEGI_SUNTER", engine="SPLINK")

    return review_fallback or Decision("UNRESOLVED", method="DEFAULT")


def evaluate_mode(
    mode: str,
    queries: list[dict[str, Any]],
    labels: dict[str, dict[str, str]],
    decide: Callable[[dict[str, Any]], Decision],
) -> dict[str, Any]:
    counts = {
        "autoMatched": 0,
        "review": 0,
        "unresolved": 0,
        "conflicts": 0,
        "falsePositive": 0,
        "falseNegative": 0,
    }
    records: list[dict[str, Any]] = []
    for query in queries:
        source_id = str(query["unique_id"])
        expected = labels[source_id].get("expected_reference_id", "")
        decision = decide(query)
        if decision.decision == "AUTO_MATCH":
            counts["autoMatched"] += 1
        elif decision.decision == "REVIEW":
            counts["review"] += 1
        else:
            counts["unresolved"] += 1

        proposed = decision.entity_id if decision.decision in {"AUTO_MATCH", "REVIEW"} else ""
        if proposed and proposed != expected:
            counts["conflicts"] += 1
        if decision.decision == "AUTO_MATCH" and proposed != expected:
            counts["falsePositive"] += 1
        if expected and decision.decision == "UNRESOLVED":
            counts["falseNegative"] += 1

        records.append(
            {
                "sourceCompanyId": source_id,
                "expectedReferenceId": expected or None,
                "decision": decision.decision,
                "candidateReferenceId": decision.entity_id or None,
                "confidence": round(decision.confidence, 6),
                "method": decision.method,
                "engine": decision.engine,
            }
        )
    return {"mode": mode, "metrics": counts, "records": records}


def main() -> int:
    parser = argparse.ArgumentParser(description="Train and evaluate the Park COMPANY Splink reference model")
    parser.add_argument("--report", type=Path, default=None, help="Write evaluation report JSON")
    parser.add_argument("--model-out", type=Path, default=None, help="Write the trained Splink model JSON")
    parser.add_argument("--assert-quality", action="store_true", help="Fail if reference quality invariants regress")
    args = parser.parse_args()

    policy = load_yaml(POLICY_PATH)
    model_spec = load_yaml(MODEL_SPEC_PATH)
    binding = model_spec["spec"]["matchingPolicy"]
    if binding["name"] != policy["metadata"]["name"] or binding["version"] != policy["metadata"]["version"]:
        raise SystemExit("model spec and matching policy binding disagree")

    reference_rows = read_csv(FIXTURE_ROOT / "references.csv")
    source_rows = read_csv(FIXTURE_ROOT / "sources.csv")
    training_labels = read_csv(FIXTURE_ROOT / "training_labels.csv")
    evaluation_label_rows = read_csv(FIXTURE_ROOT / "evaluation_labels.csv")
    evaluation_labels = {row["source_unique_id"]: row for row in evaluation_label_rows}

    references = [normalize_reference(row, policy) for row in reference_rows]
    training_sources = [normalize_source(row, policy) for row in source_rows if row["split"] == "train"]
    queries = [normalize_source(row, policy) for row in source_rows if row["split"] == "eval"]
    if set(evaluation_labels) != {row["unique_id"] for row in queries}:
        raise SystemExit("evaluation labels must cover every eval source exactly once")

    thresholds = policy["spec"]["thresholds"]
    auto_threshold = float(thresholds["autoMatchMinimum"])
    review_threshold = float(thresholds["reviewMinimum"])

    temporary: tempfile.TemporaryDirectory[str] | None = None
    if args.model_out is None:
        temporary = tempfile.TemporaryDirectory(prefix="splink-evaluation-")
        model_path = Path(temporary.name) / "model.json"
    else:
        model_path = args.model_out
    train_model(training_sources, references, training_labels, model_path)
    scores = prediction_scores(queries, references, model_path)

    rules = evaluate_mode(
        "RULE_ONLY",
        queries,
        evaluation_labels,
        lambda query: rule_only_decision(query, references),
    )
    combined = evaluate_mode(
        "RULE_PLUS_SPLINK",
        queries,
        evaluation_labels,
        lambda query: combined_decision(query, references, scores, auto_threshold, review_threshold),
    )

    report = {
        "schemaVersion": "1.0",
        "matchingPolicy": {
            "name": policy["metadata"]["name"],
            "version": policy["metadata"]["version"],
            "autoMatchMinimum": auto_threshold,
            "reviewMinimum": review_threshold,
        },
        "model": {
            "name": model_spec["metadata"]["name"],
            "version": model_spec["metadata"]["version"],
            "engine": model_spec["spec"]["engine"]["name"],
            "engineVersion": model_spec["spec"]["engine"]["version"],
        },
        "fixture": {
            "references": len(references),
            "trainingSources": len(training_sources),
            "evaluationSources": len(queries),
            "trainingPositiveLabels": len(training_labels),
            "evaluationPositiveLabels": sum(1 for row in evaluation_label_rows if row["label"] == "MATCH"),
            "evaluationNegativeLabels": sum(1 for row in evaluation_label_rows if row["label"] == "NO_MATCH"),
        },
        "results": [rules, combined],
        "metricDefinitions": {
            "autoMatched": "Core accepted the candidate without human review",
            "review": "Core produced a candidate that requires Human Review",
            "unresolved": "No candidate reached reviewMinimum and no deterministic review fallback applied",
            "conflicts": "AUTO_MATCH or REVIEW proposed a reference different from the label",
            "falsePositive": "AUTO_MATCH proposed a wrong reference or matched a NO_MATCH label",
            "falseNegative": "A labelled MATCH remained UNRESOLVED; REVIEW is not counted as a false negative",
        },
    }

    encoded = json.dumps(report, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
    if args.report is not None:
        args.report.parent.mkdir(parents=True, exist_ok=True)
        args.report.write_text(encoded, encoding="utf-8")
    print(encoded, end="")

    if args.assert_quality:
        rule_metrics = rules["metrics"]
        combined_metrics = combined["metrics"]
        if combined_metrics["falsePositive"] > rule_metrics["falsePositive"]:
            raise SystemExit("rule+Splink introduced additional automatic false positives")
        if combined_metrics["falseNegative"] > rule_metrics["falseNegative"]:
            raise SystemExit("rule+Splink increased false negatives")
        if not any(record["engine"] == "SPLINK" for record in combined["records"]):
            raise SystemExit("evaluation did not exercise the Splink candidate path")
        exact = next(record for record in combined["records"] if record["sourceCompanyId"] == "E007")
        if exact["engine"] != "RULES" or exact["method"] != "USCC_EXACT":
            raise SystemExit("strong-key precedence regressed: E007 must remain a Core USCC decision")

    if temporary is not None:
        temporary.cleanup()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
