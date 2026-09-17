from __future__ import annotations

import argparse
from pathlib import Path

import pandas as pd
import splink.comparison_library as cl
from splink import DuckDBAPI, Linker, SettingsCreator


def build_settings(reference_count: int) -> SettingsCreator:
    if reference_count <= 0:
        raise ValueError("reference_count must be positive")

    # POC reference model: one source company is expected to map to at most one
    # canonical company in the current reference set. Strong identifiers are
    # still decided by Core before this model is called.
    return SettingsCreator(
        link_type="link_only",
        probability_two_random_records_match=1 / reference_count,
        blocking_rules_to_generate_predictions=[],
        comparisons=[
            cl.JaroWinklerAtThresholds(
                "normalized_company_name",
                score_threshold_or_thresholds=[0.95, 0.88, 0.75],
            ),
            cl.JaroWinklerAtThresholds(
                "normalized_registered_address",
                score_threshold_or_thresholds=[0.95, 0.88, 0.75],
            ),
            cl.ExactMatch("legal_representative"),
            cl.ExactMatch("unified_social_credit_code"),
        ],
        retain_matching_columns=True,
        retain_intermediate_calculation_columns=True,
    )


def train(model_dir: Path) -> Path:
    references_path = model_dir / "references.csv"
    sources_path = model_dir / "training-sources.csv"
    output_path = model_dir / "model.json"

    references = pd.read_csv(references_path, keep_default_na=False, dtype=str)
    sources = pd.read_csv(sources_path, keep_default_na=False, dtype=str)
    required = {
        "unique_id",
        "cluster_id",
        "normalized_company_name",
        "normalized_registered_address",
        "legal_representative",
        "unified_social_credit_code",
    }
    for name, frame in (("references", references), ("training sources", sources)):
        missing = required.difference(frame.columns)
        if missing:
            raise ValueError(f"{name} missing columns: {sorted(missing)}")

    settings = build_settings(len(references))
    linker = Linker(
        [sources, references],
        settings,
        db_api=DuckDBAPI(),
        input_table_aliases=["source", "reference"],
        set_up_basic_logging=False,
    )

    # Both steps are deterministic for these committed fixtures.  The label
    # column provides supervised m estimates while random sampling estimates u.
    linker.training.estimate_u_using_random_sampling(max_pairs=100_000, seed=42)
    linker.training.estimate_m_from_label_column("cluster_id")
    linker.misc.save_model_to_json(str(output_path), overwrite=True)
    return output_path


def main() -> None:
    parser = argparse.ArgumentParser(description="Train the Park COMPANY Splink reference model")
    parser.add_argument(
        "--model-dir",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "models" / "park-company-v1",
    )
    args = parser.parse_args()
    output = train(args.model_dir.resolve())
    print(output)


if __name__ == "__main__":
    main()
