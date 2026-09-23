-- #204 Gold annotation Core domain.
-- Core owns annotation/review/snapshot facts. Provider-specific runtime remains
-- outside these tables and will be introduced behind an adapter in #205.

CREATE TABLE annotation_campaign (
    id                              uuid PRIMARY KEY,
    workspace_id                    uuid NOT NULL,
    input_dataset_version_id        uuid NOT NULL REFERENCES dataset_version(id),
    input_certification_id          uuid NOT NULL REFERENCES dataset_certification(id),
    annotation_contribution_resource_id uuid NOT NULL REFERENCES data_resource(id),
    purpose                         varchar(128) NOT NULL,
    action                          varchar(64) NOT NULL,
    consumer_ref                    varchar(255),
    scope_type                      varchar(64),
    scope_ref                       varchar(512),

    schema_ref                      varchar(512) NOT NULL,
    schema_version                  varchar(64) NOT NULL,
    schema_content_sha256           varchar(64) NOT NULL,
    schema_content_snapshot         text NOT NULL,

    taxonomy_ref                    varchar(512) NOT NULL,
    taxonomy_version                varchar(64) NOT NULL,
    taxonomy_content_sha256         varchar(64) NOT NULL,
    taxonomy_content_snapshot       text NOT NULL,

    rubric_ref                      varchar(512) NOT NULL,
    rubric_version                  varchar(64) NOT NULL,
    rubric_content_sha256           varchar(64) NOT NULL,
    rubric_content_snapshot         text NOT NULL,

    renderer_ref                    varchar(512) NOT NULL,
    renderer_version                varchar(64) NOT NULL,
    renderer_content_sha256         varchar(64) NOT NULL,
    renderer_content_snapshot       text NOT NULL,

    review_policy_ref               varchar(512) NOT NULL,
    review_policy_version           varchar(64) NOT NULL,
    review_policy_content_sha256    varchar(64) NOT NULL,
    review_policy_content_snapshot  text NOT NULL,

    status                          varchar(16) NOT NULL DEFAULT 'DRAFT',
    revision                        bigint NOT NULL DEFAULT 1,
    expected_task_count             integer,
    task_manifest_hash              varchar(64),
    created_at                      timestamptz NOT NULL DEFAULT now(),
    created_by                      uuid,
    activated_at                    timestamptz,
    sealed_at                       timestamptz,
    cancelled_at                    timestamptz,

    CONSTRAINT ck_annotation_campaign_status CHECK (status IN ('DRAFT','ACTIVE','SEALED','CANCELLED')),
    CONSTRAINT ck_annotation_campaign_revision CHECK (revision >= 1),
    CONSTRAINT ck_annotation_campaign_task_count CHECK (expected_task_count IS NULL OR expected_task_count > 0),
    CONSTRAINT ck_annotation_campaign_schema_hash CHECK (
        schema_content_sha256 ~ '^[0-9a-f]{64}$'
        AND encode(digest(convert_to(schema_content_snapshot, 'UTF8'), 'sha256'), 'hex') = schema_content_sha256
    ),
    CONSTRAINT ck_annotation_campaign_taxonomy_hash CHECK (
        taxonomy_content_sha256 ~ '^[0-9a-f]{64}$'
        AND encode(digest(convert_to(taxonomy_content_snapshot, 'UTF8'), 'sha256'), 'hex') = taxonomy_content_sha256
    ),
    CONSTRAINT ck_annotation_campaign_rubric_hash CHECK (
        rubric_content_sha256 ~ '^[0-9a-f]{64}$'
        AND encode(digest(convert_to(rubric_content_snapshot, 'UTF8'), 'sha256'), 'hex') = rubric_content_sha256
    ),
    CONSTRAINT ck_annotation_campaign_renderer_hash CHECK (
        renderer_content_sha256 ~ '^[0-9a-f]{64}$'
        AND encode(digest(convert_to(renderer_content_snapshot, 'UTF8'), 'sha256'), 'hex') = renderer_content_sha256
    ),
    CONSTRAINT ck_annotation_campaign_review_policy_hash CHECK (
        review_policy_content_sha256 ~ '^[0-9a-f]{64}$'
        AND encode(digest(convert_to(review_policy_content_snapshot, 'UTF8'), 'sha256'), 'hex') = review_policy_content_sha256
    ),
    CONSTRAINT ck_annotation_campaign_manifest_hash CHECK (
        task_manifest_hash IS NULL OR task_manifest_hash ~ '^[0-9a-f]{64}$'
    )
);

CREATE INDEX idx_annotation_campaign_workspace
    ON annotation_campaign(workspace_id, created_at DESC, id);
CREATE INDEX idx_annotation_campaign_input
    ON annotation_campaign(input_dataset_version_id, created_at DESC, id);

CREATE TABLE annotation_campaign_command (
    workspace_id                uuid NOT NULL,
    idempotency_key             varchar(255) NOT NULL,
    request_fingerprint         varchar(64) NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    created_at                  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, idempotency_key),
    CONSTRAINT uq_annotation_campaign_command_campaign UNIQUE (campaign_id),
    CONSTRAINT ck_annotation_campaign_command_fingerprint CHECK (
        request_fingerprint ~ '^[0-9a-f]{64}$'
    )
);

CREATE OR REPLACE FUNCTION prevent_annotation_campaign_command_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'annotation campaign command mapping is immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_campaign_command_immutable
BEFORE UPDATE OR DELETE ON annotation_campaign_command
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_campaign_command_mutation();

