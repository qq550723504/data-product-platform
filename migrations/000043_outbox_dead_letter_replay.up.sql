-- C1-e / #215: auditable dead-letter replay.
--
-- attempts remains a cumulative historical counter. attempt_base records the
-- cumulative count at the start of the current automatic-retry generation, so
-- an explicit operator replay receives a fresh MaxAttempts budget without
-- erasing how many dispatch attempts happened before the replay.
ALTER TABLE outbox_event
    ADD COLUMN attempt_base integer NOT NULL DEFAULT 0;

ALTER TABLE outbox_event
    ADD CONSTRAINT ck_outbox_attempt_base
    CHECK (attempt_base >= 0 AND attempt_base <= attempts);

CREATE TABLE outbox_event_replay (
    id uuid PRIMARY KEY,
    event_id uuid NOT NULL REFERENCES outbox_event(id),
    idempotency_key varchar(200) NOT NULL,
    request_fingerprint varchar(64) NOT NULL,
    actor_id uuid NOT NULL,
    reason text NOT NULL,
    previous_attempts integer NOT NULL,
    previous_last_error text,
    previous_dead_lettered_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uk_outbox_event_replay_request UNIQUE (event_id, idempotency_key),
    CONSTRAINT ck_outbox_event_replay_fingerprint
        CHECK (request_fingerprint ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_outbox_event_replay_reason
        CHECK (length(btrim(reason)) > 0)
);

CREATE INDEX idx_outbox_event_replay_event
    ON outbox_event_replay (event_id, created_at, id);

CREATE OR REPLACE FUNCTION reject_outbox_event_replay_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'outbox_event_replay is immutable';
END;
$$;

CREATE TRIGGER trg_outbox_event_replay_immutable
    BEFORE UPDATE OR DELETE ON outbox_event_replay
    FOR EACH ROW
    EXECUTE FUNCTION reject_outbox_event_replay_mutation();
