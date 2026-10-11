-- Serialize with all fixture/plan writers before checking for historical facts.
LOCK TABLE lifecycle_fixture_namespace, lifecycle_fixture_copy, lifecycle_disposition_plan,
           lifecycle_disposition_plan_target, lifecycle_content_use_restriction, lifecycle_content_hold,
           lifecycle_content_hold_release, lifecycle_reader_pin, lifecycle_reader_termination,
           lifecycle_disposition_approval, lifecycle_approval_revocation, lifecycle_disposition_gate_evaluation,
           lifecycle_disposition_dependency_evaluation, lifecycle_disposition_retention_evaluation,
           lifecycle_disposition_dispatch, lifecycle_provider_attempt, lifecycle_provider_observation,
           lifecycle_dispatch_release, lifecycle_hold_dispatch_conflict, cost_allocation IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM lifecycle_fixture_namespace) OR
       EXISTS (SELECT 1 FROM lifecycle_fixture_copy) OR EXISTS (SELECT 1 FROM lifecycle_disposition_plan) THEN
        RAISE EXCEPTION 'cannot remove lifecycle fixture or disposition history';
    END IF;
END $$;
DROP TRIGGER lifecycle_cost_attempt ON cost_allocation;
DROP FUNCTION lifecycle_cost_attempt_guard();
ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation DROP COLUMN lifecycle_provider_attempt_id;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
 num_nonnulls(delivery_operation_id,quality_assessment_id,quality_assessment_attempt_id,
 rights_declaration_id,rights_declaration_verification_id,rights_declaration_disposition_id,
 authorization_provenance_binding_id,authorization_provenance_binding_disposition_id,
 effective_rights_snapshot_id,dataset_certification_id,certification_disposition_id,
 annotation_review_attempt_id,annotation_engine_attempt_id,ingestion_invocation_id)=1
);
CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace() RETURNS trigger AS $$
DECLARE event_workspace uuid; subject_workspace uuid;
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
 ELSIF NEW.certification_disposition_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM certification_disposition WHERE id=NEW.certification_disposition_id;
 ELSIF NEW.annotation_review_attempt_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM annotation_review_attempt WHERE id=NEW.annotation_review_attempt_id;
 ELSIF NEW.annotation_engine_attempt_id IS NOT NULL THEN
  SELECT workspace_id INTO subject_workspace FROM annotation_engine_attempt WHERE id=NEW.annotation_engine_attempt_id;
 ELSE
  SELECT workspace_id INTO subject_workspace FROM ingestion_invocation WHERE id=NEW.ingestion_invocation_id;
 END IF;
 IF event_workspace IS DISTINCT FROM subject_workspace THEN RAISE EXCEPTION 'cost allocation crosses workspace boundary'; END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql;
DROP TABLE lifecycle_hold_dispatch_conflict;
DROP TABLE lifecycle_dispatch_release;
DROP TABLE lifecycle_provider_observation;
DROP TABLE lifecycle_provider_attempt;
DROP TABLE lifecycle_disposition_dispatch;
DROP TABLE lifecycle_disposition_dependency_evaluation;
DROP TABLE lifecycle_disposition_retention_evaluation;
DROP TABLE lifecycle_disposition_gate_evaluation;
DROP TABLE lifecycle_approval_revocation;
DROP TABLE lifecycle_disposition_approval;
DROP FUNCTION lifecycle_hold_conflict_guard();
DROP FUNCTION lifecycle_dispatch_guard();
DROP FUNCTION lifecycle_dispatch_complete_guard();
DROP FUNCTION lifecycle_dispatch_release_guard();
DROP FUNCTION lifecycle_observation_guard();
DROP FUNCTION lifecycle_disposition_gate_scope_guard();
DROP FUNCTION lifecycle_gate_inputs_hash(uuid);
DROP FUNCTION lifecycle_gate_finalize_guard();
DROP FUNCTION lifecycle_gate_input_guard();
DROP FUNCTION lifecycle_approval_scope_guard();
DROP TABLE lifecycle_reader_termination;
DROP TABLE lifecycle_reader_pin;
DROP TABLE lifecycle_content_hold_release;
DROP TABLE lifecycle_content_hold;
DROP TABLE lifecycle_content_use_restriction;
DROP FUNCTION lifecycle_termination_scope_guard();
DROP FUNCTION lifecycle_hold_release_scope_guard();
DROP FUNCTION lifecycle_restriction_scope_guard();
DROP TABLE lifecycle_disposition_plan_target;
DROP TABLE lifecycle_disposition_plan;
DROP TABLE lifecycle_fixture_copy;
DROP TABLE lifecycle_fixture_namespace;
DROP FUNCTION lifecycle_plan_finalize_guard();
DROP FUNCTION lifecycle_plan_scope_hash(uuid);
DROP FUNCTION lifecycle_plan_frame(text);
DROP FUNCTION lifecycle_plan_target_guard();
DROP FUNCTION lifecycle_fixture_identity_immutable();
DROP FUNCTION lifecycle_fixture_copy_workspace_guard();
