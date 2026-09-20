-- Minimal crash-safe credential issuance slice for DeliveryOperation.
-- The provider remains outside PostgreSQL. PostgreSQL is the source of truth
-- for the operation, authorization observations, attempt history and outcome.

ALTER TABLE cost_event
    ADD COLUMN activity_id uuid;

CREATE UNIQUE INDEX uq_cost_event_activity_component
    ON cost_event(workspace_id, activity_id, cost_type)
    WHERE activity_id IS NOT NULL;

CREATE TABLE delivery_authorization_fence (
    workspace_id uuid PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE delivery_operation (
    id                              uuid PRIMARY KEY,
    workspace_id                   uuid NOT NULL,
    dataset_version_id             uuid NOT NULL REFERENCES dataset_version(id),
    certification_ref              uuid,
    idempotency_key                varchar(255) NOT NULL,
    provider_name                  varchar(128) NOT NULL,
    provider_request_key           varchar(255) NOT NULL,
    status                         varchar(32) NOT NULL,
    current_gate_decision          varchar(16) NOT NULL,
    dependency_revision            bigint NOT NULL DEFAULT 0,
    principal_ref                  varchar(255) NOT NULL,
    effective_consumer_ref         varchar(255) NOT NULL,
    delegation_ref                 varchar(255),
    purpose                        varchar(128) NOT NULL,
    action                         varchar(64) NOT NULL,
    scope_ref                      varchar(512) NOT NULL,
    delivery_channel               varchar(64) NOT NULL,
    requested_expires_at           timestamptz NOT NULL,
    fresh_cap_expires_at           timestamptz,
    credential_ref                 varchar(512),
    credential_hash                varchar(128),
    provider_credential_expires_at timestamptz,
    terminal_reason                varchar(255),
    created_at                     timestamptz NOT NULL DEFAULT now(),
    updated_at                     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_delivery_operation_idempotency UNIQUE(workspace_id, idempotency_key),
    CONSTRAINT uq_delivery_operation_provider_key UNIQUE(provider_name, provider_request_key),
    CONSTRAINT ck_delivery_operation_status CHECK(status IN (
        'PREPARED','ISSUANCE_PENDING','CONTAINMENT_PENDING','ISSUED','BLOCKED','FAILED'
    )),
    CONSTRAINT ck_delivery_operation_gate CHECK(current_gate_decision IN ('ALLOWED','BLOCKED')),
    CONSTRAINT ck_delivery_operation_expiry CHECK(requested_expires_at > created_at),
    CONSTRAINT ck_delivery_operation_secret_free CHECK(credential_hash IS NULL OR credential_hash ~ '^[0-9a-fA-F]{64,128}$')
);

CREATE INDEX idx_delivery_operation_recovery
    ON delivery_operation(status, updated_at)
    WHERE status IN ('ISSUANCE_PENDING','CONTAINMENT_PENDING');

CREATE TABLE delivery_gate_evaluation (
    id                      uuid PRIMARY KEY,
    delivery_operation_id   uuid NOT NULL REFERENCES delivery_operation(id),
    evaluation_key          varchar(255) NOT NULL,
    stage                   varchar(32) NOT NULL,
    decision                varchar(16) NOT NULL,
    blockers                text[] NOT NULL DEFAULT ARRAY[]::text[],
    dependency_revision     bigint NOT NULL,
    principal_ref           varchar(255) NOT NULL,
    effective_consumer_ref  varchar(255) NOT NULL,
    delegation_ref          varchar(255),
    fresh_cap_expires_at    timestamptz,
    created_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_delivery_gate_evaluation_key UNIQUE(delivery_operation_id, evaluation_key),
    CONSTRAINT ck_delivery_gate_stage CHECK(stage IN ('INITIAL','PROVIDER_PREPARE','RECONCILIATION','TERMINAL_FINALIZE','REPLAY','CONTAINMENT')),
    CONSTRAINT ck_delivery_gate_decision CHECK(decision IN ('ALLOWED','BLOCKED'))
);

CREATE TABLE delivery_transition (
    id                      uuid PRIMARY KEY,
    delivery_operation_id   uuid NOT NULL REFERENCES delivery_operation(id),
    transition_key          varchar(255) NOT NULL,
    from_status             varchar(32) NOT NULL,
    to_status               varchar(32) NOT NULL,
    gate_evaluation_id      uuid REFERENCES delivery_gate_evaluation(id),
    provider_attempt_id     uuid,
    reason                  varchar(255),
    created_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_delivery_transition_key UNIQUE(delivery_operation_id, transition_key)
);

CREATE INDEX idx_delivery_transition_history
    ON delivery_transition(delivery_operation_id, created_at, id);

CREATE TABLE delivery_provider_attempt (
    id                    uuid PRIMARY KEY,
    delivery_operation_id uuid NOT NULL REFERENCES delivery_operation(id),
    provider_request_key  varchar(255) NOT NULL,
    invocation_key        varchar(255) NOT NULL,
    invocation_kind       varchar(32) NOT NULL,
    started_at            timestamptz NOT NULL DEFAULT now(),
    quantity              numeric(20,6) NOT NULL DEFAULT 1,
    unit                  varchar(32) NOT NULL DEFAULT 'invocation',
    CONSTRAINT uq_delivery_provider_attempt_key UNIQUE(delivery_operation_id, invocation_key),
    CONSTRAINT ck_delivery_provider_attempt_kind CHECK(invocation_kind IN ('ISSUE','RECONCILE','REVOKE','VERIFY')),
    CONSTRAINT ck_delivery_provider_attempt_quantity CHECK(quantity > 0)
);

CREATE TABLE delivery_provider_observation (
    id                              uuid PRIMARY KEY,
    provider_attempt_id             uuid NOT NULL REFERENCES delivery_provider_attempt(id),
    observation_kind                varchar(32) NOT NULL,
    observed_outcome                varchar(32) NOT NULL,
    capability_ref                  varchar(512),
    capability_hash                 varchar(128),
    provider_credential_expires_at  timestamptz,
    evidence_ref                    varchar(512),
    observed_at                     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_delivery_provider_observation_kind CHECK(observation_kind IN ('CALL_RETURN','TIMEOUT','RECONCILIATION','AUTHORITATIVE_LOOKUP')),
    CONSTRAINT ck_delivery_provider_observation_outcome CHECK(observed_outcome IN ('SUCCESS','FAILED','UNKNOWN','TIMEOUT','NOT_FOUND'))
);

ALTER TABLE delivery_transition
    ADD CONSTRAINT fk_delivery_transition_provider_attempt
    FOREIGN KEY (provider_attempt_id) REFERENCES delivery_provider_attempt(id);

CREATE TABLE delivery_credential_replay_decision (
    id                      uuid PRIMARY KEY,
    replay_attempt_id       uuid NOT NULL,
    delivery_operation_id   uuid NOT NULL REFERENCES delivery_operation(id),
    gate_evaluation_id      uuid NOT NULL REFERENCES delivery_gate_evaluation(id),
    decision                varchar(32) NOT NULL,
    principal_ref           varchar(255) NOT NULL,
    effective_consumer_ref  varchar(255) NOT NULL,
    dependency_revision     bigint NOT NULL,
    fresh_cap_expires_at    timestamptz,
    capability_ref          varchar(512),
    capability_hash         varchar(128),
    reason                  varchar(255),
    created_at              timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_delivery_replay_attempt UNIQUE(delivery_operation_id, replay_attempt_id),
    CONSTRAINT ck_delivery_replay_decision CHECK(decision IN ('ALLOWED','BLOCKED','CONTAINMENT_PENDING'))
);

CREATE TABLE cost_allocation (
    id                    uuid PRIMARY KEY,
    cost_event_id         uuid NOT NULL UNIQUE REFERENCES cost_event(id),
    delivery_operation_id uuid NOT NULL REFERENCES delivery_operation(id),
    created_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_cost_allocation_delivery
    ON cost_allocation(delivery_operation_id, created_at);

CREATE OR REPLACE FUNCTION prevent_delivery_history_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only history', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_delivery_gate_evaluation_immutable
BEFORE UPDATE OR DELETE ON delivery_gate_evaluation
FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();

CREATE TRIGGER trg_delivery_transition_immutable
BEFORE UPDATE OR DELETE ON delivery_transition
FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();

CREATE TRIGGER trg_delivery_provider_attempt_immutable
BEFORE UPDATE OR DELETE ON delivery_provider_attempt
FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();

CREATE TRIGGER trg_delivery_provider_observation_immutable
BEFORE UPDATE OR DELETE ON delivery_provider_observation
FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();

CREATE TRIGGER trg_delivery_replay_decision_immutable
BEFORE UPDATE OR DELETE ON delivery_credential_replay_decision
FOR EACH ROW EXECUTE FUNCTION prevent_delivery_history_mutation();

CREATE OR REPLACE FUNCTION guard_delivery_operation_mutation()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'delivery_operation is historical and cannot be deleted';
    END IF;
    IF OLD.workspace_id IS DISTINCT FROM NEW.workspace_id OR
       OLD.dataset_version_id IS DISTINCT FROM NEW.dataset_version_id OR
       OLD.certification_ref IS DISTINCT FROM NEW.certification_ref OR
       OLD.idempotency_key IS DISTINCT FROM NEW.idempotency_key OR
       OLD.provider_name IS DISTINCT FROM NEW.provider_name OR
       OLD.provider_request_key IS DISTINCT FROM NEW.provider_request_key OR
       OLD.principal_ref IS DISTINCT FROM NEW.principal_ref OR
       OLD.effective_consumer_ref IS DISTINCT FROM NEW.effective_consumer_ref OR
       OLD.delegation_ref IS DISTINCT FROM NEW.delegation_ref OR
       OLD.purpose IS DISTINCT FROM NEW.purpose OR
       OLD.action IS DISTINCT FROM NEW.action OR
       OLD.scope_ref IS DISTINCT FROM NEW.scope_ref OR
       OLD.delivery_channel IS DISTINCT FROM NEW.delivery_channel OR
       OLD.requested_expires_at IS DISTINCT FROM NEW.requested_expires_at OR
       OLD.created_at IS DISTINCT FROM NEW.created_at THEN
        RAISE EXCEPTION 'delivery_operation request identity is immutable';
    END IF;
    IF OLD.status IN ('ISSUED','BLOCKED','FAILED') AND NEW.status IS DISTINCT FROM OLD.status THEN
        RAISE EXCEPTION 'terminal delivery_operation status is immutable';
    END IF;
    IF OLD.status = 'ISSUED' AND (
       OLD.credential_ref IS DISTINCT FROM NEW.credential_ref OR
       OLD.credential_hash IS DISTINCT FROM NEW.credential_hash OR
       OLD.provider_credential_expires_at IS DISTINCT FROM NEW.provider_credential_expires_at
    ) THEN
        RAISE EXCEPTION 'issued delivery capability is immutable';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_delivery_operation_guard
BEFORE UPDATE OR DELETE ON delivery_operation
FOR EACH ROW EXECUTE FUNCTION guard_delivery_operation_mutation();
