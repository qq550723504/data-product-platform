DROP INDEX IF EXISTS idx_execution_engine_status_id;

ALTER TABLE execution
    DROP CONSTRAINT ck_execution_status;

ALTER TABLE execution
    ADD CONSTRAINT ck_execution_status
    CHECK(status IN ('QUEUED','RUNNING','SUCCEEDED','FAILED','CANCELLED'));
