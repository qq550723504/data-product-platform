ALTER TABLE compliance_result
    ADD COLUMN assessment_attempt_id uuid;

CREATE UNIQUE INDEX uq_compliance_result_assessment_attempt
    ON compliance_result(assessment_attempt_id)
    WHERE assessment_attempt_id IS NOT NULL;

CREATE INDEX idx_compliance_result_dataset_attempt
    ON compliance_result(dataset_version_id, assessment_attempt_id)
    WHERE assessment_attempt_id IS NOT NULL;
