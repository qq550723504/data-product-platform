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
              JOIN product_release_dataset prd ON prd.release_id=pr.id
             WHERE pr.status IN ('PUBLISHED','WITHDRAWN')
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
    release_workspace uuid;
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

    IF NEW.status = 'WITHDRAWN' AND OLD.status IS DISTINCT FROM 'WITHDRAWN' THEN
        RAISE EXCEPTION 'ProductRelease WITHDRAWN transition is not implemented';
    END IF;

    IF NEW.status = 'PUBLISHED' AND OLD.status IS DISTINCT FROM 'PUBLISHED' THEN
        IF OLD.status <> 'READY' THEN
            RAISE EXCEPTION 'ProductRelease can only transition to PUBLISHED from READY';
        END IF;

        SELECT p.workspace_id
          INTO release_workspace
          FROM data_product p
         WHERE p.id=OLD.product_id;

        IF release_workspace IS NULL THEN
            RAISE EXCEPTION 'ProductRelease product workspace does not exist';
        END IF;

        -- Application publication acquires this workspace fence before touching
        -- ProductRelease. A direct UPDATE already owns the ProductRelease row,
        -- so the trigger must never block waiting for the fence or it could invert
        -- the lock order. NOWAIT makes direct SQL fail closed under contention,
        -- while the normal fenced command succeeds because it already owns the row.
        BEGIN
            PERFORM 1
              FROM delivery_authorization_fence
             WHERE workspace_id=release_workspace
             FOR UPDATE NOWAIT;

            IF NOT FOUND THEN
                RAISE EXCEPTION 'ProductRelease publication requires initialized workspace delivery fence';
            END IF;
        EXCEPTION
            WHEN lock_not_available THEN
                RAISE EXCEPTION 'ProductRelease publication conflicts with workspace delivery fence';
        END;
    END IF;

    RETURN NEW;
END;
$release_history_guard$ LANGUAGE plpgsql;
