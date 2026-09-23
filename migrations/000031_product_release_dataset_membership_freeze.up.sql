-- Freeze ProductRelease dataset membership at the same linearization boundary as publish.
--
-- Every membership mutation takes the parent ProductRelease row lock first.
-- Publish takes the same parent lock and verifies the exact membership before
-- changing READY -> PUBLISHED. Therefore a mutation either commits before the
-- publish lock (and publish must observe it) or waits until after publication
-- and is rejected.

CREATE OR REPLACE FUNCTION guard_product_release_dataset_membership()
RETURNS trigger AS $membership_guard$
DECLARE
    parent_release_id uuid;
    parent_status varchar(32);
BEGIN
    parent_release_id := CASE WHEN TG_OP = 'DELETE' THEN OLD.release_id ELSE NEW.release_id END;

    IF TG_OP = 'UPDATE' AND NEW.release_id IS DISTINCT FROM OLD.release_id THEN
        RAISE EXCEPTION 'product_release_dataset release_id is immutable';
    END IF;

    SELECT status
      INTO parent_status
      FROM product_release
     WHERE id=parent_release_id
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'product_release % does not exist', parent_release_id;
    END IF;

    IF parent_status IN ('PUBLISHED','WITHDRAWN') THEN
        RAISE EXCEPTION 'product_release % dataset membership is frozen in status %', parent_release_id, parent_status;
    END IF;

    RETURN CASE WHEN TG_OP = 'DELETE' THEN OLD ELSE NEW END;
END;
$membership_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_product_release_dataset_membership
BEFORE INSERT OR UPDATE OR DELETE ON product_release_dataset
FOR EACH ROW EXECUTE FUNCTION guard_product_release_dataset_membership();
