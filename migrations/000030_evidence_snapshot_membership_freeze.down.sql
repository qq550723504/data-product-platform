LOCK TABLE evidence_snapshot, evidence_snapshot_item IN ACCESS EXCLUSIVE MODE;

-- Dropping manifest_hash_payload after a new snapshot has been created would
-- discard the exact bytes that define its root hash. Refuse that destructive
-- rollback rather than silently weakening historical verification.
DO $down_guard$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM evidence_snapshot
        WHERE manifest_hash_payload IS NOT NULL
    ) THEN
        RAISE EXCEPTION 'cannot remove EvidenceSnapshot lifecycle while hash payload facts exist';
    END IF;
END;
$down_guard$;

DROP TRIGGER IF EXISTS trg_evidence_snapshot_item_membership ON evidence_snapshot_item;
DROP FUNCTION IF EXISTS guard_evidence_snapshot_membership();

DROP TRIGGER IF EXISTS trg_evidence_snapshot_finalized_on_commit ON evidence_snapshot;
DROP FUNCTION IF EXISTS require_evidence_snapshot_finalized_on_commit();

DROP TRIGGER IF EXISTS trg_evidence_snapshot_insert_guard ON evidence_snapshot;
DROP FUNCTION IF EXISTS guard_evidence_snapshot_insert();

DROP TRIGGER IF EXISTS trg_evidence_snapshot_immutable_update ON evidence_snapshot;

-- Restore the pre-000030 header guard before removing the lifecycle columns so
-- rollback never leaves EvidenceSnapshot mutable.
CREATE OR REPLACE FUNCTION prevent_evidence_snapshot_mutation()
RETURNS trigger AS $snapshot_guard$
BEGIN
    RAISE EXCEPTION 'evidence_snapshot is immutable';
END;
$snapshot_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_evidence_snapshot_immutable_update
BEFORE UPDATE OR DELETE ON evidence_snapshot
FOR EACH ROW EXECUTE FUNCTION prevent_evidence_snapshot_mutation();

ALTER TABLE evidence_snapshot
    DROP CONSTRAINT IF EXISTS ck_evidence_snapshot_root_hash,
    DROP CONSTRAINT IF EXISTS ck_evidence_snapshot_status,
    DROP COLUMN IF EXISTS manifest_hash_payload,
    DROP COLUMN IF EXISTS status;
