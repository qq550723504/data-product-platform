-- The migration runner executes this file under its migration advisory lock
-- and in one transaction. Once an execution command has a fingerprint, the
-- column is part of the replay contract: dropping it would make the same
-- idempotency key look like a different request after a down/up cycle.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM command_idempotency
        WHERE request_fingerprint IS NOT NULL
    ) THEN
        RAISE EXCEPTION USING
            ERRCODE = 'check_violation',
            MESSAGE = 'cannot roll back 000017 while execution idempotency fingerprints exist';
    END IF;
END
$$;

ALTER TABLE command_idempotency
    DROP CONSTRAINT IF EXISTS ck_command_idempotency_fingerprint;

ALTER TABLE command_idempotency
    DROP COLUMN IF EXISTS request_fingerprint;
