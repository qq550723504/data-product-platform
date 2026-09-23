LOCK TABLE product_release, product_release_dataset, dataset_version, dataset_version_lineage IN ACCESS EXCLUSIVE MODE;

DO $lineage_guard_down$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM product_release
        WHERE status IN ('PUBLISHED','WITHDRAWN')
    ) THEN
        RAISE EXCEPTION 'cannot roll back DatasetVersion lineage freeze while published/withdrawn releases exist';
    END IF;
END;
$lineage_guard_down$;

DROP TRIGGER IF EXISTS trg_dataset_version_lineage_immutable ON dataset_version_lineage;
DROP FUNCTION IF EXISTS guard_dataset_version_lineage_mutation();
