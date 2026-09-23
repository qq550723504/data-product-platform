LOCK TABLE product_version, product_asset IN ACCESS EXCLUSIVE MODE;

DO $guard_down$
BEGIN
    IF EXISTS (SELECT 1 FROM product_version) THEN
        RAISE EXCEPTION 'cannot roll back ProductVersion asset freeze while ProductVersion history exists';
    END IF;
END;
$guard_down$;

DROP TRIGGER IF EXISTS trg_product_version_finalized_on_commit ON product_version;
DROP TRIGGER IF EXISTS trg_product_asset_immutable_insert ON product_asset;
DROP TRIGGER IF EXISTS trg_product_version_immutable_update ON product_version;
DROP TRIGGER IF EXISTS trg_product_version_immutable_delete ON product_version;

DROP FUNCTION IF EXISTS require_product_version_finalized_on_commit();
DROP FUNCTION IF EXISTS guard_product_asset_insert();
DROP FUNCTION IF EXISTS guard_product_version_mutation();

ALTER TABLE product_version
    DROP CONSTRAINT IF EXISTS ck_product_version_build_status,
    DROP COLUMN IF EXISTS build_status;

CREATE OR REPLACE FUNCTION prevent_product_version_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'product_version is immutable; create a new version instead';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_product_version_immutable_update
BEFORE UPDATE ON product_version
FOR EACH ROW EXECUTE FUNCTION prevent_product_version_mutation();

CREATE TRIGGER trg_product_version_immutable_delete
BEFORE DELETE ON product_version
FOR EACH ROW EXECUTE FUNCTION prevent_product_version_mutation();
