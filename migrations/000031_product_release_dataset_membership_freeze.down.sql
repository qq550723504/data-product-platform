LOCK TABLE product_release, product_release_dataset IN ACCESS EXCLUSIVE MODE;

DO $guard_down$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM product_release
        WHERE status IN ('PUBLISHED','WITHDRAWN')
    ) THEN
        RAISE EXCEPTION 'cannot roll back ProductRelease membership freeze while published/withdrawn releases exist';
    END IF;
END;
$guard_down$;

DROP TRIGGER IF EXISTS trg_product_release_dataset_membership ON product_release_dataset;
DROP FUNCTION IF EXISTS guard_product_release_dataset_membership();
