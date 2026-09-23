-- Refuse to remove the durable annotation-engine boundary after it has history.
LOCK TABLE
    annotation_engine_operation,
    annotation_engine_attempt,
    annotation_engine_attempt_outcome,
    annotation_engine_campaign_binding,
    annotation_engine_task_binding
IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM annotation_engine_operation LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_engine_attempt LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_engine_attempt_outcome LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_engine_campaign_binding LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_engine_task_binding LIMIT 1) THEN
        RAISE EXCEPTION 'cannot rollback annotation engine durable boundary with history';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_annotation_engine_attempt_outcome_insert ON annotation_engine_attempt_outcome;
DROP FUNCTION IF EXISTS validate_annotation_engine_attempt_outcome_insert();

DROP TRIGGER IF EXISTS trg_annotation_engine_task_binding_immutable ON annotation_engine_task_binding;
DROP TRIGGER IF EXISTS trg_annotation_engine_campaign_binding_immutable ON annotation_engine_campaign_binding;
DROP TRIGGER IF EXISTS trg_annotation_engine_attempt_outcome_immutable ON annotation_engine_attempt_outcome;
DROP TRIGGER IF EXISTS trg_annotation_engine_attempt_immutable ON annotation_engine_attempt;
DROP FUNCTION IF EXISTS prevent_annotation_engine_append_only_mutation();

DROP TRIGGER IF EXISTS trg_annotation_engine_task_binding_insert ON annotation_engine_task_binding;
DROP FUNCTION IF EXISTS validate_annotation_engine_task_binding_insert();

DROP TRIGGER IF EXISTS trg_annotation_engine_campaign_binding_insert ON annotation_engine_campaign_binding;
DROP FUNCTION IF EXISTS validate_annotation_engine_binding_insert();

DROP TRIGGER IF EXISTS trg_annotation_engine_attempt_insert ON annotation_engine_attempt;
DROP FUNCTION IF EXISTS validate_annotation_engine_attempt_insert();

DROP TRIGGER IF EXISTS trg_annotation_engine_operation_delete ON annotation_engine_operation;
DROP FUNCTION IF EXISTS prevent_annotation_engine_operation_delete();

DROP TRIGGER IF EXISTS trg_annotation_engine_operation_update ON annotation_engine_operation;
DROP FUNCTION IF EXISTS guard_annotation_engine_operation_update();

DROP TRIGGER IF EXISTS trg_annotation_engine_operation_insert ON annotation_engine_operation;
DROP FUNCTION IF EXISTS validate_annotation_engine_operation_insert();

DROP TABLE annotation_engine_task_binding;
DROP TABLE annotation_engine_campaign_binding;
DROP TABLE annotation_engine_attempt_outcome;
DROP TABLE annotation_engine_attempt;
DROP TABLE annotation_engine_operation;
