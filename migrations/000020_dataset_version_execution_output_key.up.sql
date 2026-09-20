-- C2-a: make "at most one output DatasetVersion per Execution" a database
-- constraint instead of a caller convention (#110).
--
-- The output idempotency key is (dataset_id, generated_by_execution_id). Before
-- C2-a that column was only written at SetReady time, so two concurrent replays
-- could each allocate a PENDING row that no constraint could see. C2-a also moves
-- the value into the allocation insert (see dataset repository), so the index
-- covers the whole two-phase window, not just the READY state.
--
-- The index covers the states in which a row can still become the output of an
-- Execution: the allocation window (CREATED/PROCESSING) and the published output
-- (READY). INVALID, SUPERSEDED and FAILED are outside it:
--
--   * dataset_version rows can never be deleted (guard_dataset_version_immutability)
--     and generated_by_execution_id can never be rewritten once READY, so an index
--     that also covered terminal rows would refuse to install on any installation
--     that already produced a duplicate output, with no way to remediate it;
--   * before C2-a the key was only ever written together with status READY, so the
--     duplicates such an installation can actually hold are surplus READY rows
--     (INVALID/SUPERSEDED copies are excluded either way). Those are withdrawn with
--     InvalidateDatasetVersion, which is a real, reachable path;
--   * FAILED is excluded on purpose so that the remediation the guard message
--     offers for an unproduced half-product - applying the platform's failure
--     transition - genuinely takes the row out of the constraint. An index that
--     still counted FAILED would refuse again after following its own advice.
--     The writer does not need FAILED inside the index: it reuses the row it finds
--     for the (dataset, execution) pair, and a FAILED row that is republished
--     re-enters the index through SetReady, where uniqueness is checked again.
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
    offenders text;
BEGIN
    SELECT string_agg(
               format('  (dataset_id=%s, execution_id=%s) live rows: %s',
                      dataset_id, generated_by_execution_id, live_rows),
               E'\n')
    INTO offenders
    FROM (
        SELECT dataset_id,
               generated_by_execution_id,
               array_agg(version_no::text || ':' || status ORDER BY version_no) AS live_rows
        FROM dataset_version
        WHERE generated_by_execution_id IS NOT NULL
          AND status IN ('CREATED', 'PROCESSING', 'READY')
        GROUP BY dataset_id, generated_by_execution_id
        HAVING count(*) > 1
    ) duplicates;

    IF offenders IS NOT NULL THEN
        RAISE EXCEPTION E'cannot enforce uq_dataset_version_execution_output: these (dataset, execution) pairs already have more than one live output version:\n%\nBring each pair back to one live row with an explicit state change, then re-run this migration. Historical rows are never deleted: withdraw a surplus READY output with InvalidateDatasetVersion, and take a surplus unproduced half-product (CREATED/PROCESSING) out of the live set by applying the explicit failure transition, which leaves it FAILED and outside this constraint.', offenders;
    END IF;
END
$$;

CREATE UNIQUE INDEX uq_dataset_version_execution_output
    ON dataset_version(dataset_id, generated_by_execution_id)
    WHERE generated_by_execution_id IS NOT NULL
      AND status IN ('CREATED', 'PROCESSING', 'READY');

COMMENT ON INDEX uq_dataset_version_execution_output IS
    'C2-a: one Execution may have at most one live DatasetVersion (CREATED/PROCESSING/READY) per output Dataset; the two-phase output write reuses that row instead of allocating a new version number. INVALID/SUPERSEDED rows are terminal history and FAILED rows are unproduced half-products, so all three stay outside the constraint and remain repairable or withdrawable.';
