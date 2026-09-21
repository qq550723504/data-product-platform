-- Persist the physical quality-engine attempt before evaluation starts. The
-- attempt is a typed cost subject so a failed evaluation still has an
-- auditable CostEvent and allocation even when no QualityAssessment exists.
CREATE TABLE quality_assessment_attempt (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL,
    dataset_version_id  uuid NOT NULL REFERENCES dataset_version(id),
    rule_set_ref        varchar(512) NOT NULL,
    started_at          timestamptz NOT NULL,
    lease_expires_at    timestamptz NOT NULL,
    created_by          uuid
);

CREATE TABLE quality_assessment_attempt_outcome (
    id                  uuid PRIMARY KEY,
    attempt_id          uuid NOT NULL REFERENCES quality_assessment_attempt(id),
    assessment_id       uuid REFERENCES quality_result(id),
    outcome             varchar(16) NOT NULL,
    error_message       text,
    occurred_at         timestamptz NOT NULL,
    CONSTRAINT uq_quality_assessment_attempt_outcome UNIQUE (attempt_id),
    CONSTRAINT ck_quality_assessment_attempt_outcome CHECK (
        (outcome = 'SUCCEEDED' AND assessment_id IS NOT NULL AND error_message IS NULL)
        OR
        (outcome = 'FAILED' AND assessment_id IS NULL)
    )
);

CREATE INDEX idx_quality_assessment_attempt_workspace
    ON quality_assessment_attempt(workspace_id, started_at, id);

CREATE INDEX idx_quality_assessment_attempt_dataset_version
    ON quality_assessment_attempt(dataset_version_id, started_at, id);

CREATE INDEX idx_quality_assessment_attempt_lease
    ON quality_assessment_attempt(lease_expires_at, id);

CREATE INDEX idx_quality_assessment_attempt_outcome_assessment
    ON quality_assessment_attempt_outcome(assessment_id, occurred_at, id)
    WHERE assessment_id IS NOT NULL;

CREATE OR REPLACE FUNCTION prevent_quality_assessment_attempt_history_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only quality assessment attempt history', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_quality_assessment_attempt_immutable
BEFORE UPDATE OR DELETE ON quality_assessment_attempt
FOR EACH ROW EXECUTE FUNCTION prevent_quality_assessment_attempt_history_mutation();

CREATE TRIGGER trg_quality_assessment_attempt_outcome_immutable
BEFORE UPDATE OR DELETE ON quality_assessment_attempt_outcome
FOR EACH ROW EXECUTE FUNCTION prevent_quality_assessment_attempt_history_mutation();

ALTER TABLE cost_allocation
    ADD COLUMN quality_assessment_attempt_id uuid REFERENCES quality_assessment_attempt(id);

ALTER TABLE cost_allocation
    DROP CONSTRAINT ck_cost_allocation_single_subject;

ALTER TABLE cost_allocation
    ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
        (CASE WHEN delivery_operation_id IS NOT NULL THEN 1 ELSE 0 END) +
        (CASE WHEN quality_assessment_id IS NOT NULL THEN 1 ELSE 0 END) +
        (CASE WHEN quality_assessment_attempt_id IS NOT NULL THEN 1 ELSE 0 END) = 1
    );

CREATE INDEX idx_cost_allocation_quality_assessment_attempt
    ON cost_allocation(quality_assessment_attempt_id, created_at)
    WHERE quality_assessment_attempt_id IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace()
RETURNS trigger AS $$
DECLARE
    event_workspace uuid;
    subject_workspace uuid;
BEGIN
    SELECT workspace_id
      INTO event_workspace
      FROM cost_event
     WHERE id = NEW.cost_event_id;

    IF NEW.delivery_operation_id IS NOT NULL THEN
        SELECT workspace_id
          INTO subject_workspace
          FROM delivery_operation
         WHERE id = NEW.delivery_operation_id;
    ELSIF NEW.quality_assessment_id IS NOT NULL THEN
        SELECT workspace_id
          INTO subject_workspace
          FROM quality_result
         WHERE id = NEW.quality_assessment_id;
    ELSE
        SELECT workspace_id
          INTO subject_workspace
          FROM quality_assessment_attempt
         WHERE id = NEW.quality_assessment_attempt_id;
    END IF;

    IF event_workspace IS DISTINCT FROM subject_workspace THEN
        RAISE EXCEPTION 'cost allocation crosses workspace boundary';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
