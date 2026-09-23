-- Removing durable alias reservations would reopen an already accepted
-- replay alias to later reassignment. Refuse downgrade once Annotation history
-- exists, using the same fail-closed lock discipline as 000035/000036.
LOCK TABLE
    annotation_campaign,
    annotation_campaign_command,
    annotation_task,
    annotation_result,
    annotation_result_alias,
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
       OR EXISTS (SELECT 1 FROM annotation_result_alias LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_review_attempt LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_review_decision LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_snapshot LIMIT 1) THEN
        RAISE EXCEPTION 'cannot roll back annotation result aliases while annotation history exists';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_annotation_result_canonical_alias ON annotation_result;
DROP FUNCTION IF EXISTS append_annotation_result_canonical_alias();

DROP TRIGGER IF EXISTS trg_annotation_result_alias_immutable ON annotation_result_alias;
DROP FUNCTION IF EXISTS prevent_annotation_result_alias_mutation();

DROP TRIGGER IF EXISTS trg_annotation_result_alias_insert ON annotation_result_alias;
DROP FUNCTION IF EXISTS validate_annotation_result_alias_insert();

DROP TABLE annotation_result_alias;
