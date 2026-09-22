DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM outbox_event
        WHERE routing_version IS NOT NULL
           OR required_handlers IS NOT NULL
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = 'check_violation',
            MESSAGE = 'cannot roll back outbox routing obligation while frozen event obligations exist';
    END IF;
END
$$;


ALTER TABLE outbox_event
    DROP COLUMN required_handlers,
    DROP COLUMN routing_version;
