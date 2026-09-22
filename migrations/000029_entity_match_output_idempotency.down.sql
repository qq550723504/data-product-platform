-- Do not erase producer provenance on rollback. This repository is
-- pre-production, but migration down still must preserve committed facts.
LOCK TABLE dataset_version IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM dataset_version
        WHERE generated_by_entity_match_job_id IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'cannot remove entity-match output producer identity while DatasetVersion facts exist';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_dataset_version_entity_match_producer_immutable ON dataset_version;
DROP FUNCTION IF EXISTS guard_dataset_version_entity_match_producer();
DROP INDEX IF EXISTS uq_dataset_version_entity_match_output;
ALTER TABLE dataset_version DROP CONSTRAINT IF EXISTS ck_dataset_version_single_producer;
ALTER TABLE dataset_version DROP CONSTRAINT IF EXISTS fk_dataset_version_entity_match_job;
ALTER TABLE dataset_version DROP COLUMN IF EXISTS generated_by_entity_match_job_id;
