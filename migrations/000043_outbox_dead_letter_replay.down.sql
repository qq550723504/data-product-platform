DROP INDEX IF EXISTS idx_outbox_event_replay_event;
DROP TABLE IF EXISTS outbox_event_replay;

ALTER TABLE outbox_event
    DROP CONSTRAINT IF EXISTS ck_outbox_attempt_base,
    DROP COLUMN IF EXISTS attempt_base;
