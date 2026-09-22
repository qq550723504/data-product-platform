CREATE TABLE evidence (
    id              uuid PRIMARY KEY,
    workspace_id    uuid NOT NULL,
    evidence_type   varchar(64) NOT NULL,
    title           varchar(255),
    source_type     varchar(64),
    source_id       uuid,
    storage_uri     text,
    hash_algorithm  varchar(32) NOT NULL,
    hash_value      varchar(256) NOT NULL,
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    created_by      uuid,
    CONSTRAINT ck_evidence_hash_algorithm CHECK (hash_algorithm = 'SHA256-EVIDENCE-V2')
);

CREATE INDEX idx_evidence_workspace_type ON evidence(workspace_id, evidence_type);
CREATE INDEX idx_evidence_source ON evidence(source_type, source_id);

CREATE TABLE evidence_relation (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    evidence_id     uuid NOT NULL REFERENCES evidence(id),
    object_type     varchar(64) NOT NULL,
    object_id       uuid NOT NULL,
    relation_type   varchar(64) NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_evidence_relation UNIQUE(evidence_id, object_type, object_id, relation_type)
);

CREATE INDEX idx_evidence_relation_object ON evidence_relation(object_type, object_id);

CREATE TABLE entity_type (
    id                    uuid PRIMARY KEY,
    workspace_id          uuid NOT NULL,
    code                  varchar(64) NOT NULL,
    name                  varchar(128) NOT NULL,
    key_schema            jsonb NOT NULL DEFAULT '{}'::jsonb,
    attribute_schema      jsonb NOT NULL DEFAULT '{}'::jsonb,
    matching_policy_ref   varchar(512),
    matching_policy_version varchar(64),
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_entity_type_workspace_code UNIQUE(workspace_id, code)
);

CREATE TABLE entity (
    id                  uuid PRIMARY KEY,
    workspace_id        uuid NOT NULL,
    entity_type_id      uuid NOT NULL REFERENCES entity_type(id),
    canonical_key       varchar(255),
    canonical_name      varchar(512) NOT NULL,
    attributes          jsonb NOT NULL DEFAULT '{}'::jsonb,
    status              varchar(32) NOT NULL DEFAULT 'ACTIVE',
    created_at          timestamptz NOT NULL DEFAULT now(),
    created_by          uuid,
    updated_at          timestamptz NOT NULL DEFAULT now(),
    updated_by          uuid,
    retired_at          timestamptz,
    CONSTRAINT ck_entity_status CHECK(status IN ('ACTIVE', 'MERGED', 'RETIRED'))
);

CREATE UNIQUE INDEX uq_entity_type_canonical_key
    ON entity(entity_type_id, canonical_key)
    WHERE canonical_key IS NOT NULL AND status = 'ACTIVE';
CREATE INDEX idx_entity_type_name ON entity(entity_type_id, canonical_name) WHERE status = 'ACTIVE';
CREATE INDEX idx_entity_legal_representative ON entity((attributes->>'legal_representative')) WHERE status = 'ACTIVE';

CREATE TABLE entity_mapping (
    id                    uuid PRIMARY KEY,
    entity_id             uuid NOT NULL REFERENCES entity(id),
    source_type           varchar(64) NOT NULL,
    source_ref            varchar(512) NOT NULL,
    source_key            varchar(512) NOT NULL,
    source_name           varchar(512),
    match_method          varchar(64) NOT NULL,
    match_rule_id         varchar(128),
    match_policy_version  varchar(64) NOT NULL,
    confidence            numeric(6,5),
    status                varchar(32) NOT NULL,
    reviewed_by           uuid,
    reviewed_at           timestamptz,
    reviewer_reason       text,
    evidence_id           uuid REFERENCES evidence(id),
    created_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_entity_mapping_source UNIQUE(source_type, source_ref, source_key),
    CONSTRAINT ck_entity_mapping_status CHECK(status IN ('AUTO_MATCHED', 'CONFIRMED', 'REJECTED', 'CONFLICT'))
);

CREATE INDEX idx_entity_mapping_entity ON entity_mapping(entity_id);

CREATE TABLE entity_match_job (
    id                        uuid PRIMARY KEY,
    workspace_id              uuid NOT NULL,
    entity_type_id            uuid NOT NULL REFERENCES entity_type(id),
    input_dataset_version_id  uuid NOT NULL REFERENCES dataset_version(id),
    output_dataset_id         uuid NOT NULL REFERENCES dataset(id),
    source_type               varchar(64) NOT NULL,
    source_ref                varchar(512) NOT NULL,
    source_role               varchar(32) NOT NULL DEFAULT 'REFERENCE',
    policy_ref                varchar(512) NOT NULL,
    policy_version            varchar(64) NOT NULL,
    status                    varchar(32) NOT NULL DEFAULT 'QUEUED',
    auto_match_count          bigint NOT NULL DEFAULT 0,
    review_count              bigint NOT NULL DEFAULT 0,
    unresolved_count          bigint NOT NULL DEFAULT 0,
    rejected_count            bigint NOT NULL DEFAULT 0,
    output_dataset_version_id uuid REFERENCES dataset_version(id),
    error_message             text,
    created_at                timestamptz NOT NULL DEFAULT now(),
    created_by                uuid,
    started_at                timestamptz,
    finished_at               timestamptz,
    CONSTRAINT ck_entity_match_job_role CHECK(source_role IN ('ANCHOR', 'REFERENCE')),
    CONSTRAINT ck_entity_match_job_status CHECK(status IN ('QUEUED', 'RUNNING', 'WAITING_REVIEW', 'SUCCEEDED', 'FAILED'))
);

CREATE INDEX idx_entity_match_job_workspace_status ON entity_match_job(workspace_id, status);
CREATE INDEX idx_entity_match_job_input ON entity_match_job(input_dataset_version_id);

CREATE TABLE entity_match_candidate (
    id                    uuid PRIMARY KEY,
    job_id                uuid NOT NULL REFERENCES entity_match_job(id) ON DELETE CASCADE,
    source_key            varchar(512) NOT NULL,
    source_name           varchar(512),
    source_payload        jsonb NOT NULL,
    normalized_payload    jsonb NOT NULL,
    candidate_entity_id   uuid REFERENCES entity(id),
    decision              varchar(32) NOT NULL,
    status                varchar(32) NOT NULL,
    match_method          varchar(64) NOT NULL,
    match_rule_id         varchar(128),
    confidence            numeric(6,5),
    reviewed_by           uuid,
    reviewed_at           timestamptz,
    reviewer_reason       text,
    evidence_id           uuid REFERENCES evidence(id),
    created_at            timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT uq_entity_match_candidate_source UNIQUE(job_id, source_key),
    CONSTRAINT ck_entity_match_candidate_decision CHECK(decision IN ('AUTO_MATCH', 'REVIEW', 'UNRESOLVED')),
    CONSTRAINT ck_entity_match_candidate_status CHECK(status IN ('AUTO_CONFIRMED', 'PENDING', 'CONFIRMED', 'REJECTED', 'UNRESOLVED'))
);

CREATE INDEX idx_entity_match_candidate_review ON entity_match_candidate(job_id, status);
CREATE INDEX idx_entity_match_candidate_entity ON entity_match_candidate(candidate_entity_id);
