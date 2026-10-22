-- Catalog names can change; provider-confirmed identity owns the binding.
-- Invalid or duplicate identities fail migration rather than inventing a backfill.
ALTER TABLE resource_binding
    ALTER COLUMN external_id SET NOT NULL,
    ADD CONSTRAINT ck_resource_binding_external_id CHECK (btrim(external_id) <> ''),
    ADD CONSTRAINT uq_resource_binding_provider_identity UNIQUE(provider, entity_type, external_id);
