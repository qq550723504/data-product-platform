ALTER TABLE entity_match_candidate
    DROP CONSTRAINT IF EXISTS ck_entity_match_candidate_alternatives_array;

ALTER TABLE entity_match_candidate
    DROP COLUMN IF EXISTS candidate_alternatives;
