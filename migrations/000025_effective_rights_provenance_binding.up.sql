ALTER TABLE effective_rights_action_provenance
    ADD COLUMN binding_id uuid REFERENCES authorization_provenance_binding(id);

CREATE INDEX idx_effective_rights_action_provenance_binding
    ON effective_rights_action_provenance(binding_id)
    WHERE binding_id IS NOT NULL;

ALTER TABLE authorization_provenance_binding
    ADD COLUMN activity_id uuid;

CREATE UNIQUE INDEX uq_authorization_provenance_binding_activity
    ON authorization_provenance_binding(workspace_id, activity_id)
    WHERE activity_id IS NOT NULL;

CREATE UNIQUE INDEX uq_grantor_delegation_disposition_activity
    ON grantor_authority_delegation_disposition(chain_id, COALESCE(edge_id, '00000000-0000-0000-0000-000000000000'::uuid), disposition, activity_id)
    WHERE activity_id IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_rights_declaration_child_insert()
RETURNS trigger AS $$
DECLARE
    target_declaration_id uuid;
    verified boolean;
BEGIN
    target_declaration_id := NEW.declaration_id;
    IF TG_TABLE_NAME IN ('rights_declaration_purpose','rights_declaration_scope') THEN
        SELECT p.declaration_id INTO target_declaration_id
        FROM rights_declaration_permission p
        WHERE p.id=NEW.permission_id;
        IF target_declaration_id IS NULL OR target_declaration_id IS DISTINCT FROM NEW.declaration_id THEN
            RAISE EXCEPTION 'rights declaration child does not match permission declaration';
        END IF;
    END IF;
    PERFORM 1 FROM rights_declaration WHERE id=target_declaration_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'rights declaration does not exist';
    END IF;
    SELECT EXISTS (
        SELECT 1 FROM rights_declaration_verification v
        WHERE v.declaration_id=target_declaration_id
    ) INTO verified;
    IF verified THEN
        RAISE EXCEPTION 'verified rights declaration children are immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_rights_declaration_party_insert_guard
BEFORE INSERT ON rights_declaration_party
FOR EACH ROW EXECUTE FUNCTION guard_rights_declaration_child_insert();
CREATE TRIGGER trg_rights_declaration_evidence_insert_guard
BEFORE INSERT ON rights_declaration_evidence
FOR EACH ROW EXECUTE FUNCTION guard_rights_declaration_child_insert();
CREATE TRIGGER trg_rights_declaration_permission_insert_guard
BEFORE INSERT ON rights_declaration_permission
FOR EACH ROW EXECUTE FUNCTION guard_rights_declaration_child_insert();
CREATE TRIGGER trg_rights_declaration_purpose_insert_guard
BEFORE INSERT ON rights_declaration_purpose
FOR EACH ROW EXECUTE FUNCTION guard_rights_declaration_child_insert();
CREATE TRIGGER trg_rights_declaration_scope_insert_guard
BEFORE INSERT ON rights_declaration_scope
FOR EACH ROW EXECUTE FUNCTION guard_rights_declaration_child_insert();
