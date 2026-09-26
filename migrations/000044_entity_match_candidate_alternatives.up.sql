ALTER TABLE entity_match_candidate
    ADD COLUMN candidate_alternatives jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE entity_match_candidate
    ADD CONSTRAINT ck_entity_match_candidate_alternatives_array
    CHECK (jsonb_typeof(candidate_alternatives) = 'array');
