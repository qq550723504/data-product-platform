-- Historical source facts may never be reopened by rollback.
DO $$
BEGIN
 LOCK TABLE annotation_campaign,annotation_task,annotation_engine_campaign_binding,
 annotation_result,annotation_snapshot,annotation_source_connection_owner,annotation_source_observation,annotation_source_result_binding,
 annotation_source_receipt,annotation_submission_batch,annotation_submission_batch_receipt
 IN ACCESS EXCLUSIVE MODE;
 IF EXISTS(SELECT 1 FROM annotation_source_connection_owner) OR EXISTS(SELECT 1 FROM annotation_source_observation) OR
 EXISTS(SELECT 1 FROM annotation_source_receipt) OR
 EXISTS(SELECT 1 FROM annotation_submission_batch) THEN
 RAISE EXCEPTION 'cannot remove controlled source history'; END IF;
END $$;
DROP TRIGGER annotation_review_batch_complete ON annotation_review_decision;
DROP TRIGGER annotation_snapshot_batch_complete ON annotation_snapshot;
DROP TRIGGER annotation_snapshot_source_freeze ON annotation_snapshot;
DROP TRIGGER annotation_result_source_required ON annotation_result;
DROP TRIGGER annotation_controlled_binding_validate ON annotation_engine_campaign_binding;
DROP TRIGGER annotation_batch_receipt_validate ON annotation_submission_batch_receipt;
DROP TRIGGER annotation_source_validate ON annotation_source_observation;
DROP TRIGGER annotation_source_binding_validate ON annotation_source_result_binding;
DROP TRIGGER annotation_batch_validate ON annotation_submission_batch;
DROP TRIGGER annotation_source_receipt_validate ON annotation_source_receipt;
DROP FUNCTION annotation_validate_source_insert(),annotation_validate_source_binding(),
 annotation_require_source_binding(),annotation_validate_batch(),annotation_require_complete_batch(),
 annotation_validate_controlled_binding(),annotation_validate_batch_receipt(),
 annotation_source_closure_json(uuid),annotation_snapshot_source_freeze(),annotation_validate_source_receipt(),
 annotation_source_is_expected(uuid,uuid,text,bigint),annotation_batch_membership_matches(uuid,uuid[],text[]);
DROP TABLE annotation_source_receipt,annotation_source_result_binding,
 annotation_source_observation,annotation_submission_batch_receipt,annotation_submission_batch,annotation_source_connection_owner;
ALTER TABLE annotation_engine_campaign_binding DROP CONSTRAINT ck_annotation_source_protocol,
 DROP COLUMN admission_protocol,DROP COLUMN connection_id,DROP COLUMN provider_incarnation,
 DROP COLUMN source_commit,DROP COLUMN engine_version,DROP COLUMN image_digest,DROP COLUMN normalizer_version;