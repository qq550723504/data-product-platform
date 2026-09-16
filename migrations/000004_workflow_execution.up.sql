CREATE TABLE workflow (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL,
    code            varchar(128) NOT NULL,
    name            varchar(255) NOT NULL,
    description     text,
    status          varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid,
    updated_at      timestamptz NOT NULL DEFAULT now(),
    updated_by      uuid,
    CONSTRAINT uq_workflow_workspace_code UNIQUE(workspace_id, code),
    CONSTRAINT ck_workflow_status CHECK(status IN ('ACTIVE','ARCHIVED'))
);

CREATE TABLE workflow_version (
    id                uuid PRIMARY KEY,
    workflow_id       uuid NOT NULL REFERENCES workflow(id),
    version           varchar(64) NOT NULL,
    definition_ref    varchar(512) NOT NULL,
    definition_sha256 varchar(64) NOT NULL,
    definition        jsonb NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    created_by        uuid,
    CONSTRAINT uq_workflow_version UNIQUE(workflow_id, version)
);

CREATE TABLE execution (
    id                        uuid PRIMARY KEY,
    workspace_id              uuid NOT NULL,
    workflow_version_id       uuid NOT NULL REFERENCES workflow_version(id),
    output_dataset_id         uuid NOT NULL REFERENCES dataset(id),
    output_dataset_version_id uuid REFERENCES dataset_version(id),
    target_period             varchar(7) NOT NULL,
    status                    varchar(32) NOT NULL DEFAULT 'QUEUED',
    attempt                   integer NOT NULL DEFAULT 1,
    retry_of_execution_id     uuid REFERENCES execution(id),
    engine_type               varchar(32) NOT NULL DEFAULT 'NATIVE',
    engine_execution_id       varchar(255),
    error_code                varchar(128),
    error_message             text,
    metrics                   jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at                timestamptz NOT NULL DEFAULT now(),
    created_by                uuid,
    started_at                timestamptz,
    finished_at               timestamptz,
    CONSTRAINT ck_execution_status CHECK(status IN ('QUEUED','RUNNING','SUCCEEDED','FAILED','CANCELLED')),
    CONSTRAINT ck_execution_attempt CHECK(attempt > 0)
);

CREATE INDEX idx_execution_workspace_status ON execution(workspace_id, status, created_at DESC);
CREATE INDEX idx_execution_workflow_version ON execution(workflow_version_id, created_at DESC);

CREATE TABLE execution_input (
    execution_id        uuid NOT NULL REFERENCES execution(id),
    input_name          varchar(128) NOT NULL,
    dataset_version_id  uuid NOT NULL REFERENCES dataset_version(id),
    created_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(execution_id, input_name)
);

CREATE INDEX idx_execution_input_dataset_version ON execution_input(dataset_version_id);

CREATE TABLE execution_quarantine_record (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    uuid NOT NULL REFERENCES execution(id),
    input_name      varchar(128) NOT NULL,
    source_key      varchar(512),
    reason_code     varchar(128) NOT NULL,
    reason_message  text,
    payload         jsonb NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_execution_quarantine_execution ON execution_quarantine_record(execution_id, input_name);

CREATE TABLE cost_event (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL,
    execution_id    uuid REFERENCES execution(id),
    cost_type       varchar(64) NOT NULL,
    quantity        numeric(20,6) NOT NULL DEFAULT 0,
    unit            varchar(32) NOT NULL,
    amount          numeric(20,6),
    currency        varchar(8),
    pricing_mode    varchar(32) NOT NULL DEFAULT 'POC_ESTIMATE',
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_cost_quantity CHECK(quantity >= 0)
);

CREATE INDEX idx_cost_event_execution ON cost_event(execution_id, occurred_at DESC);
CREATE INDEX idx_cost_event_workspace ON cost_event(workspace_id, occurred_at DESC);

CREATE OR REPLACE FUNCTION prevent_workflow_version_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'workflow_version is immutable; create a new version instead';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_workflow_version_immutable_update
BEFORE UPDATE ON workflow_version
FOR EACH ROW EXECUTE FUNCTION prevent_workflow_version_mutation();

CREATE TRIGGER trg_workflow_version_immutable_delete
BEFORE DELETE ON workflow_version
FOR EACH ROW EXECUTE FUNCTION prevent_workflow_version_mutation();
