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
BEGIN
    IF TG_OP IN ('UPDATE','DELETE') THEN
        RAISE EXCEPTION 'dataset_version_lineage is append-only';
    END IF;

    output_id := NEW.output_version_id;

    PERFORM 1
    FROM dataset_version
    WHERE id=output_id
    FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'lineage output DatasetVersion % does not exist', output_id;
    END IF;

    IF EXISTS (
        WITH RECURSIVE published_lineage(version_id) AS (
            SELECT prd.dataset_version_id
            FROM product_release pr
            JOIN product_release_dataset prd ON prd.release_id=pr.id
            WHERE pr.status IN ('PUBLISHED','WITHDRAWN')
            UNION
            SELECT dvl.input_version_id
            FROM dataset_version_lineage dvl
            JOIN published_lineage pl ON pl.version_id=dvl.output_version_id
        )
        SELECT 1
        FROM published_lineage
        WHERE version_id=output_id
    ) THEN
        RAISE EXCEPTION 'dataset_version_lineage for published release history is frozen';
    END IF;

    RETURN NEW;
END;
$lineage_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dataset_version_lineage_immutable
BEFORE INSERT OR UPDATE OR DELETE ON dataset_version_lineage
FOR EACH ROW EXECUTE FUNCTION guard_dataset_version_lineage_mutation();
