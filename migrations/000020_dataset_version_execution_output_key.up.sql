-- C2-a: enforce at most one live output DatasetVersion per Execution and
-- output Dataset. The producer key is (dataset_id, generated_by_execution_id).
--
-- Allocation writes generated_by_execution_id immediately, so this partial
-- unique index covers the complete two-phase output window:
-- CREATED / PROCESSING / READY.
--
-- FAILED is an unproduced half-product and may be retried in place; INVALID and
-- SUPERSEDED are terminal history. All three remain outside the live-output
-- uniqueness set. NULL generated_by_execution_id means the version was not
-- produced by an Execution and is intentionally outside this contract.

CREATE UNIQUE INDEX uq_dataset_version_execution_output
    ON dataset_version(dataset_id, generated_by_execution_id)
    WHERE generated_by_execution_id IS NOT NULL
      AND status IN ('CREATED', 'PROCESSING', 'READY');

COMMENT ON INDEX uq_dataset_version_execution_output IS
    'One Execution may have at most one live DatasetVersion (CREATED/PROCESSING/READY) per output Dataset; FAILED is a retryable half-product and INVALID/SUPERSEDED are terminal history.';
