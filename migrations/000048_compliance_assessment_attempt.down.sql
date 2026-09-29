LOCK TABLE compliance_result IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM compliance_result LIMIT 1) THEN
        RAISE EXCEPTION 'cannot rollback compliance assessment attempt identity while compliance_result history exists';
    END IF;
END
$$;

DROP INDEX IF EXISTS idx_compliance_result_dataset_attempt;
DROP INDEX IF EXISTS uq_compliance_result_assessment_attempt;

ALTER TABLE compliance_result
    DROP COLUMN assessment_attempt_id;
