-- #205: every real remote annotation-engine invocation is a typed physical
-- cost subject, including UNKNOWN and failed attempts.
ALTER TABLE cost_allocation
    ADD COLUMN annotation_engine_attempt_id uuid REFERENCES annotation_engine_attempt(id);

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
    (CASE WHEN certification_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN annotation_review_attempt_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN annotation_engine_attempt_id IS NOT NULL THEN 1 ELSE 0 END) = 1
);

CREATE INDEX idx_cost_allocation_annotation_engine_attempt
    ON cost_allocation(annotation_engine_attempt_id, created_at)
    WHERE annotation_engine_attempt_id IS NOT NULL;

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
        SELECT d.workspace_id INTO subject_workspace
          FROM rights_declaration_verification v
          JOIN rights_declaration d ON d.id=v.declaration_id
         WHERE v.id=NEW.rights_declaration_verification_id;
    ELSIF NEW.rights_declaration_disposition_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace
          FROM rights_declaration_disposition x
          JOIN rights_declaration d ON d.id=x.declaration_id
         WHERE x.id=NEW.rights_declaration_disposition_id;
    ELSIF NEW.authorization_provenance_binding_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM authorization_provenance_binding
         WHERE id=NEW.authorization_provenance_binding_id;
    ELSIF NEW.authorization_provenance_binding_disposition_id IS NOT NULL THEN
        SELECT b.workspace_id INTO subject_workspace
          FROM authorization_provenance_binding_disposition x
          JOIN authorization_provenance_binding b ON b.id=x.binding_id
         WHERE x.id=NEW.authorization_provenance_binding_disposition_id;
    ELSIF NEW.effective_rights_snapshot_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM effective_rights_snapshot
         WHERE id=NEW.effective_rights_snapshot_id;
    ELSIF NEW.dataset_certification_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM dataset_certification
         WHERE id=NEW.dataset_certification_id;
    ELSIF NEW.certification_disposition_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM certification_disposition
         WHERE id=NEW.certification_disposition_id;
    ELSIF NEW.annotation_review_attempt_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM annotation_review_attempt
         WHERE id=NEW.annotation_review_attempt_id;
    ELSE
        SELECT workspace_id INTO subject_workspace
          FROM annotation_engine_attempt
         WHERE id=NEW.annotation_engine_attempt_id;
    END IF;

    IF event_workspace IS DISTINCT FROM subject_workspace THEN
        RAISE EXCEPTION 'cost allocation crosses workspace boundary';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
