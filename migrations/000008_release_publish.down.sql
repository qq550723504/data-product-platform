DROP TRIGGER IF EXISTS trg_evidence_snapshot_immutable_update ON evidence_snapshot;
DROP FUNCTION IF EXISTS prevent_evidence_snapshot_mutation();

ALTER TABLE IF EXISTS product_release DROP CONSTRAINT IF EXISTS fk_product_release_evidence_snapshot;

DROP TABLE IF EXISTS command_idempotency;
DROP TABLE IF EXISTS evidence_snapshot_item;
DROP TABLE IF EXISTS evidence_snapshot;
