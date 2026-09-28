CREATE INDEX idx_execution_retry_of
    ON execution(retry_of_execution_id)
    WHERE retry_of_execution_id IS NOT NULL;
