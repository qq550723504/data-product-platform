-- Restore only the legacy nullable columns for schema rollback.
-- No historical value is reconstructed: quality/compliance truth remains in
-- immutable result/assessment facts.

ALTER TABLE dataset_version
    ADD COLUMN quality_status varchar(32),
    ADD COLUMN compliance_status varchar(32);
