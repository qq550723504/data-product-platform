ALTER TABLE compliance_result
    ADD COLUMN assessment_attempt_id uuid;

-- Historical rows predate the attempt contract. Reuse their immutable result ID
-- as a stable recovery identity so every row satisfies the current invariant.
UPDATE compliance_result
SET assessment_attempt_id = id
WHERE assessment_attempt_id IS NULL;

ALTER TABLE compliance_result
    ALTER COLUMN assessment_attempt_id SET NOT NULL;

CREATE UNIQUE INDEX uq_compliance_result_assessment_attempt
    ON compliance_result(assessment_attempt_id);

CREATE INDEX idx_compliance_result_dataset_attempt
    ON compliance_result(dataset_version_id, assessment_attempt_id);
