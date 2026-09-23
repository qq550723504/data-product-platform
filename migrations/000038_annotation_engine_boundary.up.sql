-- #205 Annotation Engine durable side-effect boundary.
-- Core owns stable operation identity, provider bindings and physical attempt
-- history. Provider-specific payloads remain behind the adapter.

CREATE TABLE annotation_engine_operation (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    provider                    varchar(64) NOT NULL,
    provider_instance_ref       varchar(255) NOT NULL,
    operation_kind              varchar(32) NOT NULL,
    request_id                  varchar(255) NOT NULL,
    request_fingerprint         varchar(64) NOT NULL,
    payload_manifest            jsonb NOT NULL,
    payload_manifest_hash_payload bytea NOT NULL,
    payload_manifest_sha256     varchar(64) NOT NULL,
    status                      varchar(16) NOT NULL DEFAULT 'PENDING',
    revision                    bigint NOT NULL DEFAULT 1,
    claimed_by                  varchar(255),
    claim_expires_at            timestamptz,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_engine_operation_request
        UNIQUE (workspace_id, provider, provider_instance_ref, request_id),
    CONSTRAINT ck_annotation_engine_operation_kind
        CHECK (operation_kind IN ('ENSURE_CAMPAIGN','SUBMIT_TASKS')),
    CONSTRAINT ck_annotation_engine_operation_status
        CHECK (status IN ('PENDING','SENDING','UNKNOWN','MATCHED','REJECTED','CONFLICT')),
    CONSTRAINT ck_annotation_engine_operation_revision CHECK (revision >= 1),
    CONSTRAINT ck_annotation_engine_operation_provider CHECK (
        length(btrim(provider)) > 0 AND length(btrim(provider_instance_ref)) > 0
    ),
    CONSTRAINT ck_annotation_engine_operation_request CHECK (
        length(btrim(request_id)) > 0
        AND request_fingerprint ~ '^[0-9a-f]{64}$'
        AND payload_manifest_sha256 ~ '^[0-9a-f]{64}$'
        AND convert_from(payload_manifest_hash_payload, 'UTF8')::jsonb = payload_manifest
        AND encode(digest(payload_manifest_hash_payload, 'sha256'), 'hex') = payload_manifest_sha256
    ),
    CONSTRAINT ck_annotation_engine_operation_claim CHECK (
        (claimed_by IS NULL AND claim_expires_at IS NULL)
        OR (length(btrim(claimed_by)) > 0 AND claim_expires_at IS NOT NULL)
    )
);

CREATE INDEX idx_annotation_engine_operation_pending
    ON annotation_engine_operation(provider, provider_instance_ref, status, created_at, id);

CREATE TABLE annotation_engine_attempt (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    operation_id                uuid NOT NULL REFERENCES annotation_engine_operation(id),
    attempt_no                  integer NOT NULL,
    attempt_kind                varchar(16) NOT NULL,
    started_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_engine_attempt_no UNIQUE (operation_id, attempt_no),
    CONSTRAINT ck_annotation_engine_attempt_no CHECK (attempt_no > 0),
    CONSTRAINT ck_annotation_engine_attempt_kind
        CHECK (attempt_kind IN ('SUBMIT','LOOKUP','FETCH'))
);

CREATE INDEX idx_annotation_engine_attempt_operation
    ON annotation_engine_attempt(operation_id, attempt_no);

CREATE TABLE annotation_engine_attempt_outcome (
    id                          uuid PRIMARY KEY,
    attempt_id                  uuid NOT NULL REFERENCES annotation_engine_attempt(id),
    outcome                     varchar(24) NOT NULL,
    provider_status_code        integer,
    diagnostic_ref              varchar(512),
    occurred_at                 timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_engine_attempt_outcome UNIQUE (attempt_id),
    CONSTRAINT ck_annotation_engine_attempt_outcome
        CHECK (outcome IN ('SUCCEEDED','UNKNOWN','REJECTED','CONFLICT','FAILED_PRE_SEND')),
    CONSTRAINT ck_annotation_engine_attempt_status_code
        CHECK (provider_status_code IS NULL OR provider_status_code BETWEEN 100 AND 599)
);

