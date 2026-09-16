DROP TRIGGER IF EXISTS trg_dataset_version_immutability ON dataset_version;
DROP FUNCTION IF EXISTS guard_dataset_version_immutability();
ALTER TABLE dataset DROP CONSTRAINT IF EXISTS fk_dataset_current_version;
DROP TABLE IF EXISTS dataset_version_lineage;
DROP TABLE IF EXISTS dataset_version;
DROP TABLE IF EXISTS dataset;
DROP TABLE IF EXISTS data_resource;
