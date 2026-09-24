-- Refuse to collapse resource-only Gold history back into the old
-- DatasetVersion-only membership model.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
          FROM effective_rights_input
         WHERE dependency_kind='ANNOTATION_CONTRIBUTION_RESOURCE'
    ) THEN
        RAISE EXCEPTION 'refusing rollback: Effective Rights contains Gold annotation contribution dependencies';
    END IF;
END
$$;

DROP INDEX IF EXISTS uq_effective_rights_input_resource_only;
DROP INDEX IF EXISTS uq_effective_rights_input_dataset_version;

ALTER TABLE effective_rights_input
    DROP CONSTRAINT IF EXISTS ck_effective_rights_input_typed_identity,
    DROP CONSTRAINT IF EXISTS ck_effective_rights_input_dependency_kind;

ALTER TABLE effective_rights_input
    ALTER COLUMN input_dataset_version_id SET NOT NULL;

ALTER TABLE effective_rights_input
    ADD CONSTRAINT uq_effective_rights_input UNIQUE(snapshot_id, input_dataset_version_id);

ALTER TABLE effective_rights_input
    DROP COLUMN dependency_kind;