CREATE TABLE annotation_engine_campaign_binding (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    provider                    varchar(64) NOT NULL,
    provider_instance_ref       varchar(255) NOT NULL,
    external_project_id         varchar(512) NOT NULL,
    request_id                  varchar(255) NOT NULL,
    config_sha256               varchar(64) NOT NULL,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_engine_campaign_binding_campaign
        UNIQUE (campaign_id),
    CONSTRAINT uq_annotation_engine_campaign_binding_external
        UNIQUE (provider, provider_instance_ref, external_project_id),
    CONSTRAINT ck_annotation_engine_campaign_binding_values CHECK (
        length(btrim(provider)) > 0
        AND length(btrim(provider_instance_ref)) > 0
        AND length(btrim(external_project_id)) > 0
        AND length(btrim(request_id)) > 0
        AND config_sha256 ~ '^[0-9a-f]{64}$'
    )
);

CREATE TABLE annotation_engine_task_binding (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_binding_id         uuid NOT NULL REFERENCES annotation_engine_campaign_binding(id),
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    external_task_id            varchar(512) NOT NULL,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_engine_task_binding_task UNIQUE (task_id),
    CONSTRAINT uq_annotation_engine_task_binding_external
        UNIQUE (campaign_binding_id, external_task_id),
    CONSTRAINT ck_annotation_engine_task_binding_external_id
        CHECK (length(btrim(external_task_id)) > 0)
);

CREATE OR REPLACE FUNCTION validate_annotation_engine_operation_insert()
RETURNS trigger AS $operation$
DECLARE
    campaign_workspace uuid;
BEGIN
    SELECT workspace_id
      INTO campaign_workspace
      FROM annotation_campaign
     WHERE id=NEW.campaign_id;

    IF campaign_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation engine operation crosses campaign workspace';
    END IF;
    RETURN NEW;
END;
$operation$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_engine_operation_insert
BEFORE INSERT ON annotation_engine_operation
FOR EACH ROW EXECUTE FUNCTION validate_annotation_engine_operation_insert();

CREATE OR REPLACE FUNCTION guard_annotation_engine_operation_update()
RETURNS trigger AS $operation_update$
BEGIN
    IF ROW(
        NEW.id,
        NEW.workspace_id,
        NEW.campaign_id,
        NEW.provider,
        NEW.provider_instance_ref,
        NEW.operation_kind,
        NEW.request_id,
        NEW.request_fingerprint,
        NEW.payload_manifest,
        NEW.payload_manifest_hash_payload,
        NEW.payload_manifest_sha256,
        NEW.created_at
    ) IS DISTINCT FROM ROW(
        OLD.id,
        OLD.workspace_id,
        OLD.campaign_id,
        OLD.provider,
        OLD.provider_instance_ref,
        OLD.operation_kind,
        OLD.request_id,
        OLD.request_fingerprint,
        OLD.payload_manifest,
        OLD.payload_manifest_hash_payload,
        OLD.payload_manifest_sha256,
        OLD.created_at
    ) THEN
        RAISE EXCEPTION 'annotation engine operation identity and payload are immutable';
    END IF;
    IF NEW.revision <= OLD.revision THEN
        RAISE EXCEPTION 'annotation engine operation revision must increase';
    END IF;
    NEW.updated_at := now();
    RETURN NEW;
END;
$operation_update$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_engine_operation_update
BEFORE UPDATE ON annotation_engine_operation
FOR EACH ROW EXECUTE FUNCTION guard_annotation_engine_operation_update();

CREATE OR REPLACE FUNCTION prevent_annotation_engine_operation_delete()
RETURNS trigger AS $operation_delete$
BEGIN
    RAISE EXCEPTION 'annotation engine operation history cannot be deleted';
END;
$operation_delete$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_engine_operation_delete
BEFORE DELETE ON annotation_engine_operation
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_engine_operation_delete();

