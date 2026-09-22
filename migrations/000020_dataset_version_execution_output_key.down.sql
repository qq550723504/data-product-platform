-- Reverses 000020_dataset_version_execution_output_key.
-- Only the live-output uniqueness constraint is removed; DatasetVersion facts
-- remain untouched.

DROP INDEX uq_dataset_version_execution_output;
