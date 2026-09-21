DROP INDEX IF EXISTS idx_effective_rights_action_provenance_binding;

DROP TRIGGER IF EXISTS trg_rights_declaration_scope_insert_guard ON rights_declaration_scope;
DROP TRIGGER IF EXISTS trg_rights_declaration_purpose_insert_guard ON rights_declaration_purpose;
DROP TRIGGER IF EXISTS trg_rights_declaration_permission_insert_guard ON rights_declaration_permission;
DROP TRIGGER IF EXISTS trg_rights_declaration_evidence_insert_guard ON rights_declaration_evidence;
DROP TRIGGER IF EXISTS trg_rights_declaration_party_insert_guard ON rights_declaration_party;
DROP FUNCTION IF EXISTS guard_rights_declaration_child_insert();

DROP INDEX IF EXISTS uq_authorization_provenance_binding_activity;

ALTER TABLE authorization_provenance_binding
    DROP COLUMN IF EXISTS activity_id;

ALTER TABLE effective_rights_action_provenance
    DROP COLUMN IF EXISTS binding_id;
