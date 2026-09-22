-- Freeze EvidenceSnapshot header and membership as one aggregate.
--
-- Existing snapshots predate the explicit lifecycle but are already intended to
-- be immutable, so they enter the current contract as FINALIZED. New snapshots
-- are created BUILDING, accept membership only while holding the parent row
-- lock, and transition exactly once to FINALIZED in the same business
-- transaction.

ALTER TABLE evidence_snapshot
    ADD COLUMN status varchar(16) NOT NULL DEFAULT 'FINALIZED',
    ADD CONSTRAINT ck_evidence_snapshot_status
        CHECK (status IN ('BUILDING','FINALIZED'));

CREATE OR REPLACE FUNCTION prevent_evidence_snapshot_mutation()
RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'evidence_snapshot is immutable';
    END IF;

    -- The only legal header mutation is the one-way seal operation. Content and
    -- identity are frozen even while BUILDING.
    IF OLD.status = 'BUILDING' AND NEW.status = 'FINALIZED' THEN
        IF NEW.id IS DISTINCT FROM OLD.id OR
           NEW.workspace_id IS DISTINCT FROM OLD.workspace_id OR
           NEW.object_type IS DISTINCT FROM OLD.object_type OR
           NEW.object_id IS DISTINCT FROM OLD.object_id OR
           NEW.manifest IS DISTINCT FROM OLD.manifest OR
           NEW.root_hash IS DISTINCT FROM OLD.root_hash OR
           NEW.created_at IS DISTINCT FROM OLD.created_at OR
           NEW.created_by IS DISTINCT FROM OLD.created_by THEN
            RAISE EXCEPTION 'evidence_snapshot content is immutable';
        END IF;
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'evidence_snapshot is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION require_evidence_snapshot_finalized_on_commit()
RETURNS trigger AS $finalize$
DECLARE
    current_status varchar(16);
BEGIN
    SELECT status INTO current_status
      FROM evidence_snapshot
     WHERE id = NEW.id;

    IF current_status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'evidence_snapshot % must be FINALIZED before commit', NEW.id;
    END IF;
    RETURN NULL;
END;
$finalize$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_evidence_snapshot_finalized_on_commit
AFTER INSERT ON evidence_snapshot
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_evidence_snapshot_finalized_on_commit();

CREATE OR REPLACE FUNCTION guard_evidence_snapshot_membership()
RETURNS trigger AS $$
DECLARE
    parent_status varchar(16);
BEGIN
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'evidence_snapshot membership is immutable';
    END IF;

    -- Parent-first lock is the linearization fence between the last membership
    -- insert and BUILDING -> FINALIZED. A concurrent finalizer must wait for an
    -- in-flight insert, and a later insert observes FINALIZED and fails.
    SELECT status
      INTO parent_status
      FROM evidence_snapshot
     WHERE id = NEW.snapshot_id
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'evidence_snapshot % does not exist', NEW.snapshot_id;
    END IF;
    IF parent_status <> 'BUILDING' THEN
        RAISE EXCEPTION 'evidence_snapshot % membership is finalized', NEW.snapshot_id;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_evidence_snapshot_item_membership
BEFORE INSERT OR UPDATE OR DELETE ON evidence_snapshot_item
FOR EACH ROW EXECUTE FUNCTION guard_evidence_snapshot_membership();
