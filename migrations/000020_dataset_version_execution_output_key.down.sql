-- Reverses 000020_dataset_version_execution_output_key.
-- Only the C2-a constraint is removed; DatasetVersion rows are historical facts
-- and are never deleted by a downgrade. Losing the index means the application
-- must again rely on the look-before-write path in the dataset writer, which is
-- the pre-C2-a behaviour.

DROP INDEX uq_dataset_version_execution_output;
