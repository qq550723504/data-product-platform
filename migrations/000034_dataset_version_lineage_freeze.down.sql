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


CREATE OR REPLACE FUNCTION guard_product_release_history()
RETURNS trigger AS $release_history_guard$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'product_release is historical and cannot be deleted';
    END IF;

    IF OLD.status IN ('PUBLISHED','WITHDRAWN') THEN
        RAISE EXCEPTION 'product_release in status % is immutable', OLD.status;
    END IF;

    IF OLD.product_id IS DISTINCT FROM NEW.product_id OR
       OLD.product_version_id IS DISTINCT FROM NEW.product_version_id OR
       OLD.release_no IS DISTINCT FROM NEW.release_no THEN
        RAISE EXCEPTION 'product_release identity is immutable';
    END IF;

    RETURN NEW;
END;
$release_history_guard$ LANGUAGE plpgsql;
