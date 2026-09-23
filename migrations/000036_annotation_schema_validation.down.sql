-- Refuse to remove schema enforcement once annotation history exists.
-- Lock every annotation write surface before checking emptiness so a concurrent
-- writer cannot commit history between the guard and trigger/function removal.
LOCK TABLE
    annotation_campaign,
    annotation_campaign_command,
    annotation_task,
    annotation_result,
    annotation_review_attempt,
    annotation_review_attempt_outcome,
    annotation_review_decision,
    annotation_snapshot,
    annotation_snapshot_task,
    annotation_snapshot_result,
    annotation_snapshot_decision,
    annotation_snapshot_output
IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM annotation_campaign LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_result LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_review_attempt LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_review_decision LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_snapshot LIMIT 1) THEN
        RAISE EXCEPTION 'cannot roll back annotation schema validation while annotation history exists';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_annotation_snapshot_schema_validation ON annotation_snapshot;
DROP FUNCTION IF EXISTS validate_annotation_snapshot_result_schemas();

DROP TRIGGER IF EXISTS trg_annotation_result_schema_validation ON annotation_result;
DROP FUNCTION IF EXISTS validate_annotation_result_schema_insert();

DROP FUNCTION IF EXISTS annotation_payload_matches_frozen_schema(text, bytea);

DROP FUNCTION IF EXISTS annotation_trim_space(text);
