CREATE TABLE data_product (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL,
    project_id          uuid,
    use_case_id         uuid,
    code                varchar(64) NOT NULL,
    name                varchar(255) NOT NULL,
    description         text,
    domain_code         varchar(128),
    owner_id            uuid,
    lifecycle_status    varchar(32) NOT NULL DEFAULT 'DRAFT',
    health_status       varchar(32) NOT NULL DEFAULT 'UNKNOWN',
    current_version_id  uuid,
    latest_release_id   uuid,
    metadata            jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    updated_by          uuid,
    deleted_at          timestamptz,
    CONSTRAINT uq_data_product_workspace_code UNIQUE(workspace_id, code),
    CONSTRAINT ck_data_product_lifecycle CHECK(lifecycle_status IN (
        'DRAFT','DESIGNING','DEVELOPING','TESTING','READY','PUBLISHED','ACTIVE',
        'SUSPENDED','DEPRECATED','RETIRED'
    )),
    CONSTRAINT ck_data_product_health CHECK(health_status IN ('UNKNOWN','HEALTHY','DEGRADED','UNHEALTHY'))
);

CREATE INDEX idx_data_product_workspace ON data_product(workspace_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_data_product_status ON data_product(workspace_id, lifecycle_status) WHERE deleted_at IS NULL;

CREATE TABLE product_version (
    id                    uuid PRIMARY KEY,
    product_id            uuid NOT NULL REFERENCES data_product(id),
    major_version         integer NOT NULL,
    minor_version         integer NOT NULL,
    patch_version         integer NOT NULL DEFAULT 0,
    workflow_version_id   uuid REFERENCES workflow_version(id),
    contract_version_id   uuid,
    entity_policy_ref     varchar(512),
    indicator_set_ref     varchar(512),
    definition_snapshot   jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at            timestamptz NOT NULL DEFAULT now(),
    created_by            uuid,
    CONSTRAINT uq_product_semver UNIQUE(product_id, major_version, minor_version, patch_version),
    CONSTRAINT ck_product_version_numbers CHECK(major_version >= 0 AND minor_version >= 0 AND patch_version >= 0)
);

CREATE INDEX idx_product_version_product ON product_version(product_id, major_version DESC, minor_version DESC, patch_version DESC);

CREATE TABLE product_asset (
    id                  uuid PRIMARY KEY,
    product_version_id  uuid NOT NULL REFERENCES product_version(id),
    asset_type          varchar(32) NOT NULL,
    name                varchar(255) NOT NULL,
    dataset_id          uuid REFERENCES dataset(id),
    external_ref        varchar(1024),
    delivery_config     jsonb NOT NULL DEFAULT '{}'::jsonb,
    schema_snapshot     jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_product_asset_type CHECK(asset_type IN (
        'DATASET','API','REPORT','DASHBOARD','INDICATOR_SERVICE','MODEL_RESULT','SANDBOX'
    ))
);

CREATE INDEX idx_product_asset_version ON product_asset(product_version_id);

CREATE TABLE product_release (
    id                    uuid PRIMARY KEY,
    product_id            uuid NOT NULL REFERENCES data_product(id),
    product_version_id    uuid NOT NULL REFERENCES product_version(id),
    release_no            varchar(64) NOT NULL,
    status                varchar(32) NOT NULL DEFAULT 'DRAFT',
    contract_version_id   uuid,
    rights_snapshot_id    uuid,
    quality_result_id     uuid,
    compliance_result_id  uuid,
    evidence_snapshot_id  uuid,
    release_notes         text,
    metadata              jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at            timestamptz NOT NULL DEFAULT now(),
    created_by            uuid,
    released_at           timestamptz,
    released_by           uuid,
    CONSTRAINT uq_product_release_no UNIQUE(product_id, release_no),
    CONSTRAINT ck_product_release_status CHECK(status IN (
        'DRAFT','VALIDATING','READY','PUBLISHED','SUSPENDED','WITHDRAWN','FAILED'
    ))
);

CREATE INDEX idx_product_release_product ON product_release(product_id, created_at DESC);
CREATE INDEX idx_product_release_status ON product_release(status, created_at DESC);

CREATE TABLE product_release_dataset (
    release_id          uuid NOT NULL REFERENCES product_release(id),
    dataset_version_id  uuid NOT NULL REFERENCES dataset_version(id),
    role                varchar(32) NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(release_id, dataset_version_id, role),
    CONSTRAINT ck_product_release_dataset_role CHECK(role IN ('PRIMARY','INPUT','OUTPUT','SUPPORTING'))
);

CREATE INDEX idx_product_release_dataset_version ON product_release_dataset(dataset_version_id);

ALTER TABLE data_product
    ADD CONSTRAINT fk_data_product_current_version
    FOREIGN KEY (current_version_id) REFERENCES product_version(id);

ALTER TABLE data_product
    ADD CONSTRAINT fk_data_product_latest_release
    FOREIGN KEY (latest_release_id) REFERENCES product_release(id);

CREATE OR REPLACE FUNCTION prevent_product_version_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'product_version is immutable; create a new version instead';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_product_version_immutable_update
BEFORE UPDATE ON product_version
FOR EACH ROW EXECUTE FUNCTION prevent_product_version_mutation();

CREATE TRIGGER trg_product_version_immutable_delete
BEFORE DELETE ON product_version
FOR EACH ROW EXECUTE FUNCTION prevent_product_version_mutation();

CREATE OR REPLACE FUNCTION prevent_product_asset_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'product_asset belongs to an immutable ProductVersion';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_product_asset_immutable_update
BEFORE UPDATE ON product_asset
FOR EACH ROW EXECUTE FUNCTION prevent_product_asset_mutation();

CREATE TRIGGER trg_product_asset_immutable_delete
BEFORE DELETE ON product_asset
FOR EACH ROW EXECUTE FUNCTION prevent_product_asset_mutation();

CREATE OR REPLACE FUNCTION guard_product_release_history()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'product_release is historical and cannot be deleted';
    END IF;

    IF OLD.status IN ('PUBLISHED','WITHDRAWN') THEN
        RAISE EXCEPTION 'product_release in status % is immutable', OLD.status;
    END IF;

    IF OLD.product_id IS DISTINCT FROM NEW.product_id OR
       OLD.product_version_id IS DISTINCT FROM NEW.product_version_id OR
       OLD.release_no IS DISTINCT FROM NEW.release_no THEN
        RAISE EXCEPTION 'product_release identity is immutable';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_product_release_history
BEFORE UPDATE OR DELETE ON product_release
FOR EACH ROW EXECUTE FUNCTION guard_product_release_history();
