LOCK TABLE rights_declaration, rights_declaration_verification,
    rights_declaration_disposition, authorization_provenance_binding,
    authorization_provenance_binding_disposition, grantor_authority_delegation_chain,
    grantor_authority_delegation_edge, grantor_authority_delegation_disposition,
    effective_rights_snapshot, effective_rights_input, effective_rights_action,
    effective_rights_action_provenance,
    rights_snapshot_declaration, rights_snapshot_provenance_binding, cost_allocation
    IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM rights_declaration LIMIT 1)
       OR EXISTS (SELECT 1 FROM authorization_provenance_binding LIMIT 1)
       OR EXISTS (SELECT 1 FROM grantor_authority_delegation_chain LIMIT 1)
       OR EXISTS (SELECT 1 FROM effective_rights_snapshot LIMIT 1)
       OR EXISTS (SELECT 1 FROM cost_allocation WHERE rights_declaration_id IS NOT NULL
           OR rights_declaration_verification_id IS NOT NULL
           OR rights_declaration_disposition_id IS NOT NULL
           OR authorization_provenance_binding_id IS NOT NULL
           OR authorization_provenance_binding_disposition_id IS NOT NULL
           OR effective_rights_snapshot_id IS NOT NULL LIMIT 1) THEN
        RAISE EXCEPTION 'cannot roll back #137 while rights history exists';
    END IF;
END $$;

DROP TRIGGER IF EXISTS trg_delegation_edge_immutable ON grantor_authority_delegation_edge;
DROP TRIGGER IF EXISTS trg_delegation_chain_immutable ON grantor_authority_delegation_chain;
DROP TRIGGER IF EXISTS trg_delegation_disposition_immutable ON grantor_authority_delegation_disposition;
DROP TRIGGER IF EXISTS trg_binding_disposition_immutable ON authorization_provenance_binding_disposition;
DROP TRIGGER IF EXISTS trg_binding_immutable ON authorization_provenance_binding;
DROP TRIGGER IF EXISTS trg_rights_declaration_disposition_immutable ON rights_declaration_disposition;
DROP TRIGGER IF EXISTS trg_rights_declaration_verification_immutable ON rights_declaration_verification;
DROP TRIGGER IF EXISTS trg_rights_declaration_scope_immutable ON rights_declaration_scope;
DROP TRIGGER IF EXISTS trg_rights_declaration_purpose_immutable ON rights_declaration_purpose;
DROP TRIGGER IF EXISTS trg_rights_declaration_permission_immutable ON rights_declaration_permission;
DROP TRIGGER IF EXISTS trg_rights_declaration_evidence_immutable ON rights_declaration_evidence;
DROP TRIGGER IF EXISTS trg_rights_declaration_party_immutable ON rights_declaration_party;
DROP TRIGGER IF EXISTS trg_rights_declaration_immutable ON rights_declaration;
DROP TRIGGER IF EXISTS trg_effective_rights_action_immutable ON effective_rights_action;
DROP TRIGGER IF EXISTS trg_effective_rights_action_provenance_immutable ON effective_rights_action_provenance;
DROP TRIGGER IF EXISTS trg_effective_rights_input_immutable ON effective_rights_input;
DROP TRIGGER IF EXISTS trg_effective_rights_header_immutable ON effective_rights_snapshot;
DROP TRIGGER IF EXISTS trg_rights_snapshot_binding_immutable ON rights_snapshot_provenance_binding;
DROP TRIGGER IF EXISTS trg_rights_snapshot_declaration_immutable ON rights_snapshot_declaration;
DROP TRIGGER IF EXISTS trg_rights_snapshot_authorization_immutable ON rights_snapshot_authorization;
DROP TRIGGER IF EXISTS trg_rights_snapshot_immutable_update ON rights_snapshot;
DROP TRIGGER IF EXISTS trg_rights_snapshot_immutable_delete ON rights_snapshot;
DROP TRIGGER IF EXISTS trg_cost_allocation_rights_workspace ON cost_allocation;

