LOCK TABLE outbox_event_replay IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM outbox_event_replay LIMIT 1) THEN
        RAISE EXCEPTION 'cannot rollback 000043: outbox replay history exists';
    END IF;
END;
$$;

DROP TRIGGER IF EXISTS trg_outbox_event_replay_immutable ON outbox_event_replay;
DROP FUNCTION IF EXISTS reject_outbox_event_replay_mutation();
DROP INDEX IF EXISTS idx_outbox_event_replay_event;
DROP TABLE IF EXISTS outbox_event_replay;

ALTER TABLE outbox_event
    DROP CONSTRAINT IF EXISTS ck_outbox_attempt_base,
    DROP COLUMN IF EXISTS attempt_base;
