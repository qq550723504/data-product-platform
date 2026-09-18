DROP TABLE IF EXISTS outbox_event_consumption;

DROP INDEX IF EXISTS idx_outbox_claimable;

-- DEAD_LETTER has no counterpart in the previous constraint. Rewriting those
-- rows as FAILED would erase the fact that automatic dispatch was stopped, so
-- refuse loudly and make an operator replay or export them first.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM outbox_event WHERE status = 'DEAD_LETTER') THEN
        RAISE EXCEPTION 'cannot downgrade 000015: % dead-lettered outbox events must be replayed or exported first',
            (SELECT count(*) FROM outbox_event WHERE status = 'DEAD_LETTER');
    END IF;
END $$;

ALTER TABLE outbox_event DROP CONSTRAINT ck_outbox_status;

ALTER TABLE outbox_event
    ADD CONSTRAINT ck_outbox_status
    CHECK (status IN ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED'));

ALTER TABLE outbox_event
    DROP COLUMN dead_lettered_at,
    DROP COLUMN claimed_at,
    DROP COLUMN claimed_by,
    DROP COLUMN claim_token,
    DROP COLUMN event_version;
