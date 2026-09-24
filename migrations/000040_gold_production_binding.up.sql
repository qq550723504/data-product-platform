-- #207 Gold candidate production binding.

-- target_period was an Enterprise Activity workflow concern, not a universal Execution identity.
-- Existing workflows remain period-required in application validation; Gold builder explicitly opts out.
ALTER TABLE execution ALTER COLUMN target_period DROP NOT NULL;

-- A Gold candidate is still a normal immutable DatasetVersion. This aggregate
-- freezes the exact producer/input/annotation facts that make that version a
-- Gold-produced version; it is not a second GoldDataset entity.

CREATE TABLE gold_build_request (
    execution_id                        uuid PRIMARY KEY REFERENCES execution(id),
    workspace_id                        uuid NOT NULL,
    input_dataset_version_id            uuid NOT NULL REFERENCES dataset_version(id),
    input_certification_id              uuid NOT NULL REFERENCES dataset_certification(id),
    annotation_campaign_id              uuid NOT NULL REFERENCES annotation_campaign(id),
    annotation_snapshot_id              uuid NOT NULL REFERENCES annotation_snapshot(id),
    annotation_contribution_resource_id uuid NOT NULL REFERENCES data_resource(id),
    snapshot_root_hash                  varchar(64) NOT NULL,
    request_fingerprint                 varchar(64) NOT NULL,
    created_at                          timestamptz NOT NULL DEFAULT now(),
    created_by                          uuid,
    CONSTRAINT fk_gold_build_request_execution
        FOREIGN KEY (workspace_id, execution_id) REFERENCES execution(workspace_id, id),
    CONSTRAINT ck_gold_build_request_hashes CHECK (
        snapshot_root_hash ~ '^[0-9a-f]{64}
    id                                  uuid PRIMARY KEY,
    workspace_id                        uuid NOT NULL,
    execution_id                        uuid NOT NULL UNIQUE REFERENCES execution(id),
    workflow_version_id                 uuid NOT NULL REFERENCES workflow_version(id),
    input_dataset_version_id            uuid NOT NULL REFERENCES dataset_version(id),
    input_certification_id              uuid NOT NULL REFERENCES dataset_certification(id),
    annotation_campaign_id              uuid NOT NULL REFERENCES annotation_campaign(id),
    annotation_snapshot_id              uuid NOT NULL REFERENCES annotation_snapshot(id),
    annotation_contribution_resource_id uuid NOT NULL REFERENCES data_resource(id),
    output_dataset_version_id           uuid NOT NULL UNIQUE REFERENCES dataset_version(id),

    input_checksum_sha256               varchar(64) NOT NULL,
    snapshot_root_hash                  varchar(64) NOT NULL,
    schema_content_sha256               varchar(64) NOT NULL,
    taxonomy_content_sha256             varchar(64) NOT NULL,
    rubric_content_sha256               varchar(64) NOT NULL,
    renderer_content_sha256             varchar(64) NOT NULL,
    review_policy_content_sha256        varchar(64) NOT NULL,
    output_checksum_sha256              varchar(64) NOT NULL,
    output_row_count                    bigint NOT NULL,

    manifest                            jsonb NOT NULL,
    manifest_hash_payload               bytea NOT NULL,
    root_hash                           varchar(64) NOT NULL,
    status                              varchar(16) NOT NULL DEFAULT 'BUILDING',
    created_at                          timestamptz NOT NULL DEFAULT now(),
    created_by                          uuid,
    finalized_at                        timestamptz,

    CONSTRAINT ck_gold_binding_status CHECK (status IN ('BUILDING','FINALIZED')),
    CONSTRAINT ck_gold_binding_output_count CHECK (output_row_count >= 0),
    CONSTRAINT ck_gold_binding_hashes CHECK (
        input_checksum_sha256 ~ '^[0-9a-f]{64}$'
        AND snapshot_root_hash ~ '^[0-9a-f]{64}$'
        AND schema_content_sha256 ~ '^[0-9a-f]{64}$'
        AND taxonomy_content_sha256 ~ '^[0-9a-f]{64}$'
        AND rubric_content_sha256 ~ '^[0-9a-f]{64}$'
        AND renderer_content_sha256 ~ '^[0-9a-f]{64}$'
        AND review_policy_content_sha256 ~ '^[0-9a-f]{64}$'
        AND output_checksum_sha256 ~ '^[0-9a-f]{64}$'
        AND root_hash ~ '^[0-9a-f]{64}$'
        AND encode(digest(manifest_hash_payload, 'sha256'), 'hex') = root_hash
        AND convert_from(manifest_hash_payload, 'UTF8')::jsonb = manifest
    )
);

CREATE INDEX idx_gold_binding_workspace
    ON gold_production_binding(workspace_id, created_at DESC, id);
CREATE INDEX idx_gold_binding_input
    ON gold_production_binding(input_dataset_version_id, created_at DESC, id);
CREATE INDEX idx_gold_binding_snapshot
    ON gold_production_binding(annotation_snapshot_id, created_at DESC, id);

CREATE TABLE gold_production_member (
    binding_id                 uuid NOT NULL REFERENCES gold_production_binding(id),
    task_id                    uuid NOT NULL REFERENCES annotation_task(id),
    source_item_ref            varchar(1024) NOT NULL,
    source_content_sha256      varchar(64) NOT NULL,
    decision_id                uuid NOT NULL REFERENCES annotation_review_decision(id),
    outcome                    varchar(16) NOT NULL,
    reviewed_result_id         uuid REFERENCES annotation_result(id),
    selected_result_id         uuid REFERENCES annotation_result(id),
    selected_result_sha256     varchar(64),
    output_row_index           integer,
    PRIMARY KEY(binding_id, task_id),
    CONSTRAINT uq_gold_binding_output_row UNIQUE(binding_id, output_row_index),
    CONSTRAINT ck_gold_member_outcome CHECK (outcome IN ('ACCEPT','REJECT','CORRECT')),
    CONSTRAINT ck_gold_member_source_hash CHECK (source_content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_gold_member_selected_hash CHECK (
        selected_result_sha256 IS NULL OR selected_result_sha256 ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT ck_gold_member_selection CHECK (
        (
            outcome IN ('ACCEPT','CORRECT')
            AND selected_result_id IS NOT NULL
            AND selected_result_sha256 IS NOT NULL
            AND output_row_index IS NOT NULL
            AND output_row_index >= 0
        )
        OR
        (
            outcome='REJECT'
            AND selected_result_id IS NULL
            AND selected_result_sha256 IS NULL
            AND output_row_index IS NULL
        )
    )
);

CREATE INDEX idx_gold_member_decision
    ON gold_production_member(decision_id);
CREATE INDEX idx_gold_member_selected_result
    ON gold_production_member(selected_result_id)
    WHERE selected_result_id IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_gold_binding_insert()
RETURNS trigger AS $$
BEGIN
    IF NEW.status <> 'BUILDING' OR NEW.finalized_at IS NOT NULL THEN
        RAISE EXCEPTION 'new gold production binding must start BUILDING';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_binding_insert
BEFORE INSERT ON gold_production_binding
FOR EACH ROW EXECUTE FUNCTION guard_gold_binding_insert();

CREATE OR REPLACE FUNCTION guard_gold_member_mutation()
RETURNS trigger AS $$
DECLARE
    binding_status varchar(16);
    binding_workspace uuid;
    binding_snapshot uuid;
    snapshot_task record;
    snapshot_decision record;
    snapshot_output uuid;
    result_task uuid;
    result_hash varchar(64);
BEGIN
    SELECT status, workspace_id, annotation_snapshot_id
      INTO binding_status, binding_workspace, binding_snapshot
      FROM gold_production_binding
     WHERE id=CASE WHEN TG_OP='DELETE' THEN OLD.binding_id ELSE NEW.binding_id END
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'gold production binding parent is not visible';
    END IF;
    IF binding_status <> 'BUILDING' THEN
        RAISE EXCEPTION 'gold production binding membership is finalized';
    END IF;
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'gold production binding membership is append-only';
    END IF;

    SELECT st.task_id, st.source_item_ref, st.source_content_sha256
      INTO snapshot_task
      FROM annotation_snapshot_task st
     WHERE st.snapshot_id=binding_snapshot AND st.task_id=NEW.task_id;

    IF snapshot_task.task_id IS NULL
       OR snapshot_task.source_item_ref IS DISTINCT FROM NEW.source_item_ref
       OR snapshot_task.source_content_sha256 IS DISTINCT FROM NEW.source_content_sha256 THEN
        RAISE EXCEPTION 'gold production member does not match frozen snapshot task';
    END IF;

    SELECT sd.decision_id, sd.outcome, sd.reviewed_result_id, sd.selected_result_id
      INTO snapshot_decision
      FROM annotation_snapshot_decision sd
     WHERE sd.snapshot_id=binding_snapshot AND sd.task_id=NEW.task_id;

    IF snapshot_decision.decision_id IS NULL
       OR snapshot_decision.decision_id IS DISTINCT FROM NEW.decision_id
       OR snapshot_decision.outcome IS DISTINCT FROM NEW.outcome
       OR snapshot_decision.reviewed_result_id IS DISTINCT FROM NEW.reviewed_result_id
       OR snapshot_decision.selected_result_id IS DISTINCT FROM NEW.selected_result_id THEN
        RAISE EXCEPTION 'gold production member does not match frozen snapshot decision';
    END IF;

    SELECT so.selected_result_id
      INTO snapshot_output
      FROM annotation_snapshot_output so
     WHERE so.snapshot_id=binding_snapshot AND so.task_id=NEW.task_id;

    IF NEW.outcome IN ('ACCEPT','CORRECT') THEN
        IF snapshot_output IS DISTINCT FROM NEW.selected_result_id THEN
            RAISE EXCEPTION 'gold production selected result does not match frozen snapshot output';
        END IF;
        SELECT r.task_id, r.canonical_payload_sha256
          INTO result_task, result_hash
          FROM annotation_result r
         WHERE r.id=NEW.selected_result_id;
        IF result_task IS DISTINCT FROM NEW.task_id
           OR result_hash IS DISTINCT FROM NEW.selected_result_sha256 THEN
            RAISE EXCEPTION 'gold production selected result identity/hash mismatch';
        END IF;
    ELSE
        IF snapshot_output IS NOT NULL THEN
            RAISE EXCEPTION 'gold production rejected task cannot have frozen output';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_member_insert
BEFORE INSERT ON gold_production_member
FOR EACH ROW EXECUTE FUNCTION guard_gold_member_mutation();

CREATE TRIGGER trg_gold_member_update
BEFORE UPDATE ON gold_production_member
FOR EACH ROW EXECUTE FUNCTION guard_gold_member_mutation();

CREATE TRIGGER trg_gold_member_delete
BEFORE DELETE ON gold_production_member
FOR EACH ROW EXECUTE FUNCTION guard_gold_member_mutation();

CREATE OR REPLACE FUNCTION guard_gold_binding_update()
RETURNS trigger AS $$
DECLARE
    execution_row record;
    input_bound boolean;
    input_row record;
    certification_row record;
    campaign_row record;
    snapshot_row record;
    output_row record;
    snapshot_task_count integer;
    member_count integer;
    selected_count integer;
BEGIN
    IF OLD.status <> 'BUILDING' OR NEW.status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'gold production binding is immutable after finalization';
    END IF;

    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.execution_id IS DISTINCT FROM OLD.execution_id
       OR NEW.workflow_version_id IS DISTINCT FROM OLD.workflow_version_id
       OR NEW.input_dataset_version_id IS DISTINCT FROM OLD.input_dataset_version_id
       OR NEW.input_certification_id IS DISTINCT FROM OLD.input_certification_id
       OR NEW.annotation_campaign_id IS DISTINCT FROM OLD.annotation_campaign_id
       OR NEW.annotation_snapshot_id IS DISTINCT FROM OLD.annotation_snapshot_id
       OR NEW.annotation_contribution_resource_id IS DISTINCT FROM OLD.annotation_contribution_resource_id
       OR NEW.output_dataset_version_id IS DISTINCT FROM OLD.output_dataset_version_id
       OR NEW.input_checksum_sha256 IS DISTINCT FROM OLD.input_checksum_sha256
       OR NEW.snapshot_root_hash IS DISTINCT FROM OLD.snapshot_root_hash
       OR NEW.schema_content_sha256 IS DISTINCT FROM OLD.schema_content_sha256
       OR NEW.taxonomy_content_sha256 IS DISTINCT FROM OLD.taxonomy_content_sha256
       OR NEW.rubric_content_sha256 IS DISTINCT FROM OLD.rubric_content_sha256
       OR NEW.renderer_content_sha256 IS DISTINCT FROM OLD.renderer_content_sha256
       OR NEW.review_policy_content_sha256 IS DISTINCT FROM OLD.review_policy_content_sha256
       OR NEW.output_checksum_sha256 IS DISTINCT FROM OLD.output_checksum_sha256
       OR NEW.output_row_count IS DISTINCT FROM OLD.output_row_count
       OR NEW.manifest IS DISTINCT FROM OLD.manifest
       OR NEW.manifest_hash_payload IS DISTINCT FROM OLD.manifest_hash_payload
       OR NEW.root_hash IS DISTINCT FROM OLD.root_hash
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.created_by IS DISTINCT FROM OLD.created_by
       OR NEW.finalized_at IS NULL THEN
        RAISE EXCEPTION 'gold production binding frozen semantics are immutable';
    END IF;

    SELECT e.workspace_id, e.workflow_version_id, e.output_dataset_id
      INTO execution_row
      FROM execution e
     WHERE e.id=NEW.execution_id;

    IF execution_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR execution_row.workflow_version_id IS DISTINCT FROM NEW.workflow_version_id THEN
        RAISE EXCEPTION 'gold production binding execution identity mismatch';
    END IF;

    IF NOT EXISTS (
        SELECT 1
          FROM gold_build_request r
         WHERE r.execution_id=NEW.execution_id
           AND r.workspace_id=NEW.workspace_id
           AND r.input_dataset_version_id=NEW.input_dataset_version_id
           AND r.input_certification_id=NEW.input_certification_id
           AND r.annotation_campaign_id=NEW.annotation_campaign_id
           AND r.annotation_snapshot_id=NEW.annotation_snapshot_id
           AND r.annotation_contribution_resource_id=NEW.annotation_contribution_resource_id
           AND r.snapshot_root_hash=NEW.snapshot_root_hash
    ) THEN
        RAISE EXCEPTION 'gold production binding does not match frozen build request';
    END IF;

    SELECT EXISTS(
        SELECT 1
          FROM execution_input ei
         WHERE ei.execution_id=NEW.execution_id
           AND ei.input_name='gold_input'
           AND ei.dataset_version_id=NEW.input_dataset_version_id
    ) INTO input_bound;
    IF NOT input_bound THEN
        RAISE EXCEPTION 'gold production binding requires exact gold_input execution binding';
    END IF;

    SELECT v.status, v.checksum_value
      INTO input_row
      FROM dataset_version v
     WHERE v.id=NEW.input_dataset_version_id;
    IF input_row.status NOT IN ('READY','SUPERSEDED')
       OR lower(input_row.checksum_value) IS DISTINCT FROM lower(NEW.input_checksum_sha256) THEN
        RAISE EXCEPTION 'gold production input DatasetVersion identity mismatch';
    END IF;

    SELECT c.workspace_id, c.dataset_version_id, c.decision
      INTO certification_row
      FROM dataset_certification c
     WHERE c.id=NEW.input_certification_id;
    IF certification_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR certification_row.dataset_version_id IS DISTINCT FROM NEW.input_dataset_version_id
       OR certification_row.decision <> 'CERTIFIED' THEN
        RAISE EXCEPTION 'gold production input certification mismatch';
    END IF;

    SELECT c.workspace_id, c.input_dataset_version_id, c.input_certification_id,
           c.annotation_contribution_resource_id,
           c.schema_content_sha256, c.taxonomy_content_sha256, c.rubric_content_sha256,
           c.renderer_content_sha256, c.review_policy_content_sha256
      INTO campaign_row
      FROM annotation_campaign c
     WHERE c.id=NEW.annotation_campaign_id;
    IF campaign_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR campaign_row.input_dataset_version_id IS DISTINCT FROM NEW.input_dataset_version_id
       OR campaign_row.input_certification_id IS DISTINCT FROM NEW.input_certification_id
       OR campaign_row.annotation_contribution_resource_id IS DISTINCT FROM NEW.annotation_contribution_resource_id
       OR campaign_row.schema_content_sha256 IS DISTINCT FROM NEW.schema_content_sha256
       OR campaign_row.taxonomy_content_sha256 IS DISTINCT FROM NEW.taxonomy_content_sha256
       OR campaign_row.rubric_content_sha256 IS DISTINCT FROM NEW.rubric_content_sha256
       OR campaign_row.renderer_content_sha256 IS DISTINCT FROM NEW.renderer_content_sha256
       OR campaign_row.review_policy_content_sha256 IS DISTINCT FROM NEW.review_policy_content_sha256 THEN
        RAISE EXCEPTION 'gold production campaign frozen contract mismatch';
    END IF;

    SELECT s.workspace_id, s.campaign_id, s.status, s.root_hash
      INTO snapshot_row
      FROM annotation_snapshot s
     WHERE s.id=NEW.annotation_snapshot_id;
    IF snapshot_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR snapshot_row.campaign_id IS DISTINCT FROM NEW.annotation_campaign_id
       OR snapshot_row.status <> 'FINALIZED'
       OR snapshot_row.root_hash IS DISTINCT FROM NEW.snapshot_root_hash THEN
        RAISE EXCEPTION 'gold production snapshot identity mismatch';
    END IF;

    SELECT d.workspace_id, v.dataset_id, v.status, v.generated_by_execution_id,
           v.checksum_value, COALESCE(v.row_count,0) AS row_count
      INTO output_row
      FROM dataset_version v
      JOIN dataset d ON d.id=v.dataset_id
     WHERE v.id=NEW.output_dataset_version_id;
    IF output_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR output_row.dataset_id IS DISTINCT FROM execution_row.output_dataset_id
       OR output_row.status <> 'READY'
       OR output_row.generated_by_execution_id IS DISTINCT FROM NEW.execution_id
       OR lower(output_row.checksum_value) IS DISTINCT FROM lower(NEW.output_checksum_sha256)
       OR output_row.row_count IS DISTINCT FROM NEW.output_row_count THEN
        RAISE EXCEPTION 'gold production output DatasetVersion identity mismatch';
    END IF;

    SELECT count(*) INTO snapshot_task_count
      FROM annotation_snapshot_task
     WHERE snapshot_id=NEW.annotation_snapshot_id;
    SELECT count(*), count(*) FILTER (WHERE selected_result_id IS NOT NULL)
      INTO member_count, selected_count
      FROM gold_production_member
     WHERE binding_id=NEW.id;

    IF member_count <> snapshot_task_count OR selected_count <> NEW.output_row_count THEN
        RAISE EXCEPTION 'gold production binding membership/output count mismatch';
    END IF;

    IF NEW.manifest->>'formatVersion' IS DISTINCT FROM 'gold-production-binding-v1'
       OR NEW.manifest->>'workspaceId' IS DISTINCT FROM NEW.workspace_id::text
       OR NEW.manifest->>'executionId' IS DISTINCT FROM NEW.execution_id::text
       OR NEW.manifest->>'workflowVersionId' IS DISTINCT FROM NEW.workflow_version_id::text
       OR NEW.manifest->>'inputDatasetVersionId' IS DISTINCT FROM NEW.input_dataset_version_id::text
       OR NEW.manifest->>'inputCertificationId' IS DISTINCT FROM NEW.input_certification_id::text
       OR NEW.manifest->>'annotationCampaignId' IS DISTINCT FROM NEW.annotation_campaign_id::text
       OR NEW.manifest->>'annotationSnapshotId' IS DISTINCT FROM NEW.annotation_snapshot_id::text
       OR NEW.manifest->>'annotationContributionResourceId' IS DISTINCT FROM NEW.annotation_contribution_resource_id::text
       OR NEW.manifest->>'outputDatasetVersionId' IS DISTINCT FROM NEW.output_dataset_version_id::text
       OR NEW.manifest->>'inputChecksumSha256' IS DISTINCT FROM NEW.input_checksum_sha256
       OR NEW.manifest->>'snapshotRootHash' IS DISTINCT FROM NEW.snapshot_root_hash
       OR NEW.manifest->>'outputChecksumSha256' IS DISTINCT FROM NEW.output_checksum_sha256
       OR (NEW.manifest->>'outputRowCount')::bigint IS DISTINCT FROM NEW.output_row_count THEN
        RAISE EXCEPTION 'gold production binding manifest header mismatch';
    END IF;

    IF NEW.manifest->'members' IS DISTINCT FROM (
        SELECT COALESCE(
            jsonb_agg(
                jsonb_strip_nulls(jsonb_build_object(
                    'taskId', m.task_id::text,
                    'sourceItemRef', m.source_item_ref,
                    'sourceContentSha256', m.source_content_sha256,
                    'decisionId', m.decision_id::text,
                    'outcome', m.outcome,
                    'reviewedResultId', m.reviewed_result_id::text,
                    'selectedResultId', m.selected_result_id::text,
                    'selectedResultSha256', m.selected_result_sha256,
                    'outputRowIndex', m.output_row_index
                )) ORDER BY m.task_id::text
            ), '[]'::jsonb
        )
        FROM gold_production_member m
        WHERE m.binding_id=NEW.id
    ) THEN
        RAISE EXCEPTION 'gold production binding manifest membership mismatch';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_binding_update
BEFORE UPDATE ON gold_production_binding
FOR EACH ROW EXECUTE FUNCTION guard_gold_binding_update();

CREATE OR REPLACE FUNCTION guard_gold_binding_delete()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'gold production binding is immutable history';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_binding_delete
BEFORE DELETE ON gold_production_binding
FOR EACH ROW EXECUTE FUNCTION guard_gold_binding_delete();

CREATE OR REPLACE FUNCTION require_gold_binding_finalized_on_commit()
RETURNS trigger AS $$
DECLARE
    current_status varchar(16);
BEGIN
    SELECT status INTO current_status FROM gold_production_binding WHERE id=NEW.id;
    IF current_status IS DISTINCT FROM 'FINALIZED' THEN
        RAISE EXCEPTION 'gold production binding % must be FINALIZED before commit', NEW.id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_gold_binding_require_finalized
AFTER INSERT ON gold_production_binding
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_gold_binding_finalized_on_commit();

        AND request_fingerprint ~ '^[0-9a-f]{64}
    id                                  uuid PRIMARY KEY,
    workspace_id                        uuid NOT NULL,
    execution_id                        uuid NOT NULL UNIQUE REFERENCES execution(id),
    workflow_version_id                 uuid NOT NULL REFERENCES workflow_version(id),
    input_dataset_version_id            uuid NOT NULL REFERENCES dataset_version(id),
    input_certification_id              uuid NOT NULL REFERENCES dataset_certification(id),
    annotation_campaign_id              uuid NOT NULL REFERENCES annotation_campaign(id),
    annotation_snapshot_id              uuid NOT NULL REFERENCES annotation_snapshot(id),
    annotation_contribution_resource_id uuid NOT NULL REFERENCES data_resource(id),
    output_dataset_version_id           uuid NOT NULL UNIQUE REFERENCES dataset_version(id),

    input_checksum_sha256               varchar(64) NOT NULL,
    snapshot_root_hash                  varchar(64) NOT NULL,
    schema_content_sha256               varchar(64) NOT NULL,
    taxonomy_content_sha256             varchar(64) NOT NULL,
    rubric_content_sha256               varchar(64) NOT NULL,
    renderer_content_sha256             varchar(64) NOT NULL,
    review_policy_content_sha256        varchar(64) NOT NULL,
    output_checksum_sha256              varchar(64) NOT NULL,
    output_row_count                    bigint NOT NULL,

    manifest                            jsonb NOT NULL,
    manifest_hash_payload               bytea NOT NULL,
    root_hash                           varchar(64) NOT NULL,
    status                              varchar(16) NOT NULL DEFAULT 'BUILDING',
    created_at                          timestamptz NOT NULL DEFAULT now(),
    created_by                          uuid,
    finalized_at                        timestamptz,

    CONSTRAINT ck_gold_binding_status CHECK (status IN ('BUILDING','FINALIZED')),
    CONSTRAINT ck_gold_binding_output_count CHECK (output_row_count >= 0),
    CONSTRAINT ck_gold_binding_hashes CHECK (
        input_checksum_sha256 ~ '^[0-9a-f]{64}$'
        AND snapshot_root_hash ~ '^[0-9a-f]{64}$'
        AND schema_content_sha256 ~ '^[0-9a-f]{64}$'
        AND taxonomy_content_sha256 ~ '^[0-9a-f]{64}$'
        AND rubric_content_sha256 ~ '^[0-9a-f]{64}$'
        AND renderer_content_sha256 ~ '^[0-9a-f]{64}$'
        AND review_policy_content_sha256 ~ '^[0-9a-f]{64}$'
        AND output_checksum_sha256 ~ '^[0-9a-f]{64}$'
        AND root_hash ~ '^[0-9a-f]{64}$'
        AND encode(digest(manifest_hash_payload, 'sha256'), 'hex') = root_hash
        AND convert_from(manifest_hash_payload, 'UTF8')::jsonb = manifest
    )
);

CREATE INDEX idx_gold_binding_workspace
    ON gold_production_binding(workspace_id, created_at DESC, id);
CREATE INDEX idx_gold_binding_input
    ON gold_production_binding(input_dataset_version_id, created_at DESC, id);
CREATE INDEX idx_gold_binding_snapshot
    ON gold_production_binding(annotation_snapshot_id, created_at DESC, id);

CREATE TABLE gold_production_member (
    binding_id                 uuid NOT NULL REFERENCES gold_production_binding(id),
    task_id                    uuid NOT NULL REFERENCES annotation_task(id),
    source_item_ref            varchar(1024) NOT NULL,
    source_content_sha256      varchar(64) NOT NULL,
    decision_id                uuid NOT NULL REFERENCES annotation_review_decision(id),
    outcome                    varchar(16) NOT NULL,
    reviewed_result_id         uuid REFERENCES annotation_result(id),
    selected_result_id         uuid REFERENCES annotation_result(id),
    selected_result_sha256     varchar(64),
    output_row_index           integer,
    PRIMARY KEY(binding_id, task_id),
    CONSTRAINT uq_gold_binding_output_row UNIQUE(binding_id, output_row_index),
    CONSTRAINT ck_gold_member_outcome CHECK (outcome IN ('ACCEPT','REJECT','CORRECT')),
    CONSTRAINT ck_gold_member_source_hash CHECK (source_content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_gold_member_selected_hash CHECK (
        selected_result_sha256 IS NULL OR selected_result_sha256 ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT ck_gold_member_selection CHECK (
        (
            outcome IN ('ACCEPT','CORRECT')
            AND selected_result_id IS NOT NULL
            AND selected_result_sha256 IS NOT NULL
            AND output_row_index IS NOT NULL
            AND output_row_index >= 0
        )
        OR
        (
            outcome='REJECT'
            AND selected_result_id IS NULL
            AND selected_result_sha256 IS NULL
            AND output_row_index IS NULL
        )
    )
);

CREATE INDEX idx_gold_member_decision
    ON gold_production_member(decision_id);
CREATE INDEX idx_gold_member_selected_result
    ON gold_production_member(selected_result_id)
    WHERE selected_result_id IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_gold_binding_insert()
RETURNS trigger AS $$
BEGIN
    IF NEW.status <> 'BUILDING' OR NEW.finalized_at IS NOT NULL THEN
        RAISE EXCEPTION 'new gold production binding must start BUILDING';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_binding_insert
BEFORE INSERT ON gold_production_binding
FOR EACH ROW EXECUTE FUNCTION guard_gold_binding_insert();

CREATE OR REPLACE FUNCTION guard_gold_member_mutation()
RETURNS trigger AS $$
DECLARE
    binding_status varchar(16);
    binding_workspace uuid;
    binding_snapshot uuid;
    snapshot_task record;
    snapshot_decision record;
    snapshot_output uuid;
    result_task uuid;
    result_hash varchar(64);
BEGIN
    SELECT status, workspace_id, annotation_snapshot_id
      INTO binding_status, binding_workspace, binding_snapshot
      FROM gold_production_binding
     WHERE id=COALESCE(NEW.binding_id, OLD.binding_id)
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'gold production binding parent is not visible';
    END IF;
    IF binding_status <> 'BUILDING' THEN
        RAISE EXCEPTION 'gold production binding membership is finalized';
    END IF;
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'gold production binding membership is append-only';
    END IF;

    SELECT st.task_id, st.source_item_ref, st.source_content_sha256
      INTO snapshot_task
      FROM annotation_snapshot_task st
     WHERE st.snapshot_id=binding_snapshot AND st.task_id=NEW.task_id;

    IF snapshot_task.task_id IS NULL
       OR snapshot_task.source_item_ref IS DISTINCT FROM NEW.source_item_ref
       OR snapshot_task.source_content_sha256 IS DISTINCT FROM NEW.source_content_sha256 THEN
        RAISE EXCEPTION 'gold production member does not match frozen snapshot task';
    END IF;

    SELECT sd.decision_id, sd.outcome, sd.reviewed_result_id, sd.selected_result_id
      INTO snapshot_decision
      FROM annotation_snapshot_decision sd
     WHERE sd.snapshot_id=binding_snapshot AND sd.task_id=NEW.task_id;

    IF snapshot_decision.decision_id IS NULL
       OR snapshot_decision.decision_id IS DISTINCT FROM NEW.decision_id
       OR snapshot_decision.outcome IS DISTINCT FROM NEW.outcome
       OR snapshot_decision.reviewed_result_id IS DISTINCT FROM NEW.reviewed_result_id
       OR snapshot_decision.selected_result_id IS DISTINCT FROM NEW.selected_result_id THEN
        RAISE EXCEPTION 'gold production member does not match frozen snapshot decision';
    END IF;

    SELECT so.selected_result_id
      INTO snapshot_output
      FROM annotation_snapshot_output so
     WHERE so.snapshot_id=binding_snapshot AND so.task_id=NEW.task_id;

    IF NEW.outcome IN ('ACCEPT','CORRECT') THEN
        IF snapshot_output IS DISTINCT FROM NEW.selected_result_id THEN
            RAISE EXCEPTION 'gold production selected result does not match frozen snapshot output';
        END IF;
        SELECT r.task_id, r.canonical_payload_sha256
          INTO result_task, result_hash
          FROM annotation_result r
         WHERE r.id=NEW.selected_result_id;
        IF result_task IS DISTINCT FROM NEW.task_id
           OR result_hash IS DISTINCT FROM NEW.selected_result_sha256 THEN
            RAISE EXCEPTION 'gold production selected result identity/hash mismatch';
        END IF;
    ELSE
        IF snapshot_output IS NOT NULL THEN
            RAISE EXCEPTION 'gold production rejected task cannot have frozen output';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_member_insert
BEFORE INSERT ON gold_production_member
FOR EACH ROW EXECUTE FUNCTION guard_gold_member_mutation();

CREATE TRIGGER trg_gold_member_update
BEFORE UPDATE ON gold_production_member
FOR EACH ROW EXECUTE FUNCTION guard_gold_member_mutation();

CREATE TRIGGER trg_gold_member_delete
BEFORE DELETE ON gold_production_member
FOR EACH ROW EXECUTE FUNCTION guard_gold_member_mutation();

CREATE OR REPLACE FUNCTION guard_gold_binding_update()
RETURNS trigger AS $$
DECLARE
    execution_row record;
    input_bound boolean;
    input_row record;
    certification_row record;
    campaign_row record;
    snapshot_row record;
    output_row record;
    snapshot_task_count integer;
    member_count integer;
    selected_count integer;
BEGIN
    IF OLD.status <> 'BUILDING' OR NEW.status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'gold production binding is immutable after finalization';
    END IF;

    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.execution_id IS DISTINCT FROM OLD.execution_id
       OR NEW.workflow_version_id IS DISTINCT FROM OLD.workflow_version_id
       OR NEW.input_dataset_version_id IS DISTINCT FROM OLD.input_dataset_version_id
       OR NEW.input_certification_id IS DISTINCT FROM OLD.input_certification_id
       OR NEW.annotation_campaign_id IS DISTINCT FROM OLD.annotation_campaign_id
       OR NEW.annotation_snapshot_id IS DISTINCT FROM OLD.annotation_snapshot_id
       OR NEW.annotation_contribution_resource_id IS DISTINCT FROM OLD.annotation_contribution_resource_id
       OR NEW.output_dataset_version_id IS DISTINCT FROM OLD.output_dataset_version_id
       OR NEW.input_checksum_sha256 IS DISTINCT FROM OLD.input_checksum_sha256
       OR NEW.snapshot_root_hash IS DISTINCT FROM OLD.snapshot_root_hash
       OR NEW.schema_content_sha256 IS DISTINCT FROM OLD.schema_content_sha256
       OR NEW.taxonomy_content_sha256 IS DISTINCT FROM OLD.taxonomy_content_sha256
       OR NEW.rubric_content_sha256 IS DISTINCT FROM OLD.rubric_content_sha256
       OR NEW.renderer_content_sha256 IS DISTINCT FROM OLD.renderer_content_sha256
       OR NEW.review_policy_content_sha256 IS DISTINCT FROM OLD.review_policy_content_sha256
       OR NEW.output_checksum_sha256 IS DISTINCT FROM OLD.output_checksum_sha256
       OR NEW.output_row_count IS DISTINCT FROM OLD.output_row_count
       OR NEW.manifest IS DISTINCT FROM OLD.manifest
       OR NEW.manifest_hash_payload IS DISTINCT FROM OLD.manifest_hash_payload
       OR NEW.root_hash IS DISTINCT FROM OLD.root_hash
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.created_by IS DISTINCT FROM OLD.created_by
       OR NEW.finalized_at IS NULL THEN
        RAISE EXCEPTION 'gold production binding frozen semantics are immutable';
    END IF;

    SELECT e.workspace_id, e.workflow_version_id, e.output_dataset_id
      INTO execution_row
      FROM execution e
     WHERE e.id=NEW.execution_id;

    IF execution_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR execution_row.workflow_version_id IS DISTINCT FROM NEW.workflow_version_id THEN
        RAISE EXCEPTION 'gold production binding execution identity mismatch';
    END IF;

    SELECT EXISTS(
        SELECT 1
          FROM execution_input ei
         WHERE ei.execution_id=NEW.execution_id
           AND ei.input_name='gold_input'
           AND ei.dataset_version_id=NEW.input_dataset_version_id
    ) INTO input_bound;
    IF NOT input_bound THEN
        RAISE EXCEPTION 'gold production binding requires exact gold_input execution binding';
    END IF;

    SELECT v.status, v.checksum_value
      INTO input_row
      FROM dataset_version v
     WHERE v.id=NEW.input_dataset_version_id;
    IF input_row.status NOT IN ('READY','SUPERSEDED')
       OR lower(input_row.checksum_value) IS DISTINCT FROM lower(NEW.input_checksum_sha256) THEN
        RAISE EXCEPTION 'gold production input DatasetVersion identity mismatch';
    END IF;

    SELECT c.workspace_id, c.dataset_version_id, c.decision
      INTO certification_row
      FROM dataset_certification c
     WHERE c.id=NEW.input_certification_id;
    IF certification_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR certification_row.dataset_version_id IS DISTINCT FROM NEW.input_dataset_version_id
       OR certification_row.decision <> 'CERTIFIED' THEN
        RAISE EXCEPTION 'gold production input certification mismatch';
    END IF;

    SELECT c.workspace_id, c.input_dataset_version_id, c.input_certification_id,
           c.annotation_contribution_resource_id,
           c.schema_content_sha256, c.taxonomy_content_sha256, c.rubric_content_sha256,
           c.renderer_content_sha256, c.review_policy_content_sha256
      INTO campaign_row
      FROM annotation_campaign c
     WHERE c.id=NEW.annotation_campaign_id;
    IF campaign_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR campaign_row.input_dataset_version_id IS DISTINCT FROM NEW.input_dataset_version_id
       OR campaign_row.input_certification_id IS DISTINCT FROM NEW.input_certification_id
       OR campaign_row.annotation_contribution_resource_id IS DISTINCT FROM NEW.annotation_contribution_resource_id
       OR campaign_row.schema_content_sha256 IS DISTINCT FROM NEW.schema_content_sha256
       OR campaign_row.taxonomy_content_sha256 IS DISTINCT FROM NEW.taxonomy_content_sha256
       OR campaign_row.rubric_content_sha256 IS DISTINCT FROM NEW.rubric_content_sha256
       OR campaign_row.renderer_content_sha256 IS DISTINCT FROM NEW.renderer_content_sha256
       OR campaign_row.review_policy_content_sha256 IS DISTINCT FROM NEW.review_policy_content_sha256 THEN
        RAISE EXCEPTION 'gold production campaign frozen contract mismatch';
    END IF;

    SELECT s.workspace_id, s.campaign_id, s.status, s.root_hash
      INTO snapshot_row
      FROM annotation_snapshot s
     WHERE s.id=NEW.annotation_snapshot_id;
    IF snapshot_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR snapshot_row.campaign_id IS DISTINCT FROM NEW.annotation_campaign_id
       OR snapshot_row.status <> 'FINALIZED'
       OR snapshot_row.root_hash IS DISTINCT FROM NEW.snapshot_root_hash THEN
        RAISE EXCEPTION 'gold production snapshot identity mismatch';
    END IF;

    SELECT d.workspace_id, v.dataset_id, v.status, v.generated_by_execution_id,
           v.checksum_value, COALESCE(v.row_count,0) AS row_count
      INTO output_row
      FROM dataset_version v
      JOIN dataset d ON d.id=v.dataset_id
     WHERE v.id=NEW.output_dataset_version_id;
    IF output_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR output_row.dataset_id IS DISTINCT FROM execution_row.output_dataset_id
       OR output_row.status <> 'READY'
       OR output_row.generated_by_execution_id IS DISTINCT FROM NEW.execution_id
       OR lower(output_row.checksum_value) IS DISTINCT FROM lower(NEW.output_checksum_sha256)
       OR output_row.row_count IS DISTINCT FROM NEW.output_row_count THEN
        RAISE EXCEPTION 'gold production output DatasetVersion identity mismatch';
    END IF;

    SELECT count(*) INTO snapshot_task_count
      FROM annotation_snapshot_task
     WHERE snapshot_id=NEW.annotation_snapshot_id;
    SELECT count(*), count(*) FILTER (WHERE selected_result_id IS NOT NULL)
      INTO member_count, selected_count
      FROM gold_production_member
     WHERE binding_id=NEW.id;

    IF member_count <> snapshot_task_count OR selected_count <> NEW.output_row_count THEN
        RAISE EXCEPTION 'gold production binding membership/output count mismatch';
    END IF;

    IF NEW.manifest->>'formatVersion' IS DISTINCT FROM 'gold-production-binding-v1'
       OR NEW.manifest->>'workspaceId' IS DISTINCT FROM NEW.workspace_id::text
       OR NEW.manifest->>'executionId' IS DISTINCT FROM NEW.execution_id::text
       OR NEW.manifest->>'workflowVersionId' IS DISTINCT FROM NEW.workflow_version_id::text
       OR NEW.manifest->>'inputDatasetVersionId' IS DISTINCT FROM NEW.input_dataset_version_id::text
       OR NEW.manifest->>'inputCertificationId' IS DISTINCT FROM NEW.input_certification_id::text
       OR NEW.manifest->>'annotationCampaignId' IS DISTINCT FROM NEW.annotation_campaign_id::text
       OR NEW.manifest->>'annotationSnapshotId' IS DISTINCT FROM NEW.annotation_snapshot_id::text
       OR NEW.manifest->>'annotationContributionResourceId' IS DISTINCT FROM NEW.annotation_contribution_resource_id::text
       OR NEW.manifest->>'outputDatasetVersionId' IS DISTINCT FROM NEW.output_dataset_version_id::text
       OR NEW.manifest->>'inputChecksumSha256' IS DISTINCT FROM NEW.input_checksum_sha256
       OR NEW.manifest->>'snapshotRootHash' IS DISTINCT FROM NEW.snapshot_root_hash
       OR NEW.manifest->>'outputChecksumSha256' IS DISTINCT FROM NEW.output_checksum_sha256
       OR (NEW.manifest->>'outputRowCount')::bigint IS DISTINCT FROM NEW.output_row_count THEN
        RAISE EXCEPTION 'gold production binding manifest header mismatch';
    END IF;

    IF NEW.manifest->'members' IS DISTINCT FROM (
        SELECT COALESCE(
            jsonb_agg(
                jsonb_strip_nulls(jsonb_build_object(
                    'taskId', m.task_id::text,
                    'sourceItemRef', m.source_item_ref,
                    'sourceContentSha256', m.source_content_sha256,
                    'decisionId', m.decision_id::text,
                    'outcome', m.outcome,
                    'reviewedResultId', m.reviewed_result_id::text,
                    'selectedResultId', m.selected_result_id::text,
                    'selectedResultSha256', m.selected_result_sha256,
                    'outputRowIndex', m.output_row_index
                )) ORDER BY m.task_id::text
            ), '[]'::jsonb
        )
        FROM gold_production_member m
        WHERE m.binding_id=NEW.id
    ) THEN
        RAISE EXCEPTION 'gold production binding manifest membership mismatch';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_binding_update
BEFORE UPDATE ON gold_production_binding
FOR EACH ROW EXECUTE FUNCTION guard_gold_binding_update();

CREATE OR REPLACE FUNCTION guard_gold_binding_delete()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'gold production binding is immutable history';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_binding_delete
BEFORE DELETE ON gold_production_binding
FOR EACH ROW EXECUTE FUNCTION guard_gold_binding_delete();

CREATE OR REPLACE FUNCTION require_gold_binding_finalized_on_commit()
RETURNS trigger AS $$
DECLARE
    current_status varchar(16);
BEGIN
    SELECT status INTO current_status FROM gold_production_binding WHERE id=NEW.id;
    IF current_status IS DISTINCT FROM 'FINALIZED' THEN
        RAISE EXCEPTION 'gold production binding % must be FINALIZED before commit', NEW.id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_gold_binding_require_finalized
AFTER INSERT ON gold_production_binding
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_gold_binding_finalized_on_commit();

    )
);

CREATE INDEX idx_gold_build_request_snapshot
    ON gold_build_request(annotation_snapshot_id, execution_id);

CREATE OR REPLACE FUNCTION guard_gold_build_request_insert()
RETURNS trigger AS $
DECLARE
    execution_row record;
    input_bound boolean;
    certification_row record;
    campaign_row record;
    snapshot_row record;
BEGIN
    SELECT e.status, e.workflow_version_id, wv.definition
      INTO execution_row
      FROM execution e
      JOIN workflow_version wv ON wv.id=e.workflow_version_id
     WHERE e.id=NEW.execution_id AND e.workspace_id=NEW.workspace_id;

    IF execution_row.status IS NULL OR execution_row.status <> 'QUEUED' THEN
        RAISE EXCEPTION 'gold build request requires a QUEUED execution in the same workspace';
    END IF;
    IF execution_row.definition#>>'{spec,processor}' IS DISTINCT FROM 'GOLD_DATASET_BUILDER_V1' THEN
        RAISE EXCEPTION 'gold build request requires GOLD_DATASET_BUILDER_V1 workflow';
    END IF;

    SELECT EXISTS(
        SELECT 1
          FROM execution_input ei
         WHERE ei.execution_id=NEW.execution_id
           AND ei.input_name='gold_input'
           AND ei.dataset_version_id=NEW.input_dataset_version_id
    ) INTO input_bound;
    IF NOT input_bound THEN
        RAISE EXCEPTION 'gold build request requires exact gold_input execution binding';
    END IF;

    SELECT c.workspace_id, c.dataset_version_id, c.decision
      INTO certification_row
      FROM dataset_certification c
     WHERE c.id=NEW.input_certification_id;
    IF certification_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR certification_row.dataset_version_id IS DISTINCT FROM NEW.input_dataset_version_id
       OR certification_row.decision <> 'CERTIFIED' THEN
        RAISE EXCEPTION 'gold build request input certification mismatch';
    END IF;

    SELECT c.workspace_id, c.input_dataset_version_id, c.input_certification_id,
           c.annotation_contribution_resource_id
      INTO campaign_row
      FROM annotation_campaign c
     WHERE c.id=NEW.annotation_campaign_id;
    IF campaign_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR campaign_row.input_dataset_version_id IS DISTINCT FROM NEW.input_dataset_version_id
       OR campaign_row.input_certification_id IS DISTINCT FROM NEW.input_certification_id
       OR campaign_row.annotation_contribution_resource_id IS DISTINCT FROM NEW.annotation_contribution_resource_id THEN
        RAISE EXCEPTION 'gold build request campaign mismatch';
    END IF;

    SELECT s.workspace_id, s.campaign_id, s.status, s.root_hash
      INTO snapshot_row
      FROM annotation_snapshot s
     WHERE s.id=NEW.annotation_snapshot_id;
    IF snapshot_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR snapshot_row.campaign_id IS DISTINCT FROM NEW.annotation_campaign_id
       OR snapshot_row.status <> 'FINALIZED'
       OR snapshot_row.root_hash IS DISTINCT FROM NEW.snapshot_root_hash THEN
        RAISE EXCEPTION 'gold build request requires exact FINALIZED snapshot';
    END IF;
    RETURN NEW;
END;
$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_build_request_insert
BEFORE INSERT ON gold_build_request
FOR EACH ROW EXECUTE FUNCTION guard_gold_build_request_insert();

CREATE OR REPLACE FUNCTION prevent_gold_build_request_mutation()
RETURNS trigger AS $
BEGIN
    RAISE EXCEPTION 'gold build request is immutable execution input';
END;
$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_build_request_immutable
BEFORE UPDATE OR DELETE ON gold_build_request
FOR EACH ROW EXECUTE FUNCTION prevent_gold_build_request_mutation();

CREATE TABLE gold_production_binding (
    id                                  uuid PRIMARY KEY,
    workspace_id                        uuid NOT NULL,
    execution_id                        uuid NOT NULL UNIQUE REFERENCES execution(id),
    workflow_version_id                 uuid NOT NULL REFERENCES workflow_version(id),
    input_dataset_version_id            uuid NOT NULL REFERENCES dataset_version(id),
    input_certification_id              uuid NOT NULL REFERENCES dataset_certification(id),
    annotation_campaign_id              uuid NOT NULL REFERENCES annotation_campaign(id),
    annotation_snapshot_id              uuid NOT NULL REFERENCES annotation_snapshot(id),
    annotation_contribution_resource_id uuid NOT NULL REFERENCES data_resource(id),
    output_dataset_version_id           uuid NOT NULL UNIQUE REFERENCES dataset_version(id),

    input_checksum_sha256               varchar(64) NOT NULL,
    snapshot_root_hash                  varchar(64) NOT NULL,
    schema_content_sha256               varchar(64) NOT NULL,
    taxonomy_content_sha256             varchar(64) NOT NULL,
    rubric_content_sha256               varchar(64) NOT NULL,
    renderer_content_sha256             varchar(64) NOT NULL,
    review_policy_content_sha256        varchar(64) NOT NULL,
    output_checksum_sha256              varchar(64) NOT NULL,
    output_row_count                    bigint NOT NULL,

    manifest                            jsonb NOT NULL,
    manifest_hash_payload               bytea NOT NULL,
    root_hash                           varchar(64) NOT NULL,
    status                              varchar(16) NOT NULL DEFAULT 'BUILDING',
    created_at                          timestamptz NOT NULL DEFAULT now(),
    created_by                          uuid,
    finalized_at                        timestamptz,

    CONSTRAINT ck_gold_binding_status CHECK (status IN ('BUILDING','FINALIZED')),
    CONSTRAINT ck_gold_binding_output_count CHECK (output_row_count >= 0),
    CONSTRAINT ck_gold_binding_hashes CHECK (
        input_checksum_sha256 ~ '^[0-9a-f]{64}$'
        AND snapshot_root_hash ~ '^[0-9a-f]{64}$'
        AND schema_content_sha256 ~ '^[0-9a-f]{64}$'
        AND taxonomy_content_sha256 ~ '^[0-9a-f]{64}$'
        AND rubric_content_sha256 ~ '^[0-9a-f]{64}$'
        AND renderer_content_sha256 ~ '^[0-9a-f]{64}$'
        AND review_policy_content_sha256 ~ '^[0-9a-f]{64}$'
        AND output_checksum_sha256 ~ '^[0-9a-f]{64}$'
        AND root_hash ~ '^[0-9a-f]{64}$'
        AND encode(digest(manifest_hash_payload, 'sha256'), 'hex') = root_hash
        AND convert_from(manifest_hash_payload, 'UTF8')::jsonb = manifest
    )
);

CREATE INDEX idx_gold_binding_workspace
    ON gold_production_binding(workspace_id, created_at DESC, id);
CREATE INDEX idx_gold_binding_input
    ON gold_production_binding(input_dataset_version_id, created_at DESC, id);
CREATE INDEX idx_gold_binding_snapshot
    ON gold_production_binding(annotation_snapshot_id, created_at DESC, id);

CREATE TABLE gold_production_member (
    binding_id                 uuid NOT NULL REFERENCES gold_production_binding(id),
    task_id                    uuid NOT NULL REFERENCES annotation_task(id),
    source_item_ref            varchar(1024) NOT NULL,
    source_content_sha256      varchar(64) NOT NULL,
    decision_id                uuid NOT NULL REFERENCES annotation_review_decision(id),
    outcome                    varchar(16) NOT NULL,
    reviewed_result_id         uuid REFERENCES annotation_result(id),
    selected_result_id         uuid REFERENCES annotation_result(id),
    selected_result_sha256     varchar(64),
    output_row_index           integer,
    PRIMARY KEY(binding_id, task_id),
    CONSTRAINT uq_gold_binding_output_row UNIQUE(binding_id, output_row_index),
    CONSTRAINT ck_gold_member_outcome CHECK (outcome IN ('ACCEPT','REJECT','CORRECT')),
    CONSTRAINT ck_gold_member_source_hash CHECK (source_content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_gold_member_selected_hash CHECK (
        selected_result_sha256 IS NULL OR selected_result_sha256 ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT ck_gold_member_selection CHECK (
        (
            outcome IN ('ACCEPT','CORRECT')
            AND selected_result_id IS NOT NULL
            AND selected_result_sha256 IS NOT NULL
            AND output_row_index IS NOT NULL
            AND output_row_index >= 0
        )
        OR
        (
            outcome='REJECT'
            AND selected_result_id IS NULL
            AND selected_result_sha256 IS NULL
            AND output_row_index IS NULL
        )
    )
);

CREATE INDEX idx_gold_member_decision
    ON gold_production_member(decision_id);
CREATE INDEX idx_gold_member_selected_result
    ON gold_production_member(selected_result_id)
    WHERE selected_result_id IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_gold_binding_insert()
RETURNS trigger AS $$
BEGIN
    IF NEW.status <> 'BUILDING' OR NEW.finalized_at IS NOT NULL THEN
        RAISE EXCEPTION 'new gold production binding must start BUILDING';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_binding_insert
BEFORE INSERT ON gold_production_binding
FOR EACH ROW EXECUTE FUNCTION guard_gold_binding_insert();

CREATE OR REPLACE FUNCTION guard_gold_member_mutation()
RETURNS trigger AS $$
DECLARE
    binding_status varchar(16);
    binding_workspace uuid;
    binding_snapshot uuid;
    snapshot_task record;
    snapshot_decision record;
    snapshot_output uuid;
    result_task uuid;
    result_hash varchar(64);
BEGIN
    SELECT status, workspace_id, annotation_snapshot_id
      INTO binding_status, binding_workspace, binding_snapshot
      FROM gold_production_binding
     WHERE id=COALESCE(NEW.binding_id, OLD.binding_id)
     FOR UPDATE;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'gold production binding parent is not visible';
    END IF;
    IF binding_status <> 'BUILDING' THEN
        RAISE EXCEPTION 'gold production binding membership is finalized';
    END IF;
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'gold production binding membership is append-only';
    END IF;

    SELECT st.task_id, st.source_item_ref, st.source_content_sha256
      INTO snapshot_task
      FROM annotation_snapshot_task st
     WHERE st.snapshot_id=binding_snapshot AND st.task_id=NEW.task_id;

    IF snapshot_task.task_id IS NULL
       OR snapshot_task.source_item_ref IS DISTINCT FROM NEW.source_item_ref
       OR snapshot_task.source_content_sha256 IS DISTINCT FROM NEW.source_content_sha256 THEN
        RAISE EXCEPTION 'gold production member does not match frozen snapshot task';
    END IF;

    SELECT sd.decision_id, sd.outcome, sd.reviewed_result_id, sd.selected_result_id
      INTO snapshot_decision
      FROM annotation_snapshot_decision sd
     WHERE sd.snapshot_id=binding_snapshot AND sd.task_id=NEW.task_id;

    IF snapshot_decision.decision_id IS NULL
       OR snapshot_decision.decision_id IS DISTINCT FROM NEW.decision_id
       OR snapshot_decision.outcome IS DISTINCT FROM NEW.outcome
       OR snapshot_decision.reviewed_result_id IS DISTINCT FROM NEW.reviewed_result_id
       OR snapshot_decision.selected_result_id IS DISTINCT FROM NEW.selected_result_id THEN
        RAISE EXCEPTION 'gold production member does not match frozen snapshot decision';
    END IF;

    SELECT so.selected_result_id
      INTO snapshot_output
      FROM annotation_snapshot_output so
     WHERE so.snapshot_id=binding_snapshot AND so.task_id=NEW.task_id;

    IF NEW.outcome IN ('ACCEPT','CORRECT') THEN
        IF snapshot_output IS DISTINCT FROM NEW.selected_result_id THEN
            RAISE EXCEPTION 'gold production selected result does not match frozen snapshot output';
        END IF;
        SELECT r.task_id, r.canonical_payload_sha256
          INTO result_task, result_hash
          FROM annotation_result r
         WHERE r.id=NEW.selected_result_id;
        IF result_task IS DISTINCT FROM NEW.task_id
           OR result_hash IS DISTINCT FROM NEW.selected_result_sha256 THEN
            RAISE EXCEPTION 'gold production selected result identity/hash mismatch';
        END IF;
    ELSE
        IF snapshot_output IS NOT NULL THEN
            RAISE EXCEPTION 'gold production rejected task cannot have frozen output';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_member_insert
BEFORE INSERT ON gold_production_member
FOR EACH ROW EXECUTE FUNCTION guard_gold_member_mutation();

CREATE TRIGGER trg_gold_member_update
BEFORE UPDATE ON gold_production_member
FOR EACH ROW EXECUTE FUNCTION guard_gold_member_mutation();

CREATE TRIGGER trg_gold_member_delete
BEFORE DELETE ON gold_production_member
FOR EACH ROW EXECUTE FUNCTION guard_gold_member_mutation();

CREATE OR REPLACE FUNCTION guard_gold_binding_update()
RETURNS trigger AS $$
DECLARE
    execution_row record;
    input_bound boolean;
    input_row record;
    certification_row record;
    campaign_row record;
    snapshot_row record;
    output_row record;
    snapshot_task_count integer;
    member_count integer;
    selected_count integer;
BEGIN
    IF OLD.status <> 'BUILDING' OR NEW.status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'gold production binding is immutable after finalization';
    END IF;

    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.execution_id IS DISTINCT FROM OLD.execution_id
       OR NEW.workflow_version_id IS DISTINCT FROM OLD.workflow_version_id
       OR NEW.input_dataset_version_id IS DISTINCT FROM OLD.input_dataset_version_id
       OR NEW.input_certification_id IS DISTINCT FROM OLD.input_certification_id
       OR NEW.annotation_campaign_id IS DISTINCT FROM OLD.annotation_campaign_id
       OR NEW.annotation_snapshot_id IS DISTINCT FROM OLD.annotation_snapshot_id
       OR NEW.annotation_contribution_resource_id IS DISTINCT FROM OLD.annotation_contribution_resource_id
       OR NEW.output_dataset_version_id IS DISTINCT FROM OLD.output_dataset_version_id
       OR NEW.input_checksum_sha256 IS DISTINCT FROM OLD.input_checksum_sha256
       OR NEW.snapshot_root_hash IS DISTINCT FROM OLD.snapshot_root_hash
       OR NEW.schema_content_sha256 IS DISTINCT FROM OLD.schema_content_sha256
       OR NEW.taxonomy_content_sha256 IS DISTINCT FROM OLD.taxonomy_content_sha256
       OR NEW.rubric_content_sha256 IS DISTINCT FROM OLD.rubric_content_sha256
       OR NEW.renderer_content_sha256 IS DISTINCT FROM OLD.renderer_content_sha256
       OR NEW.review_policy_content_sha256 IS DISTINCT FROM OLD.review_policy_content_sha256
       OR NEW.output_checksum_sha256 IS DISTINCT FROM OLD.output_checksum_sha256
       OR NEW.output_row_count IS DISTINCT FROM OLD.output_row_count
       OR NEW.manifest IS DISTINCT FROM OLD.manifest
       OR NEW.manifest_hash_payload IS DISTINCT FROM OLD.manifest_hash_payload
       OR NEW.root_hash IS DISTINCT FROM OLD.root_hash
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.created_by IS DISTINCT FROM OLD.created_by
       OR NEW.finalized_at IS NULL THEN
        RAISE EXCEPTION 'gold production binding frozen semantics are immutable';
    END IF;

    SELECT e.workspace_id, e.workflow_version_id, e.output_dataset_id
      INTO execution_row
      FROM execution e
     WHERE e.id=NEW.execution_id;

    IF execution_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR execution_row.workflow_version_id IS DISTINCT FROM NEW.workflow_version_id THEN
        RAISE EXCEPTION 'gold production binding execution identity mismatch';
    END IF;

    SELECT EXISTS(
        SELECT 1
          FROM execution_input ei
         WHERE ei.execution_id=NEW.execution_id
           AND ei.input_name='gold_input'
           AND ei.dataset_version_id=NEW.input_dataset_version_id
    ) INTO input_bound;
    IF NOT input_bound THEN
        RAISE EXCEPTION 'gold production binding requires exact gold_input execution binding';
    END IF;

    SELECT v.status, v.checksum_value
      INTO input_row
      FROM dataset_version v
     WHERE v.id=NEW.input_dataset_version_id;
    IF input_row.status NOT IN ('READY','SUPERSEDED')
       OR lower(input_row.checksum_value) IS DISTINCT FROM lower(NEW.input_checksum_sha256) THEN
        RAISE EXCEPTION 'gold production input DatasetVersion identity mismatch';
    END IF;

    SELECT c.workspace_id, c.dataset_version_id, c.decision
      INTO certification_row
      FROM dataset_certification c
     WHERE c.id=NEW.input_certification_id;
    IF certification_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR certification_row.dataset_version_id IS DISTINCT FROM NEW.input_dataset_version_id
       OR certification_row.decision <> 'CERTIFIED' THEN
        RAISE EXCEPTION 'gold production input certification mismatch';
    END IF;

    SELECT c.workspace_id, c.input_dataset_version_id, c.input_certification_id,
           c.annotation_contribution_resource_id,
           c.schema_content_sha256, c.taxonomy_content_sha256, c.rubric_content_sha256,
           c.renderer_content_sha256, c.review_policy_content_sha256
      INTO campaign_row
      FROM annotation_campaign c
     WHERE c.id=NEW.annotation_campaign_id;
    IF campaign_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR campaign_row.input_dataset_version_id IS DISTINCT FROM NEW.input_dataset_version_id
       OR campaign_row.input_certification_id IS DISTINCT FROM NEW.input_certification_id
       OR campaign_row.annotation_contribution_resource_id IS DISTINCT FROM NEW.annotation_contribution_resource_id
       OR campaign_row.schema_content_sha256 IS DISTINCT FROM NEW.schema_content_sha256
       OR campaign_row.taxonomy_content_sha256 IS DISTINCT FROM NEW.taxonomy_content_sha256
       OR campaign_row.rubric_content_sha256 IS DISTINCT FROM NEW.rubric_content_sha256
       OR campaign_row.renderer_content_sha256 IS DISTINCT FROM NEW.renderer_content_sha256
       OR campaign_row.review_policy_content_sha256 IS DISTINCT FROM NEW.review_policy_content_sha256 THEN
        RAISE EXCEPTION 'gold production campaign frozen contract mismatch';
    END IF;

    SELECT s.workspace_id, s.campaign_id, s.status, s.root_hash
      INTO snapshot_row
      FROM annotation_snapshot s
     WHERE s.id=NEW.annotation_snapshot_id;
    IF snapshot_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR snapshot_row.campaign_id IS DISTINCT FROM NEW.annotation_campaign_id
       OR snapshot_row.status <> 'FINALIZED'
       OR snapshot_row.root_hash IS DISTINCT FROM NEW.snapshot_root_hash THEN
        RAISE EXCEPTION 'gold production snapshot identity mismatch';
    END IF;

    SELECT d.workspace_id, v.dataset_id, v.status, v.generated_by_execution_id,
           v.checksum_value, COALESCE(v.row_count,0) AS row_count
      INTO output_row
      FROM dataset_version v
      JOIN dataset d ON d.id=v.dataset_id
     WHERE v.id=NEW.output_dataset_version_id;
    IF output_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR output_row.dataset_id IS DISTINCT FROM execution_row.output_dataset_id
       OR output_row.status <> 'READY'
       OR output_row.generated_by_execution_id IS DISTINCT FROM NEW.execution_id
       OR lower(output_row.checksum_value) IS DISTINCT FROM lower(NEW.output_checksum_sha256)
       OR output_row.row_count IS DISTINCT FROM NEW.output_row_count THEN
        RAISE EXCEPTION 'gold production output DatasetVersion identity mismatch';
    END IF;

    SELECT count(*) INTO snapshot_task_count
      FROM annotation_snapshot_task
     WHERE snapshot_id=NEW.annotation_snapshot_id;
    SELECT count(*), count(*) FILTER (WHERE selected_result_id IS NOT NULL)
      INTO member_count, selected_count
      FROM gold_production_member
     WHERE binding_id=NEW.id;

    IF member_count <> snapshot_task_count OR selected_count <> NEW.output_row_count THEN
        RAISE EXCEPTION 'gold production binding membership/output count mismatch';
    END IF;

    IF NEW.manifest->>'formatVersion' IS DISTINCT FROM 'gold-production-binding-v1'
       OR NEW.manifest->>'workspaceId' IS DISTINCT FROM NEW.workspace_id::text
       OR NEW.manifest->>'executionId' IS DISTINCT FROM NEW.execution_id::text
       OR NEW.manifest->>'workflowVersionId' IS DISTINCT FROM NEW.workflow_version_id::text
       OR NEW.manifest->>'inputDatasetVersionId' IS DISTINCT FROM NEW.input_dataset_version_id::text
       OR NEW.manifest->>'inputCertificationId' IS DISTINCT FROM NEW.input_certification_id::text
       OR NEW.manifest->>'annotationCampaignId' IS DISTINCT FROM NEW.annotation_campaign_id::text
       OR NEW.manifest->>'annotationSnapshotId' IS DISTINCT FROM NEW.annotation_snapshot_id::text
       OR NEW.manifest->>'annotationContributionResourceId' IS DISTINCT FROM NEW.annotation_contribution_resource_id::text
       OR NEW.manifest->>'outputDatasetVersionId' IS DISTINCT FROM NEW.output_dataset_version_id::text
       OR NEW.manifest->>'inputChecksumSha256' IS DISTINCT FROM NEW.input_checksum_sha256
       OR NEW.manifest->>'snapshotRootHash' IS DISTINCT FROM NEW.snapshot_root_hash
       OR NEW.manifest->>'outputChecksumSha256' IS DISTINCT FROM NEW.output_checksum_sha256
       OR (NEW.manifest->>'outputRowCount')::bigint IS DISTINCT FROM NEW.output_row_count THEN
        RAISE EXCEPTION 'gold production binding manifest header mismatch';
    END IF;

    IF NEW.manifest->'members' IS DISTINCT FROM (
        SELECT COALESCE(
            jsonb_agg(
                jsonb_strip_nulls(jsonb_build_object(
                    'taskId', m.task_id::text,
                    'sourceItemRef', m.source_item_ref,
                    'sourceContentSha256', m.source_content_sha256,
                    'decisionId', m.decision_id::text,
                    'outcome', m.outcome,
                    'reviewedResultId', m.reviewed_result_id::text,
                    'selectedResultId', m.selected_result_id::text,
                    'selectedResultSha256', m.selected_result_sha256,
                    'outputRowIndex', m.output_row_index
                )) ORDER BY m.task_id::text
            ), '[]'::jsonb
        )
        FROM gold_production_member m
        WHERE m.binding_id=NEW.id
    ) THEN
        RAISE EXCEPTION 'gold production binding manifest membership mismatch';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_binding_update
BEFORE UPDATE ON gold_production_binding
FOR EACH ROW EXECUTE FUNCTION guard_gold_binding_update();

CREATE OR REPLACE FUNCTION guard_gold_binding_delete()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'gold production binding is immutable history';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_gold_binding_delete
BEFORE DELETE ON gold_production_binding
FOR EACH ROW EXECUTE FUNCTION guard_gold_binding_delete();

CREATE OR REPLACE FUNCTION require_gold_binding_finalized_on_commit()
RETURNS trigger AS $$
DECLARE
    current_status varchar(16);
BEGIN
    SELECT status INTO current_status FROM gold_production_binding WHERE id=NEW.id;
    IF current_status IS DISTINCT FROM 'FINALIZED' THEN
        RAISE EXCEPTION 'gold production binding % must be FINALIZED before commit', NEW.id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_gold_binding_require_finalized
AFTER INSERT ON gold_production_binding
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_gold_binding_finalized_on_commit();
