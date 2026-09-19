ALTER TABLE command_idempotency
    DROP CONSTRAINT IF EXISTS ck_command_idempotency_fingerprint;

ALTER TABLE command_idempotency
    DROP COLUMN IF EXISTS request_fingerprint;
