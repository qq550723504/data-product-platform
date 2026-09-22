-- Entity-match output idempotency.
--
-- One MatchJob may publish at most one live DatasetVersion in its declared
-- STANDARDIZED output Dataset. The producer identity is written at allocation
-- time so a concurrent/replayed finalizer reuses the same half-product/output
-- instead of consuming a second version number.

ALTER TABLE dataset_version
    ADD COLUMN generated_by_entity_match_job_id uuid;

ALTER TABLE dataset_version
    ADD CONSTRAINT fk_dataset_version_entity_match_job
    FOREIGN KEY (generated_by_entity_match_job_id)
    REFERENCES entity_match_job(id);

ALTER TABLE dataset_version
    ADD CONSTRAINT ck_dataset_version_single_producer CHECK (
        NOT (
            generated_by_execution_id IS NOT NULL
            AND generated_by_entity_match_job_id IS NOT NULL
        )
    );

CREATE UNIQUE INDEX uq_dataset_version_entity_match_output
    ON dataset_version(dataset_id, generated_by_entity_match_job_id)
    WHERE generated_by_entity_match_job_id IS NOT NULL
      AND status IN ('CREATED', 'PROCESSING', 'READY');

COMMENT ON INDEX uq_dataset_version_entity_match_output IS
    'One EntityMatchJob may have at most one live DatasetVersion output per Dataset.';

-- Producer identity is assigned at INSERT/allocation time and is never mutable.
-- This separate trigger avoids rewriting the historical 000002 migration while
-- extending the current immutability contract.
CREATE OR REPLACE FUNCTION guard_dataset_version_entity_match_producer()
RETURNS trigger AS $$
BEGIN
    IF NEW.generated_by_entity_match_job_id IS DISTINCT FROM OLD.generated_by_entity_match_job_id THEN
        RAISE EXCEPTION 'dataset_version entity-match producer identity is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dataset_version_entity_match_producer_immutable
BEFORE UPDATE ON dataset_version
FOR EACH ROW EXECUTE FUNCTION guard_dataset_version_entity_match_producer();
