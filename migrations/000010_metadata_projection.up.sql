CREATE TABLE resource_binding (
    id                uuid PRIMARY KEY,
    resource_id       uuid NOT NULL REFERENCES data_resource(id),
    provider          varchar(64) NOT NULL,
    entity_type       varchar(64) NOT NULL,
    external_id       varchar(255),
    external_fqn      varchar(1024) NOT NULL,
    binding_metadata  jsonb NOT NULL DEFAULT '{}'::jsonb,
    is_primary        boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_resource_binding_provider_entity UNIQUE(provider, entity_type, external_fqn),
    CONSTRAINT uq_resource_binding_resource_fqn UNIQUE(resource_id, provider, external_fqn)
);

CREATE INDEX idx_resource_binding_resource ON resource_binding(resource_id, provider);
CREATE INDEX idx_resource_binding_external_id ON resource_binding(provider, external_id) WHERE external_id IS NOT NULL;

CREATE TABLE governance_projection (
    id                uuid PRIMARY KEY,
    workspace_id      uuid NOT NULL,
    provider          varchar(64) NOT NULL,
    object_type       varchar(64) NOT NULL,
    object_id         uuid NOT NULL,
    source_event_id   uuid,
    external_id       varchar(255),
    external_fqn      varchar(1024),
    status            varchar(16) NOT NULL DEFAULT 'PENDING',
    attempts          integer NOT NULL DEFAULT 0,
    last_error        text,
    metadata          jsonb NOT NULL DEFAULT '{}'::jsonb,
    projected_at      timestamptz,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_governance_projection_object UNIQUE(provider, object_type, object_id),
    CONSTRAINT ck_governance_projection_status CHECK(status IN ('PENDING','SUCCEEDED','FAILED')),
    CONSTRAINT ck_governance_projection_attempts CHECK(attempts >= 0)
);

CREATE INDEX idx_governance_projection_workspace ON governance_projection(workspace_id, provider, status);
CREATE INDEX idx_governance_projection_source_event ON governance_projection(source_event_id) WHERE source_event_id IS NOT NULL;
