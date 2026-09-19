-- A down migration must never silently erase proof of a completed assessment.
-- The table locks serialize this check with normal assessment/finding writes.
LOCK TABLE quality_result, quality_finding IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM quality_result LIMIT 1)
       OR EXISTS (SELECT 1 FROM quality_finding LIMIT 1) THEN
        RAISE EXCEPTION 'refusing destructive rollback of quality assessment history';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS trg_quality_finding_immutable ON quality_finding;
DROP FUNCTION IF EXISTS prevent_quality_assessment_child_mutation();

ALTER TABLE quality_finding
    DROP CONSTRAINT IF EXISTS fk_quality_finding_result;

ALTER TABLE quality_finding
    ADD CONSTRAINT quality_finding_result_id_fkey
    FOREIGN KEY (result_id) REFERENCES quality_result(id);

DROP INDEX IF EXISTS idx_quality_result_dataset_version_history;

ALTER TABLE quality_result
    DROP CONSTRAINT IF EXISTS ck_quality_result_rule_set_content_pair,
    DROP CONSTRAINT IF EXISTS ck_quality_result_rule_set_content_sha256,
    DROP CONSTRAINT IF EXISTS ck_quality_assessment_snapshot_complete,
    DROP COLUMN IF EXISTS evaluator_version,
    DROP COLUMN IF EXISTS evaluator_name,
    DROP COLUMN IF EXISTS rule_set_content,
    DROP COLUMN IF EXISTS rule_set_content_sha256;
