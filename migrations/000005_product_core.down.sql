DROP TRIGGER IF EXISTS trg_product_release_history ON product_release;
DROP FUNCTION IF EXISTS guard_product_release_history();
DROP TRIGGER IF EXISTS trg_product_asset_immutable_delete ON product_asset;
DROP TRIGGER IF EXISTS trg_product_asset_immutable_update ON product_asset;
DROP FUNCTION IF EXISTS prevent_product_asset_mutation();
DROP TRIGGER IF EXISTS trg_product_version_immutable_delete ON product_version;
DROP TRIGGER IF EXISTS trg_product_version_immutable_update ON product_version;
DROP FUNCTION IF EXISTS prevent_product_version_mutation();

ALTER TABLE IF EXISTS data_product DROP CONSTRAINT IF EXISTS fk_data_product_latest_release;
ALTER TABLE IF EXISTS data_product DROP CONSTRAINT IF EXISTS fk_data_product_current_version;

DROP TABLE IF EXISTS product_release_dataset;
DROP TABLE IF EXISTS product_release;
DROP TABLE IF EXISTS product_asset;
DROP TABLE IF EXISTS product_version;
DROP TABLE IF EXISTS data_product;
