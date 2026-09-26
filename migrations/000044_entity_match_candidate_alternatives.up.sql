ALTER TABLE entity_match_candidate
    ADD COLUMN candidate_alternatives jsonb NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE entity_match_candidate
    ADD CONSTRAINT ck_entity_match_candidate_alternatives_array
    CHECK (jsonb_typeof(candidate_alternatives) = 'array');

CREATE OR REPLACE FUNCTION prevent_entity_match_candidate_alternatives_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.candidate_alternatives IS DISTINCT FROM OLD.candidate_alternatives THEN
        RAISE EXCEPTION 'entity match candidate alternatives are immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_entity_match_candidate_alternatives_immutable
BEFORE UPDATE OF candidate_alternatives ON entity_match_candidate
FOR EACH ROW
EXECUTE FUNCTION prevent_entity_match_candidate_alternatives_mutation();
