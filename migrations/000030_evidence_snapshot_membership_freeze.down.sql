DROP TRIGGER IF EXISTS trg_evidence_snapshot_item_membership ON evidence_snapshot_item;
DROP FUNCTION IF EXISTS guard_evidence_snapshot_membership();

-- Restore the pre-000030 header guard before removing the lifecycle column so
-- rollback never leaves EvidenceSnapshot mutable.
CREATE OR REPLACE FUNCTION prevent_evidence_snapshot_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'evidence_snapshot is immutable';
END;
$$ LANGUAGE plpgsql;

ALTER TABLE evidence_snapshot
    DROP COLUMN IF EXISTS status;
