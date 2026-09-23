-- Refuse to reopen immutable annotation history by rollback once any facts exist.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM annotation_campaign LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_review_attempt LIMIT 1)
       OR EXISTS (SELECT 1 FROM annotation_snapshot LIMIT 1) THEN
        RAISE EXCEPTION 'cannot roll back annotation domain while annotation history exists';
    END IF;
END;
$$;

DROP INDEX IF EXISTS idx_cost_allocation_annotation_review_attempt;
ALTER TABLE cost_allocation DROP COLUMN annotation_review_attempt_id;
ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
    (CASE WHEN delivery_operation_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_attempt_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_verification_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN authorization_provenance_binding_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN authorization_provenance_binding_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN effective_rights_snapshot_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN dataset_certification_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN certification_disposition_id IS NOT NULL THEN 1 ELSE 0 END) = 1
);

CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace()
RETURNS trigger AS $$
DECLARE
    event_workspace uuid;
    subject_workspace uuid;
BEGIN
    SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;
    IF NEW.delivery_operation_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM delivery_operation WHERE id=NEW.delivery_operation_id;
    ELSIF NEW.quality_assessment_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM quality_result WHERE id=NEW.quality_assessment_id;
    ELSIF NEW.quality_assessment_attempt_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM quality_assessment_attempt WHERE id=NEW.quality_assessment_attempt_id;
    ELSIF NEW.rights_declaration_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM rights_declaration WHERE id=NEW.rights_declaration_id;
    ELSIF NEW.rights_declaration_verification_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace FROM rights_declaration_verification v JOIN rights_declaration d ON d.id=v.declaration_id WHERE v.id=NEW.rights_declaration_verification_id;
    ELSIF NEW.rights_declaration_disposition_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace FROM rights_declaration_disposition x JOIN rights_declaration d ON d.id=x.declaration_id WHERE x.id=NEW.rights_declaration_disposition_id;
    ELSIF NEW.authorization_provenance_binding_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM authorization_provenance_binding WHERE id=NEW.authorization_provenance_binding_id;
    ELSIF NEW.authorization_provenance_binding_disposition_id IS NOT NULL THEN
        SELECT b.workspace_id INTO subject_workspace FROM authorization_provenance_binding_disposition x JOIN authorization_provenance_binding b ON b.id=x.binding_id WHERE x.id=NEW.authorization_provenance_binding_disposition_id;
    ELSIF NEW.effective_rights_snapshot_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM effective_rights_snapshot WHERE id=NEW.effective_rights_snapshot_id;
    ELSIF NEW.dataset_certification_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM dataset_certification WHERE id=NEW.dataset_certification_id;
    ELSE
        SELECT workspace_id INTO subject_workspace FROM certification_disposition WHERE id=NEW.certification_disposition_id;
    END IF;
    IF event_workspace IS DISTINCT FROM subject_workspace THEN
        RAISE EXCEPTION 'cost allocation crosses workspace boundary';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TABLE annotation_snapshot_output;
DROP TABLE annotation_snapshot_decision;
DROP TABLE annotation_snapshot_result;
DROP TABLE annotation_snapshot_task;
DROP TABLE annotation_snapshot;
ALTER TABLE annotation_task DROP CONSTRAINT fk_annotation_task_current_decision;
DROP TABLE annotation_review_decision;
DROP TABLE annotation_review_attempt_outcome;
DROP TABLE annotation_review_attempt;
DROP TABLE annotation_result;
DROP TABLE annotation_task;
DROP TABLE annotation_campaign;

DROP FUNCTION IF EXISTS require_annotation_snapshot_finalized_on_commit();
DROP FUNCTION IF EXISTS prevent_annotation_snapshot_mutation();
DROP FUNCTION IF EXISTS guard_annotation_snapshot_membership();
DROP FUNCTION IF EXISTS validate_annotation_snapshot_insert();
DROP FUNCTION IF EXISTS project_annotation_review_decision();
DROP FUNCTION IF EXISTS validate_annotation_review_decision_insert();
DROP FUNCTION IF EXISTS validate_annotation_review_attempt_outcome_insert();
DROP FUNCTION IF EXISTS validate_annotation_review_attempt_insert();
DROP FUNCTION IF EXISTS guard_annotation_result_insert();
DROP FUNCTION IF EXISTS guard_annotation_task_update();
DROP FUNCTION IF EXISTS guard_annotation_task_insert();
DROP FUNCTION IF EXISTS guard_annotation_campaign_update();
DROP FUNCTION IF EXISTS validate_annotation_campaign_insert();
DROP FUNCTION IF EXISTS prevent_annotation_append_only_mutation();
