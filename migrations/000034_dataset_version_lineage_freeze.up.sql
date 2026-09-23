-- Freeze DatasetVersion lineage once it contributes to a published ProductRelease.
--
-- Lineage remains buildable after an output DatasetVersion becomes READY because
-- the native engine currently records edges after publishing the output row.
-- The historical boundary is ProductRelease publication: publish locks the
-- complete currently reachable lineage closure, while every lineage INSERT locks
-- its output DatasetVersion first. Therefore an edge either commits before
-- publish (and is included by fresh readiness/trace) or waits until publication
-- and is rejected. Existing lineage rows are append-only.

CREATE OR REPLACE FUNCTION guard_dataset_version_lineage_mutation()
RETURNS trigger AS $lineage_guard$
DECLARE
    output_id uuid;
    parent record;
BEGIN
    IF TG_OP IN ('UPDATE','DELETE') THEN
        RAISE EXCEPTION 'dataset_version_lineage is append-only';
    END IF;

    output_id := NEW.output_version_id;

    -- A lineage INSERT that can change a READY release's ancestry must share the
    -- ProductRelease parent fence with publish. Lock the affected releases first,
    -- in deterministic order, before touching DatasetVersion rows. Publish already
    -- locks its ProductRelease before computing/locking the lineage closure, so a
    -- blocked publisher only evaluates that closure after any prior mutation
    -- commits. Conversely, a mutation arriving after publish waits here and then
    -- observes PUBLISHED/WITHDRAWN and fails closed.
    FOR parent IN
        WITH RECURSIVE release_lineage(release_id, version_id) AS (
            SELECT pr.id, prd.dataset_version_id
            FROM product_release pr
            JOIN product_release_dataset prd ON prd.release_id=pr.id
            WHERE pr.status IN ('READY','PUBLISHED','WITHDRAWN')
            UNION
            SELECT rl.release_id, dvl.input_version_id
            FROM release_lineage rl
            JOIN dataset_version_lineage dvl ON dvl.output_version_id=rl.version_id
        )
        SELECT pr.id, pr.status
        FROM product_release pr
        JOIN (
            SELECT DISTINCT release_id
            FROM release_lineage
            WHERE version_id=output_id
        ) affected ON affected.release_id=pr.id
        WHERE pr.status IN ('READY','PUBLISHED','WITHDRAWN')
        ORDER BY pr.id
        FOR UPDATE OF pr
    LOOP
        IF parent.status IN ('PUBLISHED','WITHDRAWN') THEN
            RAISE EXCEPTION 'dataset_version_lineage for published release history is frozen';
        END IF;
    END LOOP;

    PERFORM 1
    FROM dataset_version
    WHERE id=output_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'lineage output DatasetVersion % does not exist', output_id;
    END IF;

    RETURN NEW;
END;
$lineage_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dataset_version_lineage_immutable
BEFORE INSERT OR UPDATE OR DELETE ON dataset_version_lineage
FOR EACH ROW EXECUTE FUNCTION guard_dataset_version_lineage_mutation();
