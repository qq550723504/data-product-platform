DROP INDEX IF EXISTS idx_effective_rights_action_provenance_binding;

ALTER TABLE effective_rights_action_provenance
    DROP COLUMN IF EXISTS binding_id;