CREATE TABLE annotation_task (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    source_item_ref             varchar(1024) NOT NULL,
    source_content_sha256       varchar(64) NOT NULL,
    task_text_sha256            varchar(64) NOT NULL,
    primary_annotator_ref       varchar(255),
    status                      varchar(16) NOT NULL DEFAULT 'PENDING',
    revision                    bigint NOT NULL DEFAULT 1,
    current_decision_id         uuid,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_task_source UNIQUE (campaign_id, source_item_ref),
    CONSTRAINT ck_annotation_task_status CHECK (status IN ('PENDING','REVIEWABLE','REVIEWED')),
    CONSTRAINT ck_annotation_task_revision CHECK (revision >= 1),
    CONSTRAINT ck_annotation_task_source_hash CHECK (source_content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_annotation_task_text_hash CHECK (task_text_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_annotation_task_campaign
    ON annotation_task(campaign_id, id);
CREATE INDEX idx_annotation_task_workspace
    ON annotation_task(workspace_id, campaign_id, status, id);

CREATE TABLE annotation_result (
    id                              uuid PRIMARY KEY,
    workspace_id                    uuid NOT NULL,
    campaign_id                     uuid NOT NULL REFERENCES annotation_campaign(id),
    task_id                         uuid NOT NULL REFERENCES annotation_task(id),
    author_ref                      varchar(255) NOT NULL,
    provider_binding_ref            varchar(512),
    external_task_id                varchar(512),
    external_annotation_id          varchar(512),
    external_revision               varchar(255),
    observation_key                 varchar(1024) NOT NULL,
    canonical_payload               bytea NOT NULL,
    canonical_payload_sha256        varchar(64) NOT NULL,
    normalizer_version              varchar(64) NOT NULL,
    corrected_from_result_id        uuid REFERENCES annotation_result(id),
    created_at                      timestamptz NOT NULL DEFAULT now(),
    created_by                      uuid,
    CONSTRAINT uq_annotation_result_observation UNIQUE (workspace_id, campaign_id, observation_key),
    CONSTRAINT ck_annotation_result_payload_hash CHECK (
        canonical_payload_sha256 ~ '^[0-9a-f]{64}$'
        AND encode(digest(canonical_payload, 'sha256'), 'hex') = canonical_payload_sha256
    ),
    CONSTRAINT ck_annotation_result_correction_self CHECK (
        corrected_from_result_id IS NULL OR corrected_from_result_id <> id
    )
);

CREATE INDEX idx_annotation_result_task
    ON annotation_result(task_id, created_at, id);

-- Physical human work is durable independently from the authoritative decision.
-- A review attempt is inserted before the Decision CAS transaction. If the CAS
-- loses, the attempt remains and gets a STALE_CONFLICT outcome/cost fact.
CREATE TABLE annotation_review_attempt (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    reviewer_ref                varchar(255) NOT NULL,
    expected_task_revision      bigint NOT NULL,
    review_action               varchar(16) NOT NULL,
    reason                      text NOT NULL,
    idempotency_key             varchar(255) NOT NULL,
    request_fingerprint         varchar(64) NOT NULL,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_review_attempt_key UNIQUE (workspace_id, idempotency_key),
    CONSTRAINT ck_annotation_review_attempt_revision CHECK (expected_task_revision >= 1),
    CONSTRAINT ck_annotation_review_attempt_action CHECK (review_action IN ('ACCEPT','REJECT','CORRECT')),
    CONSTRAINT ck_annotation_review_attempt_reason CHECK (length(btrim(reason)) > 0),
    CONSTRAINT ck_annotation_review_attempt_fingerprint CHECK (request_fingerprint ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_annotation_review_attempt_task
    ON annotation_review_attempt(task_id, created_at, id);

CREATE TABLE annotation_review_attempt_outcome (
    id                          uuid PRIMARY KEY,
    attempt_id                  uuid NOT NULL REFERENCES annotation_review_attempt(id),
    outcome                     varchar(32) NOT NULL,
    error_code                  varchar(128),
    occurred_at                 timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_review_attempt_outcome UNIQUE (attempt_id),
    CONSTRAINT ck_annotation_review_attempt_outcome CHECK (
        outcome IN ('SUCCEEDED','STALE_CONFLICT','REJECTED')
    )
);

CREATE TABLE annotation_review_decision (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    review_attempt_id           uuid NOT NULL REFERENCES annotation_review_attempt(id),
    reviewed_result_id          uuid REFERENCES annotation_result(id),
    selected_result_id          uuid REFERENCES annotation_result(id),
    reviewer_ref                varchar(255) NOT NULL,
    outcome                     varchar(16) NOT NULL,
    reason                      text NOT NULL,
    expected_task_revision      bigint NOT NULL,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_review_decision_task UNIQUE (task_id),
    CONSTRAINT uq_annotation_review_decision_attempt UNIQUE (review_attempt_id),
    CONSTRAINT ck_annotation_review_decision_outcome CHECK (outcome IN ('ACCEPT','REJECT','CORRECT')),
    CONSTRAINT ck_annotation_review_decision_reason CHECK (length(btrim(reason)) > 0),
    CONSTRAINT ck_annotation_review_decision_revision CHECK (expected_task_revision >= 1),
    CONSTRAINT ck_annotation_review_decision_selection CHECK (
        (outcome='ACCEPT' AND reviewed_result_id IS NOT NULL AND selected_result_id = reviewed_result_id)
        OR
        (outcome='CORRECT' AND reviewed_result_id IS NOT NULL AND selected_result_id IS NOT NULL AND selected_result_id <> reviewed_result_id)
        OR
        (outcome='REJECT' AND selected_result_id IS NULL)
    )
);

ALTER TABLE annotation_task
    ADD CONSTRAINT fk_annotation_task_current_decision
    FOREIGN KEY (current_decision_id) REFERENCES annotation_review_decision(id);

CREATE INDEX idx_annotation_review_decision_campaign
    ON annotation_review_decision(campaign_id, created_at, id);

CREATE TABLE annotation_snapshot (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    status                      varchar(16) NOT NULL DEFAULT 'BUILDING',
    manifest                    jsonb NOT NULL,
    manifest_hash_payload       bytea NOT NULL,
    root_hash                   varchar(64) NOT NULL,
    expected_task_count         integer NOT NULL,
    expected_result_count       integer NOT NULL,
    expected_decision_count     integer NOT NULL,
    expected_output_count       integer NOT NULL,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    created_by                  uuid,
    finalized_at                timestamptz,
    CONSTRAINT uq_annotation_snapshot_campaign UNIQUE (campaign_id),
    CONSTRAINT ck_annotation_snapshot_status CHECK (status IN ('BUILDING','FINALIZED')),
    CONSTRAINT ck_annotation_snapshot_root_hash CHECK (root_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_annotation_snapshot_counts CHECK (
        expected_task_count > 0
        AND expected_result_count >= 0
        AND expected_decision_count = expected_task_count
        AND expected_output_count >= 0
        AND expected_output_count <= expected_task_count
    )
);

CREATE TABLE annotation_snapshot_task (
    snapshot_id                 uuid NOT NULL REFERENCES annotation_snapshot(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    source_item_ref             varchar(1024) NOT NULL,
    source_content_sha256       varchar(64) NOT NULL,
    task_text_sha256            varchar(64) NOT NULL,
    PRIMARY KEY (snapshot_id, task_id)
);

CREATE TABLE annotation_snapshot_result (
    snapshot_id                 uuid NOT NULL REFERENCES annotation_snapshot(id),
    result_id                   uuid NOT NULL REFERENCES annotation_result(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    canonical_payload_sha256    varchar(64) NOT NULL,
    author_ref                  varchar(255) NOT NULL,
    PRIMARY KEY (snapshot_id, result_id)
);

CREATE TABLE annotation_snapshot_decision (
    snapshot_id                 uuid NOT NULL REFERENCES annotation_snapshot(id),
    decision_id                 uuid NOT NULL REFERENCES annotation_review_decision(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    outcome                     varchar(16) NOT NULL,
    reviewed_result_id          uuid,
    selected_result_id          uuid,
    reviewer_ref                varchar(255) NOT NULL,
    reason                      text NOT NULL,
    PRIMARY KEY (snapshot_id, decision_id),
    CONSTRAINT uq_annotation_snapshot_decision_task UNIQUE (snapshot_id, task_id)
);

CREATE TABLE annotation_snapshot_output (
    snapshot_id                 uuid NOT NULL REFERENCES annotation_snapshot(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    selected_result_id          uuid NOT NULL REFERENCES annotation_result(id),
    PRIMARY KEY (snapshot_id, task_id),
    CONSTRAINT uq_annotation_snapshot_output_result UNIQUE (snapshot_id, selected_result_id)
);

CREATE OR REPLACE FUNCTION prevent_annotation_append_only_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only annotation history', TG_TABLE_NAME;
END;
$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_campaign_command_immutable
BEFORE UPDATE OR DELETE ON annotation_campaign_command
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE TRIGGER trg_annotation_result_immutable
BEFORE UPDATE OR DELETE ON annotation_result
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE TRIGGER trg_annotation_review_attempt_immutable
BEFORE UPDATE OR DELETE ON annotation_review_attempt
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE TRIGGER trg_annotation_review_attempt_outcome_immutable
BEFORE UPDATE OR DELETE ON annotation_review_attempt_outcome
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE TRIGGER trg_annotation_review_decision_immutable
BEFORE UPDATE OR DELETE ON annotation_review_decision
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE OR REPLACE FUNCTION validate_annotation_campaign_insert()
RETURNS trigger AS $$
DECLARE
    dataset_workspace uuid;
    certification_workspace uuid;
    certification_dataset_version uuid;
    certification_decision varchar(16);
    resource_workspace uuid;
BEGIN
    IF NEW.status <> 'DRAFT' OR NEW.revision <> 1 THEN
        RAISE EXCEPTION 'new annotation campaign must start DRAFT at revision 1';
    END IF;

    SELECT d.workspace_id
      INTO dataset_workspace
      FROM dataset_version v
      JOIN dataset d ON d.id=v.dataset_id
     WHERE v.id=NEW.input_dataset_version_id;

    SELECT workspace_id, dataset_version_id, decision
      INTO certification_workspace, certification_dataset_version, certification_decision
      FROM dataset_certification
     WHERE id=NEW.input_certification_id;

    SELECT workspace_id
      INTO resource_workspace
      FROM data_resource
     WHERE id=NEW.annotation_contribution_resource_id;

    IF dataset_workspace IS DISTINCT FROM NEW.workspace_id
       OR certification_workspace IS DISTINCT FROM NEW.workspace_id
       OR certification_dataset_version IS DISTINCT FROM NEW.input_dataset_version_id
       OR certification_decision <> 'CERTIFIED'
       OR resource_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation campaign input/certification/contribution resource does not match workspace';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_campaign_insert
BEFORE INSERT ON annotation_campaign
FOR EACH ROW EXECUTE FUNCTION validate_annotation_campaign_insert();

CREATE OR REPLACE FUNCTION guard_annotation_campaign_update()
RETURNS trigger AS $$
DECLARE
    task_count integer;
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.input_dataset_version_id IS DISTINCT FROM OLD.input_dataset_version_id
       OR NEW.input_certification_id IS DISTINCT FROM OLD.input_certification_id
       OR NEW.annotation_contribution_resource_id IS DISTINCT FROM OLD.annotation_contribution_resource_id
       OR NEW.purpose IS DISTINCT FROM OLD.purpose
       OR NEW.action IS DISTINCT FROM OLD.action
       OR NEW.consumer_ref IS DISTINCT FROM OLD.consumer_ref
       OR NEW.scope_type IS DISTINCT FROM OLD.scope_type
       OR NEW.scope_ref IS DISTINCT FROM OLD.scope_ref
       OR NEW.schema_ref IS DISTINCT FROM OLD.schema_ref
       OR NEW.schema_version IS DISTINCT FROM OLD.schema_version
       OR NEW.schema_content_sha256 IS DISTINCT FROM OLD.schema_content_sha256
       OR NEW.schema_content_snapshot IS DISTINCT FROM OLD.schema_content_snapshot
       OR NEW.taxonomy_ref IS DISTINCT FROM OLD.taxonomy_ref
       OR NEW.taxonomy_version IS DISTINCT FROM OLD.taxonomy_version
       OR NEW.taxonomy_content_sha256 IS DISTINCT FROM OLD.taxonomy_content_sha256
       OR NEW.taxonomy_content_snapshot IS DISTINCT FROM OLD.taxonomy_content_snapshot
       OR NEW.rubric_ref IS DISTINCT FROM OLD.rubric_ref
       OR NEW.rubric_version IS DISTINCT FROM OLD.rubric_version
       OR NEW.rubric_content_sha256 IS DISTINCT FROM OLD.rubric_content_sha256
       OR NEW.rubric_content_snapshot IS DISTINCT FROM OLD.rubric_content_snapshot
       OR NEW.renderer_ref IS DISTINCT FROM OLD.renderer_ref
       OR NEW.renderer_version IS DISTINCT FROM OLD.renderer_version
       OR NEW.renderer_content_sha256 IS DISTINCT FROM OLD.renderer_content_sha256
       OR NEW.renderer_content_snapshot IS DISTINCT FROM OLD.renderer_content_snapshot
       OR NEW.review_policy_ref IS DISTINCT FROM OLD.review_policy_ref
       OR NEW.review_policy_version IS DISTINCT FROM OLD.review_policy_version
       OR NEW.review_policy_content_sha256 IS DISTINCT FROM OLD.review_policy_content_sha256
       OR NEW.review_policy_content_snapshot IS DISTINCT FROM OLD.review_policy_content_snapshot
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.created_by IS DISTINCT FROM OLD.created_by THEN
        RAISE EXCEPTION 'annotation campaign frozen semantics are immutable';
    END IF;

    IF OLD.status='DRAFT' AND NEW.status='ACTIVE' THEN
        SELECT count(*) INTO task_count FROM annotation_task WHERE campaign_id=OLD.id;
        IF NEW.expected_task_count IS NULL OR NEW.expected_task_count <> task_count OR task_count < 1
           OR NEW.task_manifest_hash IS NULL OR NEW.activated_at IS NULL
           OR NEW.revision <> OLD.revision + 1 THEN
            RAISE EXCEPTION 'annotation campaign activation requires exact non-empty task manifest and revision CAS';
        END IF;
        RETURN NEW;
    END IF;

    IF OLD.status='DRAFT' AND NEW.status='CANCELLED' THEN
        IF NEW.cancelled_at IS NULL OR NEW.revision <> OLD.revision + 1 THEN
            RAISE EXCEPTION 'annotation campaign cancellation requires timestamp and revision CAS';
        END IF;
        RETURN NEW;
    END IF;

    IF OLD.status='ACTIVE' AND NEW.status='CANCELLED' THEN
        IF NEW.cancelled_at IS NULL OR NEW.revision <> OLD.revision + 1 THEN
            RAISE EXCEPTION 'annotation campaign cancellation requires timestamp and revision CAS';
        END IF;
        RETURN NEW;
    END IF;

    IF OLD.status='ACTIVE' AND NEW.status='SEALED' THEN
        IF NEW.sealed_at IS NULL OR NEW.revision <> OLD.revision + 1 THEN
            RAISE EXCEPTION 'annotation campaign seal requires timestamp and revision CAS';
        END IF;
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'invalid annotation campaign transition % -> %', OLD.status, NEW.status;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_campaign_update
BEFORE UPDATE ON annotation_campaign
FOR EACH ROW EXECUTE FUNCTION guard_annotation_campaign_update();

CREATE TRIGGER trg_annotation_campaign_delete
BEFORE DELETE ON annotation_campaign
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE OR REPLACE FUNCTION guard_annotation_task_insert()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    campaign_workspace uuid;
BEGIN
    SELECT status, workspace_id
      INTO campaign_status, campaign_workspace
      FROM annotation_campaign
     WHERE id=NEW.campaign_id
     FOR UPDATE;

    IF campaign_status <> 'DRAFT' OR campaign_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation task requires DRAFT campaign in same workspace';
    END IF;
    IF NEW.status <> 'PENDING' OR NEW.revision <> 1 OR NEW.current_decision_id IS NOT NULL THEN
        RAISE EXCEPTION 'new annotation task must start PENDING at revision 1';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_task_insert
BEFORE INSERT ON annotation_task
FOR EACH ROW EXECUTE FUNCTION guard_annotation_task_insert();

CREATE OR REPLACE FUNCTION guard_annotation_task_update()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
BEGIN
    SELECT status
      INTO campaign_status
      FROM annotation_campaign
     WHERE id=OLD.campaign_id
     FOR UPDATE;
    IF campaign_status <> 'ACTIVE' THEN
        RAISE EXCEPTION 'annotation task is immutable outside ACTIVE campaign';
    END IF;

    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.campaign_id IS DISTINCT FROM OLD.campaign_id
       OR NEW.source_item_ref IS DISTINCT FROM OLD.source_item_ref
       OR NEW.source_content_sha256 IS DISTINCT FROM OLD.source_content_sha256
       OR NEW.task_text_sha256 IS DISTINCT FROM OLD.task_text_sha256
       OR NEW.primary_annotator_ref IS DISTINCT FROM OLD.primary_annotator_ref
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'annotation task identity/content is immutable';
    END IF;
    IF NEW.revision <> OLD.revision + 1 THEN
        RAISE EXCEPTION 'annotation task update requires revision CAS';
    END IF;
    IF OLD.status='PENDING' AND NEW.status='REVIEWABLE' AND NEW.current_decision_id IS NULL THEN
        RETURN NEW;
    END IF;
    IF OLD.status='REVIEWABLE' AND NEW.status='REVIEWABLE' AND NEW.current_decision_id IS NULL THEN
        RETURN NEW;
    END IF;
    IF OLD.status IN ('PENDING','REVIEWABLE')
       AND NEW.status='REVIEWED'
       AND NEW.current_decision_id IS NOT NULL THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'invalid annotation task transition % -> %', OLD.status, NEW.status;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_task_update
BEFORE UPDATE ON annotation_task
FOR EACH ROW EXECUTE FUNCTION guard_annotation_task_update();

CREATE TRIGGER trg_annotation_task_delete
BEFORE DELETE ON annotation_task
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE OR REPLACE FUNCTION guard_annotation_result_insert()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    task_workspace uuid;
    task_campaign uuid;
    task_status varchar(16);
    corrected_task uuid;
BEGIN
    SELECT status INTO campaign_status
      FROM annotation_campaign
     WHERE id=NEW.campaign_id
     FOR UPDATE;
    IF campaign_status <> 'ACTIVE' THEN
        RAISE EXCEPTION 'annotation result requires ACTIVE campaign';
    END IF;

    SELECT workspace_id, campaign_id, status
      INTO task_workspace, task_campaign, task_status
      FROM annotation_task
     WHERE id=NEW.task_id
     FOR UPDATE;

    IF task_workspace IS DISTINCT FROM NEW.workspace_id
       OR task_campaign IS DISTINCT FROM NEW.campaign_id
       OR task_status='REVIEWED' THEN
        RAISE EXCEPTION 'annotation result does not match active task';
    END IF;

    IF NEW.corrected_from_result_id IS NOT NULL THEN
        SELECT task_id INTO corrected_task FROM annotation_result WHERE id=NEW.corrected_from_result_id;
        IF corrected_task IS DISTINCT FROM NEW.task_id THEN
            RAISE EXCEPTION 'annotation correction must reference a result from the same task';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_result_insert
BEFORE INSERT ON annotation_result
FOR EACH ROW EXECUTE FUNCTION guard_annotation_result_insert();

CREATE OR REPLACE FUNCTION validate_annotation_review_attempt_insert()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    task_workspace uuid;
    task_campaign uuid;
    annotator_ref varchar(255);
BEGIN
    SELECT status INTO campaign_status
      FROM annotation_campaign
     WHERE id=NEW.campaign_id
     FOR UPDATE;
    SELECT workspace_id, campaign_id, primary_annotator_ref
      INTO task_workspace, task_campaign, annotator_ref
      FROM annotation_task
     WHERE id=NEW.task_id;

    IF campaign_status <> 'ACTIVE'
       OR task_workspace IS DISTINCT FROM NEW.workspace_id
       OR task_campaign IS DISTINCT FROM NEW.campaign_id THEN
        RAISE EXCEPTION 'annotation review attempt does not match active task';
    END IF;
    IF annotator_ref IS NOT NULL AND annotator_ref = NEW.reviewer_ref THEN
        RAISE EXCEPTION 'annotation reviewer must differ from primary annotator';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_review_attempt_insert
BEFORE INSERT ON annotation_review_attempt
FOR EACH ROW EXECUTE FUNCTION validate_annotation_review_attempt_insert();

CREATE OR REPLACE FUNCTION validate_annotation_review_attempt_outcome_insert()
RETURNS trigger AS $$
BEGIN
    IF NEW.outcome='SUCCEEDED' AND NOT EXISTS (
        SELECT 1 FROM annotation_review_decision d WHERE d.review_attempt_id=NEW.attempt_id
    ) THEN
        RAISE EXCEPTION 'successful annotation review attempt requires authoritative decision';
    END IF;
    IF NEW.outcome<>'SUCCEEDED' AND EXISTS (
        SELECT 1 FROM annotation_review_decision d WHERE d.review_attempt_id=NEW.attempt_id
    ) THEN
        RAISE EXCEPTION 'non-success annotation review attempt cannot own authoritative decision';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_review_attempt_outcome_insert
BEFORE INSERT ON annotation_review_attempt_outcome
FOR EACH ROW EXECUTE FUNCTION validate_annotation_review_attempt_outcome_insert();

CREATE OR REPLACE FUNCTION validate_annotation_review_decision_insert()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    task_row annotation_task%ROWTYPE;
    attempt_row annotation_review_attempt%ROWTYPE;
    reviewed_task uuid;
    selected_task uuid;
    selected_corrected_from uuid;
BEGIN
    SELECT status INTO campaign_status
      FROM annotation_campaign
     WHERE id=NEW.campaign_id
     FOR UPDATE;
    IF campaign_status <> 'ACTIVE' THEN
        RAISE EXCEPTION 'annotation review requires ACTIVE campaign';
    END IF;

    SELECT * INTO task_row
      FROM annotation_task
     WHERE id=NEW.task_id
     FOR UPDATE;

    SELECT * INTO attempt_row
      FROM annotation_review_attempt
     WHERE id=NEW.review_attempt_id;

    IF task_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR task_row.campaign_id IS DISTINCT FROM NEW.campaign_id
       OR task_row.current_decision_id IS NOT NULL
       OR task_row.status='REVIEWED'
       OR task_row.revision IS DISTINCT FROM NEW.expected_task_revision
       OR attempt_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR attempt_row.campaign_id IS DISTINCT FROM NEW.campaign_id
       OR attempt_row.task_id IS DISTINCT FROM NEW.task_id
       OR attempt_row.reviewer_ref IS DISTINCT FROM NEW.reviewer_ref
       OR attempt_row.expected_task_revision IS DISTINCT FROM NEW.expected_task_revision
       OR attempt_row.review_action IS DISTINCT FROM NEW.outcome
       OR attempt_row.reason IS DISTINCT FROM NEW.reason THEN
        RAISE EXCEPTION 'annotation review decision failed expected task/attempt CAS';
    END IF;

    IF task_row.primary_annotator_ref IS NOT NULL
       AND task_row.primary_annotator_ref = NEW.reviewer_ref THEN
        RAISE EXCEPTION 'annotation reviewer must differ from primary annotator';
    END IF;

    IF NEW.reviewed_result_id IS NOT NULL THEN
        SELECT task_id INTO reviewed_task FROM annotation_result WHERE id=NEW.reviewed_result_id;
        IF reviewed_task IS DISTINCT FROM NEW.task_id THEN
            RAISE EXCEPTION 'reviewed annotation result does not belong to task';
        END IF;
    END IF;

    IF NEW.selected_result_id IS NOT NULL THEN
        SELECT task_id, corrected_from_result_id
          INTO selected_task, selected_corrected_from
          FROM annotation_result
         WHERE id=NEW.selected_result_id;
        IF selected_task IS DISTINCT FROM NEW.task_id THEN
            RAISE EXCEPTION 'selected annotation result does not belong to task';
        END IF;
    END IF;

    IF NEW.outcome='CORRECT' AND selected_corrected_from IS DISTINCT FROM NEW.reviewed_result_id THEN
        RAISE EXCEPTION 'corrected annotation result must point to reviewed result';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_review_decision_insert
BEFORE INSERT ON annotation_review_decision
FOR EACH ROW EXECUTE FUNCTION validate_annotation_review_decision_insert();

CREATE OR REPLACE FUNCTION project_annotation_review_decision()
RETURNS trigger AS $$
BEGIN
    UPDATE annotation_task
       SET status='REVIEWED',
           current_decision_id=NEW.id,
           revision=revision+1
     WHERE id=NEW.task_id
       AND revision=NEW.expected_task_revision
       AND current_decision_id IS NULL
       AND status IN ('PENDING','REVIEWABLE');

    IF NOT FOUND THEN
        RAISE EXCEPTION 'annotation review decision lost task CAS';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_review_decision_projection
AFTER INSERT ON annotation_review_decision
FOR EACH ROW EXECUTE FUNCTION project_annotation_review_decision();

CREATE OR REPLACE FUNCTION validate_annotation_snapshot_insert()
RETURNS trigger AS $$
DECLARE
    campaign_workspace uuid;
BEGIN
    IF NEW.status <> 'BUILDING' OR NEW.finalized_at IS NOT NULL THEN
        RAISE EXCEPTION 'new annotation snapshot must start BUILDING';
    END IF;
    SELECT workspace_id INTO campaign_workspace FROM annotation_campaign WHERE id=NEW.campaign_id;
    IF campaign_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation snapshot crosses workspace boundary';
    END IF;
    IF convert_from(NEW.manifest_hash_payload, 'UTF8')::jsonb IS DISTINCT FROM NEW.manifest THEN
        RAISE EXCEPTION 'annotation snapshot hash payload does not match manifest';
    END IF;
    IF encode(digest(NEW.manifest_hash_payload, 'sha256'), 'hex') IS DISTINCT FROM NEW.root_hash THEN
        RAISE EXCEPTION 'annotation snapshot root hash does not match manifest payload';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_snapshot_insert
BEFORE INSERT ON annotation_snapshot
FOR EACH ROW EXECUTE FUNCTION validate_annotation_snapshot_insert();

CREATE OR REPLACE FUNCTION guard_annotation_snapshot_membership()
RETURNS trigger AS $$
DECLARE
    target_snapshot uuid;
    parent_status varchar(16);
BEGIN
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'annotation snapshot membership is immutable';
    END IF;

    target_snapshot := NEW.snapshot_id;
    SELECT status INTO parent_status
      FROM annotation_snapshot
     WHERE id=target_snapshot
     FOR UPDATE;

    IF parent_status <> 'BUILDING' THEN
        RAISE EXCEPTION 'annotation snapshot membership is finalized';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_snapshot_task_membership
BEFORE INSERT OR UPDATE OR DELETE ON annotation_snapshot_task
FOR EACH ROW EXECUTE FUNCTION guard_annotation_snapshot_membership();
CREATE TRIGGER trg_annotation_snapshot_result_membership
BEFORE INSERT OR UPDATE OR DELETE ON annotation_snapshot_result
FOR EACH ROW EXECUTE FUNCTION guard_annotation_snapshot_membership();
CREATE TRIGGER trg_annotation_snapshot_decision_membership
BEFORE INSERT OR UPDATE OR DELETE ON annotation_snapshot_decision
FOR EACH ROW EXECUTE FUNCTION guard_annotation_snapshot_membership();
CREATE TRIGGER trg_annotation_snapshot_output_membership
BEFORE INSERT OR UPDATE OR DELETE ON annotation_snapshot_output
FOR EACH ROW EXECUTE FUNCTION guard_annotation_snapshot_membership();

CREATE OR REPLACE FUNCTION prevent_annotation_snapshot_mutation()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    campaign_workspace uuid;
    campaign_expected integer;
    campaign_input_version uuid;
    campaign_input_certification uuid;
    campaign_contribution_resource uuid;
    campaign_task_manifest_hash varchar(64);
    campaign_schema_hash varchar(64);
    campaign_taxonomy_hash varchar(64);
    campaign_rubric_hash varchar(64);
    campaign_renderer_hash varchar(64);
    campaign_review_policy_hash varchar(64);
    task_count integer;
    result_count integer;
    decision_count integer;
    output_count integer;
    persisted_tasks jsonb;
    persisted_results jsonb;
    persisted_decisions jsonb;
    persisted_outputs jsonb;
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'annotation snapshot is immutable';
    END IF;

    IF OLD.status <> 'BUILDING' OR NEW.status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'annotation snapshot is immutable';
    END IF;

    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.campaign_id IS DISTINCT FROM OLD.campaign_id
       OR NEW.manifest IS DISTINCT FROM OLD.manifest
       OR NEW.manifest_hash_payload IS DISTINCT FROM OLD.manifest_hash_payload
       OR NEW.root_hash IS DISTINCT FROM OLD.root_hash
       OR NEW.expected_task_count IS DISTINCT FROM OLD.expected_task_count
       OR NEW.expected_result_count IS DISTINCT FROM OLD.expected_result_count
       OR NEW.expected_decision_count IS DISTINCT FROM OLD.expected_decision_count
       OR NEW.expected_output_count IS DISTINCT FROM OLD.expected_output_count
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.created_by IS DISTINCT FROM OLD.created_by
       OR NEW.finalized_at IS NULL THEN
        RAISE EXCEPTION 'annotation snapshot content is immutable';
    END IF;

    SELECT status, workspace_id, expected_task_count,
           input_dataset_version_id, input_certification_id, annotation_contribution_resource_id,
           task_manifest_hash, schema_content_sha256, taxonomy_content_sha256,
           rubric_content_sha256, renderer_content_sha256, review_policy_content_sha256
      INTO campaign_status, campaign_workspace, campaign_expected,
           campaign_input_version, campaign_input_certification, campaign_contribution_resource,
           campaign_task_manifest_hash, campaign_schema_hash, campaign_taxonomy_hash,
           campaign_rubric_hash, campaign_renderer_hash, campaign_review_policy_hash
      FROM annotation_campaign
     WHERE id=OLD.campaign_id
     FOR UPDATE;

    IF campaign_status <> 'ACTIVE'
       OR campaign_workspace IS DISTINCT FROM OLD.workspace_id
       OR campaign_expected IS DISTINCT FROM OLD.expected_task_count THEN
        RAISE EXCEPTION 'annotation snapshot campaign is not sealable';
    END IF;

    IF NEW.manifest->>'formatVersion' IS DISTINCT FROM 'annotation-snapshot-v1'
       OR NEW.manifest->>'campaignId' IS DISTINCT FROM OLD.campaign_id::text
       OR NEW.manifest->>'inputDatasetVersionId' IS DISTINCT FROM campaign_input_version::text
       OR NEW.manifest->>'inputCertificationId' IS DISTINCT FROM campaign_input_certification::text
       OR NEW.manifest->>'annotationContributionResourceId' IS DISTINCT FROM campaign_contribution_resource::text
       OR NEW.manifest->>'taskManifestHash' IS DISTINCT FROM campaign_task_manifest_hash
       OR NEW.manifest->>'schemaHash' IS DISTINCT FROM campaign_schema_hash
       OR NEW.manifest->>'taxonomyHash' IS DISTINCT FROM campaign_taxonomy_hash
       OR NEW.manifest->>'rubricHash' IS DISTINCT FROM campaign_rubric_hash
       OR NEW.manifest->>'rendererHash' IS DISTINCT FROM campaign_renderer_hash
       OR NEW.manifest->>'reviewPolicyHash' IS DISTINCT FROM campaign_review_policy_hash THEN
        RAISE EXCEPTION 'annotation snapshot manifest header does not match frozen campaign facts';
    END IF;

    SELECT count(*) INTO task_count FROM annotation_snapshot_task WHERE snapshot_id=OLD.id;
    SELECT count(*) INTO result_count FROM annotation_snapshot_result WHERE snapshot_id=OLD.id;
    SELECT count(*) INTO decision_count FROM annotation_snapshot_decision WHERE snapshot_id=OLD.id;
    SELECT count(*) INTO output_count FROM annotation_snapshot_output WHERE snapshot_id=OLD.id;

    IF task_count <> OLD.expected_task_count
       OR result_count <> OLD.expected_result_count
       OR decision_count <> OLD.expected_decision_count
       OR output_count <> OLD.expected_output_count
       OR decision_count <> task_count THEN
        RAISE EXCEPTION 'annotation snapshot persisted membership does not match expected counts';
    END IF;

    IF jsonb_typeof(NEW.manifest->'tasks') IS DISTINCT FROM 'array'
       OR jsonb_typeof(NEW.manifest->'results') IS DISTINCT FROM 'array'
       OR jsonb_typeof(NEW.manifest->'decisions') IS DISTINCT FROM 'array'
       OR jsonb_typeof(NEW.manifest->'outputs') IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION 'annotation snapshot manifest must contain membership arrays';
    END IF;

    SELECT COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'id', st.task_id::text,
                'sourceItemRef', st.source_item_ref,
                'sourceContentSha256', st.source_content_sha256,
                'taskTextSha256', st.task_text_sha256
            ) ORDER BY st.task_id::text
        ),
        '[]'::jsonb
    ) INTO persisted_tasks
    FROM annotation_snapshot_task st
    WHERE st.snapshot_id=OLD.id;

    SELECT COALESCE(
        jsonb_agg(
            jsonb_strip_nulls(jsonb_build_object(
                'id', sr.result_id::text,
                'taskId', sr.task_id::text,
                'canonicalPayloadSha256', sr.canonical_payload_sha256,
                'authorRef', sr.author_ref,
                'correctedFromResultId', r.corrected_from_result_id::text
            )) ORDER BY sr.result_id::text
        ),
        '[]'::jsonb
    ) INTO persisted_results
    FROM annotation_snapshot_result sr
    JOIN annotation_result r ON r.id=sr.result_id
    WHERE sr.snapshot_id=OLD.id;

    SELECT COALESCE(
        jsonb_agg(
            jsonb_strip_nulls(jsonb_build_object(
                'id', sd.decision_id::text,
                'taskId', sd.task_id::text,
                'outcome', sd.outcome,
                'reviewedResultId', sd.reviewed_result_id::text,
                'selectedResultId', sd.selected_result_id::text,
                'reviewerRef', sd.reviewer_ref,
                'reason', sd.reason
            )) ORDER BY sd.decision_id::text
        ),
        '[]'::jsonb
    ) INTO persisted_decisions
    FROM annotation_snapshot_decision sd
    WHERE sd.snapshot_id=OLD.id;

    SELECT COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'taskId', so.task_id::text,
                'selectedResultId', so.selected_result_id::text
            ) ORDER BY so.task_id::text
        ),
        '[]'::jsonb
    ) INTO persisted_outputs
    FROM annotation_snapshot_output so
    WHERE so.snapshot_id=OLD.id;

    IF NEW.manifest->'tasks' IS DISTINCT FROM persisted_tasks
       OR NEW.manifest->'results' IS DISTINCT FROM persisted_results
       OR NEW.manifest->'decisions' IS DISTINCT FROM persisted_decisions
       OR NEW.manifest->'outputs' IS DISTINCT FROM persisted_outputs THEN
        RAISE EXCEPTION 'annotation snapshot manifest membership does not match persisted membership';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_task t
         WHERE t.campaign_id=OLD.campaign_id
           AND NOT EXISTS (
               SELECT 1 FROM annotation_snapshot_task st
                WHERE st.snapshot_id=OLD.id AND st.task_id=t.id
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_snapshot_task st
          JOIN annotation_task t ON t.id=st.task_id
         WHERE st.snapshot_id=OLD.id
           AND t.campaign_id<>OLD.campaign_id
    ) THEN
        RAISE EXCEPTION 'annotation snapshot task membership is not exact campaign task set';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_result r
         WHERE r.campaign_id=OLD.campaign_id
           AND NOT EXISTS (
               SELECT 1 FROM annotation_snapshot_result sr
                WHERE sr.snapshot_id=OLD.id AND sr.result_id=r.id
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_review_decision d
         WHERE d.campaign_id=OLD.campaign_id
           AND NOT EXISTS (
               SELECT 1 FROM annotation_snapshot_decision sd
                WHERE sd.snapshot_id=OLD.id AND sd.decision_id=d.id
           )
    ) THEN
        RAISE EXCEPTION 'annotation snapshot result/decision membership is incomplete';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_snapshot_task st
          JOIN annotation_task t ON t.id=st.task_id
         WHERE st.snapshot_id=OLD.id
           AND (
               st.source_item_ref IS DISTINCT FROM t.source_item_ref
               OR st.source_content_sha256 IS DISTINCT FROM t.source_content_sha256
               OR st.task_text_sha256 IS DISTINCT FROM t.task_text_sha256
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_snapshot_result sr
          JOIN annotation_result r ON r.id=sr.result_id
         WHERE sr.snapshot_id=OLD.id
           AND (
               sr.task_id IS DISTINCT FROM r.task_id
               OR sr.canonical_payload_sha256 IS DISTINCT FROM r.canonical_payload_sha256
               OR sr.author_ref IS DISTINCT FROM r.author_ref
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_snapshot_decision sd
          JOIN annotation_review_decision d ON d.id=sd.decision_id
         WHERE sd.snapshot_id=OLD.id
           AND (
               sd.task_id IS DISTINCT FROM d.task_id
               OR sd.outcome IS DISTINCT FROM d.outcome
               OR sd.reviewed_result_id IS DISTINCT FROM d.reviewed_result_id
               OR sd.selected_result_id IS DISTINCT FROM d.selected_result_id
               OR sd.reviewer_ref IS DISTINCT FROM d.reviewer_ref
               OR sd.reason IS DISTINCT FROM d.reason
           )
    ) THEN
        RAISE EXCEPTION 'annotation snapshot frozen membership does not match source facts';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_task t
         WHERE t.campaign_id=OLD.campaign_id
           AND (t.status<>'REVIEWED' OR t.current_decision_id IS NULL)
    ) THEN
        RAISE EXCEPTION 'annotation snapshot requires every task terminally reviewed';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_snapshot_output o
          JOIN annotation_review_decision d
            ON d.task_id=o.task_id
         WHERE o.snapshot_id=OLD.id
           AND (d.outcome NOT IN ('ACCEPT','CORRECT') OR d.selected_result_id IS DISTINCT FROM o.selected_result_id)
    ) OR EXISTS (
        SELECT 1
          FROM annotation_review_decision d
         WHERE d.campaign_id=OLD.campaign_id
           AND d.outcome IN ('ACCEPT','CORRECT')
           AND NOT EXISTS (
               SELECT 1 FROM annotation_snapshot_output o
                WHERE o.snapshot_id=OLD.id
                  AND o.task_id=d.task_id
                  AND o.selected_result_id=d.selected_result_id
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_review_decision d
         WHERE d.campaign_id=OLD.campaign_id
           AND d.outcome='REJECT'
           AND EXISTS (
               SELECT 1 FROM annotation_snapshot_output o
                WHERE o.snapshot_id=OLD.id AND o.task_id=d.task_id
           )
    ) THEN
        RAISE EXCEPTION 'annotation snapshot output membership does not match authoritative decisions';
    END IF;

    UPDATE annotation_campaign
       SET status='SEALED', sealed_at=NEW.finalized_at, revision=revision+1
     WHERE id=OLD.campaign_id
       AND status='ACTIVE';

    IF NOT FOUND THEN
        RAISE EXCEPTION 'annotation campaign seal lost CAS';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_snapshot_immutable
BEFORE UPDATE OR DELETE ON annotation_snapshot
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_snapshot_mutation();

CREATE OR REPLACE FUNCTION require_annotation_snapshot_finalized_on_commit()
RETURNS trigger AS $$
DECLARE
    current_status varchar(16);
BEGIN
    SELECT status INTO current_status FROM annotation_snapshot WHERE id=NEW.id;
    IF current_status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'annotation_snapshot % must be FINALIZED before commit', NEW.id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_annotation_snapshot_finalized_on_commit
AFTER INSERT ON annotation_snapshot
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_annotation_snapshot_finalized_on_commit();

-- Review attempts are typed physical cost subjects so stale/failed human work
-- remains auditable even when no authoritative ReviewDecision commits.
ALTER TABLE cost_allocation
    ADD COLUMN annotation_review_attempt_id uuid REFERENCES annotation_review_attempt(id);

ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
    (CASE WHEN delivery_operation_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_attempt_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_verification_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN authorization_provenance_binding_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN authorization_provenance_binding_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN effective_rights_snapshot_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN dataset_certification_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN certification_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN annotation_review_attempt_id IS NOT NULL THEN 1 ELSE 0 END) = 1
);

CREATE INDEX idx_cost_allocation_annotation_review_attempt
    ON cost_allocation(annotation_review_attempt_id, created_at)
    WHERE annotation_review_attempt_id IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace()
RETURNS trigger AS $$
DECLARE
    event_workspace uuid;
    subject_workspace uuid;
BEGIN
    SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;
    IF NEW.delivery_operation_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM delivery_operation WHERE id=NEW.delivery_operation_id;
    ELSIF NEW.quality_assessment_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM quality_result WHERE id=NEW.quality_assessment_id;
    ELSIF NEW.quality_assessment_attempt_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM quality_assessment_attempt WHERE id=NEW.quality_assessment_attempt_id;
    ELSIF NEW.rights_declaration_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM rights_declaration WHERE id=NEW.rights_declaration_id;
    ELSIF NEW.rights_declaration_verification_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace
          FROM rights_declaration_verification v
          JOIN rights_declaration d ON d.id=v.declaration_id
         WHERE v.id=NEW.rights_declaration_verification_id;
    ELSIF NEW.rights_declaration_disposition_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace
          FROM rights_declaration_disposition x
          JOIN rights_declaration d ON d.id=x.declaration_id
         WHERE x.id=NEW.rights_declaration_disposition_id;
    ELSIF NEW.authorization_provenance_binding_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM authorization_provenance_binding
         WHERE id=NEW.authorization_provenance_binding_id;
    ELSIF NEW.authorization_provenance_binding_disposition_id IS NOT NULL THEN
        SELECT b.workspace_id INTO subject_workspace
          FROM authorization_provenance_binding_disposition x
          JOIN authorization_provenance_binding b ON b.id=x.binding_id
         WHERE x.id=NEW.authorization_provenance_binding_disposition_id;
    ELSIF NEW.effective_rights_snapshot_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM effective_rights_snapshot
         WHERE id=NEW.effective_rights_snapshot_id;
    ELSIF NEW.dataset_certification_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM dataset_certification
         WHERE id=NEW.dataset_certification_id;
    ELSIF NEW.certification_disposition_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM certification_disposition
         WHERE id=NEW.certification_disposition_id;
    ELSE
        SELECT workspace_id INTO subject_workspace
          FROM annotation_review_attempt
         WHERE id=NEW.annotation_review_attempt_id;
    END IF;

    IF event_workspace IS DISTINCT FROM subject_workspace THEN
        RAISE EXCEPTION 'cost allocation crosses workspace boundary';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

    )
);

CREATE OR REPLACE FUNCTION validate_annotation_campaign_command_insert()
RETURNS trigger AS $
DECLARE
    campaign_workspace uuid;
BEGIN
    SELECT workspace_id
      INTO campaign_workspace
      FROM annotation_campaign
     WHERE id=NEW.campaign_id;

    IF campaign_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation campaign command crosses workspace boundary';
    END IF;
    RETURN NEW;
END;
$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_campaign_command_insert
BEFORE INSERT ON annotation_campaign_command
FOR EACH ROW EXECUTE FUNCTION validate_annotation_campaign_command_insert();

CREATE TABLE annotation_task (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    source_item_ref             varchar(1024) NOT NULL,
    source_content_sha256       varchar(64) NOT NULL,
    task_text_sha256            varchar(64) NOT NULL,
    primary_annotator_ref       varchar(255),
    status                      varchar(16) NOT NULL DEFAULT 'PENDING',
    revision                    bigint NOT NULL DEFAULT 1,
    current_decision_id         uuid,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_task_source UNIQUE (campaign_id, source_item_ref),
    CONSTRAINT ck_annotation_task_status CHECK (status IN ('PENDING','REVIEWABLE','REVIEWED')),
    CONSTRAINT ck_annotation_task_revision CHECK (revision >= 1),
    CONSTRAINT ck_annotation_task_source_hash CHECK (source_content_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_annotation_task_text_hash CHECK (task_text_sha256 ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_annotation_task_campaign
    ON annotation_task(campaign_id, id);
CREATE INDEX idx_annotation_task_workspace
    ON annotation_task(workspace_id, campaign_id, status, id);

CREATE TABLE annotation_result (
    id                              uuid PRIMARY KEY,
    workspace_id                    uuid NOT NULL,
    campaign_id                     uuid NOT NULL REFERENCES annotation_campaign(id),
    task_id                         uuid NOT NULL REFERENCES annotation_task(id),
    author_ref                      varchar(255) NOT NULL,
    provider_binding_ref            varchar(512),
    external_task_id                varchar(512),
    external_annotation_id          varchar(512),
    external_revision               varchar(255),
    observation_key                 varchar(1024) NOT NULL,
    canonical_payload               bytea NOT NULL,
    canonical_payload_sha256        varchar(64) NOT NULL,
    normalizer_version              varchar(64) NOT NULL,
    corrected_from_result_id        uuid REFERENCES annotation_result(id),
    created_at                      timestamptz NOT NULL DEFAULT now(),
    created_by                      uuid,
    CONSTRAINT uq_annotation_result_observation UNIQUE (workspace_id, campaign_id, observation_key),
    CONSTRAINT ck_annotation_result_payload_hash CHECK (
        canonical_payload_sha256 ~ '^[0-9a-f]{64}$'
        AND encode(digest(canonical_payload, 'sha256'), 'hex') = canonical_payload_sha256
    ),
    CONSTRAINT ck_annotation_result_correction_self CHECK (
        corrected_from_result_id IS NULL OR corrected_from_result_id <> id
    )
);

CREATE INDEX idx_annotation_result_task
    ON annotation_result(task_id, created_at, id);

-- Physical human work is durable independently from the authoritative decision.
-- A review attempt is inserted before the Decision CAS transaction. If the CAS
-- loses, the attempt remains and gets a STALE_CONFLICT outcome/cost fact.
CREATE TABLE annotation_review_attempt (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    reviewer_ref                varchar(255) NOT NULL,
    expected_task_revision      bigint NOT NULL,
    review_action               varchar(16) NOT NULL,
    reason                      text NOT NULL,
    idempotency_key             varchar(255) NOT NULL,
    request_fingerprint         varchar(64) NOT NULL,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_review_attempt_key UNIQUE (workspace_id, idempotency_key),
    CONSTRAINT ck_annotation_review_attempt_revision CHECK (expected_task_revision >= 1),
    CONSTRAINT ck_annotation_review_attempt_action CHECK (review_action IN ('ACCEPT','REJECT','CORRECT')),
    CONSTRAINT ck_annotation_review_attempt_reason CHECK (length(btrim(reason)) > 0),
    CONSTRAINT ck_annotation_review_attempt_fingerprint CHECK (request_fingerprint ~ '^[0-9a-f]{64}$')
);

CREATE INDEX idx_annotation_review_attempt_task
    ON annotation_review_attempt(task_id, created_at, id);

CREATE TABLE annotation_review_attempt_outcome (
    id                          uuid PRIMARY KEY,
    attempt_id                  uuid NOT NULL REFERENCES annotation_review_attempt(id),
    outcome                     varchar(32) NOT NULL,
    error_code                  varchar(128),
    occurred_at                 timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_review_attempt_outcome UNIQUE (attempt_id),
    CONSTRAINT ck_annotation_review_attempt_outcome CHECK (
        outcome IN ('SUCCEEDED','STALE_CONFLICT','REJECTED')
    )
);

CREATE TABLE annotation_review_decision (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    review_attempt_id           uuid NOT NULL REFERENCES annotation_review_attempt(id),
    reviewed_result_id          uuid REFERENCES annotation_result(id),
    selected_result_id          uuid REFERENCES annotation_result(id),
    reviewer_ref                varchar(255) NOT NULL,
    outcome                     varchar(16) NOT NULL,
    reason                      text NOT NULL,
    expected_task_revision      bigint NOT NULL,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_annotation_review_decision_task UNIQUE (task_id),
    CONSTRAINT uq_annotation_review_decision_attempt UNIQUE (review_attempt_id),
    CONSTRAINT ck_annotation_review_decision_outcome CHECK (outcome IN ('ACCEPT','REJECT','CORRECT')),
    CONSTRAINT ck_annotation_review_decision_reason CHECK (length(btrim(reason)) > 0),
    CONSTRAINT ck_annotation_review_decision_revision CHECK (expected_task_revision >= 1),
    CONSTRAINT ck_annotation_review_decision_selection CHECK (
        (outcome='ACCEPT' AND reviewed_result_id IS NOT NULL AND selected_result_id = reviewed_result_id)
        OR
        (outcome='CORRECT' AND reviewed_result_id IS NOT NULL AND selected_result_id IS NOT NULL AND selected_result_id <> reviewed_result_id)
        OR
        (outcome='REJECT' AND selected_result_id IS NULL)
    )
);

ALTER TABLE annotation_task
    ADD CONSTRAINT fk_annotation_task_current_decision
    FOREIGN KEY (current_decision_id) REFERENCES annotation_review_decision(id);

CREATE INDEX idx_annotation_review_decision_campaign
    ON annotation_review_decision(campaign_id, created_at, id);

CREATE TABLE annotation_snapshot (
    id                          uuid PRIMARY KEY,
    workspace_id                uuid NOT NULL,
    campaign_id                 uuid NOT NULL REFERENCES annotation_campaign(id),
    status                      varchar(16) NOT NULL DEFAULT 'BUILDING',
    manifest                    jsonb NOT NULL,
    manifest_hash_payload       bytea NOT NULL,
    root_hash                   varchar(64) NOT NULL,
    expected_task_count         integer NOT NULL,
    expected_result_count       integer NOT NULL,
    expected_decision_count     integer NOT NULL,
    expected_output_count       integer NOT NULL,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    created_by                  uuid,
    finalized_at                timestamptz,
    CONSTRAINT uq_annotation_snapshot_campaign UNIQUE (campaign_id),
    CONSTRAINT ck_annotation_snapshot_status CHECK (status IN ('BUILDING','FINALIZED')),
    CONSTRAINT ck_annotation_snapshot_root_hash CHECK (root_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT ck_annotation_snapshot_counts CHECK (
        expected_task_count > 0
        AND expected_result_count >= 0
        AND expected_decision_count = expected_task_count
        AND expected_output_count >= 0
        AND expected_output_count <= expected_task_count
    )
);

CREATE TABLE annotation_snapshot_task (
    snapshot_id                 uuid NOT NULL REFERENCES annotation_snapshot(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    source_item_ref             varchar(1024) NOT NULL,
    source_content_sha256       varchar(64) NOT NULL,
    task_text_sha256            varchar(64) NOT NULL,
    PRIMARY KEY (snapshot_id, task_id)
);

CREATE TABLE annotation_snapshot_result (
    snapshot_id                 uuid NOT NULL REFERENCES annotation_snapshot(id),
    result_id                   uuid NOT NULL REFERENCES annotation_result(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    canonical_payload_sha256    varchar(64) NOT NULL,
    author_ref                  varchar(255) NOT NULL,
    PRIMARY KEY (snapshot_id, result_id)
);

CREATE TABLE annotation_snapshot_decision (
    snapshot_id                 uuid NOT NULL REFERENCES annotation_snapshot(id),
    decision_id                 uuid NOT NULL REFERENCES annotation_review_decision(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    outcome                     varchar(16) NOT NULL,
    reviewed_result_id          uuid,
    selected_result_id          uuid,
    reviewer_ref                varchar(255) NOT NULL,
    reason                      text NOT NULL,
    PRIMARY KEY (snapshot_id, decision_id),
    CONSTRAINT uq_annotation_snapshot_decision_task UNIQUE (snapshot_id, task_id)
);

CREATE TABLE annotation_snapshot_output (
    snapshot_id                 uuid NOT NULL REFERENCES annotation_snapshot(id),
    task_id                     uuid NOT NULL REFERENCES annotation_task(id),
    selected_result_id          uuid NOT NULL REFERENCES annotation_result(id),
    PRIMARY KEY (snapshot_id, task_id),
    CONSTRAINT uq_annotation_snapshot_output_result UNIQUE (snapshot_id, selected_result_id)
);

CREATE OR REPLACE FUNCTION prevent_annotation_append_only_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only annotation history', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_result_immutable
BEFORE UPDATE OR DELETE ON annotation_result
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE TRIGGER trg_annotation_review_attempt_immutable
BEFORE UPDATE OR DELETE ON annotation_review_attempt
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE TRIGGER trg_annotation_review_attempt_outcome_immutable
BEFORE UPDATE OR DELETE ON annotation_review_attempt_outcome
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE TRIGGER trg_annotation_review_decision_immutable
BEFORE UPDATE OR DELETE ON annotation_review_decision
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE OR REPLACE FUNCTION validate_annotation_campaign_insert()
RETURNS trigger AS $$
DECLARE
    dataset_workspace uuid;
    certification_workspace uuid;
    certification_dataset_version uuid;
    certification_decision varchar(16);
    resource_workspace uuid;
BEGIN
    IF NEW.status <> 'DRAFT' OR NEW.revision <> 1 THEN
        RAISE EXCEPTION 'new annotation campaign must start DRAFT at revision 1';
    END IF;

    SELECT d.workspace_id
      INTO dataset_workspace
      FROM dataset_version v
      JOIN dataset d ON d.id=v.dataset_id
     WHERE v.id=NEW.input_dataset_version_id;

    SELECT workspace_id, dataset_version_id, decision
      INTO certification_workspace, certification_dataset_version, certification_decision
      FROM dataset_certification
     WHERE id=NEW.input_certification_id;

    SELECT workspace_id
      INTO resource_workspace
      FROM data_resource
     WHERE id=NEW.annotation_contribution_resource_id;

    IF dataset_workspace IS DISTINCT FROM NEW.workspace_id
       OR certification_workspace IS DISTINCT FROM NEW.workspace_id
       OR certification_dataset_version IS DISTINCT FROM NEW.input_dataset_version_id
       OR certification_decision <> 'CERTIFIED'
       OR resource_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation campaign input/certification/contribution resource does not match workspace';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_campaign_insert
BEFORE INSERT ON annotation_campaign
FOR EACH ROW EXECUTE FUNCTION validate_annotation_campaign_insert();

CREATE OR REPLACE FUNCTION guard_annotation_campaign_update()
RETURNS trigger AS $$
DECLARE
    task_count integer;
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.input_dataset_version_id IS DISTINCT FROM OLD.input_dataset_version_id
       OR NEW.input_certification_id IS DISTINCT FROM OLD.input_certification_id
       OR NEW.annotation_contribution_resource_id IS DISTINCT FROM OLD.annotation_contribution_resource_id
       OR NEW.purpose IS DISTINCT FROM OLD.purpose
       OR NEW.action IS DISTINCT FROM OLD.action
       OR NEW.consumer_ref IS DISTINCT FROM OLD.consumer_ref
       OR NEW.scope_type IS DISTINCT FROM OLD.scope_type
       OR NEW.scope_ref IS DISTINCT FROM OLD.scope_ref
       OR NEW.schema_ref IS DISTINCT FROM OLD.schema_ref
       OR NEW.schema_version IS DISTINCT FROM OLD.schema_version
       OR NEW.schema_content_sha256 IS DISTINCT FROM OLD.schema_content_sha256
       OR NEW.schema_content_snapshot IS DISTINCT FROM OLD.schema_content_snapshot
       OR NEW.taxonomy_ref IS DISTINCT FROM OLD.taxonomy_ref
       OR NEW.taxonomy_version IS DISTINCT FROM OLD.taxonomy_version
       OR NEW.taxonomy_content_sha256 IS DISTINCT FROM OLD.taxonomy_content_sha256
       OR NEW.taxonomy_content_snapshot IS DISTINCT FROM OLD.taxonomy_content_snapshot
       OR NEW.rubric_ref IS DISTINCT FROM OLD.rubric_ref
       OR NEW.rubric_version IS DISTINCT FROM OLD.rubric_version
       OR NEW.rubric_content_sha256 IS DISTINCT FROM OLD.rubric_content_sha256
       OR NEW.rubric_content_snapshot IS DISTINCT FROM OLD.rubric_content_snapshot
       OR NEW.renderer_ref IS DISTINCT FROM OLD.renderer_ref
       OR NEW.renderer_version IS DISTINCT FROM OLD.renderer_version
       OR NEW.renderer_content_sha256 IS DISTINCT FROM OLD.renderer_content_sha256
       OR NEW.renderer_content_snapshot IS DISTINCT FROM OLD.renderer_content_snapshot
       OR NEW.review_policy_ref IS DISTINCT FROM OLD.review_policy_ref
       OR NEW.review_policy_version IS DISTINCT FROM OLD.review_policy_version
       OR NEW.review_policy_content_sha256 IS DISTINCT FROM OLD.review_policy_content_sha256
       OR NEW.review_policy_content_snapshot IS DISTINCT FROM OLD.review_policy_content_snapshot
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.created_by IS DISTINCT FROM OLD.created_by THEN
        RAISE EXCEPTION 'annotation campaign frozen semantics are immutable';
    END IF;

    IF OLD.status='DRAFT' AND NEW.status='ACTIVE' THEN
        SELECT count(*) INTO task_count FROM annotation_task WHERE campaign_id=OLD.id;
        IF NEW.expected_task_count IS NULL OR NEW.expected_task_count <> task_count OR task_count < 1
           OR NEW.task_manifest_hash IS NULL OR NEW.activated_at IS NULL
           OR NEW.revision <> OLD.revision + 1 THEN
            RAISE EXCEPTION 'annotation campaign activation requires exact non-empty task manifest and revision CAS';
        END IF;
        RETURN NEW;
    END IF;

    IF OLD.status='DRAFT' AND NEW.status='CANCELLED' THEN
        IF NEW.cancelled_at IS NULL OR NEW.revision <> OLD.revision + 1 THEN
            RAISE EXCEPTION 'annotation campaign cancellation requires timestamp and revision CAS';
        END IF;
        RETURN NEW;
    END IF;

    IF OLD.status='ACTIVE' AND NEW.status='CANCELLED' THEN
        IF NEW.cancelled_at IS NULL OR NEW.revision <> OLD.revision + 1 THEN
            RAISE EXCEPTION 'annotation campaign cancellation requires timestamp and revision CAS';
        END IF;
        RETURN NEW;
    END IF;

    IF OLD.status='ACTIVE' AND NEW.status='SEALED' THEN
        IF NEW.sealed_at IS NULL OR NEW.revision <> OLD.revision + 1 THEN
            RAISE EXCEPTION 'annotation campaign seal requires timestamp and revision CAS';
        END IF;
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'invalid annotation campaign transition % -> %', OLD.status, NEW.status;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_campaign_update
BEFORE UPDATE ON annotation_campaign
FOR EACH ROW EXECUTE FUNCTION guard_annotation_campaign_update();

CREATE TRIGGER trg_annotation_campaign_delete
BEFORE DELETE ON annotation_campaign
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE OR REPLACE FUNCTION guard_annotation_task_insert()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    campaign_workspace uuid;
BEGIN
    SELECT status, workspace_id
      INTO campaign_status, campaign_workspace
      FROM annotation_campaign
     WHERE id=NEW.campaign_id
     FOR UPDATE;

    IF campaign_status <> 'DRAFT' OR campaign_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation task requires DRAFT campaign in same workspace';
    END IF;
    IF NEW.status <> 'PENDING' OR NEW.revision <> 1 OR NEW.current_decision_id IS NOT NULL THEN
        RAISE EXCEPTION 'new annotation task must start PENDING at revision 1';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_task_insert
BEFORE INSERT ON annotation_task
FOR EACH ROW EXECUTE FUNCTION guard_annotation_task_insert();

CREATE OR REPLACE FUNCTION guard_annotation_task_update()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
BEGIN
    SELECT status
      INTO campaign_status
      FROM annotation_campaign
     WHERE id=OLD.campaign_id
     FOR UPDATE;
    IF campaign_status <> 'ACTIVE' THEN
        RAISE EXCEPTION 'annotation task is immutable outside ACTIVE campaign';
    END IF;

    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.campaign_id IS DISTINCT FROM OLD.campaign_id
       OR NEW.source_item_ref IS DISTINCT FROM OLD.source_item_ref
       OR NEW.source_content_sha256 IS DISTINCT FROM OLD.source_content_sha256
       OR NEW.task_text_sha256 IS DISTINCT FROM OLD.task_text_sha256
       OR NEW.primary_annotator_ref IS DISTINCT FROM OLD.primary_annotator_ref
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'annotation task identity/content is immutable';
    END IF;
    IF NEW.revision <> OLD.revision + 1 THEN
        RAISE EXCEPTION 'annotation task update requires revision CAS';
    END IF;
    IF OLD.status='PENDING' AND NEW.status='REVIEWABLE' AND NEW.current_decision_id IS NULL THEN
        RETURN NEW;
    END IF;
    IF OLD.status='REVIEWABLE' AND NEW.status='REVIEWABLE' AND NEW.current_decision_id IS NULL THEN
        RETURN NEW;
    END IF;
    IF OLD.status IN ('PENDING','REVIEWABLE')
       AND NEW.status='REVIEWED'
       AND NEW.current_decision_id IS NOT NULL THEN
        RETURN NEW;
    END IF;
    RAISE EXCEPTION 'invalid annotation task transition % -> %', OLD.status, NEW.status;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_task_update
BEFORE UPDATE ON annotation_task
FOR EACH ROW EXECUTE FUNCTION guard_annotation_task_update();

CREATE TRIGGER trg_annotation_task_delete
BEFORE DELETE ON annotation_task
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_append_only_mutation();

CREATE OR REPLACE FUNCTION guard_annotation_result_insert()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    task_workspace uuid;
    task_campaign uuid;
    task_status varchar(16);
    corrected_task uuid;
BEGIN
    SELECT status INTO campaign_status
      FROM annotation_campaign
     WHERE id=NEW.campaign_id
     FOR UPDATE;
    IF campaign_status <> 'ACTIVE' THEN
        RAISE EXCEPTION 'annotation result requires ACTIVE campaign';
    END IF;

    SELECT workspace_id, campaign_id, status
      INTO task_workspace, task_campaign, task_status
      FROM annotation_task
     WHERE id=NEW.task_id
     FOR UPDATE;

    IF task_workspace IS DISTINCT FROM NEW.workspace_id
       OR task_campaign IS DISTINCT FROM NEW.campaign_id
       OR task_status='REVIEWED' THEN
        RAISE EXCEPTION 'annotation result does not match active task';
    END IF;

    IF NEW.corrected_from_result_id IS NOT NULL THEN
        SELECT task_id INTO corrected_task FROM annotation_result WHERE id=NEW.corrected_from_result_id;
        IF corrected_task IS DISTINCT FROM NEW.task_id THEN
            RAISE EXCEPTION 'annotation correction must reference a result from the same task';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_result_insert
BEFORE INSERT ON annotation_result
FOR EACH ROW EXECUTE FUNCTION guard_annotation_result_insert();

CREATE OR REPLACE FUNCTION validate_annotation_review_attempt_insert()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    task_workspace uuid;
    task_campaign uuid;
    annotator_ref varchar(255);
BEGIN
    SELECT status INTO campaign_status
      FROM annotation_campaign
     WHERE id=NEW.campaign_id
     FOR UPDATE;
    SELECT workspace_id, campaign_id, primary_annotator_ref
      INTO task_workspace, task_campaign, annotator_ref
      FROM annotation_task
     WHERE id=NEW.task_id;

    IF campaign_status <> 'ACTIVE'
       OR task_workspace IS DISTINCT FROM NEW.workspace_id
       OR task_campaign IS DISTINCT FROM NEW.campaign_id THEN
        RAISE EXCEPTION 'annotation review attempt does not match active task';
    END IF;
    IF annotator_ref IS NOT NULL AND annotator_ref = NEW.reviewer_ref THEN
        RAISE EXCEPTION 'annotation reviewer must differ from primary annotator';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_review_attempt_insert
BEFORE INSERT ON annotation_review_attempt
FOR EACH ROW EXECUTE FUNCTION validate_annotation_review_attempt_insert();

CREATE OR REPLACE FUNCTION validate_annotation_review_attempt_outcome_insert()
RETURNS trigger AS $$
BEGIN
    IF NEW.outcome='SUCCEEDED' AND NOT EXISTS (
        SELECT 1 FROM annotation_review_decision d WHERE d.review_attempt_id=NEW.attempt_id
    ) THEN
        RAISE EXCEPTION 'successful annotation review attempt requires authoritative decision';
    END IF;
    IF NEW.outcome<>'SUCCEEDED' AND EXISTS (
        SELECT 1 FROM annotation_review_decision d WHERE d.review_attempt_id=NEW.attempt_id
    ) THEN
        RAISE EXCEPTION 'non-success annotation review attempt cannot own authoritative decision';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_review_attempt_outcome_insert
BEFORE INSERT ON annotation_review_attempt_outcome
FOR EACH ROW EXECUTE FUNCTION validate_annotation_review_attempt_outcome_insert();

CREATE OR REPLACE FUNCTION validate_annotation_review_decision_insert()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    task_row annotation_task%ROWTYPE;
    attempt_row annotation_review_attempt%ROWTYPE;
    reviewed_task uuid;
    selected_task uuid;
    selected_corrected_from uuid;
BEGIN
    SELECT status INTO campaign_status
      FROM annotation_campaign
     WHERE id=NEW.campaign_id
     FOR UPDATE;
    IF campaign_status <> 'ACTIVE' THEN
        RAISE EXCEPTION 'annotation review requires ACTIVE campaign';
    END IF;

    SELECT * INTO task_row
      FROM annotation_task
     WHERE id=NEW.task_id
     FOR UPDATE;

    SELECT * INTO attempt_row
      FROM annotation_review_attempt
     WHERE id=NEW.review_attempt_id;

    IF task_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR task_row.campaign_id IS DISTINCT FROM NEW.campaign_id
       OR task_row.current_decision_id IS NOT NULL
       OR task_row.status='REVIEWED'
       OR task_row.revision IS DISTINCT FROM NEW.expected_task_revision
       OR attempt_row.workspace_id IS DISTINCT FROM NEW.workspace_id
       OR attempt_row.campaign_id IS DISTINCT FROM NEW.campaign_id
       OR attempt_row.task_id IS DISTINCT FROM NEW.task_id
       OR attempt_row.reviewer_ref IS DISTINCT FROM NEW.reviewer_ref
       OR attempt_row.expected_task_revision IS DISTINCT FROM NEW.expected_task_revision
       OR attempt_row.review_action IS DISTINCT FROM NEW.outcome
       OR attempt_row.reason IS DISTINCT FROM NEW.reason THEN
        RAISE EXCEPTION 'annotation review decision failed expected task/attempt CAS';
    END IF;

    IF task_row.primary_annotator_ref IS NOT NULL
       AND task_row.primary_annotator_ref = NEW.reviewer_ref THEN
        RAISE EXCEPTION 'annotation reviewer must differ from primary annotator';
    END IF;

    IF NEW.reviewed_result_id IS NOT NULL THEN
        SELECT task_id INTO reviewed_task FROM annotation_result WHERE id=NEW.reviewed_result_id;
        IF reviewed_task IS DISTINCT FROM NEW.task_id THEN
            RAISE EXCEPTION 'reviewed annotation result does not belong to task';
        END IF;
    END IF;

    IF NEW.selected_result_id IS NOT NULL THEN
        SELECT task_id, corrected_from_result_id
          INTO selected_task, selected_corrected_from
          FROM annotation_result
         WHERE id=NEW.selected_result_id;
        IF selected_task IS DISTINCT FROM NEW.task_id THEN
            RAISE EXCEPTION 'selected annotation result does not belong to task';
        END IF;
    END IF;

    IF NEW.outcome='CORRECT' AND selected_corrected_from IS DISTINCT FROM NEW.reviewed_result_id THEN
        RAISE EXCEPTION 'corrected annotation result must point to reviewed result';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_review_decision_insert
BEFORE INSERT ON annotation_review_decision
FOR EACH ROW EXECUTE FUNCTION validate_annotation_review_decision_insert();

CREATE OR REPLACE FUNCTION project_annotation_review_decision()
RETURNS trigger AS $$
BEGIN
    UPDATE annotation_task
       SET status='REVIEWED',
           current_decision_id=NEW.id,
           revision=revision+1
     WHERE id=NEW.task_id
       AND revision=NEW.expected_task_revision
       AND current_decision_id IS NULL
       AND status IN ('PENDING','REVIEWABLE');

    IF NOT FOUND THEN
        RAISE EXCEPTION 'annotation review decision lost task CAS';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_review_decision_projection
AFTER INSERT ON annotation_review_decision
FOR EACH ROW EXECUTE FUNCTION project_annotation_review_decision();

CREATE OR REPLACE FUNCTION validate_annotation_snapshot_insert()
RETURNS trigger AS $$
DECLARE
    campaign_workspace uuid;
BEGIN
    IF NEW.status <> 'BUILDING' OR NEW.finalized_at IS NOT NULL THEN
        RAISE EXCEPTION 'new annotation snapshot must start BUILDING';
    END IF;
    SELECT workspace_id INTO campaign_workspace FROM annotation_campaign WHERE id=NEW.campaign_id;
    IF campaign_workspace IS DISTINCT FROM NEW.workspace_id THEN
        RAISE EXCEPTION 'annotation snapshot crosses workspace boundary';
    END IF;
    IF convert_from(NEW.manifest_hash_payload, 'UTF8')::jsonb IS DISTINCT FROM NEW.manifest THEN
        RAISE EXCEPTION 'annotation snapshot hash payload does not match manifest';
    END IF;
    IF encode(digest(NEW.manifest_hash_payload, 'sha256'), 'hex') IS DISTINCT FROM NEW.root_hash THEN
        RAISE EXCEPTION 'annotation snapshot root hash does not match manifest payload';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_snapshot_insert
BEFORE INSERT ON annotation_snapshot
FOR EACH ROW EXECUTE FUNCTION validate_annotation_snapshot_insert();

CREATE OR REPLACE FUNCTION guard_annotation_snapshot_membership()
RETURNS trigger AS $$
DECLARE
    target_snapshot uuid;
    parent_status varchar(16);
BEGIN
    IF TG_OP <> 'INSERT' THEN
        RAISE EXCEPTION 'annotation snapshot membership is immutable';
    END IF;

    target_snapshot := NEW.snapshot_id;
    SELECT status INTO parent_status
      FROM annotation_snapshot
     WHERE id=target_snapshot
     FOR UPDATE;

    IF parent_status <> 'BUILDING' THEN
        RAISE EXCEPTION 'annotation snapshot membership is finalized';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_snapshot_task_membership
BEFORE INSERT OR UPDATE OR DELETE ON annotation_snapshot_task
FOR EACH ROW EXECUTE FUNCTION guard_annotation_snapshot_membership();
CREATE TRIGGER trg_annotation_snapshot_result_membership
BEFORE INSERT OR UPDATE OR DELETE ON annotation_snapshot_result
FOR EACH ROW EXECUTE FUNCTION guard_annotation_snapshot_membership();
CREATE TRIGGER trg_annotation_snapshot_decision_membership
BEFORE INSERT OR UPDATE OR DELETE ON annotation_snapshot_decision
FOR EACH ROW EXECUTE FUNCTION guard_annotation_snapshot_membership();
CREATE TRIGGER trg_annotation_snapshot_output_membership
BEFORE INSERT OR UPDATE OR DELETE ON annotation_snapshot_output
FOR EACH ROW EXECUTE FUNCTION guard_annotation_snapshot_membership();

CREATE OR REPLACE FUNCTION prevent_annotation_snapshot_mutation()
RETURNS trigger AS $$
DECLARE
    campaign_status varchar(16);
    campaign_workspace uuid;
    campaign_expected integer;
    campaign_input_version uuid;
    campaign_input_certification uuid;
    campaign_contribution_resource uuid;
    campaign_task_manifest_hash varchar(64);
    campaign_schema_hash varchar(64);
    campaign_taxonomy_hash varchar(64);
    campaign_rubric_hash varchar(64);
    campaign_renderer_hash varchar(64);
    campaign_review_policy_hash varchar(64);
    task_count integer;
    result_count integer;
    decision_count integer;
    output_count integer;
    persisted_tasks jsonb;
    persisted_results jsonb;
    persisted_decisions jsonb;
    persisted_outputs jsonb;
BEGIN
    IF TG_OP='DELETE' THEN
        RAISE EXCEPTION 'annotation snapshot is immutable';
    END IF;

    IF OLD.status <> 'BUILDING' OR NEW.status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'annotation snapshot is immutable';
    END IF;

    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.workspace_id IS DISTINCT FROM OLD.workspace_id
       OR NEW.campaign_id IS DISTINCT FROM OLD.campaign_id
       OR NEW.manifest IS DISTINCT FROM OLD.manifest
       OR NEW.manifest_hash_payload IS DISTINCT FROM OLD.manifest_hash_payload
       OR NEW.root_hash IS DISTINCT FROM OLD.root_hash
       OR NEW.expected_task_count IS DISTINCT FROM OLD.expected_task_count
       OR NEW.expected_result_count IS DISTINCT FROM OLD.expected_result_count
       OR NEW.expected_decision_count IS DISTINCT FROM OLD.expected_decision_count
       OR NEW.expected_output_count IS DISTINCT FROM OLD.expected_output_count
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.created_by IS DISTINCT FROM OLD.created_by
       OR NEW.finalized_at IS NULL THEN
        RAISE EXCEPTION 'annotation snapshot content is immutable';
    END IF;

    SELECT status, workspace_id, expected_task_count,
           input_dataset_version_id, input_certification_id, annotation_contribution_resource_id,
           task_manifest_hash, schema_content_sha256, taxonomy_content_sha256,
           rubric_content_sha256, renderer_content_sha256, review_policy_content_sha256
      INTO campaign_status, campaign_workspace, campaign_expected,
           campaign_input_version, campaign_input_certification, campaign_contribution_resource,
           campaign_task_manifest_hash, campaign_schema_hash, campaign_taxonomy_hash,
           campaign_rubric_hash, campaign_renderer_hash, campaign_review_policy_hash
      FROM annotation_campaign
     WHERE id=OLD.campaign_id
     FOR UPDATE;

    IF campaign_status <> 'ACTIVE'
       OR campaign_workspace IS DISTINCT FROM OLD.workspace_id
       OR campaign_expected IS DISTINCT FROM OLD.expected_task_count THEN
        RAISE EXCEPTION 'annotation snapshot campaign is not sealable';
    END IF;

    IF NEW.manifest->>'formatVersion' IS DISTINCT FROM 'annotation-snapshot-v1'
       OR NEW.manifest->>'campaignId' IS DISTINCT FROM OLD.campaign_id::text
       OR NEW.manifest->>'inputDatasetVersionId' IS DISTINCT FROM campaign_input_version::text
       OR NEW.manifest->>'inputCertificationId' IS DISTINCT FROM campaign_input_certification::text
       OR NEW.manifest->>'annotationContributionResourceId' IS DISTINCT FROM campaign_contribution_resource::text
       OR NEW.manifest->>'taskManifestHash' IS DISTINCT FROM campaign_task_manifest_hash
       OR NEW.manifest->>'schemaHash' IS DISTINCT FROM campaign_schema_hash
       OR NEW.manifest->>'taxonomyHash' IS DISTINCT FROM campaign_taxonomy_hash
       OR NEW.manifest->>'rubricHash' IS DISTINCT FROM campaign_rubric_hash
       OR NEW.manifest->>'rendererHash' IS DISTINCT FROM campaign_renderer_hash
       OR NEW.manifest->>'reviewPolicyHash' IS DISTINCT FROM campaign_review_policy_hash THEN
        RAISE EXCEPTION 'annotation snapshot manifest header does not match frozen campaign facts';
    END IF;

    SELECT count(*) INTO task_count FROM annotation_snapshot_task WHERE snapshot_id=OLD.id;
    SELECT count(*) INTO result_count FROM annotation_snapshot_result WHERE snapshot_id=OLD.id;
    SELECT count(*) INTO decision_count FROM annotation_snapshot_decision WHERE snapshot_id=OLD.id;
    SELECT count(*) INTO output_count FROM annotation_snapshot_output WHERE snapshot_id=OLD.id;

    IF task_count <> OLD.expected_task_count
       OR result_count <> OLD.expected_result_count
       OR decision_count <> OLD.expected_decision_count
       OR output_count <> OLD.expected_output_count
       OR decision_count <> task_count THEN
        RAISE EXCEPTION 'annotation snapshot persisted membership does not match expected counts';
    END IF;

    IF jsonb_typeof(NEW.manifest->'tasks') IS DISTINCT FROM 'array'
       OR jsonb_typeof(NEW.manifest->'results') IS DISTINCT FROM 'array'
       OR jsonb_typeof(NEW.manifest->'decisions') IS DISTINCT FROM 'array'
       OR jsonb_typeof(NEW.manifest->'outputs') IS DISTINCT FROM 'array' THEN
        RAISE EXCEPTION 'annotation snapshot manifest must contain membership arrays';
    END IF;

    SELECT COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'id', st.task_id::text,
                'sourceItemRef', st.source_item_ref,
                'sourceContentSha256', st.source_content_sha256,
                'taskTextSha256', st.task_text_sha256
            ) ORDER BY st.task_id::text
        ),
        '[]'::jsonb
    ) INTO persisted_tasks
    FROM annotation_snapshot_task st
    WHERE st.snapshot_id=OLD.id;

    SELECT COALESCE(
        jsonb_agg(
            jsonb_strip_nulls(jsonb_build_object(
                'id', sr.result_id::text,
                'taskId', sr.task_id::text,
                'canonicalPayloadSha256', sr.canonical_payload_sha256,
                'authorRef', sr.author_ref,
                'correctedFromResultId', r.corrected_from_result_id::text
            )) ORDER BY sr.result_id::text
        ),
        '[]'::jsonb
    ) INTO persisted_results
    FROM annotation_snapshot_result sr
    JOIN annotation_result r ON r.id=sr.result_id
    WHERE sr.snapshot_id=OLD.id;

    SELECT COALESCE(
        jsonb_agg(
            jsonb_strip_nulls(jsonb_build_object(
                'id', sd.decision_id::text,
                'taskId', sd.task_id::text,
                'outcome', sd.outcome,
                'reviewedResultId', sd.reviewed_result_id::text,
                'selectedResultId', sd.selected_result_id::text,
                'reviewerRef', sd.reviewer_ref,
                'reason', sd.reason
            )) ORDER BY sd.decision_id::text
        ),
        '[]'::jsonb
    ) INTO persisted_decisions
    FROM annotation_snapshot_decision sd
    WHERE sd.snapshot_id=OLD.id;

    SELECT COALESCE(
        jsonb_agg(
            jsonb_build_object(
                'taskId', so.task_id::text,
                'selectedResultId', so.selected_result_id::text
            ) ORDER BY so.task_id::text
        ),
        '[]'::jsonb
    ) INTO persisted_outputs
    FROM annotation_snapshot_output so
    WHERE so.snapshot_id=OLD.id;

    IF NEW.manifest->'tasks' IS DISTINCT FROM persisted_tasks
       OR NEW.manifest->'results' IS DISTINCT FROM persisted_results
       OR NEW.manifest->'decisions' IS DISTINCT FROM persisted_decisions
       OR NEW.manifest->'outputs' IS DISTINCT FROM persisted_outputs THEN
        RAISE EXCEPTION 'annotation snapshot manifest membership does not match persisted membership';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_task t
         WHERE t.campaign_id=OLD.campaign_id
           AND NOT EXISTS (
               SELECT 1 FROM annotation_snapshot_task st
                WHERE st.snapshot_id=OLD.id AND st.task_id=t.id
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_snapshot_task st
          JOIN annotation_task t ON t.id=st.task_id
         WHERE st.snapshot_id=OLD.id
           AND t.campaign_id<>OLD.campaign_id
    ) THEN
        RAISE EXCEPTION 'annotation snapshot task membership is not exact campaign task set';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_result r
         WHERE r.campaign_id=OLD.campaign_id
           AND NOT EXISTS (
               SELECT 1 FROM annotation_snapshot_result sr
                WHERE sr.snapshot_id=OLD.id AND sr.result_id=r.id
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_review_decision d
         WHERE d.campaign_id=OLD.campaign_id
           AND NOT EXISTS (
               SELECT 1 FROM annotation_snapshot_decision sd
                WHERE sd.snapshot_id=OLD.id AND sd.decision_id=d.id
           )
    ) THEN
        RAISE EXCEPTION 'annotation snapshot result/decision membership is incomplete';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_snapshot_task st
          JOIN annotation_task t ON t.id=st.task_id
         WHERE st.snapshot_id=OLD.id
           AND (
               st.source_item_ref IS DISTINCT FROM t.source_item_ref
               OR st.source_content_sha256 IS DISTINCT FROM t.source_content_sha256
               OR st.task_text_sha256 IS DISTINCT FROM t.task_text_sha256
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_snapshot_result sr
          JOIN annotation_result r ON r.id=sr.result_id
         WHERE sr.snapshot_id=OLD.id
           AND (
               sr.task_id IS DISTINCT FROM r.task_id
               OR sr.canonical_payload_sha256 IS DISTINCT FROM r.canonical_payload_sha256
               OR sr.author_ref IS DISTINCT FROM r.author_ref
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_snapshot_decision sd
          JOIN annotation_review_decision d ON d.id=sd.decision_id
         WHERE sd.snapshot_id=OLD.id
           AND (
               sd.task_id IS DISTINCT FROM d.task_id
               OR sd.outcome IS DISTINCT FROM d.outcome
               OR sd.reviewed_result_id IS DISTINCT FROM d.reviewed_result_id
               OR sd.selected_result_id IS DISTINCT FROM d.selected_result_id
               OR sd.reviewer_ref IS DISTINCT FROM d.reviewer_ref
               OR sd.reason IS DISTINCT FROM d.reason
           )
    ) THEN
        RAISE EXCEPTION 'annotation snapshot frozen membership does not match source facts';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_task t
         WHERE t.campaign_id=OLD.campaign_id
           AND (t.status<>'REVIEWED' OR t.current_decision_id IS NULL)
    ) THEN
        RAISE EXCEPTION 'annotation snapshot requires every task terminally reviewed';
    END IF;

    IF EXISTS (
        SELECT 1
          FROM annotation_snapshot_output o
          JOIN annotation_review_decision d
            ON d.task_id=o.task_id
         WHERE o.snapshot_id=OLD.id
           AND (d.outcome NOT IN ('ACCEPT','CORRECT') OR d.selected_result_id IS DISTINCT FROM o.selected_result_id)
    ) OR EXISTS (
        SELECT 1
          FROM annotation_review_decision d
         WHERE d.campaign_id=OLD.campaign_id
           AND d.outcome IN ('ACCEPT','CORRECT')
           AND NOT EXISTS (
               SELECT 1 FROM annotation_snapshot_output o
                WHERE o.snapshot_id=OLD.id
                  AND o.task_id=d.task_id
                  AND o.selected_result_id=d.selected_result_id
           )
    ) OR EXISTS (
        SELECT 1
          FROM annotation_review_decision d
         WHERE d.campaign_id=OLD.campaign_id
           AND d.outcome='REJECT'
           AND EXISTS (
               SELECT 1 FROM annotation_snapshot_output o
                WHERE o.snapshot_id=OLD.id AND o.task_id=d.task_id
           )
    ) THEN
        RAISE EXCEPTION 'annotation snapshot output membership does not match authoritative decisions';
    END IF;

    UPDATE annotation_campaign
       SET status='SEALED', sealed_at=NEW.finalized_at, revision=revision+1
     WHERE id=OLD.campaign_id
       AND status='ACTIVE';

    IF NOT FOUND THEN
        RAISE EXCEPTION 'annotation campaign seal lost CAS';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_annotation_snapshot_immutable
BEFORE UPDATE OR DELETE ON annotation_snapshot
FOR EACH ROW EXECUTE FUNCTION prevent_annotation_snapshot_mutation();

CREATE OR REPLACE FUNCTION require_annotation_snapshot_finalized_on_commit()
RETURNS trigger AS $$
DECLARE
    current_status varchar(16);
BEGIN
    SELECT status INTO current_status FROM annotation_snapshot WHERE id=NEW.id;
    IF current_status <> 'FINALIZED' THEN
        RAISE EXCEPTION 'annotation_snapshot % must be FINALIZED before commit', NEW.id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE CONSTRAINT TRIGGER trg_annotation_snapshot_finalized_on_commit
AFTER INSERT ON annotation_snapshot
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW EXECUTE FUNCTION require_annotation_snapshot_finalized_on_commit();

-- Review attempts are typed physical cost subjects so stale/failed human work
-- remains auditable even when no authoritative ReviewDecision commits.
ALTER TABLE cost_allocation
    ADD COLUMN annotation_review_attempt_id uuid REFERENCES annotation_review_attempt(id);

ALTER TABLE cost_allocation DROP CONSTRAINT ck_cost_allocation_single_subject;
ALTER TABLE cost_allocation ADD CONSTRAINT ck_cost_allocation_single_subject CHECK (
    (CASE WHEN delivery_operation_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN quality_assessment_attempt_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_verification_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN rights_declaration_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN authorization_provenance_binding_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN authorization_provenance_binding_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN effective_rights_snapshot_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN dataset_certification_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN certification_disposition_id IS NOT NULL THEN 1 ELSE 0 END) +
    (CASE WHEN annotation_review_attempt_id IS NOT NULL THEN 1 ELSE 0 END) = 1
);

CREATE INDEX idx_cost_allocation_annotation_review_attempt
    ON cost_allocation(annotation_review_attempt_id, created_at)
    WHERE annotation_review_attempt_id IS NOT NULL;

CREATE OR REPLACE FUNCTION guard_cost_allocation_workspace()
RETURNS trigger AS $$
DECLARE
    event_workspace uuid;
    subject_workspace uuid;
BEGIN
    SELECT workspace_id INTO event_workspace FROM cost_event WHERE id=NEW.cost_event_id;
    IF NEW.delivery_operation_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM delivery_operation WHERE id=NEW.delivery_operation_id;
    ELSIF NEW.quality_assessment_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM quality_result WHERE id=NEW.quality_assessment_id;
    ELSIF NEW.quality_assessment_attempt_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM quality_assessment_attempt WHERE id=NEW.quality_assessment_attempt_id;
    ELSIF NEW.rights_declaration_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace FROM rights_declaration WHERE id=NEW.rights_declaration_id;
    ELSIF NEW.rights_declaration_verification_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace
          FROM rights_declaration_verification v
          JOIN rights_declaration d ON d.id=v.declaration_id
         WHERE v.id=NEW.rights_declaration_verification_id;
    ELSIF NEW.rights_declaration_disposition_id IS NOT NULL THEN
        SELECT d.workspace_id INTO subject_workspace
          FROM rights_declaration_disposition x
          JOIN rights_declaration d ON d.id=x.declaration_id
         WHERE x.id=NEW.rights_declaration_disposition_id;
    ELSIF NEW.authorization_provenance_binding_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM authorization_provenance_binding
         WHERE id=NEW.authorization_provenance_binding_id;
    ELSIF NEW.authorization_provenance_binding_disposition_id IS NOT NULL THEN
        SELECT b.workspace_id INTO subject_workspace
          FROM authorization_provenance_binding_disposition x
          JOIN authorization_provenance_binding b ON b.id=x.binding_id
         WHERE x.id=NEW.authorization_provenance_binding_disposition_id;
    ELSIF NEW.effective_rights_snapshot_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM effective_rights_snapshot
         WHERE id=NEW.effective_rights_snapshot_id;
    ELSIF NEW.dataset_certification_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM dataset_certification
         WHERE id=NEW.dataset_certification_id;
    ELSIF NEW.certification_disposition_id IS NOT NULL THEN
        SELECT workspace_id INTO subject_workspace
          FROM certification_disposition
         WHERE id=NEW.certification_disposition_id;
    ELSE
        SELECT workspace_id INTO subject_workspace
          FROM annotation_review_attempt
         WHERE id=NEW.annotation_review_attempt_id;
    END IF;

    IF event_workspace IS DISTINCT FROM subject_workspace THEN
        RAISE EXCEPTION 'cost allocation crosses workspace boundary';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
