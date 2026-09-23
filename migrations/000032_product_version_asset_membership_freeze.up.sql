-- Freeze ProductVersion asset membership as part of ProductVersion creation.
--
-- This repository has no production/customer rows to preserve. Migration 000032
-- establishes the strict current contract only; existing development databases
-- with ProductVersion rows must be rebuilt instead of backfilled.
--
-- New application-created versions explicitly start BUILDING, insert all assets
-- while holding the parent row lock, then transition to FINALIZED in the same
-- transaction. BUILDING is not allowed to commit.

LOCK TABLE product_version, product_asset IN ACCESS EXCLUSIVE MODE;

DO $precondition$
BEGIN
    IF EXISTS (SELECT 1 FROM product_version) THEN
        RAISE EXCEPTION 'migration 000032 requires an empty product_version table; rebuild the development database';
    END IF;
END;
$precondition$;

DROP TRIGGER IF EXISTS trg_product_version_immutable_update ON product_version;
DROP TRIGGER IF EXISTS trg_product_version_immutable_delete ON product_version;

ALTER TABLE product_version
    ADD COLUMN build_status varchar(16) NOT NULL,
    ADD COLUMN expected_asset_count integer NOT NULL,
    ADD CONSTRAINT ck_product_version_build_status
        CHECK (build_status IN ('BUILDING','FINALIZED')),
    ADD CONSTRAINT ck_product_version_expected_asset_count
        CHECK (expected_asset_count >= 0);

CREATE OR REPLACE FUNCTION guard_product_version_mutation()
RETURNS trigger AS $version_guard$
DECLARE
    actual_asset_count integer;
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'product_version is immutable; create a new version instead';
    END IF;

    IF OLD.build_status = 'BUILDING' AND NEW.build_status = 'FINALIZED' THEN
        IF NEW.id IS DISTINCT FROM OLD.id OR
           NEW.product_id IS DISTINCT FROM OLD.product_id OR
           NEW.major_version IS DISTINCT FROM OLD.major_version OR
           NEW.minor_version IS DISTINCT FROM OLD.minor_version OR
           NEW.patch_version IS DISTINCT FROM OLD.patch_version OR
           NEW.workflow_version_id IS DISTINCT FROM OLD.workflow_version_id OR
           NEW.contract_version_id IS DISTINCT FROM OLD.contract_version_id OR
           NEW.entity_policy_ref IS DISTINCT FROM OLD.entity_policy_ref OR
           NEW.indicator_set_ref IS DISTINCT FROM OLD.indicator_set_ref OR
           NEW.definition_snapshot IS DISTINCT FROM OLD.definition_snapshot OR
           NEW.expected_asset_count IS DISTINCT FROM OLD.expected_asset_count OR
           NEW.created_at IS DISTINCT FROM OLD.created_at OR
           NEW.created_by IS DISTINCT FROM OLD.created_by THEN
            RAISE EXCEPTION 'product_version content is immutable';
        END IF;

        SELECT count(*)
          INTO actual_asset_count
          FROM product_asset
         WHERE product_version_id=OLD.id;

        IF actual_asset_count <> OLD.expected_asset_count THEN
            RAISE EXCEPTION 'product_version % asset membership count mismatch: expected %, got %',
                OLD.id, OLD.expected_asset_count, actual_asset_count;
        END IF;

        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'product_version is immutable; create a new version instead';
END;
$version_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_product_version_immutable_update
BEFORE UPDATE ON product_version
FOR EACH ROW EXECUTE FUNCTION guard_product_version_mutation();

CREATE TRIGGER trg_product_version_immutable_delete
BEFORE DELETE ON product_version
FOR EACH ROW EXECUTE FUNCTION guard_product_version_mutation();

CREATE OR REPLACE FUNCTION guard_product_asset_insert()
RETURNS trigger AS $asset_guard$
DECLARE
    parent_status varchar(16);
BEGIN
    SELECT build_status
      INTO parent_status
      FROM product_version
     WHERE id=NEW.product_version_id
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'product_version % does not exist', NEW.product_version_id;
    END IF;

    IF parent_status <> 'BUILDING' THEN
        RAISE EXCEPTION 'product_version % asset membership is finalized', NEW.product_version_id;
    END IF;

    RETURN NEW;
END;
$asset_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_product_asset_immutable_insert
BEFORE INSERT ON product_asset
FOR EACH ROW EXECUTE FUNCTION guard_product_asset_insert();

CREATE OR REPLACE FUNCTION require_product_version_finalized_on_commit()
RETURNS trigger AS $finalize_guard$
DECLARE
    current_status varchar(16);
BEGIN
    SELECT build_status
      INTO current_status
      FROM product_version
     WHERE id=NEW.id;

    IF current_status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'product_version % must be FINALIZED before commit', NEW.id;
    END IF;

    RETURN NULL;
END;
$finalize_guard$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_product_version_finalized_on_commit
AFTER INSERT OR UPDATE ON product_version
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_product_version_finalized_on_commit();
