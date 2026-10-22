-- Parent-first, matching Prepare/dispatch. Locking attempt first could deadlock
-- with an admitted writer holding publication/binding before its child INSERT.
LOCK TABLE external_identity_binding,product_release,external_identifier,external_publication,external_publication_attempt,external_provider_call,external_provider_dispatch,external_provider_observation,external_publication_attempt_resolution,cost_allocation,external_publication_transition IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM external_publication_attempt) OR EXISTS(SELECT 1 FROM external_provider_call) OR EXISTS(SELECT 1 FROM external_publication WHERE status IN ('REGISTERED','PUBLISHED')) THEN RAISE EXCEPTION 'external execution history prevents destructive down'; END IF;
END $$;
ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation DROP COLUMN external_provider_call_id;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK(
 num_nonnulls(delivery_operation_id,quality_assessment_id,quality_assessment_attempt_id,
 rights_declaration_id,rights_declaration_verification_id,rights_declaration_disposition_id,
 authorization_provenance_binding_id,authorization_provenance_binding_disposition_id,
 effective_rights_snapshot_id,dataset_certification_id,certification_disposition_id,
 annotation_review_attempt_id,annotation_engine_attempt_id,ingestion_invocation_id)=1);
ALTER TABLE external_publication_transition DROP CONSTRAINT external_publication_transition_check;
ALTER TABLE external_publication_transition DROP COLUMN provider_observation_id;
ALTER TABLE external_publication_transition ADD CONSTRAINT external_publication_transition_check CHECK((revision=0 AND from_status='' AND to_status='DRAFT') OR (revision=1 AND from_status='DRAFT' AND to_status='SUBMITTING'));
ALTER TABLE external_publication DROP CONSTRAINT external_publication_status_check;
ALTER TABLE external_publication DROP CONSTRAINT external_publication_revision_check;
ALTER TABLE external_publication ADD CONSTRAINT external_publication_status_check CHECK(status IN ('DRAFT','SUBMITTING'));
ALTER TABLE external_publication ADD CONSTRAINT external_publication_revision_check CHECK(revision IN (0,1));
DROP TABLE external_publication_attempt_resolution,external_provider_observation,external_provider_dispatch,external_provider_call,external_publication_attempt;
DROP FUNCTION guard_external_execution_reference();
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
CREATE OR REPLACE FUNCTION guard_external_publication_projection() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' THEN
    RAISE EXCEPTION 'external publication cannot be deleted' USING ERRCODE='23514';
  END IF;
  IF TG_OP='INSERT' THEN
    IF NEW.status<>'DRAFT' OR NEW.revision<>0 THEN
      RAISE EXCEPTION 'external publication starts DRAFT' USING ERRCODE='23514';
    END IF;
  ELSE
    IF (to_jsonb(NEW)-'status'-'revision') IS DISTINCT FROM (to_jsonb(OLD)-'status'-'revision') OR
       OLD.status<>'DRAFT' OR NEW.status<>'SUBMITTING' OR OLD.revision<>0 OR NEW.revision<>1 THEN
      RAISE EXCEPTION 'external publication permits only explicit DRAFT to SUBMITTING' USING ERRCODE='23514';
    END IF;
    -- Parent lock is already owned by UPDATE. Application locks identity first;
    -- direct DB projection writes must not wait on identity in reverse order.
    PERFORM 1 FROM external_identity_binding WHERE id=NEW.identity_binding_id FOR UPDATE NOWAIT;
    IF EXISTS(SELECT 1 FROM external_identity_binding_disposition WHERE binding_id=NEW.identity_binding_id) THEN
      RAISE EXCEPTION 'external publication identity invalidated' USING ERRCODE='23514';
    END IF;
  END IF;
  RETURN NEW;
END $$;
CREATE OR REPLACE FUNCTION guard_external_transition() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE p external_publication; expected_event text;
BEGIN
  SELECT * INTO p FROM external_publication WHERE id=NEW.publication_id FOR UPDATE;
  IF p.id IS NULL OR p.workspace_id<>NEW.workspace_id OR p.revision<>NEW.revision OR p.status<>NEW.to_status OR
     NOT EXISTS(SELECT 1 FROM evidence WHERE id=NEW.evidence_id AND workspace_id=p.workspace_id) THEN
    RAISE EXCEPTION 'external transition projection/evidence mismatch' USING ERRCODE='23514';
  END IF;
  expected_event:=CASE NEW.to_status WHEN 'DRAFT' THEN 'ExternalPublicationCreated' ELSE 'ExternalPublicationRequested' END;
  IF NOT EXISTS(SELECT 1 FROM outbox_event WHERE id=NEW.event_id AND aggregate_id=p.id AND
    aggregate_type='ExternalPublication' AND event_type=expected_event AND
    payload->>'workspaceId'=p.workspace_id::text AND payload->>'revision'=NEW.revision::text AND
    payload->>'factId'=NEW.id::text AND payload->>'actorId'=NEW.created_by::text AND
    payload->>'principalRef'=NEW.principal_ref) THEN
    RAISE EXCEPTION 'external transition event mismatch' USING ERRCODE='23514';
  END IF;
  RETURN NEW;
END $$;
