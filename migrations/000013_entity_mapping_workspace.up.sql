-- Scope EntityMapping to a workspace and separate the mutable "current mapping"
-- projection from the immutable decision history that produced it.
--
-- This pre-production migration defines the current workspace-scoped mapping
-- contract. No legacy mapping rows are preserved or backfilled.

-- Referenced by the composite workspace-consistency foreign keys below.
ALTER TABLE entity
    ADD CONSTRAINT uq_entity_workspace_id UNIQUE (workspace_id, id);

ALTER TABLE entity_mapping
    ADD COLUMN workspace_id uuid NOT NULL;

ALTER TABLE entity_mapping
    DROP CONSTRAINT uq_entity_mapping_source,
    ADD CONSTRAINT uq_entity_mapping_workspace_source UNIQUE (workspace_id, source_type, source_ref, source_key),
    -- Referenced by entity_mapping_decision so a decision can never point at a
    -- mapping from another workspace.
    ADD CONSTRAINT uq_entity_mapping_workspace_id UNIQUE (workspace_id, id),
    -- Database-level workspace consistency: the mapping and the entity it points
    -- at must live in the same workspace.
    ADD CONSTRAINT fk_entity_mapping_entity FOREIGN KEY (workspace_id, entity_id) REFERENCES entity(workspace_id, id);

CREATE INDEX idx_entity_mapping_workspace_entity ON entity_mapping(workspace_id, entity_id);

-- Current mapping vs immutable decision history. entity_mapping stays the
-- updatable projection used by lookups; entity_mapping_decision appends one
-- immutable row for every accepted mapping decision.
CREATE TABLE entity_mapping_decision (
    id                    uuid PRIMARY KEY,
    -- Monotonic insertion order. decided_at is caller-supplied and can tie, so
    -- the sequence is the authoritative tie-breaker for history reads.
    decided_seq           bigint GENERATED ALWAYS AS IDENTITY,
    workspace_id          uuid NOT NULL,
    mapping_id            uuid NOT NULL,
    entity_id             uuid NOT NULL,
    source_type           varchar(64) NOT NULL,
    source_ref            varchar(512) NOT NULL,
    source_key            varchar(512) NOT NULL,
    source_name           varchar(512),
    match_method          varchar(64) NOT NULL,
    match_rule_id         varchar(128),
    match_policy_version  varchar(64) NOT NULL,
    match_engine_name     varchar(64) NOT NULL,
    match_engine_version  varchar(64) NOT NULL,
    match_model_version   varchar(128),
    confidence            numeric(6,5),
    status                varchar(32) NOT NULL,
    reviewed_by           uuid,
    reviewed_at           timestamptz,
    reviewer_reason       text,
    evidence_id           uuid REFERENCES evidence(id),
    decided_at            timestamptz NOT NULL DEFAULT now(),
    decided_by            uuid,
    CONSTRAINT ck_entity_mapping_decision_status CHECK(status IN ('AUTO_MATCHED', 'CONFIRMED', 'REJECTED', 'CONFLICT')),
    CONSTRAINT fk_entity_mapping_decision_mapping FOREIGN KEY (workspace_id, mapping_id) REFERENCES entity_mapping(workspace_id, id),
    CONSTRAINT fk_entity_mapping_decision_entity FOREIGN KEY (workspace_id, entity_id) REFERENCES entity(workspace_id, id)
);

CREATE INDEX idx_entity_mapping_decision_mapping ON entity_mapping_decision(mapping_id, decided_seq);
CREATE INDEX idx_entity_mapping_decision_source ON entity_mapping_decision(workspace_id, source_type, source_ref, source_key);

-- Decision history is append-only: AGENTS.md §3 forbids overwriting history.
CREATE OR REPLACE FUNCTION prevent_entity_mapping_decision_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'entity_mapping_decision is immutable; append a new decision instead';
END $$ LANGUAGE plpgsql;

CREATE TRIGGER prevent_entity_mapping_decision_mutation
    BEFORE UPDATE OR DELETE ON entity_mapping_decision
    FOR EACH ROW EXECUTE FUNCTION prevent_entity_mapping_decision_mutation();
