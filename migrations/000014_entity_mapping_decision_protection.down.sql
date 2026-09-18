-- Roll back the decision-chain protection added by 000014. The immutable
-- decision rows themselves are never touched; only the pointer and the
-- provenance columns added by this migration are removed.
ALTER TABLE entity_mapping
    DROP CONSTRAINT IF EXISTS fk_entity_mapping_current_decision,
    DROP COLUMN IF EXISTS current_decision_id;

ALTER TABLE entity_mapping_decision
    DROP CONSTRAINT IF EXISTS uq_entity_mapping_decision_workspace_mapping_id;

DROP INDEX IF EXISTS uq_entity_mapping_decision_idempotency;

ALTER TABLE entity_mapping_decision
    DROP CONSTRAINT IF EXISTS ck_entity_mapping_decision_source_origin,
    DROP CONSTRAINT IF EXISTS fk_entity_mapping_decision_job,
    DROP CONSTRAINT IF EXISTS fk_entity_mapping_decision_candidate,
    DROP COLUMN IF EXISTS source_candidate_id,
    DROP COLUMN IF EXISTS source_job_id,
    DROP COLUMN IF EXISTS source_origin,
    DROP COLUMN IF EXISTS idempotency_key;
