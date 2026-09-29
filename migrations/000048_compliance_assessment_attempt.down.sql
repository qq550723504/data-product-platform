DROP INDEX IF EXISTS idx_compliance_result_dataset_attempt;
DROP INDEX IF EXISTS uq_compliance_result_assessment_attempt;

ALTER TABLE compliance_result
    DROP COLUMN IF EXISTS assessment_attempt_id;
