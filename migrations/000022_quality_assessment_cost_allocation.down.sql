LOCK TABLE quality_result, cost_event, cost_allocation IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM cost_allocation WHERE quality_assessment_id IS NOT NULL LIMIT 1) THEN
        RAISE EXCEPTION 'refusing destructive rollback of quality assessment cost allocation history';
    END IF;
    IF EXISTS (SELECT 1 FROM cost_allocation WHERE delivery_operation_id IS NULL LIMIT 1) THEN
        RAISE EXCEPTION 'refusing rollback with untyped cost allocation rows';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS trg_cost_allocation_workspace ON cost_allocation;
DROP FUNCTION IF EXISTS guard_cost_allocation_workspace();
DROP INDEX IF EXISTS idx_cost_allocation_quality_assessment;
ALTER TABLE cost_allocation
    DROP CONSTRAINT IF EXISTS ck_cost_allocation_single_subject,
    DROP COLUMN IF EXISTS quality_assessment_id;
ALTER TABLE cost_allocation
    ALTER COLUMN delivery_operation_id SET NOT NULL;
