#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
import tempfile
import unicodedata
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import pandas as pd
import splink.comparison_library as cl
import yaml
from splink import DuckDBAPI, Linker, SettingsCreator, block_on

ROOT = Path(__file__).resolve().parents[3]
FIXTURE_ROOT = ROOT / "examples" / "enterprise-activity" / "entity-resolution-evaluation"
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


def normalize_uscc(value: str, policy: dict[str, Any]) -> str:
    cfg = policy["spec"]["normalization"]["unifiedSocialCreditCode"]
    if cfg.get("trim"):
        value = value.strip()
    if cfg.get("uppercase"):
        value = value.upper()
    return value


def normalized_reference(row: dict[str, str], policy: dict[str, Any]) -> dict[str, Any]:
    return {
        "unique_id": row["entity_id"],
        "truth_id": row["entity_id"],
        "normalized_company_name": normalize_name(row["normalized_company_name"], policy),
        "normalized_registered_address": normalize_address(row["normalized_registered_address"], policy),
        "legal_representative": row["legal_representative"].strip(),
        "unified_social_credit_code": normalize_uscc(row["unified_social_credit_code"], policy) or None,
    }


def normalized_query(row: dict[str, str], label: dict[str, str], policy: dict[str, Any]) -> dict[str, Any]:
    expected = label.get("expected_entity_id", "")
    return {
        "unique_id": row["source_company_id"],
        "truth_id": expected or f"NO_MATCH:{row['source_company_id']}",
        "normalized_company_name": normalize_name(row["company_name"], policy),
        "normalized_registered_address": normalize_address(row["registered_address"], policy),
        "legal_representative": row["legal_representative"].strip(),
        "unified_social_credit_code": normalize_uscc(row["unified_social_credit_code"], policy) or None,
    }


def training_variants(references: list[dict[str, Any]]) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    for index, ref in enumerate(references, start=1):
        base_name = str(ref["normalized_company_name"])
        base_address = str(ref["normalized_registered_address"])
        base_legal = str(ref["legal_representative"])
        variants = [
            (base_name, base_address, base_legal, ref["unified_social_credit_code"]),
            (base_name, base_address, base_legal, None),
            (base_name.replace("科技", "技术"), base_address, base_legal, None),
            (base_name.replace("有限公司", "公司"), base_address + "A座", base_legal, None),
            (base_name.replace("智能", "智造"), base_address + "1栋", base_legal, None),
            (base_name.replace("装备", "设备"), base_address, base_legal, None),
        ]
        for variant_index, (name, address, legal, uscc) in enumerate(variants, start=1):
            result.append(
                {
                    "unique_id": f"TRAIN-{index:02d}-{variant_index:02d}",
                    "truth_id": ref["truth_id"],
                    "normalized_company_name": name,
                    "normalized_registered_address": address,
                    "legal_representative": legal,
                    "unified_social_credit_code": uscc,
                }
            )
    return result


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


def train_model(references: list[dict[str, Any]], output_path: Path) -> None:
    source_df = pd.DataFrame(training_variants(references))
    reference_df = pd.DataFrame(references)
    linker = Linker(
        [source_df, reference_df],
        settings_for(len(references)),
        input_table_aliases=["source", "reference"],
        db_api=DuckDBAPI(),
    )
    linker.training.estimate_u_using_random_sampling(max_pairs=10000, seed=42)
    linker.training.estimate_m_from_label_column("truth_id")
    output_path.parent.mkdir(parents=True, exist_ok=True)
    linker.misc.save_model_to_json(str(output_path), overwrite=True)


