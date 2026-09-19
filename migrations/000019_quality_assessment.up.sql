-- QualityResult remains the storage/API compatibility name. These columns
-- promote it to an immutable QualityAssessment by recording exactly what was
-- evaluated, not merely where the current policy file happens to live.
ALTER TABLE quality_result
    ADD COLUMN rule_set_content_sha256 varchar(64),
    ADD COLUMN rule_set_content text,
    ADD COLUMN evaluator_name varchar(128),
    ADD COLUMN evaluator_version varchar(64);

ALTER TABLE quality_result
    ADD CONSTRAINT ck_quality_result_rule_set_content_sha256
        CHECK (rule_set_content_sha256 IS NULL OR rule_set_content_sha256 ~ '^[0-9a-f]{64}$'),
    ADD CONSTRAINT ck_quality_result_rule_set_content_pair
        CHECK ((rule_set_content IS NULL) = (rule_set_content_sha256 IS NULL));

-- NOT VALID preserves pre-019 legacy rows, while PostgreSQL still applies the
-- check to every new row and every later UPDATE. Legacy rows are frozen by the
-- existing parent immutability trigger and cannot become new assessments.
ALTER TABLE quality_result
    ADD CONSTRAINT ck_quality_assessment_snapshot_complete
    CHECK (
        rule_set_content IS NOT NULL
        AND rule_set_content_sha256 IS NOT NULL
        AND encode(digest(convert_to(rule_set_content, 'UTF8'), 'sha256'), 'hex') = rule_set_content_sha256
        AND NULLIF(btrim(evaluator_name), '') IS NOT NULL
        AND NULLIF(btrim(evaluator_version), '') IS NOT NULL
    ) NOT VALID;

CREATE INDEX idx_quality_result_dataset_version_history
    ON quality_result(dataset_version_id, created_at DESC, id DESC);

-- Findings are part of the assessment fact. Protecting only the parent row
-- would still allow a direct SQL writer to rewrite the assessment's meaning.
CREATE OR REPLACE FUNCTION prevent_quality_assessment_child_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'quality assessment findings are historical and immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_quality_finding_immutable
BEFORE UPDATE OR DELETE ON quality_finding
FOR EACH ROW EXECUTE FUNCTION prevent_quality_assessment_child_mutation();
