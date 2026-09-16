CREATE TABLE data_authorization (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL,
    code            varchar(64) NOT NULL,
    grantor_ref     varchar(255) NOT NULL,
    grantee_ref     varchar(255) NOT NULL,
    purpose         varchar(128) NOT NULL,
    status          varchar(32) NOT NULL DEFAULT 'DRAFT',
    valid_from      timestamptz,
    valid_to        timestamptz,
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      uuid,
    CONSTRAINT uq_authorization_workspace_code UNIQUE(workspace_id, code),
    CONSTRAINT ck_authorization_status CHECK(status IN (
        'DRAFT','REVIEWING','APPROVED','ACTIVE','SUSPENDED','REVOKED','EXPIRED','REJECTED'
    )),
    CONSTRAINT ck_authorization_validity CHECK(valid_to IS NULL OR valid_from IS NULL OR valid_to > valid_from)
);

CREATE INDEX idx_authorization_workspace_status ON data_authorization(workspace_id, status);
CREATE INDEX idx_authorization_valid_to ON data_authorization(valid_to) WHERE status='ACTIVE' AND valid_to IS NOT NULL;

CREATE TABLE authorization_resource (
    id                uuid PRIMARY KEY,
    authorization_id  uuid NOT NULL REFERENCES data_authorization(id),
    data_resource_id  uuid NOT NULL REFERENCES data_resource(id),
    actions           text[] NOT NULL DEFAULT ARRAY[]::text[],
    scope             jsonb NOT NULL DEFAULT '{}'::jsonb,
    raw_export_allowed boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_authorization_resource UNIQUE(authorization_id, data_resource_id)
);

CREATE INDEX idx_authorization_resource_resource ON authorization_resource(data_resource_id);

CREATE TABLE rights_snapshot (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL,
    product_release_id  uuid REFERENCES product_release(id),
    purpose             varchar(128) NOT NULL,
    consumer_ref        varchar(255),
    as_of               timestamptz NOT NULL,
    manifest            jsonb NOT NULL,
    root_hash           varchar(64) NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid
);

CREATE INDEX idx_rights_snapshot_release ON rights_snapshot(product_release_id);

CREATE TABLE rights_snapshot_authorization (
    rights_snapshot_id  uuid NOT NULL REFERENCES rights_snapshot(id),
    authorization_id    uuid NOT NULL REFERENCES data_authorization(id),
    PRIMARY KEY(rights_snapshot_id, authorization_id)
);

CREATE TABLE data_contract (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL,
    code            varchar(64) NOT NULL,
    name            varchar(255) NOT NULL,
    product_code    varchar(64),
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid,
    CONSTRAINT uq_data_contract_workspace_code UNIQUE(workspace_id, code)
);

CREATE TABLE contract_version (
    id                  uuid PRIMARY KEY,
    contract_id         uuid NOT NULL REFERENCES data_contract(id),
    major_version       integer NOT NULL,
    minor_version       integer NOT NULL,
    patch_version       integer NOT NULL DEFAULT 0,
    status              varchar(32) NOT NULL DEFAULT 'DRAFT',
    document            jsonb NOT NULL,
    source_ref          varchar(1024),
    source_sha256       varchar(64) NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid,
    published_at        timestamptz,
    published_by        uuid,
    CONSTRAINT uq_contract_semver UNIQUE(contract_id, major_version, minor_version, patch_version),
    CONSTRAINT ck_contract_version_status CHECK(status IN ('DRAFT','PUBLISHED','DEPRECATED')),
    CONSTRAINT ck_contract_version_numbers CHECK(major_version >= 0 AND minor_version >= 0 AND patch_version >= 0)
);

CREATE INDEX idx_contract_version_contract ON contract_version(contract_id, major_version DESC, minor_version DESC, patch_version DESC);

ALTER TABLE product_version
    ADD CONSTRAINT fk_product_version_contract_version
    FOREIGN KEY (contract_version_id) REFERENCES contract_version(id);

ALTER TABLE product_release
    ADD CONSTRAINT fk_product_release_contract_version
    FOREIGN KEY (contract_version_id) REFERENCES contract_version(id);

ALTER TABLE product_release
    ADD CONSTRAINT fk_product_release_rights_snapshot
    FOREIGN KEY (rights_snapshot_id) REFERENCES rights_snapshot(id);

CREATE OR REPLACE FUNCTION guard_contract_version_immutability()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'contract_version is historical and cannot be deleted';
    END IF;

    IF OLD.contract_id IS DISTINCT FROM NEW.contract_id OR
       OLD.major_version IS DISTINCT FROM NEW.major_version OR
       OLD.minor_version IS DISTINCT FROM NEW.minor_version OR
       OLD.patch_version IS DISTINCT FROM NEW.patch_version OR
       OLD.document IS DISTINCT FROM NEW.document OR
       OLD.source_ref IS DISTINCT FROM NEW.source_ref OR
       OLD.source_sha256 IS DISTINCT FROM NEW.source_sha256 OR
       OLD.created_at IS DISTINCT FROM NEW.created_at OR
       OLD.created_by IS DISTINCT FROM NEW.created_by THEN
        RAISE EXCEPTION 'contract_version content is immutable; create a new version instead';
    END IF;

    IF OLD.status='PUBLISHED' AND NEW.status NOT IN ('PUBLISHED','DEPRECATED') THEN
        RAISE EXCEPTION 'PUBLISHED contract_version cannot transition to %', NEW.status;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_contract_version_immutability
BEFORE UPDATE OR DELETE ON contract_version
FOR EACH ROW EXECUTE FUNCTION guard_contract_version_immutability();

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
