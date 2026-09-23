-- Freeze DatasetVersion lineage once it contributes to a published ProductRelease.
--
-- Lineage remains buildable after an output DatasetVersion becomes READY because
-- the native engine currently records edges after publishing the output row.
-- The historical boundary is ProductRelease publication. Every lineage INSERT
-- and ProductRelease publish shares the existing workspace delivery fence before
-- any release/closure locks. This serializes even detached subtree construction
-- with publication and avoids relying on a potentially stale reachability query.
-- Existing lineage rows are append-only.

CREATE OR REPLACE FUNCTION guard_dataset_version_lineage_mutation()
RETURNS trigger AS $lineage_guard$
DECLARE
    output_workspace uuid;
    input_workspace uuid;
BEGIN
    IF TG_OP IN ('UPDATE','DELETE') THEN
        RAISE EXCEPTION 'dataset_version_lineage is append-only';
    END IF;

    SELECT d.workspace_id
      INTO output_workspace
      FROM dataset_version v
      JOIN dataset d ON d.id=v.dataset_id
     WHERE v.id=NEW.output_version_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'lineage output DatasetVersion % does not exist', NEW.output_version_id;
    END IF;

    SELECT d.workspace_id
      INTO input_workspace
      FROM dataset_version v
      JOIN dataset d ON d.id=v.dataset_id
     WHERE v.id=NEW.input_version_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'lineage input DatasetVersion % does not exist', NEW.input_version_id;
    END IF;

    -- Shared workspace fence for all lineage writes, including direct SQL.
    -- Publish acquires this same fence before locking ProductRelease or lineage,
    -- so the order is always fence -> release -> closure.
    INSERT INTO delivery_authorization_fence(workspace_id)
    VALUES (output_workspace)
    ON CONFLICT (workspace_id) DO NOTHING;

    PERFORM 1
      FROM delivery_authorization_fence
     WHERE workspace_id=output_workspace
     FOR UPDATE;

    -- Preserve AddLineage crash/replay idempotency under the same fence used by
    -- publish. This second-state check cannot race with publication or another
    -- lineage writer in the workspace.
    IF EXISTS (
        SELECT 1
        FROM dataset_version_lineage
        WHERE output_version_id=NEW.output_version_id
          AND input_version_id=NEW.input_version_id
          AND relation_type=NEW.relation_type
    ) THEN
        RETURN NEW;
    END IF;

    IF input_workspace <> output_workspace THEN
        RAISE EXCEPTION 'dataset_version_lineage crosses workspace boundary';
    END IF;

    -- The workspace fence makes this reachability snapshot stable: no other
    -- lineage INSERT in the workspace and no ProductRelease publication in the
    -- workspace can cross this check until the transaction ends.
    IF EXISTS (
        WITH RECURSIVE published_lineage(version_id) AS (
            SELECT prd.dataset_version_id
              FROM product_release pr
              JOIN data_product p ON p.id=pr.product_id
              JOIN product_release_dataset prd ON prd.release_id=pr.id
             WHERE p.workspace_id=output_workspace
               AND pr.status IN ('PUBLISHED','WITHDRAWN')
            UNION
            SELECT dvl.input_version_id
              FROM dataset_version_lineage dvl
              JOIN published_lineage pl ON pl.version_id=dvl.output_version_id
        )
        SELECT 1
          FROM published_lineage
         WHERE version_id=NEW.output_version_id
    ) THEN
        RAISE EXCEPTION 'dataset_version_lineage for published release history is frozen';
    END IF;

    RETURN NEW;
END;
$lineage_guard$ LANGUAGE plpgsql;

CREATE TRIGGER trg_dataset_version_lineage_immutable
BEFORE INSERT OR UPDATE OR DELETE ON dataset_version_lineage
FOR EACH ROW EXECUTE FUNCTION guard_dataset_version_lineage_mutation();


CREATE OR REPLACE FUNCTION guard_product_release_history()
RETURNS trigger AS $release_history_guard$
DECLARE
    publish_permit text;
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

    IF OLD.status = 'READY' AND NEW.status = 'PUBLISHED' THEN
        publish_permit := current_setting('app.product_release_publish_id', true);
        IF publish_permit IS DISTINCT FROM OLD.id::text THEN
            RAISE EXCEPTION 'ProductRelease publication requires fenced publish command';
        END IF;
    END IF;

    RETURN NEW;
END;
$release_history_guard$ LANGUAGE plpgsql;
