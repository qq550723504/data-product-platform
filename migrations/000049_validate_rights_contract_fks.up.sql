-- Validate the rights/contract foreign keys in a separate migration so the
-- NOT VALID constraints installed by 000006 can commit first and immediately
-- protect new writes even when historical rows still require remediation.

ALTER TABLE product_version
    VALIDATE CONSTRAINT fk_product_version_contract_version;

ALTER TABLE product_release
    VALIDATE CONSTRAINT fk_product_release_contract_version;

ALTER TABLE product_release
    VALIDATE CONSTRAINT fk_product_release_rights_snapshot;
