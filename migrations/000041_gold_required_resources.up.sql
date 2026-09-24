-- #207: Effective Rights must freeze typed required-resource dependencies.
-- Dataset lineage leaves remain DATASET_VERSION dependencies. Gold annotation
-- contribution resources are RESOURCE_ONLY dependencies and must never be
-- represented by a fake DatasetVersion identity.

ALTER TABLE effective_rights_input
    ADD COLUMN dependency_kind varchar(64);

UPDATE effective_rights_input
   SET dependency_kind='DATASET_VERSION'
 WHERE dependency_kind IS NULL;

ALTER TABLE effective_rights_input
    ALTER COLUMN dependency_kind SET NOT NULL,
    ALTER COLUMN input_dataset_version_id DROP NOT NULL;

ALTER TABLE effective_rights_input
    DROP CONSTRAINT uq_effective_rights_input;

ALTER TABLE effective_rights_input
    ADD CONSTRAINT ck_effective_rights_input_dependency_kind CHECK (
        dependency_kind IN ('DATASET_VERSION','ANNOTATION_CONTRIBUTION_RESOURCE')
    ),
    ADD CONSTRAINT ck_effective_rights_input_typed_identity CHECK (
        (dependency_kind='DATASET_VERSION' AND input_dataset_version_id IS NOT NULL)
        OR
        (dependency_kind='ANNOTATION_CONTRIBUTION_RESOURCE' AND input_dataset_version_id IS NULL)
    );

CREATE UNIQUE INDEX uq_effective_rights_input_dataset_version
    ON effective_rights_input(snapshot_id, input_dataset_version_id)
    WHERE dependency_kind='DATASET_VERSION';

CREATE UNIQUE INDEX uq_effective_rights_input_resource_only
    ON effective_rights_input(snapshot_id, dependency_kind, data_resource_id)
    WHERE dependency_kind='ANNOTATION_CONTRIBUTION_RESOURCE';
