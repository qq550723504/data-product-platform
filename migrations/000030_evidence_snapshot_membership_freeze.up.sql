-- Freeze EvidenceSnapshot header and membership as one aggregate.
--
-- Existing snapshots predate the explicit lifecycle but are already intended to
-- be immutable, so they are backfilled to FINALIZED. New snapshots must enter
-- through BUILDING and carry the exact JSON bytes used to compute root_hash.
-- The aggregate is sealed only after persisted membership matches the manifest.

LOCK TABLE evidence_snapshot, evidence_snapshot_item IN ACCESS EXCLUSIVE MODE;

-- Migration 000008 installed this UPDATE/DELETE trigger with an unconditional
-- immutable function. Remove the trigger inside this migration transaction so
-- existing historical rows can receive the lifecycle backfill. PostgreSQL DDL
-- is transactional, and the stronger guard is recreated below before commit.
DROP TRIGGER IF EXISTS trg_evidence_snapshot_immutable_update ON evidence_snapshot;

ALTER TABLE evidence_snapshot
    ADD COLUMN status varchar(16),
    ADD COLUMN manifest_hash_payload bytea;

UPDATE evidence_snapshot
   SET status='FINALIZED'
 WHERE status IS NULL;

ALTER TABLE evidence_snapshot
    ALTER COLUMN status SET DEFAULT 'BUILDING',
    ALTER COLUMN status SET NOT NULL,
    ADD CONSTRAINT ck_evidence_snapshot_status
        CHECK (status IN ('BUILDING','FINALIZED')),
    ADD CONSTRAINT ck_evidence_snapshot_root_hash
        CHECK (root_hash ~ '^[0-9a-f]{64}$');

CREATE OR REPLACE FUNCTION guard_evidence_snapshot_insert()
RETURNS trigger AS $insert_guard$
DECLARE
    payload_manifest jsonb;
    computed_hash varchar(64);
BEGIN
    IF NEW.status IS DISTINCT FROM 'BUILDING' THEN
        RAISE EXCEPTION 'new evidence_snapshot must start BUILDING';
    END IF;

    IF NEW.manifest_hash_payload IS NULL THEN
        RAISE EXCEPTION 'evidence_snapshot manifest hash payload is required';
    END IF;

    payload_manifest := convert_from(NEW.manifest_hash_payload, 'UTF8')::jsonb;
    IF payload_manifest IS DISTINCT FROM NEW.manifest THEN
        RAISE EXCEPTION 'evidence_snapshot hash payload does not match manifest';
    END IF;

    computed_hash := encode(digest(NEW.manifest_hash_payload, 'sha256'), 'hex');
    IF computed_hash IS DISTINCT FROM NEW.root_hash THEN
        RAISE EXCEPTION 'evidence_snapshot root hash does not match manifest payload';
    END IF;

    RETURN NEW;
END;
$insert_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_evidence_snapshot_insert_guard
BEFORE INSERT ON evidence_snapshot
FOR EACH ROW EXECUTE FUNCTION guard_evidence_snapshot_insert();

