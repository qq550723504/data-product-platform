-- Extend the typed cost allocation table to cover QualityAssessment.
-- Migration 000021 introduced DeliveryOperation as the first subject; this
-- migration generalizes the subject relation explicitly.

ALTER TABLE cost_allocation
    ALTER COLUMN delivery_operation_id DROP NOT NULL,
    ADD COLUMN quality_assessment_id uuid REFERENCES quality_result(id);

ALTER TABLE cost_allocation
    ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
        (CASE WHEN delivery_operation_id IS NOT NULL THEN 1 ELSE 0 END) +
        (CASE WHEN quality_assessment_id IS NOT NULL THEN 1 ELSE 0 END) = 1
    );

CREATE INDEX idx_cost_allocation_quality_assessment
    ON cost_allocation(quality_assessment_id, created_at)
    WHERE quality_assessment_id IS NOT NULL;

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
    ELSE
        SELECT workspace_id
          INTO subject_workspace
          FROM quality_result
         WHERE id = NEW.quality_assessment_id;
    END IF;

    IF event_workspace IS DISTINCT FROM subject_workspace THEN
        RAISE EXCEPTION 'cost allocation crosses workspace boundary';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_cost_allocation_workspace
BEFORE INSERT ON cost_allocation
FOR EACH ROW EXECUTE FUNCTION guard_cost_allocation_workspace();
