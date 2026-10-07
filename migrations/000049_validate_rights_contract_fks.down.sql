-- Restore the pre-validation staged state while preserving enforcement for
-- new and updated rows.

ALTER TABLE product_version
    DROP CONSTRAINT fk_product_version_contract_version;
ALTER TABLE product_version
    ADD CONSTRAINT fk_product_version_contract_version
    FOREIGN KEY (contract_version_id) REFERENCES contract_version(id) NOT VALID;

ALTER TABLE product_release
    DROP CONSTRAINT fk_product_release_contract_version;
ALTER TABLE product_release
    ADD CONSTRAINT fk_product_release_contract_version
    FOREIGN KEY (contract_version_id) REFERENCES contract_version(id) NOT VALID;

ALTER TABLE product_release
    DROP CONSTRAINT fk_product_release_rights_snapshot;
ALTER TABLE product_release
    ADD CONSTRAINT fk_product_release_rights_snapshot
    FOREIGN KEY (rights_snapshot_id) REFERENCES rights_snapshot(id) NOT VALID;
