-- T2 execution command idempotency.
-- Existing command_idempotency rows predate request fingerprints and remain
-- valid for their original command implementations. New execution commands
-- always write a canonical SHA-256 request fingerprint in the same transaction
-- as the business fact and its outbox event.

ALTER TABLE command_idempotency
    ADD COLUMN request_fingerprint varchar(64);

ALTER TABLE command_idempotency
    ADD CONSTRAINT ck_command_idempotency_fingerprint
    CHECK (request_fingerprint IS NULL OR request_fingerprint ~ '^[0-9a-f]{64}$');
