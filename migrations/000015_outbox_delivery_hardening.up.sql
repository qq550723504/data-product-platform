-- C1-a/C1-b outbox delivery hardening (#103, #110):
--   * event_version distinguishes payload structure versions; old flat payloads
--     keep version 1 and stay consumable without rewriting historical rows.
--   * claim_token identifies the current dispatch lease holder. Every terminal
--     write (PUBLISHED / FAILED / DEAD_LETTER) must match the token, so a worker
--     whose lease expired cannot overwrite the result of the next claimer.
--   * DEAD_LETTER stops automatic dispatch for poison events after
--     max attempts instead of retrying forever.
--   * outbox_event_consumption records per-consumer dispatch confirmations
--     keyed by (consumer_name, event_id); redelivery after a lost
--     acknowledgement stays safe and is not mistaken for first delivery.
-- Forward-only additions to migration 000001; historical rows are not rewritten.

ALTER TABLE outbox_event
    ADD COLUMN event_version smallint NOT NULL DEFAULT 1,
    ADD COLUMN claim_token uuid,
    ADD COLUMN claimed_by varchar(128),
    ADD COLUMN claimed_at timestamptz,
    ADD COLUMN dead_lettered_at timestamptz;

ALTER TABLE outbox_event DROP CONSTRAINT ck_outbox_status;

ALTER TABLE outbox_event
    ADD CONSTRAINT ck_outbox_status
    CHECK (status IN ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED', 'DEAD_LETTER'));

CREATE TABLE IF NOT EXISTS outbox_event_consumption (
    consumer_name varchar(128) NOT NULL,
    event_id      uuid NOT NULL REFERENCES outbox_event(id),
    consumed_at   timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer_name, event_id)
);

-- Covers lease-expiry reclaim: PROCESSING rows with an expired available_at
-- must be findable by the claim scan, not only PENDING/FAILED rows.
CREATE INDEX IF NOT EXISTS idx_outbox_claimable
    ON outbox_event (available_at, created_at)
    WHERE status IN ('PENDING', 'FAILED', 'PROCESSING');
