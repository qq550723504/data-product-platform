-- Dependency bindings are historical facts. A rollback that deletes them would
-- destroy the proof of what production consumed, so the migration is explicitly
-- non-destructive and refuses a down migration once the schema is in use.
LOCK TABLE
    execution_dependency_binding,
    execution_mapping_usage,
    execution_dependency_preparation,
    entity_resolution_output_decision,
    entity_match_job
IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM execution_dependency_preparation LIMIT 1)
       OR EXISTS (SELECT 1 FROM execution_dependency_binding LIMIT 1)
       OR EXISTS (SELECT 1 FROM execution_mapping_usage LIMIT 1)
       OR EXISTS (SELECT 1 FROM entity_resolution_output_decision LIMIT 1)
       OR EXISTS (SELECT 1 FROM entity_match_job WHERE policy_content IS NOT NULL LIMIT 1) THEN
        RAISE EXCEPTION 'refusing destructive rollback of T3/B2 dependency facts';
    END IF;
END
$$;

DROP TABLE entity_resolution_output_decision;
DROP TABLE execution_mapping_usage;
DROP TABLE execution_dependency_binding;
DROP TABLE execution_dependency_preparation;
DROP FUNCTION IF EXISTS prevent_execution_dependency_fact_mutation();

DROP TRIGGER IF EXISTS trg_entity_match_job_policy_snapshot_immutable ON entity_match_job;
DROP FUNCTION IF EXISTS prevent_entity_match_policy_snapshot_mutation();

ALTER TABLE entity_match_job
    DROP CONSTRAINT ck_entity_match_job_policy_content_sha256,
    DROP COLUMN policy_content_sha256,
    DROP COLUMN policy_content;

ALTER TABLE entity_mapping_decision
    DROP CONSTRAINT uq_entity_mapping_decision_workspace_id;

ALTER TABLE entity_match_job
    DROP CONSTRAINT uq_entity_match_job_workspace_id;

ALTER TABLE execution
    DROP CONSTRAINT uq_execution_workspace_id;
