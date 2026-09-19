-- T3/B2 freezes the dependencies actually consumed by a production Execution.
-- The preparation row is the commit boundary: dependency and mapping-usage rows
-- are only authoritative when the PREPARED row exists in the same transaction.

ALTER TABLE execution
    ADD CONSTRAINT uq_execution_workspace_id UNIQUE (workspace_id, id);

ALTER TABLE entity_mapping_decision
    ADD CONSTRAINT uq_entity_mapping_decision_workspace_id UNIQUE (workspace_id, id);

ALTER TABLE entity_match_job
    ADD CONSTRAINT uq_entity_match_job_workspace_id UNIQUE (workspace_id, id);

ALTER TABLE entity_match_job
    ADD COLUMN policy_content_sha256 varchar(64),
    ADD COLUMN policy_content bytea;

ALTER TABLE entity_match_job
    ADD CONSTRAINT ck_entity_match_job_policy_content_sha256
    CHECK (policy_content_sha256 IS NULL OR policy_content_sha256 ~ '^[0-9a-f]{64}$');

CREATE TABLE execution_dependency_preparation (
    execution_id       uuid PRIMARY KEY REFERENCES execution(id),
    workspace_id       uuid NOT NULL,
    binding_fingerprint varchar(64) NOT NULL,
    status              varchar(32) NOT NULL,
    prepared_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_execution_dependency_preparation_status CHECK (status IN ('PREPARED')),
    CONSTRAINT fk_execution_dependency_preparation_execution
        FOREIGN KEY (workspace_id, execution_id) REFERENCES execution(workspace_id, id)
);

CREATE TABLE execution_dependency_binding (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id          uuid NOT NULL REFERENCES execution(id),
    workspace_id          uuid NOT NULL,
    dependency_name       varchar(128) NOT NULL,
    dataset_version_id    uuid REFERENCES dataset_version(id),
    reference             varchar(512) NOT NULL,
    version               varchar(128) NOT NULL,
    content_sha256        varchar(64) NOT NULL,
    content               bytea NOT NULL DEFAULT ''::bytea,
    created_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_execution_dependency_binding UNIQUE (execution_id, dependency_name),
    CONSTRAINT ck_execution_dependency_binding_sha256 CHECK (content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT fk_execution_dependency_binding_execution
        FOREIGN KEY (workspace_id, execution_id) REFERENCES execution(workspace_id, id)
);

CREATE INDEX idx_execution_dependency_binding_dataset
    ON execution_dependency_binding(dataset_version_id)
    WHERE dataset_version_id IS NOT NULL;

CREATE TABLE execution_mapping_usage (
    id                           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id                 uuid NOT NULL REFERENCES execution(id),
    workspace_id                 uuid NOT NULL,
    input_name                   varchar(128) NOT NULL,
    input_dataset_version_id     uuid NOT NULL REFERENCES dataset_version(id),
    resolution_dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),
    source_type                  varchar(64) NOT NULL,
    source_ref                   varchar(512) NOT NULL,
    source_key                   varchar(512) NOT NULL,
    decision_id                  uuid NOT NULL,
    entity_id                    uuid NOT NULL,
    created_at                   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_execution_mapping_usage_source
        UNIQUE (execution_id, input_name, input_dataset_version_id, source_type, source_ref, source_key),
    CONSTRAINT fk_execution_mapping_usage_execution
        FOREIGN KEY (workspace_id, execution_id) REFERENCES execution(workspace_id, id),
    CONSTRAINT fk_execution_mapping_usage_decision
        FOREIGN KEY (workspace_id, decision_id) REFERENCES entity_mapping_decision(workspace_id, id),
    CONSTRAINT fk_execution_mapping_usage_entity
        FOREIGN KEY (workspace_id, entity_id) REFERENCES entity(workspace_id, id)
);

CREATE INDEX idx_execution_mapping_usage_execution
    ON execution_mapping_usage(execution_id, input_name, source_key);

CREATE TABLE entity_resolution_output_decision (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    output_dataset_version_id uuid NOT NULL REFERENCES dataset_version(id),
    workspace_id             uuid NOT NULL,
    source_job_id            uuid NOT NULL REFERENCES entity_match_job(id),
    source_type              varchar(64) NOT NULL,
    source_ref               varchar(512) NOT NULL,
    source_key               varchar(512) NOT NULL,
    decision_id              uuid NOT NULL,
    entity_id                uuid NOT NULL,
    created_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_entity_resolution_output_decision_source
        UNIQUE (output_dataset_version_id, source_key),
    CONSTRAINT uq_entity_resolution_output_decision_decision
        UNIQUE (output_dataset_version_id, decision_id),
    CONSTRAINT fk_entity_resolution_output_decision_job_workspace
        FOREIGN KEY (workspace_id, source_job_id) REFERENCES entity_match_job(workspace_id, id),
    CONSTRAINT fk_entity_resolution_output_decision_decision
        FOREIGN KEY (workspace_id, decision_id) REFERENCES entity_mapping_decision(workspace_id, id),
    CONSTRAINT fk_entity_resolution_output_decision_entity
        FOREIGN KEY (workspace_id, entity_id) REFERENCES entity(workspace_id, id)
);

CREATE INDEX idx_entity_resolution_output_decision_output
    ON entity_resolution_output_decision(output_dataset_version_id, source_key);
