ALTER TABLE execution
    DROP CONSTRAINT ck_execution_status;

ALTER TABLE execution
    ADD CONSTRAINT ck_execution_status
    CHECK(status IN ('QUEUED','SUBMITTING','RUNNING','SUCCEEDED','FAILED','CANCELLED'));

CREATE INDEX idx_execution_engine_status_id
    ON execution(engine_type, status, id);
