LOCK TABLE quality_assessment_attempt, quality_assessment_attempt_outcome,
    cost_event, cost_allocation
    IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM quality_assessment_attempt LIMIT 1)
       OR EXISTS (SELECT 1 FROM quality_assessment_attempt_outcome LIMIT 1)
       OR EXISTS (SELECT 1 FROM cost_allocation WHERE quality_assessment_attempt_id IS NOT NULL LIMIT 1) THEN
        RAISE EXCEPTION 'refusing destructive rollback of quality assessment attempt cost history';
    END IF;
END
$$;

DROP INDEX IF EXISTS idx_cost_allocation_quality_assessment_attempt;
ALTER TABLE cost_allocation
    DROP CONSTRAINT IF EXISTS ck_cost_allocation_single_subject,
    DROP COLUMN IF EXISTS quality_assessment_attempt_id;

ALTER TABLE cost_allocation
    ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
        (CASE WHEN delivery_operation_id IS NOT NULL THEN 1 ELSE 0 END) +
        (CASE WHEN quality_assessment_id IS NOT NULL THEN 1 ELSE 0 END) = 1
    );

DROP TRIGGER IF EXISTS trg_quality_assessment_attempt_outcome_immutable ON quality_assessment_attempt_outcome;
DROP TRIGGER IF EXISTS trg_quality_assessment_attempt_immutable ON quality_assessment_attempt;
DROP FUNCTION IF EXISTS prevent_quality_assessment_attempt_history_mutation();
DROP TABLE quality_assessment_attempt_outcome;
DROP TABLE quality_assessment_attempt;

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
