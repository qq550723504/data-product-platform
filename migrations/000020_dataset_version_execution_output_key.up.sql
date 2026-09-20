-- C2-a: make "at most one output DatasetVersion per Execution" a database
-- constraint instead of a caller convention (#110).
--
-- The output idempotency key is (dataset_id, generated_by_execution_id). Before
-- C2-a that column was only written at SetReady time, so two concurrent replays
-- could each allocate a PENDING row that no constraint could see. C2-a also moves
-- the value into the allocation insert (see dataset repository), so the index
-- covers the whole two-phase window, not just the READY state.
--
-- The index covers the states in which a row can still be "the output" of an
-- Execution: the allocation window (CREATED/PROCESSING), the half-product repair
-- target (FAILED) and the published output (READY). INVALID and SUPERSEDED are
-- terminal historical facts and are excluded on purpose:
--
--   * dataset_version rows can never be deleted (guard_dataset_version_immutability)
--     and generated_by_execution_id can never be rewritten once READY, so an index
--     that also covered terminal rows would refuse to install on any installation
--     that already produced a duplicate output, with no way to remediate it;
--   * excluding them keeps a real remediation path: an extra copy can be taken out
--     of the live set with an explicit state change (InvalidateDatasetVersion for a
--     READY row, FAIL for a half-written one) instead of rewriting history.
--
-- A pair may therefore have several historical rows, but only one live one.
-- Falling out of the index never deletes or rewrites a fact: the withdrawn output
-- stays queryable, and the writer still refuses to reuse an INVALID/SUPERSEDED
-- output (see upload_version.go) rather than silently resurrecting it.
--
-- Historical rows are not rewritten and NULL stays "no recorded producing
-- execution": the index is partial so pre-existing non-output versions and any
-- future non-execution versions are unaffected.

DO $$
DECLARE
    duplicate_pairs integer;
BEGIN
    SELECT count(*) INTO duplicate_pairs
    FROM (
        SELECT dataset_id, generated_by_execution_id
        FROM dataset_version
        WHERE generated_by_execution_id IS NOT NULL
          AND status NOT IN ('INVALID', 'SUPERSEDED')
        GROUP BY dataset_id, generated_by_execution_id
        HAVING count(*) > 1
    ) duplicates;

    IF duplicate_pairs > 0 THEN
        RAISE EXCEPTION 'cannot enforce uq_dataset_version_execution_output: % (dataset, execution) pair(s) already have more than one live output version; bring each pair back to one live row with an explicit state change (InvalidateDatasetVersion for a READY extra, FAIL for a half-written extra), then re-run this migration', duplicate_pairs;
    END IF;
END
$$;

CREATE UNIQUE INDEX uq_dataset_version_execution_output
    ON dataset_version(dataset_id, generated_by_execution_id)
    WHERE generated_by_execution_id IS NOT NULL
      AND status NOT IN ('INVALID', 'SUPERSEDED');

COMMENT ON INDEX uq_dataset_version_execution_output IS
    'C2-a: one Execution may have at most one live DatasetVersion (CREATED/PROCESSING/READY/FAILED) per output Dataset; the two-phase output write reuses that row instead of allocating a new version number. INVALID/SUPERSEDED rows are terminal history and stay outside the constraint.';
