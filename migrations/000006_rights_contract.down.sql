DROP TRIGGER IF EXISTS trg_rights_snapshot_immutable_delete ON rights_snapshot;
DROP TRIGGER IF EXISTS trg_rights_snapshot_immutable_update ON rights_snapshot;
DROP FUNCTION IF EXISTS prevent_rights_snapshot_mutation();
DROP TRIGGER IF EXISTS trg_contract_version_immutability ON contract_version;
DROP FUNCTION IF EXISTS guard_contract_version_immutability();

ALTER TABLE IF EXISTS product_release DROP CONSTRAINT IF EXISTS fk_product_release_rights_snapshot;
ALTER TABLE IF EXISTS product_release DROP CONSTRAINT IF EXISTS fk_product_release_contract_version;
ALTER TABLE IF EXISTS product_version DROP CONSTRAINT IF EXISTS fk_product_version_contract_version;

DROP TABLE IF EXISTS contract_version;
DROP TABLE IF EXISTS data_contract;
DROP TABLE IF EXISTS rights_snapshot_authorization;
DROP TABLE IF EXISTS rights_snapshot;
DROP TABLE IF EXISTS authorization_resource;
DROP TABLE IF EXISTS data_authorization;