CREATE OR REPLACE FUNCTION validate_annotation_engine_attempt_insert()
RETURNS trigger AS $attempt$
DECLARE
    operation_workspace uuid;
BEGIN
    SELECT workspace_id
      INTO operation_workspace
      FROM annotation_engine_operation
     WHERE id=NEW.operation_id;

    IF operation_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation engine attempt crosses operation workspace';
    END IF;
    RETURN NEW;
END;
$attempt$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_engine_attempt_insert
BEFORE INSERT ON annotation_engine_attempt
FOR EACH ROW EXECUTE FUNCTION validate_annotation_engine_attempt_insert();

CREATE OR REPLACE FUNCTION validate_annotation_engine_binding_insert()
RETURNS trigger AS $binding$
DECLARE
    campaign_workspace uuid;
BEGIN
    SELECT workspace_id
      INTO campaign_workspace
      FROM annotation_campaign
     WHERE id=NEW.campaign_id;

    IF campaign_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation engine binding crosses campaign workspace';
    END IF;
    RETURN NEW;
END;
$binding$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_engine_campaign_binding_insert
BEFORE INSERT ON annotation_engine_campaign_binding
FOR EACH ROW EXECUTE FUNCTION validate_annotation_engine_binding_insert();

CREATE OR REPLACE FUNCTION validate_annotation_engine_task_binding_insert()
RETURNS trigger AS $task_binding$
DECLARE
    binding_workspace uuid;
    binding_campaign uuid;
    task_workspace uuid;
    task_campaign uuid;
BEGIN
    SELECT workspace_id, campaign_id
      INTO binding_workspace, binding_campaign
      FROM annotation_engine_campaign_binding
     WHERE id=NEW.campaign_binding_id;

    SELECT workspace_id, campaign_id
      INTO task_workspace, task_campaign
      FROM annotation_task
     WHERE id=NEW.task_id;

    IF binding_workspace IS DISTINCT FROM NEW.workspace_id
       OR task_workspace IS DISTINCT FROM NEW.workspace_id
       OR binding_campaign IS DISTINCT FROM NEW.campaign_id
       OR task_campaign IS DISTINCT FROM NEW.campaign_id THEN
        RAISE EXCEPTION 'annotation engine task binding crosses Core boundary';
    END IF;
    RETURN NEW;
END;
$task_binding$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_engine_task_binding_insert
BEFORE INSERT ON annotation_engine_task_binding
FOR EACH ROW EXECUTE FUNCTION validate_annotation_engine_task_binding_insert();

CREATE OR REPLACE FUNCTION prevent_annotation_engine_append_only_mutation()
RETURNS trigger AS $append_only$
BEGIN
    RAISE EXCEPTION '% is append-only annotation engine history', TG_TABLE_NAME;
END;
$append_only$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_engine_attempt_immutable
BEFORE UPDATE OR DELETE ON annotation_engine_attempt
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_engine_append_only_mutation();

CREATE TRIGGER trg_annotation_engine_attempt_outcome_immutable
BEFORE UPDATE OR DELETE ON annotation_engine_attempt_outcome
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_engine_append_only_mutation();

CREATE TRIGGER trg_annotation_engine_campaign_binding_immutable
BEFORE UPDATE OR DELETE ON annotation_engine_campaign_binding
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_engine_append_only_mutation();

CREATE TRIGGER trg_annotation_engine_task_binding_immutable
BEFORE UPDATE OR DELETE ON annotation_engine_task_binding
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_engine_append_only_mutation();

CREATE OR REPLACE FUNCTION validate_annotation_engine_attempt_outcome_insert()
RETURNS trigger AS $attempt_outcome$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM annotation_engine_attempt WHERE id=NEW.attempt_id
    ) THEN
        RAISE EXCEPTION 'annotation engine attempt outcome references missing attempt';
    END IF;
    RETURN NEW;
END;
$attempt_outcome$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_engine_attempt_outcome_insert
BEFORE INSERT ON annotation_engine_attempt_outcome
FOR EACH ROW EXECUTE FUNCTION validate_annotation_engine_attempt_outcome_insert();
