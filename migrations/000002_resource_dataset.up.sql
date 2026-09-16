CREATE TABLE data_resource (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL,
    project_id          uuid,
    code                varchar(64) NOT NULL,
    name                varchar(255) NOT NULL,
    description         text,
    domain_code         varchar(128),
    resource_type       varchar(32) NOT NULL,
    owner_id            uuid,
    sensitivity_level   varchar(32),
    rights_status       varchar(32) NOT NULL DEFAULT 'UNKNOWN',
    quality_status      varchar(32) NOT NULL DEFAULT 'UNKNOWN',
    lifecycle_status    varchar(32) NOT NULL DEFAULT 'DISCOVERED',
    business_metadata   jsonb NOT NULL DEFAULT '{}'::jsonb,
    extension           jsonb NOT NULL DEFAULT '{}'::jsonb,
    revision            bigint NOT NULL DEFAULT 1,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    updated_by          uuid,
    deleted_at          timestamptz,
    CONSTRAINT uq_data_resource_workspace_code UNIQUE (workspace_id, code),
    CONSTRAINT ck_data_resource_type CHECK (resource_type IN ('TABLE_LIKE', 'STREAM', 'FILE_COLLECTION', 'API', 'DOCUMENT', 'IMAGE_COLLECTION', 'OTHER')),
    CONSTRAINT ck_data_resource_lifecycle CHECK (lifecycle_status IN ('DISCOVERED', 'CATALOGED', 'CLASSIFIED', 'RIGHTS_REVIEWED', 'READY', 'BLOCKED', 'ARCHIVED'))
);

CREATE INDEX idx_data_resource_workspace ON data_resource(workspace_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_data_resource_project ON data_resource(project_id) WHERE deleted_at IS NULL;

CREATE TABLE dataset (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL,
    project_id          uuid,
    code                varchar(64) NOT NULL,
    name                varchar(255) NOT NULL,
    description         text,
    dataset_type        varchar(32) NOT NULL,
    source_resource_id  uuid REFERENCES data_resource(id),
    owner_id            uuid,
    current_version_id  uuid,
    lifecycle_status    varchar(32) NOT NULL DEFAULT 'ACTIVE',
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    updated_by          uuid,
    deleted_at          timestamptz,
    CONSTRAINT uq_dataset_workspace_code UNIQUE (workspace_id, code),
    CONSTRAINT ck_dataset_type CHECK (dataset_type IN ('RAW', 'STANDARDIZED', 'CURATED', 'PRODUCT')),
    CONSTRAINT ck_dataset_lifecycle CHECK (lifecycle_status IN ('ACTIVE', 'DEPRECATED', 'ARCHIVED'))
);

CREATE INDEX idx_dataset_workspace ON dataset(workspace_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_dataset_project ON dataset(project_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_dataset_resource ON dataset(source_resource_id) WHERE deleted_at IS NULL;

CREATE TABLE dataset_version (
    id                        uuid PRIMARY KEY,
    dataset_id                uuid NOT NULL REFERENCES dataset(id),
    version_no                bigint NOT NULL,
    status                    varchar(32) NOT NULL DEFAULT 'CREATED',
    schema_version            varchar(64),
    storage_type              varchar(32),
    storage_uri               text,
    content_type              varchar(128),
    row_count                 bigint,
    byte_size                 bigint,
    checksum_algorithm        varchar(32),
    checksum_value            varchar(256),
    generated_by_execution_id uuid,
    rights_snapshot_id        uuid,
    quality_status            varchar(32),
    compliance_status         varchar(32),
    snapshot_from             timestamptz,
    snapshot_to               timestamptz,
    metadata                  jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at                timestamptz NOT NULL DEFAULT now(),
    created_by                uuid,
    ready_at                  timestamptz,
    invalidated_at            timestamptz,
    invalidation_reason       text,
    CONSTRAINT uq_dataset_version_no UNIQUE (dataset_id, version_no),
    CONSTRAINT ck_dataset_version_status CHECK (status IN ('CREATED', 'PROCESSING', 'READY', 'FAILED', 'INVALID', 'SUPERSEDED')),
    CONSTRAINT ck_dataset_version_storage CHECK (
        (status IN ('CREATED', 'PROCESSING', 'FAILED')) OR
        (status IN ('READY', 'INVALID', 'SUPERSEDED') AND storage_uri IS NOT NULL AND checksum_value IS NOT NULL)
    )
);

ALTER TABLE dataset
    ADD CONSTRAINT fk_dataset_current_version
    FOREIGN KEY (current_version_id) REFERENCES dataset_version(id);

CREATE INDEX idx_dataset_version_dataset ON dataset_version(dataset_id, version_no DESC);
CREATE INDEX idx_dataset_version_status ON dataset_version(status);

CREATE TABLE dataset_version_lineage (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    output_version_id   uuid NOT NULL REFERENCES dataset_version(id),
    input_version_id    uuid NOT NULL REFERENCES dataset_version(id),
    relation_type       varchar(32) NOT NULL DEFAULT 'DERIVED_FROM',
    execution_id        uuid,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_dataset_lineage UNIQUE (output_version_id, input_version_id, relation_type),
    CONSTRAINT ck_dataset_lineage_no_self CHECK (output_version_id <> input_version_id)
);

CREATE OR REPLACE FUNCTION guard_dataset_version_immutability()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'dataset_version is historical and cannot be deleted';
    END IF;

    IF OLD.status IN ('INVALID', 'SUPERSEDED') THEN
        RAISE EXCEPTION 'dataset_version in status % is immutable', OLD.status;
    END IF;

    IF OLD.status = 'READY' THEN
        IF NEW.status NOT IN ('READY', 'INVALID', 'SUPERSEDED') THEN
            RAISE EXCEPTION 'READY dataset_version cannot transition to %', NEW.status;
        END IF;

        IF NEW.dataset_id IS DISTINCT FROM OLD.dataset_id OR
           NEW.version_no IS DISTINCT FROM OLD.version_no OR
           NEW.schema_version IS DISTINCT FROM OLD.schema_version OR
           NEW.storage_type IS DISTINCT FROM OLD.storage_type OR
           NEW.storage_uri IS DISTINCT FROM OLD.storage_uri OR
           NEW.content_type IS DISTINCT FROM OLD.content_type OR
           NEW.row_count IS DISTINCT FROM OLD.row_count OR
           NEW.byte_size IS DISTINCT FROM OLD.byte_size OR
           NEW.checksum_algorithm IS DISTINCT FROM OLD.checksum_algorithm OR
           NEW.checksum_value IS DISTINCT FROM OLD.checksum_value OR
           NEW.generated_by_execution_id IS DISTINCT FROM OLD.generated_by_execution_id OR
           NEW.rights_snapshot_id IS DISTINCT FROM OLD.rights_snapshot_id OR
           NEW.snapshot_from IS DISTINCT FROM OLD.snapshot_from OR
           NEW.snapshot_to IS DISTINCT FROM OLD.snapshot_to OR
           NEW.metadata IS DISTINCT FROM OLD.metadata OR
           NEW.created_at IS DISTINCT FROM OLD.created_at OR
           NEW.created_by IS DISTINCT FROM OLD.created_by OR
           NEW.ready_at IS DISTINCT FROM OLD.ready_at THEN
            RAISE EXCEPTION 'READY dataset_version content is immutable';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dataset_version_immutability
BEFORE UPDATE OR DELETE ON dataset_version
FOR EACH ROW EXECUTE FUNCTION guard_dataset_version_immutability();
