CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS outbox_event (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type  varchar(64) NOT NULL,
    aggregate_id    uuid NOT NULL,
    event_type      varchar(128) NOT NULL,
    payload         jsonb NOT NULL,
    status          varchar(16) NOT NULL DEFAULT 'PENDING',
    attempts        integer NOT NULL DEFAULT 0,
    available_at    timestamptz NOT NULL DEFAULT now(),
    created_at      timestamptz NOT NULL DEFAULT now(),
    published_at    timestamptz,
    last_error      text,
    CONSTRAINT ck_outbox_status CHECK (status IN ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED'))
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON outbox_event (status, available_at, created_at)
    WHERE status IN ('PENDING', 'FAILED');

CREATE TABLE IF NOT EXISTS audit_event (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id    uuid,
    actor_type      varchar(32) NOT NULL DEFAULT 'SYSTEM',
    actor_id        uuid,
    action          varchar(128) NOT NULL,
    object_type     varchar(64) NOT NULL,
    object_id       uuid NOT NULL,
    before_state    jsonb,
    after_state     jsonb,
    reason          text,
    trace_id        varchar(128),
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ck_audit_actor_type CHECK (actor_type IN ('USER', 'SYSTEM', 'SERVICE'))
);

CREATE INDEX IF NOT EXISTS idx_audit_object
    ON audit_event (object_type, object_id, occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_audit_workspace_time
    ON audit_event (workspace_id, occurred_at DESC);
