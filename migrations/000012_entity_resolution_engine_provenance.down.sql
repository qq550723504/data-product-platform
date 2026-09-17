ALTER TABLE entity_mapping
    DROP COLUMN IF EXISTS match_model_version,
    DROP COLUMN IF EXISTS match_engine_version,
    DROP COLUMN IF EXISTS match_engine_name;

ALTER TABLE entity_match_candidate
    DROP COLUMN IF EXISTS match_model_version,
    DROP COLUMN IF EXISTS match_engine_version,
    DROP COLUMN IF EXISTS match_engine_name;