CREATE OR REPLACE FUNCTION prevent_evidence_snapshot_mutation()
RETURNS trigger AS $snapshot_guard$
DECLARE
    persisted_items jsonb;
    payload_manifest jsonb;
    computed_hash varchar(64);
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'evidence_snapshot is immutable';
    END IF;

    -- The only legal header mutation is the one-way seal operation. Content,
    -- producer hash bytes and identity are frozen even while BUILDING.
    IF OLD.status = 'BUILDING' AND NEW.status = 'FINALIZED' THEN
        IF NEW.id IS DISTINCT FROM OLD.id OR
           NEW.workspace_id IS DISTINCT FROM OLD.workspace_id OR
           NEW.object_type IS DISTINCT FROM OLD.object_type OR
           NEW.object_id IS DISTINCT FROM OLD.object_id OR
           NEW.manifest IS DISTINCT FROM OLD.manifest OR
           NEW.root_hash IS DISTINCT FROM OLD.root_hash OR
           NEW.manifest_hash_payload IS DISTINCT FROM OLD.manifest_hash_payload OR
           NEW.created_at IS DISTINCT FROM OLD.created_at OR
           NEW.created_by IS DISTINCT FROM OLD.created_by THEN
            RAISE EXCEPTION 'evidence_snapshot content is immutable';
        END IF;

        IF NEW.manifest_hash_payload IS NULL THEN
            RAISE EXCEPTION 'evidence_snapshot manifest hash payload is required';
        END IF;

        payload_manifest := convert_from(NEW.manifest_hash_payload, 'UTF8')::jsonb;
        IF payload_manifest IS DISTINCT FROM NEW.manifest THEN
            RAISE EXCEPTION 'evidence_snapshot hash payload does not match manifest';
        END IF;

        computed_hash := encode(digest(NEW.manifest_hash_payload, 'sha256'), 'hex');
        IF computed_hash IS DISTINCT FROM NEW.root_hash THEN
            RAISE EXCEPTION 'evidence_snapshot root hash does not match manifest payload';
        END IF;

        IF jsonb_typeof(NEW.manifest->'evidenceItems') IS DISTINCT FROM 'array' THEN
            RAISE EXCEPTION 'evidence_snapshot manifest must contain evidenceItems array';
        END IF;

        SELECT COALESCE(
                   jsonb_agg(
                       jsonb_build_object(
                           'evidenceId', evidence_id::text,
                           'category', category
                       )
                       ORDER BY category, evidence_id::text
                   ),
                   '[]'::jsonb
               )
          INTO persisted_items
          FROM evidence_snapshot_item
         WHERE snapshot_id=OLD.id;

        IF NEW.manifest->'evidenceItems' IS DISTINCT FROM persisted_items THEN
            RAISE EXCEPTION 'evidence_snapshot membership does not match manifest';
        END IF;

        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'evidence_snapshot is immutable';
END;
$snapshot_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_evidence_snapshot_immutable_update
BEFORE UPDATE OR DELETE ON evidence_snapshot
FOR EACH ROW EXECUTE FUNCTION prevent_evidence_snapshot_mutation();

CREATE OR REPLACE FUNCTION require_evidence_snapshot_finalized_on_commit()
RETURNS trigger AS $finalize_guard$
DECLARE
    current_status varchar(16);
BEGIN
    SELECT status
      INTO current_status
      FROM evidence_snapshot
     WHERE id=NEW.id;

    IF current_status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'evidence_snapshot % must be FINALIZED before commit', NEW.id;
    END IF;

    RETURN NULL;
END;
$finalize_guard$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_evidence_snapshot_finalized_on_commit
AFTER INSERT ON evidence_snapshot
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_evidence_snapshot_finalized_on_commit();

CREATE OR REPLACE FUNCTION guard_evidence_snapshot_membership()
RETURNS trigger AS $membership_guard$
DECLARE
    parent_status varchar(16);
BEGIN
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'evidence_snapshot membership is immutable';
    END IF;

    -- Parent-first lock is the linearization fence between the last membership
    -- insert and BUILDING -> FINALIZED. A concurrent finalizer waits for an
    -- in-flight insert; an insert after finalization observes FINALIZED and fails.
    SELECT status
      INTO parent_status
      FROM evidence_snapshot
     WHERE id=NEW.snapshot_id
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'evidence_snapshot % does not exist', NEW.snapshot_id;
    END IF;

    IF parent_status <> 'BUILDING' THEN
        RAISE EXCEPTION 'evidence_snapshot % membership is finalized', NEW.snapshot_id;
    END IF;

    RETURN NEW;
END;
$membership_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_evidence_snapshot_item_membership
BEFORE INSERT OR UPDATE OR DELETE ON evidence_snapshot_item
FOR EACH ROW EXECUTE FUNCTION guard_evidence_snapshot_membership();
