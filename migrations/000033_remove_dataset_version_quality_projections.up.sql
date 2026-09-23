-- Remove legacy DatasetVersion quality/compliance projections.
--
-- Quality/compliance business truth is held by immutable assessment/result facts
-- and the ProductRelease readiness bindings. Keeping mutable projection columns
-- on dataset_version creates a second, stale source of truth.

ALTER TABLE dataset_version
    DROP COLUMN quality_status,
    DROP COLUMN compliance_status;