def prediction_scores(
    queries: list[dict[str, Any]],
    references: list[dict[str, Any]],
    model_path: Path,
) -> dict[str, list[tuple[str, float]]]:
    source_df = pd.DataFrame(queries).drop(columns=["truth_id"], errors="ignore")
    reference_df = pd.DataFrame(references).drop(columns=["truth_id"], errors="ignore")
    linker = Linker(
        [source_df, reference_df],
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
            source_id, entity_id = left_id, right_id
        elif left_source == "reference" and right_source == "source":
            source_id, entity_id = right_id, left_id
        else:
            continue
        score = float(row.get("match_probability") or 0.0)
        result.setdefault(source_id, []).append((entity_id, score))
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


def deterministic_decision(query: dict[str, Any], references: list[dict[str, Any]]) -> Decision:
    uscc = query.get("unified_social_credit_code")
    if uscc:
        for ref in references:
            if ref.get("unified_social_credit_code") == uscc:
                return Decision("AUTO_MATCH", str(ref["unique_id"]), 1.0, "USCC_EXACT")

    for ref in references:
        if (
            query["normalized_company_name"] == ref["normalized_company_name"]
            and query["normalized_registered_address"] == ref["normalized_registered_address"]
        ):
            return Decision("AUTO_MATCH", str(ref["unique_id"]), 0.98, "NAME_ADDRESS_EXACT")

    if query["legal_representative"]:
        best: tuple[dict[str, Any], float] | None = None
        for ref in references:
            if query["legal_representative"] != ref["legal_representative"]:
                continue
            score = similarity(str(query["normalized_company_name"]), str(ref["normalized_company_name"]))
            if score >= 0.88 and (best is None or score > best[1]):
                best = (ref, score)
        if best is not None:
            return Decision("REVIEW", str(best[0]["unique_id"]), min(0.85, best[1]), "NAME_SIMILAR_LEGAL_EXACT")

    return Decision("UNRESOLVED", method="DEFAULT")


def combined_decision(
    query: dict[str, Any],
    references: list[dict[str, Any]],
    scores: dict[str, list[tuple[str, float]]],
    auto_threshold: float,
    review_threshold: float,
) -> Decision:
    deterministic = deterministic_decision(query, references)
    if deterministic.decision != "UNRESOLVED":
        return deterministic
    candidates = scores.get(str(query["unique_id"]), [])
    if not candidates:
        return deterministic
    entity_id, score = candidates[0]
    if score >= auto_threshold:
        return Decision("AUTO_MATCH", entity_id, score, "FELLEGI_SUNTER", "SPLINK")
    if score >= review_threshold:
        return Decision("REVIEW", entity_id, score, "FELLEGI_SUNTER", "SPLINK")
    return Decision("UNRESOLVED", confidence=score, method="FELLEGI_SUNTER", engine="SPLINK")


def evaluate_mode(
    mode: str,
    queries: list[dict[str, Any]],
    labels: dict[str, dict[str, str]],
    decide: Any,
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
        expected = labels[source_id].get("expected_entity_id", "")
        decision: Decision = decide(query)
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
                "expectedEntityId": expected or None,
                "decision": decision.decision,
                "candidateEntityId": decision.entity_id or None,
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

    canonical_rows = read_csv(FIXTURE_ROOT / "canonical-companies.csv")
    query_rows = read_csv(FIXTURE_ROOT / "query-companies.csv")
    label_rows = read_csv(FIXTURE_ROOT / "labels.csv")
    labels = {row["source_company_id"]: row for row in label_rows}
    if set(labels) != {row["source_company_id"] for row in query_rows}:
        raise SystemExit("evaluation labels must cover every query exactly once")

    references = [normalized_reference(row, policy) for row in canonical_rows]
    queries = [normalized_query(row, labels[row["source_company_id"]], policy) for row in query_rows]
    thresholds = policy["spec"]["thresholds"]
    auto_threshold = float(thresholds["autoMatchMinimum"])
    review_threshold = float(thresholds["reviewMinimum"])

    temporary: tempfile.TemporaryDirectory[str] | None = None
    if args.model_out is None:
        temporary = tempfile.TemporaryDirectory(prefix="splink-evaluation-")
        model_path = Path(temporary.name) / "model.json"
    else:
        model_path = args.model_out
    train_model(references, model_path)
    scores = prediction_scores(queries, references, model_path)

    rules = evaluate_mode(
        "RULE_ONLY",
        queries,
        labels,
        lambda query: deterministic_decision(query, references),
    )
    combined = evaluate_mode(
        "RULE_PLUS_SPLINK",
        queries,
        labels,
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
            "canonicalCompanies": len(references),
            "queries": len(queries),
            "positiveLabels": sum(1 for row in label_rows if row["label"] == "MATCH"),
            "negativeLabels": sum(1 for row in label_rows if row["label"] == "NO_MATCH"),
        },
        "results": [rules, combined],
        "metricDefinitions": {
            "autoMatched": "Core accepted the candidate without human review",
            "review": "Core produced a candidate that requires Human Review",
            "unresolved": "No candidate reached reviewMinimum",
            "conflicts": "AUTO_MATCH or REVIEW proposed an entity different from the label",
            "falsePositive": "AUTO_MATCH proposed a wrong entity or matched a NO_MATCH label",
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
        exact = next(record for record in combined["records"] if record["sourceCompanyId"] == "EVAL-001")
        if exact["engine"] != "RULES" or exact["method"] != "USCC_EXACT":
            raise SystemExit("strong-key precedence regressed: EVAL-001 must remain a Core USCC decision")

    if temporary is not None:
        temporary.cleanup()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