DROP FUNCTION IF EXISTS guard_delegation_edge_mutation();
DROP FUNCTION IF EXISTS guard_delegation_chain_mutation();
DROP FUNCTION IF EXISTS guard_effective_rights_membership_mutation();
DROP FUNCTION IF EXISTS guard_effective_rights_header_mutation();
DROP FUNCTION IF EXISTS guard_rights_snapshot_membership_mutation();
DROP FUNCTION IF EXISTS guard_rights_snapshot_header_status();
DROP FUNCTION IF EXISTS prevent_rights_history_mutation();
DROP FUNCTION IF EXISTS prevent_rights_declaration_mutation();

CREATE OR REPLACE FUNCTION prevent_rights_snapshot_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'rights_snapshot is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rights_snapshot_immutable_update
BEFORE UPDATE ON rights_snapshot
FOR EACH ROW EXECUTE FUNCTION prevent_rights_snapshot_mutation();

CREATE TRIGGER trg_rights_snapshot_immutable_delete
BEFORE DELETE ON rights_snapshot
FOR EACH ROW EXECUTE FUNCTION prevent_rights_snapshot_mutation();

ALTER TABLE cost_allocation
    DROP CONSTRAINT IF EXISTS ck_cost_allocation_single_subject,
    DROP COLUMN IF EXISTS effective_rights_snapshot_id,
    DROP COLUMN IF EXISTS authorization_provenance_binding_disposition_id,
    DROP COLUMN IF EXISTS authorization_provenance_binding_id,
    DROP COLUMN IF EXISTS rights_declaration_disposition_id,
    DROP COLUMN IF EXISTS rights_declaration_verification_id,
    DROP COLUMN IF EXISTS rights_declaration_id;

ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
    (CASE WHEN delivery_operation_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_attempt_id IS NOT NULL THEN 1 ELSE 0 END) = 1
);

CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace()
RETURNS trigger AS $$
DECLARE event_workspace uuid; subject_workspace uuid;
BEGIN
    SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;
    IF NEW.delivery_operation_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM delivery_operation WHERE id=NEW.delivery_operation_id;
    ELSIF NEW.quality_assessment_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM quality_result WHERE id=NEW.quality_assessment_id;
    ELSE
        SELECT workspace_id INTO subject_workspace FROM quality_assessment_attempt WHERE id=NEW.quality_assessment_attempt_id;
    END IF;
    IF event_workspace IS DISTINCT FROM subject_workspace THEN RAISE EXCEPTION 'cost allocation crosses workspace boundary'; END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TABLE IF EXISTS effective_rights_action_provenance;
DROP TABLE IF EXISTS effective_rights_action;
DROP TABLE IF EXISTS effective_rights_input;
DROP TABLE IF EXISTS effective_rights_snapshot;
DROP TABLE IF EXISTS rights_snapshot_provenance_binding;
DROP TABLE IF EXISTS rights_snapshot_declaration;
DROP TABLE IF EXISTS authorization_provenance_binding_disposition;
DROP TABLE IF EXISTS authorization_provenance_binding;
DROP TABLE IF EXISTS grantor_authority_delegation_disposition;
DROP TABLE IF EXISTS grantor_authority_delegation_edge;
DROP TABLE IF EXISTS grantor_authority_delegation_chain;
DROP TABLE IF EXISTS rights_declaration_disposition;
DROP TABLE IF EXISTS rights_declaration_verification;
DROP TABLE IF EXISTS rights_declaration_evidence;
DROP TABLE IF EXISTS rights_declaration_scope;
DROP TABLE IF EXISTS rights_declaration_purpose;
DROP TABLE IF EXISTS rights_declaration_permission;
DROP TABLE IF EXISTS rights_declaration_party;
DROP TABLE IF EXISTS rights_declaration;

ALTER TABLE rights_snapshot DROP CONSTRAINT IF EXISTS ck_rights_snapshot_status;
ALTER TABLE rights_snapshot DROP COLUMN IF EXISTS status;
ALTER TABLE authorization_resource
    DROP CONSTRAINT IF EXISTS ck_authorization_resource_normalized_scope,
    DROP COLUMN IF EXISTS scope_ref,
    DROP COLUMN IF EXISTS scope_type;
DROP INDEX IF EXISTS idx_authorization_resource_scope;
