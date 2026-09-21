ALTER TABLE effective_rights_action_provenance
    ADD COLUMN binding_id uuid REFERENCES authorization_provenance_binding(id);

CREATE INDEX idx_effective_rights_action_provenance_binding
    ON effective_rights_action_provenance(binding_id)
    WHERE binding_id IS NOT NULL;
