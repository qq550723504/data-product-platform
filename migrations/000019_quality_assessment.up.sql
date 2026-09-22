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

-- Every QualityAssessment in this pre-production system must satisfy the
-- complete frozen snapshot contract. Development databases should be rebuilt
-- rather than carrying compatibility for hypothetical pre-019 rows.
ALTER TABLE quality_result
    ADD CONSTRAINT ck_quality_assessment_snapshot_complete
    CHECK (
        rule_set_content IS NOT NULL
        AND rule_set_content_sha256 IS NOT NULL
        AND encode(digest(convert_to(rule_set_content, 'UTF8'), 'sha256'), 'hex') = rule_set_content_sha256
        AND NULLIF(btrim(evaluator_name), '') IS NOT NULL
        AND NULLIF(btrim(evaluator_version), '') IS NOT NULL
        AND jsonb_typeof(metrics->'dimensions') = 'object'
    );

CREATE INDEX idx_quality_result_dataset_version_history
    ON quality_result(dataset_version_id, created_at DESC, id DESC);

-- Findings are part of the assessment fact. The deferred FK lets the
-- application insert the child rows before the parent during the one creation
-- transaction. Once the parent exists, the INSERT branch below rejects any
-- later attempt to append findings.
ALTER TABLE quality_finding
    DROP CONSTRAINT IF EXISTS quality_finding_result_id_fkey;

ALTER TABLE quality_finding
    ADD CONSTRAINT fk_quality_finding_result
    FOREIGN KEY (result_id) REFERENCES quality_result(id)
    DEFERRABLE INITIALLY DEFERRED;

CREATE OR REPLACE FUNCTION prevent_quality_assessment_child_mutation()
RETURNS trigger AS $$
BEGIN
	IF TG_OP = 'INSERT' THEN
		IF EXISTS (SELECT 1 FROM quality_result WHERE id = NEW.result_id) THEN
			RAISE EXCEPTION 'quality assessment findings are historical and immutable';
		END IF;
		RETURN NEW;
	END IF;
    RAISE EXCEPTION 'quality assessment findings are historical and immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_quality_finding_immutable
BEFORE INSERT OR UPDATE OR DELETE ON quality_finding
FOR EACH ROW EXECUTE FUNCTION prevent_quality_assessment_child_mutation();
