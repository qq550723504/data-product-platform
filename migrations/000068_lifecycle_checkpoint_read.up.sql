-- Maintenance verification reuses the existing physical-call/cost journal.
-- It does not admit a business reader or mutable custody intent.
ALTER TABLE lifecycle_storage_call DROP CONSTRAINT lifecycle_storage_call_check;
ALTER TABLE lifecycle_storage_call ADD CONSTRAINT lifecycle_storage_call_check CHECK (
 (operation IN ('STAT_EXACT','INSPECT_RETENTION','INSPECT_LEGAL_HOLD','LIST_NAMESPACE_VERSIONS','READ_CHECKPOINT_EXACT') AND copy_operation_id IS NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NULL)
 OR (operation IN ('ARCHIVE_COPY','RESTORE_COPY','VERIFY_COPY_BYTES','RECOVER_COPY_VERSIONS','RECOVER_COPY_EXACT') AND copy_operation_id IS NOT NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NULL)
 OR (operation='READ_PINNED' AND copy_operation_id IS NULL AND reader_pin_id IS NOT NULL AND processing_output_intent_id IS NULL)
 OR (operation='PROCESSING_PUT' AND copy_operation_id IS NULL AND reader_pin_id IS NULL AND processing_output_intent_id IS NOT NULL)
);
